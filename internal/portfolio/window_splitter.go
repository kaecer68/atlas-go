package portfolio

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SplitMethod selects the train/test partition strategy.
type SplitMethod string

const (
	// SplitChronological puts the first TrainRatio of outcomes into train
	// and the rest into test. Order is preserved; outcomes must already
	// be sorted chronologically by caller.
	SplitChronological SplitMethod = "chronological"
	// SplitWalkForward produces multiple rolling (train, test) folds,
	// each of fixed TrainSize + TestSize, advancing by TestSize.
	SplitWalkForward SplitMethod = "walk_forward"
)

// SplitConfig parameterizes the train/test split. Only the fields
// relevant to the chosen Method are read.
type SplitConfig struct {
	Method      SplitMethod
	TrainRatio  float64 // for SplitChronological (default 0.8)
	TrainSize   int     // for SplitWalkForward
	TestSize    int     // for SplitWalkForward
	EmbargoDays int     // for SplitWalkForward: non-overlap gap (trading days) between train end and test start of next fold. 0 = no embargo. Consumers that want a safety default (e.g. oosWindow) should apply it at their read site, not via the zero value.
}

// Split returns (train, test) outcomes according to cfg. Returns nil
// for both when input is empty. For chronological splits, all outcomes
// must be sorted by RecordedAt ascending (caller's responsibility).
func Split(outcomes []domain.RecommendationOutcome, cfg SplitConfig) ([]domain.RecommendationOutcome, []domain.RecommendationOutcome) {
	if len(outcomes) == 0 {
		return nil, nil
	}
	switch cfg.Method {
	case SplitChronological:
		ratio := cfg.TrainRatio
		if ratio <= 0 || ratio >= 1 {
			ratio = 0.8
		}
		splitIdx := max(int(math.Floor(float64(len(outcomes))*ratio)), 1)
		if splitIdx >= len(outcomes) {
			splitIdx = len(outcomes) - 1
		}
		return outcomes[:splitIdx], outcomes[splitIdx:]
	default:
		return outcomes, nil
	}
}

// WalkForwardFolds produces sequential (train, test) folds. Each fold
// has cfg.TrainSize train outcomes followed by cfg.TestSize test outcomes.
// cfg.EmbargoDays controls the non-overlap gap (trading days) between the
// train end and the test start of the *next* fold, and is reserved between
// the train end and test start of the same fold. This prevents data leakage
// from corporate-event reorgs and event-driven labeling lag.
//   - EmbargoDays <= 0  → no gap (preserves pre-embargo behavior; the
//     zero value is a no-op so existing callers are unaffected).
//   - Folds whose test window would extend beyond the available data after
//     the embargo gap are skipped (no partial folds).
//
// Returns nil if total outcomes < TrainSize (cannot form even one fold).
func WalkForwardFolds(outcomes []domain.RecommendationOutcome, cfg SplitConfig) []Fold {
	if len(outcomes) < cfg.TrainSize {
		return nil
	}
	step := max(cfg.TestSize, 1)
	embargo := max(cfg.EmbargoDays, 0)
	var folds []Fold
	for start := 0; start+cfg.TrainSize <= len(outcomes); start += step {
		end := start + cfg.TrainSize
		testStart := end + embargo
		testEnd := testStart + cfg.TestSize
		if testEnd > len(outcomes) {
			break // skip partial folds when embargo + test size overflows
		}
		folds = append(folds, Fold{
			Train: outcomes[start:end],
			Test:  outcomes[testStart:testEnd],
		})
	}
	return folds
}

// Fold is one (train, test) pair from WalkForwardFolds.
type Fold struct {
	Train []domain.RecommendationOutcome
	Test  []domain.RecommendationOutcome
}

// IsOOSDivergent returns true when the in-sample / out-of-sample Sharpe
// ratio exceeds the overfit threshold AND both have non-trivial magnitude.
// Returns false when IS is near zero (no signal to overfit) or ratio is
// at/below threshold.
//
// Special cases treated as divergent (classic overfit signatures):
//   - IS > 0 AND OOS ≤ 0 (OOS nullifies the apparent skill)
//   - IS > 0 AND OOS = 0 (OOS has no edge despite IS showing one)
func IsOOSDivergent(isSharpe, oosSharpe, overfitThreshold float64) bool {
	if isSharpe > 0 && oosSharpe <= 0 {
		return true
	}
	if math.Abs(isSharpe) < 0.01 {
		return false
	}
	if math.Abs(oosSharpe) < 0.01 {
		return true
	}
	ratio := math.Abs(isSharpe / oosSharpe)
	return ratio > overfitThreshold
}

// SortOutcomesByTime is a helper that orders outcomes by RecordedAt
// ascending. Chronological Split requires this.
func SortOutcomesByTime(outcomes []domain.RecommendationOutcome) []domain.RecommendationOutcome {
	sorted := make([]domain.RecommendationOutcome, len(outcomes))
	copy(sorted, outcomes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].RecordedAt.Before(sorted[j].RecordedAt)
	})
	return sorted
}
