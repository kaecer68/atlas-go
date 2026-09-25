package marketdata

// symbol_industry_firstparty_test.go — symbol_industry 第一方來源測試。
//
// 契約（2026-09-24 對真實端點實測，見 symbol_industry_firstparty.go 檔頭）：
//   - 上市 TWSE /opendata/t187ap03_L：JSON array，欄位 公司代號／公司名稱／產業別
//   - 上櫃 TPEx /mopsfin_t187ap03_O：JSON array，欄位
//     SecuritiesCompanyCode／CompanyName／SecuritiesIndustryCode
//
// 測試資料刻意採用真實 payload 的內容（2330 台積電→24、1101 台泥→01、
// 1584 精剛→20 其他業未對映、9103 美德醫療-DR→91 未宣告），這樣「映射表
// 被改壞」或「狀態語意被搞混」都會在斷言中現形，而不是只驗證 mock 自己。
//
// 註：spec 範例寫「8069→24」，但 2026-09-24 官方 payload 的 8069（元太科技）
// 是 26 光電業；本檔以實測為準（26→optoelectronics），並保留 3105 穩懋
// 作為上櫃 24 半導體的樣本。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// symbolIndustryTestClock 是測試固定時鐘：AsOf 與 UpdatedAt 都必須由它決定，
// 否則「同一時鐘下兩次 FetchAll 逐位元相同」不可能成立。
var symbolIndustryTestClock = time.Date(2026, 9, 24, 9, 30, 0, 0, time.UTC)

// ─── mock server ─────────────────────────────────────────────────────────

// symbolIndustryMockServer 同時扮演 TWSE 與 TPEx（兩個 base 都指向它，以路徑
// 分辨），因此測試同時守住兩個路徑常數沒有被亂改。
type symbolIndustryMockServer struct {
	srv       *httptest.Server
	twseCalls atomic.Int64
	tpexCalls atomic.Int64
}

// symbolIndustryMockOpts 是 mock 的內容。所有欄位都在建 server 前決定，並以
// 值捕捉進 handler，避免測試 goroutine 與 handler goroutine 競態。
type symbolIndustryMockOpts struct {
	twse       []map[string]string
	tpex       []map[string]string
	statusTWSE int // 非 0 代表該邊回這個 HTTP 狀態（模擬上游故障）
	statusTPEx int
}

func newSymbolIndustryMockServer(t *testing.T, opts symbolIndustryMockOpts) *symbolIndustryMockServer {
	t.Helper()
	m := &symbolIndustryMockServer{}
	twse, tpex := opts.twse, opts.tpex
	statusTWSE, statusTPEx := opts.statusTWSE, opts.statusTPEx
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case symbolIndustryTWSEReportPath:
			m.twseCalls.Add(1)
			if statusTWSE != 0 {
				http.Error(w, "upstream boom", statusTWSE)
				return
			}
			writeSymbolIndustryRows(w, twse)
		case symbolIndustryTPExReportPath:
			m.tpexCalls.Add(1)
			if statusTPEx != 0 {
				http.Error(w, "upstream boom", statusTPEx)
				return
			}
			writeSymbolIndustryRows(w, tpex)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func writeSymbolIndustryRows(w http.ResponseWriter, rows []map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if rows == nil {
		rows = []map[string]string{}
	}
	_ = json.NewEncoder(w).Encode(rows)
}

// twseIndustryRow / tpexIndustryRow 用真實欄位名組一列（欄位名寫錯 → 整份
// payload 解析成 0 列 → 測試失敗，這正是我們要的守門）。
func twseIndustryRow(symbol, name, code string) map[string]string {
	return map[string]string{"公司代號": symbol, "公司名稱": name, "產業別": code}
}

func tpexIndustryRow(symbol, name, code string) map[string]string {
	return map[string]string{
		"SecuritiesCompanyCode":  symbol,
		"CompanyName":            name,
		"SecuritiesIndustryCode": code,
	}
}

// newSymbolIndustryTestProvider 建 provider 指向 mock，時鐘固定，並把兩個共用
// token bucket 換成無限速率（測試不該真的照 3 req/5s 政策等待；與 twse_sbl
// 第一方測試同一手法）。
func newSymbolIndustryTestProvider(t *testing.T, m *symbolIndustryMockServer, dir string) *SymbolIndustryProvider {
	t.Helper()
	oldTWSE := SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTWSESharedLimiterForTest(oldTWSE) })
	oldTPEx := SetTPExSharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTPExSharedLimiterForTest(oldTPEx) })

	p := NewSymbolIndustryProvider(dir)
	if m != nil {
		p.SetHTTPClient(m.srv.Client())
		p.SetBaseURLs(m.srv.URL, m.srv.URL)
	}
	p.SetClock(func() time.Time { return symbolIndustryTestClock })
	return p
}

