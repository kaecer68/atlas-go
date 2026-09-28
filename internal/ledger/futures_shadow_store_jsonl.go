package ledger

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// JSONLFuturesShadowStore 是影子列的 JSONL 後端。
//
// 存在的理由與期貨 bar store 相同：ResolveStoreBackend 的歷史預設後端是 jsonl，
// 為了不讓「未設定 ATLAS_STORE_BACKEND」變成靜默失敗或靜默改寫 sqlite，
// 這個組合必須有一個誠實可用的落點。
type JSONLFuturesShadowStore struct {
	baseDir string
	mu      sync.Mutex
}

// NewJSONLFuturesShadowStore 建立 JSONL 後端。
func NewJSONLFuturesShadowStore(baseDir string) *JSONLFuturesShadowStore {
	return &JSONLFuturesShadowStore{baseDir: baseDir}
}

var _ FuturesShadowStore = (*JSONLFuturesShadowStore)(nil)

func (s *JSONLFuturesShadowStore) path() string {
	return filepath.Join(s.baseDir, "futures_shadow_predictions.jsonl")
}

func futuresShadowKey(row FuturesShadowRow) string {
	return row.Sample.Contract + "|" + row.Sample.TradeDate.Format("2006-01-02") + "|" + row.ModelVersion
}

// RecordFuturesShadow 讀入既有內容、以新值覆蓋同鍵者，再整檔寫回（後寫覆蓋）。
func (s *JSONLFuturesShadowStore) RecordFuturesShadow(_ context.Context, rows []FuturesShadowRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir all: %w", err)
	}
	existing, err := s.readAllLocked()
	if err != nil {
		return 0, err
	}
	merged := make(map[string]FuturesShadowRow, len(existing)+len(rows))
	for _, r := range existing {
		merged[futuresShadowKey(r)] = r
	}
	for _, r := range rows {
		if r.ModelVersion == "" {
			return 0, fmt.Errorf("futures shadow row %s: empty model version", r.Sample.TradeDate.Format("2006-01-02"))
		}
		merged[futuresShadowKey(r)] = r
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tmp := s.path() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, k := range keys {
		if err := enc.Encode(merged[k]); err != nil {
			_ = f.Close()
			return 0, fmt.Errorf("encode futures shadow row: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("flush: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		return 0, fmt.Errorf("rename temp file: %w", err)
	}
	return len(rows), nil
}

func (s *JSONLFuturesShadowStore) readAllLocked() ([]FuturesShadowRow, error) {
	f, err := os.Open(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open jsonl: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []FuturesShadowRow
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var row FuturesShadowRow
		if err := json.Unmarshal(line, &row); err != nil {
			continue // 單列壞掉不讓整檔不可讀
		}
		out = append(out, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return out, nil
}

// LoadFuturesShadow 過濾後回傳（依交易日升冪）。
func (s *JSONLFuturesShadowStore) LoadFuturesShadow(_ context.Context, contract, modelVersion string, start, end time.Time) ([]FuturesShadowRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	startStr, endStr := start.Format("2006-01-02"), end.Format("2006-01-02")
	var out []FuturesShadowRow
	for _, row := range all {
		ds := row.Sample.TradeDate.Format("2006-01-02")
		if row.Sample.Contract != contract || ds < startStr || ds > endStr {
			continue
		}
		if modelVersion != "" && row.ModelVersion != modelVersion {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sample.TradeDate.Before(out[j].Sample.TradeDate) })
	return out, nil
}
