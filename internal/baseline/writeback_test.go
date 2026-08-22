package baseline

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/charter"
)

// testDelta builds a CharterDelta with an explicit verdict (the verdict
// classification itself is covered in internal/charter/delta_test.go).
func testDelta(step int, switchName string, verdict charter.Verdict, p, meanDiff float64) charter.CharterDelta {
	d := charter.CharterDelta{
		Step:   step,
		Switch: switchName,
		Window: charter.DeltaWindow{Start: "2026-01-01", End: "2026-08-21", Days: 166},
		MetricDiffs: charter.MetricDiffs{
			Sharpe: 0.5785, MaxDD: 0.1662, WinRate: 0.09, TotalReturn: -277.0, RawRecs: -7451,
		},
		PairedT: charter.PairedT{T: -0.9836, DF: 164, P: p, MeanDiff: meanDiff, Significant: p < 0.05},
		BCaSharpe95CI: charter.BCaSharpeCI{
			Observed: 0.5785, CI95Low: -0.7116, CI95High: 2.0688, Resamples: 10000,
		},
		Verdict: verdict,
	}
	d.Evidence = d.EvidenceString()
	return d
}

// saveTempPolicy writes a policy to a temp file and returns its path.
func saveTempPolicy(t *testing.T, p Policy) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "baseline_policy.json")
	if err := SaveWithLock(path, p); err != nil {
		t.Fatalf("save temp policy: %v", err)
	}
	return path
}

// TestWritebackCharter_RoundTrip — write deltas to a temp policy, reload it,
// and verify promotions/version/evidence are correct (the C4 round trip:
// write → Load → verify).
func TestWritebackCharter_RoundTrip(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())
	manager := NewManager(path)

	deltas := []charter.CharterDelta{
		testDelta(1, "PeriodOnly", charter.VerdictDegenerate, 0.2368, 0.030681),
		testDelta(2, "StrategyFilter", charter.VerdictDirectionalWatch, 0.3268, -0.025568),
		testDelta(3, "MacroFlow", charter.VerdictInert, 0.7443, 0.0),
	}

	next, err := manager.WritebackCharter(deltas, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("writeback: %v", err)
	}
	if next.Version != 4 {
		t.Errorf("version = %d, want 4 (1 base + 3 records)", next.Version)
	}
	if len(next.Promotions) != 3 {
		t.Fatalf("promotions = %d, want 3", len(next.Promotions))
	}

	// Reload from disk — the written file must round-trip.
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Version != 4 {
		t.Errorf("reloaded version = %d, want 4", reloaded.Version)
	}
	if len(reloaded.Promotions) != 3 {
		t.Fatalf("reloaded promotions = %d, want 3", len(reloaded.Promotions))
	}

	checks := []struct {
		idx          int
		experiment   string
		skill        string
		mutation     string
		status       string
		versionAfter int
	}{
		{0, "charter-ab-step-1", "PeriodOnly", MutationTypeCharterDelta, StatusRecorded, 2},
		{1, "charter-ab-step-2", "StrategyFilter", MutationTypeCharterDelta, StatusWatch, 3},
		{2, "charter-ab-step-3", "MacroFlow", MutationTypeCharterDelta, StatusRecorded, 4},
	}
	for _, c := range checks {
		p := reloaded.Promotions[c.idx]
		if p.ExperimentID != c.experiment || p.TargetSkill != c.skill || p.MutationType != c.mutation ||
			p.Status != c.status || p.VersionAfter != c.versionAfter {
			t.Errorf("promotions[%d] = {id:%s skill:%s mutation:%s status:%s v:%d}, want {%s %s %s %s %d}",
				c.idx, p.ExperimentID, p.TargetSkill, p.MutationType, p.Status, p.VersionAfter,
				c.experiment, c.skill, c.mutation, c.status, c.versionAfter)
		}
		if p.ConstraintsSnapshot != nil {
			t.Errorf("promotions[%d]: watch/finding must not carry a constraints snapshot", c.idx)
		}
	}

	// Evidence must carry p value + window (full evidence recorded).
	ev := reloaded.Promotions[1].PromptSnapshot
	if !strings.Contains(ev, "p=0.3268") || !strings.Contains(ev, "2026-01-01") {
		t.Errorf("watch evidence missing p/window: %s", ev)
	}
	var parsed charter.CharterDelta
	if err := json.Unmarshal([]byte(ev), &parsed); err != nil {
		t.Errorf("evidence not a CharterDelta JSON: %v", err)
	}
	if parsed.Verdict != charter.VerdictDirectionalWatch {
		t.Errorf("evidence verdict = %s", parsed.Verdict)
	}

	// directional_watch must NOT override runtime values.
	if reloaded.Constraints.ReserveCashFraction != DefaultPolicy().Constraints.ReserveCashFraction {
		t.Error("directional_watch must not override constraints")
	}
	if reloaded.Constraints.MinRecommendationConviction != DefaultPolicy().Constraints.MinRecommendationConviction {
		t.Error("directional_watch must not override conviction floor")
	}
}