func findSymbolIndustryEntry(t *testing.T, snap *SymbolIndustrySnapshot, symbol string) SymbolIndustryEntry {
	t.Helper()
	for _, e := range snap.Entries {
		if e.Symbol == symbol {
			return e
		}
	}
	t.Fatalf("entry %s not found in snapshot (%d entries)", symbol, len(snap.Entries))
	return SymbolIndustryEntry{}
}

// ─── happy path：兩個市場合併、映射、排序、落檔、可重讀 ──────────────────

func TestSymbolIndustryProvider_FetchAll_MergesBothMarkets(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{
			twseIndustryRow("1101", "臺灣水泥股份有限公司", "01"),
			twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24"),
		},
		tpex: []map[string]string{
			tpexIndustryRow("3105", "穩懋半導體股份有限公司", "24"),
			tpexIndustryRow("8069", "元太科技工業股份有限公司", "26"),
			tpexIndustryRow("1584", "精剛精密科技股份有限公司", "20"),
		},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if snap.Channel != "symbol_industry" {
		t.Errorf("channel = %q, want symbol_industry", snap.Channel)
	}
	if snap.UpdatedAt != "2026-09-24T09:30:00Z" {
		t.Errorf("updated_at = %q, want the injected clock in RFC3339 UTC", snap.UpdatedAt)
	}
	wantSources := []string{"TWSE:t187ap03_L", "TPEx:mopsfin_t187ap03_O"}
	if !reflect.DeepEqual(snap.Sources, wantSources) {
		t.Errorf("sources = %v, want %v", snap.Sources, wantSources)
	}

	wantCounts := SymbolIndustryCounts{Total: 5, Mapped: 4, Unmapped: 1, Unknown: 0, CanonicalL1: 3}
	if snap.Counts != wantCounts {
		t.Errorf("counts = %+v, want %+v", snap.Counts, wantCounts)
	}
	wantL1 := map[string]int{"cement": 1, "semiconductor": 2, "optoelectronics": 1}
	if !reflect.DeepEqual(snap.L1Counts, wantL1) {
		t.Errorf("l1_counts = %v, want %v", snap.L1Counts, wantL1)
	}

	// entries 必須依代號遞增（檔案內容與上游列序無關）。
	wantOrder := []string{"1101", "1584", "2330", "3105", "8069"}
	gotOrder := make([]string, 0, len(snap.Entries))
	for _, e := range snap.Entries {
		gotOrder = append(gotOrder, e.Symbol)
		if e.AsOf != "2026-09-24" {
			t.Errorf("%s as_of = %q, want 2026-09-24", e.Symbol, e.AsOf)
		}
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("entry order = %v, want %v", gotOrder, wantOrder)
	}

	cement := findSymbolIndustryEntry(t, snap, "1101")
	if cement.IndustryCode != "01" || cement.CanonicalL1 != "cement" ||
		cement.MappingStatus != "mapped" || cement.IndustryNameZH != "水泥工業" ||
		cement.Market != "TWSE" || cement.Source != "TWSE:t187ap03_L" {
		t.Errorf("1101 mapped wrong: %+v", cement)
	}
	tsmc := findSymbolIndustryEntry(t, snap, "2330")
	if tsmc.IndustryCode != "24" || tsmc.CanonicalL1 != "semiconductor" ||
		tsmc.IndustryNameZH != "半導體業" || tsmc.MappingStatus != "mapped" {
		t.Errorf("2330 mapped wrong: %+v", tsmc)
	}
	opto := findSymbolIndustryEntry(t, snap, "8069")
	if opto.CanonicalL1 != "optoelectronics" || opto.IndustryCode != "26" ||
		opto.Market != "TPEx" || opto.Source != "TPEx:mopsfin_t187ap03_O" {
		t.Errorf("8069 mapped wrong: %+v", opto)
	}
	unmappedRow := findSymbolIndustryEntry(t, snap, "1584")
	if unmappedRow.IndustryCode != "20" || unmappedRow.MappingStatus != "unmapped" ||
		unmappedRow.CanonicalL1 != "" || unmappedRow.IndustryNameZH != "其他業" {
		t.Errorf("1584 disposition wrong: %+v", unmappedRow)
	}

	// unmapped 清單：代號 20、1 檔、附理由；unknown 清單為空。
	if len(snap.UnmappedCodes) != 1 {
		t.Fatalf("unmapped_codes = %+v, want exactly one entry", snap.UnmappedCodes)
	}
	if got := snap.UnmappedCodes[0]; got.Code != "20" || got.Count != 1 ||
		got.Name != "其他業" || !strings.Contains(got.Reason, "其他業") {
		t.Errorf("unmapped_codes[0] = %+v", got)
	}
	if len(snap.UnknownCodes) != 0 {
		t.Errorf("unknown_codes = %+v, want empty", snap.UnknownCodes)
	}
	// unmapped 的代號不可出現在 L1Counts（否則未對映的股票會憑空產生 L1 曝險）。
	if _, ok := snap.L1Counts["other"]; ok {
		t.Errorf("l1_counts leaked an entry for the unmapped code: %v", snap.L1Counts)
	}

	// 落檔 + 重讀。
	wantFile := filepath.Join(dir, "symbol_industry.json")
	if got := p.StateFile(); got != wantFile {
		t.Fatalf("StateFile = %q, want %q", got, wantFile)
	}
	loaded, err := p.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if !reflect.DeepEqual(loaded, snap) {
		t.Errorf("LoadSnapshot round-trip mismatch:\nloaded = %+v\nsnap   = %+v", loaded, snap)
	}
	if m.twseCalls.Load() != 1 || m.tpexCalls.Load() != 1 {
		t.Errorf("upstream calls = TWSE %d / TPEx %d, want 1 / 1",
			m.twseCalls.Load(), m.tpexCalls.Load())
	}
}

