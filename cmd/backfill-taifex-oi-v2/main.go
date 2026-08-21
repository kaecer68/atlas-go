// Command backfill-taifex-oi-v2 backfills the 三大法人 外資期貨未平倉淨口數
// (foreign_futures_oi_net) into data/state/macro snapshot files for historical
// dates.
//
// Background: the daily TAIFEX OpenAPI channel
// (internal/marketdata/taifex_institutional.go) only returns the latest
// trading session and has no date parameter, so it cannot backfill. The TAIFEX
// website CSV endpoint 區分各期貨契約之交易明細表 (futContractsDateDown) accepts a
// date parameter and can serve any trading day back to 2024-07 and earlier.
// This command fetches that CSV day by day, extracts the 臺股期貨 外資及陸資
// 多空未平倉口數淨額, and merges it into each existing snapshot file
// (macrobackfill mode: refuse-overwrite unless -force).
//
// Usage:
//
//	backfill-taifex-oi-v2 -workdir . -start 2024-07-01 -end 2026-08-21
//	backfill-taifex-oi-v2 -workdir . -start 2024-07-01 -dry-run
//
// Throttling: at least 1500ms between TAIFEX requests (pacing is enforced by
// the CLI), at most 3 retries per date on failure. A single failed date does
// not abort the full run; errors are collected and reported in the summary.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

const (
	taifexFutDownPath = "https://www.taifex.com.tw/cht/3/futContractsDateDown"

	taifexContractTX   = "TXF"   // commodityId for 臺股期貨
	contractNameTX     = "臺股期貨"  // 商品名稱 in the CSV
	traderForeign      = "外資及陸資" // 身份別 in the CSV
	csvHeaderOINet     = "多空未平倉口數淨額"
	snapshotField      = "foreign_futures_oi_net"
	snapshotSymbol     = "TX_FOREIGN_OI_NET"
	defaultStartDate   = "2024-07-01"
	backfillLogName    = "backfill_log.jsonl"
	minPacingMillis    = 1500
	maxRetriesDefault  = 3
	httpTimeoutSeconds = 30
)

// errRateLimited marks a response that was throttled by the TAIFEX WAF
// (HTTP 429 / "Just a moment" challenge). Retries for this condition use a
// longer cooldown so the block can lift.
var errRateLimited = errors.New("TAIFEX rate-limited (HTTP 429 / challenge)")

