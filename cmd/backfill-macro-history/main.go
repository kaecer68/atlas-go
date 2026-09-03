// Command backfill-macro-history backfills historical daily closes for
// Yahoo-chart-backed macro channels (taiex ^TWII, tsm_adr NYSE:TSM) into
// the per-date macro snapshot files data/state/macro/YYYY-MM-DD.json.
//
// Purpose: the cf-hypotheses validator (cmd/validate-capital-flow-hypotheses)
// needs >=252 TAIEX/TSM-ADR paired days, but the runtime MacroIngestor only
// persists the current day. Yahoo's v8 chart endpoint returns the whole range
// in ONE request per symbol and does not consume FinMind quota.
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

var defaultChannels = []channel{
	{field: "taiex", symbol: "^TWII", ticker: "^TWII"},
	{field: "tsm_adr", symbol: "TSM", ticker: "TSM"},
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

func main() {
	var (
		workDir  = flag.String("workdir", ".", "atlas work directory (repo root)")
		startStr = flag.String("start", "2025-05-01", "backfill start date YYYY-MM-DD (inclusive)")
		endStr   = flag.String("end", time.Now().Format("2006-01-02"), "backfill end date YYYY-MM-DD (inclusive)")
		chans    = flag.String("channels", "taiex:^TWII,tsm_adr:TSM", "comma list of field:ticker pairs")
		outDir   = flag.String("out", "data/state/macro", "output directory under workdir")
		dryRun   = flag.Bool("dry-run", false, "fetch nothing: print plan only")
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
	defer logf.Close()

	client := &http.Client{Timeout: 60 * time.Second}
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	for _, ch := range chs {
		bars, err := fetchHistory(ctx, client, ch, start, end)
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
// computes per-date closes with prev-close change percentages.
func fetchHistory(ctx context.Context, client *http.Client, ch channel, start, end time.Time) ([]bar, error) {
	// Pad 10 days before start so the first in-range bar has a valid prev close.
	p1 := start.AddDate(0, 0, -10).Unix()
	p2 := end.AddDate(0, 0, 1).Unix()
	u := fmt.Sprintf(chartURLTemplate, url.PathEscape(ch.ticker), p1, p2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
