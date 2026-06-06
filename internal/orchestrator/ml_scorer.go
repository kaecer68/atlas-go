package orchestrator

import (
	"fmt"
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ScoredQuote pairs a Quote with its ML-predicted score.
type ScoredQuote struct {
	Quote domain.Quote
	Score float64
}

// MLScorer wraps an ml.Model to score quotes using learned factor weights.
// Train() must be called with historical DailyBar data before Score/BatchScore.
type MLScorer struct {
	model   ml.Model
	factors []portfolio.FactorType
	trained bool
}

// NewMLScorer creates a new MLScorer backed by the given model.
// The scorer uses four core factors: Momentum, Value, Quality, and Liquidity.
func NewMLScorer(model ml.Model) *MLScorer {
	return &MLScorer{
		model: model,
		factors: []portfolio.FactorType{
			portfolio.FactorMomentum,
			portfolio.FactorValue,
			portfolio.FactorQuality,
			portfolio.FactorLiquidity,
		},
	}
}

// Train extracts features from historical DailyBar data and fits the underlying model.
// Features are computed directly from OHLCV data (no FactorEngine dependency at training time):
//
//	Momentum:  (Close - Open) / Open    (intraday return)
//	Value:     Close / Open              (price range proxy)
//	Quality:   1.0 - (High-Low)/Close    (inverse intraday volatility)
//	Liquidity: log(1 + Volume)           (log-normalized volume)
//
// Labels are forward returns: (next_bar.Close - bar.Close) / bar.Close.
func (s *MLScorer) Train(bars []domain.DailyBar) error {
	if len(bars) == 0 {
		return fmt.Errorf("ml_scorer: Train called with empty bars")
	}

	// Group bars by symbol and sort chronologically.
	symbolBars := groupBarsBySymbol(bars)

	var X [][]float64
	var y []float64

	for _, b := range symbolBars {
		if len(b) < 2 {
			continue
		}
		// Sort by date ascending.
		sort.Slice(b, func(i, j int) bool {
			return b[i].Date.Before(b[j].Date)
		})

		for i := 0; i < len(b)-1; i++ {
			bar := b[i]
			next := b[i+1]

			features := extractFeatures(bar)
			label := forwardReturn(bar, next)

			X = append(X, features)
			y = append(y, label)
		}
	}

	if len(X) == 0 {
		return fmt.Errorf("ml_scorer: insufficient data after feature extraction (need 2+ consecutive bars per symbol)")
	}

	if err := s.model.Fit(X, y); err != nil {
		return fmt.Errorf("ml_scorer: model Fit failed: %w", err)
	}

	s.trained = true
	return nil
}

// Score returns an ML-predicted score for a single quote using the given
// pre-computed factor scores as feature inputs.
func (s *MLScorer) Score(quote domain.Quote, factorScores map[portfolio.FactorType]float64) (float64, error) {
	if !s.trained {
		return 0, fmt.Errorf("ml_scorer: Score called before Train")
	}

	features := make([]float64, len(s.factors))
	for i, ft := range s.factors {
		if score, ok := factorScores[ft]; ok {
			features[i] = score
		}
	}

	predictions, err := s.model.Predict([][]float64{features})
	if err != nil {
		return 0, fmt.Errorf("ml_scorer: Predict failed: %w", err)
	}
	if len(predictions) == 0 {
		return 0, fmt.Errorf("ml_scorer: Predict returned empty result")
	}
	return predictions[0], nil
}

// BatchScore scores multiple quotes using pre-computed factor scores from the
// given FactorQuery. Results are sorted by score in descending order.
func (s *MLScorer) BatchScore(quotes []domain.Quote, factorSnap FactorQuery) ([]ScoredQuote, error) {
	if !s.trained {
		return nil, fmt.Errorf("ml_scorer: BatchScore called before Train")
	}
	if len(quotes) == 0 {
		return nil, nil
	}

	X := make([][]float64, 0, len(quotes))
	validIdx := make([]int, 0, len(quotes))

	for idx, q := range quotes {
		features := make([]float64, len(s.factors))
		hasData := false
		for i, ft := range s.factors {
			if score, ok := factorSnap.GetScore(q.Symbol, ft); ok {
				features[i] = score
				hasData = true
			}
		}
		if !hasData {
			continue
		}
		X = append(X, features)
		validIdx = append(validIdx, idx)
	}

	if len(X) == 0 {
		return nil, fmt.Errorf("ml_scorer: no valid factor scores available for any quote")
	}

	predictions, err := s.model.Predict(X)
	if err != nil {
		return nil, fmt.Errorf("ml_scorer: Predict failed: %w", err)
	}
	if len(predictions) != len(X) {
		return nil, fmt.Errorf("ml_scorer: Predict returned %d results, expected %d", len(predictions), len(X))
	}

	result := make([]ScoredQuote, len(predictions))
	for i := range predictions {
		result[i] = ScoredQuote{
			Quote: quotes[validIdx[i]],
			Score: predictions[i],
		}
	}

	// Sort descending by score.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result, nil
}

// IsTrained returns whether the scorer has been successfully trained.
func (s *MLScorer) IsTrained() bool {
	return s.trained
}

// ── internal helpers ───────────────────────────────────────────────────────────

// groupBarsBySymbol groups DailyBar entries by their Symbol field.
// Each group preserves the original order of appearance (callers should sort).
func groupBarsBySymbol(bars []domain.DailyBar) map[string][]domain.DailyBar {
	groups := make(map[string][]domain.DailyBar)
	for _, bar := range bars {
		groups[bar.Symbol] = append(groups[bar.Symbol], bar)
	}
	return groups
}

// extractFeatures computes a feature vector from a single DailyBar.
// Features: [momentum, value, quality, liquidity].
func extractFeatures(bar domain.DailyBar) []float64 {
	open := bar.Open
	close := bar.Close
	high := bar.High
	low := bar.Low
	volume := float64(bar.Volume)

	if open == 0 {
		open = close
	}

	momentum := (close - open) / open
	value := close / open
	quality := 1.0 - (high-low)/close
	if close == 0 {
		quality = 0
	}
	liquidity := math.Log(1 + volume)

	return []float64{momentum, value, quality, liquidity}
}

// forwardReturn computes the return from current bar's close to next bar's close.
func forwardReturn(current, next domain.DailyBar) float64 {
	if current.Close == 0 {
		return 0
	}
	return (next.Close - current.Close) / current.Close
}

// compile-time interface check: MLScorer implements nothing external yet.
var _ = (*MLScorer)(nil) // ensure type compiles
