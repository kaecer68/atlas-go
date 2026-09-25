// Command backfill-var-returns rebuilds the daily-return series of an atlas
// simulation_state.json from the session summaries on disk.
//
// Usage:
//
//	backfill-var-returns [-sessions-dir DIR] [-state-file FILE] [-on-duplicate last|first] [-dry-run]
//	backfill-var-returns <sessions-dir> <state-file>    # legacy positional form
//
// # Why the date fields matter (#1935)
//
// daily_returns is a "one entry per trading day" series, not "one entry per
// run". Since #1900 / PR #1932 that semantic is owned by sim.Engine.RunDay and
// carried by two fields of domain.SimulationState:
//
//   - LastSessionDate — the trading day the last entry belongs to. RunDay
//     replaces the last entry when it re-runs that day and appends otherwise;
//     without the field it cannot tell a re-run from a new day.
//   - SessionBaseValue — the close of the session *before* LastSessionDate,
//     i.e. the denominator the last entry was computed from.
//
// This command is the 4th writer of that series (after auto_daily_simulation,
// stress_test_daily and POST /admin/trigger-simulation). It used to rewrite
// daily_returns alone and leave both fields untouched, so the rebuilt file had
// no date semantics: the next engine run for a session already present in the
// series appended a second, near-zero entry for the same trading day instead of
// replacing it. Those zeros fill the lower tail ComputeRiskSnapshot reads and
// flatten var95/cvar95 to 0.
//
// The command therefore:
//
//  1. de-duplicates session directories by trading day — several directories
//     can describe the same day (session-<YYYYMMDD>-<replay mode>), and
//     feeding all of them in produced one return per directory, i.e. same-day
//     duplicates (see -on-duplicate for which one wins), and
//  2. writes LastSessionDate and SessionBaseValue with exactly the semantics
//     RunDay uses, so the engine sees the trading day of the last entry as
//     already recorded.
//
// # Deliberate non-goals (#1935)
//
//   - Old files that already contain same-day zero returns are NOT cleaned:
//     a return cannot be attributed to a trading day after the fact.
//   - equity_curve and previous_values["_portfolio_"] are not rebuilt. This
//     command only owns the return series; rewriting those fields would change
//     behavior beyond the issue and could truncate history the file still
//     carries.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Choices for -on-duplicate.
const (
	duplicateLast  = "last"
	duplicateFirst = "first"
)

// minVaRObservations mirrors risk.MinObservationsForVaR: below it the note about
// the 252-gate VaR is printed.
const minVaRObservations = 252

// sessionSummary is the subset of a session's summary.json this command needs.
type sessionSummary struct {
	SessionID      string    `json:"session_id"`
	PortfolioValue float64   `json:"portfolio_value"`
	RecordedAt     time.Time `json:"recorded_at"`
}

// sessionPoint is one de-duplicated trading session: one trading day, one
// winning session directory.
type sessionPoint struct {
	Date       string    // trading day, YYYY-MM-DD (domain.SessionDateLayout)
	Dir        string    // directory name; deterministic tie-break
	RecordedAt time.Time // summary.json recorded_at (zero when absent)
	Value      float64   // summary.json portfolio_value
}

// seriesEntry is one entry of the rebuilt series together with the trading day
// it belongs to. Only the values are persisted; the dates drive the diagnostics
// and the tests.
type seriesEntry struct {
	Date   string
	Return float64
}

// duplicateGroup records one trading day that more than one session directory
// described, so the operator can see which one the rebuild trusted.
type duplicateGroup struct {
	Date    string
	Kept    string
	Dropped []string
}

type options struct {
	sessionsDir string
	stateFile   string
	onDuplicate string
	dryRun      bool
}

