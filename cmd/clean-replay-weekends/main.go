// Command clean-replay-weekends scans replay JSONL/CSV datasets, removes rows
// whose date is not a Taiwan trading day (weekends + public holidays per
// marketdata.IsTaiwanTradingDay — the same calendar used by
// cmd/daily-replay-sync runGapBackfill), prints removal statistics, and
// regenerates the merged replay CSV from the remaining rows.
//
// Background (2026-08-23): cron-quote-backfill / backfill-quotes had no
// weekend guard, so R1–R4 backfills wrote "copy of the previous trading day"
// rows for closed-market dates into the replay dataset. Those duplicate rows
// made ForwardReturn's duplicate detection fire for every sample and
// EvaluateModels ended up with no usable data. This tool removes such rows
// from existing replay files; the backfill guards prevent them from
// reappearing.
//
// Usage:
//
//	clean-replay-weekends -inputs a.jsonl,b.csv [-output data/replay/merged.csv] [-rewrite] [-dry-run]
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// row is the flexible replay row model. encoding/json matches keys
// case-insensitively, so both the lowercase (date/symbol/…) and capitalized
// (Date/Symbol/…) jsonl variants in data/replay decode correctly.
type row struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

// fileStats tracks per-file scan results.
type fileStats struct {
	Path           string
	Scanned        int
	Kept           int
	Removed        int
	RemovedWeekend int
	RemovedHoliday int
	SkippedParse   int // lines/records that could not be parsed
}

func (s *fileStats) add(o fileStats) {
	s.Scanned += o.Scanned
	s.Kept += o.Kept
	s.Removed += o.Removed
	s.RemovedWeekend += o.RemovedWeekend
	s.RemovedHoliday += o.RemovedHoliday
	s.SkippedParse += o.SkippedParse
}

func (s fileStats) String() string {
	return fmt.Sprintf("%s: scanned=%d kept=%d removed=%d (weekend=%d holiday=%d skipped_parse=%d)",
		s.Path, s.Scanned, s.Kept, s.Removed, s.RemovedWeekend, s.RemovedHoliday, s.SkippedParse)
}

// classifyDate returns "weekend", "holiday" (weekday public holiday), or
// "trading" for the given date.
func classifyDate(d time.Time) string {
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		return "weekend"
	}
	if !marketdata.IsTaiwanTradingDay(d) {
		return "holiday"
	}
	return "trading"
}

// parseReplayDate parses the date layouts used across data/replay: RFC3339
// ("2024-07-01T00:00:00Z") and plain "2006-01-02".
func parseReplayDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func classifyRow(r row) (string, bool) {
	t, ok := parseReplayDate(r.Date)
	if !ok {
		return "", false
	}
	return classifyDate(t), true
}

// cleanJSONL scans one replay jsonl file. It returns the kept parsed rows,
// the kept raw lines (for -rewrite, preserving original field casing), and
// per-file stats.
func cleanJSONL(path string) (keptRows []row, keptLines [][]byte, st fileStats, err error) {
	st.Path = path
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, st, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		st.Scanned++
		var r row
		if err := json.Unmarshal(line, &r); err != nil {
			st.SkippedParse++
			continue
		}
		kind, ok := classifyRow(r)
		if !ok || kind == "weekend" || kind == "holiday" {
			if ok && kind == "weekend" {
				st.RemovedWeekend++
			}
			if ok && kind == "holiday" {
				st.RemovedHoliday++
			}
			st.Removed++
			continue
		}
		st.Kept++
		keptRows = append(keptRows, r)
		keptLines = append(keptLines, append([]byte(nil), line...))
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, st, fmt.Errorf("read %s: %w", path, err)
	}
	return keptRows, keptLines, st, nil
}

// cleanCSV scans one replay CSV (header: Date,Code,Name,TradeVolume,Open,
// High,Low,Close — the merged.csv layout). Returns kept parsed rows, kept
// raw records (for -rewrite), and per-file stats.
func cleanCSV(path string) (keptRows []row, header []string, keptRecords [][]string, st fileStats, err error) {
	st.Path = path
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, st, err
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	header, err = reader.Read()
	if err != nil {
		return nil, nil, nil, st, fmt.Errorf("read header %s: %w", path, err)
	}
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.TrimSpace(col)] = i
	}
	dateCol, okDate := idx["Date"]
	symCol, okSym := idx["Code"]
	if !okDate || !okSym {
		return nil, nil, nil, st, fmt.Errorf("%s: CSV missing Date/Code columns (got %v)", path, header)
	}

	for {
		rec, err := reader.Read()
		if err != nil {
			break // io.EOF
		}
		st.Scanned++
		dateStr := strings.TrimSpace(rec[dateCol])
		t, ok := parseReplayDate(dateStr)
		if !ok {
			st.SkippedParse++
			continue
		}
		kind := classifyDate(t)
		if kind == "weekend" || kind == "holiday" {
			if kind == "weekend" {
				st.RemovedWeekend++
			} else {
				st.RemovedHoliday++
			}
			st.Removed++
			continue
		}
		st.Kept++
		symbol := strings.TrimSpace(rec[symCol])
		if !strings.HasSuffix(symbol, ".TW") {
			symbol += ".TW"
		}
		keptRows = append(keptRows, row{
			Date:   t.Format("2006-01-02"),
			Symbol: symbol,
			Name:   fieldOr(rec, idx, "Name"),
			Open:   parseFloatOr(fieldOr(rec, idx, "Open")),
			High:   parseFloatOr(fieldOr(rec, idx, "High")),
			Low:    parseFloatOr(fieldOr(rec, idx, "Low")),
			Close:  parseFloatOr(fieldOr(rec, idx, "Close")),
			Volume: parseIntOr(fieldOr(rec, idx, "TradeVolume")),
		})
		keptRecords = append(keptRecords, rec)
	}
	return keptRows, header, keptRecords, st, nil
}

