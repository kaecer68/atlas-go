package dailyreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fullReport has period + risk warning so all three claim types are created.
func fullReport(date string) *Report {
	r := testReport(date)
	r.Risk.Warning = "警戒：外資連續三日淨賣超"
	r.Risk.StressIndex = 0.8
	return r
}

func TestTradingDaysAfter(t *testing.T) {
	tests := []struct {
		date string
		n    int
		want string
	}{
		// 2026-08-10 Mon → +5 trading days = 2026-08-17 Mon
		{"2026-08-10", 5, "2026-08-17"},
		// 2026-02-13 Fri → 5 trading days: spring closure 2/12-2/20 all
		// non-trading (settlement + 除夕~初五 + 補假), reopening 2/23 Mon;
		// +5 trading days = 2026-02-27
		{"2026-02-13", 5, "2026-02-27"},
		// A report on a Friday: next trading day is Monday.
		{"2026-08-14", 1, "2026-08-17"},
	}
	for _, tt := range tests {
		got, err := tradingDaysAfter(tt.date, tt.n)
		if err != nil {
			t.Fatalf("tradingDaysAfter(%s, %d): %v", tt.date, tt.n, err)
		}
		if got != tt.want {
			t.Errorf("tradingDaysAfter(%s, %d) = %s, want %s", tt.date, tt.n, got, tt.want)
		}
	}
	if _, err := tradingDaysAfter("not-a-date", 5); err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestTracker_CreateClaimsFromReport(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	trk.SetNow(func() time.Time { return time.Date(2026, 8, 10, 14, 5, 0, 0, time.UTC) })

	r := fullReport("2026-08-10")
	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("CreateClaimsFromReport: %v", err)
	}
	claims := trk.ListClaims()
	if len(claims) != 3 {
		t.Fatalf("claims = %d, want 3", len(claims))
	}
	byType := map[ClaimType]TrackedClaim{}
	for _, c := range claims {
		byType[c.ClaimType] = c
	}
	if _, ok := byType[ClaimStrategyRecommendation]; !ok {
		t.Error("missing strategy claim")
	}
	if _, ok := byType[ClaimPeriodCall]; !ok {
		t.Error("missing period claim")
	}
	if _, ok := byType[ClaimRiskWarning]; !ok {
		t.Error("missing risk claim")
	}
	for _, c := range claims {
		if c.Status != ClaimTracking {
			t.Errorf("claim %s status = %s, want tracking", c.ID, c.Status)
		}
		if c.VerifyAfter != "2026-08-17" {
			t.Errorf("claim %s verify_after = %s, want 2026-08-17", c.ID, c.VerifyAfter)
		}
		if c.Symbol != "TAIEX" {
			t.Errorf("claim %s symbol = %s, want TAIEX", c.ID, c.Symbol)
		}
	}
	// Strategy claim text must embed the direction marker for verification.
	if !strings.Contains(byType[ClaimStrategyRecommendation].ClaimText, "方向:偏多") {
		t.Errorf("strategy claim text = %q", byType[ClaimStrategyRecommendation].ClaimText)
	}
}

func TestTracker_CreateClaims_Idempotent(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	r := fullReport("2026-08-10")

	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("second create: %v", err)
	}
	if got := len(trk.ListClaims()); got != 3 {
		t.Errorf("claims after regenerate = %d, want 3 (idempotent)", got)
	}
	// A later regenerate of the same report must not duplicate either.
	r2 := fullReport("2026-08-10")
	r2.Risk.Warning = "different warning"
	if err := trk.CreateClaimsFromReport(r2); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if got := len(trk.ListClaims()); got != 3 {
		t.Errorf("claims after re-create = %d, want 3", got)
	}
}

func TestTracker_CreateClaims_MinimalReport(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	r := testReport("2026-08-10")
	r.Period = nil
	r.Risk.Warning = ""
	r.Risk.DrawdownAlert = false
	r.Risk.StressIndex = 0.3

	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("CreateClaimsFromReport: %v", err)
	}
	claims := trk.ListClaims()
	if len(claims) != 1 {
		t.Fatalf("minimal report claims = %d, want 1 (strategy only)", len(claims))
	}
	if claims[0].ClaimType != ClaimStrategyRecommendation {
		t.Errorf("claim type = %s", claims[0].ClaimType)
	}
}

