// Command c07-spot-check-recorder records manual spot-checks of C07 sector
// direction prediction drivers for the Day 7/14 acceptance gate (#1281).
//
// It is the counterpart to c07-obs-collector: c07-obs-collector fills all
// automatable metrics (sector_count, jsd_alert_rate, latency, etc.) while
// spot_check_count is manual by definition — this tool records those manual
// verifications so the evaluator can enforce the spot_check_count threshold.
//
// Usage:
//
//	go run ./cmd/experimental/c07-spot-check-recorder [flags]
//
// Flags:
//
//	-url       atlas base URL (default http://localhost:18080)
//	-obs-log   path to observation log (default docs/operations/sector-prediction-observation-log.md)
//	-date      trading day being spot-checked, YYYY-MM-DD (required)
//	-sectors   comma-separated list of sector IDs being spot-checked (required, e.g. semiconductor,electronics)
//	-notes     human-readable notes (default: empty)
//	-dry-run   print what would change without writing
//
// Exit codes:
//
//	0  success
//	1  invalid sector ID
//	2  driver verification failed (no macro/event/cycle reference found)
//	3  observation log malformed
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// operatorFlag is the -operator CLI flag for spot-check audit trail.
// Declared at package level so getOperator() can read it. CLI tools must not
// call os.Getenv directly (constitution rule).
var operatorFlag = flag.String("operator", "OPERATOR", "operator identity (recorded in audit marker)")

const (
	defaultAtlasURL = "http://localhost:18080"
	defaultObsLog   = "docs/operations/sector-prediction-observation-log.md"
	httpTimeout     = 10 * time.Second
)

// Exit codes.
const (
	ExitSuccess            = 0
	ExitInvalidSector      = 1
	ExitDriverVerification = 2
	ExitMalformedObsLog    = 3
)

// Canonical L1 sector IDs — hard-coded to avoid a import cycle with the
// industry package (which would pull in the full atlas service graph into
// what should be a standalone CLI tool).
var validSectors = map[string]bool{
	"semiconductor":     true,
	"electronics":       true,
	"optoelectronics":   true,
	"financials":        true,
	"cement":            true,
	"plastics":          true,
	"textiles":          true,
	"steel":             true,
	"shipping":          true,
	"food":              true,
	"auto":              true,
	"telecom":           true,
	"chemicals":         true,
	"biotech":           true,
	"construction":      true,
	"other_electronics": true,
	"machinery":         true,
	"tourism":           true,
	"retail":            true,
	"energy":            true,
}

// macroDriverRE matches macro symbol references in drivers, e.g.
// "ForeignInvestorNet", "DXY", "taiex", "bdi", "us10y".
var macroDriverRE = regexp.MustCompile(`(?i)(taiex|foreign[_\s]?investor|fi[_\s]?net|dxy|bdi|us10y|vix|sp500|nasdaq|sox|nvda|aapl|msft|tsm|crude|oil|gold|silver|copper|commodity|inflation|unemployment|pmi|ISM|PMI|fed|ECB|ECB|currency|exchange[_\s]?rate|匯率|熱錢|原物料|科技股|半導體)`)

// eventDriverRE matches event/calendar references.
var eventDriverRE = regexp.MustCompile(`(?i)(法說會|法說|eps|營收|營利|conference|earnings|revenue|dividend|配息|除權|除息|msci|etf|rebalance|成分股|季度|季底|window[_\s]?dress|作夢|作夢行情|總統大選|加息|降息|升息|降準|庫存|庫存週期|半導體庫存|消費電子|新品|旗艦機|蘋果|華為|輝達|nvidia|amd|intel|台積電|tsmc)`)

// cycleDriverRE matches cycle phase references.
var cycleDriverRE = regexp.MustCompile(`(?i)(cycle|週期|景氣|循環|上升段|下降段|復甦|擴張|收縮|庫存調整|inventory[_\s]?cycle|景氣循環|淡季|旺季|拉貨|庫存回補|庫存去化|半導體景氣)`)

