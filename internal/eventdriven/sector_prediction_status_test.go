package eventdriven

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ── I4 / I5 / I6 (#1944 Batch 3) ─────────────────────────────────────────
//
// The sector-prediction path used to surface its wiring state only as a silent
// `sector_predictions: []`. These tests pin the machine-readable replacement:
// PredictionReport.SectorPredictionStatus, driven by the live predictor state.

type stubMacroProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (s stubMacroProvider) Name() string { return "stub" }

func (s stubMacroProvider) FetchSnapshot(context.Context) (marketdata.MacroDataSnapshot, error) {
	return s.snap, s.err
}

func staleSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		TAIEX: marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 20000, ChangePct: 1.2},
		SOXIndex: marketdata.MacroDataPoint{
			Symbol: "SOX", Value: 5000, ChangePct: 2.0,
		},
		NVDA:               marketdata.MacroDataPoint{Symbol: "NVDA", Value: 120, ChangePct: 3.0},
		TSMADR:             marketdata.MacroDataPoint{Symbol: "TSM", Value: 180, ChangePct: 2.5},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "foreign", Value: 1000, ChangePct: 1.0},
	}
}

func predictOnce(t *testing.T, h *Handler) PredictionReport {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	code, data := h.HandlePrediction(req)
	if code != http.StatusOK {
		t.Fatalf("HandlePrediction status = %d, want 200", code)
	}
	report, ok := data.(PredictionReport)
	if !ok {
		t.Fatalf("HandlePrediction returned %T, want PredictionReport", data)
	}
	return report
}

// TestProductionSectorPredictionStatusFlagOff pins the shipped production shape:
// SECTOR_PREDICTION_ENABLED defaults to false, cmd/atlas therefore never calls
// SetMacroProvider, and /api/events/prediction must SAY so instead of returning
// a bare empty array (I5).
func TestProductionSectorPredictionStatusFlagOff(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	report := predictOnce(t, h)

	if report.SectorPredictions == nil {
		t.Error("SectorPredictions must be a non-nil empty slice, not nil")
	}
	if len(report.SectorPredictions) != 0 {
		t.Errorf("expected 0 sector days with the flag off, got %d", len(report.SectorPredictions))
	}
	st := report.SectorPredictionStatus
	if st == nil {
		t.Fatal("SectorPredictionStatus must be populated so a disabled flag is not silent")
	}
	if st.Enabled {
		t.Error("status.Enabled must be false without a macro provider")
	}
	if st.Applied {
		t.Error("status.Applied must be false when no rows were produced")
	}
	if st.Reason != SectorPredictionReasonFlagDisabled {
		t.Errorf("status.Reason = %q, want %q", st.Reason, SectorPredictionReasonFlagDisabled)
	}
	if st.Days != 0 || st.SectorRows != 0 {
		t.Errorf("status shape = %d days / %d rows, want 0/0", st.Days, st.SectorRows)
	}
	if st.StrategicPriorApplied {
		t.Error("status.StrategicPriorApplied must be false when no predictor exists")
	}
	if st.CycleProviderWired != SectorCycleProviderWired {
		t.Errorf("status.CycleProviderWired = %v, want %v", st.CycleProviderWired, SectorCycleProviderWired)
	}
	if st.Persisted != SectorPredictionPersisted {
		t.Errorf("status.Persisted = %v, want %v", st.Persisted, SectorPredictionPersisted)
	}
	if st.PersistenceReason != SectorPredictionPersistenceReason {
		t.Errorf("status.PersistenceReason = %q, want %q", st.PersistenceReason, SectorPredictionPersistenceReason)
	}
}

// TestInjectedSectorPredictorSurvivesFlagOffRequest pins that the fail-closed
// reset in HandlePrediction only replaces predictors the handler built itself:
// an explicitly injected predictor is the caller's, not the flag's, business.
func TestInjectedSectorPredictorSurvivesFlagOffRequest(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	h.SetSectorPredictor(NewSectorPredictor(nil, nil))
	report := predictOnce(t, h)

	st := report.SectorPredictionStatus
	if st == nil {
		t.Fatal("SectorPredictionStatus must be populated")
	}
	if st.Enabled {
		t.Error("status.Enabled must be false: no macro provider is wired")
	}
	if !st.Applied {
		t.Error("an explicitly injected predictor must keep producing rows")
	}
	if st.Reason != "" {
		t.Errorf("status.Reason = %q, want empty when rows were produced", st.Reason)
	}
	if st.SectorRows != 5*20 {
		t.Errorf("status.SectorRows = %d, want 100", st.SectorRows)
	}
}

// TestSectorPredictionsAreNeverPersisted pins I6 in machine-readable form: the
// ledger landing type has no sector column, so sector rows cannot be
// reconciled and must never be reported as persisted.
func TestSectorPredictionsAreNeverPersisted(t *testing.T) {
	if SectorPredictionPersisted {
		t.Fatal("SectorPredictionPersisted must stay false until the ledger stores sector rows (I6/I32)")
	}
	if SectorPredictionPersistenceReason == "" {
		t.Fatal("SectorPredictionPersistenceReason must name the blocking schema gap")
	}
	if SectorCycleProviderWired {
		t.Fatal("SectorCycleProviderWired must stay false until one shared, measured CycleTracker is wired (I4/I13)")
	}
}

