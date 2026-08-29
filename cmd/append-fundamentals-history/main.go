// append-fundamentals-history appends the daily fundamentals.json snapshot to
// data/fundamentals_history.jsonl as one ndjson line per symbol.
//
// It is intentionally a standalone CLI: it does not register as a BTM task,
// does not call any database, and only depends on the standard library. The
// output is meant to accumulate an honest point-in-time history of PE/PB/PS
// and dividend yield starting from the day it is first run.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// taipeiOffsetSeconds is UTC+8, the timezone used by the Taiwan stock exchange.
const taipeiOffsetSeconds = 8 * 60 * 60

// rawFundamental is the JSON shape of a single symbol inside
// data/fundamentals.json. Pointer fields let us distinguish a missing field
// from a zero value so that incomplete rows can be skipped.
type rawFundamental struct {
	PE            *float64 `json:"PE"`
	PB            *float64 `json:"PB"`
	PS            *float64 `json:"PS,omitempty"`
	DividendYield *float64 `json:"DividendYield"`
	Sector        *string  `json:"Sector"`
}

// historyRecord is one line of fundamentals_history.jsonl. Field names use
// snake_case to stay consistent with the rest of the data/ directory.
type historyRecord struct {
	Date          string   `json:"date"`
	Symbol        string   `json:"symbol"`
	PE            float64  `json:"pe"`
	PB            float64  `json:"pb"`
	PS            *float64 `json:"ps,omitempty"`
	DividendYield float64  `json:"dividend_yield"`
	Sector        string   `json:"sector"`
	RecordedAt    string   `json:"recorded_at"`
}

// app holds the CLI configuration and a replaceable clock for tests.
type app struct {
	workDir string
	date    string
	force   bool
	now     func() time.Time
}

func main() {
	a := app{now: time.Now}
	flag.StringVar(&a.workDir, "workdir", ".", "atlas repo root")
	flag.StringVar(&a.date, "date", "", "trading date (YYYY-MM-DD); default auto-detect from -workdir timezone")
	flag.BoolVar(&a.force, "force", false, "force overwrite existing records for the resolved date")
	flag.Parse()

	if err := a.run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (a *app) run() error {
	tradingDate, err := a.resolveTradingDate()
	if err != nil {
		return err
	}

	records, err := a.loadAndValidate(tradingDate)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		log.Printf("[append-fundamentals] no valid symbols for %s; nothing written", tradingDate)
		return nil
	}

	written, kept, removed, err := a.writeHistory(tradingDate, records)
	if err != nil {
		return err
	}

	log.Printf("[append-fundamentals] date=%s written=%d kept=%d removed=%d", tradingDate, written, kept, removed)
	return nil
}

// resolveTradingDate returns the -date flag if set, otherwise the most recent
// trading day relative to a.now(). After market close (>= 13:30 TPE) we use
// today; before close we roll back to the previous weekday. Weekends roll back
// to the preceding Friday.
func (a *app) resolveTradingDate() (string, error) {
	if a.date != "" {
		if _, err := time.Parse("2006-01-02", a.date); err != nil {
			return "", fmt.Errorf("invalid -date %q: %w", a.date, err)
		}
		return a.date, nil
	}

	loc := time.FixedZone("Asia/Taipei", taipeiOffsetSeconds)
	t := a.now().In(loc)

	if t.Hour() < 13 || (t.Hour() == 13 && t.Minute() < 30) {
		return previousWeekday(t.AddDate(0, 0, -1)), nil
	}

	// After market close. If today is Saturday or Sunday, roll back to Friday.
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, -1).Format("2006-01-02"), nil
	case time.Sunday:
		return t.AddDate(0, 0, -2).Format("2006-01-02"), nil
	default:
		return t.Format("2006-01-02"), nil
	}
}

// previousWeekday walks backwards from t until it lands on a weekday.
func previousWeekday(t time.Time) string {
	for {
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
			break
		}
		t = t.AddDate(0, 0, -1)
	}
	return t.Format("2006-01-02")
}

