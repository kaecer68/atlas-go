package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// TaiwanIndexHistoryStore 持久化 TAIEX 每日 close（file-backed JSON），供
// tw_vol 在 Yahoo ^TWII 失效時 fallback 計算 20 日波動率（需 ≥21 筆）。
//
// B02（2026-08-10）：tw_vol 目前完全仰賴 Yahoo；DNS/上游故障時無替代。
// 本 store 由 provider 在 Yahoo 成功時每日寫入（同交易日覆寫），Yahoo
// transport error 時 provider 讀取最近 closes 計算波動率（資料時間戳較舊，
// 但非 transport failure）。設計沿用 VIXBaselineTracker 的 file-backed
// JSON pattern（load 容錯：檔案不存在/解析失敗 → 空）。
type TaiwanIndexHistoryStore struct {
	path  string
	mu    sync.RWMutex
	daily map[string]float64 // "20060102"（twseLocation）→ close
}

// NewTaiwanIndexHistoryStore 建立 store；path 指向 JSON 檔案。
// 檔案不存在時視為空歷史（不 error）。
func NewTaiwanIndexHistoryStore(path string) *TaiwanIndexHistoryStore {
	s := &TaiwanIndexHistoryStore{
		path:  path,
		daily: make(map[string]float64),
	}
	s.load()
	return s
}

// Append 記錄某交易日的 close。同日覆寫（provider 每日可能多次 fetch，
// 只保留當日最新值）。寫入後立即持久化。
func (s *TaiwanIndexHistoryStore) Append(date time.Time, close float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := date.In(twseLocation).Format("20060102")
	s.daily[key] = close
	_ = s.saveLocked()
}

// RecentCloses 回傳最近 n 筆 close（依日期升序）。不足 n 時回全部。
// 用於 20 日波動率計算（n=21 需 20 個 log returns）。
func (s *TaiwanIndexHistoryStore) RecentCloses(n int) []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.daily) == 0 {
		return nil
	}
	keys := make([]string, 0, len(s.daily))
	for k := range s.daily {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if n > 0 && len(keys) > n {
		keys = keys[len(keys)-n:]
	}
	out := make([]float64, 0, len(keys))
	for _, k := range keys {
		out = append(out, s.daily[k])
	}
	return out
}

// LastDate 回傳 store 中最後一筆資料的交易日（twseLocation），
// 供 fallback snapshot 標記資料時間戳。無資料時 ok=false。
func (s *TaiwanIndexHistoryStore) LastDate() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.daily) == 0 {
		return time.Time{}, false
	}
	var last string
	for k := range s.daily {
		if k > last {
			last = k
		}
	}
	t, err := time.ParseInLocation("20060102", last, twseLocation)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (s *TaiwanIndexHistoryStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return // 不存在/讀取失敗 → 空歷史
	}
	var daily map[string]float64
	if err := json.Unmarshal(data, &daily); err != nil {
		return // 解析失敗 → 空歷史（下次 Append 覆寫）
	}
	if daily == nil {
		daily = make(map[string]float64)
	}
	s.daily = daily
}

func (s *TaiwanIndexHistoryStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s.daily)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
