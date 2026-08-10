package marketdata

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// B02：TAIEX daily close history store — 供 tw_vol 在 Yahoo 失效時 fallback
// 計算 20 日波動率（需 ≥21 筆 closes）。

func TestTaiwanIndexHistoryStore_AppendAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taiwan_index_history.json")
	s := NewTaiwanIndexHistoryStore(path)

	// 依日期升序 append（provider 每日一次）
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, twseLocation)
	for i := 0; i < 25; i++ {
		s.Append(base.AddDate(0, 0, i), 23000.0+float64(i)*10)
	}

	closes := s.RecentCloses(21)
	if len(closes) != 21 {
		t.Fatalf("RecentCloses(21) = %d, want 21", len(closes))
	}
	// 升序且為最後 21 筆
	if closes[0] != 23000.0+4*10 {
		t.Errorf("first close = %v, want %v (4th entry)", closes[0], 23000.0+4*10)
	}
	if closes[20] != 23000.0+24*10 {
		t.Errorf("last close = %v, want %v (24th entry)", closes[20], 23000.0+24*10)
	}

	// 不足 n 時回全部
	if got := s.RecentCloses(100); len(got) != 25 {
		t.Errorf("RecentCloses(100) = %d, want 25", len(got))
	}
}

func TestTaiwanIndexHistoryStore_Persists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taiwan_index_history.json")
	s := NewTaiwanIndexHistoryStore(path)
	s.Append(time.Date(2026, 8, 7, 0, 0, 0, 0, twseLocation), 45000.0)

	// 新 store 從同一 path 載入
	s2 := NewTaiwanIndexHistoryStore(path)
	closes := s2.RecentCloses(5)
	if len(closes) != 1 || closes[0] != 45000.0 {
		t.Fatalf("loaded closes = %v, want [45000]", closes)
	}
}

func TestTaiwanIndexHistoryStore_MissingFile(t *testing.T) {
	s := NewTaiwanIndexHistoryStore(filepath.Join(t.TempDir(), "nonexistent.json"))
	if got := s.RecentCloses(10); len(got) != 0 {
		t.Errorf("missing file RecentCloses = %v, want empty", got)
	}
	// Append 應建立檔案
	s.Append(time.Now(), 100.0)
	if _, err := os.Stat(s.path); err != nil {
		t.Fatalf("Append should create file: %v", err)
	}
}

func TestTaiwanIndexHistoryStore_SameDateOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taiwan_index_history.json")
	s := NewTaiwanIndexHistoryStore(path)
	d := time.Date(2026, 8, 10, 0, 0, 0, 0, twseLocation)
	s.Append(d, 100.0)
	s.Append(d, 200.0) // 同日覆寫（避免重複交易日）

	if got := s.RecentCloses(10); len(got) != 1 || got[0] != 200.0 {
		t.Fatalf("same-date overwrite failed: %v", got)
	}
}
