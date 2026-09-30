package calibration

// darwinian_risk_vol_band_test.go — issue #2169.
//
// The darwinian calibration wrote darwinian.risk_volatility_threshold from
// agentVols[P75]/sqrt(252) with NO sanity band, while every sibling value in the
// same function is clamped. On 2026-06-13 that produced an annualized-scale
// 1.3551911 (18.6x the daily-scale value) and on 2026-09-30 it produced 0.0729;
// the consumer (DarwinianAgentWeight.RollingVolatility) is a DAILY standard
// deviation. These tests pin the band: in-band values still calibrate exactly as
// before, out-of-band values are NOT written and say why.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
)

// writeDarwinianOutcomes creates a work dir whose session outcomes make each
// agent's forward-return series have the requested per-observation scale, and
// chdirs into it (CalibrateDarwinian resolves its work dir from os.Getwd).
func writeDarwinianOutcomes(t *testing.T, scale float64) {
	t.Helper()
	tmp := t.TempDir()
	sessionDir := filepath.Join(tmp, "data", "state", "sessions", "session-20260930-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	agents := []string{"agent-a", "agent-b", "agent-c", "agent-d", "agent-e", "agent-f"}
	f, err := os.Create(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create outcomes: %v", err)
	}
	enc := json.NewEncoder(f)
	for i, agent := range agents {
		for j := range 5 {
			o := recommendation.RecommendationOutcome{
				AgentID:       agent,
				Symbol:        "2330",
				Side:          "buy",
				Conviction:    5,
				Window:        "1d",
				Hit:           i%2 == 0,
				ForwardReturn: scale * (float64(i+1) + 0.1*float64(j)),
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

// findRiskVol picks the risk_volatility_threshold entry out of a result.
func findRiskVol(t *testing.T, res CalibrationResult) CalibratedParameter {
	t.Helper()
	for _, p := range res.Parameters {
		if p.Path == "darwinian.risk_volatility_threshold" {
			return p
		}
	}
	t.Fatalf("no darwinian.risk_volatility_threshold entry in %+v", res.Parameters)
	return CalibratedParameter{}
}

func newDarwinianCfg() (*config.InferenceEngine, *config.ParametersConfig) {
	cfg := config.DefaultParametersConfig()
	cfg.Darwinian.SharpeMinSampleSize.Value = 2
	// Make the "before" value far from any computed value so the write path is
	// exercised whenever the band admits the computation.
	cfg.Darwinian.RiskVolatilityThreshold.Value = 0.0123
	return config.NewInferenceEngine(cfg), cfg
}

// TestCalibrateDarwinian_RiskVolInBandStillWrites pins ①: a plausible DAILY-scale
// input keeps the previous behaviour (percentile_based write).
func TestCalibrateDarwinian_RiskVolInBandStillWrites(t *testing.T) {
	writeDarwinianOutcomes(t, 0.01) // per-observation σ ≈ 0.0016/day ⇒ inside the band
	ie, cfg := newDarwinianCfg()

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	if entry.Method != "percentile_based" {
		t.Fatalf("method = %q, want percentile_based (the band must not change in-band behaviour)", entry.Method)
	}
	if entry.After == entry.Before {
		t.Fatalf("entry = %+v, want a computed value different from before", entry)
	}
	if got, ok := ie.GetParameter("darwinian_risk_volatility_threshold"); !ok || got != entry.After {
		t.Errorf("effective value = %v (found=%v), want the computed %v", got, ok, entry.After)
	}
}

// TestCalibrateDarwinian_RiskVolOutOfBandDoesNotWrite pins ②: an annualized-scale
// input (the 2026-06-13 failure mode) must NOT be written, and must say why.
func TestCalibrateDarwinian_RiskVolOutOfBandDoesNotWrite(t *testing.T) {
	// Scale the same fixture up ⇒ the computed DAILY volatility lands in the
	// annualized-scale region (≫ 0.25/day) exactly like the 1.3551911 write.
	writeDarwinianOutcomes(t, 5.0)
	ie, cfg := newDarwinianCfg()
	before := cfg.Darwinian.RiskVolatilityThreshold.Value

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	if entry.Method != "skipped_out_of_band" {
		t.Fatalf("method = %q, want skipped_out_of_band (value must not be written)", entry.Method)
	}
	if entry.After != entry.Before {
		t.Errorf("entry.After = %v, want unchanged %v (overlay persists only changed leaves)", entry.After, entry.Before)
	}
	if got, _ := ie.GetParameter("darwinian_risk_volatility_threshold"); got != before {
		t.Errorf("effective value = %v, want the untouched %v", got, before)
	}
	if entry.CalibrationNotes == "" {
		t.Error("an out-of-band skip must explain itself (CalibrationNotes)")
	}
}

// TestCalibrateDarwinian_RiskVolBandBoundaries documents the band edges so a future
// edit cannot widen them silently.
func TestCalibrateDarwinian_RiskVolBandBoundaries(t *testing.T) {
	const (
		minDaily = 0.0005
		maxDaily = 0.25
	)
	// The documented intent is 8%/day "extreme" (config defaults) ⇒ the band must
	// admit it with headroom, and must reject the annualized scale (= daily*sqrt(252)).
	if maxDaily < 0.08*2 {
		t.Errorf("upper bound %.4f gives no headroom over the documented 8%%/day extreme", maxDaily)
	}
	annualizedScale := 1.3551911193487005 // the value actually written on 2026-06-13
	if annualizedScale <= maxDaily {
		t.Errorf("upper bound %.4f would admit the annualized-scale value %.4f", maxDaily, annualizedScale)
	}
	dailyScale := 0.07287290714870037 // the value written on 2026-09-30
	if dailyScale < minDaily || dailyScale > maxDaily {
		t.Errorf("band [%.4f, %.2f] rejects the observed legitimate daily value %.6f", minDaily, maxDaily, dailyScale)
	}
}