// ─── unknown（91 TDR）：回報 drift，不猜、不進 L1 ────────────────────────

func TestSymbolIndustryProvider_UnknownCodeIsReportedAsDrift(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{
			twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24"),
			twseIndustryRow("9103", "美德向邦醫療國際股份有限公司", "91"),
		},
		tpex: []map[string]string{},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	tdr := findSymbolIndustryEntry(t, snap, "9103")
	if tdr.MappingStatus != "unknown" {
		t.Errorf("9103 status = %q, want unknown", tdr.MappingStatus)
	}
	if tdr.CanonicalL1 != "" {
		t.Errorf("9103 canonical_l1 = %q, want empty (unknown codes must not be mapped)", tdr.CanonicalL1)
	}
	if !strings.Contains(tdr.MappingReason, "drift") {
		t.Errorf("9103 reason = %q, want it to name the upstream drift", tdr.MappingReason)
	}
	if tdr.IndustryNameZH != "" {
		t.Errorf("9103 name = %q, want empty (code 91 is not declared, so there is no name to report)", tdr.IndustryNameZH)
	}

	if snap.Counts.Unknown != 1 || snap.Counts.Mapped != 1 || snap.Counts.Unmapped != 0 {
		t.Errorf("counts = %+v, want mapped 1 / unknown 1", snap.Counts)
	}
	if _, ok := snap.L1Counts["semiconductor"]; !ok {
		t.Errorf("l1_counts = %v, want semiconductor from 2330", snap.L1Counts)
	}
	if len(snap.L1Counts) != 1 {
		t.Errorf("l1_counts = %v, want only semiconductor (91 must not contribute)", snap.L1Counts)
	}
	if len(snap.UnknownCodes) != 1 || snap.UnknownCodes[0].Code != "91" || snap.UnknownCodes[0].Count != 1 {
		t.Errorf("unknown_codes = %+v, want one entry for code 91", snap.UnknownCodes)
	}
	if !strings.Contains(snap.UnknownCodes[0].Reason, "drift") {
		t.Errorf("unknown_codes[0].reason = %q, want the drift text", snap.UnknownCodes[0].Reason)
	}
	// L1Counts 內不得出現「TDR 專屬」的桶；CanonicalL1 覆蓋數不因 unknown 增加。
	if snap.Counts.CanonicalL1 != 1 {
		t.Errorf("canonical_l1 = %d, want 1", snap.Counts.CanonicalL1)
	}
}

