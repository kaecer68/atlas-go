// Package main implements cmd/atlas-stage4-backfill — Stage 4 PR#1.
//
// Behavior:
//   - Reads sessions, macro snapshots, and outcome records for the
//     requested lookback window (default 90 days).
//   - Emits 4 staging JSONL files under --out (default ./data/staging):
//     regime_history_90d.jsonl
//     event_calendar_90d.jsonl
//     stress_index_history_90d.jsonl
//     prediction_actual_90d.jsonl
//   - Each line carries captured_at + is_synthetic=1 stamps.
//
// Idempotency:
//   - Re-running overwrites every output file (truncate-then-write).
//   - Safe to re-run while a running atlas process is reading old data;
//     the writer holds the file lock only for the duration of the run.
//
// CLI flags:
//
//	--source <path>        data state root (default ./data/state)
//	--out <path>           staging output dir (default ./data/staging)
//	--lookback-days <int>  default 90
//	--dry-run              print plan, do not write files
//	-h / --help            usage message
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ------------------------------------------------------------------
// Inputs and Run() pipeline.
// ------------------------------------------------------------------

// RunOptions collects CLI inputs that affect extraction behavior.
type RunOptions struct {
	Source       string // data/state root
	StagingDir   string // output dir for 4 JSONLs
	LookbackDays int
	Now          time.Time // injected for tests; CLI uses time.Now().UTC()
	DryRun       bool
	Logger       *log.Logger
}

func (o RunOptions) lookbackStart() time.Time {
	end := time.Date(o.Now.Year(), o.Now.Month(), o.Now.Day(), 0, 0, 0, 0, time.UTC)
	return end.AddDate(0, 0, -o.LookbackDays+1)
}

// RunStats reports what the extractor produced. Surfaced in CLI output and
// returned from tests for golden-file assertions.
type RunStats struct {
	SessionsScanned             int
	SessionsInWindow            int
	MacroFilesScanned           int
	MacroFilesInWindow          int
	RegimeRowsWritten           int
	EventCalendarRowsWritten    int
	StressIndexRowsWritten      int
	PredictionActualRowsWritten int
	SessionsMissingSummary      int
	SessionsMissingOutcomes     int
	MacroFilesMissing           int
}

// Run executes the backfill end-to-end and returns the stats. Tests call
// Run directly with a synthesized RunOptions; main() wraps Run and prints.
//
// The 4 extractors share a single pass over sessions/macro dir to avoid
// double-walking; extractors are independent except for the session walk.
func Run(opts RunOptions) (RunStats, error) {
	stats := RunStats{}
	if opts.Logger == nil {
		opts.Logger = log.New(io.Discard, "", 0)
	}
	if opts.LookbackDays <= 0 {
		return stats, fmt.Errorf("lookback days must be > 0, got %d", opts.LookbackDays)
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}

	staging, err := ResolveStagingFiles(opts.StagingDir)
	if err != nil {
		return stats, err
	}
	if !opts.DryRun {
		if err := os.MkdirAll(opts.StagingDir, 0o755); err != nil {
			return stats, fmt.Errorf("mkdir staging dir: %w", err)
		}
	}

	// Pass 1: walk sessions dir.
	sessionsDir := filepath.Join(opts.Source, "sessions")
	sessionAggs, sessionStats, err := walkSessions(opts, sessionsDir)
	if err != nil {
		return stats, err
	}
	stats.SessionsScanned = sessionStats.Scanned
	stats.SessionsInWindow = sessionStats.InWindow
	stats.SessionsMissingSummary = sessionStats.MissingSummary
	stats.SessionsMissingOutcomes = sessionStats.MissingOutcomes

	// Pass 2: walk macro dir.
	macroDir := filepath.Join(opts.Source, "macro")
	macroAggs, macroStats, err := walkMacro(opts, macroDir)
	if err != nil {
		return stats, err
	}
	stats.MacroFilesScanned = macroStats.Scanned
	stats.MacroFilesInWindow = macroStats.InWindow
	stats.MacroFilesMissing = macroStats.Missing

	if opts.DryRun {
		return stats, nil
	}

	// Pass 3: write 4 JSONLs.
	nowUTC := opts.Now.UTC()
	if err := writeRegimeHistory(staging.RegimeHistory, nowUTC, sessionAggs, &stats); err != nil {
		return stats, fmt.Errorf("regime: %w", err)
	}
	if err := writeEventCalendarHistory(staging.EventCalendarHistory, nowUTC, sessionAggs, &stats); err != nil {
		return stats, fmt.Errorf("event calendar: %w", err)
	}
	if err := writeStressIndexHistory(staging.StressIndexHistory, nowUTC, macroAggs, &stats); err != nil {
		return stats, fmt.Errorf("stress index: %w", err)
	}
	if err := writePredictionActual(staging.PredictionActual, nowUTC, sessionAggs, &stats); err != nil {
		return stats, fmt.Errorf("prediction actual: %w", err)
	}
	return stats, nil
}

