package capitalflow

// Shared runner for the Phase 1 pre-registered hypothesis validator
// (H-CF-01 / H-CF-02 / H-CF-05). Extracted from the offline CLI
// (cmd/validate-capital-flow-hypotheses) so the scheduled
// cf_hypothesis_validation task can rerun the exact same read-only
// procedure in-process instead of exec'ing the binary.
//
// Governance boundaries (docs/specs/capital-flow-seven-dimension-spec.md
// §10; plan .omo/plans/2026-09-04-capital-flow-model-plan.md §3):
//
//   - This runner is strictly OFFLINE and READ-ONLY: it reads local
//     snapshots under workdir and never touches the network, so it
//     burns no FinMind quota. Safe to schedule pre-market.
//   - It produces a versioned validation report only; it NEVER writes
//     state or config, and flipping automation eligibility stays a
//     separate human config PR (CF-INV-13).
//   - The scheduled rerun only handles "data unlock" re-judgements
//     (INSUFFICIENT_DATA → a real verdict once samples cross the
//     pre-registered 252-day threshold). Governance one-shot
//     executions — e.g. the H-CF-01 v2a/v2a′/v2b same-batch Holm
//     correction — are manual-only by design.
//   - All decision thresholds are compile-time constants in this
//     package; nothing here accepts verdict parameters.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default input locations for RunHypothesisValidation, relative to the
// workdir. These mirror the CLI defaults; the scheduled task uses the
// zero-value ValidationInputs to get exactly these.
const (
	ValidationDefaultReplayPath  = "data/replay/tw_extended_90days.csv"
	ValidationDefaultOIDir       = "data/state/taifex_oi"
	ValidationDefaultT86Dir      = "data/state/capital_flow"
	ValidationDefaultMacroDir    = "data/state/macro"
	ValidationDefaultRollingPath = "data/state/capital_flow_rolling.json"
	validationSentinelFutureDate = "9999-12-31"
	validationRollingCapacity    = 252 // must match the server wiring (main.go uses 252)
)

// ValidationInputs selects the offline inputs for a validation run.
// Empty fields fall back to the ValidationDefault* constants; the
// optional date window restricts the trading calendar.
type ValidationInputs struct {
	WorkDir     string
	StartDate   string // YYYY-MM-DD inclusive, empty = no lower bound
	EndDate     string // YYYY-MM-DD inclusive, empty = no upper bound
	ReplayPath  string // trading calendar CSV, relative to WorkDir
	OIDir       string
	T86Dir      string
	MacroDir    string
	RollingPath string
}

func (in ValidationInputs) replay() string {
	if in.ReplayPath == "" {
		return ValidationDefaultReplayPath
	}
	return in.ReplayPath
}

func (in ValidationInputs) oiDir() string {
	if in.OIDir == "" {
		return ValidationDefaultOIDir
	}
	return in.OIDir
}

func (in ValidationInputs) t86Dir() string {
	if in.T86Dir == "" {
		return ValidationDefaultT86Dir
	}
	return in.T86Dir
}

func (in ValidationInputs) macroDir() string {
	if in.MacroDir == "" {
		return ValidationDefaultMacroDir
	}
	return in.MacroDir
}

func (in ValidationInputs) rollingPath() string {
	if in.RollingPath == "" {
		return ValidationDefaultRollingPath
	}
	return in.RollingPath
}

// macroRow is the slice of a macro snapshot the validator needs:
// TAIEX close (target series) and TSM ADR daily change in percent.
type macroRow struct {
	taiex    float64
	adrPct   float64
	hasADR   bool
	hasTaiex bool
}

// RunHypothesisValidation loads the offline inputs from workdir and
// replays the three pre-registered validators, returning the assembled
// report. It performs no writes of its own (the caller decides whether
// to persist the report) and no network I/O.
func RunHypothesisValidation(ctx context.Context, in ValidationInputs) (ValidationReport, error) {
	if in.StartDate != "" || in.EndDate != "" {
		if err := ValidateValidationDateArg(in.StartDate); err != nil {
			return ValidationReport{}, fmt.Errorf("start date: %w", err)
		}
		if err := ValidateValidationDateArg(in.EndDate); err != nil {
			return ValidationReport{}, fmt.Errorf("end date: %w", err)
		}
	}

	dates, err := loadValidationTradingDates(in)
	if err != nil {
		return ValidationReport{}, err
	}
	oi, err := LoadValidationFuturesOI(filepath.Join(in.WorkDir, in.oiDir()))
	if err != nil {
		return ValidationReport{}, err
	}
	spot, err := LoadValidationForeignSpot(filepath.Join(in.WorkDir, in.t86Dir()))
	if err != nil {
		return ValidationReport{}, err
	}
	macro, err := LoadValidationMacroSnapshots(filepath.Join(in.WorkDir, in.macroDir()))
	if err != nil {
		return ValidationReport{}, err
	}
	samples, err := loadValidationRollingSamples(ctx, filepath.Join(in.WorkDir, in.rollingPath()))
	if err != nil {
		return ValidationReport{}, err
	}

	taiex := make(map[string]float64, len(macro))
	adr := make(map[string]float64, len(macro))
	for d, row := range macro {
		if row.hasTaiex {
			taiex[d] = row.taiex
		}
		if row.hasADR {
			adr[d] = row.adrPct
		}
	}

	results := []HypothesisResult{
		ValidateHypothesis01(oi, spot, dates),
		ValidateHypothesis02(adr, taiex, dates),
		ValidateHypothesis05(samples, taiex, dates),
	}
	return BuildValidationReport(in.WorkDir, map[string]int{
		"calendar_days": len(dates),
		"oi_days":       len(oi),
		"t86_spot_days": len(spot),
		"macro_days":    len(macro),
		"taiex_days":    len(taiex),
		"adr_days":      len(adr),
	}, results), nil
}

