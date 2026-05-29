package orchestrator

import (
	"fmt"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// mockModel implements ml.Model for testing.
type mockModel struct {
	fitCalled     bool
	predictCalled bool
	predictVal    float64
	fitErr        error
	predictErr    error
}

func (m *mockModel) Fit(X [][]float64, y []float64) error {
	m.fitCalled = true
	return m.fitErr
}

func (m *mockModel) Predict(X [][]float64) ([]float64, error) {
	m.predictCalled = true
	if m.predictErr != nil {
		return nil, m.predictErr
	}
	preds := make([]float64, len(X))
	for i := range preds {
		preds[i] = m.predictVal
	}
	return preds, nil
}

func makeDailyBar(symbol string, date string, open, high, low, close float64, volume int64) domain.DailyBar {
	t, _ := time.Parse("2006-01-02", date)
	return domain.DailyBar{
		Date:   t,
		Symbol: symbol,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}
}

func TestNewMLScorer(t *testing.T) {
	m := &mockModel{}
	s := NewMLScorer(m)

	if s.IsTrained() {
		t.Error("new scorer should not be trained")
	}
	if s.model != m {
		t.Error("model not set correctly")
	}
	if len(s.factors) != 4 {
		t.Errorf("expected 4 factors, got %d", len(s.factors))
	}
}

func TestMLScorerTrainWithEmptyBars(t *testing.T) {
	s := NewMLScorer(&mockModel{})
	err := s.Train(nil)
	if err == nil {
		t.Error("expected error for nil bars")
	}
	err = s.Train([]domain.DailyBar{})
	if err == nil {
		t.Error("expected error for empty bars")
	}
}

func TestMLScorerTrainInsufficientData(t *testing.T) {
	s := NewMLScorer(&mockModel{})
	// Single bar: cannot compute forward return.
	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
	}
	err := s.Train(bars)
	if err == nil {
		t.Error("expected error for insufficient data (single bar)")
	}
}

func TestMLScorerTrainSuccess(t *testing.T) {
	m := &mockModel{}
	s := NewMLScorer(m)

	// Two consecutive bars for 2330 — one training sample.
	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	err := s.Train(bars)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	if !s.IsTrained() {
		t.Error("scorer should be trained after successful Train")
	}
	if !m.fitCalled {
		t.Error("Fit should have been called")
	}
}

func TestMLScorerTrainFitError(t *testing.T) {
	m := &mockModel{fitErr: fmt.Errorf("singular matrix")}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	err := s.Train(bars)
	if err == nil {
		t.Error("expected error when Fit fails")
	}
	if s.IsTrained() {
		t.Error("scorer should not be trained after failed Fit")
	}
}

func TestMLScorerScoreBeforeTraining(t *testing.T) {
	s := NewMLScorer(&mockModel{})
	_, err := s.Score(domain.Quote{}, map[portfolio.FactorType]float64{})
	if err == nil {
		t.Error("expected error when scoring before training")
	}
}

func TestMLScorerScoreAfterTraining(t *testing.T) {
	m := &mockModel{predictVal: 0.75}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	quote := domain.Quote{Symbol: "2330", Last: 515}
	factorScores := map[portfolio.FactorType]float64{
		portfolio.FactorMomentum:  0.03,
		portfolio.FactorValue:     0.04,
		portfolio.FactorQuality:   0.05,
		portfolio.FactorLiquidity: 0.02,
	}

	score, err := s.Score(quote, factorScores)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if score != 0.75 {
		t.Errorf("expected score 0.75, got %v", score)
	}
	if !m.predictCalled {
		t.Error("Predict should have been called")
	}
}

func TestMLScorerBatchScoreBeforeTraining(t *testing.T) {
	s := NewMLScorer(&mockModel{})
	_, err := s.BatchScore([]domain.Quote{}, nil)
	if err == nil {
		t.Error("expected error when batch scoring before training")
	}
}

