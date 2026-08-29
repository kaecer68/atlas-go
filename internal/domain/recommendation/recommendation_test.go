package recommendation

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// ---- RecommendationOutcome.Validate ----

func TestRecommendationOutcomeValidate(t *testing.T) {
	valid := RecommendationOutcome{
		AgentID:    "agent-1",
		Symbol:     "2330.TW",
		Side:       shared.SideBuy,
		Window:     "2026-06-10",
		Conviction: 3,
	}
	tests := []struct {
		name    string
		mutate  func(o *RecommendationOutcome)
		wantErr bool
		substr  string
	}{
		{"valid", func(o *RecommendationOutcome) {}, false, ""},
		{"missing AgentID", func(o *RecommendationOutcome) { o.AgentID = "" }, true, "AgentID"},
		{"missing Symbol", func(o *RecommendationOutcome) { o.Symbol = "" }, true, "Symbol"},
		{"missing Side", func(o *RecommendationOutcome) { o.Side = "" }, true, "Side"},
		{"missing Window", func(o *RecommendationOutcome) { o.Window = "" }, true, "Window"},
		{"conviction zero", func(o *RecommendationOutcome) { o.Conviction = 0 }, true, "Conviction"},
		{"conviction negative", func(o *RecommendationOutcome) { o.Conviction = -1 }, true, "Conviction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := valid
			tt.mutate(&o)
			err := o.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && tt.substr != "" && err != nil {
				if !strings.Contains(err.Error(), tt.substr) {
					t.Fatalf("error %q should contain %q", err.Error(), tt.substr)
				}
			}
		})
	}
}

// ---- HumanIntervention.IsExpired ----

func TestHumanInterventionIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"zero time never expires", time.Time{}, false},
		{"past time expired", time.Now().Add(-1 * time.Hour), true},
		{"future time not expired", time.Now().Add(24 * time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := HumanIntervention{ExpiresAt: tt.expiresAt}
			got := h.IsExpired()
			if got != tt.want {
				t.Fatalf("IsExpired() = %v, want %v (expiresAt=%v, now=%v)", got, tt.want, tt.expiresAt, time.Now())
			}
		})
	}
}

// ---- PromptControl Extract / Render ----

func TestExtractPromptControl(t *testing.T) {
	prompt := `# Semiconductor Desk

<!-- control_block -->
{
  "volume_floor": 1500000,
  "volume_downgrade": 25,
  "close_strength_boost": 10
}
<!-- /control_block -->
`
	ctrl, ok := ExtractPromptControl(prompt)
	if !ok {
		t.Fatal("expected control block to be found")
	}
	if ctrl.VolumeFloor != 1500000 {
		t.Fatalf("volume_floor expected 1500000, got %d", ctrl.VolumeFloor)
	}
	if ctrl.VolumeDowngrade != 25 {
		t.Fatalf("volume_downgrade expected 25, got %d", ctrl.VolumeDowngrade)
	}
	if ctrl.CloseStrengthBoost != 10 {
		t.Fatalf("close_strength_boost expected 10, got %d", ctrl.CloseStrengthBoost)
	}
}

func TestExtractPromptControlMissing(t *testing.T) {
	_, ok := ExtractPromptControl("# Plain prompt without control block")
	if ok {
		t.Fatal("expected no control block")
	}
}

func TestExtractPromptControlEmptyString(t *testing.T) {
	_, ok := ExtractPromptControl("")
	if ok {
		t.Fatal("expected no control block from empty prompt")
	}
}

func TestExtractPromptControlInvalidJSON(t *testing.T) {
	prompt := `<!-- control_block -->
{not valid json}
<!-- /control_block -->`
	_, ok := ExtractPromptControl(prompt)
	if ok {
		t.Fatal("expected no control block from invalid JSON")
	}
}

func TestExtractPromptControlAllFields(t *testing.T) {
	prompt := `<!-- control_block -->
{
  "volume_floor": 500000,
  "volume_downgrade": 10,
  "close_strength_boost": 5,
  "hard_reject_volume": 100000,
  "price_condition": "above_ma5",
  "conviction_floor": 2,
  "volume_boost": 15,
  "require_trend": true,
  "neutral_penalty_reduction": 3
}
<!-- /control_block -->`
	ctrl, ok := ExtractPromptControl(prompt)
	if !ok {
		t.Fatal("expected control block to be found")
	}
	if ctrl.VolumeFloor != 500000 {
		t.Fatalf("VolumeFloor = %d, want 500000", ctrl.VolumeFloor)
	}
	if ctrl.VolumeDowngrade != 10 {
		t.Fatalf("VolumeDowngrade = %d, want 10", ctrl.VolumeDowngrade)
	}
	if ctrl.CloseStrengthBoost != 5 {
		t.Fatalf("CloseStrengthBoost = %d, want 5", ctrl.CloseStrengthBoost)
	}
	if ctrl.HardRejectVolume != 100000 {
		t.Fatalf("HardRejectVolume = %d, want 100000", ctrl.HardRejectVolume)
	}
	if ctrl.PriceCondition != "above_ma5" {
		t.Fatalf("PriceCondition = %q, want above_ma5", ctrl.PriceCondition)
	}
	if ctrl.ConvictionFloor != 2 {
		t.Fatalf("ConvictionFloor = %d, want 2", ctrl.ConvictionFloor)
	}
	if ctrl.VolumeBoost != 15 {
		t.Fatalf("VolumeBoost = %d, want 15", ctrl.VolumeBoost)
	}
	if !ctrl.RequireTrend {
		t.Fatal("RequireTrend should be true")
	}
	if ctrl.NeutralPenaltyReduction != 3 {
		t.Fatalf("NeutralPenaltyReduction = %d, want 3", ctrl.NeutralPenaltyReduction)
	}
}

