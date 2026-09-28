package marketdata

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// 連續契約建構的 **golden test**（規格 §6.4 配套 1）：
// 固定輸入 → 固定輸出。任何人改了調整法或換倉推導，這裡必須紅。

func gPtr(v float64) *float64 { return &v }
func gInt(v int64) *int64     { return &v }

func goldBar(month, date string, close float64, volume int64, oi int64) domain.FuturesBar {
	d, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		panic(err)
	}
	return domain.FuturesBar{
		Contract:      "TX",
		ContractMonth: month,
		TradeDate:     d,
		Session:       domain.SessionRegular,
		Open:          gPtr(close - 1),
		High:          gPtr(close + 1),
		Low:           gPtr(close - 2),
		Close:         gPtr(close),
		Volume:        gInt(volume),
		OpenInterest:  gInt(oi),
		Source:        "golden",
	}
}

// goldenBars 是三段（202606 → 202607 → 202608）的合成資料。
//
//	段 A（202606）：06-15 close 100、06-16 close 101、06-17(=換倉日) close 102
//	段 B（202607）：06-17 close 110（換倉日同日可比較）、07-15 close 112、07-16(=換倉日) close 113
//	段 C（202608）：07-16 close 120、08-19 close 121
//
// splice 價差：diff1 = 110 − 102 = 8；diff2 = 120 − 113 = 7。
func goldenBars() []domain.FuturesBar {
	return []domain.FuturesBar{
		goldBar("202606", "2026-06-15", 100, 10, 1000),
		goldBar("202606", "2026-06-16", 101, 11, 1001),
		goldBar("202606", "2026-06-17", 102, 12, 1002),
		goldBar("202607", "2026-06-17", 110, 20, 2000),
		goldBar("202607", "2026-07-15", 112, 21, 2001),
		goldBar("202607", "2026-07-16", 113, 22, 2002),
		goldBar("202608", "2026-07-16", 120, 30, 3000),
		goldBar("202608", "2026-08-19", 121, 31, 3001),
	}
}

func TestBuildContinuousSeries_BackAdjustGolden(t *testing.T) {
	bars, rollovers, err := BuildContinuousSeries(goldenBars(), domain.AdjustPriceDiff)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// splice 事件：2 個，價差 8 與 7。
	if len(rollovers) != 2 {
		t.Fatalf("want 2 rollovers, got %d: %+v", len(rollovers), rollovers)
	}
	if got := rollovers[0]; got.RollDate.Format("2006-01-02") != "2026-06-17" || got.FromMonth != "202606" || got.ToMonth != "202607" {
		t.Fatalf("rollover[0] = %+v", got)
	}
	if rollovers[0].PriceDiff == nil || *rollovers[0].PriceDiff != 8 {
		t.Fatalf("rollover[0] diff = %v, want 8", rollovers[0].PriceDiff)
	}
	if rollovers[1].PriceDiff == nil || *rollovers[1].PriceDiff != 7 {
		t.Fatalf("rollover[1] diff = %v, want 7", rollovers[1].PriceDiff)
	}
	if rollovers[1].RollDate.Format("2006-01-02") != "2026-07-16" {
		t.Fatalf("rollover[1] date = %s", rollovers[1].RollDate.Format("2006-01-02"))
	}

	// 調整後收盤：段 A +15、段 B +7、段 C +0。
	want := map[string]struct {
		adjClose float64
		shift    float64
	}{
		"2026-06-15": {115, 15},
		"2026-06-16": {116, 15},
		"2026-06-17": {117, 15},
		"2026-07-15": {119, 7},
		"2026-07-16": {120, 7},
		"2026-08-19": {121, 0},
	}
	if len(bars) != len(want) {
		t.Fatalf("want %d continuous bars, got %d", len(want), len(bars))
	}
	for _, b := range bars {
		w, ok := want[b.TradeDate.Format("2006-01-02")]
		if !ok {
			t.Fatalf("unexpected date %s", b.TradeDate.Format("2006-01-02"))
		}
		if b.AdjustedClose == nil {
			t.Fatalf("%s adjusted close is nil", b.TradeDate.Format("2006-01-02"))
		}
		if *b.AdjustedClose != w.adjClose {
			t.Errorf("%s adjusted close = %v, want %v", b.TradeDate.Format("2006-01-02"), *b.AdjustedClose, w.adjClose)
		}
		if b.CumulativeDiff != w.shift {
			t.Errorf("%s cumulative diff = %v, want %v", b.TradeDate.Format("2006-01-02"), b.CumulativeDiff, w.shift)
		}
		// 原始值必須保留（可回溯真實成交價）。
		if b.Close == nil || *b.Close != *b.AdjustedClose-w.shift {
			t.Errorf("%s raw close = %v, want %v", b.TradeDate.Format("2006-01-02"), b.Close, *b.AdjustedClose-w.shift)
		}
		// 量與 OI **不調整**。
		if b.Volume == nil || b.OpenInterest == nil {
			t.Errorf("%s lost volume/OI in adjustment", b.TradeDate.Format("2006-01-02"))
		}
	}

	// 換倉日兩契約的調整後收盤必須相等（＝序列連續）。
	roll17 := findContinuous(t, bars, "2026-06-17")
	if roll17.AdjustedClose == nil || *roll17.AdjustedClose != 117 {
		t.Fatalf("roll day adjusted close = %v, want 117", roll17.AdjustedClose)
	}
}

