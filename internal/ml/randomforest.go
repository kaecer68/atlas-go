package ml

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// RandomForest implements a random forest regression model.
// It fits an ensemble of CART decision trees on bootstrap samples
// with random subspace feature selection at each split.
type RandomForest struct {
	// NTrees is the number of trees in the forest (default 100).
	NTrees int
	// MaxDepth limits tree depth. 0 = use default of 10.
	MaxDepth int
	// MinSamplesSplit is the minimum samples required to split a node (default 5).
	MinSamplesSplit int
	// MaxFeatures controls feature sampling per split: "sqrt" or "all".
	MaxFeatures string
	// Seed for reproducibility.
	Seed uint64

	// internal state
	trees  []*decisionTree
	nFeats int
	fitted bool
}

// decisionTree is a single CART regression tree.
type decisionTree struct {
	root *treeNode
}

// treeNode is a node in the regression tree.
type treeNode struct {
	isLeaf       bool
	prediction   float64
	splitFeature int
	splitValue   float64
	left         *treeNode
	right        *treeNode
}

// NewRandomForest returns a new RandomForest with sensible defaults.
func NewRandomForest() *RandomForest {
	return &RandomForest{
		NTrees:          100,
		MaxDepth:        10,
		MinSamplesSplit: 5,
		MaxFeatures:     "sqrt",
		Seed:            42,
	}
}

// maxFeaturesPerSplit returns the number of features to consider at each split.
func maxFeaturesPerSplit(nFeatures int, maxFeat string) int {
	switch maxFeat {
	case "all":
		return nFeatures
	case "sqrt":
		nf := max(int(math.Floor(math.Sqrt(float64(nFeatures)))), 1)
		return nf
	default:
		return nFeatures
	}
}

// bootstrapSample draws n samples with replacement from X and y.
// Returns bootstrap X, y, and OOB indices (not selected).
func bootstrapSample(X [][]float64, y []float64, rng *rand.Rand) ([][]float64, []float64, []int) {
	n := len(X)
	Xb := make([][]float64, n)
	yb := make([]float64, n)
	selected := make([]bool, n)

	for i := range n {
		idx := rng.IntN(n)
		Xb[i] = X[idx]
		yb[i] = y[idx]
		selected[idx] = true
	}

	var oob []int
	for i := range n {
		if !selected[i] {
			oob = append(oob, i)
		}
	}
	return Xb, yb, oob
}

