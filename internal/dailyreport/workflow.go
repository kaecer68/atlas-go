package dailyreport

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Report workflow states.
//
// The report lifecycle is a lightweight state machine (Go-native, no external
// orchestration framework):
//
//		generated ──(auto-generate)──▶ needs_review ──(revise)──▶ corrected
//		                                    │                         │
//		                                    └─────(approve)───────────┴──▶ approved
//
//	  - generated:     initial state of a freshly built Report (zero value).
//	  - needs_review:  an auto-generated report awaiting human review. This is
//	    the state Generate() leaves reports in.
//	  - corrected:     a report that received at least one manual revision
//	    (POST /api/reports/{date}/revise). A corrected report is the version
//	    downstream consumers should use.
//	  - approved:      a report that passed review without revision
//	    (POST /api/reports/{date}/approve), or was approved after corrections.
//
// Cross-day claim tracking uses an independent state machine — see tracker.go.
const (
	WorkflowGenerated   = "generated"
	WorkflowNeedsReview = "needs_review"
	WorkflowCorrected   = "corrected"
	WorkflowApproved    = "approved"
)

// allowedTransitions encodes the report workflow state machine.
// Legacy reports (empty status, persisted before this feature) may transition
// directly to corrected or approved.
var allowedTransitions = map[string]map[string]bool{
	"":                  {WorkflowCorrected: true, WorkflowApproved: true},
	WorkflowGenerated:   {WorkflowNeedsReview: true},
	WorkflowNeedsReview: {WorkflowCorrected: true, WorkflowApproved: true},
	WorkflowCorrected:   {WorkflowCorrected: true, WorkflowApproved: true},
	WorkflowApproved:    {WorkflowCorrected: true, WorkflowApproved: true}, // approve is retry-idempotent
}

// CanTransitionTo reports whether the workflow state machine allows moving
// from the report's current status to target.
func (r *Report) CanTransitionTo(target string) bool {
	allowed := allowedTransitions[r.WorkflowStatus]
	if allowed == nil {
		// Unknown status: be conservative and only allow terminal review states.
		allowed = allowedTransitions[WorkflowGenerated]
	}
	return allowed[target]
}

// RevisionEntry records a single manual revision applied to a report.
type RevisionEntry struct {
	At           time.Time     `json:"at"`
	By           string        `json:"by"`
	Note         string        `json:"note"`
	FieldChanges []FieldChange `json:"field_changes,omitempty"`
}

