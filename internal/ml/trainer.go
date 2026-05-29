package ml

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Model is the interface that all supervised learning models must implement.
type Model interface {
	// Fit trains the model on feature matrix X (n_samples × n_features) and
	// target vector y (n_samples). Returns an error if training fails.
	Fit(X [][]float64, y []float64) error

	// Predict returns predictions for the given feature matrix X.
	Predict(X [][]float64) ([]float64, error)
}

// Splitter defines how to partition data into train/validation folds.
type Splitter interface {
	// Split returns a slice of fold indices. Each fold is a pair of
	// (trainIndices, valIndices).
	Split(nSamples int) [][2][]int
}

// KFoldSplitter implements k-fold cross-validation splitting.
type KFoldSplitter struct {
	K    int
	Seed uint64
}

// Split partitions nSamples into k folds. Each fold uses (k-1) folds for
// training and the remaining fold for validation.
func (s *KFoldSplitter) Split(nSamples int) [][2][]int {
	if s.K <= 1 || s.K > nSamples {
		s.K = min(5, nSamples)
	}

	// Create shuffled indices.
	indices := make([]int, nSamples)
	for i := range indices {
		indices[i] = i
	}
	rng := rand.New(rand.NewPCG(s.Seed, 0))
	rng.Shuffle(nSamples, func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	foldSize := nSamples / s.K
	folds := make([][2][]int, s.K)

	for fold := 0; fold < s.K; fold++ {
		start := fold * foldSize
		end := start + foldSize
		if fold == s.K-1 {
			end = nSamples
		}

		valIdx := indices[start:end]
		valSet := make(map[int]bool, len(valIdx))
		for _, idx := range valIdx {
			valSet[idx] = true
		}

		trainIdx := make([]int, 0, nSamples-len(valIdx))
		for _, idx := range indices {
			if !valSet[idx] {
				trainIdx = append(trainIdx, idx)
			}
		}

		folds[fold] = [2][]int{trainIdx, valIdx}
	}

	return folds
}

// FoldMetrics holds evaluation metrics for a single cross-validation fold.
type FoldMetrics struct {
	Fold      int
	MSE       float64
	MAE       float64
	R2        float64
	TrainSize int
	ValSize   int
}

// Trainer handles model training with cross-validation.
type Trainer struct {
	Model    Model
	Splitter Splitter
}

// NewTrainer creates a Trainer with the given model and k-fold splitter.
// If splitter is nil, a default 5-fold splitter is used.
func NewTrainer(model Model, splitter Splitter) *Trainer {
	if splitter == nil {
		splitter = &KFoldSplitter{K: 5, Seed: 42}
	}
	return &Trainer{Model: model, Splitter: splitter}
}

// CrossValidate performs k-fold cross-validation and returns per-fold metrics
// plus aggregated summary statistics.
func (t *Trainer) CrossValidate(X [][]float64, y []float64) ([]FoldMetrics, error) {
	if len(X) == 0 || len(y) == 0 {
		return nil, fmt.Errorf("ml: empty data")
	}
	if len(X) != len(y) {
		return nil, fmt.Errorf("ml: X and y have different lengths: %d vs %d", len(X), len(y))
	}

	nSamples := len(X)
	folds := t.Splitter.Split(nSamples)
	metrics := make([]FoldMetrics, len(folds))

	for i, fold := range folds {
		trainIdx, valIdx := fold[0], fold[1]
		if len(trainIdx) == 0 || len(valIdx) == 0 {
			return nil, fmt.Errorf("ml: fold %d has empty train or validation set", i)
		}

		// Build training data.
		Xtrain := make([][]float64, len(trainIdx))
		ytrain := make([]float64, len(trainIdx))
		for j, idx := range trainIdx {
			Xtrain[j] = X[idx]
			ytrain[j] = y[idx]
		}

		// Build validation data.
		Xval := make([][]float64, len(valIdx))
		yval := make([]float64, len(valIdx))
		for j, idx := range valIdx {
			Xval[j] = X[idx]
			yval[j] = y[idx]
		}

		// Fit and predict.
		if err := t.Model.Fit(Xtrain, ytrain); err != nil {
			return nil, fmt.Errorf("ml: fold %d fit: %w", i, err)
		}
		pred, err := t.Model.Predict(Xval)
		if err != nil {
			return nil, fmt.Errorf("ml: fold %d predict: %w", i, err)
		}

		// Compute metrics.
		mse, mae := computeMSE(yval, pred), computeMAE(yval, pred)
		r2 := computeR2(yval, pred)

		metrics[i] = FoldMetrics{
			Fold:      i + 1,
			MSE:       mse,
			MAE:       mae,
			R2:        r2,
			TrainSize: len(trainIdx),
			ValSize:   len(valIdx),
		}
	}

	return metrics, nil
}

// Summary aggregates cross-validation metrics into mean ± std.
type CVSummary struct {
	MeanMSE float64
	StdMSE  float64
	MeanMAE float64
	StdMAE  float64
	MeanR2  float64
	StdR2   float64
	NFolds  int
}

// Summarize computes aggregate statistics from fold metrics.
func Summarize(metrics []FoldMetrics) CVSummary {
	if len(metrics) == 0 {
		return CVSummary{}
	}
	n := float64(len(metrics))
	mseSum, maeSum, r2Sum := 0.0, 0.0, 0.0
	mseSq, maeSq, r2Sq := 0.0, 0.0, 0.0

	for _, m := range metrics {
		mseSum += m.MSE
		maeSum += m.MAE
		r2Sum += m.R2
		mseSq += m.MSE * m.MSE
		maeSq += m.MAE * m.MAE
		r2Sq += m.R2 * m.R2
	}

	meanMSE := mseSum / n
	meanMAE := maeSum / n
	meanR2 := r2Sum / n

	return CVSummary{
		MeanMSE: meanMSE,
		StdMSE:  math.Sqrt(mseSq/n - meanMSE*meanMSE),
		MeanMAE: meanMAE,
		StdMAE:  math.Sqrt(maeSq/n - meanMAE*meanMAE),
		MeanR2:  meanR2,
		StdR2:   math.Sqrt(r2Sq/n - meanR2*meanR2),
		NFolds:  len(metrics),
	}
}

// CrossValidateAndSummarize runs cross-validation and returns a summary.
func (t *Trainer) CrossValidateAndSummarize(X [][]float64, y []float64) (CVSummary, error) {
	metrics, err := t.CrossValidate(X, y)
	if err != nil {
		return CVSummary{}, err
	}
	return Summarize(metrics), nil
}

// --- metric helpers ---

func computeMSE(y, pred []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	var sum float64
	for i := range y {
		diff := y[i] - pred[i]
		sum += diff * diff
	}
	return sum / float64(len(y))
}

func computeMAE(y, pred []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	var sum float64
	for i := range y {
		sum += math.Abs(y[i] - pred[i])
	}
	return sum / float64(len(y))
}

func computeR2(y, pred []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	var meanY float64
	for _, v := range y {
		meanY += v
	}
	meanY /= float64(len(y))

	var ssRes, ssTot float64
	for i := range y {
		ssRes += (y[i] - pred[i]) * (y[i] - pred[i])
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}
	if ssTot == 0 {
		return 0
	}
	return 1 - ssRes/ssTot
}