// TestWritebackCharter_Idempotent — re-running the writeback must not
// duplicate records.
func TestWritebackCharter_Idempotent(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())
	manager := NewManager(path)
	deltas := []charter.CharterDelta{
		testDelta(2, "StrategyFilter", charter.VerdictDirectionalWatch, 0.3268, -0.025568),
	}

	first, err := manager.WritebackCharter(deltas, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("first writeback: %v", err)
	}
	second, err := manager.WritebackCharter(deltas, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("second writeback: %v", err)
	}
	if second.Version != first.Version {
		t.Errorf("version changed on re-run: %d → %d", first.Version, second.Version)
	}
	if len(second.Promotions) != len(first.Promotions) {
		t.Errorf("promotions changed on re-run: %d → %d", len(first.Promotions), len(second.Promotions))
	}
	if second.Promotions[0].Status != StatusWatch {
		t.Errorf("record status = %s, want watch", second.Promotions[0].Status)
	}
}

// TestWritebackCharter_NoDeltaNoChange — an empty delta set must leave the
// policy untouched.
func TestWritebackCharter_NoDeltaNoChange(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())
	before, err := Load(path)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}

	manager := NewManager(path)
	next, err := manager.WritebackCharter(nil, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("writeback empty: %v", err)
	}
	if next.Version != before.Version {
		t.Errorf("version changed with no deltas: %d → %d", before.Version, next.Version)
	}
	if len(next.Promotions) != len(before.Promotions) {
		t.Errorf("promotions changed with no deltas")
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Version != before.Version || len(reloaded.Promotions) != len(before.Promotions) {
		t.Error("file changed with no deltas")
	}
}

// TestWritebackCharter_SignificantEnable_ConvictionFloor — a p<0.05
// ConvictionFloor delta writes the constraint (base floor + 20), updates the
// execution policy, and records a charter_constraint promotion with a
// constraints snapshot.
func TestWritebackCharter_SignificantEnable_ConvictionFloor(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())
	base := DefaultPolicy()

	delta := testDelta(5, "ConvictionFloor", charter.VerdictSignificantEnable, 0.0042, 0.012)
	manager := NewManager(path)
	next, err := manager.WritebackCharter([]charter.CharterDelta{delta}, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("writeback: %v", err)
	}

	wantFloor := base.Constraints.MinRecommendationConviction + 20 // charter black_swan delta
	if next.Constraints.MinRecommendationConviction != wantFloor {
		t.Errorf("MinRecommendationConviction = %d, want %d", next.Constraints.MinRecommendationConviction, wantFloor)
	}
	if next.ExecutionPolicy.ConvictionFloor != wantFloor {
		t.Errorf("ExecutionPolicy.ConvictionFloor = %d, want %d", next.ExecutionPolicy.ConvictionFloor, wantFloor)
	}

	rec := next.Promotions[0]
	if rec.MutationType != MutationTypeCharterConstraint || rec.Status != StatusPromoted {
		t.Errorf("record = %s/%s, want charter_constraint/promoted", rec.MutationType, rec.Status)
	}
	if rec.ConstraintsSnapshot == nil {
		t.Fatal("promoted record must carry a constraints snapshot")
	}
	if rec.ConstraintsSnapshot.MinRecommendationConviction != wantFloor {
		t.Errorf("snapshot conviction = %d, want %d", rec.ConstraintsSnapshot.MinRecommendationConviction, wantFloor)
	}

	// Round-trip: the promoted constraint must survive Load.
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Constraints.MinRecommendationConviction != wantFloor {
		t.Errorf("reloaded conviction = %d, want %d", reloaded.Constraints.MinRecommendationConviction, wantFloor)
	}
	if reloaded.Version != 2 {
		t.Errorf("reloaded version = %d, want 2", reloaded.Version)
	}
}

