package config

import (
	"fmt"
	"math"
	"sort"
)

const (
	gpJitter     = 1e-6
	eiMinExploit = 0.01
)

type gaussianProcess struct {
	xTrain  [][]float64
	yTrain  []float64
	kernel  squaredExponential
	noise   float64
	lFactor [][]float64
	alpha   []float64
	trained bool
}

type squaredExponential struct {
	lengthScale float64
	outputScale float64
}

type point struct {
	X     []float64
	Value float64
}

func newGP(lengthScale, outputScale, noise float64) *gaussianProcess {
	return &gaussianProcess{
		kernel: squaredExponential{lengthScale: lengthScale, outputScale: outputScale},
		noise:  noise,
	}
}

func (gp *gaussianProcess) fit(x [][]float64, y []float64) {
	n := len(x)
	if n == 0 {
		return
	}
	gp.xTrain = make([][]float64, n)
	gp.yTrain = make([]float64, n)
	copy(gp.xTrain, x)
	copy(gp.yTrain, y)

	k := make([][]float64, n)
	for i := range n {
		k[i] = make([]float64, n)
		for j := range n {
			// gosec G602: false positive — kernelMatrix iterates over len(x[i]) (feature dim), not n.
			k[i][j] = gp.kernelMatrix(x[i], x[j]) //nolint:gosec
			if i == j {
				k[i][j] += gp.noise*gp.noise + gpJitter
			}
		}
	}

	gp.lFactor = cholesky(k)
	gp.alpha = solveTriangular(gp.lFactor, y)
	gp.trained = true
}

func (gp *gaussianProcess) predict(xStar []float64) (mean, std float64) {
	if !gp.trained || len(gp.xTrain) == 0 {
		return 0, gp.kernel.outputScale
	}
	n := len(gp.xTrain)
	kStar := make([]float64, n)
	for i := range gp.xTrain {
		kStar[i] = gp.kernelMatrix(gp.xTrain[i], xStar)
	}

	mean = 0
	for i := range n {
		mean += kStar[i] * gp.alpha[i]
	}

	v := forwardSubstitution(gp.lFactor, kStar)
	var variance float64
	variance = gp.kernelMatrix(xStar, xStar) - dotProduct(v, v) + gp.noise*gp.noise
	if variance < gpJitter {
		variance = gpJitter
	}
	std = math.Sqrt(variance)
	return
}

func (gp *gaussianProcess) kernelMatrix(a, b []float64) float64 {
	sqDist := 0.0
	for i := range a {
		d := (a[i] - b[i]) / gp.kernel.lengthScale
		sqDist += d * d
	}
	return gp.kernel.outputScale * gp.kernel.outputScale * math.Exp(-0.5*sqDist)
}

type BayesianOptimizer struct {
	bounds        [][2]float64
	observations  []point
	bestY         float64
	bestX         []float64
	evaluator     func(x []float64) (float64, error)
	gp            *gaussianProcess
	initialPoints int
	iterations    int
	rngState      uint32
}

type OptimizerConfig struct {
	InitialPoints int
	Iterations    int
	LengthScale   float64
	OutputScale   float64
	Noise         float64
}

func DefaultOptimizerConfig() OptimizerConfig {
	return OptimizerConfig{
		InitialPoints: 10,
		Iterations:    20,
		LengthScale:   0.5,
		OutputScale:   1.0,
		Noise:         0.01,
	}
}

// NewBayesianOptimizer creates a standalone optimizer for multi-dimensional search.
func NewBayesianOptimizer(bounds [][2]float64, evaluator func(x []float64) (float64, error), cfg OptimizerConfig) *BayesianOptimizer {
	return &BayesianOptimizer{
		bounds:        bounds,
		evaluator:     evaluator,
		gp:            newGP(cfg.LengthScale, cfg.OutputScale, cfg.Noise),
		initialPoints: cfg.InitialPoints,
		iterations:    cfg.Iterations,
	}
}