// TestSectorPredictionStatusWithMacroButNoPrior pins the state that existed
// before this change: predictor built, macro drivers active, prior unwired ⇒
// the overall_baseline contribution is identically 0 and must not be claimed.
func TestSectorPredictionStatusWithMacroButNoPrior(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	h.SetMacroProvider(stubMacroProvider{snap: staleSnapshot()})
	report := predictOnce(t, h)

	st := report.SectorPredictionStatus
	if st == nil {
		t.Fatal("SectorPredictionStatus must be populated")
	}
	if !st.Enabled {
		t.Error("status.Enabled must be true when a macro provider is wired (flag on)")
	}
	if !st.Applied {
		t.Fatalf("status.Applied must be true; reason=%q", st.Reason)
	}
	if st.Reason != "" {
		t.Errorf("status.Reason = %q, want empty when applied", st.Reason)
	}
	if st.Days != 5 || st.SectorRows != 5*20 {
		t.Errorf("status shape = %d days / %d rows, want 5 / 100", st.Days, st.SectorRows)
	}
	if st.StrategicPriorApplied {
		t.Error("status.StrategicPriorApplied must be false without SetSectorStrategicPrior")
	}
	if st.CycleProviderWired {
		t.Error("status.CycleProviderWired must be false; production never calls SetSectorCycleProvider")
	}
	// overall_baseline can only contribute when a prior is attached.
	for _, day := range report.SectorPredictions {
		for _, s := range day.Sectors {
			for _, d := range s.Drivers {
				if d == "overall_baseline" {
					t.Fatalf("sector %s reported an overall_baseline driver while no prior is wired", s.SectorID)
				}
				if d == "cycle_position" {
					t.Fatalf("sector %s reported a cycle_position driver while no cycle provider is wired", s.SectorID)
				}
			}
		}
	}
}

// TestSectorPredictionStatusWiresStrategicPrior pins item I4: the strategic
// prior is now attached to every predictor instance the handler builds, and the
// attached prior actually drives the advertised `overall_baseline` contribution.
func TestSectorPredictionStatusWiresStrategicPrior(t *testing.T) {
	prior, err := sectorallocation.LoadStrategicPrior(config.GetParametersConfig())
	if err != nil {
		t.Fatalf("LoadStrategicPrior: %v", err)
	}
	if prior.Source != "heuristic" {
		t.Fatalf("test premise: production prior source = %q, want heuristic", prior.Source)
	}

	h := NewHandler(industry.NewEventCalendar())
	h.SetMacroProvider(stubMacroProvider{snap: staleSnapshot()})
	h.SetSectorStrategicPrior(prior)
	report := predictOnce(t, h)

	st := report.SectorPredictionStatus
	if st == nil || !st.StrategicPriorApplied {
		t.Fatalf("status.StrategicPriorApplied = %v, want true after SetSectorStrategicPrior", st)
	}
	if st.CycleProviderWired {
		t.Error("status.CycleProviderWired must stay false")
	}

	// Consumption evidence: the same macro input without a prior must yield a
	// different distribution for a sector whose prior weight is non-zero,
	// i.e. the prior really reaches predictSector() and is not merely stored.
	plain := NewHandler(industry.NewEventCalendar())
	plain.SetMacroProvider(stubMacroProvider{snap: staleSnapshot()})
	plainReport := predictOnce(t, plain)

	withDist := semiconductorDistribution(t, report)
	withoutDist := semiconductorDistribution(t, plainReport)
	if withDist == withoutDist {
		t.Errorf("strategic prior had no effect on the semiconductor distribution (%+v)", withDist)
	}

	// It is applied on the rebuild path too (the second request re-enters
	// HandlePrediction), i.e. it is not dropped when the predictor instance is
	// replaced.
	second := predictOnce(t, h)
	if second.SectorPredictionStatus == nil || !second.SectorPredictionStatus.StrategicPriorApplied {
		t.Fatal("strategic prior must survive a predictor rebuild")
	}
}

func semiconductorDistribution(t *testing.T, report PredictionReport) PredictionDistribution {
	t.Helper()
	for _, day := range report.SectorPredictions {
		for _, s := range day.Sectors {
			if s.SectorID == string(industry.SectorSemiconductor) {
				return s.Distribution
			}
		}
	}
	t.Fatalf("no semiconductor entry in sector predictions (rows=%d)", len(report.SectorPredictions))
	return PredictionDistribution{}
}

