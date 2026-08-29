package risk

import (
	"math"
	"math/rand"
	"testing"
)

// generateNormal creates n samples from a normal distribution with given mean and stddev.
func generateNormal(n int, mean, stddev float64, rng *rand.Rand) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.NormFloat64()*stddev + mean
	}
	return out
}

// TestSpillover_IdenticalSeries verifies that when all return series are
// independent AR(1) processes with the same parameters (identical in distribution
// but uncorrelated), the spillover between distinct variables is close to zero.
//
// Each variable follows the same AR(1) law with independent noise, so no
// variable carries predictive information about any other.
func TestSpillover_IdenticalSeries(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	n := 200
	sd := 0.01

	// Three independent AR(1) series with identical parameters.
	x := make([]float64, n)
	y := make([]float64, n)
	z := make([]float64, n)
	x[0] = rng.NormFloat64() * sd
	y[0] = rng.NormFloat64() * sd
	z[0] = rng.NormFloat64() * sd
	for i := 1; i < n; i++ {
		x[i] = 0.5*x[i-1] + rng.NormFloat64()*sd
		y[i] = 0.5*y[i-1] + rng.NormFloat64()*sd
		z[i] = 0.5*z[i-1] + rng.NormFloat64()*sd
	}

	vars := []string{"X", "Y", "Z"}
	result, err := ComputeSpillover([][]float64{x, y, z}, vars, 10)
	if err != nil {
		t.Fatalf("ComputeSpillover failed: %v", err)
	}

	// Since series are independent, cross-variable spillover should be low.
	// Each variable should explain most of its own forecast error variance.
	for _, vi := range vars {
		ownShare := result.FromTo[vi][vi]
		if ownShare < 60.0 {
			t.Errorf("%s own-variance share too low: %.2f%%, want > 60%% for independent series", vi, ownShare)
		}
	}

	if result.Total > 30.0 {
		t.Errorf("total spillover too high for independent series: got %.2f%%, want < 30%%", result.Total)
	}
}

// TestSpillover_LeaderFollower verifies directional spillover detection:
// series[0] is a leader (Granger-causes series[1]), so spillover 0→1 should
// be substantially higher than 1→0.
func TestSpillover_LeaderFollower(t *testing.T) {
	rng := rand.New(rand.NewSource(777))
	n := 200

	// Leader: AR(1) process
	leader := make([]float64, n)
	// Follower: depends on lagged leader + own AR(1) term
	follower := make([]float64, n)
	// Independent: pure noise (control)
	independent := make([]float64, n)

	leader[0] = rng.NormFloat64() * 0.01
	follower[0] = rng.NormFloat64() * 0.01
	independent[0] = rng.NormFloat64() * 0.01

	for t := 1; t < n; t++ {
		leader[t] = 0.7*leader[t-1] + rng.NormFloat64()*0.005
		follower[t] = 0.5*leader[t-1] + 0.3*follower[t-1] + rng.NormFloat64()*0.005
		independent[t] = 0.1*independent[t-1] + rng.NormFloat64()*0.01
	}

	returns := [][]float64{leader, follower, independent}
	vars := []string{"LDR", "FLW", "IND"}
	result, err := ComputeSpillover(returns, vars, 10)
	if err != nil {
		t.Fatalf("ComputeSpillover failed: %v", err)
	}

	ldrToFlw := result.FromTo["LDR"]["FLW"]
	flwToLdr := result.FromTo["FLW"]["LDR"]

	t.Logf("LDR → FLW = %.2f%%, FLW → LDR = %.2f%%", ldrToFlw, flwToLdr)

	if ldrToFlw <= flwToLdr {
		t.Errorf("expected leader→follower spillover > follower→leader: got LDR→FLW=%.2f%%, FLW→LDR=%.2f%%", ldrToFlw, flwToLdr)
	}
}