func (opt *BayesianOptimizer) Optimize() (OptimizeResult, error) {
	dim := len(opt.bounds)
	opt.rngState = 42
	opt.observations = make([]point, 0, opt.initialPoints+opt.iterations)
	opt.bestY = math.Inf(-1)

	for i := 0; i < opt.initialPoints; i++ {
		x := make([]float64, dim)
		for d := range dim {
			t := float64(i+1) / float64(opt.initialPoints+1)
			x[d] = opt.bounds[d][0] + t*(opt.bounds[d][1]-opt.bounds[d][0])
		}
		y, err := opt.evaluator(x)
		if err != nil {
			continue
		}
		opt.record(x, y)
	}

	for i := 0; i < opt.iterations; i++ {
		opt.fitGP()
		next := opt.proposeNext()
		y, err := opt.evaluator(next)
		if err != nil {
			continue
		}
		opt.record(next, y)
	}

	return opt.result(), nil
}

func (opt *BayesianOptimizer) record(x []float64, y float64) {
	pt := point{X: make([]float64, len(x)), Value: y}
	copy(pt.X, x)
	opt.observations = append(opt.observations, pt)
	if y > opt.bestY {
		opt.bestY = y
		opt.bestX = make([]float64, len(x))
		copy(opt.bestX, x)
	}
}

func (opt *BayesianOptimizer) fitGP() {
	if len(opt.observations) < 10 {
		return
	}
	n := len(opt.observations)
	x := make([][]float64, n)
	y := make([]float64, n)
	for i, obs := range opt.observations {
		x[i] = obs.X
		y[i] = obs.Value
	}

	opt.gp = newGP(opt.gp.kernel.lengthScale, stdFromBounds(opt.bounds)*0.5, 0.01)
	opt.gp.fit(x, y)
}

func stdFromBounds(bounds [][2]float64) float64 {
	sum := 0.0
	for _, b := range bounds {
		sum += (b[1] - b[0]) * (b[1] - b[0])
	}
	return math.Sqrt(sum) / 2.0
}

func (opt *BayesianOptimizer) proposeNext() []float64 {
	if len(opt.observations) < 10 {
		return opt.randomPoint()
	}
	dim := len(opt.bounds)
	nCandidates := 2000
	bestEI := math.Inf(-1)
	bestX := make([]float64, dim)

	for range nCandidates {
		x := make([]float64, dim)
		for d := range dim {
			x[d] = opt.bounds[d][0] + opt.randFloat()*(opt.bounds[d][1]-opt.bounds[d][0])
		}
		mean, std := opt.gp.predict(x)
		ei := expectedImprovement(mean, std, opt.bestY)
		if ei > bestEI {
			bestEI = ei
			copy(bestX, x)
		}
	}
	return bestX
}

func expectedImprovement(mu, sigma, bestF float64) float64 {
	if sigma < gpJitter {
		return 0
	}
	z := (mu - bestF - eiMinExploit) / sigma
	return (mu-bestF-eiMinExploit)*normCDF(z) + sigma*normPDF(z)
}

func normCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

func normPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}

func (opt *BayesianOptimizer) randomPoint() []float64 {
	dim := len(opt.bounds)
	x := make([]float64, dim)
	for d := range dim {
		x[d] = opt.bounds[d][0] + opt.randFloat()*(opt.bounds[d][1]-opt.bounds[d][0])
	}
	return x
}

func (opt *BayesianOptimizer) randFloat() float64 {
	opt.rngState = opt.rngState*1664525 + 1013904223
	return float64(opt.rngState) / float64(math.MaxUint32)
}

type OptimizeResult struct {
	BestX        []float64
	BestScore    float64
	Observations int
	ParamValues  map[string]float64
}

func (opt *BayesianOptimizer) result() OptimizeResult {
	r := OptimizeResult{
		BestScore:    opt.bestY,
		Observations: len(opt.observations),
		ParamValues:  make(map[string]float64),
	}
	if opt.bestX != nil {
		r.BestX = make([]float64, len(opt.bestX))
		copy(r.BestX, opt.bestX)
	}
	return r
}

