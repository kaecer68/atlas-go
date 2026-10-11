package ml

import (
	"testing"
)

func dailyTimes(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i)
	}
	return out
}

// TestPurgedKFold_NoLeakage 每 fold：訓練樣本標籤窗 [t, t+H] 不得與驗證窗
// 重疊；測試塊後 EmbargoDays 內不得有訓練樣本；驗證塊恰好覆蓋全體一次。
func TestPurgedKFold_NoLeakage(t *testing.T) {
	times := dailyTimes(100)
	s := &PurgedKFoldSplitter{K: 5, HorizonDays: 5, EmbargoDays: 3}
	folds := s.SplitTimes(times)
	if len(folds) != 5 {
		t.Fatalf("folds = %d, want 5", len(folds))
	}
	seen := map[int]int{}
	for fi, fold := range folds {
		train, val := fold[0], fold[1]
		if len(val) == 0 {
			t.Fatalf("fold %d: empty validation block", fi)
		}
		for _, v := range val {
			seen[v]++
		}
		valEnd := times[val[len(val)-1]]
		for _, tr := range train {
			tt := times[tr]
			// purge：訓練標籤窗 [t, t+H] 不得碰驗證任一樣本的 [vt, vt+H]
			for _, v := range val {
				vt := times[v]
				if tt <= vt+5 && vt <= tt+5 {
					t.Fatalf("fold %d: train %d (t=%d) label overlaps val %d (vt=%d)", fi, tr, tt, v, vt)
				}
			}
			// embargo：測試塊後 3 天內不得有訓練樣本
			if tt > valEnd && tt <= valEnd+3 {
				t.Fatalf("fold %d: train %d (t=%d) inside embargo after valEnd=%d", fi, tr, tt, valEnd)
			}
		}
	}
	for i := 0; i < 100; i++ {
		if seen[i] != 1 {
			t.Fatalf("sample %d validated %d times, want exactly once", i, seen[i])
		}
	}
}

// TestPurgedKFold_FullOverlapPurge horizon 蓋掉全窗時訓練集可為空（不 crash）。
func TestPurgedKFold_FullOverlapPurge(t *testing.T) {
	s := &PurgedKFoldSplitter{K: 2, HorizonDays: 200, EmbargoDays: 0}
	folds := s.SplitTimes(dailyTimes(100))
	if len(folds) != 2 {
		t.Fatalf("folds = %d, want 2", len(folds))
	}
	if len(folds[0][0]) != 0 {
		t.Fatalf("degenerate horizon must purge all train samples, got %d", len(folds[0][0]))
	}
}

// TestWalkForward_PastOnly 訓練恆在驗證之前；驗證塊時序不重疊。
func TestWalkForward_PastOnly(t *testing.T) {
	times := dailyTimes(100)
	s := &WalkForwardSplitter{Folds: 3, MinTrain: 20, HorizonDays: 5}
	folds := s.SplitTimes(times)
	if len(folds) != 3 {
		t.Fatalf("folds = %d, want 3", len(folds))
	}
	prevValEnd := int64(-1)
	for fi, fold := range folds {
		train, val := fold[0], fold[1]
		if len(train) < 20 {
			t.Fatalf("fold %d: train %d < MinTrain 20", fi, len(train))
		}
		if len(val) == 0 {
			t.Fatalf("fold %d: empty validation block", fi)
		}
		trainMax := times[train[len(train)-1]]
		valStart := times[val[0]]
		valEnd := times[val[len(val)-1]]
		// 訓練樣本標籤窗 [t, t+H] 必須在驗證開始前結束
		if trainMax+5 >= valStart {
			t.Fatalf("fold %d: train max t=%d leaks into valStart=%d", fi, trainMax, valStart)
		}
		if valStart <= prevValEnd {
			t.Fatalf("fold %d: val blocks overlap or unordered", fi)
		}
		prevValEnd = valEnd
	}
}

// TestSplitters_ImplementSplitter 編譯期保證兩 splitter 可插進既有 Trainer。
func TestSplitters_ImplementSplitter(t *testing.T) {
	var _ Splitter = &PurgedKFoldSplitter{}
	var _ Splitter = &WalkForwardSplitter{}
}
