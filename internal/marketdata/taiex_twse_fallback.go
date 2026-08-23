package marketdata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

)

var taiexTWSEBaseURL = "https://www.twse.com.tw/exchangeReport/MI_INDEX"

// twseTAIEXResponse matches the JSON returned by TWSE MI_INDEX?type=IND.
type twseTAIEXResponse struct {
	Stat   string `json:"stat"`
	Tables []struct {
		Title  string     `json:"title"`
		Fields []string   `json:"fields"`
		Data   [][]string `json:"data"`
	} `json:"tables"`
}

// twseTAIEXTargetDate is overridable in tests so the fallback request date is
// deterministic regardless of the wall clock.
var twseTAIEXTargetDate = func() time.Time {
	return time.Now().In(twseLocation)
}

// fetchTWSETAIEXFallback fetches the TAIEX closing index from TWSE OpenAPI.
// It is used when the primary Yahoo ^TWII path fails.
//
// The request date is rolled back to the most recent Taiwan trading day, so
// weekend/holiday calls serve the previous close instead of failing. During a
// trading day's pre-market window (00:00–09:00 CST) TWSE has not yet published
// today's MI_INDEX (data:[] until ~14:00), so the target also rewinds to the
// previous trading day — mirroring twiiCacheTimestampIsCurrentTradingDay's
// pre-market pattern (N1 S2). The response's reported date (parsed from the
// table title in ROC calendar) MUST still match the requested date, which
// prevents writing stale data as today's value when the market has not yet
// produced the current report.
func fetchTWSETAIEXFallback(ctx context.Context) (MacroDataPoint, error) {
	// Roll back to the most recent Taiwan trading day. On weekends/holidays the
	// wall-clock date has no MI_INDEX row, so requesting it would always return
	// "TAIEX row not found" and count as a failure. latestTaiwanTradingDay is a
	// no-op on trading days and rewinds to the previous close on non-trading days.
	// Pre-market on a trading day additionally rewinds to the previous trading
	// day (today's row is not published until ~14:00 CST).
	now := twseTAIEXTargetDate()
	target := latestTaiwanTradingDay(now)
	if isTaiwanTradingDay(now) && now.Hour() < twseMarketOpenHour {
		target = latestTaiwanTradingDay(now.AddDate(0, 0, -1))
	}
	dateStr := target.Format("20060102")
	url := fmt.Sprintf("%s?response=json&date=%s&type=IND", taiexTWSEBaseURL, dateStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: create request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	// P1-13: share the TWSEClient's HTTP client AND token bucket instead of
	// building a fresh raw &http.Client per call (which bypassed both the
	// timeout parameter consistency AND the rate limit — a busy cycle could
	// fire this fallback at the same host concurrently with the 11 TWSE
	// providers). The shared client's timeout comes from
	// marketdata.twse_api_timeout_sec (N1 S4: 20s default).
	client := GetSharedTWSEClient().HTTPClient()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := GetSharedTWSEClient().RateLimiter().Wait(waitCtx); err != nil {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: rate limit wait (10s timeout): %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	var tw twseTAIEXResponse
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &tw); err != nil {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: decode response: %w", err)
	}

	if tw.Stat != "OK" {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: TWSE stat != OK: %q", tw.Stat)
	}
	if len(tw.Tables) == 0 {
		return MacroDataPoint{}, errors.New("taiex_twse: TWSE response has no tables")
	}

	// Date guard: the table title contains the ROC date, e.g.
	// "115年07月24日 價格指數(臺灣證券交易所)".
	reportedDate, err := parseTWSETitleDate(tw.Tables[0].Title)
	if err != nil {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: %w", err)
	}
	if !sameDate(reportedDate, target) {
		return MacroDataPoint{}, fmt.Errorf("taiex_twse: reported date %s does not match requested %s (refusing stale/previous-day data)",
			reportedDate.Format("2006-01-02"), target.Format("2006-01-02"))
	}

	for _, row := range tw.Tables[0].Data {
		if len(row) < 5 {
			continue
		}
		indexName := row[0]
		if indexName != "發行量加權股價指數" && indexName != "TAIEX" {
			continue
		}
		value, err := parseTAIEXValue(row[1])
		if err != nil {
			return MacroDataPoint{}, fmt.Errorf("taiex_twse: parse TAIEX closing %q: %w", row[1], err)
		}
		// TWSE returns the official daily change percentage in row[4].
		changePct, err := parseTAIEXValue(row[4])
		if err != nil {
			changePct = 0
		}

		return MacroDataPoint{
			Symbol:    "^TWII",
			Value:     value,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	return MacroDataPoint{}, fmt.Errorf("taiex_twse: TAIEX row not found for %s", dateStr)
}

var twseTitleDateRE = regexp.MustCompile(`(\d{3})年(\d{2})月(\d{2})日`)

func parseTWSETitleDate(title string) (time.Time, error) {
	m := twseTitleDateRE.FindStringSubmatch(title)
	if len(m) != 4 {
		return time.Time{}, fmt.Errorf("cannot parse ROC date from title %q", title)
	}
	rocYear, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	year := rocYear + 1911
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, twseLocation), nil
}

func parseTAIEXValue(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	if s == "" {
		return 0, errors.New("empty value")
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, err
	}
	return v, nil
}