// say writes one line of CLI output and deliberately discards the write error:
// this command's exit status must reflect what it did to the state file, not
// whether stdout was still connected when it reported. (errcheck whitelists
// fmt.Fprintf(os.Stderr, ...) but not an io.Writer variable, hence the explicit
// discard.)
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "backfill-var-returns: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	sessions, duplicates, skipped, err := collectSessions(opts.sessionsDir, opts.onDuplicate)
	if err != nil {
		return err
	}
	for _, d := range duplicates {
		say(stdout, "de-duplicated trading day %s: kept %s, dropped %s\n",
			d.Date, d.Kept, strings.Join(d.Dropped, ", "))
	}
	if len(skipped) > 0 {
		say(stderr, "warning: skipped %d session director(y|ies): %s\n",
			len(skipped), strings.Join(skipped, ", "))
	}

	// A series needs a previous close, so a rebuild needs two trading days.
	// Directories that collapsed into one day during de-duplication no longer
	// provide one — report that instead of silently emitting a same-day return.
	if len(sessions) < 2 {
		return fmt.Errorf("need at least 2 sessions on distinct trading days, got %d (from %d session director(y|ies) after de-duplication by trading day); nothing written",
			len(sessions), len(sessions)+countDropped(duplicates))
	}

	entries := rebuildSeries(sessions)
	returns := make([]float64, len(entries))
	for i, e := range entries {
		returns[i] = e.Return
	}
	// The last trading day of the rebuilt series, and the close it is measured
	// against — the same pair RunDay writes for the session it just ran.
	last := sessions[len(sessions)-1]
	base := sessions[len(sessions)-2]

	say(stdout, "Sessions: %d (distinct trading days), Returns: %d\n", len(sessions), len(returns))
	if len(returns) > 0 {
		say(stdout, "First: %.4f%%, Last: %.4f%%\n", returns[0]*100, returns[len(returns)-1]*100)
	}
	say(stdout, "last_session_date: %s, session_base_value: %.2f\n", last.Date, base.Value)
	if len(entries) > 0 && entries[len(entries)-1].Date != last.Date {
		say(stderr, "warning: the newest entry of the rebuilt series belongs to %s while last_session_date is %s (the %s close is not positive); RunDay treats %s as the recorded session\n",
			entries[len(entries)-1].Date, last.Date, last.Date, last.Date)
	}

	if opts.dryRun {
		say(stdout, "dry-run: %s left untouched\n", opts.stateFile)
		return nil
	}

	raw, err := os.ReadFile(opts.stateFile)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	var state domain.SimulationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("parse state: %w", err)
	}
	// Same normalization sim.LoadPersistentState applies, so the file this
	// command writes is loadable by the engine either way.
	if state.Positions == nil {
		state.Positions = make([]domain.Position, 0)
	}
	if state.EquityCurve == nil {
		state.EquityCurve = make([]float64, 0)
	}
	if state.PreviousValues == nil {
		state.PreviousValues = make(map[string]float64)
	}

	say(stdout, "daily_returns: %d -> %d entries\n", len(state.DailyReturns), len(returns))
	state.DailyReturns = returns
	state.LastSessionDate = last.Date
	state.SessionBaseValue = base.Value

	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if unknown := unknownTopLevelKeys(raw, out); len(unknown) > 0 {
		// Faithful warning: the rewrite round-trips through the canonical
		// schema, so keys that schema does not define are dropped.
		say(stderr, "warning: state keys outside domain.SimulationState are dropped by this rewrite: %s\n",
			strings.Join(unknown, ", "))
	}

	tmpPath := opts.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, opts.stateFile); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	say(stdout, "Wrote %d daily returns to %s\n", len(returns), opts.stateFile)
	if len(returns) < minVaRObservations {
		say(stdout, "NOTE: %d more observations needed for full 252-gate VaR.\n", minVaRObservations-len(returns))
		say(stdout, "Use CalculateVaRPercentile for lower-gate VaR estimates.\n")
	}
	return nil
}

// flagNamesTakingAValue lists the flags that consume the next argument, so
// normaliseArgs can keep a flag's value attached to it while reordering.
var flagNamesTakingAValue = map[string]bool{
	"-sessions-dir": true,
	"-state-file":   true,
	"-on-duplicate": true,
}

// normaliseArgs moves flags in front of positional arguments, because the
// stdlib flag package stops parsing at the first non-flag argument. Both
// `cmd -dry-run <dir> <file>` and `cmd <dir> <file> -dry-run` therefore mean the
// same thing.
func normaliseArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	expectValue := false
	for _, arg := range args {
		switch {
		case expectValue:
			flags = append(flags, arg)
			expectValue = false
		case strings.HasPrefix(arg, "-") && arg != "-":
			flags = append(flags, arg)
			// "-flag=value" carries its own value; a bare "-flag" does not.
			expectValue = flagNamesTakingAValue[arg]
		default:
			positional = append(positional, arg)
		}
	}
	return append(flags, positional...)
}