// uniqueSorted returns sorted unique values from a slice.
func uniqueSorted(vals []float64) []float64 {
	if len(vals) == 0 {
		return nil
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	uniq := make([]float64, 0, len(sorted))
	for _, v := range sorted {
		if len(uniq) == 0 || v != uniq[len(uniq)-1] {
			uniq = append(uniq, v)
		}
	}
	return uniq
}

// mse computes mean squared error.
func mseOf(y []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range y {
		mean += v
	}
	mean /= float64(len(y))
	sum := 0.0
	for _, v := range y {
		d := v - mean
		sum += d * d
	}
	return sum
}

// meanOf computes the mean of a slice.
func meanOf(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// buildTree recursively builds a CART regression tree.
func buildTree(X [][]float64, y []float64, depth int, maxDepth int, minSamplesSplit int, maxFeats int, rng *rand.Rand) *treeNode {
	n := len(y)

	// Stopping criteria: leaf node.
	if n < minSamplesSplit || depth >= maxDepth || n <= 1 {
		return &treeNode{isLeaf: true, prediction: meanOf(y)}
	}

	// Check if all y values are the same.
	allSame := true
	for i := 1; i < n; i++ {
		if y[i] != y[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return &treeNode{isLeaf: true, prediction: y[0]}
	}

	nFeatures := len(X[0])
	parentMSE := mseOf(y)

	// Select random subset of features to consider.
	featurePool := make([]int, nFeatures)
	for i := range nFeatures {
		featurePool[i] = i
	}
	// Shuffle and take first maxFeats.
	rng.Shuffle(nFeatures, func(i, j int) {
		featurePool[i], featurePool[j] = featurePool[j], featurePool[i]
	})
	featSet := featurePool[:min(maxFeats, nFeatures)]

	bestMSE := parentMSE
	bestFeature := -1
	bestThreshold := 0.0
	bestLeftY := []float64(nil)
	bestRightY := []float64(nil)

	for _, feat := range featSet {
		// Extract feature values.
		vals := make([]float64, n)
		for i := range n {
			vals[i] = X[i][feat]
		}
		uniq := uniqueSorted(vals)
		if len(uniq) <= 1 {
			continue
		}

		// Test midpoints between consecutive unique values.
		for k := 0; k < len(uniq)-1; k++ {
			threshold := (uniq[k] + uniq[k+1]) / 2.0

			// Split and compute MSE.
			var leftY, rightY []float64
			for i := range n {
				if vals[i] <= threshold {
					leftY = append(leftY, y[i])
				} else {
					rightY = append(rightY, y[i])
				}
			}

			// Skip if either side is too small.
			if len(leftY) < minSamplesSplit || len(rightY) < minSamplesSplit {
				continue
			}

			// Weighted MSE.
			leftMSE := mseOf(leftY)
			rightMSE := mseOf(rightY)
			wl := float64(len(leftY)) / float64(n)
			wr := float64(len(rightY)) / float64(n)
			splitMSE := wl*leftMSE + wr*rightMSE

			if splitMSE < bestMSE {
				bestMSE = splitMSE
				bestFeature = feat
				bestThreshold = threshold
				bestLeftY = leftY
				bestRightY = rightY
			}
		}
	}

	// If no improvement, create leaf.
	if bestFeature == -1 {
		return &treeNode{isLeaf: true, prediction: meanOf(y)}
	}

	// Split data and recurse.
	var leftX, rightX [][]float64
	for i := range n {
		if X[i][bestFeature] <= bestThreshold {
			leftX = append(leftX, X[i])
		} else {
			rightX = append(rightX, X[i])
		}
	}

	node := &treeNode{
		splitFeature: bestFeature,
		splitValue:   bestThreshold,
	}
	node.left = buildTree(leftX, bestLeftY, depth+1, maxDepth, minSamplesSplit, maxFeats, rng)
	node.right = buildTree(rightX, bestRightY, depth+1, maxDepth, minSamplesSplit, maxFeats, rng)

	return node
}

// predictSample traverses the tree to predict a single sample.
func predictSample(node *treeNode, x []float64) float64 {
	if node.isLeaf {
		return node.prediction
	}
	if x[node.splitFeature] <= node.splitValue {
		return predictSample(node.left, x)
	}
	return predictSample(node.right, x)
}

// Fit trains the random forest on the given data.
func (rf *RandomForest) Fit(X [][]float64, y []float64) error {
	nSamples := len(X)
	if nSamples == 0 {
		return fmt.Errorf("randomforest: Fit called with empty X")
	}
	if len(y) != nSamples {
		return fmt.Errorf("randomforest: X and y have mismatched lengths: %d vs %d", nSamples, len(y))
	}
	nFeatures := len(X[0])
	if nFeatures == 0 {
		return fmt.Errorf("randomforest: Fit called with zero features")
	}
	for _, row := range X {
		if len(row) != nFeatures {
			return fmt.Errorf("randomforest: inconsistent feature counts in X")
		}
	}

	maxDepth := rf.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}
	minSplit := max(rf.MinSamplesSplit, 2)
	maxFeats := maxFeaturesPerSplit(nFeatures, rf.MaxFeatures)

	rf.trees = make([]*decisionTree, rf.NTrees)
	rf.nFeats = nFeatures

	for t := 0; t < rf.NTrees; t++ {
		// Each tree gets its own RNG for reproducibility.
		treeRNG := rand.New(rand.NewPCG(rf.Seed+uint64(t), 0))
		Xb, yb, _ := bootstrapSample(X, y, treeRNG)
		root := buildTree(Xb, yb, 0, maxDepth, minSplit, maxFeats, treeRNG)
		rf.trees[t] = &decisionTree{root: root}
	}

	rf.fitted = true
	return nil
}

// Predict computes predictions by averaging all tree predictions.
func (rf *RandomForest) Predict(X [][]float64) ([]float64, error) {
	if !rf.fitted {
		return nil, fmt.Errorf("randomforest: Predict called before Fit")
	}
	nSamples := len(X)
	if nSamples == 0 {
		return nil, fmt.Errorf("randomforest: Predict called with empty X")
	}
	nFeatures := len(X[0])
	if nFeatures != rf.nFeats {
		return nil, fmt.Errorf("randomforest: dimension mismatch: expected %d features, got %d", rf.nFeats, nFeatures)
	}
	for _, row := range X {
		if len(row) != nFeatures {
			return nil, fmt.Errorf("randomforest: inconsistent feature counts in X")
		}
	}

	pred := make([]float64, nSamples)
	for i := 0; i < nSamples; i++ {
		sum := 0.0
		for _, tree := range rf.trees {
			sum += predictSample(tree.root, X[i])
		}
		pred[i] = sum / float64(len(rf.trees))
	}

	return pred, nil
}