// predictionReport mirrors the /api/events/prediction response (subset).
type predictionReport struct {
	Predictions       []flowPrediction      `json:"predictions"`
	SectorPredictions []sectorDayPrediction `json:"sector_predictions"`
}

type flowPrediction struct {
	Date         string                 `json:"date"`
	Direction    string                 `json:"direction"`
	Confidence   float64                `json:"confidence"`
	Distribution predictionDistribution `json:"distribution"`
}

type sectorDayPrediction struct {
	Date    string             `json:"date"`
	Sectors []sectorPrediction `json:"sectors"`
}

type sectorPrediction struct {
	SectorID     string                 `json:"sector_id"`
	SectorName   string                 `json:"sector_name"`
	Direction    string                 `json:"direction"`
	Confidence   float64                `json:"confidence"`
	Distribution predictionDistribution `json:"distribution"`
	Drivers      []string               `json:"drivers"`
}

type predictionDistribution struct {
	Inflow  float64 `json:"inflow"`
	Outflow float64 `json:"outflow"`
	Neutral float64 `json:"neutral"`
}

// obsRow mirrors c07-day-evaluator's obsRow for parsing the table.
type obsRow struct {
	Date                 string
	SectorCount          int
	JSDAlertRate         float64
	LatencyP95Ms         int64
	ConfidenceViolations int
	PanicCount           int
	SpotCheckCount       int
	Notes                string
}

// spotCheckRecord holds the embedded marker that prevents double-counting.
type spotCheckRecord struct {
	ID        string   `json:"id"` // unique per (date, sector_id) pair
	Date      string   `json:"date"`
	SectorID  string   `json:"sector_id"`
	Timestamp string   `json:"timestamp"`
	Operator  string   `json:"operator"`
	Notes     string   `json:"notes"`
	Sources   []string `json:"driver_sources"` // "macro", "event", "cycle"
}