// macroDataPoint mirrors the snapshot point shape for foreign_futures_oi_net.
type macroDataPoint struct {
	Symbol    string  `json:"symbol"`
	Value     float64 `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Timestamp int64   `json:"timestamp"`
}

// backfillLogEntry records one backfill provenance row (same shape as
// cmd/macrobackfill so both tools share data/state/macro/backfill_log.jsonl).
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

type config struct {
	workDir    string
	start      time.Time
	end        time.Time
	pacing     time.Duration
	maxRetries int
	batchDays  int // 0 = per-day; N = fetch N-calendar-day ranges per request
	dryRun     bool
	force      bool
}

type runStats struct {
	dates           int
	fetched         int
	noData          int
	carriedForward  int
	merged          int
	overwritten     int
	alreadyPresent  int
	noSnapshotFile  int
	fetchErrors     int
	fetchErrorDates []string
}

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("backfill-taifex-oi-v2: %v", err)
	}
}

// runFromOSArgs parses os.Args via the package-level flag package and calls run.
func runFromOSArgs() error {
	var (
		workDir   = flag.String("workdir", ".", "atlas repo root (must contain data/state/macro)")
		start     = flag.String("start", defaultStartDate, "backfill start date YYYY-MM-DD")
		end       = flag.String("end", "", "backfill end date YYYY-MM-DD (default: today Asia/Taipei)")
		pacing    = flag.Int("pacing", minPacingMillis, "milliseconds between TAIFEX requests (min 1500)")
		retries   = flag.Int("max-retries", maxRetriesDefault, "fetch retries per date (0..3)")
		batchDays = flag.Int("batch-days", 0, "fetch N-calendar-day ranges per request (0 = per-day; use e.g. 21 for bulk backfill to reduce request count)")
		dryRun    = flag.Bool("dry-run", false, "print what would be written without touching files")
		force     = flag.Bool("force", false, "overwrite existing non-zero foreign_futures_oi_net values")
	)
	flag.Parse()

	if *pacing < minPacingMillis {
		return fmt.Errorf("--pacing must be >= %dms (got %d)", minPacingMillis, *pacing)
	}
	if *retries < 0 || *retries > maxRetriesDefault {
		return fmt.Errorf("--max-retries must be in 0..%d (got %d)", maxRetriesDefault, *retries)
	}
	if *batchDays < 0 || *batchDays > 92 {
		return fmt.Errorf("--batch-days must be in 0..92 (got %d)", *batchDays)
	}

	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return fmt.Errorf("load Asia/Taipei timezone: %w", err)
	}
	startTime, err := time.ParseInLocation("2006-01-02", *start, loc)
	if err != nil {
		return fmt.Errorf("parse --start: %w", err)
	}
	endStr := *end
	if endStr == "" {
		endStr = time.Now().In(loc).Format("2006-01-02")
	}
	endTime, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return fmt.Errorf("parse --end: %w", err)
	}
	if endTime.Before(startTime) {
		return fmt.Errorf("--end %s is before --start %s", endStr, *start)
	}

	return run(config{
		workDir:    *workDir,
		start:      startTime,
		end:        endTime,
		pacing:     time.Duration(*pacing) * time.Millisecond,
		maxRetries: *retries,
		batchDays:  *batchDays,
		dryRun:     *dryRun,
		force:      *force,
	})
}

// run executes the backfill. Exposed for tests.
func run(cfg config) error {
	dir := filepath.Join(cfg.workDir, "data", "state", "macro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure macro dir: %w", err)
	}

	fetcher := &taifexFetcher{
		client:     &http.Client{Timeout: httpTimeoutSeconds * time.Second},
		url:        taifexFutDownURL,
		maxRetries: cfg.maxRetries,
		batchDays:  cfg.batchDays,
	}
	ctx := context.Background()

	var (
		lastValue      float64
		lastSourceDate string
		haveLast       bool
	)
	stats := &runStats{}

	for d := cfg.start; !d.After(cfg.end); d = d.AddDate(0, 0, 1) {
		stats.dates++
		dateStr := d.Format("2006-01-02")

		// macrobackfill mode: only dates that already have a snapshot file can
		// receive the field, so skip (without fetching) everything else.
		snapPath := filepath.Join(dir, dateStr+".json")
		if _, err := os.Stat(snapPath); err != nil {
			stats.noSnapshotFile++
			continue
		}

		value, sourceDate, action, err := dailyValue(ctx, fetcher, d, lastValue, lastSourceDate, haveLast)
		if err != nil {
			stats.fetchErrors++
			stats.fetchErrorDates = append(stats.fetchErrorDates, dateStr)
			fmt.Fprintf(os.Stderr, "[%s] ERROR: %v\n", dateStr, err)
			continue
		}
		switch action {
		case actionFetched:
			lastValue, lastSourceDate, haveLast = value, dateStr, true
			stats.fetched++
		case actionNoData:
			stats.noData++
			if haveLast {
				value, sourceDate = lastValue, lastSourceDate
				action = actionCarryForward
				stats.carriedForward++
			} else {
				fmt.Printf("[%s] no data and no prior baseline → skip\n", dateStr)
				continue
			}
		case actionCarryForward:
			stats.carriedForward++
		}

		if cfg.dryRun {
			fmt.Printf("[%s] DRY-RUN: would merge %s=%.0f (source=%s)\n", dateStr, snapshotField, value, sourceDate)
			continue
		}

		merged, overwrote, err := mergeSnapshot(dir, dateStr, value, cfg.force)
		if err != nil {
			stats.fetchErrors++
			stats.fetchErrorDates = append(stats.fetchErrorDates, dateStr)
			fmt.Fprintf(os.Stderr, "[%s] merge failed: %v\n", dateStr, err)
			continue
		}
		if !merged {
			stats.alreadyPresent++
			fmt.Printf("[%s] %-5s %s=%.0f already present, refuse-overwrite → skip\n", dateStr, action, snapshotField, value)
			continue
		}
		if overwrote {
			stats.overwritten++
		}
		stats.merged++
		fmt.Printf("[%s] %-5s %s=%.0f (source=%s)\n", dateStr, action, snapshotField, value, sourceDate)

		appendLogEntry(dir, backfillLogEntry{
			Date:            dateStr,
			Field:           snapshotField,
			Value:           value,
			ChangePct:       0,
			SourceURL:       taifexFutDownURL,
			SourceFetchedAt: time.Now().Unix(),
			BackfilledAt:    time.Now().Unix(),
			BaselineDate:    sourceDate,
			BaselineValue:   value,
		})
	}

	printSummary(dir, stats)
	return nil
}

type dailyAction int

const (
	actionFetched dailyAction = iota
	actionNoData
	actionCarryForward
)

func (a dailyAction) String() string {
	switch a {
	case actionFetched:
		return "fetch"
	case actionNoData:
		return "nodata"
	case actionCarryForward:
		return "carry"
	default:
		return "unknown"
	}
}

// dailyValue resolves the foreign OI net value for one calendar day.
// Weekends skip the HTTP request and carry the previous value forward.
// Weekday holidays return 查無資料 from TAIFEX and also carry forward.
func dailyValue(ctx context.Context, f *taifexFetcher, d time.Time, lastValue float64, lastSourceDate string, haveLast bool) (float64, string, dailyAction, error) {
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		if !haveLast {
			return 0, "", actionNoData, nil
		}
		return lastValue, lastSourceDate, actionCarryForward, nil
	}
	v, found, err := f.fetchForeignOINet(ctx, d)
	if err != nil {
		return 0, "", actionNoData, err
	}
	if found {
		return v, d.Format("2006-01-02"), actionFetched, nil
	}
	return 0, "", actionNoData, nil
}

// taifexFutDownURL is the TAIFEX website CSV download endpoint (區分各期貨
// 契約之交易明細表, per-contract institutional futures stats). Variables so
// tests can stub the endpoint / shrink cooldowns.
var (
	taifexFutDownURL  = taifexFutDownPath
	rateLimitCooldown = 30 * time.Second // Cloudflare challenge backoff
)

// taifexFetcher performs the TAIFEX website CSV download with retry.
// When batchDays > 0, one request fetches a range of dates and the results
// are cached, so the per-day run loop serves subsequent days from cache.
type taifexFetcher struct {
	client     *http.Client
	url        string
	maxRetries int
	batchDays  int

	cache   map[string]float64 // date (2006-01-02) → foreign OI net
	noData  map[string]bool    // date confirmed without TAIFEX data (holiday)
	cacheLo time.Time          // cached window start (inclusive)
	cacheHi time.Time          // cached window end (inclusive)
}

// fetchForeignOINet downloads the per-contract CSV for one date and returns
// the 臺股期貨 外資及陸資 OI net. found=false means the date has no TAIFEX data
// (holiday). When batchDays > 0 the request covers a range of dates and the
// results are cached for subsequent days. Errors are retried up to maxRetries
// times with backoff.
func (f *taifexFetcher) fetchForeignOINet(ctx context.Context, d time.Time) (float64, bool, error) {
	dateStr := d.Format("2006-01-02")
	if f.batchDays <= 0 {
		form := url.Values{}
		form.Set("queryStartDate", d.Format("2006/01/02"))
		form.Set("queryEndDate", d.Format("2006/01/02"))
		form.Set("commodityId", taifexContractTX)
		return f.fetchWithRetry(ctx, form, d, d, dateStr)
	}

	// Batch mode: serve from cache when the requested date is inside the
	// already-fetched window.
	if f.cache != nil && !d.Before(f.cacheLo) && !d.After(f.cacheHi) {
		if v, ok := f.cache[dateStr]; ok {
			return v, true, nil
		}
		if f.noData[dateStr] {
			return 0, false, nil
		}
	}

	windowEnd := d.AddDate(0, 0, f.batchDays-1)
	form := url.Values{}
	form.Set("queryStartDate", d.Format("2006/01/02"))
	form.Set("queryEndDate", windowEnd.Format("2006/01/02"))
	form.Set("commodityId", taifexContractTX)
	return f.fetchWithRetry(ctx, form, d, windowEnd, dateStr)
}

// fetchWithRetry performs one POST and applies the retry/backoff policy.
// start/end are the requested calendar range; label is used in error logs.
func (f *taifexFetcher) fetchWithRetry(ctx context.Context, form url.Values, start, end time.Time, label string) (float64, bool, error) {
	var lastErr error
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			if errors.Is(lastErr, errRateLimited) {
				backoff = rateLimitCooldown // give the WAF block time to lift
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return 0, false, ctx.Err()
			}
		}
		values, found, err := f.fetchOnce(ctx, form)
		if err == nil {
			if f.batchDays > 0 {
				f.storeRange(values, found, start, end)
			}
			v, ok := values[start.Format("2006-01-02")]
			if !ok {
				return 0, false, nil
			}
			return v, true, nil
		}
		lastErr = err
		fmt.Fprintf(os.Stderr, "  [%s] attempt %d/%d failed: %v\n", label, attempt+1, f.maxRetries+1, err)
	}
	return 0, false, fmt.Errorf("fetch %s after %d attempts: %w", label, f.maxRetries+1, lastErr)
}

// storeRange records fetched values and confirmed no-data dates for a window.
func (f *taifexFetcher) storeRange(values map[string]float64, found bool, start, end time.Time) {
	if f.cache == nil {
		f.cache = make(map[string]float64)
		f.noData = make(map[string]bool)
	}
	f.cacheLo, f.cacheHi = start, end
	if !found {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			f.noData[d.Format("2006-01-02")] = true
		}
		return
	}
	for date, v := range values {
		f.cache[date] = v
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		if _, ok := values[ds]; !ok {
			f.noData[ds] = true
		}
	}
}

func (f *taifexFetcher) fetchOnce(ctx context.Context, form url.Values) (map[string]float64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || bytes.Contains(body, []byte("Just a moment")) {
		return nil, false, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d: %.160s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	text, err := decodeBody(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, false, err
	}
	if strings.Contains(text, "查無資料") {
		return nil, false, nil
	}
	return parseForeignOINetCSVMulti(text)
}

// parseForeignOINetCSVMulti parses the TAIFEX per-contract CSV text and returns
// a map of date → 臺股期貨 外資及陸資 多空未平倉口數淨額 for every date present in
// the CSV (one date for a single-day query, many dates for a range query).
// found=false means the CSV has no data rows at all (empty session /
// not a trading day).
func parseForeignOINetCSVMulti(text string) (values map[string]float64, found bool, err error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, false, nil
	}
	if !strings.HasPrefix(trimmed, "日期") {
		return nil, false, fmt.Errorf("unexpected TAIFEX response (not CSV): %.100s", trimmed)
	}

	r := csv.NewReader(strings.NewReader(trimmed))
	records, err := r.ReadAll()
	if err != nil {
		return nil, false, fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, false, nil // header only → no data rows
	}

	header := records[0]
	oiNetIdx, dateIdx := -1, -1
	for i, h := range header {
		switch strings.TrimSpace(h) {
		case csvHeaderOINet:
			oiNetIdx = i
		case "日期":
			dateIdx = i
		}
	}
	if oiNetIdx < 0 {
		return nil, false, fmt.Errorf("CSV header missing %q (got %v)", csvHeaderOINet, header)
	}
	if dateIdx < 0 {
		return nil, false, fmt.Errorf("CSV header missing 日期 (got %v)", header)
	}

	values = make(map[string]float64)
	for _, row := range records[1:] {
		if len(row) <= oiNetIdx || len(row) <= dateIdx {
			continue
		}
		if strings.TrimSpace(row[1]) != contractNameTX {
			continue
		}
		if strings.TrimSpace(row[2]) != traderForeign {
			continue
		}
		raw := strings.ReplaceAll(strings.TrimSpace(row[oiNetIdx]), ",", "")
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse OI net %q: %w", row[oiNetIdx], err)
		}
		date := strings.ReplaceAll(strings.TrimSpace(row[dateIdx]), "/", "-")
		values[date] = float64(v)
	}
	return values, len(values) > 0, nil
}

// parseForeignOINetCSV parses a single-day CSV and returns the value.
// found=false means the date has no TAIFEX data (holiday / not a trading day).
func parseForeignOINetCSV(text string) (value float64, found bool, err error) {
	values, found, err := parseForeignOINetCSVMulti(text)
	if err != nil || !found {
		return 0, found, err
	}
	for _, v := range values {
		return v, true, nil
	}
	return 0, false, nil
}

// decodeBody transcodes a response body according to its Content-Type charset.
// The TAIFEX CSV is Big5 (charset=MS950); the 查無資料 page is UTF-8.
func decodeBody(body []byte, contentType string) (string, error) {
	charset := strings.ToLower(charsetFromContentType(contentType))
	switch charset {
	case "", "utf-8", "utf8", "ascii", "us-ascii":
		return string(body), nil
	case "ms950", "big5", "big-5", "cp950", "windows-950":
		out, _, err := transform.Bytes(traditionalchinese.Big5.NewDecoder(), body)
		if err != nil {
			return "", fmt.Errorf("big5 decode: %w", err)
		}
		return string(out), nil
	default:
		enc, err := htmlindex.Get(charset)
		if err != nil {
			return "", fmt.Errorf("unsupported charset %q: %w", charset, err)
		}
		out, _, err := transform.Bytes(enc.NewDecoder(), body)
		if err != nil {
			return "", fmt.Errorf("transcode %s: %w", charset, err)
		}
		return string(out), nil
	}
}

func charsetFromContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return params["charset"]
}

// mergeSnapshot inserts foreign_futures_oi_net into the snapshot file for date.
// It refuses to overwrite an existing non-zero value unless force is set.
// Returns (merged, overwrote, err); merged=false with nil error means the
// snapshot already had a value and force was not set.
func mergeSnapshot(dir, date string, value float64, force bool) (bool, bool, error) {
	path := filepath.Join(dir, date+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, false, fmt.Errorf("read snapshot %s: %w", path, err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		return false, false, fmt.Errorf("parse snapshot %s: %w", path, err)
	}

	overwrote := false
	if existing, ok := snap[snapshotField]; ok && !force {
		var probe macroDataPoint
		if err := json.Unmarshal(existing, &probe); err == nil && (probe.Symbol != "" || probe.Value != 0 || probe.Timestamp != 0) {
			return false, false, nil // refuse-overwrite
		}
	}
	if existing, ok := snap[snapshotField]; ok && force {
		var probe macroDataPoint
		if err := json.Unmarshal(existing, &probe); err == nil && (probe.Symbol != "" || probe.Value != 0 || probe.Timestamp != 0) {
			overwrote = true
		}
	}

	ts, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false, false, fmt.Errorf("parse date %s: %w", date, err)
	}
	point := macroDataPoint{
		Symbol:    snapshotSymbol,
		Value:     value,
		ChangePct: 0,
		Timestamp: ts.Unix(),
	}
	pointBytes, err := json.Marshal(point)
	if err != nil {
		return false, false, fmt.Errorf("marshal point: %w", err)
	}

	merged, err := rewriteMergePreservingOrder(raw, snapshotField, pointBytes)
	if err != nil {
		return false, false, fmt.Errorf("merge: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, merged, 0o644); err != nil {
		return false, false, fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, false, fmt.Errorf("rename tmp→final: %w", err)
	}
	return true, overwrote, nil
}

// rewriteMergePreservingOrder inserts a new key:value pair at the end of the
// JSON object without reformatting the existing content (same approach as
// cmd/macrobackfill). The output preserves the original file's indentation
// and key order byte-for-byte, so `git diff` shows only the new key insertion.
func rewriteMergePreservingOrder(raw []byte, newKey string, newValue json.RawMessage) ([]byte, error) {
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

	prefix := end - 1
	for prefix > 0 && (raw[prefix-1] == ' ' || raw[prefix-1] == '\t' || raw[prefix-1] == '\n' || raw[prefix-1] == '\r') {
		prefix--
	}
	hasComma := prefix > 0 && raw[prefix-1] == ','

	keyBytes, err := json.Marshal(newKey)
	if err != nil {
		return nil, err
	}

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

func appendLogEntry(dir string, entry backfillLogEntry) {
	logPath := filepath.Join(dir, backfillLogName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "append backfill log: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()
	_ = json.NewEncoder(f).Encode(entry)
}

// printSummary prints run statistics and the verification count of snapshot
// files with a non-zero foreign_futures_oi_net value.
func printSummary(dir string, stats *runStats) {
	nonzero, total, err := countNonZeroSnapshots(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "count non-zero snapshots: %v\n", err)
	}
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("dates scanned:    %d\n", stats.dates)
	fmt.Printf("fetched (real):   %d\n", stats.fetched)
	fmt.Printf("no-data days:     %d\n", stats.noData)
	fmt.Printf("carried forward:  %d\n", stats.carriedForward)
	fmt.Printf("merged:           %d\n", stats.merged)
	fmt.Printf("overwritten:      %d\n", stats.overwritten)
	fmt.Printf("already present:  %d\n", stats.alreadyPresent)
	fmt.Printf("no snapshot file: %d\n", stats.noSnapshotFile)
	fmt.Printf("fetch/merge errors: %d\n", stats.fetchErrors)
	if len(stats.fetchErrorDates) > 0 {
		fmt.Printf("  error dates: %s\n", strings.Join(stats.fetchErrorDates, ", "))
	}
	fmt.Printf("\nverification: %d/%d snapshot files have non-zero %s\n", nonzero, total, snapshotField)
	if stats.fetchErrors > 0 {
		fmt.Printf("⚠  %d date(s) failed; rerun the command to retry them\n", stats.fetchErrors)
	}
}

func countNonZeroSnapshots(dir string) (nonzero, total int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "20") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if len(name) != 10+len(".json") || name[4] != '-' || name[7] != '-' {
			continue
		}
		total++
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var snap map[string]json.RawMessage
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}
		rawPt, ok := snap[snapshotField]
		if !ok {
			continue
		}
		var pt macroDataPoint
		if err := json.Unmarshal(rawPt, &pt); err != nil {
			continue
		}
		if pt.Value != 0 {
			nonzero++
		}
	}
	return nonzero, total, nil
}