// ─── 同一代號出現在兩個市場：保留 TWSE 列 ───────────────────────────────

func TestSymbolIndustryProvider_DuplicateSymbolKeepsTWSERow(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{twseIndustryRow("2330", "上市 TSMC", "24")},
		tpex: []map[string]string{tpexIndustryRow("2330", "上櫃 TSMC", "20")},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(snap.Entries) != 1 {
		t.Fatalf("entries = %d, want 1 (deduped)", len(snap.Entries))
	}
	got := snap.Entries[0]
	if got.Market != "TWSE" || got.Source != "TWSE:t187ap03_L" || got.CompanyName != "上市 TSMC" {
		t.Errorf("kept row = %+v, want the TWSE row", got)
	}
	if got.CanonicalL1 != "semiconductor" {
		t.Errorf("canonical_l1 = %q, want semiconductor (from the TWSE code 24)", got.CanonicalL1)
	}
	if snap.Counts.Total != 1 || snap.Counts.Unmapped != 0 {
		t.Errorf("counts = %+v, want a single mapped row", snap.Counts)
	}
}

// ─── 單邊失敗：另一邊照樣寫檔，Sources 只列成功來源 ─────────────────────

func TestSymbolIndustryProvider_PartialUpstreamFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusTWSE int
		statusTPEx int
		wantSource []string
		wantSymbol string
	}{
		{
			name:       "TWSE down",
			statusTWSE: http.StatusInternalServerError,
			wantSource: []string{"TPEx:mopsfin_t187ap03_O"},
			wantSymbol: "3105",
		},
		{
			name:       "TPEx down",
			statusTPEx: http.StatusInternalServerError,
			wantSource: []string{"TWSE:t187ap03_L"},
			wantSymbol: "2330",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
				twse:       []map[string]string{twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24")},
				tpex:       []map[string]string{tpexIndustryRow("3105", "穩懋半導體股份有限公司", "24")},
				statusTWSE: tc.statusTWSE,
				statusTPEx: tc.statusTPEx,
			})
			p := newSymbolIndustryTestProvider(t, m, dir)

			snap, err := p.FetchAll(context.Background())
			if err != nil {
				t.Fatalf("FetchAll with one upstream down: %v (partial coverage is better than none)", err)
			}
			if !reflect.DeepEqual(snap.Sources, tc.wantSource) {
				t.Errorf("sources = %v, want %v", snap.Sources, tc.wantSource)
			}
			if len(snap.Entries) != 1 || snap.Entries[0].Symbol != tc.wantSymbol {
				t.Fatalf("entries = %+v, want only %s", snap.Entries, tc.wantSymbol)
			}
			if snap.Counts.Mapped != 1 {
				t.Errorf("counts = %+v, want 1 mapped row", snap.Counts)
			}
			if _, err := p.LoadSnapshot(); err != nil {
				t.Errorf("partial fetch must still persist the surviving market: %v", err)
			}
		})
	}
}

// ─── 兩邊都失敗：joined error，不寫檔 ───────────────────────────────────

func TestSymbolIndustryProvider_BothUpstreamsFail(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse:       []map[string]string{twseIndustryRow("2330", "台積電", "24")},
		tpex:       []map[string]string{tpexIndustryRow("3105", "穩懋", "24")},
		statusTWSE: http.StatusInternalServerError,
		statusTPEx: http.StatusInternalServerError,
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err == nil {
		t.Fatal("FetchAll with both upstreams down = nil error, want failure")
	}
	if snap != nil {
		t.Errorf("snapshot = %+v, want nil", snap)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("err = %v, want it to wrap ErrUpstream (typed, so severity/breaker classify it)", err)
	}
	// joined error 必須同時帶兩邊的失敗，否則只會看到一半的原因。
	msg := err.Error()
	if !strings.Contains(msg, symbolIndustryTWSEReportPath) || !strings.Contains(msg, symbolIndustryTPExReportPath) {
		t.Errorf("err = %q, want both upstream paths named", msg)
	}
	if _, statErr := os.Stat(p.StateFile()); !os.IsNotExist(statErr) {
		t.Errorf("state file must not be written when both upstreams fail (stat err = %v)", statErr)
	}
}

