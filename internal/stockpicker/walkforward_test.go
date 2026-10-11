package stockpicker

import (
	"testing"
)

// wfOutcomes 建 30 筆日頻 outcome（單 symbol、單 source，報酬交替，9 月）。
func wfOutcomes() []SignalOutcome {
	out := make([]SignalOutcome, 0, 30)
	for d := 1; d <= 30; d++ {
		r := 0.03
		if d%2 == 0 {
			r = -0.01
		}
		out = append(out, SignalOutcome{
			Symbol:        "2330",
			TriggerDate:   "2026-09-" + twoDigit(d),
			ForwardReturn: r,
			Source:        "stockpicker-x",
		})
	}
	return out
}

func twoDigit(d int) string {
	if d < 10 {
		return "0" + string(rune('0'+d))
	}
	return string(rune('0'+d/10)) + string(rune('0'+d%10))
}

// TestWalkForwardFolds_Chronological 驗證切分：只驗證尾部、訓練恆在過去
// （標籤窗結束於驗證開始前）、訓練不足的 head fold 跳過。
// 30 筆/folds=3：block=7，fold0（val 09-08..）訓練掛零跳過，
// 剩 fold1（val 09-15..09-21、train 7）與 fold2（val 09-22..09-30）。
func TestWalkForwardFolds_Chronological(t *testing.T) {
	folds := WalkForwardFolds(wfOutcomes(), 3, 7, 5)
	if len(folds) != 2 {
		t.Fatalf("folds = %d, want 2 (head fold skipped for MinTrain)", len(folds))
	}
	first := folds[0]
	if first.Val[0].TriggerDate != "2026-09-15" {
		t.Fatalf("fold0 val start = %s, want 2026-09-15", first.Val[0].TriggerDate)
	}
	if len(first.Train) != 7 {
		t.Fatalf("fold0 train = %d, want 7 (09-01..09-07)", len(first.Train))
	}
	last := folds[1]
	if last.Val[len(last.Val)-1].TriggerDate != "2026-09-30" {
		t.Fatalf("fold1 val end = %s, want 2026-09-30", last.Val[len(last.Val)-1].TriggerDate)
	}
	// 全 fold 驗證不重疊且訓練嚴格在驗證之前。
	seen := map[string]bool{}
	for fi, fold := range folds {
		valStart := fold.Val[0].TriggerDate
		for _, o := range fold.Val {
			if seen[o.TriggerDate] {
				t.Fatalf("fold %d: val %s validated twice", fi, o.TriggerDate)
			}
			seen[o.TriggerDate] = true
		}
		for _, o := range fold.Train {
			if o.TriggerDate >= valStart {
				t.Fatalf("fold %d: train %s not strictly before val %s", fi, o.TriggerDate, valStart)
			}
		}
	}
}

// TestFoldWinRates_Caliber fold 勝率與 SignalWinRate 同口徑：單鍵 fold 的
// WinRate 等於直接聚合該子集。
func TestFoldWinRates_Caliber(t *testing.T) {
	folds := WalkForwardFolds(wfOutcomes(), 3, 7, 5)
	sums := FoldWinRates(folds[0], "120d", 0.00585, 1, 0.95)
	if len(sums) != 1 {
		t.Fatalf("summaries = %d, want 1 (single symbol/source)", len(sums))
	}
	direct, err := SignalWinRate(folds[0].Val, 0.00585, 1, 0.95)
	if err != nil {
		t.Fatalf("direct: %v", err)
	}
	if sums[0].WinRate != direct.WinRate || sums[0].Hits != direct.Hits || sums[0].Observations != direct.Observations {
		t.Fatalf("fold summary %+v != direct %+v", sums[0], direct)
	}
}

// TestHitRateDistribution 分佈統計：mean/min/max/N。
func TestHitRateDistribution(t *testing.T) {
	d := SummarizeHitRates([]float64{0.6, 0.4, 0.8})
	if d.N != 3 || d.Mean != 0.6 || d.Min != 0.4 || d.Max != 0.8 {
		t.Fatalf("distribution = %+v, want N=3 mean=0.6 min=0.4 max=0.8", d)
	}
	empty := SummarizeHitRates(nil)
	if empty.N != 0 {
		t.Fatalf("empty distribution = %+v, want N=0", empty)
	}
}