// ValidateValidationDateArg checks a YYYY-MM-DD window argument.
func ValidateValidationDateArg(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("bad date %q: %w", s, err)
	}
	return nil
}

// loadValidationTradingDates loads the replay trading calendar and
// restricts it to the optional start/end window. It reuses the
// production loader so a malformed calendar row is a hard error.
func loadValidationTradingDates(in ValidationInputs) ([]string, error) {
	dates, err := LoadReplayTradingDates(filepath.Join(in.WorkDir, in.replay()))
	if err != nil {
		return nil, fmt.Errorf("load trading calendar: %w", err)
	}
	if in.StartDate == "" && in.EndDate == "" {
		return dates, nil
	}
	out := make([]string, 0, len(dates))
	for _, d := range dates {
		if in.StartDate != "" && d < in.StartDate {
			continue
		}
		if in.EndDate != "" && d > in.EndDate {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// LoadValidationFuturesOI reads every YYYY-MM-DD.json TAIFEX
// institutional snapshot under dir and returns the TX (大台) foreign
// open-interest net in contracts, keyed by trading date.
func LoadValidationFuturesOI(dir string) (map[string]float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("read OI dir: %w", err)
	}
	out := make(map[string]float64, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue // not a dated snapshot
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var raw struct {
			Contracts map[string]struct {
				Foreign struct {
					OINet int64 `json:"oi_net"`
				} `json:"foreign"`
			} `json:"contracts"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		tx, ok := raw.Contracts["TX"]
		if !ok {
			continue
		}
		out[date] = float64(tx.Foreign.OINet)
	}
	return out, nil
}

// LoadValidationForeignSpot reuses the production T86 loader: foreign
// investor spot net (hundred_million_shares) keyed by trading date.
// A missing directory is tolerated as "no data yet" (the wrapped error
// from LoadT86CapitalFlow would hide os.IsNotExist, so the existence
// check happens here).
func LoadValidationForeignSpot(dir string) (map[string]float64, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("stat T86 dir: %w", err)
	}
	recs, err := LoadT86CapitalFlow(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(recs))
	for d, r := range recs {
		out[d] = r.ForeignNet
	}
	return out, nil
}

// LoadValidationMacroSnapshots reads the dated macro snapshots under
// dir and extracts the TAIEX close and TSM ADR change percent. Older
// files without a tsm_adr channel simply contribute no ADR sample.
func LoadValidationMacroSnapshots(dir string) (map[string]macroRow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]macroRow{}, nil
		}
		return nil, fmt.Errorf("read macro dir: %w", err)
	}
	out := make(map[string]macroRow, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "latest.json" || name == "previous.json" {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var raw struct {
			Taiex struct {
				Value float64 `json:"value"`
			} `json:"taiex"`
			TSMADR struct {
				ChangePct float64 `json:"change_pct"`
			} `json:"tsm_adr"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		row := macroRow{taiex: raw.Taiex.Value, hasTaiex: raw.Taiex.Value != 0}
		if raw.TSMADR.ChangePct != 0 {
			row.adrPct = raw.TSMADR.ChangePct
			row.hasADR = true
		}
		out[date] = row
	}
	return out, nil
}

// loadValidationRollingSamples reads back the full persisted rolling
// store (all seven dimensions) via the production store contract.
func loadValidationRollingSamples(ctx context.Context, path string) (map[ForceName][]RollingSample, error) {
	store := NewFileRollingSampleStore(path, validationRollingCapacity)
	dims := []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail,
	}
	out := make(map[ForceName][]RollingSample, len(dims))
	for _, dim := range dims {
		rows, err := store.History(ctx, dim, validationSentinelFutureDate, validationRollingCapacity)
		if err != nil {
			// A missing store file means the dimension simply has no
			// history; every hypothesis then reports
			// INSUFFICIENT_DATA, which is the honest outcome.
			if os.IsNotExist(err) {
				out[dim] = nil
				continue
			}
			return nil, fmt.Errorf("read rolling store %s: %w", dim, err)
		}
		out[dim] = rows
	}
	return out, nil
}
