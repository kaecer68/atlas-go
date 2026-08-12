package retail

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestComputeFinal_AllInputsZero(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	result := c.ComputeFinal(RSITwInput{})

	if result.Score < -1.0 || result.Score > 1.0 {
		t.Errorf("score %f out of range [-1, 1]", result.Score)
	}
	if len(result.SubIndicators) == 0 {
		t.Error("expected sub-indicators even with zero input")
	}
	// With all zero inputs, most sub-indicators should be fallback
	fallbackCount := 0
	for _, si := range result.SubIndicators {
		if si.IsFallback {
			fallbackCount++
		}
	}
	if fallbackCount == 0 {
		t.Error("expected at least some fallback indicators with zero inputs")
	}
}

func TestComputeFinal_HappyPath(t *testing.T) {
	c := NewCalculator()
	// Build up some history for A1 z-score computation
	c.UpdateHistory(RSITwInput{MarginBalance: 5000})
	c.UpdateHistory(RSITwInput{MarginBalance: 5100})
	c.UpdateHistory(RSITwInput{MarginBalance: 5200})

	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	input := RSITwInput{
		MarginBalance:      5300,
		MarginPercentile:   0.7,
		DayTrading:         &DayTradingStats{Volume: 500, VolumeRatio: 0.3},
		VIXLevel:           22,
		ForeignInvestorNet: 2_000_000_000,
		DomesticFundNet:    500_000_000,
		GeopoliticalRisk:   0.6,
		CreditTightening:   false,
		PutCallRatio:       1.2,
		OddLotImbalance:    0.15,
		RetailFuturesPct:   15,
		ETFNetSubscription: 500_000_000,
	}

	result := c.ComputeFinal(input)

	if result.Score < -1.0 || result.Score > 1.0 {
		t.Errorf("score %f out of range [-1, 1]", result.Score)
	}
	if result.PartAScore < -1.0 || result.PartAScore > 1.0 {
		t.Errorf("part_a_score %f out of range [-1, 1]", result.PartAScore)
	}
	if result.PartCScore < -1.0 || result.PartCScore > 1.0 {
		t.Errorf("part_c_score %f out of range [-1, 1]", result.PartCScore)
	}
	if result.AdjustmentFactor < 0.8 || result.AdjustmentFactor > 1.2 {
		t.Errorf("adjustment_factor %f out of range [0.8, 1.2]", result.AdjustmentFactor)
	}

	// Verify specific sub-indicators exist
	expectedKeys := []string{
		"a1_margin_z", "a2_day_trading", "a3_margin_maint", "a4_vix_map",
		"a5_pcr_proxy", "a6_odd_lot",
		"c1_futures_oi", "c2_inst_flow", "c3_etf_sub",
		"d1_geopolitical", "d2_vix_spike", "d3_credit_control",
	}
	for _, key := range expectedKeys {
		if _, ok := result.SubIndicators[key]; !ok {
			t.Errorf("expected sub-indicator key %q, not found", key)
		}
	}

	// With VIX 22 and geopolitical risk 0.6, adjustment factor should be < 1.0
	if result.AdjustmentFactor >= 1.0 {
		t.Error("expected adjustment_factor < 1.0 with elevated geopolitical risk")
	}
}

func TestComputeFinal_FallbackPath(t *testing.T) {
	c := NewCalculator()

	input := RSITwInput{
		MarginBalance: 5000,
		// No history → A1 fallback
		DayTrading: nil, // A2 fallback
		VIXLevel:   0,   // A4 fallback
	}

	result := c.ComputeFinal(input)

	// A2 should be fallback because DayTrading is nil
	if si, ok := result.SubIndicators["a2_day_trading"]; ok {
		if !si.IsFallback {
			t.Error("expected a2_day_trading to be fallback when DayTrading is nil")
		}
	}

	// A4 should be fallback because VIX is 0
	if si, ok := result.SubIndicators["a4_vix_map"]; ok {
		if !si.IsFallback {
			t.Error("expected a4_vix_map to be fallback when VIXLevel is 0")
		}
	}
}

func TestSubA1_WithHistory(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	history := []float64{5000, 5100, 5200, 5300, 5400}
	subs := make(map[string]RSISubIndicator)

	data := RSITwInput{MarginBalance: 5500}
	score := c.subA1(data, history, subs, &params)

	if score < -0.5 || score > 0.5 {
		t.Errorf("subA1 score %f out of expected range [-0.5, 0.5]", score)
	}
	si := subs["a1_margin_z"]
	if si.IsFallback {
		t.Error("subA1 should not be fallback with sufficient history")
	}
}

