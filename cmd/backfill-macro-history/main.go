// Command backfill-macro-history backfills historical daily closes for
// Yahoo-chart-backed macro channels (taiex ^TWII, tsm_adr NYSE:TSM) into
// the per-date macro snapshot files data/state/macro/YYYY-MM-DD.json.
//
// Purpose: the cf-hypotheses validator (cmd/validate-capital-flow-hypotheses)
// needs >=252 TAIEX/TSM-ADR paired days, but the runtime MacroIngestor only
// persists the current day. Yahoo's v8 chart endpoint returns the whole range
// in ONE request per symbol and does not consume FinMind quota.
//
// bdi is backfilled from BDRY (Breakwave Dry Bulk Shipping ETF) as a PROXY:
// the Yahoo chart endpoint 404s on both .BADI and ^BDIY
// (ranaroussi/yfinance#1667). BDRY tracks dry-bulk freight futures
// (Capesize/Panamax/Supramax), so its change_pct is a directional proxy for
// Baltic Dry Index moves — the only shape every bdi consumer reads
// (narrative detectBDIShippingEvent, sector predictor). Absolute values are
// NOT comparable with CNBC .BADI levels; existing non-zero .BADI points are
// never overwritten by the merge discipline below, and the point's symbol
// field records which source produced it.
//
// Merge discipline (same as cmd/macrobackfill): an existing non-zero value in
// a per-date snapshot is NEVER overwritten; the tool only fills missing or
// zero-valued points. Provenance is appended to <out>/backfill_log.jsonl.
//
// Usage:
//
//	backfill-macro-history -workdir . -start 2025-05-01 -end 2026-09-03
//	backfill-macro-history -channels taiex:^TWII -dry-run
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDailyChangePct = 30.0 // same implausible-change gate as YahooStockProvider
	logFileName       = "backfill_log.jsonl"
)

// channel maps a macro snapshot key to its Yahoo chart ticker.
type channel struct {
	field  string // key inside the macro snapshot JSON, e.g. "tsm_adr"
	symbol string // symbol recorded inside the MacroDataPoint, e.g. "TSM"
	ticker string // Yahoo chart ticker, e.g. "TSM"
}

// defaultChannels lists the Yahoo-chart-backed macro snapshot fields a default
// run repairs. bdi maps to BDRY (Breakwave Dry Bulk Shipping ETF) as a proxy
// because the Yahoo chart endpoint returns 404 for both .BADI and ^BDIY
// (ranaroussi/yfinance#1667): BDRY tracks dry-bulk freight futures, so only
// its change_pct is semantically meaningful for bdi consumers — see the
// package comment for the proxy disclosure. The merge discipline never
// overwrites existing non-zero .BADI points captured by the live CNBC channel.
func defaultChannels() string {
	return "taiex:^TWII,tsm_adr:TSM," +
		"vix:^VIX,usd_twd:USDTWD=X,dxy:DX-Y.NYB,us10y:^TNX," +
		"sox_index:^SOX,spx_index:^GSPC,ndx_index:^IXIC,dji_index:^DJI," +
		"nvda:NVDA,aapl:AAPL,msft:MSFT," +
		"oil:CL=F,gold:GC=F,silver:SI=F,copper:HG=F," +
		"jpy:JPY=X,dram_spot_price:MU," +
		"bdi:BDRY"
}