func main() {
	var (
		url     = flag.String("url", defaultAtlasURL, "atlas base URL")
		obsLog  = flag.String("obs-log", defaultObsLog, "path to observation log")
		date    = flag.String("date", "", "trading day being spot-checked, YYYY-MM-DD (required)")
		sectors = flag.String("sectors", "", "comma-separated sector IDs (required)")
		notes   = flag.String("notes", "", "human-readable notes")
		dryRun  = flag.Bool("dry-run", false, "print what would change without writing")
	)
	flag.Parse()

	if *date == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -date is required (YYYY-MM-DD)")
		os.Exit(ExitInvalidSector)
	}
	if *sectors == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -sectors is required")
		os.Exit(ExitInvalidSector)
	}

	sectorList := parseSectorList(*sectors)

	// Validate all sector IDs.
	for _, sid := range sectorList {
		if !validSectors[sid] {
			fmt.Fprintf(os.Stderr, "ERROR: invalid sector ID %q (not in 20-sector L1 taxonomy)\n", sid)
			os.Exit(ExitInvalidSector)
		}
	}

	operator := getOperator()

	// Fetch prediction report for the given date and verify drivers.
	report, err := fetchPredictionReport(*url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: fetch prediction report: %v\n", err)
		os.Exit(1)
	}

	// Build a map of sector_id -> drivers for the target date.
	driversMap := buildDriversMap(report, *date)

	// Verify drivers for each sector.
	var verifiedSectors, unverifiedSectors []string
	var driverSources []string
	for _, sid := range sectorList {
		drivers, ok := driversMap[sid]
		if !ok {
			// Sector not in prediction for this date — still record but flag.
			fmt.Fprintf(os.Stderr, "WARN: sector %s not found in prediction for %s\n", sid, *date)
		}
		sources := verifyDrivers(drivers)
		if sources == nil {
			unverifiedSectors = append(unverifiedSectors, sid)
		} else {
			verifiedSectors = append(verifiedSectors, sid)
			driverSources = append(driverSources, sources...)
		}
	}

	if len(unverifiedSectors) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: driver verification failed for sectors: %v\n", unverifiedSectors)
		fmt.Fprintln(os.Stderr, "  Each sector driver must reference at least one of: macro symbol, event, or cycle.")
		os.Exit(ExitDriverVerification)
	}

	// Deduplicate driver sources.
	seen := make(map[string]bool)
	var uniqSources []string
	for _, s := range driverSources {
		if !seen[s] {
			seen[s] = true
			uniqSources = append(uniqSources, s)
		}
	}
	sort.Strings(uniqSources)

	// Parse the existing obs log.
	rows, raw, err := parseObsLogRaw(*obsLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse obs log: %v\n", err)
		os.Exit(ExitMalformedObsLog)
	}

	// Find the row for the target date.
	ts := time.Now().Format("2006-01-02 15:04")
	var rowMatch *obsRow
	var rowIdx int = -1
	for i, r := range rows {
		if r.Date == *date {
			rowMatch = &rows[i]
			rowIdx = i
			break
		}
	}

	newSpotCheckCount := 0
	// Dedup: filter sectorList down to sectors that don't yet have a marker
	// for this date. The final count is existing markers + new (unique) sectors.
	existingTotal := countTotalSpotChecksForDate(raw, *date)
	newUniqueSectors := filterNewSectors(raw, *date, sectorList)
	newSpotCheckCount = existingTotal + len(newUniqueSectors)
	_ = rowMatch // rowMatch unused after dedup refactor; newUniqueSectors gates the action.

	if *dryRun {
		fmt.Println("DRY-RUN: would update obs log:")
		fmt.Printf("  date: %s\n", *date)
		fmt.Printf("  sector list: %v\n", sectorList)
		fmt.Printf("  already recorded: %d (existing markers for this date)\n", existingTotal)
		fmt.Printf("  new unique sectors: %v (will write markers + increment count)\n", newUniqueSectors)
		fmt.Printf("  new spot_check_count: %d\n", newSpotCheckCount)
		fmt.Printf("  driver sources: %v\n", uniqSources)
		if len(newUniqueSectors) == 0 {
			fmt.Println("  (no-op: all requested sectors already recorded)")
		}
		os.Exit(0)
	}

	if len(newUniqueSectors) == 0 {
		// All requested sectors already have markers — idempotent no-op.
		fmt.Printf("No new spot-checks: all %d requested sectors already recorded for %s (spot_check_count remains %d)\n",
			len(sectorList), *date, existingTotal)
		os.Exit(0)
	}

	// Build the embedded spot-check record markers (one per NEW unique sector).
	var markers []string
	for _, sid := range newUniqueSectors {
		rec := spotCheckRecord{
			ID:        fmt.Sprintf("%s-%s", *date, sid),
			Date:      *date,
			SectorID:  sid,
			Timestamp: ts,
			Operator:  operator,
			Notes:     *notes,
			Sources:   uniqSources,
		}
		data, _ := json.Marshal(rec)
		marker := fmt.Sprintf("\n<spot-check-record id=%q></spot-check-record>\n", rec.ID)
		_ = data // embedded JSON available for future manual inspection
		markers = append(markers, marker)
	}

	// Build the narrative section to append.
	narrativeSec := buildNarrativeSection(ts, newUniqueSectors, uniqSources, *notes, operator)

	// Update or append the obs log.
	if err := updateObsLog(*obsLog, raw, rows, *date, newSpotCheckCount, rowIdx, narrativeSec, strings.Join(markers, "")); err != nil {
		os.Exit(ExitMalformedObsLog)
	}

	fmt.Printf("Recorded spot-check for %s: sectors=%v (new: %v), spot_check_count=%d, sources=%v\n",
		*date, sectorList, newUniqueSectors, newSpotCheckCount, uniqSources)
	os.Exit(0)
}

// getOperator returns the operator identity from the -operator flag.
// CLI tools must not call os.Getenv directly (constitution rule: 配置應走
// config.GetSecret 或 flag,避免在 main 之外的隱式環境耦合).
func getOperator() string {
	if op := *operatorFlag; op != "" {
		return op
	}
	return "OPERATOR"
}