func TestExtractPromptControlPartialBlock(t *testing.T) {
	// Only opening tag, no closing tag.
	prompt := `<!-- control_block -->
{"volume_floor": 42}
`
	_, ok := ExtractPromptControl(prompt)
	if ok {
		t.Fatal("expected no control block from unmatched tags")
	}
}

func TestRenderPromptControl(t *testing.T) {
	ctrl := PromptControl{VolumeFloor: 2000000, VolumeDowngrade: 30}
	rendered := RenderPromptControl(ctrl)
	if !strings.Contains(rendered, "<!-- control_block -->") {
		t.Fatal("missing control_block open tag")
	}
	if !strings.Contains(rendered, "<!-- /control_block -->") {
		t.Fatal("missing control_block close tag")
	}
	if !strings.Contains(rendered, "2000000") {
		t.Fatal("missing volume_floor value")
	}
}

func TestRenderPromptControlEmpty(t *testing.T) {
	rendered := RenderPromptControl(PromptControl{})
	if !strings.Contains(rendered, "<!-- control_block -->") {
		t.Fatal("missing control_block open tag")
	}
	if !strings.Contains(rendered, "<!-- /control_block -->") {
		t.Fatal("missing control_block close tag")
	}
}

func TestPromptControlRoundTrip(t *testing.T) {
	original := PromptControl{
		VolumeFloor:             750000,
		VolumeDowngrade:         15,
		CloseStrengthBoost:      5,
		HardRejectVolume:        50000,
		PriceCondition:          "above_ma20",
		ConvictionFloor:         2,
		VolumeBoost:             10,
		RequireTrend:            false,
		NeutralPenaltyReduction: 0,
	}
	rendered := RenderPromptControl(original)
	extracted, ok := ExtractPromptControl(rendered)
	if !ok {
		t.Fatal("round-trip: failed to extract rendered control block")
	}
	if extracted != original {
		t.Fatalf("round-trip mismatch:\n  original: %+v\n  extracted: %+v", original, extracted)
	}
}

func TestPromptControlRoundTripEmpty(t *testing.T) {
	original := PromptControl{}
	rendered := RenderPromptControl(original)
	extracted, ok := ExtractPromptControl(rendered)
	if !ok {
		t.Fatal("round-trip empty: failed to extract rendered control block")
	}
	if extracted != original {
		t.Fatalf("round-trip empty mismatch:\n  original: %+v\n  extracted: %+v", original, extracted)
	}
}

// ---- ScreeningCriteria.HasFilters ----

//go:fix inline
func ptrFloat64(v float64) *float64 { return new(v) }

//go:fix inline
func ptrInt64(v int64) *int64 { return new(v) }

func TestScreeningCriteriaHasFilters(t *testing.T) {
	tests := []struct {
		name string
		sc   ScreeningCriteria
		want bool
	}{
		{"empty all nil", ScreeningCriteria{}, false},
		{"PE range set", ScreeningCriteria{PE: &RangeFilter{Min: ptrFloat64(10)}}, true},
		{"PB range set", ScreeningCriteria{PB: &RangeFilter{Max: ptrFloat64(2)}}, true},
		{"dividend yield set", ScreeningCriteria{DividendYield: &RangeFilter{Min: new(0.03)}}, true},
		{"momentum 20d set", ScreeningCriteria{Momentum20Day: &RangeFilter{Min: new(-0.05), Max: new(0.15)}}, true},
		{"volatility 20d set", ScreeningCriteria{Volatility20Day: &RangeFilter{Max: new(0.35)}}, true},
		{"volume intraday set", ScreeningCriteria{VolumeIntraday: &MinFilter{Min: ptrInt64(1000000)}}, true},
		{"min factor score set", ScreeningCriteria{MinTotalFactorScore: new(0.5)}, true},
		{"required factors non-empty", ScreeningCriteria{RequiredFactors: []string{"momentum", "value"}}, true},
		{
			"multiple criteria set",
			ScreeningCriteria{
				PE:              &RangeFilter{Min: ptrFloat64(8), Max: ptrFloat64(20)},
				VolumeIntraday:  &MinFilter{Min: ptrInt64(500000)},
				RequiredFactors: []string{"momentum"},
			},
			true,
		},
		{
			"empty range filter still counts",
			ScreeningCriteria{PE: &RangeFilter{}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sc.HasFilters()
			if got != tt.want {
				t.Fatalf("HasFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}
