package feature

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func makeBars(n int) []domain.DailyBar {
	bars := make([]domain.DailyBar, n)
	start := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		bars[i] = domain.DailyBar{
			Date:   start.AddDate(0, 0, i),
			Symbol: "TEST",
			Close:  100.0 + float64(i),
			Open:   99.0 + float64(i),
			High:   102.0 + float64(i),
			Low:    98.0 + float64(i),
			Volume: 1000000 + int64(i)*10000,
		}
	}
	return bars
}

func TestRegistry_AllFeatures(t *testing.T) {
	expected := []string{
		"adx_14", "amihud", "atr_14", "bb_pct_b", "close",
		"hl_range_pct", "hl_ratio", "kurtosis_20d", "liquidity", "ma_ratio",
		"macd", "macd_signal", "momentum_intra", "obv", "price_position",
		"quality_intra", "return_1d", "return_5d", "return_autocorr", "rsi_14",
		"skewness_20d", "value_intra", "volatility_20d", "volume", "volume_ratio",
		"volume_trend",
	}
	names := Available()
	if len(names) != 26 {
		t.Errorf("expected 26 features, got %d: %v", len(names), names)
	}
	for i, n := range expected {
		if names[i] != n {
			t.Errorf("names[%d]: expected %q, got %q", i, n, names[i])
		}
	}
	for _, n := range expected {
		if _, ok := Registry[n]; !ok {
			t.Errorf("feature %q not in registry", n)
		}
	}
}

func TestFeatureClose(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["close"]
	if v := fn(bars[1], 1, bars); v != 101.0 {
		t.Errorf("close: expected 101.0, got %f", v)
	}
}

func TestFeatureVolume(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["volume"]
	expected := math.Log(1000000)
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("volume: expected %f, got %f", expected, v)
	}
	bars[0].Volume = 0
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("zero volume: expected 0, got %f", v)
	}
	bars[0].Volume = -1
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("negative volume: expected 0, got %f", v)
	}
}

func TestFeatureReturn1d(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["return_1d"]
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("idx=0: expected 0, got %f", v)
	}
	expected := (101.0 - 100.0) / 100.0
	if v := fn(bars[1], 1, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=1: expected %f, got %f", expected, v)
	}
	bars[0].Close = 0
	if v := fn(bars[1], 1, bars); v != 0 {
		t.Errorf("zero prev close: expected 0, got %f", v)
	}
}

func TestFeatureReturn5d(t *testing.T) {
	bars := makeBars(10)
	fn := Registry["return_5d"]
	for i := range 5 {
		if v := fn(bars[i], i, bars); v != 0 {
			t.Errorf("idx=%d: expected 0, got %f", i, v)
		}
	}
	expected := (105.0 - 100.0) / 100.0
	if v := fn(bars[5], 5, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=5: expected %f, got %f", expected, v)
	}
}

func TestFeatureHLRatio(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["hl_ratio"]
	expected := (102.0 - 98.0) / 100.0
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}
	bars[0].Close = 0
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("zero close: expected 0, got %f", v)
	}
}

func TestFeatureMARatio(t *testing.T) {
	bars := makeBars(25)
	fn := Registry["ma_ratio"]
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 1.0 {
			t.Errorf("idx=%d: expected 1.0, got %f", i, v)
		}
	}
	var sum float64
	for j := 0; j <= 19; j++ {
		sum += bars[j].Close
	}
	expected := bars[19].Close / (sum / 20.0)
	if v := fn(bars[19], 19, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=19: expected %f, got %f", expected, v)
	}
}

func TestFeatureVolumeRatio(t *testing.T) {
	bars := makeBars(25)
	fn := Registry["volume_ratio"]
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 1.0 {
			t.Errorf("idx=%d: expected 1.0, got %f", i, v)
		}
	}
	bars[20].Volume = 0
	if v := fn(bars[20], 20, bars); v != 1.0 {
		t.Errorf("zero volume: expected 1.0, got %f", v)
	}
}