func TestBuildContinuousSeries_NoAdjust(t *testing.T) {
	bars, rollovers, err := BuildContinuousSeries(goldenBars(), domain.AdjustNone)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, b := range bars {
		if b.CumulativeDiff != 0 {
			t.Fatalf("%s cumulative diff = %v, want 0 under AdjustNone", b.TradeDate.Format("2006-01-02"), b.CumulativeDiff)
		}
		if b.AdjustedClose == nil || b.Close == nil || *b.AdjustedClose != *b.Close {
			t.Fatalf("%s adjusted close must equal raw close under AdjustNone", b.TradeDate.Format("2006-01-02"))
		}
	}
	// splice 事件仍然推導（供稽核），只是不套用。
	if len(rollovers) != 2 {
		t.Fatalf("want 2 rollovers even under AdjustNone, got %d", len(rollovers))
	}
}

func TestBuildContinuousSeries_InvalidSpliceKeepsEventWithNilDiff(t *testing.T) {
	bars := goldenBars()
	// 拿掉換倉日上「新契約」的報價 ⇒ splice 無法計算。
	var filtered []domain.FuturesBar
	for _, b := range bars {
		if b.ContractMonth == "202607" && b.TradeDate.Format("2006-01-02") == "2026-06-17" {
			continue
		}
		filtered = append(filtered, b)
	}
	_, rollovers, err := BuildContinuousSeries(filtered, domain.AdjustPriceDiff)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(rollovers) == 0 {
		t.Fatal("splice event must still be reported")
	}
	if rollovers[0].SpliceValid() {
		t.Fatal("splice without a comparable close must be reported as invalid (nil diff), not silently filled")
	}
}

func TestBuildContinuousSeries_RejectsUnknownMethod(t *testing.T) {
	if _, _, err := BuildContinuousSeries(goldenBars(), domain.AdjustMethod("ratio")); err == nil {
		t.Fatal("unsupported adjust method must error")
	}
}

func TestBuildContinuousSeries_SkipsNonCanonicalAndNonMonthly(t *testing.T) {
	bars := goldenBars()
	// 週契約與盤後列都不得進入連續序列。
	bars = append(bars, goldBar("202609W3", "2026-08-20", 999, 5, 5))
	after := goldBar("202606", "2026-06-15", 999, 5, 5)
	after.Session = domain.SessionAfterHours
	bars = append(bars, after)

	out, _, err := BuildContinuousSeries(bars, domain.AdjustPriceDiff)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, b := range out {
		if b.ContractMonth == "202609W3" {
			t.Fatal("weekly contract leaked into the continuous series")
		}
		if b.Session != domain.SessionRegular {
			t.Fatalf("non-canonical session leaked: %q", b.Session)
		}
	}
	if len(out) != 6 {
		t.Fatalf("want 6 continuous bars, got %d", len(out))
	}
}

func findContinuous(t *testing.T, bars []domain.FuturesContinuousBar, date string) domain.FuturesContinuousBar {
	t.Helper()
	for _, b := range bars {
		if b.TradeDate.Format("2006-01-02") == date {
			return b
		}
	}
	t.Fatalf("continuous bar %s not found", date)
	return domain.FuturesContinuousBar{}
}