// TestWritebackCharter_SignificantEnable_CashReserve — a p<0.05 CashReserve
// delta raises the static reserve floor from the methodology rules.
func TestWritebackCharter_SignificantEnable_CashReserve(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())

	delta := testDelta(4, "CashReserve", charter.VerdictSignificantEnable, 0.01, 0.005)
	manager := NewManager(path)
	next, err := manager.WritebackCharter([]charter.CharterDelta{delta}, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("writeback: %v", err)
	}
	if next.Constraints.ReserveCashFraction <= DefaultPolicy().Constraints.ReserveCashFraction {
		t.Errorf("ReserveCashFraction = %.3f, want > baseline %.3f (strictest period reserve)",
			next.Constraints.ReserveCashFraction, DefaultPolicy().Constraints.ReserveCashFraction)
	}
	if next.Constraints.ReserveCashFraction > 1.0 {
		t.Errorf("ReserveCashFraction = %.3f, want ≤ 1.0", next.Constraints.ReserveCashFraction)
	}
	rec := next.Promotions[0]
	if rec.Status != StatusPromoted || rec.ConstraintsSnapshot == nil {
		t.Errorf("record = %s snapshot=%v, want promoted with snapshot", rec.Status, rec.ConstraintsSnapshot != nil)
	}
}

// TestWritebackCharter_MissingFile — writeback against a nonexistent policy
// must fail (never silently create a policy from defaults).
func TestWritebackCharter_MissingFile(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "no-such-policy.json"))
	if _, err := manager.WritebackCharter(
		[]charter.CharterDelta{testDelta(1, "PeriodOnly", charter.VerdictInert, 0.9, 0)},
		"/tmp/charter-ab"); err == nil {
		t.Fatal("expected error for missing policy file")
	}
}

// TestWritebackCharter_MixedBatch — only not-yet-recorded deltas are
// appended; already-recorded ones are skipped.
func TestWritebackCharter_MixedBatch(t *testing.T) {
	path := saveTempPolicy(t, DefaultPolicy())
	manager := NewManager(path)

	first := []charter.CharterDelta{testDelta(2, "StrategyFilter", charter.VerdictDirectionalWatch, 0.3268, -0.025568)}
	if _, err := manager.WritebackCharter(first, "/tmp/charter-ab"); err != nil {
		t.Fatalf("first writeback: %v", err)
	}

	// Second batch: step-2 already recorded (skip), step-1 new (append).
	second := []charter.CharterDelta{
		testDelta(1, "PeriodOnly", charter.VerdictDegenerate, 0.2368, 0.03),
		testDelta(2, "StrategyFilter", charter.VerdictDirectionalWatch, 0.3268, -0.025568),
	}
	next, err := manager.WritebackCharter(second, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("second writeback: %v", err)
	}
	if len(next.Promotions) != 2 {
		t.Errorf("promotions = %d, want 2 (step-2 kept, step-1 added)", len(next.Promotions))
	}
	// Existing records keep their position; the new step-1 is appended.
	if next.Promotions[0].ExperimentID != "charter-ab-step-2" || next.Promotions[1].ExperimentID != "charter-ab-step-1" {
		t.Errorf("promotion order/ids wrong: %s, %s", next.Promotions[0].ExperimentID, next.Promotions[1].ExperimentID)
	}
	if next.Promotions[1].VersionAfter != 3 {
		t.Errorf("new record version_after = %d, want 3", next.Promotions[1].VersionAfter)
	}
	if next.Version != 3 {
		t.Errorf("version = %d, want 3", next.Version)
	}
}

// TestWritebackCharter_PreservesExistingHistory — charter records must not
// disturb existing promotions/revert history.
func TestWritebackCharter_PreservesExistingHistory(t *testing.T) {
	p := DefaultPolicy()
	p.Version = 19
	p.Promotions = []PromotionRecord{
		{ExperimentID: "exec-etf-rotation-01-1775356834", MutationType: "prompt_tightening", Status: "accepted", VersionAfter: 19},
	}
	p.RevertHistory = []RevertRecord{{FromVersion: 19, ToVersion: 18, Reason: "manual"}}
	path := saveTempPolicy(t, p)

	manager := NewManager(path)
	deltas := []charter.CharterDelta{testDelta(2, "StrategyFilter", charter.VerdictDirectionalWatch, 0.3268, -0.025568)}
	next, err := manager.WritebackCharter(deltas, "/tmp/charter-ab")
	if err != nil {
		t.Fatalf("writeback: %v", err)
	}
	if next.Version != 20 {
		t.Errorf("version = %d, want 20", next.Version)
	}
	if len(next.Promotions) != 2 {
		t.Errorf("promotions = %d, want 2", len(next.Promotions))
	}
	if next.Promotions[0].ExperimentID != "exec-etf-rotation-01-1775356834" {
		t.Errorf("existing promotion disturbed: %s", next.Promotions[0].ExperimentID)
	}
	if len(next.RevertHistory) != 1 {
		t.Errorf("revert history disturbed: %d", len(next.RevertHistory))
	}
	if next.LastUpdatedAt.IsZero() {
		t.Error("last_updated_at must be set after writeback")
	}
}
