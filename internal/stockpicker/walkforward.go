package stockpicker

import (
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/ml"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// ─── Walk-forward 驗證（read-only，對標 Lopez de Prado AFML Ch.7）────────
//
// 對已持久化的 SignalOutcome 做時序前進切分：每 fold 只用標籤窗在驗證開始
// 前結束的過去樣本，輸出每 fold 勝率與分佈。Read-only by construction：
// 不重算回測、不寫 row、不改既有數字。
//
// 切分走 ml.WalkForwardSplitter 同一實作（單一真相來源）；日期→時間軸轉換
// 與週末緩衝是 stockpicker 層唯一自有邏輯。

// labelEndDate 回傳觸發日之後第 forwardDays 個**交易日**的日期（YYYY-MM-DD）。
//
// 標籤窗必須以交易日曆計，不能用「forwardDays + 週末緩衝」的日曆天近似：
// 實測 2026-02-26 的 5 交易日窗跨 8 個日曆天（農曆年假），近似值少 1 個
// 交易日 ⇒ purge 不足、訓練集與驗證集標籤重疊（FU-20261012-04）。
// 與 daily_update.newestTriggerDate 同源（皆走 taiwanholidays.IsTradingDay）。
func labelEndDate(triggerDate string, forwardDays int) string {
	t, err := time.Parse(triggerDateLayout, triggerDate)
	if err != nil {
		return triggerDate
	}
	d := t
	for range forwardDays {
		d = d.AddDate(0, 0, 1)
		for !taiwanholidays.IsTradingDay(d) {
			d = d.AddDate(0, 0, 1)
		}
	}
	return d.Format(triggerDateLayout)
}

// WalkForwardFold 是一個前進切分：Train 全在 Val 之前。
type WalkForwardFold struct {
	Train []SignalOutcome
	Val   []SignalOutcome
}

// WalkForwardFolds 將 outcomes（原地不排序，內部複製後按 TriggerDate 排序）
// 切成至多 folds 個前進 folds。forwardDays 為持有期（**交易日**）；標籤窗以
// 交易日曆逐樣本計算（labelEndDate），長連假不會低估窗長。
// 日期解析失敗的列丟棄（不回填推測）。訓練不足 minTrain 的 fold 跳過。
func WalkForwardFolds(outcomes []SignalOutcome, folds, minTrain, forwardDays int) []WalkForwardFold {
	type dated struct {
		o   SignalOutcome
		day int64
	}
	sorted := make([]SignalOutcome, len(outcomes))
	copy(sorted, outcomes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TriggerDate < sorted[j].TriggerDate })

	var kept []dated
	starts := make([]int64, 0, len(sorted))
	ends := make([]int64, 0, len(sorted))
	for _, o := range sorted {
		t, err := time.Parse(triggerDateLayout, o.TriggerDate)
		if err != nil {
			continue
		}
		day := t.Unix() / 86400
		end, err := time.Parse(triggerDateLayout, labelEndDate(o.TriggerDate, forwardDays))
		if err != nil {
			ends = append(ends, day)
		} else {
			ends = append(ends, end.Unix()/86400)
		}
		kept = append(kept, dated{o: o, day: day})
		starts = append(starts, day)
	}
	splits := (&ml.WalkForwardSplitter{
		Folds:    folds,
		MinTrain: minTrain,
	}).SplitTimesLabeled(starts, ends)

	out := make([]WalkForwardFold, 0, len(splits))
	for _, sp := range splits {
		fold := WalkForwardFold{}
		for _, idx := range sp[0] {
			fold.Train = append(fold.Train, kept[idx].o)
		}
		for _, idx := range sp[1] {
			fold.Val = append(fold.Val, kept[idx].o)
		}
		out = append(out, fold)
	}
	return out
}

// FoldWinRates 對 fold 的驗證集走 GroupAndSummarize 同一口徑（window 標籤
// 僅帶入 key）。空驗證集回傳空切片。
func FoldWinRates(fold WalkForwardFold, window string, costRate float64, minSamples int, confidence float64) []StockWinRateSummary {
	if len(fold.Val) == 0 {
		return nil
	}
	return GroupAndSummarize(fold.Val, window, costRate, minSamples, confidence)
}

// FoldHitRate 回傳 fold 驗證集的 pooled 淨勝率（NetHit 口徑，與 SignalWinRate
// 一致）。空驗證集回傳 0。
func FoldHitRate(fold WalkForwardFold, costRate float64) float64 {
	if len(fold.Val) == 0 {
		return 0
	}
	hits := 0
	for _, o := range fold.Val {
		if NetHit(o.ForwardReturn, costRate) {
			hits++
		}
	}
	return WinRate(hits, len(fold.Val))
}

// HitRateDistribution 是每 fold pooled 勝率的分佈摘要：用分佈取代單點
// Sharpe，樣本外穩健性的讀法。
type HitRateDistribution struct {
	N    int     `json:"n"`
	Mean float64 `json:"mean"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

// SummarizeHitRates 計算分佈摘要。空輸入回傳 N=0。
func SummarizeHitRates(rates []float64) HitRateDistribution {
	d := HitRateDistribution{N: len(rates)}
	if len(rates) == 0 {
		return d
	}
	sum := 0.0
	d.Min, d.Max = rates[0], rates[0]
	for _, r := range rates {
		sum += r
		d.Min = min(d.Min, r)
		d.Max = max(d.Max, r)
	}
	d.Mean = sum / float64(len(rates))
	return d
}