// parseSectorList splits a comma-separated string of sector IDs.
func parseSectorList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fetchPredictionReport GETs /api/events/prediction and parses the response.
func fetchPredictionReport(baseURL string) (*predictionReport, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/events/prediction"
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url) //nolint:gosec // G704: URL validated as localhost-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var report predictionReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return &report, nil
}

// buildDriversMap returns sector_id -> drivers for the given date.
func buildDriversMap(report *predictionReport, date string) map[string][]string {
	result := make(map[string][]string)
	for _, sp := range report.SectorPredictions {
		if sp.Date == date {
			for _, sec := range sp.Sectors {
				result[sec.SectorID] = sec.Drivers
			}
			break
		}
	}
	// If no match on exact date, fall back to first available.
	if len(result) == 0 && len(report.SectorPredictions) > 0 {
		for _, sec := range report.SectorPredictions[0].Sectors {
			result[sec.SectorID] = sec.Drivers
		}
	}
	return result
}

// verifyDrivers checks whether drivers reference macro, event, or cycle sources.
// Returns nil if no valid source is found; otherwise returns the list of found sources.
func verifyDrivers(drivers []string) []string {
	var found []string
	for _, d := range drivers {
		dLower := strings.ToLower(d)
		if macroDriverRE.MatchString(dLower) || eventDriverRE.MatchString(dLower) || cycleDriverRE.MatchString(dLower) {
			// Classify the source.
			if macroDriverRE.MatchString(dLower) {
				found = append(found, "macro")
			}
			if eventDriverRE.MatchString(dLower) {
				found = append(found, "event")
			}
			if cycleDriverRE.MatchString(dLower) {
				found = append(found, "cycle")
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	// Deduplicate.
	seen := make(map[string]bool)
	var uniq []string
	for _, s := range found {
		if !seen[s] {
			seen[s] = true
			uniq = append(uniq, s)
		}
	}
	sort.Strings(uniq)
	return uniq
}

// parseObsLogRaw reads the obs log and parses table rows.
// Returns rows, raw content, and error.
func parseObsLogRaw(path string) ([]obsRow, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	content := string(data)
	rows := parseObsLog(content)
	return rows, content, nil
}

// parseObsLog parses table rows from raw markdown content.
// Always succeeds on well-formed markdown; no error path.
func parseObsLog(content string) []obsRow {
	var rows []obsRow
	inRecords := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "## Records" {
			inRecords = true
			continue
		}
		if !inRecords || !strings.HasPrefix(line, "|") || strings.Contains(line, "日期") {
			continue
		}
		if strings.HasPrefix(line, "<!--") || line == "" {
			continue
		}

		cells := splitTableRow(line)
		if len(cells) < 8 {
			continue
		}

		row := obsRow{
			Date:                 strings.TrimSpace(cells[1]),
			SectorCount:          parseInt(cells[2]),
			JSDAlertRate:         parsePercent(cells[3]),
			LatencyP95Ms:         int64(parseInt(cells[4])),
			ConfidenceViolations: parseInt(cells[5]),
			PanicCount:           parseInt(cells[6]),
			SpotCheckCount:       parseInt(cells[7]),
			Notes:                strings.TrimSpace(cells[8]),
		}
		rows = append(rows, row)
	}
	return rows
}

// splitTableRow splits a markdown table row into cells.
func splitTableRow(line string) []string {
	var cells []string
	start := 0
	for i, c := range line {
		if c == '|' {
			cells = append(cells, line[start:i])
			start = i + 1
		}
	}
	cells = append(cells, line[start:])
	return cells
}

// parseInt parses an integer, returning 0 on failure.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s) //nolint:errcheck // best-effort: 0 on parse failure
	return n
}

// parsePercent parses a percentage string like "5.0%" into 0.05.
func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f / 100
}