// stubVerifier is a scriptable ClaimVerifier for tracker lifecycle tests.
type stubVerifier struct {
	outcome string
	ok      bool
}

func (s *stubVerifier) Verify(*TrackedClaim) (string, bool) { return s.outcome, s.ok }

func TestTracker_VerifyDueClaims_Verified(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	trk.SetVerifier(&stubVerifier{outcome: "hit:+3.00%", ok: true})
	r := fullReport("2026-08-10")
	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Before the window: nothing changes.
	n, err := trk.VerifyDueClaims(time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("verify early: %v", err)
	}
	if n != 0 {
		t.Errorf("early verify changed %d claims, want 0", n)
	}

	// On/after the window: all three claims verified.
	n, err = trk.VerifyDueClaims(time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("verify due: %v", err)
	}
	if n != 3 {
		t.Errorf("due verify changed %d claims, want 3", n)
	}
	for _, c := range trk.ListClaims() {
		if c.Status != ClaimVerified {
			t.Errorf("claim %s status = %s, want verified", c.ID, c.Status)
		}
		if c.Outcome != "hit:+3.00%" {
			t.Errorf("claim %s outcome = %q", c.ID, c.Outcome)
		}
		if c.VerifiedAt.IsZero() {
			t.Errorf("claim %s verified_at not set", c.ID)
		}
	}

	// Idempotent: a second run changes nothing.
	n, err = trk.VerifyDueClaims(time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("verify again: %v", err)
	}
	if n != 0 {
		t.Errorf("second verify changed %d claims, want 0", n)
	}
}

func TestTracker_VerifyDueClaims_ExpiredOnNoData(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	trk.SetVerifier(&stubVerifier{outcome: "", ok: false})
	r := fullReport("2026-08-10")
	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("create: %v", err)
	}

	n, err := trk.VerifyDueClaims(time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if n != 3 {
		t.Errorf("changed = %d, want 3", n)
	}
	for _, c := range trk.ListClaims() {
		if c.Status != ClaimExpired {
			t.Errorf("claim %s status = %s, want expired", c.ID, c.Status)
		}
		if c.Outcome != "unverified:no_data" {
			t.Errorf("claim %s outcome = %q", c.ID, c.Outcome)
		}
	}
}

func TestTracker_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	r := fullReport("2026-08-10")
	if err := trk.CreateClaimsFromReport(r); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A fresh tracker over the same ledger dir sees the same claims.
	trk2 := NewTracker(dir, filepath.Join(dir, "replay"))
	claims := trk2.ListClaims()
	if len(claims) != 3 {
		t.Fatalf("reloaded claims = %d, want 3", len(claims))
	}
	if got := trk2.GetClaim("2026-08-10-strategy_recommendation"); got == nil {
		t.Error("GetClaim missed persisted strategy claim")
	}
	if got := trk2.GetClaim("nope"); got != nil {
		t.Error("GetClaim should return nil for unknown id")
	}
}