// ------------------------------------------------------------------
// Pass 1: walk sessions.
// ------------------------------------------------------------------

type walkSessionsStats struct {
	Scanned         int
	InWindow        int
	MissingSummary  int
	MissingOutcomes int
}

// sessionSummaryLite is the subset of summary.json we care about.
// RecordedAt is parsed best-effort; missing → zero time.
type sessionSummaryLite struct {
	SessionID  string
	Regime     string
	RecordedAt time.Time
}

// outcomeLite is the subset of RecommendationOutcome we care about.
type outcomeLite struct {
	ForwardReturn  float64
	HasForwardRet  bool
	Hit            bool
	Conviction     int
	Side           string
	SupportingEvts []string
	Regime         string
}

// sessionFile wraps a single session's parsed summary + outcomes.
type sessionFile struct {
	Path     string
	Summary  *sessionSummaryLite
	Outcomes []outcomeLite
}

var sessionIDPattern = regexp.MustCompile(`^session-(\d{8})-daily$`)

func walkSessions(opts RunOptions, dir string) (map[string]*sessionFile, walkSessionsStats, error) {
	out := map[string]*sessionFile{}
	stats := walkSessionsStats{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, stats, nil
		}
		return nil, stats, fmt.Errorf("read sessions dir %s: %w", dir, err)
	}
	start := opts.lookbackStart()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		stats.Scanned++

		date, ok := parseSessionDate(name)
		if !ok {
			opts.Logger.Printf("skipping %s: bad session id", name)
			continue
		}
		dateTime, err := time.Parse("2006-01-02", date)
		if err != nil {
			opts.Logger.Printf("skipping %s: bad date %q", name, date)
			continue
		}
		if dateTime.Before(start) {
			continue
		}
		stats.InWindow++

		sf := &sessionFile{Path: filepath.Join(dir, name)}
		summaryPath := filepath.Join(sf.Path, "summary.json")
		outcomesPath := filepath.Join(sf.Path, "recommendation_outcomes.jsonl")

		if sum, err := parseSessionSummary(summaryPath); err == nil {
			sf.Summary = sum
		} else if !os.IsNotExist(err) {
			opts.Logger.Printf("parse summary %s: %v", summaryPath, err)
		} else {
			stats.MissingSummary++
		}
		if outs, err := parseOutcomeJSONL(outcomesPath); err == nil {
			sf.Outcomes = outs
		} else if !os.IsNotExist(err) {
			opts.Logger.Printf("parse outcomes %s: %v", outcomesPath, err)
		} else {
			stats.MissingOutcomes++
		}
		out[date] = sf
	}
	return out, stats, nil
}

func parseSessionDate(sessionID string) (string, bool) {
	m := sessionIDPattern.FindStringSubmatch(sessionID)
	if len(m) != 2 {
		return "", false
	}
	// Best-effort: 20260415 → 2026-04-15; do not error on fake dates because
	// only the lexicographic sort matters for our window filter.
	if _, err := time.Parse("20060102", m[1]); err != nil {
		return "", false
	}
	return m[1][:4] + "-" + m[1][4:6] + "-" + m[1][6:8], true
}

func parseSessionSummary(path string) (*sessionSummaryLite, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var s struct {
		SessionID  string `json:"session_id"`
		Regime     string `json:"regime"`
		RecordedAt string `json:"recorded_at"`
	}
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out := &sessionSummaryLite{SessionID: s.SessionID, Regime: s.Regime}
	if s.RecordedAt != "" {
		// recorded_at formats in atlas-go are RFC3339-style; tolerate trailing Z or +08:00.
		if t, err := time.Parse(time.RFC3339, s.RecordedAt); err == nil {
			out.RecordedAt = t.UTC()
		} else if t, err := time.Parse("2006-01-02T15:04:05Z", s.RecordedAt); err == nil {
			out.RecordedAt = t.UTC()
		}
	}
	return out, nil
}

