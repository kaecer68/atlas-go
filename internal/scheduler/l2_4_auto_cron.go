// Package scheduler — L2.4 observation auto-cron safety gate.
//
// DEPRECATED (2026-08-06, Issue #825 closed):
//   - ShouldL24AutoCronFire has ZERO production callers (only test calls it).
//     gitnexus_impact: 0 affected processes — pure function library, never
//     wired into BackgroundTaskManager.Register().
//   - The automation gap this gate addressed is now covered by the C07
//     sector-prediction parallel track (cmd/experimental/c07-obs-collector
//   - c07-day-evaluator + c07-preflight, production-deployed with cron
//     entrypoint) and the canonical launch-gate pattern in
//     internal/startup/preflight.go (PR #1037).
//   - L2.4 observation itself never started (use_llm_sector_agents.value
//     still false), so no baseline exists to validate an auto-cron.
//   - This file is KEPT (not removed) because the 5-condition gate logic is
//     a tested, self-contained safety net that can be reused if L2.4 is ever
//     restarted. Do NOT wire it into production without re-opening Issue #825.
//
// Original intent (PR #1029, 2026-07-08):
//
// This file ships the gate logic for followup.md §1 prereq #3
// (Issue #825 auto-cron scheduler) — but DEFAULT-DISABLED.
//
// followup.md §1 currently says: 「是否現在可以開始實作？否」
//
// This PR does NOT violate that decision because:
//
//  1. The cron SCHEDULER is not wired into BackgroundTaskManager.
//     The actual `Register()` call against `apigateway.BackgroundTaskManager`
//     is deferred to a separate PR after followup.md §1 prereqs are satisfied.
//
//  2. Even if someone wires it (against this PR's recommendation),
//     ShouldL24AutoCronFire() returns false UNLESS ALL FIVE conditions are true:
//
//     a. Env var L2_4_AUTO_CRON_ENABLED = "true"  (explicit opt-in)
//     b. parameters.AutoEnabled = true           (parameters.json flag)
//     c. Observation log file exists              (Week 0 Baseline filed)
//     d. Observation log contains Day 7+ entry    (observation progressed)
//     e. Current time within configured cron window
//
//  3. Condition (d) is the binding gate — even with env var set and
//     auto_enabled=true, if observation hasn't reached Day 7+ in
//     the observation log, cron NO-OPs.
//
// Net effect: shipping this PR puts the gate LOGIC in main, but no
// cron will fire until:
//   - Operator sets L2_4_AUTO_CRON_ENABLED=true
//   - Operator sets parameters.json auto_enabled=true
//   - Operator runs a successful Day 7 observation cycle
//   - Current time is within cron window
//
// That's exactly the "graduation" followup.md §1 was asking for.
package scheduler

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

const (
	// L24AutoCronEnvVar is the explicit operator opt-in.
	// DEFAULT-NOT-SET. Cron never fires unless this is explicitly "true".
	L24AutoCronEnvVar = "L2_4_AUTO_CRON_ENABLED"

	// L24AutoCronDefaultObservationLogPath is the default location
	// for the operator's manual observation log.
	L24AutoCronDefaultObservationLogPath = ".omo/evidence/l2-4-observation-log.md"

	// L24AutoCronDefaultCronWindow is the default firing window
	// (weekdays 09:00-09:30 UTC, ~30 min buffer for trading-day open).
	L24AutoCronDefaultCronWindowStart = "09:00"
	L24AutoCronDefaultCronWindowEnd   = "09:30"
	L24AutoCronDefaultCronDays        = "Mon-Fri"
)

// AutoCronGateState captures the result of the gate evaluation.
// Returned by ShouldL24AutoCronFire for logging + manual inspection.
type AutoCronGateState struct {
	EnvVarSet        bool
	AutoEnabledParam bool
	ObservationLogOK bool
	HasDay7Entry     bool
	InCronWindow     bool
	Fired            bool
	SkippedReason    string
}

// ParametersL24Access is the minimal ParametersConfig interface used by this
// gate. Decoupled from full ParametersConfig so tests can pass a stub.
type ParametersL24Access interface {
	GetAutoEnabled() bool
}

// ParametersConfigL24Adapter adapts the real *config.ParametersConfig to
// the ParametersL24Access interface.
type ParametersConfigL24Adapter struct {
	Cfg *config.ParametersConfig
}

func (a *ParametersConfigL24Adapter) GetAutoEnabled() bool {
	if a == nil || a.Cfg == nil {
		return false
	}
	// Field path verified via grep parameters.go:281 (L2_4Schedule)
	// and parameters.go:90 (AutoEnabled inside L2_4ScheduleParameters).
	return a.Cfg.Orchestrator.L2_4Schedule.AutoEnabled.Value
}