// loadAndValidate reads data/fundamentals.json and builds history records for
// every symbol with the required fields present. Malformed or incomplete
// symbols are logged and skipped instead of failing the whole run.
func (a *app) loadAndValidate(tradingDate string) (records []historyRecord, err error) {
	path := filepath.Join(a.workDir, "data", "fundamentals.json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("fundamentals file not found: %s", path)
		}
		return nil, fmt.Errorf("open fundamentals: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close fundamentals: %w", cerr)
		}
	}()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode fundamentals: %w", err)
	}

	loc := time.FixedZone("Asia/Taipei", taipeiOffsetSeconds)
	dateT, err := time.Parse("2006-01-02", tradingDate)
	if err != nil {
		return nil, fmt.Errorf("parse trading date: %w", err)
	}
	recordedAt := time.Date(
		dateT.Year(), dateT.Month(), dateT.Day(),
		15, 30, 0, 0, loc,
	).Format(time.RFC3339)

	symbols := make([]string, 0, len(raw))
	for sym := range raw {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	records = make([]historyRecord, 0, len(symbols))
	for _, sym := range symbols {
		var data rawFundamental
		if err := json.Unmarshal(raw[sym], &data); err != nil {
			log.Printf("[append-fundamentals] skipping %s: invalid JSON: %v", sym, err)
			continue
		}
		if data.PE == nil || data.PB == nil || data.DividendYield == nil || data.Sector == nil {
			log.Printf("[append-fundamentals] skipping %s: missing required field", sym)
			continue
		}

		symbol := normalizeSymbol(sym)
		records = append(records, historyRecord{
			Date:          tradingDate,
			Symbol:        symbol,
			PE:            *data.PE,
			PB:            *data.PB,
			PS:            data.PS,
			DividendYield: *data.DividendYield,
			Sector:        *data.Sector,
			RecordedAt:    recordedAt,
		})
	}

	return records, nil
}

// normalizeSymbol strips the .TW/.TWO suffix that fundamentals.json uses as
// a canonical key so that downstream history consumers see the bare stock ID.
func normalizeSymbol(symbol string) string {
	for _, suf := range []string{".TW", ".TWO"} {
		if before, ok := strings.CutSuffix(symbol, suf); ok {
			return before
		}
	}
	return symbol
}

// writeHistory rewrites data/fundamentals_history.jsonl, keeping every record
// whose date differs from tradingDate and appending the new records. The -force
// flag is accepted and results in the same delete-and-rewrite behavior, making
// the CLI idempotent by default.
func (a *app) writeHistory(tradingDate string, records []historyRecord) (written, kept, removed int, err error) {
	histPath := filepath.Join(a.workDir, "data", "fundamentals_history.jsonl")
	tmpPath := histPath + ".tmp"

	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("create temp history file: %w", err)
	}
	defer func() {
		_ = out.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	bw := bufio.NewWriter(out)

	if _, statErr := os.Stat(histPath); statErr == nil {
		in, err := os.Open(histPath)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("open history file: %w", err)
		}
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			line := scanner.Bytes()
			var rec struct {
				Date string `json:"date"`
			}
			if json.Unmarshal(line, &rec) == nil && rec.Date == tradingDate {
				removed++
				continue
			}
			if _, err := bw.Write(line); err != nil {
				_ = in.Close()
				return 0, 0, 0, fmt.Errorf("write history line: %w", err)
			}
			if err := bw.WriteByte('\n'); err != nil {
				_ = in.Close()
				return 0, 0, 0, fmt.Errorf("write newline: %w", err)
			}
			kept++
		}
		_ = in.Close()
		if err := scanner.Err(); err != nil {
			return 0, 0, 0, fmt.Errorf("read history file: %w", err)
		}
	}

	enc := json.NewEncoder(bw)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return 0, 0, 0, fmt.Errorf("encode record: %w", err)
		}
		written++
	}

	if err := bw.Flush(); err != nil {
		return 0, 0, 0, fmt.Errorf("flush history file: %w", err)
	}
	if err := out.Close(); err != nil {
		return 0, 0, 0, fmt.Errorf("close history file: %w", err)
	}

	if err := os.Rename(tmpPath, histPath); err != nil {
		return 0, 0, 0, fmt.Errorf("replace history file: %w", err)
	}
	return written, kept, removed, nil
}