// TestSpillover_TotalSum verifies normalization integrity:
// for every variable, FromOthers[v] + own_variance_share ≈ 100,
// and the total spillover index equals the average of FromOthers.
func TestSpillover_TotalSum(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	n := 150

	// Three AR(1) series with cross-correlated noise.
	a := make([]float64, n)
	b := make([]float64, n)
	c := make([]float64, n)
	a[0] = rng.NormFloat64() * 0.01
	b[0] = rng.NormFloat64() * 0.01
	c[0] = rng.NormFloat64() * 0.01

	for t := 1; t < n; t++ {
		noise := rng.NormFloat64() * 0.005
		a[t] = 0.6*a[t-1] + 0.2*b[t-1] + noise
		b[t] = 0.2*a[t-1] + 0.5*b[t-1] + 0.1*c[t-1] + rng.NormFloat64()*0.005
		c[t] = 0.1*b[t-1] + 0.4*c[t-1] + rng.NormFloat64()*0.005
	}

	returns := [][]float64{a, b, c}
	vars := []string{"A", "B", "C"}
	result, err := ComputeSpillover(returns, vars, 10)
	if err != nil {
		t.Fatalf("ComputeSpillover failed: %v", err)
	}

	// Each row should sum to ~100: own_share + FromOthers ≈ 100
	for _, v := range vars {
		own := result.FromTo[v][v]
		from := result.FromOthers[v]
		rowSum := own + from
		if math.Abs(rowSum-100.0) > 0.01 {
			t.Errorf("row sum for %s: own=%.4f + from_others=%.4f = %.4f, want ≈100", v, own, from, rowSum)
		}
	}

	// Total spillover index = average of FromOthers
	avgFromOthers := (result.FromOthers["A"] + result.FromOthers["B"] + result.FromOthers["C"]) / 3.0
	if math.Abs(result.Total-avgFromOthers) > 0.01 {
		t.Errorf("Total spillover mismatch: Total=%.4f, avg(FromOthers)=%.4f", result.Total, avgFromOthers)
	}
}

// TestSpillover_NumericalStability verifies that near-singular input matrices
// do not cause panics or infinite loops. The implementation must handle
// ill-conditioned covariance gracefully.
func TestSpillover_NumericalStability(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	n := 100

	// Construct two highly correlated series → near-singular covariance.
	common := generateNormal(n, 0.0, 0.01, rng)
	a := make([]float64, n)
	b := make([]float64, n)
	c := make([]float64, n)
	for i := range n {
		// a and b share almost all variance from "common".
		a[i] = common[i] + rng.NormFloat64()*1e-6
		b[i] = common[i] + rng.NormFloat64()*1e-6
		c[i] = rng.NormFloat64() * 0.01
	}

	returns := [][]float64{a, b, c}
	vars := []string{"P", "Q", "R"}

	// Must not panic.
	result, err := ComputeSpillover(returns, vars, 10)
	if err != nil {
		t.Fatalf("ComputeSpillover failed on near-singular data: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Total < 0 || result.Total > 100 {
		t.Errorf("Total spillover out of [0,100]: %.4f", result.Total)
	}

	// All maps should be populated.
	if len(result.FromTo) != 3 {
		t.Errorf("expected 3 entries in FromTo, got %d", len(result.FromTo))
	}
	if len(result.FromOthers) != 3 {
		t.Errorf("expected 3 entries in FromOthers, got %d", len(result.FromOthers))
	}
	if len(result.ToOthers) != 3 {
		t.Errorf("expected 3 entries in ToOthers, got %d", len(result.ToOthers))
	}
	if len(result.NetSpillover) != 3 {
		t.Errorf("expected 3 entries in NetSpillover, got %d", len(result.NetSpillover))
	}
}

// TestSpillover_InputValidation covers edge cases for input validation.
func TestSpillover_InputValidation(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := ComputeSpillover(nil, nil, 10)
		if err == nil {
			t.Error("expected error for nil input")
		}
	})

	t.Run("mismatched counts", func(t *testing.T) {
		returns := [][]float64{{1.0, 2.0}}
		_, err := ComputeSpillover(returns, []string{"A", "B"}, 10)
		if err == nil {
			t.Error("expected error for mismatched returns/vars count")
		}
	})

	t.Run("short sample", func(t *testing.T) {
		returns := [][]float64{make([]float64, 10)}
		_, err := ComputeSpillover(returns, []string{"A"}, 10)
		if err == nil {
			t.Error("expected error for T < 30")
		}
	})

	t.Run("zero variance", func(t *testing.T) {
		zero := make([]float64, 100)
		_, err := ComputeSpillover([][]float64{zero}, []string{"A"}, 10)
		if err == nil {
			t.Error("expected error for truly zero-variance series")
		}
	})
}

func TestNewSpilloverIndex(t *testing.T) {
	vars := []string{"SPX", "NDX", "TAIEX"}
	idx := NewSpilloverIndex(vars)
	if idx == nil {
		t.Fatal("NewSpilloverIndex returned nil")
	}
	if idx.horizon != 10 {
		t.Errorf("horizon = %d, want 10", idx.horizon)
	}
	if idx.varLags != 2 {
		t.Errorf("varLags = %d, want 2", idx.varLags)
	}
	if len(idx.variables) != 3 || idx.variables[0] != "SPX" {
		t.Errorf("variables = %v, want %v", idx.variables, vars)
	}
}
