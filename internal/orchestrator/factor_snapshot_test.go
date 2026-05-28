package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// mockFactorQuery provides deterministic factor scores for testing adjustment helpers.
type mockFactorQuery struct {
	scores map[string]map[portfolio.FactorType]float64
}

func (m mockFactorQuery) GetScore(symbol string, factor portfolio.FactorType) (float64, bool) {
	if m.scores == nil {
		return 0, false
	}
	s, ok := m.scores[symbol]
	if !ok {
		return 0, false
	}
	v, ok := s[factor]
	return v, ok
}

// ── NewFactorSnapshot tests ─────────────────────────────────

func TestNewFactorSnapshot_NilEngine(t *testing.T) {
	fs := NewFactorSnapshot(nil, nil)
	if fs == nil {
		t.Fatal("expected non-nil FactorSnapshot")
	}
	score, ok := fs.GetScore("any", portfolio.FactorMomentum)
	if ok || score != 0 {
		t.Errorf("expected (0, false) for nil engine, got (%.2f, %v)", score, ok)
	}
}

func TestNewFactorSnapshot_EmptyQuotes(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	fs := NewFactorSnapshot(nil, fe)
	if fs == nil {
		t.Fatal("expected non-nil FactorSnapshot with empty quotes")
	}
	score, ok := fs.GetScore("any", portfolio.FactorMomentum)
	if ok {
		t.Errorf("expected (0, false) for missing symbol, got (%.2f, %v)", score, ok)
	}
}

func TestNewFactorSnapshot_WithQuotes(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Open: 100, High: 105, Low: 99, Last: 103, Volume: 1000000, IsTradable: true},
		"B": {Symbol: "B", Open: 50, High: 51, Low: 49, Last: 50, Volume: 500000, IsTradable: true},
	}
	fs := NewFactorSnapshot(quotes, fe)
	if fs == nil {
		t.Fatal("expected non-nil FactorSnapshot")
	}
	// Momentum should be calculable (even if fallback) — just ensure it doesn't crash
	_, ok := fs.GetScore("A", portfolio.FactorMomentum)
	if !ok {
		t.Error("expected momentum score for symbol A")
	}
	_, ok = fs.GetScore("A", portfolio.FactorValue)
	if !ok {
		t.Error("expected value score for symbol A")
	}
	_, ok = fs.GetScore("A", portfolio.FactorQuality)
	if !ok {
		t.Error("expected quality score for symbol A")
	}
	_, ok = fs.GetScore("A", portfolio.FactorLiquidity)
	if !ok {
		t.Error("expected liquidity score for symbol A")
	}
}

// ── GetScore edge cases ─────────────────────────────────────

func TestGetScore_NilReceiver(t *testing.T) {
	var fs *FactorSnapshot
	_, ok := fs.GetScore("any", portfolio.FactorMomentum)
	if ok {
		t.Error("expected false for nil receiver")
	}
}

func TestGetScore_MissingSymbol(t *testing.T) {
	fs := &FactorSnapshot{scores: map[string]map[portfolio.FactorType]float64{}}
	_, ok := fs.GetScore("missing", portfolio.FactorMomentum)
	if ok {
		t.Error("expected false for missing symbol")
	}
}

func TestGetScore_MissingFactor(t *testing.T) {
	fs := &FactorSnapshot{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorMomentum: 0.5},
	}}
	_, ok := fs.GetScore("A", portfolio.FactorPreciousMetals)
	if ok {
		t.Error("expected false for missing factor")
	}
}

// ── addMomentumAdjustment tests ─────────────────────────────

func TestAddMomentumAdjustment_High(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorMomentum: 0.6},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, "A", fc)

	conv, cb := b.build()
	if cb.Final < 100+fc.momHighD {
		t.Errorf("expected delta >= %d for momentum>high, got final=%d", fc.momHighD, conv)
	}
}

func TestAddMomentumAdjustment_Moderate(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorMomentum: 0.25},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, "A", fc)

	conv, cb := b.build()
	finalBase := 100 + fc.momModD
	if cb.Final < finalBase-1 || cb.Final > finalBase+1 {
		t.Errorf("expected delta ~%d for momentum>mod, got final=%d", fc.momModD, conv)
	}
}

func TestAddMomentumAdjustment_Weak(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorMomentum: -0.3},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, "A", fc)

	conv, cb := b.build()
	if cb.Final > 100+fc.momWeakD {
		t.Errorf("expected delta <= %d for momentum<weak, got final=%d", fc.momWeakD, conv)
	}
}

func TestAddMomentumAdjustment_Neutral(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorMomentum: 0.0},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, "A", fc)

	conv, _ := b.build()
	if conv != 100 {
		t.Errorf("expected no delta for neutral momentum, got final=%d", conv)
	}
}

func TestAddMomentumAdjustment_NilQuery(t *testing.T) {
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addMomentumAdjustment(b, nil, "A", fc)

	conv, _ := b.build()
	if conv != 100 {
		t.Errorf("expected no delta for nil query, got final=%d", conv)
	}
}

// ── addValueAdjustment tests ────────────────────────────────

func TestAddValueAdjustment_High(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorValue: 0.5},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addValueAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final <= 100 {
		t.Errorf("expected positive delta for value>high, got final=%d", cb.Final)
	}
}

func TestAddValueAdjustment_Moderate(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorValue: 0.2},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addValueAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final <= 100 {
		t.Errorf("expected positive delta for value>mod, got final=%d", cb.Final)
	}
}

func TestAddValueAdjustment_Weak(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorValue: -0.5},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addValueAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final >= 100 {
		t.Errorf("expected negative delta for value<weak, got final=%d", cb.Final)
	}
}

func TestAddValueAdjustment_Neutral(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorValue: 0.0},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addValueAdjustment(b, fq, "A", fc)

	conv, _ := b.build()
	if conv != 100 {
		t.Errorf("expected no delta for neutral value, got final=%d", conv)
	}
}

// ── addQualityAdjustment tests ──────────────────────────────

func TestAddQualityAdjustment_Boost(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorQuality: 0.4},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addQualityAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final <= 100 {
		t.Errorf("expected positive delta for quality>thresh, got final=%d", cb.Final)
	}
}

func TestAddQualityAdjustment_Below(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorQuality: 0.0},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addQualityAdjustment(b, fq, "A", fc)

	conv, _ := b.build()
	if conv != 100 {
		t.Errorf("expected no delta for quality below thresh, got final=%d", conv)
	}
}

// ── addLiquidityAdjustment tests ────────────────────────────

func TestAddLiquidityAdjustment_High(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorLiquidity: 0.7},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addLiquidityAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final <= 100 {
		t.Errorf("expected positive delta for liquidity>high, got final=%d", cb.Final)
	}
}

func TestAddLiquidityAdjustment_Good(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorLiquidity: 0.35},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addLiquidityAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final <= 100 {
		t.Errorf("expected positive delta for liquidity>good, got final=%d", cb.Final)
	}
}

func TestAddLiquidityAdjustment_Low(t *testing.T) {
	fq := mockFactorQuery{scores: map[string]map[portfolio.FactorType]float64{
		"A": {portfolio.FactorLiquidity: -0.5},
	}}
	b := newConvictionBuilder(100, 50)
	fc := loadFactorConfig()
	addLiquidityAdjustment(b, fq, "A", fc)

	_, cb := b.build()
	if cb.Final >= 100 {
		t.Errorf("expected negative delta for liquidity<low, got final=%d", cb.Final)
	}
}
