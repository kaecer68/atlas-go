package marketdata

// US CPI (YoY) provider backed by the BLS public API.
//
// Why: the narrative detectors inflation_cool (L5 "通膨降，科技漲") and
// inflation_moderate (L6 "通膨溫，銀行漲") read MarketNarrativeData.CPIYoY, and
// MacroDataSnapshot.CPIYoY existed as a field — but no channel or provider ever
// populated it, so both detectors were inert in production (found during iMac
// acceptance, 2026-09-16).
//
// Source: US Bureau of Labor Statistics public API v1, series CUUR0000SA0
// (CPI-U, all items, US city average, not seasonally adjusted). v1 needs no API
// key and allows a handful of queries per day, which matches the daily cadence.
// The provider derives YoY (vs the same month a year ago) and MoM changes from
// the monthly index levels; a config value (Narrative.InflationEstimate) stays
// the fallback if the fetch fails.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	blsCPIEndpoint = "https://api.bls.gov/publicAPI/v1/timeseries/data/"
	blsCPISeriesID = "CUUR0000SA0"
	// blsCPILookbackYears covers "same month last year" plus one extra year of
	// history so an early-January run can still find 13 comparable months.
	blsCPILookbackYears = 3
)

// BLSCPIProvider fetches the US CPI-U index level and derives YoY/MoM changes.
type BLSCPIProvider struct {
	endpoint string
	seriesID string
	client   *http.Client
	nowFn    func() time.Time
}

// NewBLSCPIProvider creates a provider using the public BLS endpoint.
func NewBLSCPIProvider() *BLSCPIProvider {
	return &BLSCPIProvider{
		endpoint: blsCPIEndpoint,
		seriesID: blsCPISeriesID,
		client:   &http.Client{Timeout: 20 * time.Second},
		nowFn:    time.Now,
	}
}

// Name returns the data channel identifier.
func (p *BLSCPIProvider) Name() string { return "us_cpi" }

type blsCPIObservation struct {
	year   int
	month  int
	period string
	value  float64
}

// FetchSnapshot fetches the latest CPI-U level and computes YoY/MoM percentages.
func (p *BLSCPIProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	now := p.nowFn()
	body, err := json.Marshal(map[string]any{
		"seriesid":  []string{p.seriesID},
		"startyear": strconv.Itoa(now.Year() - blsCPILookbackYears),
		"endyear":   strconv.Itoa(now.Year()),
	})
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi http status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	// Decoded with an inline anonymous struct on purpose: gentags emits any
	// file-scope struct that carries json tags into the frontend type bundle,
	// and these BLS wire types are not part of the API surface.
	// NOTE: the BLS API returns "message" as an ARRAY of strings (empty on
	// success, e.g. ["Series does not exist"] on failure) — see the production
	// failure "cannot unmarshal array into Go struct field .message of type
	// string" (2026-09-16). Typed as []string on purpose.
	var wire struct {
		Status  string   `json:"status"`
		Message []string `json:"message"`
		Results struct {
			Series []struct {
				Data []struct {
					Year   string `json:"year"`
					Period string `json:"period"`
					Value  string `json:"value"`
				} `json:"data"`
			} `json:"series"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi unmarshal: %w", err)
	}
	if wire.Status != "REQUEST_SUCCEEDED" {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi bls status %q: %s", wire.Status, strings.Join(wire.Message, "; "))
	}
	if len(wire.Results.Series) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi: empty series in response")
	}

	obs := parseBLSObservations(wire.Results.Series[0].Data)
	if len(obs) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi: no monthly observations parsed")
	}

	latest := obs[0]
	yoy, ok := findYearOverYear(obs, latest)
	if !ok {
		return MacroDataSnapshot{}, fmt.Errorf("us_cpi: no same-month observation for YoY (%d-%02d)", latest.year, latest.month)
	}

	var mom float64
	if len(obs) > 1 {
		mom = blsPctChange(obs[1].value, latest.value)
	}

	return MacroDataSnapshot{
		CPIYoY: MacroDataPoint{
			Symbol:    p.seriesID,
			Value:     blsPctChange(yoy, latest.value),
			ChangePct: mom,
			Timestamp: now.Unix(),
		},
	}, nil
}

// parseBLSObservations converts BLS monthly entries to observations sorted by
// recency (newest first), skipping annual averages (period "M13") and any
// unparseable rows.
func parseBLSObservations(entries []struct {
	Year   string `json:"year"`
	Period string `json:"period"`
	Value  string `json:"value"`
}) []blsCPIObservation {
	out := make([]blsCPIObservation, 0, len(entries))
	for _, e := range entries {
		if !strings.HasPrefix(e.Period, "M") || e.Period == "M13" {
			continue
		}
		month, err := strconv.Atoi(strings.TrimPrefix(e.Period, "M"))
		if err != nil || month < 1 || month > 12 {
			continue
		}
		year, err := strconv.Atoi(e.Year)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(e.Value), 64)
		if err != nil || value <= 0 {
			continue
		}
		out = append(out, blsCPIObservation{year: year, month: month, period: e.Period, value: value})
	}
	sortByRecency(out)
	return out
}

// sortByRecency sorts observations newest-first (insertion sort keeps this
// dependency-free; the slice is at most a few dozen entries).
func sortByRecency(obs []blsCPIObservation) {
	for i := 1; i < len(obs); i++ {
		for j := i; j > 0; j-- {
			newer := obs[j].year > obs[j-1].year ||
				(obs[j].year == obs[j-1].year && obs[j].month > obs[j-1].month)
			if !newer {
				break
			}
			obs[j], obs[j-1] = obs[j-1], obs[j]
		}
	}
}

// findYearOverYear returns the level for the same month one year earlier.
func findYearOverYear(obs []blsCPIObservation, latest blsCPIObservation) (float64, bool) {
	for _, o := range obs {
		if o.year == latest.year-1 && o.month == latest.month {
			return o.value, true
		}
	}
	return 0, false
}

// blsPctChange returns the percentage change from prev to curr.
func blsPctChange(prev, curr float64) float64 {
	if prev == 0 {
		return 0
	}
	return (curr - prev) / prev * 100
}