func TestSubA1_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	subs := make(map[string]RSISubIndicator)

	data := RSITwInput{MarginBalance: 5500}
	score := c.subA1(data, []float64{5000}, subs, &params) // only 1 entry

	if score != 0 {
		t.Errorf("subA1 with insufficient history should return 0, got %f", score)
	}
	si := subs["a1_margin_z"]
	if !si.IsFallback {
		t.Error("subA1 should be fallback with insufficient history")
	}
}

func TestSubA2_NilDayTrading(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	subs := make(map[string]RSISubIndicator)

	data := RSITwInput{DayTrading: nil}
	score := c.subA2(data, subs, &params)

	if score != 0 {
		t.Errorf("subA2 with nil DayTrading should return 0, got %f", score)
	}
	si := subs["a2_day_trading"]
	if !si.IsFallback {
		t.Error("subA2 should be fallback when DayTrading is nil")
	}
}

func TestSubA3_MarginPercentile(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	tests := []struct {
		name       string
		percentile float64
		wantSign   int // 1=positive, -1=negative, 0=zero
	}{
		{"high percentile bearish", 0.9, 1},
		{"mid percentile neutral", 0.5, 0},
		{"low percentile bullish", 0.1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs := make(map[string]RSISubIndicator)
			data := RSITwInput{MarginPercentile: tt.percentile}
			score := c.subA3(data, subs, &params)

			if tt.wantSign > 0 && score <= 0 {
				t.Errorf("expected positive score for percentile %f, got %f", tt.percentile, score)
			}
			if tt.wantSign < 0 && score >= 0 {
				t.Errorf("expected negative score for percentile %f, got %f", tt.percentile, score)
			}
			if tt.wantSign == 0 && score != 0 {
				t.Errorf("expected zero score for percentile 0.5, got %f", score)
			}
		})
	}
}

func TestVixMap(t *testing.T) {
	params := config.DefaultParametersConfig().RSITw
	tests := []struct {
		vix  float64
		want float64
	}{
		// Audit A10 (2026-08-12): VIX 高 = 市場恐慌 → 散戶恐慌 → 推低分數。
		// 低 VIX 輕微樂觀 (+0.1)，高 VIX 恐慌 (-1.0)。
		{10, 0.1},
		{17, 0.0},
		{22, -0.3},
		{27, -0.5},
		{32, -0.7},
		{40, -1.0},
	}
	for _, tt := range tests {
		got := vixMapParam(tt.vix, &params)
		if got != tt.want {
			t.Errorf("vixMapParam(%f) = %f, want %f", tt.vix, got, tt.want)
		}
	}
}

func TestSubA4_Fallback(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	subs := make(map[string]RSISubIndicator)

	data := RSITwInput{VIXLevel: 0}
	score := c.subA4(data, subs, &params)

	if score != 0 { // fallback: 資料缺失不貢獻
		t.Errorf("subA4 fallback expected 0, got %f", score)
	}
	si := subs["a4_vix_map"]
	if !si.IsFallback {
		t.Error("subA4 should be fallback when VIX is 0")
	}
}

func TestSubA5_PCRThresholds(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	tests := []struct {
		pcr  float64
		want float64
	}{
		// Audit A10 (2026-08-12): PCR 高（賣權成交多）= 散戶避險/看空 → 恐慌 → 推低分數；
		// PCR 低（買權成交多）= 樂觀 → 推高。fallback 0 不貢獻。
		{0, 0.0},    // fallback
		{1.6, -0.9}, // > 1.5
		{1.2, -0.5}, // > 1.0
		{0.9, -0.2}, // > 0.8
		{0.5, 0.3},  // else（樂觀）
	}
	for _, tt := range tests {
		subs := make(map[string]RSISubIndicator)
		data := RSITwInput{PutCallRatio: tt.pcr}
		score := c.subA5(data, subs, &params)

		expectedScore := tt.want * 0.10 // weight
		if math.Abs(score-expectedScore) > 0.001 {
			t.Errorf("subA5(pcr=%f) = %f, want %f", tt.pcr, score, expectedScore)
		}
	}
}

func TestSubA6_OddLotThresholds(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)
	tests := []struct {
		imb  float64
		want float64
	}{
		{0, 0.5},    // fallback
		{0.3, 0.85}, // > 0.2
		{0.15, 0.65},
		{0, 0.5},
		{-0.05, 0.5},
		{-0.15, 0.35},
		{-0.3, 0.15},
	}
	for _, tt := range tests {
		subs := make(map[string]RSISubIndicator)
		data := RSITwInput{OddLotImbalance: tt.imb}
		score := c.subA6(data, subs, &params)

		expectedScore := tt.want * 0.10
		if math.Abs(score-expectedScore) > 0.001 {
			t.Errorf("subA6(imb=%f) = %f, want %f", tt.imb, score, expectedScore)
		}
	}
}