// chartQuote is the subset of the Yahoo v8 chart response this tool needs.
type chartQuote struct {
	Chart struct {
		Result []struct {
			Meta struct {
				ExchangeTimezoneName string `json:"exchangeTimezoneName"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// bar is one trading day of a channel: local exchange date → close/change.
type bar struct {
	date      string
	value     float64
	changePct float64
	ts        int64
}

type logEntry struct {
	Date         string  `json:"date"`
	Field        string  `json:"field"`
	Symbol       string  `json:"symbol"`
	Value        float64 `json:"value"`
	ChangePct    float64 `json:"change_pct"`
	Action       string  `json:"action"` // filled | skipped_existing | skipped_zero_change
	Source       string  `json:"source"`
	BackfilledAt string  `json:"backfilled_at"`
}

// chartURLTemplate is the Yahoo v8 chart endpoint. Overridden in tests.
var chartURLTemplate = "https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d"

// yahooUserAgent is the browser-like UA Yahoo's tightened v8 access expects
// (same pattern as internal/marketdata).
const yahooUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// yahooCookieURL and yahooCrumbURLTemplate back the crumb handshake.
// Overridden in tests.
var (
	yahooCookieURL        = "https://fc.yahoo.com/"
	yahooCrumbURLTemplate = "https://%s/v1/test/getcrumb"
	yahooChartHosts       = []string{"query1.finance.yahoo.com", "query2.finance.yahoo.com"}
)

// yahooAuth holds the cookie+crumb credentials Yahoo's tightened v8 chart
// access expects (mirrors internal/marketdata.yahooSession in miniature):
// one fc.yahoo.com cookie fetch plus one getcrumb call, reused for every
// channel request. Failures are non-fatal — bare requests still work for
// some hosts/IPs, so ensure() logs and continues without credentials.
type yahooAuth struct {
	cookie string
	crumb  string
}

// ensure populates cookie and crumb, best effort. Safe to call once at
// startup; a failed handshake leaves zero values and requests go out bare.
func (a *yahooAuth) ensure(ctx context.Context, client *http.Client) {
	if a == nil || (a.cookie != "" && a.crumb != "") {
		return
	}
	// Step 1: session cookie from fc.yahoo.com.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, yahooCookieURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", yahooUserAgent)
		if resp, err := client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			var fallback string
			for _, c := range resp.Cookies() {
				if c.Name == "A3" || c.Name == "B3" {
					a.cookie = c.Name + "=" + c.Value
					break
				}
				if fallback == "" {
					fallback = c.Name + "=" + c.Value
				}
			}
			if a.cookie == "" {
				a.cookie = fallback
			}
		} else {
			log.Printf("  yahoo auth: cookie fetch failed: %v", err)
		}
	}
	// Step 2: crumb token tied to the session cookie.
	for _, host := range yahooChartHosts {
		u := fmt.Sprintf(yahooCrumbURLTemplate, host)
		cReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		cReq.Header.Set("User-Agent", yahooUserAgent)
		cReq.Header.Set("Referer", "https://finance.yahoo.com/")
		if a.cookie != "" {
			cReq.Header.Set("Cookie", a.cookie)
		}
		cResp, err := client.Do(cReq)
		if err != nil {
			log.Printf("  yahoo auth: crumb host %s failed: %v", host, err)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(cResp.Body, 256))
		_ = cResp.Body.Close()
		if err == nil && cResp.StatusCode == http.StatusOK && len(body) > 0 {
			a.crumb = strings.TrimSpace(string(body))
			break
		}
	}
	if a.crumb == "" {
		log.Printf("  yahoo auth: no crumb obtained (proceeding without it)")
	}
}

// apply attaches the auth headers + crumb query param to a chart request.
// The Referer/Accept headers are set unconditionally so bare requests still
// carry the headers production's yahooSession sends.
func (a *yahooAuth) apply(req *http.Request) {
	req.Header.Set("User-Agent", yahooUserAgent)
	req.Header.Set("Referer", "https://finance.yahoo.com/")
	req.Header.Set("Accept", "application/json")
	if a == nil {
		return
	}
	if a.cookie != "" {
		req.Header.Set("Cookie", a.cookie)
	}
	if a.crumb != "" {
		q := req.URL.Query()
		q.Set("crumb", a.crumb)
		req.URL.RawQuery = q.Encode()
	}
}

func main() {
	var (
		workDir  = flag.String("workdir", ".", "atlas work directory (repo root)")
		startStr = flag.String("start", "2025-05-01", "backfill start date YYYY-MM-DD (inclusive)")
		endStr   = flag.String("end", time.Now().Format("2006-01-02"), "backfill end date YYYY-MM-DD (inclusive)")
		// Default channels cover every Yahoo-chart-backed field in
		// MacroDataSnapshot that the composite live provider fills, so a single
		// backfill run repairs the same field set the runtime would have persisted.
		// jpy is Yahoo-chart-backed here (historical only; live uses the
		// frankfurter_fx channel), and dram_spot_price maps to MU closes.
		// bdi is filled from the BDRY ETF as a change_pct proxy (.BADI/^BDIY
		// both 404 on Yahoo, ranaroussi/yfinance#1667; the live CNBC channel
		// has no history and remains the live source). Also intentionally
		// NOT in this list: taiwan_semi_index, tsmc_revenue (FinMind/TWSE)
		// and the institutional/flow fields.
		chans = flag.String("channels", defaultChannels(),
			"comma list of field:ticker pairs")
		outDir = flag.String("out", "data/state/macro", "output directory under workdir")
		dryRun = flag.Bool("dry-run", false, "fetch nothing: print plan only")
	)
	flag.Parse()

	start, err := time.ParseInLocation("2006-01-02", *startStr, time.Local)
	if err != nil {
		log.Fatalf("parse -start: %v", err)
	}
	end, err := time.ParseInLocation("2006-01-02", *endStr, time.Local)
	if err != nil {
		log.Fatalf("parse -end: %v", err)
	}
	if end.Before(start) {
		log.Fatalf("-end %s before -start %s", *endStr, *startStr)
	}

	var chs []channel
	for _, spec := range strings.Split(*chans, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		parts := strings.SplitN(spec, ":", 2)
		if len(parts) != 2 {
			log.Fatalf("bad -channels entry %q (want field:ticker)", spec)
		}
		ch := channel{field: parts[0], ticker: parts[1], symbol: parts[1]}
		chs = append(chs, ch)
	}
	if len(chs) == 0 {
		log.Fatal("no channels given (-channels)")
	}

	outRoot := filepath.Join(*workDir, *outDir)
	log.Printf("macro-history-backfill: %s..%s channels=%v out=%s dry=%v",
		start.Format("2006-01-02"), end.Format("2006-01-02"), chs, outRoot, *dryRun)
	if *dryRun {
		fmt.Printf("plan: %d channel(s) x 1 Yahoo chart call each = %d Yahoo call(s) (no FinMind quota)\n", len(chs), len(chs))
		return
	}

	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outRoot, err)
	}
	logf, err := os.OpenFile(filepath.Join(outRoot, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("open log: %v", err)
	}
	defer func() { _ = logf.Close() }()

	client := &http.Client{Timeout: 60 * time.Second}
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// One cookie+crumb handshake for the whole run (mirrors the production
	// yahooSession pattern so the tool survives Yahoo's tightened v8 access).
	auth := &yahooAuth{}
	auth.ensure(ctx, client)

	for _, ch := range chs {
		bars, err := fetchHistory(ctx, client, auth, ch, start, end)
		if err != nil {
			log.Printf("  %s (%s): ERROR: %v", ch.field, ch.ticker, err)
			continue
		}
		if len(bars) == 0 {
			log.Printf("  %s (%s): no bars in range", ch.field, ch.ticker)
			continue
		}
		var filled, skipped int
		for _, b := range bars {
			action, err := mergeBar(outRoot, ch, b)
			if err != nil {
				log.Printf("  %s %s: merge error: %v", ch.field, b.date, err)
				continue
			}
			writeLog(logf, logEntry{
				Date: b.date, Field: ch.field, Symbol: ch.symbol,
				Value: b.value, ChangePct: b.changePct, Action: action,
				Source: "yahoo:v8-chart:" + ch.ticker, BackfilledAt: now,
			})
			switch action {
			case "filled":
				filled++
			default:
				skipped++
			}
		}
		log.Printf("  %s (%s): %d bars %s..%s, filled=%d skipped_existing=%d",
			ch.field, ch.ticker, len(bars), bars[0].date, bars[len(bars)-1].date, filled, skipped)
	}
}

// fetchHistory pulls the whole daily range for one ticker in one request and
// computes per-date closes with prev-close change percentages. A nil auth
// sends bare requests (used by tests against httptest servers).
func fetchHistory(ctx context.Context, client *http.Client, auth *yahooAuth, ch channel, start, end time.Time) ([]bar, error) {
	// Pad 10 days before start so the first in-range bar has a valid prev close.
	p1 := start.AddDate(0, 0, -10).Unix()
	p2 := end.AddDate(0, 0, 1).Unix()
	u := fmt.Sprintf(chartURLTemplate, url.PathEscape(ch.ticker), p1, p2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	auth.apply(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("yahoo %s: HTTP %d: %s", ch.ticker, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var q chartQuote
	if err := json.Unmarshal(body, &q); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", ch.ticker, err)
	}
	if q.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo %s error: [%s] %s", ch.ticker, q.Chart.Error.Code, q.Chart.Error.Description)
	}
	if len(q.Chart.Result) == 0 || len(q.Chart.Result[0].Timestamp) == 0 {
		return nil, fmt.Errorf("yahoo %s: no chart result", ch.ticker)
	}
	res := q.Chart.Result[0]
	if len(res.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("yahoo %s: no quote section", ch.ticker)
	}
	closes := res.Indicators.Quote[0].Close
	if len(closes) != len(res.Timestamp) {
		return nil, fmt.Errorf("yahoo %s: timestamp/close length mismatch (%d vs %d)",
			ch.ticker, len(res.Timestamp), len(closes))
	}
	loc, err := time.LoadLocation(res.Meta.ExchangeTimezoneName)
	if err != nil {
		return nil, fmt.Errorf("exchange tz %q: %w", res.Meta.ExchangeTimezoneName, err)
	}

	type dated struct {
		date  string
		ts    int64
		close float64
	}
	var rows []dated
	seen := map[string]bool{}
	for i, ts := range res.Timestamp {
		if closes[i] == nil || *closes[i] <= 0 {
			continue
		}
		t := time.Unix(ts, 0).In(loc)
		day := t.Format("2006-01-02")
		if seen[day] {
			continue // keep the first bar of each exchange date
		}
		if day < start.Format("2006-01-02") || day > end.Format("2006-01-02") {
			// Still recorded: out-of-range pad days provide the prev close.
			rows = append(rows, dated{date: day, ts: ts, close: *closes[i]})
			seen[day] = true
			continue
		}
		rows = append(rows, dated{date: day, ts: ts, close: *closes[i]})
		seen[day] = true
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].date < rows[j].date })

	var out []bar
	var prevClose float64
	for _, r := range rows {
		b := bar{date: r.date, value: r.close, ts: r.ts}
		if prevClose > 0 {
			chg := (r.close/prevClose - 1) * 100
			if chg > maxDailyChangePct || chg < -maxDailyChangePct {
				prevClose = r.close
				continue // implausible daily change: reject the bar
			}
			b.changePct = chg
		}
		if r.date >= start.Format("2006-01-02") && (b.changePct != 0 || prevClose > 0) {
			out = append(out, b)
		}
		prevClose = r.close
	}
	return out, nil
}

// mergeBar fills a missing/zero point inside data/state/macro/<date>.json.
// Existing non-zero values are never overwritten.
func mergeBar(outRoot string, ch channel, b bar) (string, error) {
	path := filepath.Join(outRoot, b.date+".json")
	snap := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &snap); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	}

	if raw, ok := snap[ch.field]; ok {
		var existing struct {
			Symbol    string  `json:"symbol"`
			Value     float64 `json:"value"`
			ChangePct float64 `json:"change_pct"`
		}
		if err := json.Unmarshal(raw, &existing); err == nil {
			if existing.Value != 0 {
				return "skipped_existing", nil
			}
			// ADR history matters through change_pct (validator input);
			// keep an existing non-zero change_pct even when value is 0.
			if ch.field == "tsm_adr" && existing.ChangePct != 0 {
				return "skipped_existing", nil
			}
		}
	}
	data, err := json.Marshal(map[string]interface{}{
		"symbol":     ch.symbol,
		"value":      b.value,
		"change_pct": b.changePct,
		"timestamp":  b.ts,
	})
	if err != nil {
		return "", err
	}
	snap[ch.field] = data
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return "", err
	}
	return "filled", nil
}

func writeLog(f *os.File, e logEntry) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}