func parseOutcomeJSONL(path string) ([]outcomeLite, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := []outcomeLite{}
	scanner := bufio.NewScanner(f)
	// Some lines can be long (factor breakdown JSON grows huge); raise buffer.
	scanner.Buffer(make([]byte, 0, 1<<16), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var row struct {
			ForwardReturn  *float64 `json:"forward_return"`
			Hit            *bool    `json:"hit"`
			Conviction     *int     `json:"conviction"`
			Side           string   `json:"side"`
			SupportingEvts []string `json:"supporting_events"`
			Regime         string   `json:"regime"`
			Window         string   `json:"window"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			continue // tolerate malformed lines (e.g. trailing CLI output)
		}
		o := outcomeLite{Side: strings.ToUpper(row.Side), Regime: row.Regime}
		if row.ForwardReturn != nil {
			o.ForwardReturn = *row.ForwardReturn
			o.HasForwardRet = true
		}
		if row.Hit != nil {
			o.Hit = *row.Hit
		}
		if row.Conviction != nil {
			o.Conviction = *row.Conviction
		}
		if len(row.SupportingEvts) > 0 {
			o.SupportingEvts = row.SupportingEvts
		}
		out = append(out, o)
	}
	return out, scanner.Err()
}

// ------------------------------------------------------------------
// Pass 2: walk macro snapshots.
// ------------------------------------------------------------------

type walkMacroStats struct {
	Scanned  int
	InWindow int
	Missing  int
}

// macroFileLite is the subset of macro/YYYY-MM-DD.json we use.
type macroFileLite struct {
	Date   string
	HasSI  bool
	Score  float64
	Regime string
	Comps  map[string]any
}

func walkMacro(opts RunOptions, dir string) (map[string]*macroFileLite, walkMacroStats, error) {
	out := map[string]*macroFileLite{}
	stats := walkMacroStats{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, stats, nil
		}
		return nil, stats, fmt.Errorf("read macro dir %s: %w", dir, err)
	}
	start := opts.lookbackStart()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		stats.Scanned++
		date := strings.TrimSuffix(name, ".json")
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		if t.Before(start) {
			continue
		}
		stats.InWindow++
		mf, err := parseMacroFile(filepath.Join(dir, name))
		if err != nil {
			stats.Missing++
			opts.Logger.Printf("parse macro %s: %v", name, err)
			continue
		}
		out[date] = mf
	}
	return out, stats, nil
}

func parseMacroFile(path string) (*macroFileLite, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	out := &macroFileLite{}
	// The macro file shape differs per period (newer files have stress_index,
	// older files have only VIXScore/DXYScore). We treat the file as
	// generic JSON and look for "stress_index" or "stress" subblock.
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if block, ok := root["stress_index"].(map[string]any); ok {
		out.HasSI = true
		out.Comps = block
		if s, ok := block["score"].(float64); ok {
			out.Score = s
		}
		if s, ok := block["regime"].(string); ok {
			out.Regime = s
		}
	}
	return out, nil
}

// ------------------------------------------------------------------
// Pass 3a: regime_history_90d.jsonl writer.
// ------------------------------------------------------------------

func writeRegimeHistory(path string, now time.Time, sessions map[string]*sessionFile, stats *RunStats) error {
	w, err := openJSONL(path)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	keys := sortedKeys(sessions)
	for _, date := range keys {
		sf := sessions[date]
		if sf.Summary == nil {
			continue
		}
		rec := &RegimeRecord{
			Date:            date,
			Regime:          sf.Summary.Regime,
			SourceSessionID: sf.Summary.SessionID,
		}
		if !sf.Summary.RecordedAt.IsZero() {
			rec.RecordedAt = sf.Summary.RecordedAt
		}
		stampDefaults(rec, now)
		if err := EncodeLine(w, rec); err != nil {
			return err
		}
		stats.RegimeRowsWritten++
	}
	return nil
}

// ------------------------------------------------------------------
// Pass 3b: event_calendar_90d.jsonl writer (derived from supporting_events).
// ------------------------------------------------------------------

// themeRegex pulls a stable "theme" from an event id like
// "evt-tech-peak-1783940201148250418" → "tech-peak".
var themeRegex = regexp.MustCompile(`^evt-([a-z0-9_-]+?)(-\d+)?$`)

func writeEventCalendarHistory(path string, now time.Time, sessions map[string]*sessionFile, stats *RunStats) error {
	w, err := openJSONL(path)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	keys := sortedKeys(sessions)
	for _, date := range keys {
		sf := sessions[date]
		if len(sf.Outcomes) == 0 {
			continue
		}
		seen := map[string]bool{}
		var ids []string
		var themes []string
		themeSeen := map[string]bool{}
		for _, o := range sf.Outcomes {
			for _, id := range o.SupportingEvts {
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				ids = append(ids, id)
				if m := themeRegex.FindStringSubmatch(id); len(m) >= 2 {
					theme := m[1]
					if !themeSeen[theme] {
						themeSeen[theme] = true
						themes = append(themes, theme)
					}
				}
			}
		}
		if len(ids) == 0 {
			continue
		}
		rec := &EventCalendarRecord{
			Date:         date,
			EventIDs:     ids,
			Source:       "session-derive",
			ActiveThemes: themes,
		}
		stampDefaults(rec, now)
		if err := EncodeLine(w, rec); err != nil {
			return err
		}
		stats.EventCalendarRowsWritten++
	}
	return nil
}

// ------------------------------------------------------------------
// Pass 3c: stress_index_history_90d.jsonl writer.
// ------------------------------------------------------------------

func writeStressIndexHistory(path string, now time.Time, macros map[string]*macroFileLite, stats *RunStats) error {
	w, err := openJSONL(path)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	keys := sortedKeys(macros)
	for _, date := range keys {
		mf := macros[date]
		rec := &StressIndexRecord{Date: date, Score: mf.Score, Components: mf.Comps}
		if mf.HasSI {
			rec.Regime = mf.Regime
			rec.Source = "macro-file"
		} else {
			rec.Regime = "raw"
			rec.Source = "raw"
		}
		stampDefaults(rec, now)
		if err := EncodeLine(w, rec); err != nil {
			return err
		}
		stats.StressIndexRowsWritten++
	}
	return nil
}

// ------------------------------------------------------------------
// Pass 3d: prediction_actual_90d.jsonl writer.
// ------------------------------------------------------------------

func writePredictionActual(path string, now time.Time, sessions map[string]*sessionFile, stats *RunStats) error {
	w, err := openJSONL(path)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	keys := sortedKeys(sessions)
	for _, date := range keys {
		sf := sessions[date]
		if len(sf.Outcomes) == 0 {
			continue
		}
		rec := aggregateOutcomeSummary(date, sf)
		stampDefaults(rec, now)
		if err := EncodeLine(w, rec); err != nil {
			return err
		}
		stats.PredictionActualRowsWritten++
	}
	return nil
}

func aggregateOutcomeSummary(date string, sf *sessionFile) *PredictionActualRecord {
	rec := &PredictionActualRecord{
		Date:            date,
		SourceSessionID: date, // session_id == date at this layer
	}
	regimeTally := map[string]int{}
	var rets []float64
	var flowNums []float64
	for _, o := range sf.Outcomes {
		rec.TotalOutcomes++
		if o.Hit {
			rec.HitOutcomesCount++
		}
		if o.HasForwardRet {
			rets = append(rets, o.ForwardReturn)
			// Capital flow change proxy: forward_return scaled by conviction.
			flowNums = append(flowNums, o.ForwardReturn*float64(o.Conviction)/100.0)
		}
		switch o.Side {
		case "BUY":
			if o.Hit {
				rec.BullishOutcomes++
			}
		case "SELL", "SHORT":
			if o.Hit {
				rec.BearishOutcomes++
			}
		}
		if o.Regime != "" {
			regimeTally[o.Regime]++
		}
	}
	if rec.TotalOutcomes > 0 {
		rec.WinRate = float64(rec.HitOutcomesCount) / float64(rec.TotalOutcomes)
	}
	if len(rets) > 0 {
		rec.MeanForwardReturn = meanFloat64(rets)
		rec.StdDevForwardReturn = stdDevFloat64(rets, rec.MeanForwardReturn)
	}
	if len(flowNums) > 0 {
		rec.CapitalFlowChangeProxy = meanFloat64(flowNums)
	}
	rec.PredominantRegime = pickPredominant(regimeTally)
	return rec
}

func meanFloat64(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}

func stdDevFloat64(xs []float64, mean float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		d := v - mean
		s += d * d
	}
	// Sample variance (n-1) is the unbiased estimator; for n=2 the divisor
	// becomes 1 anyway.
	return math.Sqrt(s / float64(len(xs)-1))
}

func pickPredominant(tally map[string]int) string {
	if len(tally) == 0 {
		return ""
	}
	best := ""
	bestCount := -1
	for k, v := range tally {
		if v > bestCount {
			best = k
			bestCount = v
		}
	}
	return best
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func openJSONL(path string) (*atomicJSONLWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	// Per CONVENTIONS_CHECKLIST §4.2 — write to .tmp + rename.
	tmp, err := os.CreateTemp(dir, ".stage4-backfill-*.jsonl.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	enc := json.NewEncoder(tmp)
	// Return the wrapper so callers' defer w.Close() reaches the atomic
	// flush+rename path. Callers use w.Encode(r) on the inner encoder.
	return &atomicJSONLWriter{Encoder: enc, tmp: tmp, path: tmpPath, finalPath: path}, nil
}

// atomicJSONLWriter flushes the temp file on Close and renames it to finalPath.
type atomicJSONLWriter struct {
	*json.Encoder
	tmp       *os.File
	path      string
	finalPath string
	closed    bool
}

func (a *atomicJSONLWriter) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	if err := a.tmp.Sync(); err != nil {
		_ = a.tmp.Close()
		return fmt.Errorf("sync temp %s: %w", a.path, err)
	}
	if err := a.tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", a.path, err)
	}
	if err := os.Rename(a.path, a.finalPath); err != nil {
		return fmt.Errorf("rename temp %s → %s: %w", a.path, a.finalPath, err)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ------------------------------------------------------------------
// CLI entry.
// ------------------------------------------------------------------

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage(os.Stderr)
		os.Exit(2)
	}
	stats, err := Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stage4 backfill failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Stage 4 backfill complete\n")
	fmt.Printf("  Sessions scanned (window): %d / %d\n", stats.SessionsInWindow, stats.SessionsScanned)
	fmt.Printf("  Macro files (window):      %d / %d\n", stats.MacroFilesInWindow, stats.MacroFilesScanned)
	fmt.Printf("  Rows written:\n")
	fmt.Printf("    regime_history_90d.jsonl:           %d\n", stats.RegimeRowsWritten)
	fmt.Printf("    event_calendar_90d.jsonl:           %d\n", stats.EventCalendarRowsWritten)
	fmt.Printf("    stress_index_history_90d.jsonl:     %d\n", stats.StressIndexRowsWritten)
	fmt.Printf("    prediction_actual_90d.jsonl:        %d\n", stats.PredictionActualRowsWritten)
	fmt.Printf("  Sessions missing summary.json:        %d\n", stats.SessionsMissingSummary)
	fmt.Printf("  Sessions missing outcomes.jsonl:      %d\n", stats.SessionsMissingOutcomes)
	fmt.Printf("  Macro files missing/parse-failed:     %d\n", stats.MacroFilesMissing)
	if !opts.DryRun {
		fmt.Printf("  Staging dir: %s\n", opts.StagingDir)
	}
}

func parseFlags(args []string) (RunOptions, error) {
	fs := flag.NewFlagSet("atlas-stage4-backfill", flag.ContinueOnError)
	source := fs.String("source", "./data/state", "data state root (must contain sessions/ and macro/)")
	out := fs.String("out", "./data/staging", "output directory for 4 staging JSONL files")
	lookback := fs.Int("lookback-days", 90, "lookback window in days (UTC calendar days)")
	dryRun := fs.Bool("dry-run", false, "scan but do not write output files")
	if err := fs.Parse(args); err != nil {
		return RunOptions{}, err
	}
	opts := RunOptions{
		Source:       *source,
		StagingDir:   *out,
		LookbackDays: *lookback,
		DryRun:       *dryRun,
		Now:          time.Now().UTC(),
	}
	if opts.LookbackDays <= 0 {
		return opts, fmt.Errorf("-lookback-days must be > 0")
	}
	return opts, nil
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: atlas-stage4-backfill [-source PATH] [-out PATH] [-lookback-days N] [-dry-run]

Stage 4 historical backfill CLI. Walks data/state/sessions + data/state/macro
for the lookback window and emits 4 staging JSONL files under -out.

	-source         data state root (default ./data/state)
	-out            staging output dir (default ./data/staging)
	-lookback-days  days back from today (default 90)
	-dry-run        scan only; do not write output files
	-h, -help       show this help
`)
}