func (ie *InferenceEngine) OptimizeBayesian(paramNames []string, evaluator func(cfg *ParametersConfig) (float64, error), cfg OptimizerConfig) (OptimizeResult, error) {
	n := len(paramNames)
	if n == 0 {
		return OptimizeResult{}, fmt.Errorf("no parameters to optimize")
	}
	if cfg.InitialPoints <= 0 {
		cfg = DefaultOptimizerConfig()
	}

	bounds := make([][2]float64, n)
	for i, name := range paramNames {
		val, ok := ie.GetParameter(name)
		if !ok {
			return OptimizeResult{}, fmt.Errorf("unknown parameter: %s", name)
		}
		if val == 0 {
			bounds[i] = [2]float64{0.01, 1.0}
		} else {
			bounds[i] = [2]float64{val * 0.3, val * 3.0}
		}
	}

	wrapped := func(x []float64) (float64, error) {
		testCfg := ie.cloneParams()
		for i, name := range paramNames {
			if err := ie.setParameterOnConfig(testCfg, name, x[i]); err != nil {
				return 0, err
			}
		}
		return evaluator(testCfg)
	}

	opt := &BayesianOptimizer{
		bounds:        bounds,
		evaluator:     wrapped,
		gp:            newGP(cfg.LengthScale, cfg.OutputScale, cfg.Noise),
		initialPoints: cfg.InitialPoints,
		iterations:    cfg.Iterations,
	}
	result, err := opt.Optimize()
	if err != nil {
		return result, err
	}

	result.ParamValues = make(map[string]float64)
	for i, name := range paramNames {
		result.ParamValues[name] = result.BestX[i]
	}
	return result, nil
}

func dotProduct(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func cholesky(a [][]float64) [][]float64 {
	n := len(a)
	l := make([][]float64, n)
	for i := range n {
		l[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= l[i][k] * l[j][k]
			}
			if i == j {
				if sum <= 0 {
					sum = gpJitter
				}
				l[i][j] = math.Sqrt(sum)
			} else {
				l[i][j] = sum / l[j][j]
			}
		}
	}
	return l
}

func forwardSubstitution(l [][]float64, b []float64) []float64 {
	n := len(l)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := b[i]
		for j := 0; j < i; j++ {
			sum -= l[i][j] * y[j]
		}
		y[i] = sum / l[i][i]
	}
	return y
}

func solveTriangular(l [][]float64, b []float64) []float64 {
	y := forwardSubstitution(l, b)
	n := len(l)
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := y[i]
		for j := i + 1; j < n; j++ {
			sum -= l[j][i] * x[j]
		}
		x[i] = sum / l[i][i]
	}
	return x
}

func findMax(scores []float64) (maxIdx int, maxVal float64) {
	maxVal = math.Inf(-1)
	for i, v := range scores {
		if v > maxVal {
			maxVal = v
			maxIdx = i
		}
	}
	return
}

type SweepRecommendation struct {
	Action       string
	CurrentValue float64
	BestValue    float64
	Improvement  float64
	Confidence   string
}

func (ie *InferenceEngine) RecommendFromSweep(name string, values []float64, scores []float64) SweepRecommendation {
	current, ok := ie.GetParameter(name)
	if !ok {
		return SweepRecommendation{Action: "unknown_parameter"}
	}
	bestIdx, bestScore := findMax(scores)
	bestVal := values[bestIdx]
	improvement := bestScore

	sort.Float64s(scores)
	median := 0.0
	n := len(scores)
	if n > 0 {
		if n%2 == 0 {
			median = (scores[n/2-1] + scores[n/2]) / 2.0
		} else {
			median = scores[n/2]
		}
	}

	confidence := "low"
	if n >= 10 && improvement > median*1.5 {
		confidence = "high"
	} else if n >= 5 && improvement > median*1.2 {
		confidence = "medium"
	}

	action := "keep_current"
	if math.Abs(bestVal-current) > 1e-9 && improvement > 0 {
		if bestVal > current {
			action = "increase"
		} else {
			action = "decrease"
		}
	}

	return SweepRecommendation{
		Action:       action,
		CurrentValue: current,
		BestValue:    bestVal,
		Improvement:  improvement,
		Confidence:   confidence,
	}
}