func TestSubC1_FuturesOIThresholds(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	tests := []struct {
		pct  float64
		want float64
	}{
		// Audit A15 (2026-08-12): threshold 由 20/10/-10/-20 修正為 5/2/-2/-5（匹配實測量級 ±10）。
		// Audit A02: fallback 走 params.C1FallbackScore (0.5)。
		{0, 0.5},   // fallback
		{6, 0.9},   // > C1VeryBullishThreshold (5)
		{3, 0.7},   // > C1BullishThreshold (2)
		{0, 0.5},   // > C1BearishThreshold (-2)
		{-3, 0.25}, // > C1VeryBearishThreshold (-5)
		{-6, 0.1},  // else
	}
	for _, tt := range tests {
		subs := make(map[string]RSISubIndicator)
		data := RSITwInput{RetailFuturesPct: tt.pct}
		score := c.subC1(data, subs, &params)

		expectedScore := tt.want * 0.40
		if math.Abs(score-expectedScore) > 0.001 {
			t.Errorf("subC1(pct=%f) = %f, want %f", tt.pct, score, expectedScore)
		}
	}
}

func TestSubC2_InstFlow(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	tests := []struct {
		name         string
		foreignNet   float64
		domesticNet  float64
		wantFallback bool
	}{
		{"zero net flow", 0, 0, true},
		// Audit A07 (2026-08-12): ForeignInvestorNet 單位為億股（T86 股數/1e8）。
		// scaling 由 1e9（假設 TWD 元）修正為 10（億股量級）。
		{"positive net flow 億股", 5, 3, false},
		{"negative net flow 億股", -8, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs := make(map[string]RSISubIndicator)
			data := RSITwInput{
				ForeignInvestorNet: tt.foreignNet,
				DomesticFundNet:    tt.domesticNet,
			}
			score := c.subC2(data, subs, &params)

			si := subs["c2_inst_flow"]
			if si.IsFallback != tt.wantFallback {
				t.Errorf("fallback = %v, want %v", si.IsFallback, tt.wantFallback)
			}
			if tt.wantFallback && score != 0 {
				t.Errorf("fallback should return 0, got %f", score)
			}
			if !tt.wantFallback && (si.ZScore < 0.1 || si.ZScore > 0.9) {
				t.Errorf("z-score %f out of range [0.1, 0.9]", si.ZScore)
			}
			// Audit A07: 淨買超 8 億股應產生明顯偏離 0.5 的分數（有鑑別力）
			if tt.name == "positive net flow 億股" {
				want := 0.5 + (5+3)/10.0
				if want > 0.9 {
					want = 0.9 // clamped upper bound
				}
				if math.Abs(si.ZScore-want) > 0.001 {
					t.Errorf("positive flow z-score = %f, want %f", si.ZScore, want)
				}
			}
			if tt.name == "negative net flow 億股" {
				want := 0.5 + (-8)/10.0
				if want < 0.1 {
					want = 0.1
				}
				if math.Abs(si.ZScore-want) > 0.001 {
					t.Errorf("negative flow z-score = %f, want %f", si.ZScore, want)
				}
			}
		})
	}
}

func TestSubC3_ETFThresholds(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	tests := []struct {
		netSub float64
		want   float64
	}{
		{0, 0},               // B03: 資料不可用 → 不貢獻（IsFallback），不再假裝中性
		{2_000_000_000, 0.9}, // > C3VeryBullishThreshold (1B)
		{500_000_000, 0.7},   // > C3BullishThreshold (100M)
		{50_000_000, 0.55},   // > 0
		{-50_000_000, 0.45},  // > C3BearishThreshold (-100M)
		{-500_000_000, 0.2},  // else
	}
	for _, tt := range tests {
		subs := make(map[string]RSISubIndicator)
		data := RSITwInput{ETFNetSubscription: tt.netSub}
		score := c.subC3(data, subs, &params)

		expectedScore := tt.want * 0.25
		if math.Abs(score-expectedScore) > 0.001 {
			t.Errorf("subC3(netSub=%f) = %f, want %f", tt.netSub, score, expectedScore)
		}
	}
}