func TestAvailable(t *testing.T) {
	names := Available()
	if len(names) != 26 {
		t.Errorf("expected 26 features, got %d", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("not sorted: %q >= %q", names[i-1], names[i])
		}
	}
}

func TestValidate(t *testing.T) {
	u := Validate([]string{"close", "bogus", "volume"})
	if len(u) != 1 || u[0] != "bogus" {
		t.Errorf("expected [bogus], got %v", u)
	}
	u = Validate([]string{"close", "volume"})
	if len(u) != 0 {
		t.Errorf("expected empty, got %v", u)
	}
	u = Validate(nil)
	if u != nil {
		t.Errorf("expected nil for nil input, got %v", u)
	}
}

func TestParseNames(t *testing.T) {
	names := ParseNames("close, volume ,  return_1d")
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d: %v", len(names), names)
	}
	if names[0] != "close" || names[1] != "volume" || names[2] != "return_1d" {
		t.Errorf("unexpected: %v", names)
	}
	names = ParseNames("")
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
	names = ParseNames(" , , ")
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestMakeExtractor(t *testing.T) {
	bars := makeBars(5)
	extract := MakeExtractor([]string{"close", "volume"})
	X := extract(bars)
	if len(X) != 5 {
		t.Errorf("expected 5 rows, got %d", len(X))
	}
	for i, row := range X {
		if len(row) != 2 {
			t.Errorf("row[%d]: expected 2 cols, got %d", i, len(row))
		}
		if row[0] != bars[i].Close {
			t.Errorf("row[%d].close: expected %f, got %f", i, bars[i].Close, row[0])
		}
	}
	X = extract(nil)
	if len(X) != 0 {
		t.Errorf("empty bars: expected 0 rows, got %d", len(X))
	}
	extract = MakeExtractor(nil)
	X = extract(bars)
	for _, row := range X {
		if len(row) != 0 {
			t.Errorf("empty features: expected 0 cols, got %d", len(row))
		}
	}
}

func TestForwardReturnLabel(t *testing.T) {
	bars := makeBars(5)
	labelFn := ForwardReturnLabel()
	labels := labelFn(bars)
	if len(labels) != 5 {
		t.Errorf("expected 5 labels, got %d", len(labels))
	}
	for i := range 4 {
		expected := (bars[i+1].Close - bars[i].Close) / bars[i].Close
		if math.Abs(labels[i]-expected) > 1e-9 {
			t.Errorf("label[%d]: expected %f, got %f", i, expected, labels[i])
		}
	}
	if labels[4] != 0 {
		t.Errorf("label[4]: expected 0, got %f", labels[4])
	}
	labels = labelFn(nil)
	if len(labels) != 0 {
		t.Errorf("empty bars: expected 0 labels, got %d", len(labels))
	}
	bars = makeBars(1)
	labels = labelFn(bars)
	if len(labels) != 1 || labels[0] != 0 {
		t.Errorf("single bar: expected [0], got %v", labels)
	}
}

// --- New feature tests (Phase C1) ---

func TestRSI14(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["rsi_14"]

	// idx < 14 → 50.0 (neutral).
	for i := range 14 {
		if v := fn(bars[i], i, bars); math.Abs(v-50.0) > 1e-9 {
			t.Errorf("idx=%d: expected 50.0, got %f", i, v)
		}
	}

	// All up days → RSI = 100.
	upBars := makeBars(20)
	for i := 1; i < 20; i++ {
		upBars[i].Close = upBars[i-1].Close + 1.0
	}
	v := fn(upBars[19], 19, upBars)
	if math.Abs(v-100.0) > 1e-9 {
		t.Errorf("all up: expected 100.0, got %f", v)
	}

	// All down days → RSI = 0.
	downBars := makeBars(20)
	for i := 1; i < 20; i++ {
		downBars[i].Close = downBars[i-1].Close - 1.0
	}
	v = fn(downBars[19], 19, downBars)
	if math.Abs(v-0.0) > 1e-9 {
		t.Errorf("all down: expected 0.0, got %f", v)
	}
}

