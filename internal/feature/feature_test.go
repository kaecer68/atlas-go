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
			High:   102.0 + float64(i),
			Low:    98.0 + float64(i),
			Volume: 1000000 + int64(i)*10000,
		}
	}
	return bars
}

func TestRegistry_AllFeatures(t *testing.T) {
	expected := []string{"close", "hl_ratio", "ma_ratio", "return_1d", "return_5d", "volume", "volume_ratio"}
	names := Available()
	if len(names) != 7 {
		t.Errorf("expected 7 features, got %d: %v", len(names), names)
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
	// Volume=1000000 → log(1000000)
	expected := math.Log(1000000)
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("volume: expected %f, got %f", expected, v)
	}
	// Zero volume.
	bars[0].Volume = 0
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("zero volume: expected 0, got %f", v)
	}
	// Negative volume.
	bars[0].Volume = -1
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("negative volume: expected 0, got %f", v)
	}
}

func TestFeatureReturn1d(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["return_1d"]
	// idx=0 → 0 (no previous bar).
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("idx=0: expected 0, got %f", v)
	}
	// idx=1: (101-100)/100 = 0.01.
	expected := (101.0 - 100.0) / 100.0
	if v := fn(bars[1], 1, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=1: expected %f, got %f", expected, v)
	}
	// Zero previous close → 0.
	bars[0].Close = 0
	if v := fn(bars[1], 1, bars); v != 0 {
		t.Errorf("zero prev close: expected 0, got %f", v)
	}
}

func TestFeatureReturn5d(t *testing.T) {
	bars := makeBars(10)
	fn := Registry["return_5d"]
	// idx=0..4 → 0.
	for i := 0; i < 5; i++ {
		if v := fn(bars[i], i, bars); v != 0 {
			t.Errorf("idx=%d: expected 0, got %f", i, v)
		}
	}
	// idx=5: (105-100)/100 = 0.05.
	expected := (105.0 - 100.0) / 100.0
	if v := fn(bars[5], 5, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=5: expected %f, got %f", expected, v)
	}
}

func TestFeatureHLRatio(t *testing.T) {
	bars := makeBars(3)
	fn := Registry["hl_ratio"]
	// (102-98)/100 = 0.04 for idx=0.
	expected := (102.0 - 98.0) / 100.0
	if v := fn(bars[0], 0, bars); math.Abs(v-expected) > 1e-9 {
		t.Errorf("idx=0: expected %f, got %f", expected, v)
	}
	// Zero close → 0.
	bars[0].Close = 0
	if v := fn(bars[0], 0, bars); v != 0 {
		t.Errorf("zero close: expected 0, got %f", v)
	}
}

func TestFeatureMARatio(t *testing.T) {
	bars := makeBars(25)
	fn := Registry["ma_ratio"]
	// idx < 19 → 1.0.
	for i := 0; i < 19; i++ {
		if v := fn(bars[i], i, bars); v != 1.0 {
			t.Errorf("idx=%d: expected 1.0, got %f", i, v)
		}
	}
	// idx=19: Close[19] / mean(Close[0..19]).
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
	// idx < 19 → 1.0.
	for i := 0; i < 19; i++ {
		if v := fn(bars[i], i, bars); v != 1.0 {
			t.Errorf("idx=%d: expected 1.0, got %f", i, v)
		}
	}
	// Zero volume → 1.0.
	bars[20].Volume = 0
	if v := fn(bars[20], 20, bars); v != 1.0 {
		t.Errorf("zero volume: expected 1.0, got %f", v)
	}
}

func TestAvailable(t *testing.T) {
	names := Available()
	if len(names) != 7 {
		t.Errorf("expected 7 features, got %d", len(names))
	}
	// Verify sorted.
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
	// Empty string.
	names = ParseNames("")
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
	// Only commas.
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
	// Empty bars.
	X = extract(nil)
	if len(X) != 0 {
		t.Errorf("empty bars: expected 0 rows, got %d", len(X))
	}
	// Empty feature names.
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
	// Forward return: (next.Close - Close) / Close.
	for i := 0; i < 4; i++ {
		expected := (bars[i+1].Close - bars[i].Close) / bars[i].Close
		if math.Abs(labels[i]-expected) > 1e-9 {
			t.Errorf("label[%d]: expected %f, got %f", i, expected, labels[i])
		}
	}
	// Last bar → 0.
	if labels[4] != 0 {
		t.Errorf("label[4]: expected 0, got %f", labels[4])
	}
	// Empty bars.
	labels = labelFn(nil)
	if len(labels) != 0 {
		t.Errorf("empty bars: expected 0 labels, got %d", len(labels))
	}
	// Single bar.
	bars = makeBars(1)
	labels = labelFn(bars)
	if len(labels) != 1 || labels[0] != 0 {
		t.Errorf("single bar: expected [0], got %v", labels)
	}
}