func TestMLScorerBatchScoreEmpty(t *testing.T) {
	m := &mockModel{predictVal: 0.5}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	result, err := s.BatchScore([]domain.Quote{}, nil)
	if err != nil {
		t.Fatalf("BatchScore with empty quotes failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(result))
	}
}

// stubFactorSnap implements FactorQuery for testing.
type stubFactorSnap struct {
	scores map[string]map[portfolio.FactorType]float64
}

func (s *stubFactorSnap) GetScore(symbol string, factor portfolio.FactorType) (float64, bool) {
	if s.scores == nil {
		return 0, false
	}
	entry, ok := s.scores[symbol]
	if !ok {
		return 0, false
	}
	score, ok := entry[factor]
	return score, ok
}

func TestMLScorerBatchScoreSuccess(t *testing.T) {
	m := &mockModel{predictVal: 0.85}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	snap := &stubFactorSnap{
		scores: map[string]map[portfolio.FactorType]float64{
			"2330": {
				portfolio.FactorMomentum:  0.03,
				portfolio.FactorValue:     0.04,
				portfolio.FactorQuality:   0.05,
				portfolio.FactorLiquidity: 0.02,
			},
			"2454": {
				portfolio.FactorMomentum:  0.05,
				portfolio.FactorValue:     0.02,
				portfolio.FactorQuality:   0.03,
				portfolio.FactorLiquidity: 0.01,
			},
		},
	}

	quotes := []domain.Quote{
		{Symbol: "2330", Last: 515},
		{Symbol: "2454", Last: 1100},
	}

	result, err := s.BatchScore(quotes, snap)
	if err != nil {
		t.Fatalf("BatchScore failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Both scores come from mock (both predict 0.85), but verify they're populated.
	for _, r := range result {
		if r.Score != 0.85 {
			t.Errorf("expected score 0.85, got %v", r.Score)
		}
	}
}

func TestMLScorerBatchScoreNoValidData(t *testing.T) {
	m := &mockModel{predictVal: 0.5}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	snap := &stubFactorSnap{} // empty scores

	quotes := []domain.Quote{
		{Symbol: "9999", Last: 100},
	}

	_, err := s.BatchScore(quotes, snap)
	if err == nil {
		t.Error("expected error for no valid factor scores")
	}
}

func TestMLScorerPredictError(t *testing.T) {
	m := &mockModel{predictErr: fmt.Errorf("not fitted")}
	s := NewMLScorer(m)

	bars := []domain.DailyBar{
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
	}
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	snap := &stubFactorSnap{
		scores: map[string]map[portfolio.FactorType]float64{
			"2330": {portfolio.FactorMomentum: 0.03},
		},
	}
	quotes := []domain.Quote{{Symbol: "2330"}}

	_, err := s.BatchScore(quotes, snap)
	if err == nil {
		t.Error("expected error when Predict fails")
	}
}

func TestMLScorerMultiSymbolTraining(t *testing.T) {
	m := &mockModel{}
	s := NewMLScorer(m)

	// Two symbols, 3 bars each — 2 training samples per symbol = 4 total.
	bars := []domain.DailyBar{
		// Symbol 2330 — 3 bars → 2 training samples
		makeDailyBar("2330", "2026-01-02", 500, 510, 490, 505, 10000),
		makeDailyBar("2330", "2026-01-03", 505, 520, 500, 515, 12000),
		makeDailyBar("2330", "2026-01-04", 515, 530, 510, 525, 11000),
		// Symbol 2454 — 3 bars → 2 training samples
		makeDailyBar("2454", "2026-01-02", 1100, 1120, 1080, 1110, 5000),
		makeDailyBar("2454", "2026-01-03", 1110, 1130, 1090, 1125, 6000),
		makeDailyBar("2454", "2026-01-04", 1125, 1140, 1110, 1130, 5500),
	}

	if err := s.Train(bars); err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	if !s.IsTrained() {
		t.Error("scorer should be trained")
	}
	if !m.fitCalled {
		t.Error("Fit should have been called")
	}
}

func TestExtractFeaturesZeroOpen(t *testing.T) {
	bar := domain.DailyBar{
		Open:   0,
		High:   510,
		Low:    490,
		Close:  505,
		Volume: 10000,
	}
	features := extractFeatures(bar)
	// When Open is 0, it's set to Close → momentum should be 0.
	if len(features) != 4 {
		t.Fatalf("expected 4 features, got %d", len(features))
	}
	if features[0] != 0 {
		t.Errorf("momentum with zero open should be 0, got %v", features[0])
	}
}

func TestExtractFeaturesZeroClose(t *testing.T) {
	bar := domain.DailyBar{
		Open:   500,
		High:   510,
		Low:    490,
		Close:  0,
		Volume: 10000,
	}
	features := extractFeatures(bar)
	if features[2] != 0 {
		t.Errorf("quality with zero close should be 0, got %v", features[2])
	}
}

func TestForwardReturnZeroClose(t *testing.T) {
	current := makeDailyBar("2330", "2026-01-02", 0, 0, 0, 0, 0)
	next := makeDailyBar("2330", "2026-01-03", 0, 0, 0, 100, 0)
	if forwardReturn(current, next) != 0 {
		t.Error("forward return with zero current close should be 0")
	}
}