func TestMACD(t *testing.T) {
	bars := makeBars(40)
	fn := Registry["macd"]

	// idx < 25 → 0.0 (need 26 bars for MACD: EMA12 + EMA26).
	for i := range 25 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	// At idx=25, MACD should be computable (26 bars: 0..25).
	v := fn(bars[25], 25, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=25: expected finite MACD, got %f", v)
	}
}

func TestMACDSignal(t *testing.T) {
	bars := makeBars(40)
	fn := Registry["macd_signal"]

	// idx < 33 → 0.0 (26 for first MACD + 9 for signal EMA, first valid idx=33).
	for i := range 33 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[33], 33, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=33: expected finite macd_signal, got %f", v)
	}
}

func TestBBPctB(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["bb_pct_b"]

	// idx < 19 → 0.0 (need 20 bars for MA20 + std20).
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	// At idx=19, should be computable (20 bars: 0..19).
	v := fn(bars[19], 19, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=20: expected finite bb_pct_b, got %f", v)
	}

	// With constant close (zero std), should return 0.
	flatBars := makeBars(25)
	for i := range flatBars {
		flatBars[i].Close = 100.0
	}
	v = fn(flatBars[19], 19, flatBars)
	if v != 0.0 {
		t.Errorf("zero std: expected 0.0, got %f", v)
	}
}

func TestATR14(t *testing.T) {
	bars := makeBars(20)
	fn := Registry["atr_14"]

	// idx < 14 → 0.0.
	for i := range 14 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	// At idx=14, ATR should be positive.
	v := fn(bars[14], 14, bars)
	if v <= 0.0 {
		t.Errorf("idx=14: expected positive ATR, got %f", v)
	}
}

func TestOBV(t *testing.T) {
	bars := makeBars(10)
	fn := Registry["obv"]

	// idx=0 → 0.0.
	if v := fn(bars[0], 0, bars); v != 0.0 {
		t.Errorf("idx=0: expected 0.0, got %f", v)
	}

	// OBV should be cumulative.
	prev := fn(bars[0], 0, bars)
	for i := 1; i < 10; i++ {
		cur := fn(bars[i], i, bars)
		// All bars have increasing Close, so OBV should increase.
		if cur < prev {
			t.Errorf("idx=%d: OBV should increase (prev=%f, cur=%f)", i, prev, cur)
		}
		prev = cur
	}
}

func TestADX14(t *testing.T) {
	bars := makeBars(40)
	fn := Registry["adx_14"]

	// idx < 28 → 0.0 (need 14 for DI + 14 for smoothing = 28).
	for i := range 28 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	// At idx=28, ADX should be computable.
	v := fn(bars[28], 28, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=28: expected finite ADX, got %f", v)
	}
}

func TestVolatility20d(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["volatility_20d"]

	// idx < 20 → 0.0.
	for i := range 20 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[20], 20, bars)
	if v < 0.0 || math.IsNaN(v) {
		t.Errorf("idx=20: expected non-negative vol, got %f", v)
	}
}

func TestSkewness20d(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["skewness_20d"]

	for i := range 20 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[20], 20, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=20: expected finite skewness, got %f", v)
	}
}

func TestKurtosis20d(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["kurtosis_20d"]

	for i := range 20 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[20], 20, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=20: expected finite kurtosis, got %f", v)
	}
}

func TestAmihud(t *testing.T) {
	bars := makeBars(5)
	fn := Registry["amihud"]

	// With valid Close and Volume, should compute.
	v := fn(bars[1], 1, bars)
	if v < 0.0 || math.IsNaN(v) {
		t.Errorf("idx=1: expected non-negative amihud, got %f", v)
	}

	// Zero Close → 0.0.
	bars[1].Close = 0
	if v := fn(bars[1], 1, bars); v != 0.0 {
		t.Errorf("zero close: expected 0.0, got %f", v)
	}

	// Zero Volume → 0.0.
	bars[1].Close = 101.0
	bars[1].Volume = 0
	if v := fn(bars[1], 1, bars); v != 0.0 {
		t.Errorf("zero volume: expected 0.0, got %f", v)
	}
}