func TestFactorD1_GeopoliticalRisk(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	// Below threshold → no adjustment
	subs := make(map[string]RSISubIndicator)
	data := RSITwInput{GeopoliticalRisk: 0.3}
	factor := c.factorD1(data, subs, &params)
	if factor != 1.0 {
		t.Errorf("factorD1 below threshold should be 1.0, got %f", factor)
	}

	// Above threshold → apply multiplier (0.85)
	subs = make(map[string]RSISubIndicator)
	data.GeopoliticalRisk = 0.6
	factor = c.factorD1(data, subs, &params)
	if factor == 1.0 {
		t.Error("factorD1 above threshold should not be 1.0")
	}
}

func TestFactorD2_VIXSpike(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	c.SetParams(params)

	subs := make(map[string]RSISubIndicator)
	data := RSITwInput{VIXLevel: 35}
	factor := c.factorD2(data, subs, &params)
	if factor == 1.0 {
		t.Error("factorD2 with VIX 35 should apply multiplier")
	}

	subs = make(map[string]RSISubIndicator)
	data.VIXLevel = 20
	factor = c.factorD2(data, subs, &params)
	if factor != 1.0 {
		t.Errorf("factorD2 with VIX 20 should be 1.0, got %f", factor)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want float64
	}{
		{0.5, -1, 1, 0.5},
		{2.0, -1, 1, 1.0},
		{-2.0, -1, 1, -1.0},
		{0.0, 0.8, 1.2, 0.8},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestRound4(t *testing.T) {
	tests := []struct {
		v    float64
		want float64
	}{
		{0.12345, 0.1235},
		{0.12344, 0.1234},
		{1.0, 1.0},
		{-0.55555, -0.5556},
	}
	for _, tt := range tests {
		got := round4(tt.v)
		if got != tt.want {
			t.Errorf("round4(%f) = %f, want %f", tt.v, got, tt.want)
		}
	}
}

func TestUpdateHistory_MaintainsCap(t *testing.T) {
	c := NewCalculator()
	for i := 0; i < 100; i++ {
		c.UpdateHistory(RSITwInput{
			MarginBalance: float64(5000 + i*10),
			VIXLevel:      float64(20 + i%10),
		})
	}

	c.mu.RLock()
	marginLen := len(c.marginHistory)
	vixLen := len(c.vixHistory)
	c.mu.RUnlock()

	if marginLen > 90 {
		t.Errorf("margin history cap exceeded: %d > 90", marginLen)
	}
	if vixLen > 90 {
		t.Errorf("vix history cap exceeded: %d > 90", vixLen)
	}
}

func TestCalculatorSingleton(t *testing.T) {
	c1 := GetCalculator()
	c2 := GetCalculator()
	if c1 != c2 {
		t.Error("GetCalculator should return the same instance")
	}
}

func TestLastScore_Initial(t *testing.T) {
	c := NewCalculator()
	if c.LastScore() != 0 {
		t.Errorf("initial LastScore = %f, want 0", c.LastScore())
	}
}

func TestLastScore_AfterCompute(t *testing.T) {
	c := NewCalculator()
	c.UpdateHistory(RSITwInput{MarginBalance: 5000})
	c.UpdateHistory(RSITwInput{MarginBalance: 5100})

	input := RSITwInput{
		MarginBalance:      5200,
		MarginPercentile:   0.5,
		VIXLevel:           22,
		ForeignInvestorNet: 1_000_000_000,
		DomesticFundNet:    500_000_000,
	}
	result := c.ComputeFinal(input)
	last := c.LastScore()
	// lastScore 存未 round 的 final，snapshot.Score 是 round4(final) —
	// 比較時需對齊同一精度（C3 改動後浮點差異 >1e-6 浮現）。
	if math.Abs(result.Score-round4(last)) > 1e-6 {
		t.Errorf("LastScore = %f, want %f (from ComputeFinal)", round4(last), result.Score)
	}
}

func TestFactorD3_CreditTightening(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw

	subs := make(map[string]RSISubIndicator)
	data := RSITwInput{CreditTightening: true}
	factor := c.factorD3(data, subs, &params)

	expectedMultiplier := params.DCreditTighteningMultiplier.Value
	if factor != expectedMultiplier {
		t.Errorf("factorD3 with CreditTightening = %f, want %f", factor, expectedMultiplier)
	}
	si := subs["d3_credit_control"]
	if !si.IsFallback {
		t.Error("factorD3 credit control should be marked fallback")
	}
}

func TestFactorD3_NoCreditTightening(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw

	subs := make(map[string]RSISubIndicator)
	data := RSITwInput{CreditTightening: false}
	factor := c.factorD3(data, subs, &params)

	if factor != 1.0 {
		t.Errorf("factorD3 without CreditTightening = %f, want 1.0", factor)
	}
}

func TestAvgScore(t *testing.T) {
	results := []calibrationResult{
		{Score: 0.6}, {Score: 0.4}, {Score: 0.2},
	}
	avg := avgScore(results)
	if math.Abs(avg-0.4) > 1e-6 {
		t.Errorf("avgScore = %f, want 0.4", avg)
	}
}

func TestAvgScore_Empty(t *testing.T) {
	if avgScore(nil) != 0 {
		t.Error("avgScore(nil) should return 0")
	}
}

func TestApplyChange(t *testing.T) {
	params := config.DefaultParametersConfig().RSITw
	original := params.A1Weight.Value

	applyChange(&params, CalibrationMetadata{
		Parameter: "a1_weight",
		After:     original * 1.1,
	})

	if params.A1Weight.Value != original*1.1 {
		t.Errorf("A1Weight = %f, want %f", params.A1Weight.Value, original*1.1)
	}
}

func TestApplyChange_UnknownParameter(t *testing.T) {
	params := config.DefaultParametersConfig().RSITw
	applyChange(&params, CalibrationMetadata{Parameter: "unknown", After: 999})
}

func TestCalibrateRSITw_InsufficientData(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rsi-cal-test")
	defer os.RemoveAll(dir)

	report, err := CalibrateRSITw(dir)
	if err != nil {
		t.Fatalf("CalibrateRSITw with empty dir: %v", err)
	}
	if report.Verdict != "insufficient_data" {
		t.Errorf("Verdict = %s, want insufficient_data", report.Verdict)
	}
	if report.SampleCount != 0 {
		t.Errorf("SampleCount = %d, want 0", report.SampleCount)
	}
}

func TestCalibrateRSITw_WithData(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rsi-cal-test2")
	defer os.RemoveAll(dir)

	macroDir := filepath.Join(dir, "data", "state", "macro")
	os.MkdirAll(macroDir, 0o755)

	for i := 0; i < 15; i++ {
		ts := time.Date(2026, 7, 1+i, 8, 0, 0, 0, time.UTC)
		data := map[string]interface{}{
			"recorded_at":           ts.Format(time.RFC3339),
			"retail_margin_balance": map[string]interface{}{"value": 5000.0 + float64(i)*100},
			"vix":                   map[string]interface{}{"value": 22.0 + float64(i%5)},
			"foreign_investor_net":  map[string]interface{}{"value": 1_000_000_000.0},
			"domestic_fund_net":     map[string]interface{}{"value": 500_000_000.0},
		}
		fname := fmt.Sprintf("2026-07-%02d.json", 1+i)
		raw, _ := json.Marshal(data)
		os.WriteFile(filepath.Join(macroDir, fname), raw, 0o644)
	}
	os.WriteFile(filepath.Join(macroDir, "latest.json"), []byte("{}"), 0o644)

	report, err := CalibrateRSITw(dir)
	if err != nil {
		t.Fatalf("CalibrateRSITw with data: %v", err)
	}
	if report.Verdict == "insufficient_data" || report.Verdict == "" {
		t.Errorf("Verdict = %s, want no_improvement or calibrated", report.Verdict)
	}
	if report.SampleCount < 10 {
		t.Errorf("SampleCount = %d, want >= 10", report.SampleCount)
	}
}

func TestLoadLastCalibrationReport_NotFound(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rsi-cal-load")
	defer os.RemoveAll(dir)

	_, err := LoadLastCalibrationReport(dir)
	if err == nil {
		t.Error("LoadLastCalibrationReport should error when no report exists")
	}
}

func TestLoadLastCalibrationReport_Success(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rsi-cal-load2")
	defer os.RemoveAll(dir)

	os.MkdirAll(filepath.Join(dir, "data", "state"), 0o755)

	report := &CalibrationReport{
		Timestamp: time.Now(),
		Verdict:   "calibrated",
		Score:     0.75,
		Changes:   []CalibrationMetadata{},
	}
	saveCalibrationReport(dir, report)

	loaded, err := LoadLastCalibrationReport(dir)
	if err != nil {
		t.Fatalf("LoadLastCalibrationReport: %v", err)
	}
	if loaded.Verdict != "calibrated" {
		t.Errorf("Verdict = %s, want calibrated", loaded.Verdict)
	}
	if loaded.Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", loaded.Score)
	}
}

func TestSetParams(t *testing.T) {
	c := NewCalculator()
	params := config.DefaultParametersConfig().RSITw
	params.C1VeryBullishThreshold.Value = 999 // custom value
	c.SetParams(params)

	c.mu.RLock()
	stored := c.params
	c.mu.RUnlock()

	if stored.C1VeryBullishThreshold.Value != 999 {
		t.Errorf("SetParams didn't persist: got %f, want 999", stored.C1VeryBullishThreshold.Value)
	}
}
