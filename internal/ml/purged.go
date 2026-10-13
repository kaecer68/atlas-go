package ml

// ─── Purged + embargo cross-validation（Lopez de Prado AFML Ch.7）────────
//
// 標準 KFoldSplitter 是 shuffled：金融時間序列的標籤窗（trigger→+horizon）
// 互相重疊，訓練集會偷看到驗證集的未來 → 績效虛高、上線即失效。
// 這兩個 splitter 以標籤窗重疊判定 purge、測試塊後 embargo，輸出仍是
// Splitter 介面的 index pairs，可直接插進既有 Trainer。

// PurgedKFoldSplitter 將有序樣本切成 K 個連續驗證塊；每 fold 的訓練集刪掉
// 標籤窗 [t, t+HorizonDays] 與任一驗證標籤窗重疊者，外加驗證塊結束後
// EmbargoDays 內的樣本。Horizon/Embargo 為負視為 0。
type PurgedKFoldSplitter struct {
	K           int
	HorizonDays int
	EmbargoDays int
}

// Split 實作 Splitter：假設樣本有序且大致日頻（times[i] = i）。
func (s *PurgedKFoldSplitter) Split(nSamples int) [][2][]int {
	times := make([]int64, nSamples)
	for i := range times {
		times[i] = int64(i)
	}
	return s.SplitTimes(times)
}

// SplitTimes 以真實時間軸 purge/embargo。times 不必等距，但必須升冪。
func (s *PurgedKFoldSplitter) SplitTimes(times []int64) [][2][]int {
	n := len(times)
	k := s.K
	if k <= 1 || n == 0 {
		return nil
	}
	k = min(k, n)
	h := max(int64(s.HorizonDays), 0)
	e := max(int64(s.EmbargoDays), 0)

	base := n / k
	rem := n % k
	folds := make([][2][]int, 0, k)
	start := 0
	for f := 0; f < k; f++ {
		size := base
		if f < rem {
			size++
		}
		end := start + size // val = [start, end)
		val := make([]int, 0, size)
		for i := start; i < end; i++ {
			val = append(val, i)
		}
		valEnd := times[end-1]
		var train []int
		for i := 0; i < n; i++ {
			if i >= start && i < end {
				continue
			}
			t := times[i]
			if t > valEnd && t <= valEnd+e {
				continue
			}
			overlap := false
			for _, v := range val {
				vt := times[v]
				if t <= vt+h && vt <= t+h {
					overlap = true
					break
				}
			}
			if overlap {
				continue
			}
			train = append(train, i)
		}
		folds = append(folds, [2][]int{train, val})
		start = end
	}
	return folds
}

// WalkForwardSplitter 產生 Folds 個時序前進切分：fold i 在第 i+1 個 block
// 上驗證，只用標籤窗在驗證開始前結束的過去樣本訓練（只看過去，無 embargo
// 需求）。訓練不足 MinTrain 的 fold 跳過（文件化：短序列回傳較少 folds）。
type WalkForwardSplitter struct {
	Folds       int
	MinTrain    int
	HorizonDays int
}

// Split 實作 Splitter：假設樣本有序且大致日頻（times[i] = i）。
func (s *WalkForwardSplitter) Split(nSamples int) [][2][]int {
	times := make([]int64, nSamples)
	for i := range times {
		times[i] = int64(i)
	}
	return s.SplitTimes(times)
}

// SplitTimes 以真實時間軸切分。times 必須升冪。
func (s *WalkForwardSplitter) SplitTimes(times []int64) [][2][]int {
	h := max(int64(s.HorizonDays), 0)
	ends := make([]int64, len(times))
	for i, t := range times {
		ends[i] = t + h
	}
	return s.SplitTimesLabeled(times, ends)
}

// SplitTimesLabeled 是 SplitTimes 的 label-interval 變體：每個樣本自帶標籤
// 窗 [starts[i], ends[i]]，訓練樣本須在驗證開始前**整窗結束**。
//
// 當標籤窗長度不一致時必須用它（以交易日曆計窗時：5 交易日的窗在長連假可跨
// 8 個日曆天，用單一 HorizonDays 近似會 purge 不足——見 FU-20261012-04）。
// ends[i] < starts[i] 視為未標註，退化為 starts[i]。
func (s *WalkForwardSplitter) SplitTimesLabeled(starts, ends []int64) [][2][]int {
	times := starts
	if len(ends) != len(starts) {
		ends = starts
	}
	labelEnd := func(i int) int64 {
		if ends[i] < starts[i] {
			return starts[i]
		}
		return ends[i]
	}
	n := len(times)
	f := s.Folds
	if f <= 0 || n == 0 {
		return nil
	}
	block := n / (f + 1)
	if block <= 0 {
		return nil
	}
	var folds [][2][]int
	for i := 0; i < f; i++ {
		vStart := (i + 1) * block
		vEnd := (i + 2) * block
		if i == f-1 {
			vEnd = n
		}
		if vStart >= n {
			break
		}
		val := make([]int, 0, vEnd-vStart)
		for idx := vStart; idx < vEnd; idx++ {
			val = append(val, idx)
		}
		// 訓練樣本須在驗證首日之前**整窗結束**（用各樣本自己的標籤窗，
		// 不是單一 horizon 近似——長連假會讓近似不足）。
		valFirst := times[vStart]
		var train []int
		for idx := 0; idx < vStart; idx++ {
			if labelEnd(idx) < valFirst {
				train = append(train, idx)
			}
		}
		if len(train) < s.MinTrain {
			continue
		}
		folds = append(folds, [2][]int{train, val})
	}
	return folds
}