func TestPricePosition(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["price_position"]

	// idx < 19 → 0.0 (need 20 bars for MA20).
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	// Close > MA20 → positive.
	v := fn(bars[19], 19, bars)
	if math.IsNaN(v) {
		t.Errorf("idx=20: expected finite price_position, got %f", v)
	}

	// Zero Close → 0.0.
	bars[19].Close = 0
	if v := fn(bars[19], 19, bars); v != 0.0 {
		t.Errorf("zero close: expected 0.0, got %f", v)
	}
}

func TestVolumeTrend(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["volume_trend"]

	// idx < 19 → 1.0 (need 20 bars for MA20 of volume).
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 1.0 {
			t.Errorf("idx=%d: expected 1.0, got %f", i, v)
		}
	}

	v := fn(bars[19], 19, bars)
	if v <= 0.0 || math.IsNaN(v) {
		t.Errorf("idx=20: expected positive volume_trend, got %f", v)
	}
}

func TestHLRangePct(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["hl_range_pct"]

	// idx < 19 → 0.0 (need 20 bars for MA20).
	for i := range 19 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[19], 19, bars)
	if v <= 0.0 || math.IsNaN(v) {
		t.Errorf("idx=20: expected positive hl_range_pct, got %f", v)
	}
}

func TestReturnAutocorr(t *testing.T) {
	bars := makeBars(30)
	fn := Registry["return_autocorr"]

	for i := range 21 {
		if v := fn(bars[i], i, bars); v != 0.0 {
			t.Errorf("idx=%d: expected 0.0, got %f", i, v)
		}
	}

	v := fn(bars[21], 21, bars)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("idx=21: expected finite return_autocorr, got %f", v)
	}

	// Range check: correlation should be in [-1, 1].
	if v < -1.0 || v > 1.0 {
		t.Errorf("idx=21: autocorr %f outside [-1,1]", v)
	}
}

func TestMomentumIntra(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["momentum_intra"]

	// bar[0]: Close=100, Open=99 → momentum = (100-99)/99 = 0.010101...
	expected := (100.0 - 99.0) / 99.0
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-6 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}

	// Open == 0 → use Close as fallback → 0.0.
	bars[0].Open = 0
	if v := fn(bars[0], 0, bars); v != 0.0 {
		t.Errorf("zero open: expected 0.0, got %f", v)
	}
}

func TestValueIntra(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["value_intra"]

	// bar[0]: Close=100, Open=99 → value = 100/99.
	expected := 100.0 / 99.0
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}

	// Open == 0 → 1.0.
	bars[0].Open = 0
	if v := fn(bars[0], 0, bars); v != 1.0 {
		t.Errorf("zero open: expected 1.0, got %f", v)
	}
}

func TestQualityIntra(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["quality_intra"]

	// bar[0]: H=102, L=98, C=100 → quality = 1 - (102-98)/100 = 0.96.
	expected := 1.0 - (102.0-98.0)/100.0
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}

	// Close == 0 → 0.0.
	bars[0].Close = 0
	if v := fn(bars[0], 0, bars); v != 0.0 {
		t.Errorf("zero close: expected 0.0, got %f", v)
	}
}

func TestLiquidity(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["liquidity"]

	// Volume=1000000 → log(1+1000000).
	expected := math.Log(1 + 1000000)
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}

	// Volume <= 0 → 0.0.
	bars[0].Volume = 0
	if v := fn(bars[0], 0, bars); v != 0.0 {
		t.Errorf("zero volume: expected 0.0, got %f", v)
	}
	bars[0].Volume = -1
	if v := fn(bars[0], 0, bars); v != 0.0 {
		t.Errorf("negative volume: expected 0.0, got %f", v)
	}
}