// TestStrategicPriorDrivesOverallBaselineDriver pins, at the predictor level,
// that attaching a strategic prior is what makes the advertised
// `overall_baseline` contribution non-zero (I4). With no macro snapshot and no
// active events, `overall_baseline` is the only possible driver.
func TestStrategicPriorDrivesOverallBaselineDriver(t *testing.T) {
	prior, err := sectorallocation.LoadStrategicPrior(config.GetParametersConfig())
	if err != nil {
		t.Fatalf("LoadStrategicPrior: %v", err)
	}
	fp := FlowPrediction{
		Date:         time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Direction:    "inflow",
		Confidence:   0.8,
		Distribution: PredictionDistribution{Inflow: 0.6, Neutral: 0.2, Outflow: 0.2},
	}

	withoutPrior := NewSectorPredictor(nil, nil)
	withPrior := NewSectorPredictor(nil, nil)
	withPrior.SetStrategicPrior(prior)

	plain := sectorByName(t, withoutPrior.Predict([]FlowPrediction{fp}, nil), industry.SectorSemiconductor)
	priored := sectorByName(t, withPrior.Predict([]FlowPrediction{fp}, nil), industry.SectorSemiconductor)

	if slices.Contains(plain.Drivers, "overall_baseline") {
		t.Fatalf("unwired prior produced an overall_baseline driver: %v", plain.Drivers)
	}
	if !slices.Contains(priored.Drivers, "overall_baseline") {
		t.Fatalf("wired prior did not produce an overall_baseline driver: %v", priored.Drivers)
	}
	if plain.Distribution == priored.Distribution {
		t.Errorf("prior did not change the distribution: %+v", priored.Distribution)
	}
	for _, s := range []SectorPrediction{plain, priored} {
		if slices.Contains(s.Drivers, "cycle_position") {
			t.Errorf("cycle_position driver present although no cycle provider is wired: %v", s.Drivers)
		}
	}
}

func sectorByName(t *testing.T, days []SectorDayPrediction, sid industry.SectorID) SectorPrediction {
	t.Helper()
	if len(days) == 0 {
		t.Fatal("no sector days produced")
	}
	for _, s := range days[0].Sectors {
		if s.SectorID == string(sid) {
			return s
		}
	}
	t.Fatalf("sector %s missing from predictions", sid)
	return SectorPrediction{}
}

// TestSectorPredictionStatusMacroUnavailable pins the fail-closed branch: a
// failed snapshot must not silently reuse a stale predictor.
func TestSectorPredictionStatusMacroUnavailable(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	h.SetMacroProvider(stubMacroProvider{err: errors.New("boom")})
	report := predictOnce(t, h)

	st := report.SectorPredictionStatus
	if st == nil {
		t.Fatal("SectorPredictionStatus must be populated")
	}
	if !st.Enabled {
		t.Error("status.Enabled must be true: the flag (macro provider) is wired")
	}
	if st.Applied {
		t.Error("status.Applied must be false when the snapshot fetch failed")
	}
	if st.Reason != SectorPredictionReasonMacroUnavailable {
		t.Errorf("status.Reason = %q, want %q", st.Reason, SectorPredictionReasonMacroUnavailable)
	}
	if len(report.SectorPredictions) != 0 {
		t.Errorf("expected no sector days, got %d", len(report.SectorPredictions))
	}
}

// TestSectorPredictionStatusJSONContract pins the wire contract of the status
// block: the exact keys a consumer (frontend, MCP tool, c07 collector) reads,
// with the shipped flag-off values. This is the evidence that a disabled
// SECTOR_PREDICTION_ENABLED is visible instead of a silent empty array (I5).
func TestSectorPredictionStatusJSONContract(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	report := predictOnce(t, h)

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if !strings.Contains(string(raw), `"sector_prediction_status"`) {
		t.Fatalf("report JSON has no sector_prediction_status key: %s", raw)
	}
	var decoded struct {
		SectorPredictions []json.RawMessage `json:"sector_predictions"`
		Status            struct {
			Enabled            bool   `json:"enabled"`
			Applied            bool   `json:"applied"`
			Days               int    `json:"days"`
			SectorRows         int    `json:"sector_rows"`
			StrategicPrior     bool   `json:"strategic_prior_applied"`
			CycleProviderWired bool   `json:"cycle_provider_wired"`
			Persisted          bool   `json:"persisted"`
			PersistenceReason  string `json:"persistence_reason"`
			Reason             string `json:"reason"`
		} `json:"sector_prediction_status"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if decoded.Status.Enabled || decoded.Status.Applied {
		t.Errorf("flag-off status = %+v, want enabled=false applied=false", decoded.Status)
	}
	if decoded.Status.Reason != SectorPredictionReasonFlagDisabled {
		t.Errorf("reason = %q, want %q", decoded.Status.Reason, SectorPredictionReasonFlagDisabled)
	}
	if decoded.Status.Persisted {
		t.Error("persisted must be false (I6: the ledger has no sector column)")
	}
	if decoded.Status.PersistenceReason != SectorPredictionPersistenceReason {
		t.Errorf("persistence_reason = %q, want %q", decoded.Status.PersistenceReason, SectorPredictionPersistenceReason)
	}
	if decoded.Status.CycleProviderWired {
		t.Error("cycle_provider_wired must be false")
	}
	if decoded.SectorPredictions == nil {
		t.Error("sector_predictions must serialize as [] rather than null")
	}
}