// ShouldL24AutoCronFire returns the gate state. If Fired=true, all 5 conditions
// are met. If Fired=false, SkippedReason explains which condition failed.
//
// IMPORTANT: This function does NOT actually call l24Mgr.Start/Stop.
// It only verifies the gate. The wiring of gate → actual scheduler is
// a separate concern (deliberately deferred per followup.md §1).
func ShouldL24AutoCronFire(
	params ParametersL24Access,
	observationLogPath string,
	cronWindowStart, cronWindowEnd, cronDays string,
	now time.Time,
) AutoCronGateState {
	state := AutoCronGateState{}

	// Condition 1: explicit operator opt-in via env var
	if os.Getenv(L24AutoCronEnvVar) != "true" {
		state.SkippedReason = fmt.Sprintf("env var %s != \"true\" (default-off)", L24AutoCronEnvVar)
		return state
	}
	state.EnvVarSet = true

	// Condition 2: parameters.json auto_enabled flag
	if params == nil {
		state.SkippedReason = "ParametersL24Access is nil"
		return state
	}
	if !params.GetAutoEnabled() {
		state.SkippedReason = "parameters.AutoEnabled = false (must be true)"
		return state
	}
	state.AutoEnabledParam = true

	// Condition 3: observation log file exists
	if observationLogPath == "" {
		observationLogPath = L24AutoCronDefaultObservationLogPath
	}
	if _, err := os.Stat(observationLogPath); err != nil {
		state.SkippedReason = fmt.Sprintf("observation log not found at %s", observationLogPath)
		return state
	}
	state.ObservationLogOK = true

	// Condition 4: observation log has Day 7+ entry (observation progressed)
	data, err := os.ReadFile(observationLogPath)
	if err != nil {
		state.SkippedReason = fmt.Sprintf("cannot read observation log: %v", err)
		return state
	}
	state.HasDay7Entry = hasDay7PlusEntry(string(data))
	if !state.HasDay7Entry {
		state.SkippedReason = "observation log has no Day 7+ entry (Day 14 acceptance gate unreachable)"
		return state
	}

	// Condition 5: current time within cron window
	if cronWindowStart == "" {
		cronWindowStart = L24AutoCronDefaultCronWindowStart
	}
	if cronWindowEnd == "" {
		cronWindowEnd = L24AutoCronDefaultCronWindowEnd
	}
	if cronDays == "" {
		cronDays = L24AutoCronDefaultCronDays
	}
	inWindow, err := inCronWindow(now, cronWindowStart, cronWindowEnd, cronDays)
	if err != nil {
		state.SkippedReason = fmt.Sprintf("invalid cron window: %v", err)
		return state
	}
	state.InCronWindow = inWindow
	if !inWindow {
		state.SkippedReason = fmt.Sprintf("current time %s outside cron window %s-%s %s",
			now.Format(time.RFC3339), cronWindowStart, cronWindowEnd, cronDays)
		return state
	}

	state.Fired = true
	return state
}

// dayMarkerRegex matches "Day 7" through "Day 30" with word boundaries.
// Word boundaries prevent "Day 99" from accidentally matching "Day 9" via
// naive substring search — both "9"s are word characters, so \b fails between
// them. Pattern: \bDay (7|8|9|10|11|...|29|30)\b.
var dayMarkerRegex = regexp.MustCompile(`\bDay (?:[7-9]|1[0-9]|2[0-9]|30)\b`)

// hasDay7PlusEntry returns true if the observation log contains any Day 7+
// marker. Conservative — looks for explicit "Day N" strings in comments or
// section headers, which is how PR #1019's example entries format them.
func hasDay7PlusEntry(content string) bool {
	return dayMarkerRegex.MatchString(content)
}

// inCronWindow checks if `now` falls within [start, end] on any day in daysSpec.
// daysSpec supports "Mon-Fri" range or "Mon,Wed,Fri" list. Times are HH:MM 24h.
//
// Fail-safe: returns (false, err) on any parse error.
func inCronWindow(now time.Time, startStr, endStr, daysSpec string) (bool, error) {
	startHour, startMin, err := parseClock(startStr)
	if err != nil {
		return false, fmt.Errorf("start time: %w", err)
	}
	endHour, endMin, err := parseClock(endStr)
	if err != nil {
		return false, fmt.Errorf("end time: %w", err)
	}
	if startHour*60+startMin > endHour*60+endMin {
		return false, fmt.Errorf("start %s after end %s", startStr, endStr)
	}

	weekday := now.Weekday().String()[:3] // "Mon", "Tue", etc.
	if !dayMatches(weekday, daysSpec) {
		return false, nil
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	return nowMinutes >= startHour*60+startMin && nowMinutes <= endHour*60+endMin, nil
}

func parseClock(s string) (hour, minute int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid clock format: %s (want HH:MM)", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("hour %q: %w", parts[0], err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("minute %q: %w", parts[1], err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("clock out of range: %s", s)
	}
	return h, m, nil
}

var allDays = map[string]int{"Mon": 0, "Tue": 1, "Wed": 2, "Thu": 3, "Fri": 4, "Sat": 5, "Sun": 6}

func dayMatches(weekday, daysSpec string) bool {
	weekdayNum, ok := allDays[weekday]
	if !ok {
		return false
	}

	if strings.Contains(daysSpec, "-") {
		parts := strings.Split(daysSpec, "-")
		if len(parts) == 2 {
			startNum, startOK := allDays[strings.TrimSpace(parts[0])]
			endNum, endOK := allDays[strings.TrimSpace(parts[1])]
			if startOK && endOK {
				return weekdayNum >= startNum && weekdayNum <= endNum
			}
		}
	}

	for _, day := range strings.Split(daysSpec, ",") {
		if strings.TrimSpace(day) == weekday {
			return true
		}
	}

	return false
}