func fieldOr(rec []string, idx map[string]int, name string) string {
	if i, ok := idx[name]; ok && i < len(rec) {
		return rec[i]
	}
	return ""
}

func parseFloatOr(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseIntOr(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

// mergeRows dedups rows by date:code (first wins, like cmd/merge-replay) and
// sorts by date then code.
func mergeRows(rows []row) []row {
	seen := make(map[string]bool, len(rows))
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		date := strings.TrimSuffix(r.Date, "T00:00:00Z")
		code := strings.TrimSuffix(r.Symbol, ".TW")
		key := date + ":" + code
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

func writeMergedCSV(path string, rows []row) (int, error) {
	out, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = out.Close() }()

	w := csv.NewWriter(out)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for _, r := range rows {
		_ = w.Write([]string{
			strings.TrimSuffix(r.Date, "T00:00:00Z"),
			strings.TrimSuffix(r.Symbol, ".TW"),
			r.Name,
			strconv.FormatInt(r.Volume, 10),
			strconv.FormatFloat(r.Open, 'f', -1, 64),
			strconv.FormatFloat(r.High, 'f', -1, 64),
			strconv.FormatFloat(r.Low, 'f', -1, 64),
			strconv.FormatFloat(r.Close, 'f', -1, 64),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func writeLines(path string, lines [][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, err := w.Write(l); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

func writeCSV(path string, header []string, records [][]string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	w := csv.NewWriter(out)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, rec := range records {
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func run(args []string) error {
	fs := flag.NewFlagSet("clean-replay-weekends", flag.ContinueOnError)
	inputsFlag := fs.String("inputs", "", "comma-separated replay files (jsonl and/or csv)")
	outputFlag := fs.String("output", "data/replay/merged.csv", "cleaned merged CSV output path (empty disables merged output)")
	rewrite := fs.Bool("rewrite", false, "rewrite input files in place, keeping only trading-day rows")
	dryRun := fs.Bool("dry-run", false, "report only; write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputsFlag == "" {
		return fmt.Errorf("-inputs required: comma-separated replay jsonl/csv files")
	}

	var allRows []row
	var total fileStats
	var rewritten []string

	for raw := range strings.SplitSeq(*inputsFlag, ",") {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if strings.HasSuffix(strings.ToLower(path), ".csv") {
			keptRows, header, keptRecs, st, err := cleanCSV(path)
			if err != nil {
				return err
			}
			fmt.Println(st.String())
			total.add(st)
			allRows = append(allRows, keptRows...)
			if *rewrite && !*dryRun {
				if err := writeCSV(path, header, keptRecs); err != nil {
					return fmt.Errorf("rewrite %s: %w", path, err)
				}
				rewritten = append(rewritten, path)
			}
		} else {
			keptRows, keptLines, st, err := cleanJSONL(path)
			if err != nil {
				return err
			}
			fmt.Println(st.String())
			total.add(st)
			allRows = append(allRows, keptRows...)
			if *rewrite && !*dryRun {
				if err := writeLines(path, keptLines); err != nil {
					return fmt.Errorf("rewrite %s: %w", path, err)
				}
				rewritten = append(rewritten, path)
			}
		}
	}

	fmt.Printf("total: %s\n", total.String())

	if *outputFlag != "" && !*dryRun {
		merged := mergeRows(allRows)
		n, err := writeMergedCSV(*outputFlag, merged)
		if err != nil {
			return fmt.Errorf("write merged CSV: %w", err)
		}
		fmt.Printf("wrote cleaned merged CSV: %s (%d unique bars)\n", *outputFlag, n)
	}
	if len(rewritten) > 0 {
		fmt.Printf("rewrote %d file(s) in place: %s\n", len(rewritten), strings.Join(rewritten, ", "))
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "clean-replay-weekends:", err)
		os.Exit(1)
	}
}