// ─── 空 payload 不是成功（#1953 DegradedOnEmpty）─────────────────────────

func TestSymbolIndustryProvider_EmptyPayloadIsNoData(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{},
		tpex: []map[string]string{},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err == nil {
		t.Fatal("FetchAll on empty payloads = nil error, want ErrNoData")
	}
	if !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want errors.Is(err, ErrNoData)", err)
	}
	if snap != nil {
		t.Errorf("snapshot = %+v, want nil", snap)
	}
	if _, statErr := os.Stat(p.StateFile()); !os.IsNotExist(statErr) {
		t.Errorf("an empty payload must not create a state file (stat err = %v)", statErr)
	}
}

// ─── 有股票、沒有產業別：unknown 且保留（不可靜默丟掉）──────────────────

func TestSymbolIndustryProvider_RowWithoutIndustryCodeIsUnknown(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{twseIndustryRow("1234", "某某公司", "  ")},
		tpex: []map[string]string{},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(snap.Entries) != 1 {
		t.Fatalf("entries = %+v, want the symbol kept", snap.Entries)
	}
	got := snap.Entries[0]
	if got.Symbol != "1234" || got.IndustryCode != "" || got.MappingStatus != "unknown" ||
		got.CanonicalL1 != "" || got.IndustryNameZH != "" {
		t.Errorf("row without industry code = %+v", got)
	}
	if got.MappingReason != symbolIndustryReasonNoCode {
		t.Errorf("reason = %q, want %q", got.MappingReason, symbolIndustryReasonNoCode)
	}
	if snap.Counts.Unknown != 1 || snap.Counts.Total != 1 {
		t.Errorf("counts = %+v, want one unknown row", snap.Counts)
	}
	if len(snap.UnknownCodes) != 1 || snap.UnknownCodes[0].Reason != symbolIndustryReasonNoCode {
		t.Errorf("unknown_codes = %+v, want the no-code disposition", snap.UnknownCodes)
	}
	if len(snap.L1Counts) != 0 {
		t.Errorf("l1_counts = %v, want empty", snap.L1Counts)
	}
}

// ─── 中文彙總列（非英數代號）防禦性丟棄 ────────────────────────────────

func TestSymbolIndustryProvider_DropsNonAlnumSummaryRow(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{
			twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24"),
			twseIndustryRow("合計", "上市合計", ""),
		},
		tpex: []map[string]string{},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(snap.Entries) != 1 || snap.Entries[0].Symbol != "2330" {
		t.Fatalf("entries = %+v, want only 2330", snap.Entries)
	}
	if snap.Counts.Total != 1 {
		t.Errorf("counts = %+v, want 1 row", snap.Counts)
	}
}

// ─── 沒有檔 / 停用持久化時 LoadSnapshot 必須報錯 ────────────────────────

func TestSymbolIndustryProvider_LoadSnapshotMissingFile(t *testing.T) {
	p := NewSymbolIndustryProvider(t.TempDir())
	snap, err := p.LoadSnapshot()
	if err == nil {
		t.Fatalf("LoadSnapshot on missing file = %+v, want error", snap)
	}
	if snap != nil {
		t.Errorf("snapshot = %+v, want nil", snap)
	}
	if !strings.Contains(err.Error(), "symbol_industry.json") {
		t.Errorf("err = %v, want the state file path in the message", err)
	}
}

func TestSymbolIndustryProvider_NoStorageDirDisablesPersistence(t *testing.T) {
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		twse: []map[string]string{twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24")},
		tpex: []map[string]string{tpexIndustryRow("3105", "穩懋半導體股份有限公司", "24")},
	})
	p := newSymbolIndustryTestProvider(t, m, "")

	if got := p.StateFile(); got != "" {
		t.Errorf("StateFile = %q, want empty when persistence is disabled", got)
	}
	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll without persistence: %v", err)
	}
	if len(snap.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(snap.Entries))
	}
	if _, err := p.LoadSnapshot(); err == nil {
		t.Error("LoadSnapshot with persistence disabled = nil error, want error")
	}
}

