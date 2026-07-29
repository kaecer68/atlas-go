// Command macrobackfill backfills a single missing field in a MacroDataSnapshot
// from the TWSE OpenAPI. Scope is intentionally limited to the `taiex` field
// and to a single date per invocation. Refuses to overwrite existing values.
//
// Provenance is recorded in <dir>/backfill_log.jsonl instead of by mutating
// the MacroDataPoint schema.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	fieldTAIEX = "taiex"
	taiexSymbol = "^TWII"
)

var twseMIIndexURL = "https://www.twse.com.tw/exchangeReport/MI_INDEX"

type macroDataPoint struct {
	Symbol    string  `json:"symbol"`
	Value     float64 `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Timestamp int64   `json:"timestamp"`
}

type snapshot map[string]json.RawMessage

// twseIndexRow is one row in the TWSE MI_INDEX IND table. We model it
// positionally via the fields array because the API returns the row as
// a JSON array (not an object) in some endpoints; we re-parse positionally
// in fetchTWSEClosingIndex. The struct below is only used when the
// `data` array is an array of objects keyed by the Chinese field names.
type twseIndexRow struct {
	Index     string `json:"指數"`
	Closing   string `json:"收盤指數"`
	Change    string `json:"漲跌點數"`
	ChangePct string `json:"漲跌百分比(%)"`
}

type twseResponse struct {
	Stat   string `json:"stat"`
	Tables []struct {
		Title  string     `json:"title"`
		Fields []string   `json:"fields"`
		Data   [][]string `json:"data"`
	} `json:"tables"`
}

type backfillLogEntry struct {
	Date            string  `json:"date"`
	Field           string  `json:"field"`
	Value           float64 `json:"value"`
	ChangePct       float64 `json:"change_pct"`
	SourceURL       string  `json:"source_url"`
	SourceFetchedAt int64   `json:"source_fetched_at"`
	BackfilledAt    int64   `json:"backfilled_at"`
	BaselineDate    string  `json:"baseline_date"`
	BaselineValue   float64 `json:"baseline_value"`
}

type args struct {
	dir   string
	date  string
	field string
}

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("macrobackfill: %v", err)
	}
}

// runFromOSArgs parses os.Args via the package-level flag package and calls run.
func runFromOSArgs() error {
	var (
		dir   = flag.String("dir", "", "path to macro snapshot directory (e.g. data/state/macro)")
		date  = flag.String("date", "", "snapshot date in YYYY-MM-DD (TW local)")
		field = flag.String("field", fieldTAIEX, "field name (only \"taiex\" supported)")
	)
	flag.Parse()
	return run(args{dir: *dir, date: *date, field: *field})
}

// run executes the backfill. Exposed for tests.
func run(a args) error {
	if a.dir == "" || a.date == "" {
		return errors.New("usage: macrobackfill --dir <snapshot-dir> --date <YYYY-MM-DD> --field taiex")
	}
	if a.field != fieldTAIEX {
		return fmt.Errorf("only --field taiex is supported (got %q)", a.field)
	}
	dir := a.dir
	date := a.date

	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return fmt.Errorf("load Asia/Taipei timezone: %w", err)
	}
	target, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return fmt.Errorf("parse --date: %w", err)
	}
	targetMidnight := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, loc)
	if target.Weekday() == time.Saturday || target.Weekday() == time.Sunday {
		return fmt.Errorf("refuse: %s is a weekend", date)
	}

	// 1. Read target snapshot
	snapPath := filepath.Join(dir, date+".json")
	raw, err := os.ReadFile(snapPath)
	if err != nil {
		return fmt.Errorf("read snapshot %s: %w", snapPath, err)
	}
	var snap snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return fmt.Errorf("parse snapshot %s: %w", snapPath, err)
	}

	// 2. Refuse if taiex already present and non-zero
	if existing, ok := snap[fieldTAIEX]; ok {
		var probe macroDataPoint
		if err := json.Unmarshal(existing, &probe); err == nil {
			if probe.Symbol != "" || probe.Value != 0 || probe.Timestamp != 0 {
				return fmt.Errorf("refuse: %s already has a `taiex` entry: %s", snapPath, string(existing))
			}
		}
	}

	// 3. Find the most recent prior date with a non-zero taiex in the same directory.
	baseline, baselineValue, err := findMostRecentBaseline(dir, date)
	if err != nil {
		return fmt.Errorf("find baseline: %w", err)
	}
	if baseline == "" {
		log.Printf("warning: no prior taiex baseline found; change_pct will be 0")
	}

	// 4. Fetch TAIEX close from TWSE OpenAPI
	value, fetchAt, err := fetchTWSEClosingIndex(target)
	if err != nil {
		return fmt.Errorf("fetch TWSE: %w", err)
	}

	// 5. Compute change_pct from baseline
	var changePct float64
	if baselineValue != 0 {
		changePct = (value - baselineValue) / baselineValue * 100
	}

	point := macroDataPoint{
		Symbol:    taiexSymbol,
		Value:     round2(value),
		ChangePct: round4(changePct),
		Timestamp: targetMidnight.Unix(),
	}
	pointBytes, err := json.Marshal(point)
	if err != nil {
		return fmt.Errorf("marshal taiex point: %w", err)
	}

	// 6. Insert taiex key, preserving existing key order, appending at end.
	merged, err := rewriteMergePreservingOrder(raw, fieldTAIEX, pointBytes)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}

	// 7. Write back atomically
	tmp := snapPath + ".tmp"
	if err := os.WriteFile(tmp, merged, 0644); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, snapPath); err != nil {
		return fmt.Errorf("rename tmp→final: %w", err)
	}

	// 8. Append provenance log
	logEntry := backfillLogEntry{
		Date:            date,
		Field:           fieldTAIEX,
		Value:           point.Value,
		ChangePct:       point.ChangePct,
		SourceURL:       twseURLForDate(target),
		SourceFetchedAt: fetchAt,
		BackfilledAt:    time.Now().Unix(),
		BaselineDate:    baseline,
		BaselineValue:   round2(baselineValue),
	}
	if err := appendLog(dir, logEntry); err != nil {
		return fmt.Errorf("append backfill log: %w", err)
	}

	log.Printf("OK  backfilled %s/%s = %.2f (change_pct=%.4f, baseline=%s=%.2f)",
		date, fieldTAIEX, point.Value, point.ChangePct, baseline, baselineValue)
	return nil
}

// findMostRecentBaseline scans all earlier snapshot files in dir and returns
// the most recent date that has a non-zero taiex value.
func findMostRecentBaseline(dir, target string) (string, float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}
	var candidates []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == "latest.json" || name == "previous.json" {
			continue
		}
		if !looksLikeDate(name) {
			continue
		}
		dateStr := strings.TrimSuffix(name, ".json")
		if dateStr >= target {
			continue
		}
		candidates = append(candidates, dateStr)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))

	for _, dateStr := range candidates {
		raw, err := os.ReadFile(filepath.Join(dir, dateStr+".json"))
		if err != nil {
			continue
		}
		var s snapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		rawPt, ok := s[fieldTAIEX]
		if !ok {
			continue
		}
		var pt macroDataPoint
		if err := json.Unmarshal(rawPt, &pt); err != nil {
			continue
		}
		if pt.Value == 0 {
			continue
		}
		return dateStr, pt.Value, nil
	}
	return "", 0, nil
}

// fetchTWSEClosingIndex calls TWSE OpenAPI and parses the TAIEX closing index.
func fetchTWSEClosingIndex(target time.Time) (float64, int64, error) {
	dateStr := target.Format("20060102")
	url := fmt.Sprintf("%s?response=json&date=%s&type=IND", twseMIIndexURL, dateStr)
	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, 0, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}
	var tw twseResponse
	if err := json.Unmarshal(body, &tw); err != nil {
		return 0, 0, fmt.Errorf("parse JSON: %w (body=%s)", err, truncate(string(body), 200))
	}
	if tw.Stat != "OK" {
		return 0, 0, fmt.Errorf("TWSE stat != OK: %q (body=%s)", tw.Stat, truncate(string(body), 200))
	}
	if len(tw.Tables) == 0 {
		return 0, 0, errors.New("TWSE response has no tables")
	}
	for _, row := range tw.Tables[0].Data {
		if len(row) < 2 {
			continue
		}
		indexName := row[0]
		// TWSE real response uses "發行量加權股價指數" (and
		// sometimes the English alias "TAIEX" depending on locale).
		if indexName == "發行量加權股價指數" || indexName == "TAIEX" {
			parsed, err := parseTAIEXClosing(row[1])
			if err != nil {
				return 0, 0, fmt.Errorf("parse TAIEX closing %q: %w", row[1], err)
			}
			return parsed, time.Now().Unix(), nil
		}
	}
	return 0, 0, fmt.Errorf("TAIEX row not found in TWSE response for %s", dateStr)
}

func parseTAIEXClosing(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty closing value")
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, err
	}
	return v, nil
}

func twseURLForDate(t time.Time) string {
	return fmt.Sprintf("%s?response=json&date=%s&type=IND", twseMIIndexURL, t.Format("20060102"))
}

// rewriteMergePreservingOrder inserts a new key:value pair at the end of
// the JSON object without reformatting the existing content. The output
// preserves the original file's indentation and key order byte-for-byte,
// so a `git diff` shows only the new key insertion.
func rewriteMergePreservingOrder(raw []byte, newKey string, newValue json.RawMessage) ([]byte, error) {
	// Locate the final closing brace of the top-level object.
	end := len(raw)
	for end > 0 {
		c := raw[end-1]
		if c == '}' {
			break
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		return nil, fmt.Errorf("expected top-level object, got trailing char %q", c)
	}
	if end == 0 {
		return nil, errors.New("empty or non-object input")
	}

	// Decide separator: if the last non-whitespace byte before `}` is a
	// comma, we just append; if it's a `}`, the original object had one
	// key only and we need to insert a leading comma.
	prefix := end - 1
	for prefix > 0 && (raw[prefix-1] == ' ' || raw[prefix-1] == '\t' || raw[prefix-1] == '\n' || raw[prefix-1] == '\r') {
		prefix--
	}
	hasComma := prefix > 0 && raw[prefix-1] == ','

	// Marshal the new key to a valid JSON string.
	keyBytes, err := json.Marshal(newKey)
	if err != nil {
		return nil, err
	}

	// Build the insertion matching the original indentation.
	indent := detectIndent(raw)

	var sb strings.Builder
	sb.Write(raw[:end-1])
	if !hasComma {
		sb.WriteByte(',')
	}
	sb.WriteByte('\n')
	sb.WriteString(indent)
	sb.Write(keyBytes)
	sb.WriteString(": ")
	sb.Write(newValue)
	sb.WriteByte('\n')
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// detectIndent scans the first indented key in the JSON object and returns
// the leading whitespace string used before it. If the file is single-line
// (no indentation), it returns an empty string.
func detectIndent(raw []byte) string {
	openIdx := -1
	for i, b := range raw {
		if b == '{' {
			openIdx = i
			break
		}
	}
	if openIdx == -1 {
		return ""
	}
	nlIdx := -1
	for i := openIdx + 1; i < len(raw); i++ {
		if raw[i] == '\n' {
			nlIdx = i
			break
		}
		if raw[i] == '}' {
			return ""
		}
	}
	if nlIdx == -1 {
		return ""
	}
	var indent []byte
	for i := nlIdx + 1; i < len(raw); i++ {
		c := raw[i]
		if c == ' ' || c == '\t' {
			indent = append(indent, c)
			continue
		}
		break
	}
	return string(indent)
}

func appendLog(dir string, entry backfillLogEntry) error {
	logPath := filepath.Join(dir, "backfill_log.jsonl")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(entry)
}

func looksLikeDate(name string) bool {
	const want = "2006-01-02.json"
	if len(name) != len(want) {
		return false
	}
	for i, c := range name {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		case 10:
			if c != '.' {
				return false
			}
		case 11, 12, 13, 14:
			if c != 'j' && c != 's' && c != 'o' && c != 'n' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

func round4(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*10000+0.5)) / 10000
	}
	return float64(int64(v*10000-0.5)) / 10000
}