func TestMarketClaimVerifier_PricePath(t *testing.T) {
	dir := t.TempDir()
	replay := filepath.Join(dir, "replay")
	if err := os.MkdirAll(replay, 0o755); err != nil {
		t.Fatal(err)
	}
	// 2026-08-10 (Mon) base 100 → 2026-08-17 (Mon) 103 → +3% (long hit).
	indexData := `{"date":"2026-08-10","index":100}
{"date":"2026-08-11","index":101}
{"date":"2026-08-17","index":103}
`
	if err := os.WriteFile(filepath.Join(replay, "twse_index_6months.jsonl"), []byte(indexData), 0o644); err != nil {
		t.Fatal(err)
	}

	v := NewMarketClaimVerifier(dir, replay)
	claim := TrackedClaim{
		ReportDate:  "2026-08-10",
		VerifyAfter: "2026-08-17",
		ClaimType:   ClaimStrategyRecommendation,
		ClaimText:   "策略:all_weather 方向:偏多 進場:x",
		Symbol:      "TAIEX",
	}
	outcome, ok := v.Verify(&claim)
	if !ok {
		t.Fatal("expected price verification to succeed")
	}
	if outcome != "hit:+3.00%" {
		t.Errorf("outcome = %q, want hit:+3.00%%", outcome)
	}

	// 偏空 direction on the same rising market → miss.
	claim.ClaimText = "策略:x 方向:偏空 進場:x"
	outcome, ok = v.Verify(&claim)
	if !ok {
		t.Fatal("expected ok")
	}
	if outcome != "miss:+3.00%" {
		t.Errorf("short outcome = %q, want miss:+3.00%%", outcome)
	}

	// Falling market + risk warning → confirmed.
	falling := `{"date":"2026-08-10","index":100}
{"date":"2026-08-17","index":97}
`
	if err := os.WriteFile(filepath.Join(replay, "twse_index_6months.jsonl"), []byte(falling), 0o644); err != nil {
		t.Fatal(err)
	}
	riskClaim := TrackedClaim{
		ReportDate:  "2026-08-10",
		VerifyAfter: "2026-08-17",
		ClaimType:   ClaimRiskWarning,
		ClaimText:   "風險等級:high 壓力指數:0.80 警告:警戒",
		Symbol:      "TAIEX",
	}
	outcome, ok = v.Verify(&riskClaim)
	if !ok {
		t.Fatal("expected ok for risk claim")
	}
	if outcome != "confirmed:-3.00%" {
		t.Errorf("risk outcome = %q, want confirmed:-3.00%%", outcome)
	}
}

func TestMarketClaimVerifier_NoDataExpires(t *testing.T) {
	dir := t.TempDir()
	// No replay files at all.
	v := NewMarketClaimVerifier(dir, filepath.Join(dir, "replay"))
	claim := TrackedClaim{
		ReportDate:  "2026-08-10",
		VerifyAfter: "2026-08-17",
		ClaimType:   ClaimStrategyRecommendation,
		ClaimText:   "策略:all_weather 方向:偏多",
		Symbol:      "TAIEX",
	}
	if _, ok := v.Verify(&claim); ok {
		t.Fatal("expected ok=false with no price data")
	}
}

func TestMarketClaimVerifier_OutcomePath(t *testing.T) {
	dir := t.TempDir()
	// Symbol-level claim verified via recommendation outcomes.
	outcomes := `{"symbol":"2330.TW","forward_return":0.02,"hit":true,"recorded_at":"2026-08-11T08:00:00Z"}
{"symbol":"2330.TW","forward_return":-0.01,"hit":false,"recorded_at":"2026-08-12T08:00:00Z"}
{"symbol":"9999.TW","forward_return":9.9,"hit":true,"recorded_at":"2026-08-11T08:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(outcomes), 0o644); err != nil {
		t.Fatal(err)
	}
	v := NewMarketClaimVerifier(dir, filepath.Join(dir, "replay"))
	claim := TrackedClaim{
		ReportDate:  "2026-08-10",
		VerifyAfter: "2026-08-17",
		ClaimType:   ClaimStrategyRecommendation,
		ClaimText:   "策略:x 方向:偏多",
		Symbol:      "2330.TW",
	}
	outcome, ok := v.Verify(&claim)
	if !ok {
		t.Fatal("expected outcome verification to succeed")
	}
	// 1/2 hit → hit verdict (hit ratio >= 0.5).
	if outcome != "hit:1/2 outcomes avg_fwd=+0.50%" {
		t.Errorf("outcome = %q", outcome)
	}
}

func TestPeriodDirection(t *testing.T) {
	tests := []struct {
		period, name string
		want         string
	}{
		{"bull", "上升", "偏多"},
		{"turnaround_up", "轉折開高", "偏多"},
		{"downturn", "低迷", "偏空"},
		{"black_swan", "黑天鵝", "偏空"},
		{"consolidation", "盤整", "中性"},
		{"plateau", "高原", "中性"},
	}
	for _, tt := range tests {
		if got := periodDirection(tt.period, tt.name); got != tt.want {
			t.Errorf("periodDirection(%s, %s) = %s, want %s", tt.period, tt.name, got, tt.want)
		}
	}
}
