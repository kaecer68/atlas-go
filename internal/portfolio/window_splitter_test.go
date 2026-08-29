package portfolio

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

func makeOutcomes(n int, baseReturn float64) []domain.RecommendationOutcome {
	out := make([]domain.RecommendationOutcome, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		out[i] = domain.RecommendationOutcome{
			AgentID:       "test-agent",
			Skill:         "test_skill",
			Layer:         shared.AgentLayer("sector"),
			Symbol:        "2330",
			Window:        "session-20260101-daily",
			ForwardReturn: baseReturn + float64(i)*0.001,
			Hit:           true,
		}
		out[i].RecordedAt = base.AddDate(0, 0, i)
	}
	return out
}

func TestSplitChronological_8020(t *testing.T) {
	outcomes := makeOutcomes(100, 0.01)
	cfg := SplitConfig{Method: SplitChronological, TrainRatio: 0.8}
	tr, te := Split(outcomes, cfg)
	if len(tr) != 80 {
		t.Fatalf("train count = %d, want 80", len(tr))
	}
	if len(te) != 20 {
		t.Fatalf("test count = %d, want 20", len(te))
	}
}

func TestSplitChronological_PreservesOrder(t *testing.T) {
	outcomes := makeOutcomes(50, 0.0)
	cfg := SplitConfig{Method: SplitChronological, TrainRatio: 0.8}
	tr, te := Split(outcomes, cfg)
	if tr[0].RecordedAt.After(tr[len(tr)-1].RecordedAt) {
		t.Fatal("train set must be in chronological order")
	}
	if te[0].RecordedAt.Before(tr[len(tr)-1].RecordedAt) {
		t.Fatal("test set must start after train set ends")
	}
}

func TestSplitChronological_SmallRatioRoundsDown(t *testing.T) {
	outcomes := makeOutcomes(99, 0.01)
	cfg := SplitConfig{Method: SplitChronological, TrainRatio: 0.8}
	tr, te := Split(outcomes, cfg)
	if len(tr)+len(te) != 99 {
		t.Fatalf("split should cover all 99 outcomes, got %d+%d", len(tr), len(te))
	}
}

func TestSplitWalkForward_OneFold(t *testing.T) {
	outcomes := makeOutcomes(100, 0.01)
	cfg := SplitConfig{Method: SplitWalkForward, TrainSize: 60, TestSize: 20}
	folds := WalkForwardFolds(outcomes, cfg)
	if len(folds) != 2 {
		t.Fatalf("expected 2 non-overlapping folds (60+20 twice over 100), got %d", len(folds))
	}
	for i, f := range folds {
		if len(f.Train) != 60 {
			t.Fatalf("fold %d train size = %d, want 60", i, len(f.Train))
		}
		if len(f.Test) != 20 {
			t.Fatalf("fold %d test size = %d, want 20", i, len(f.Test))
		}
	}
}

func TestSplitWalkForward_InsufficientForFullFold(t *testing.T) {
	outcomes := makeOutcomes(50, 0.01)
	cfg := SplitConfig{Method: SplitWalkForward, TrainSize: 60, TestSize: 20}
	folds := WalkForwardFolds(outcomes, cfg)
	if len(folds) != 0 {
		t.Fatalf("expected 0 folds when data < TrainSize, got %d", len(folds))
	}
}

func TestSplit_EmptyInput(t *testing.T) {
	cfg := SplitConfig{Method: SplitChronological, TrainRatio: 0.8}
	tr, te := Split(nil, cfg)
	if tr != nil || te != nil {
		t.Fatalf("empty input should return nil slices, got %v / %v", tr, te)
	}
}

func TestIsOOSDivergent(t *testing.T) {
	tests := []struct {
		name       string
		is, oos    float64
		threshold  float64
		wantResult bool
	}{
		{"both positive IS=2 OOS=0.5 ratio=4", 2.0, 0.5, 2.0, true},
		{"both positive IS=1 OOS=0.8 ratio=1.25", 1.0, 0.8, 2.0, false},
		{"IS=0 OOS=0", 0, 0, 2.0, false},
		{"OOS=0 IS=2", 2.0, 0, 2.0, true},
		{"OOS=negative IS=positive (overfit)", 1.0, -0.5, 2.0, true},
		{"ratio=2.0 exactly (boundary, not divergent)", 2.0, 1.0, 2.0, false},
		{"IS=0.5 OOS=0.3 (small values, ratio=1.67)", 0.5, 0.3, 2.0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsOOSDivergent(tt.is, tt.oos, tt.threshold)
			if got != tt.wantResult {
				t.Fatalf("IsOOSDivergent(%v, %v, %v) = %v, want %v",
					tt.is, tt.oos, tt.threshold, got, tt.wantResult)
			}
		})
	}
}
