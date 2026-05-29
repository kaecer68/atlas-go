package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// MLRetrainScheduler periodically retrains ML models (OLS, ElasticNet, PCR, PLS)
// using the latest replay data and persists trained model parameters to disk.
type MLRetrainScheduler struct {
	models   map[string]ml.Model
	dataPath string
	logger   *slog.Logger
	workDir  string
}

// NewMLRetrainScheduler creates a scheduler with all four models initialised.
func NewMLRetrainScheduler(dataPath string) *MLRetrainScheduler {
	return &MLRetrainScheduler{
		models: map[string]ml.Model{
			"ols":        ml.NewOLS(),
			"elasticnet": ml.NewElasticNet(),
			"pcr":        ml.NewPCR(),
			"pls":        ml.NewPLS(),
		},
		dataPath: dataPath,
		logger:   slog.Default().With("component", "ml_retrain"),
	}
}

// SetWorkDir configures the root work directory for output paths.
func (s *MLRetrainScheduler) SetWorkDir(dir string) {
	s.workDir = dir
}

// RetrainAll loads the latest replay data, fits every registered model,
// and persists the trained model parameters.
func (s *MLRetrainScheduler) RetrainAll(ctx context.Context) error {
	ds, err := replay.LoadTWSEOpenDataCSV(s.dataPath)
	if err != nil {
		return fmt.Errorf("ml_retrain: load replay: %w", err)
	}

	bars := datasetToBars(ds)
	if len(bars) == 0 {
		return fmt.Errorf("ml_retrain: no DailyBar data found in %s", s.dataPath)
	}

	X := extractFeatures(bars)
	y := extractLabels(bars)

	if len(X) == 0 || len(y) == 0 {
		return fmt.Errorf("ml_retrain: empty feature/label data")
	}

	s.logger.Info("retrain_all_start", "bars", len(bars), "features", len(X[0]))

	var firstErr error
	for name, model := range s.models {
		if err := s.fitAndPersist(ctx, name, model, X, y); err != nil {
			s.logger.Error("retrain_failed", "model", name, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// RetrainSingle loads the latest replay data and fits a single named model.
func (s *MLRetrainScheduler) RetrainSingle(ctx context.Context, name string) error {
	model, ok := s.models[name]
	if !ok {
		return fmt.Errorf("ml_retrain: unknown model: %s", name)
	}

	ds, err := replay.LoadTWSEOpenDataCSV(s.dataPath)
	if err != nil {
		return fmt.Errorf("ml_retrain: load replay: %w", err)
	}

	bars := datasetToBars(ds)
	if len(bars) == 0 {
		return fmt.Errorf("ml_retrain: no DailyBar data found in %s", s.dataPath)
	}

	X := extractFeatures(bars)
	y := extractLabels(bars)

	return s.fitAndPersist(ctx, name, model, X, y)
}

// GetLatestModel returns the most recently trained model for the given name.
func (s *MLRetrainScheduler) GetLatestModel(name string) (ml.Model, error) {
	model, ok := s.models[name]
	if !ok {
		return nil, fmt.Errorf("ml_retrain: unknown model: %s", name)
	}
	return model, nil
}

func (s *MLRetrainScheduler) fitAndPersist(ctx context.Context, name string, model ml.Model, X [][]float64, y []float64) error {
	if err := model.Fit(X, y); err != nil {
		return fmt.Errorf("ml_retrain: fit %s: %w", name, err)
	}

	s.logger.Info("retrain_ok", "model", name, "samples", len(X))

	if err := s.saveModelState(name, model, len(X), len(X[0])); err != nil {
		s.logger.Warn("ml_retrain_save", "model", name, "err", err)
	}

	return nil
}

// modelState is the serialisable representation of a trained model.
type modelState struct {
	Name        string          `json:"name"`
	ModelType   string          `json:"model_type"`
	TrainedAt   time.Time       `json:"trained_at"`
	NumSamples  int             `json:"num_samples"`
	NumFeatures int             `json:"num_features"`
	DataPath    string          `json:"data_path"`
	ModelConfig json.RawMessage `json:"model_config"`
}

func (s *MLRetrainScheduler) saveModelState(name string, model ml.Model, nSamples, nFeatures int) error {
	cfg, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("marshal model config: %w", err)
	}

	state := modelState{
		Name:        name,
		ModelType:   fmt.Sprintf("%T", model),
		TrainedAt:   time.Now(),
		NumSamples:  nSamples,
		NumFeatures: nFeatures,
		DataPath:    s.dataPath,
		ModelConfig: cfg,
	}

	outDir := s.outputDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create ml_models dir: %w", err)
	}

	path := filepath.Join(outDir, name+".json")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	s.logger.Info("model_saved", "model", name, "path", path)
	return nil
}

func (s *MLRetrainScheduler) outputDir() string {
	if s.workDir != "" {
		return filepath.Join(s.workDir, "data", "state", "ml_models")
	}
	return "data/state/ml_models"
}

func extractFeatures(bars []domain.DailyBar) [][]float64 {
	n := len(bars)
	if n < 6 {
		return nil
	}

	var sumO, sumH, sumL, sumC, sumV float64
	for _, b := range bars {
		sumO += b.Open
		sumH += b.High
		sumL += b.Low
		sumC += b.Close
		sumV += float64(b.Volume)
	}
	meanO, meanH, meanL, meanC, meanV := sumO/float64(n), sumH/float64(n), sumL/float64(n), sumC/float64(n), sumV/float64(n)

	var so, sh, sl, sc, sv float64
	for _, b := range bars {
		so += (b.Open - meanO) * (b.Open - meanO)
		sh += (b.High - meanH) * (b.High - meanH)
		sl += (b.Low - meanL) * (b.Low - meanL)
		sc += (b.Close - meanC) * (b.Close - meanC)
		sv += (float64(b.Volume) - meanV) * (float64(b.Volume) - meanV)
	}
	stdO := math.Sqrt(so / float64(n))
	stdH := math.Sqrt(sh / float64(n))
	stdL := math.Sqrt(sl / float64(n))
	stdC := math.Sqrt(sc / float64(n))
	stdV := math.Sqrt(sv / float64(n))

	X := make([][]float64, 0, n-6)
	for i := 5; i < n-1; i++ {
		b := bars[i]
		row := make([]float64, 8)

		row[0] = zScore(b.Open, meanO, stdO)
		row[1] = zScore(b.High, meanH, stdH)
		row[2] = zScore(b.Low, meanL, stdL)
		row[3] = zScore(b.Close, meanC, stdC)
		row[4] = zScore(float64(b.Volume), meanV, stdV)

		ma5Open := (bars[i-4].Open + bars[i-3].Open + bars[i-2].Open + bars[i-1].Open + b.Open) / 5
		ma5Close := (bars[i-4].Close + bars[i-3].Close + bars[i-2].Close + bars[i-1].Close + b.Close) / 5
		ma5Vol := float64(bars[i-4].Volume+bars[i-3].Volume+bars[i-2].Volume+bars[i-1].Volume+b.Volume) / 5

		row[5] = safeDiv(b.Open, ma5Open) - 1
		row[6] = safeDiv(b.Close, ma5Close) - 1
		row[7] = safeDiv(float64(b.Volume), ma5Vol) - 1

		X = append(X, row)
	}
	return X
}

func extractLabels(bars []domain.DailyBar) []float64 {
	n := len(bars)
	if n < 2 {
		return nil
	}
	y := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		y[i] = bars[i+1].Close/bars[i].Close - 1
	}
	skip := 5
	if len(y) <= skip {
		return nil
	}
	return y[skip:]
}

func datasetToBars(ds *replay.Dataset) []domain.DailyBar {
	if ds == nil {
		return nil
	}
	var bars []domain.DailyBar
	for _, date := range ds.Dates {
		day := ds.ByDate[date.Format("2006-01-02")]
		for _, bar := range day {
			bars = append(bars, bar)
		}
	}
	return bars
}

func zScore(v, mean, std float64) float64 {
	if std == 0 {
		return 0
	}
	return (v - mean) / std
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// LoadModelState reads a saved model state JSON file.
func LoadModelState(path string) (*modelState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ml_retrain: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var state modelState
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, fmt.Errorf("ml_retrain: decode %s: %w", path, err)
	}
	return &state, nil
}
