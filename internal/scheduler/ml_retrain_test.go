package scheduler

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
)

type simpleRand struct{ state uint64 }

func (r *simpleRand) next() float64 {
	r.state = r.state*6364136223846793005 + 1
	return float64(r.state>>11) / float64(1<<53)
}

func TestMLRetrainScheduler_OLSDeterministic(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")

	rng := simpleRand{state: 42}
	n := 50
	bars := make([]domain.DailyBar, n)
	for i := range n {
		date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		base := 100.0 + float64(i)*0.3
		bars[i] = domain.DailyBar{
			Date:   date,
			Symbol: "2330.TW",
			Name:   "TSMC",
			Open:   base + rng.next()*2 - 1,
			High:   base + 2 + rng.next()*3 - 1.5,
			Low:    base - 2 + rng.next()*3 - 1.5,
			Close:  base + rng.next()*2 - 0.5,
			Volume: 1000000 + int64(rng.next()*500000),
			Source: "test",
		}
	}

	writeTestCSV(t, csvPath, bars)

	sched := NewMLRetrainScheduler(csvPath)
	sched.SetWorkDir(dir)

	ctx := context.Background()
	if err := sched.RetrainAll(ctx); err != nil {
		t.Fatalf("RetrainAll failed: %v", err)
	}

	X := extractFeatures(bars)
	if len(X) == 0 {
		t.Fatal("expected non-empty features")
	}

	for _, name := range []string{"ols", "elasticnet", "pcr", "pls"} {
		model, err := sched.GetLatestModel(name)
		if err != nil {
			t.Fatalf("GetLatestModel(%q): %v", name, err)
		}
		pred, err := model.Predict(X)
		if err != nil {
			t.Fatalf("%s.Predict: %v", name, err)
		}
		if len(pred) != len(X) {
			t.Fatalf("%s.Predict: got %d predictions, want %d", name, len(pred), len(X))
		}
		t.Logf("%s: predictions[0]=%.4f predictions[last]=%.4f", name, pred[0], pred[len(pred)-1])
	}

	for _, name := range []string{"ols", "elasticnet", "pcr", "pls"} {
		statePath := filepath.Join(dir, "data", "state", "ml_models", name+".json")
		state, err := LoadModelState(statePath)
		if err != nil {
			t.Fatalf("LoadModelState(%q): %v", statePath, err)
		}
		if state.Name != name {
			t.Errorf("state.Name = %q, want %q", state.Name, name)
		}
		if state.NumSamples < 1 {
			t.Errorf("state.NumSamples = %d, want > 0", state.NumSamples)
		}
		t.Logf("%s state: samples=%d features=%d model_type=%s", name, state.NumSamples, state.NumFeatures, state.ModelType)
	}
}

func TestMLRetrainScheduler_EmptyData(t *testing.T) {
	sched := NewMLRetrainScheduler("/nonexistent/path.csv")

	ctx := context.Background()
	err := sched.RetrainAll(ctx)
	if err == nil {
		t.Error("expected error for nonexistent replay path")
	}
}

func TestMLRetrainScheduler_RetrainSingle(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")

	rng := simpleRand{state: 17}
	n := 30
	bars := make([]domain.DailyBar, n)
	for i := range n {
		date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		base := 50.0 + float64(i)*2
		bars[i] = domain.DailyBar{
			Date:   date,
			Symbol: "2317.TW",
			Name:   "Hon Hai",
			Open:   base + rng.next()*4 - 2,
			High:   base + 4 + rng.next()*4 - 2,
			Low:    base - 3 + rng.next()*4 - 2,
			Close:  base*1.2 + rng.next()*3 - 1.5,
			Volume: 500000 + int64(rng.next()*200000),
			Source: "test",
		}
	}

	writeTestCSV(t, csvPath, bars)

	sched := NewMLRetrainScheduler(csvPath)
	sched.SetWorkDir(dir)
	ctx := context.Background()

	if err := sched.RetrainSingle(ctx, "ols"); err != nil {
		t.Fatalf("RetrainSingle(ols): %v", err)
	}

	_, err := sched.GetLatestModel("ols")
	if err != nil {
		t.Fatalf("GetLatestModel(ols) after retrain: %v", err)
	}

	statePath := filepath.Join(dir, "data", "state", "ml_models", "ols.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("ols state file not created: %v", err)
	}

	for _, name := range []string{"elasticnet", "pcr", "pls"} {
		otherPath := filepath.Join(dir, "data", "state", "ml_models", name+".json")
		if _, err := os.Stat(otherPath); err == nil {
			t.Errorf("%s state file created but only ols was trained", name)
		}
	}
}

func TestMLRetrainScheduler_PredictionConsistency(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")

	rng := simpleRand{state: 99}
	n := 40
	bars := make([]domain.DailyBar, n)
	for i := range n {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		base := 200.0 + float64(i)*0.5
		bars[i] = domain.DailyBar{
			Date:   date,
			Symbol: "2454.TW",
			Name:   "Mediatek",
			Open:   base + rng.next()*5 - 2.5,
			High:   base + 5 + rng.next()*5 - 2.5,
			Low:    base - 4 + rng.next()*5 - 2.5,
			Close:  base*0.9 + rng.next()*4 - 2,
			Volume: 2000000 + int64(rng.next()*1000000),
			Source: "test",
		}
	}

	writeTestCSV(t, csvPath, bars)

	ols1 := ml.NewOLS()
	X := extractFeatures(bars)
	y := extractLabels(bars)
	if err := ols1.Fit(X, y); err != nil {
		t.Fatalf("ols1.Fit: %v", err)
	}
	pred1, err := ols1.Predict(X)
	if err != nil {
		t.Fatalf("ols1.Predict: %v", err)
	}

	ols2 := ml.NewOLS()
	if err := ols2.Fit(X, y); err != nil {
		t.Fatalf("ols2.Fit: %v", err)
	}
	pred2, err := ols2.Predict(X)
	if err != nil {
		t.Fatalf("ols2.Predict: %v", err)
	}

	if len(pred1) != len(pred2) {
		t.Fatalf("pred length mismatch: %d vs %d", len(pred1), len(pred2))
	}

	for i := range pred1 {
		if math.Abs(pred1[i]-pred2[i]) > 1e-10 {
			t.Errorf("pred[%d] mismatch: %f vs %f", i, pred1[i], pred2[i])
		}
	}

	t.Logf("OLS prediction consistency: %d values match within 1e-10", len(pred1))
}

func writeTestCSV(t *testing.T, path string, bars []domain.DailyBar) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test CSV: %v", err)
	}
	defer func() { _ = f.Close() }()

	_, _ = f.WriteString("Date,Code,Name,TradeVolume,Open,High,Low,Close\n")

	for _, b := range bars {
		dateStr := b.Date.Format("2006-01-02")
		code := b.Symbol
		if len(code) > 3 && code[len(code)-3:] == ".TW" {
			code = code[:len(code)-3]
		}
		line := fmt.Sprintf(
			"%s,%s,%s,%d,%s,%s,%s,%s\n",
			dateStr,
			code,
			b.Name,
			b.Volume,
			fmt.Sprintf("%.2f", b.Open),
			fmt.Sprintf("%.2f", b.High),
			fmt.Sprintf("%.2f", b.Low),
			fmt.Sprintf("%.2f", b.Close),
		)
		_, _ = f.WriteString(line)
	}
}
