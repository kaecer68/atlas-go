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

	"github.com/kaecer68/atlas-go/internal/domain"
)

// JSONLFuturesBarStore 是 futures_bars 的 JSONL 後端。
//
// 存在的理由：ResolveStoreBackend 的歷史預設後端是 jsonl。為避免「未設定
// ATLAS_STORE_BACKEND 就直接失敗」或更糟的「靜默改寫 sqlite」，本後端讓該組態仍有
// 一個誠實、可用的落點（<ledgerDir>/futures_bars.jsonl）。
//
// 寫入語意與 SQLite 後端一致：以 (contract, contract_month, trade_date, session)
// 為冪等鍵，後寫覆蓋（last-write-wins），整檔重寫以保持唯一性（檔案為本機開發用途）。
type JSONLFuturesBarStore struct {
	baseDir string
	mu      sync.Mutex
}

// NewJSONLFuturesBarStore 建立 JSONL 後端。
func NewJSONLFuturesBarStore(baseDir string) *JSONLFuturesBarStore {
	return &JSONLFuturesBarStore{baseDir: baseDir}
}

// Compile-time assertion.
var _ FuturesBarStore = (*JSONLFuturesBarStore)(nil)

func (s *JSONLFuturesBarStore) path() string {
	return filepath.Join(s.baseDir, "futures_bars.jsonl")
}

func futuresBarKey(b domain.FuturesBar) string {
	return b.Contract + "|" + b.ContractMonth + "|" + b.TradeDate.Format("2006-01-02") + "|" + string(b.Session)
}

// RecordFuturesBars 讀入既有資料、以新值覆蓋同鍵者，再整檔寫回。
func (s *JSONLFuturesBarStore) RecordFuturesBars(_ context.Context, bars []domain.FuturesBar) (int, error) {
	if len(bars) == 0 {
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
	merged := make(map[string]domain.FuturesBar, len(existing)+len(bars))
	for _, b := range existing {
		merged[futuresBarKey(b)] = b
	}
	for _, b := range bars {
		merged[futuresBarKey(b)] = b
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
			return 0, fmt.Errorf("encode futures bar: %w", err)
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
	return len(bars), nil
}

func (s *JSONLFuturesBarStore) readAllLocked() ([]domain.FuturesBar, error) {
	f, err := os.Open(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open jsonl: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []domain.FuturesBar
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var b domain.FuturesBar
		if err := json.Unmarshal(line, &b); err != nil {
			// 單列壞掉不應讓整檔不可讀；跳過並繼續。
			continue
		}
		out = append(out, b)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return out, nil
}

// LoadFuturesBars 過濾後回傳（依交易日、時段升冪）。
func (s *JSONLFuturesBarStore) LoadFuturesBars(_ context.Context, contract, contractMonth string, session domain.FuturesSession, start, end time.Time) ([]domain.FuturesBar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	startStr, endStr := start.Format("2006-01-02"), end.Format("2006-01-02")
	var out []domain.FuturesBar
	for _, b := range all {
		ds := b.TradeDate.Format("2006-01-02")
		if b.Contract != contract || ds < startStr || ds > endStr {
			continue
		}
		if contractMonth != "" && b.ContractMonth != contractMonth {
			continue
		}
		if session != "" && b.Session != session {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].TradeDate.Equal(out[j].TradeDate) {
			return out[i].TradeDate.Before(out[j].TradeDate)
		}
		return out[i].Session < out[j].Session
	})
	return out, nil
}

// LoadLatestFuturesBars 回傳每個到期月最新一日的 bar。
func (s *JSONLFuturesBarStore) LoadLatestFuturesBars(_ context.Context, contract string) ([]domain.FuturesBar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	latest := make(map[string]time.Time)
	for _, b := range all {
		if b.Contract != contract {
			continue
		}
		if cur, ok := latest[b.ContractMonth]; !ok || b.TradeDate.After(cur) {
			latest[b.ContractMonth] = b.TradeDate
		}
	}
	var out []domain.FuturesBar
	for _, b := range all {
		if b.Contract != contract {
			continue
		}
		if d, ok := latest[b.ContractMonth]; ok && d.Equal(b.TradeDate) {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ContractMonth != out[j].ContractMonth {
			return out[i].ContractMonth < out[j].ContractMonth
		}
		return out[i].Session < out[j].Session
	})
	return out, nil
}

// RecordFuturesRollovers upsert splice 事件（後寫覆蓋，檔內唯一）。
func (s *JSONLFuturesBarStore) RecordFuturesRollovers(_ context.Context, rollovers []domain.FuturesRollover) (int, error) {
	if len(rollovers) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir all: %w", err)
	}
	existing, err := s.readRolloversLocked()
	if err != nil {
		return 0, err
	}
	merged := make(map[string]domain.FuturesRollover, len(existing)+len(rollovers))
	for _, r := range existing {
		merged[futuresRolloverKey(r)] = r
	}
	for _, r := range rollovers {
		merged[futuresRolloverKey(r)] = r
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tmp := s.rolloversPath() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("create temp rollover file: %w", err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, k := range keys {
		if err := enc.Encode(merged[k]); err != nil {
			_ = f.Close()
			return 0, fmt.Errorf("encode futures rollover: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("flush: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close temp rollover file: %w", err)
	}
	if err := os.Rename(tmp, s.rolloversPath()); err != nil {
		return 0, fmt.Errorf("rename temp rollover file: %w", err)
	}
	return len(rollovers), nil
}

func (s *JSONLFuturesBarStore) rolloversPath() string {
	return filepath.Join(s.baseDir, "futures_rollovers.jsonl")
}

func futuresRolloverKey(r domain.FuturesRollover) string {
	return r.Contract + "|" + r.RollDate.Format("2006-01-02") + "|" + r.FromMonth + "|" + r.ToMonth
}

func (s *JSONLFuturesBarStore) readRolloversLocked() ([]domain.FuturesRollover, error) {
	f, err := os.Open(s.rolloversPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open rollovers jsonl: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []domain.FuturesRollover
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r domain.FuturesRollover
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan rollovers jsonl: %w", err)
	}
	return out, nil
}

// LoadFuturesRollovers 回傳某契約的 splice 事件（依 roll_date 升冪）。
func (s *JSONLFuturesBarStore) LoadFuturesRollovers(_ context.Context, contract string) ([]domain.FuturesRollover, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readRolloversLocked()
	if err != nil {
		return nil, err
	}
	var out []domain.FuturesRollover
	for _, r := range all {
		if r.Contract == contract {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RollDate.Before(out[j].RollDate) })
	return out, nil
}
