package stockpicker

import (
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/ml"
)

// ─── Walk-forward 驗證（read-only，對標 Lopez de Prado AFML Ch.7）────────
//
// 對已持久化的 SignalOutcome 做時序前進切分：每 fold 只用標籤窗在驗證開始
// 前結束的過去樣本，輸出每 fold 勝率與分佈。Read-only by construction：
// 不重算回測、不寫 row、不改既有數字。
//
// 切分走 ml.WalkForwardSplitter 同一實作（單一真相來源）；日期→時間軸轉換
// 與週末緩衝是 stockpicker 層唯一自有邏輯。

// weekendBufferDays 是標籤窗的日曆天緩衝：forwardDays 以交易日計，
// 5 交易日 ≈ 7 日曆天。 Purge 方向取保守（寧可多 purge 一點，不少）。
const weekendBufferDays = 2

// WalkForwardFold 是一個前進切分：Train 全在 Val 之前。
type WalkForwardFold struct {
	Train []SignalOutcome
	Val   []SignalOutcome
}

// WalkForwardFolds 將 outcomes（原地不排序，內部複製後按 TriggerDate 排序）
// 切成至多 folds 個前進 folds。forwardDays 為持有期（交易日）；內部轉成
// 日曆天標籤窗 forwardDays+weekendBufferDays。日期解析失敗的列丟棄
// （不回填推測）。訓練不足 minTrain 的 fold 跳過。
func WalkForwardFolds(outcomes []SignalOutcome, folds, minTrain, forwardDays int) []WalkForwardFold {
	type dated struct {
		o   SignalOutcome
		day int64
	}
	sorted := make([]SignalOutcome, len(outcomes))
	copy(sorted, outcomes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TriggerDate < sorted[j].TriggerDate })

	var kept []dated
	times := make([]int64, 0, len(sorted))
	for _, o := range sorted {
		t, err := time.Parse(triggerDateLayout, o.TriggerDate)
		if err != nil {
			continue
		}
		day := t.Unix() / 86400
		kept = append(kept, dated{o: o, day: day})
		times = append(times, day)
	}
	splits := (&ml.WalkForwardSplitter{
		Folds:       folds,
		MinTrain:    minTrain,
		HorizonDays: forwardDays + weekendBufferDays,
	}).SplitTimes(times)

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