// parseOptions accepts both the flag form and the legacy positional form
// (<sessions-dir> <state-file>), so existing operator invocations keep working.
func parseOptions(args []string) (options, error) {
	opts := options{onDuplicate: duplicateLast}
	fs := flag.NewFlagSet("backfill-var-returns", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.sessionsDir, "sessions-dir", "", "directory holding session-<YYYYMMDD>-<mode> directories")
	fs.StringVar(&opts.stateFile, "state-file", "", "path to simulation_state.json to rewrite")
	fs.StringVar(&opts.onDuplicate, "on-duplicate", duplicateLast,
		"which session directory wins when several describe the same trading day: last (most recently recorded, default) or first (earliest)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "print the plan without writing the state file")
	if err := fs.Parse(normaliseArgs(args)); err != nil {
		return options{}, err
	}

	rest := fs.Args()
	if len(rest) > 2 {
		return options{}, fmt.Errorf("too many positional arguments: %s", strings.Join(rest, " "))
	}
	if len(rest) >= 1 {
		if opts.sessionsDir != "" {
			return options{}, errors.New("sessions directory given both positionally and via -sessions-dir")
		}
		opts.sessionsDir = rest[0]
	}
	if len(rest) == 2 {
		if opts.stateFile != "" {
			return options{}, errors.New("state file given both positionally and via -state-file")
		}
		opts.stateFile = rest[1]
	}
	if opts.sessionsDir == "" || opts.stateFile == "" {
		return options{}, errors.New("usage: backfill-var-returns [-sessions-dir DIR] [-state-file FILE] [-on-duplicate last|first] [-dry-run] | backfill-var-returns <sessions-dir> <state-file>")
	}
	if opts.onDuplicate != duplicateLast && opts.onDuplicate != duplicateFirst {
		return options{}, fmt.Errorf("-on-duplicate must be %q or %q, got %q", duplicateLast, duplicateFirst, opts.onDuplicate)
	}
	return opts, nil
}

// collectSessions reads every session-* directory, keeps the ones that name a
// trading day, sorts them by (trading day, recorded_at, directory name) and
// de-duplicates them by trading day.
//
// It returns the surviving sessions, one group per trading day that had more
// than one directory, and the directories that were skipped (no summary.json,
// unreadable summary.json, or no parseable trading day).
func collectSessions(sessionsDir, onDuplicate string) ([]sessionPoint, []duplicateGroup, []string, error) {
	dirents, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read dir: %w", err)
	}

	points := make([]sessionPoint, 0, len(dirents))
	var skipped []string
	for _, e := range dirents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "session-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name(), "summary.json"))
		if err != nil {
			skipped = append(skipped, e.Name()+" (no summary.json)")
			continue
		}
		var s sessionSummary
		if err := json.Unmarshal(data, &s); err != nil {
			skipped = append(skipped, e.Name()+" (unreadable summary.json)")
			continue
		}
		date := tradingDay(s.SessionID, e.Name())
		if date == "" {
			skipped = append(skipped, e.Name()+" (no trading day in session_id or directory name)")
			continue
		}
		points = append(points, sessionPoint{
			Date:       date,
			Dir:        e.Name(),
			RecordedAt: s.RecordedAt,
			Value:      s.PortfolioValue,
		})
	}

	sort.Slice(points, func(i, j int) bool { return sessionPointLess(points[i], points[j]) })
	sessions, duplicates := dedupeByTradingDay(points, onDuplicate)
	return sessions, duplicates, skipped, nil
}

// sessionPointLess orders sessions by trading day, then by recording time, then
// by directory name. Summary files without recorded_at sort as the oldest of
// their day, which makes the result independent of directory listing order.
func sessionPointLess(a, b sessionPoint) bool {
	if a.Date != b.Date {
		return a.Date < b.Date
	}
	if !a.RecordedAt.Equal(b.RecordedAt) {
		return a.RecordedAt.Before(b.RecordedAt)
	}
	return a.Dir < b.Dir
}

// dedupeByTradingDay keeps exactly one session per trading day: the most
// recently recorded one (default) or the earliest one.
func dedupeByTradingDay(points []sessionPoint, onDuplicate string) ([]sessionPoint, []duplicateGroup) {
	kept := make([]sessionPoint, 0, len(points))
	var groups []duplicateGroup
	for i := 0; i < len(points); {
		j := i
		for j+1 < len(points) && points[j+1].Date == points[i].Date {
			j++
		}
		day := points[i : j+1]
		winner := day[len(day)-1]
		if onDuplicate == duplicateFirst {
			winner = day[0]
		}
		kept = append(kept, winner)
		if len(day) > 1 {
			g := duplicateGroup{Date: winner.Date, Kept: winner.Dir}
			for _, p := range day {
				if p.Dir != winner.Dir {
					g.Dropped = append(g.Dropped, p.Dir)
				}
			}
			groups = append(groups, g)
		}
		i = j + 1
	}
	return kept, groups
}

// tradingDay returns the trading day (YYYY-MM-DD) a session belongs to. The
// session_id is authoritative; the directory name (session-YYYYMMDD-<mode>) is
// the fallback, so a summary with a broken id does not drop a real session.
func tradingDay(sessionID, dirName string) string {
	for _, id := range []string{sessionID, dirName} {
		if day := domain.SessionDateFromID(id); !day.IsZero() {
			return day.Format(domain.SessionDateLayout)
		}
	}
	return ""
}

func countDropped(groups []duplicateGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.Dropped)
	}
	return n
}

// rebuildSeries turns the de-duplicated sessions into one return per trading
// day: entry i belongs to sessions[i+1]. A pair is skipped when either close is
// not positive, exactly as the engine does (it records no return then).
func rebuildSeries(sessions []sessionPoint) []seriesEntry {
	entries := make([]seriesEntry, 0, len(sessions)-1)
	for i := 1; i < len(sessions); i++ {
		prev, curr := sessions[i-1], sessions[i]
		if prev.Value > 0 && curr.Value > 0 {
			entries = append(entries, seriesEntry{
				Date:   curr.Date,
				Return: (curr.Value - prev.Value) / prev.Value,
			})
		}
	}
	return entries
}

// unknownTopLevelKeys lists the top-level keys present in the original file but
// absent from the marshaled canonical schema.
func unknownTopLevelKeys(original, rewritten []byte) []string {
	before := map[string]json.RawMessage{}
	if err := json.Unmarshal(original, &before); err != nil {
		return nil
	}
	after := map[string]json.RawMessage{}
	if err := json.Unmarshal(rewritten, &after); err != nil {
		return nil
	}
	var unknown []string
	for k := range before {
		if _, ok := after[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	return unknown
}
