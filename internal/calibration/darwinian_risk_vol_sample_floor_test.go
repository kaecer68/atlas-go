package calibration

// darwinian_risk_vol_sample_floor_test.go — FU-20260930-09.
//
// The risk_volatility_threshold percentile is taken at nearest-rank index
// int(0.75*n) of the sorted agent vols, and the enclosing sample floor for the
// darwinian section is len(agentSharpes) >= 3. For n <= 4 that index IS the
// largest sample, so one agent's volatility alone decided the written
// threshold (measured on a four-agent fixture: moving ONE agent's daily
// volatility 0.008 -> 0.050 moved the written value 0.0082 -> 0.0513, and the
// value stayed inside the band, so the band does not cover the case).
//
// These tests pin the derived floor: the estimated rank must have at least one
// observation above it (n >= 5 for the int(0.75*n) convention). Below the floor
// the value is NOT written and the skip is recorded. Above the floor the
// computation is bit-for-bit what it always was (asserted exactly, so a future
// edit to the estimator cannot slip through as "still writes").

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
)

// writeDarwinianVolFixture writes one agent per entry of dailyVols whose
// per-observation sample standard deviation equals that entry (alternating +-d
// around a positive mean, 20 observations each), then chdirs into the fixture
// dir so CalibrateDarwinian reads it.
func writeDarwinianVolFixture(t *testing.T, dailyVols []float64) {
	t.Helper()
	tmp := t.TempDir()
	sessionDir := filepath.Join(tmp, "data", "state", "sessions", "session-20260930-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	f, err := os.Create(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create outcomes: %v", err)
	}
	enc := json.NewEncoder(f)
	for i, d := range dailyVols {
		agent := fmt.Sprintf("agent-%c", 'a'+i)
		for j := range 20 {
			sign := 1.0
			if j%2 == 1 {
				sign = -1
			}
			o := recommendation.RecommendationOutcome{
				AgentID:       agent,
				Symbol:        "2330",
				Side:          "buy",
				Conviction:    5,
				Window:        "1d",
				Hit:           i%2 == 0,
				ForwardReturn: 0.001 + sign*d,
			}
			if err := enc.Encode(o); err != nil {
				t.Fatalf("encode outcome: %v", err)
			}
		}
	}
	_ = f.Close()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// outlierFixtureVols is the four-agent sample used for the reproduction: four
// comparable agents plus one ten-times-more-volatile agent. Its sample standard
// deviation is d*sqrt(20/19), so the outlier's DAILY volatility is
// 0.05*sqrt(20/19) = 0.051298917604257706 — the value the unfixed code wrote.
func outlierFixtureVols() []float64 {
	return []float64{0.002, 0.004, 0.006, 0.05}
}

// TestCalibrateDarwinian_RiskVolSmallSampleDoesNotWrite is the regression for
// FU-20260930-09: a four-agent sample must NOT publish the most volatile
// agent's own volatility as the risk threshold.
func TestCalibrateDarwinian_RiskVolSmallSampleDoesNotWrite(t *testing.T) {
	const outlierDailyVol = 0.051298917604257706 // = 0.05*sqrt(20/19)

	writeDarwinianVolFixture(t, outlierFixtureVols())
	ie, cfg := newDarwinianCfg()
	before := cfg.Darwinian.RiskVolatilityThreshold.Value

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)

	if entry.Method != "skipped_insufficient_sample" {
		t.Fatalf("method = %q, want skipped_insufficient_sample (a percentile equal to the sample maximum must not be published)", entry.Method)
	}
	if entry.After != entry.Before {
		t.Errorf("entry.After = %v, want the unchanged %v (overlay persists only changed leaves)", entry.After, entry.Before)
	}
	if math.Abs(entry.After-outlierDailyVol) < 1e-9 {
		t.Errorf("entry.After = %v is the single most volatile agent's volatility; the outlier still decides the threshold", entry.After)
	}
	if entry.SampleSize != 4 {
		t.Errorf("SampleSize = %d, want 4", entry.SampleSize)
	}
	if entry.CalibrationNotes == "" {
		t.Error("an insufficient-sample skip must explain itself (CalibrationNotes)")
	}
	if got, _ := ie.GetParameter("darwinian_risk_volatility_threshold"); got != before {
		t.Errorf("effective value = %v, want the untouched %v", got, before)
	}
}

// TestCalibrateDarwinian_RiskVolAdequateSampleUnchanged pins the exact value the
// sufficient-sample path produced before the sample floor was added, so the
// floor cannot silently move a value that real samples do produce.
func TestCalibrateDarwinian_RiskVolAdequateSampleUnchanged(t *testing.T) {
	// The six-agent fixture from darwinian_risk_vol_band_test.go. The pin must
	// catch a *method* change (a different quantile, a re-scaled input), not the
	// last-bit rounding difference between platforms: the same code computes
	// 0.001581138830084191 (float64 bits 4564934538734833853) here on
	// darwin/arm64 and 0.0015811388300841912 (bits 4564934538734833854) in CI on
	// linux/amd64 — adjacent float64s, 1 ULP = 1.3e-16 relative apart. Exact
	// equality therefore tests the platform, not the calibration. 1e-12
	// relative is ~4 orders of magnitude above that ULP gap and still ~1e-9 of
	// the 6.25x shift that the outlier sensitivity this file fixes produces, so
	// it cannot hide a real change.
	const preFloorValue = 0.001581138830084191
	const preFloorRelTol = 1e-12

	writeDarwinianOutcomes(t, 0.01)
	ie, cfg := newDarwinianCfg()

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	if entry.Method != "percentile_based" {
		t.Fatalf("method = %q, want percentile_based (the floor must not touch adequate samples)", entry.Method)
	}
	if math.Abs(entry.After-preFloorValue) > preFloorRelTol*math.Abs(preFloorValue) {
		t.Errorf("After = %.17g, want the unchanged %.17g within %.0e relative (relative tolerance: platform ULP noise, not method drift)",
			entry.After, preFloorValue, preFloorRelTol)
	}
}

// TestCalibrateDarwinian_RiskVolSampleFloorBoundary documents where the floor
// falls and why: the nearest-rank upper quartile int(0.75*n) equals the sample
// maximum for n <= 4, so the smallest supported sample is 5. Same outlier shape
// in every row — only the agent count changes.
func TestCalibrateDarwinian_RiskVolSampleFloorBoundary(t *testing.T) {
	cases := []struct {
		agents int
		want   string
	}{
		{3, "skipped_insufficient_sample"},
		{4, "skipped_insufficient_sample"},
		{5, "percentile_based"},
		{6, "percentile_based"},
	}
	for _, tc := range cases {
		vols := make([]float64, tc.agents)
		for i := range vols {
			vols[i] = 0.002 * float64(i+1)
		}
		vols[tc.agents-1] = 0.05 // one outlier in every row

		writeDarwinianVolFixture(t, vols)
		ie, cfg := newDarwinianCfg()
		res := CalibrateDarwinian(ie, 100, cfg)
		entry := findRiskVol(t, res)

		rank := int(float64(tc.agents) * 0.75)
		t.Logf("agents=%d rank=%d max_rank=%d method=%s after=%.6f", tc.agents, rank, tc.agents-1, entry.Method, entry.After)
		if entry.Method != tc.want {
			t.Errorf("agents=%d: method = %q, want %q", tc.agents, entry.Method, tc.want)
		}
		if tc.want == "percentile_based" && math.Abs(entry.After-0.051298917604257706) < 1e-9 {
			t.Errorf("agents=%d: after = %v is the outlier's own volatility; the outlier decides the threshold", tc.agents, entry.After)
		}
	}
}