// FieldChange records one whitelisted field overwrite applied by a revision.
type FieldChange struct {
	Path     string `json:"path"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

// ReviseField is one whitelisted field overwrite in a revise request.
// Value arrives as decoded JSON (float64 for numbers, []any for arrays) and
// is coerced to the target Go type of the field path.
type ReviseField struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// ReviseRequest is the body of POST /api/reports/{date}/revise.
type ReviseRequest struct {
	Note   string        `json:"note"`
	By     string        `json:"by,omitempty"`
	Fields []ReviseField `json:"fields"`
}

// reviseFieldGetters returns the current value of a whitelisted field path.
// Used to snapshot old values before overwriting for the revision history.
type reviseFieldGetter func(r *Report) any

// revocableFields is the whitelist of report fields a human operator may
// overwrite. Paths are the JSON field paths used by the revise API.
// Only StrategySection / PeriodSection / RiskSection fields are revocable;
// Global/Capital/Events remain machine-owned (data provenance).
var revocableFields = map[string]reviseFieldGetter{
	"strategy.active_strategy":  func(r *Report) any { return r.Strategy.Active },
	"strategy.entry_condition":  func(r *Report) any { return r.Strategy.EntryCond },
	"strategy.direction":        func(r *Report) any { return r.Strategy.Direction },
	"period.market_period":      func(r *Report) any { return r.Period.MarketPeriod },
	"period.period_name_zh":     func(r *Report) any { return r.Period.PeriodNameZH },
	"period.cash_reserve":       func(r *Report) any { return r.Period.CashReserve },
	"period.allowed_strategies": func(r *Report) any { return r.Period.AllowedStrategies },
	"period.confidence":         func(r *Report) any { return r.Period.Confidence },
	"period.conditions_hit":     func(r *Report) any { return r.Period.ConditionsHit },
	"period.conditions_total":   func(r *Report) any { return r.Period.ConditionsTotal },
	"risk.stress_index":         func(r *Report) any { return r.Risk.StressIndex },
	"risk.drawdown_alert":       func(r *Report) any { return r.Risk.DrawdownAlert },
	"risk.risk_level":           func(r *Report) any { return r.Risk.RiskLevel },
	"risk.warning":              func(r *Report) any { return r.Risk.Warning },
}

// applyReviseField writes a coerced value into the report at path.
func applyReviseField(r *Report, path string, v any) {
	switch path {
	case "strategy.active_strategy":
		r.Strategy.Active = v.(string)
	case "strategy.entry_condition":
		r.Strategy.EntryCond = v.(string)
	case "strategy.direction":
		r.Strategy.Direction = v.(string)
	case "period.market_period":
		r.Period.MarketPeriod = v.(string)
	case "period.period_name_zh":
		r.Period.PeriodNameZH = v.(string)
	case "period.cash_reserve":
		r.Period.CashReserve = v.(float64)
	case "period.allowed_strategies":
		r.Period.AllowedStrategies = v.([]string)
	case "period.confidence":
		r.Period.Confidence = v.(float64)
	case "period.conditions_hit":
		r.Period.ConditionsHit = v.(int)
	case "period.conditions_total":
		r.Period.ConditionsTotal = v.(int)
	case "risk.stress_index":
		r.Risk.StressIndex = v.(float64)
	case "risk.drawdown_alert":
		r.Risk.DrawdownAlert = v.(bool)
	case "risk.risk_level":
		r.Risk.RiskLevel = v.(string)
	case "risk.warning":
		r.Risk.Warning = v.(string)
	}
}

// coerceReviseField normalizes a decoded JSON value to the target Go type of
// the whitelisted field path.
func coerceReviseField(path string, v any) (any, error) {
	switch path {
	case "period.cash_reserve", "period.confidence", "risk.stress_index":
		f, ok := toFloat64(v)
		if !ok {
			return nil, fmt.Errorf("expected number, got %T", v)
		}
		return f, nil
	case "period.conditions_hit", "period.conditions_total":
		n, ok := toInt(v)
		if !ok {
			return nil, fmt.Errorf("expected integer, got %T", v)
		}
		return n, nil
	case "period.allowed_strategies":
		ss, ok := toStringSlice(v)
		if !ok {
			return nil, fmt.Errorf("expected array of strings, got %T", v)
		}
		return ss, nil
	case "risk.drawdown_alert":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected boolean, got %T", v)
		}
		return b, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", v)
		}
		return s, nil
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func toStringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	}
	return nil, false
}

// ApplyRevision applies a manual revision to the report:
// validates the whitelist, overwrites fields, appends the revision history
// entry and transitions the workflow state to corrected.
func (r *Report) ApplyRevision(req ReviseRequest) error {
	if len(req.Fields) == 0 {
		return errors.New("revise: at least one field required")
	}
	// Reject unknown paths up front so a typo never silently passes.
	paths := make([]string, 0, len(req.Fields))
	values := make(map[string]any, len(req.Fields))
	for _, f := range req.Fields {
		if f.Path == "" {
			return errors.New("revise: field path must not be empty")
		}
		if _, ok := revocableFields[f.Path]; !ok {
			return fmt.Errorf("revise: field %q is not whitelisted", f.Path)
		}
		if _, dup := values[f.Path]; dup {
			return fmt.Errorf("revise: duplicate field %q in request", f.Path)
		}
		paths = append(paths, f.Path)
		values[f.Path] = f.Value
	}
	if r.Period == nil {
		for _, path := range paths {
			if len(path) >= 7 && path[:7] == "period." {
				return errors.New("revise: report has no period section")
			}
		}
	}
	if !r.CanTransitionTo(WorkflowCorrected) {
		return fmt.Errorf("revise: workflow state %q cannot transition to corrected", r.WorkflowStatus)
	}
	sort.Strings(paths)

	entry := RevisionEntry{
		At:   time.Now(),
		By:   req.By,
		Note: req.Note,
	}
	if entry.By == "" {
		entry.By = "admin"
	}
	for _, path := range paths {
		value, err := coerceReviseField(path, values[path])
		if err != nil {
			return fmt.Errorf("revise: field %s: %w", path, err)
		}
		old := revocableFields[path](r)
		applyReviseField(r, path, value)
		entry.FieldChanges = append(entry.FieldChanges, FieldChange{
			Path:     path,
			OldValue: old,
			NewValue: value,
		})
	}

	r.WorkflowStatus = WorkflowCorrected
	r.RevisedAt = entry.At
	r.RevisedBy = entry.By
	r.RevisionNote = entry.Note
	r.RevisionHistory = append(r.RevisionHistory, entry)
	return nil
}

// MarkNeedsReview performs the automatic generated → needs_review transition
// (what Generate() does when it stamps a fresh report).
func (r *Report) MarkNeedsReview() error {
	if !r.CanTransitionTo(WorkflowNeedsReview) {
		return fmt.Errorf("needs_review: workflow state %q cannot transition to needs_review", r.WorkflowStatus)
	}
	r.WorkflowStatus = WorkflowNeedsReview
	return nil
}

// Approve transitions the report to the approved workflow state.
func (r *Report) Approve() error {
	if !r.CanTransitionTo(WorkflowApproved) {
		return fmt.Errorf("approve: workflow state %q cannot transition to approved", r.WorkflowStatus)
	}
	r.WorkflowStatus = WorkflowApproved
	return nil
}