// ─── 決定性：同一時鐘下兩次 FetchAll 產出逐位元相同的檔 ────────────────

func TestSymbolIndustryProvider_DeterministicOutput(t *testing.T) {
	dir := t.TempDir()
	m := newSymbolIndustryMockServer(t, symbolIndustryMockOpts{
		// 上游列序刻意與排序後的順序不同（且 TPEx 先），驗證輸出與列序無關。
		twse: []map[string]string{
			twseIndustryRow("9103", "美德向邦醫療國際股份有限公司", "91"),
			twseIndustryRow("2330", "台灣積體電路製造股份有限公司", "24"),
			twseIndustryRow("1101", "臺灣水泥股份有限公司", "01"),
		},
		tpex: []map[string]string{
			tpexIndustryRow("1584", "精剛精密科技股份有限公司", "20"),
			tpexIndustryRow("3105", "穩懋半導體股份有限公司", "24"),
		},
	})
	p := newSymbolIndustryTestProvider(t, m, dir)

	if _, err := p.FetchAll(context.Background()); err != nil {
		t.Fatalf("first FetchAll: %v", err)
	}
	first, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if _, err := p.FetchAll(context.Background()); err != nil {
		t.Fatalf("second FetchAll: %v", err)
	}
	second, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatalf("re-read state file: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("two runs with the same clock differ (len %d vs %d)", len(first), len(second))
	}
	// 檔案裡不該出現 map 迭代順序的痕跡：entries 依代號遞增。
	var decoded SymbolIndustrySnapshot
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("decode state file: %v", err)
	}
	for i := 1; i < len(decoded.Entries); i++ {
		if decoded.Entries[i-1].Symbol > decoded.Entries[i].Symbol {
			t.Fatalf("entries not sorted: %s before %s",
				decoded.Entries[i-1].Symbol, decoded.Entries[i].Symbol)
		}
	}
	if len(decoded.UnknownCodes) != 1 || decoded.UnknownCodes[0].Code != "91" {
		t.Errorf("unknown_codes = %+v, want code 91", decoded.UnknownCodes)
	}
	if len(decoded.UnmappedCodes) != 1 || decoded.UnmappedCodes[0].Code != "20" {
		t.Errorf("unmapped_codes = %+v, want code 20", decoded.UnmappedCodes)
	}
}

// ─── 設定面：預設 base、尾斜線、Name ────────────────────────────────────

func TestSymbolIndustryProvider_Defaults(t *testing.T) {
	p := NewSymbolIndustryProvider("")
	if got := p.Name(); got != "symbol_industry" {
		t.Errorf("Name = %q, want symbol_industry", got)
	}
	if p.twseBase != symbolIndustryTWSEDefaultBaseURL || p.tpexBase != symbolIndustryTPExDefaultBaseURL {
		t.Errorf("default bases = %q / %q", p.twseBase, p.tpexBase)
	}
	// 空字串代表「保留預設」（測試只換 host 時不必重寫路徑）。
	p.SetBaseURLs("", "")
	if p.twseBase != symbolIndustryTWSEDefaultBaseURL || p.tpexBase != symbolIndustryTPExDefaultBaseURL {
		t.Errorf("empty override changed the bases: %q / %q", p.twseBase, p.tpexBase)
	}
	// 尾斜線要吃掉，否則會組出 //opendata/...。
	p.SetBaseURLs("http://127.0.0.1:1/v1/", "http://127.0.0.1:1/v1/")
	if p.twseBase != "http://127.0.0.1:1/v1" || p.tpexBase != "http://127.0.0.1:1/v1" {
		t.Errorf("trailing slash not trimmed: %q / %q", p.twseBase, p.tpexBase)
	}
	if p.StateFile() != "" {
		t.Errorf("StateFile = %q, want empty", p.StateFile())
	}
	// 上游路徑片段是契約的一部分（SetBaseURLs 只換 base）。
	if symbolIndustryTWSEReportPath != "/opendata/t187ap03_L" ||
		symbolIndustryTPExReportPath != "/mopsfin_t187ap03_O" {
		t.Errorf("upstream paths drifted: %q / %q",
			symbolIndustryTWSEReportPath, symbolIndustryTPExReportPath)
	}
}