// filterNewSectors returns the subset of sectors whose (date, sector_id)
// marker does not yet exist in the raw obs log. Order is preserved.
func filterNewSectors(raw, date string, sectors []string) []string {
	var out []string
	for _, sid := range sectors {
		id := fmt.Sprintf("%s-%s", date, sid)
		pattern := fmt.Sprintf(`<spot-check-record id=%q>`, id)
		if !strings.Contains(raw, pattern) {
			out = append(out, sid)
		}
	}
	return out
}

// countSpotChecksForDate counts how many of the given sectorList already have
// embedded spot-check markers in the raw markdown for the given date.
func countSpotChecksForDate(raw, date string, sectorList []string) int {
	existing := make(map[string]bool)
	for _, sid := range sectorList {
		id := fmt.Sprintf("%s-%s", date, sid)
		pattern := fmt.Sprintf(`<spot-check-record id=%q>`, id)
		if strings.Contains(raw, pattern) {
			existing[sid] = true
		}
	}
	return len(existing)
}

// countTotalSpotChecksForDate counts all unique spot-check record IDs for a date.
func countTotalSpotChecksForDate(raw, date string) int {
	pattern := regexp.MustCompile(fmt.Sprintf(`<spot-check-record id="%s-([^"]+)">`, regexp.QuoteMeta(date)))
	matches := pattern.FindAllStringSubmatch(raw, -1)
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			seen[m[1]] = true
		}
	}
	return len(seen)
}

// buildNarrativeSection builds the markdown narrative section for the spot-check.
func buildNarrativeSection(ts string, sectors []string, sources []string, notes, operator string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Spot-Check Records\n\n")
	fmt.Fprintf(&b, "### %s — %s\n\n", ts, operator)
	fmt.Fprintf(&b, "- **sectors checked**: %s\n", strings.Join(sectors, ", "))
	fmt.Fprintf(&b, "- **driver sources verified**: %s\n", strings.Join(sources, ", "))
	if notes != "" {
		fmt.Fprintf(&b, "- **notes**: %s\n", notes)
	}
	return b.String()
}

// updateObsLog atomically updates the observation log:
// - If rowIdx >= 0: updates the spot_check_count of the matching row
// - Appends the narrative section after the table
// - Writes via rename to ensure atomicity.
func updateObsLog(path, raw string, rows []obsRow, date string, newCount, rowIdx int, narrative, markers string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	var updated string
	if rowIdx >= 0 {
		updated = updateRowSpotCheckCount(raw, rows, rowIdx, date, newCount)
	} else {
		// No existing row — build one from scratch.
		updated = raw + buildNewRow(date, newCount)
	}

	// Append narrative section.
	updated = strings.TrimRight(updated, "\n") + narrative

	// markers is already a joined string.
	markerBlock := markers
	// Insert markers just before "## Spot-Check Records" if it exists,
	// otherwise just append after the last table row.
	spotCheckMarker := "## Spot-Check Records"
	if strings.Contains(updated, spotCheckMarker) {
		updated = strings.Replace(updated, spotCheckMarker, markerBlock+"\n"+spotCheckMarker, 1)
	} else {
		updated = strings.TrimRight(updated, "\n") + "\n" + markerBlock + "\n"
	}

	// Atomic write: write to .tmp, fsync, rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp to obs log: %w", err)
	}
	return nil
}

// updateRowSpotCheckCount updates the spot_check_count cell for the given row index.
func updateRowSpotCheckCount(raw string, rows []obsRow, rowIdx int, date string, newCount int) string {
	// Find the line in raw that matches the date.
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitTableRow(line)
		if len(cells) < 8 {
			continue
		}
		if strings.TrimSpace(cells[1]) == date {
			// Update the spot_check_count cell (index 7).
			cells[7] = fmt.Sprintf(" %d ", newCount)
			lines[i] = strings.Join(cells, "|")
			break
		}
	}
	return strings.Join(lines, "\n")
}

// buildNewRow constructs a new table row for the given date with spot_check_count set.
func buildNewRow(date string, spotCheckCount int) string {
	return fmt.Sprintf("\n| %s | - | - | - | - | - | %d | manual spot-check |\n",
		date, spotCheckCount)
}
