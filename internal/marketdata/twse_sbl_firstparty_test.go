package marketdata

// fix/20260924-finmind-quota — twse_sbl 第一方來源（TWSE TWT93U + TPEx
// margin/sbl）測試；FU-20260929-02 起含供給面來源 TWSE TWT72U。
//
// 契約（2026-09-24 / 2026-10-16 對真實端點實測，見 twse_sbl_firstparty.go 檔頭）：
//   - 上市：TWSE exchangeReport/TWT93U，stat=OK、fields 15 欄、data[][15]
//   - 上櫃：TPEx www/zh-tw/margin/sbl，tables[0].fields/data 同構
//   - 借券半部欄位：當日賣出(9) → SBLShortVolume、當日還券(10) →
//     SBLReturnVolume、當日餘額(12) → SBLShortBalance
//   - 供給面：TWSE exchangeReport/TWT72U（借券餘額合計表，上市 + 上櫃），
//     fields 的第 5 欄「本日借券餘額股(4)=(1)+(2)-(3)」→ SBLBorrowBalance
//   - 非交易日：三張表都回 stat 提示 + data 空（假日/週末/尚未發布）

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// sblFinMindRowsJSON 是 FinMind fallback 的 mock 資料（單日、單列）。
const sblFinMindRowsJSON = `[
	{"date":"2026-09-23","stock_id":"2330","SBLShortSalesCurrentDayBalance":130,"SBLShortSalesShortSales":20,"SBLShortSalesReturns":30}
]`

// twt72uFields 是 TWSE「證交所借券系統與證商/證金營業處所借券餘額合計表」
// （exchangeReport/TWT72U）的真實 9 欄表頭 —— 逐字取自 2026-09-16 的官方回應
// （FU-20260929-02：供給面借券餘額的唯一來源）。
var twt72uFields = []string{
	"證券代號", "證券名稱", "前日借券餘額(1)股", "本日異動股借券(2)", "本日異動股還券(3)",
	"本日借券餘額股(4)=(1)+(2)-(3)", "本日收盤價(5)單位：元", "借券餘額市值單位：元(6)=(4)*(5)", "市場別",
}

// sblBorrowRow 依 TWT72U 的 9 欄順序組一列（欄序同真實回應）。
func sblBorrowRow(symbol, name string, prev, delta, ret, balance int64, market string) []string {
	return []string{
		symbol, name, fmt.Sprint(prev), fmt.Sprint(delta), fmt.Sprint(ret),
		fmt.Sprint(balance), "1.00", "0", market,
	}
}

// sblFields 是兩家交易所共用的 14 欄表頭（融資半部 + 借券半部）。
var sblFields = []string{
	"代號", "名稱", "前日餘額", "賣出", "買進", "現券", "今日餘額", "次一營業日限額",
	"前日餘額", "當日賣出", "當日還券", "當日調整", "當日餘額", "次一營業日可限額", "備註",
}

// sblRow 依 14 欄順序組一列：融券半部(前日/賣出/買進/現券/餘額/限額) +
// 借券半部(前日/賣出/還券/調整/餘額/可限額)。融券半部刻意填入與借券半部不同
// 的數字（101..103 vs 1..3），這樣「抓錯欄位群組」一定會在斷言中現形。
func sblRow(symbol, name string, sblSell, sblReturn, sblBalance int64) []string {
	return []string{
		symbol, name, "1", "2", "3", "4", "5", "6",
		"7", fmt.Sprint(sblSell), fmt.Sprint(sblReturn), "8", fmt.Sprint(sblBalance), "9", " ",
	}
}

// sblMockDay 是 mock 端點要服務的一天。
//
// borrow = TWT72U 的借券餘額列。nil 且 noBorrow=false 時，mock 會用 twse+tpex 的
// 代號自動造出對應列（借券餘額 = 借券賣出餘額 × 2，市場別依來源主機）——真實世界
// 的 TWT72U 涵蓋上市 + 上櫃，因此預設讓每個有資料的證券都有借券餘額，測試才不會
// 每個都變成「TWT72U 未發布」。noBorrow=true 用來模擬「TWT93U 已發布、TWT72U 尚未
// 發布」的日報未齊備情境。
type sblMockDay struct {
	twse     []SBLStatsLike
	tpex     []SBLStatsLike
	borrow   []sblBorrowLike
	noBorrow bool
}

// sblBorrowLike 是 mock 的一列借券餘額（TWT72U 第 6 欄）。
type sblBorrowLike struct {
	Symbol  string
	Balance int64
	Market  string
}

// borrowRows 是 mock 要回的 TWT72U 列：明確指定優先，否則由 twse/tpex 推導。
func (d sblMockDay) borrowRows() [][]string {
	if d.noBorrow {
		return nil
	}
	if d.borrow != nil {
		rows := make([][]string, 0, len(d.borrow))
		for _, b := range d.borrow {
			rows = append(rows, sblBorrowRow(b.Symbol, "名稱"+b.Symbol, b.Balance, 0, 0, b.Balance, b.Market))
		}
		return rows
	}
	rows := make([][]string, 0, len(d.twse)+len(d.tpex))
	for _, s := range d.twse {
		rows = append(rows, sblBorrowRow(s.Symbol, "上市"+s.Symbol, s.Balance*2, 0, 0, s.Balance*2, "集中市場"))
	}
	for _, s := range d.tpex {
		rows = append(rows, sblBorrowRow(s.Symbol, "上櫃"+s.Symbol, s.Balance*2, 0, 0, s.Balance*2, "櫃檯買賣中心"))
	}
	return rows
}

// SBLStatsLike 避免與 production 型別混淆：測試資料用最小三元組 + 代號。
type SBLStatsLike struct {
	Symbol  string
	Sell    int64
	Return  int64
	Balance int64
}

// tpexSBLFields 是 TPEx 的表頭：注意融券半部的餘額欄也叫「當日餘額」
// （index 6），與借券半部的「當日餘額」（index 12）同名 —— 這是最容易抓錯群組
// 的地方，必須靠「取最後一次出現」+ 位置順序檢查處理。
var tpexSBLFields = []string{
	"股票代號", "股票名稱", "前日餘額", "賣出", "買進", "現券", "當日餘額", "限額",
	"前日餘額", "當日賣出", "當日還券", "當日調整數額", "當日餘額", "次一營業日可借券賣出限額", "備註",
}

// sblMockServer 同時扮演 TWSE 與 TPEx（不同 path），並計數兩邊的請求數。
type sblMockServer struct {
	srv        *httptest.Server
	twseCalls  atomic.Int64
	tpexCalls  atomic.Int64
	twt72Calls atomic.Int64
	// failTWSE / failTPEx / failTWT72 讓指定端點回 500（測部分失敗與全失敗）。
	failTWSE  bool
	failTPEx  bool
	failTWT72 bool
	// layoutBreak / layoutBreakTWT72 讓上游回傳不符預期的表頭
	// （測「不可靜默產生空資料」）。
	layoutBreak      bool
	layoutBreakTWT72 bool
	days             map[string]sblMockDay // key = "20260923"
}

func newSBLMockServer(t *testing.T, days map[string]sblMockDay) *sblMockServer {
	t.Helper()
	m := &sblMockServer{days: days}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/exchangeReport/TWT93U"):
			m.twseCalls.Add(1)
			if m.failTWSE {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			day := r.URL.Query().Get("date")
			rows := [][]string{}
			if d, ok := m.days[day]; ok {
				for _, s := range d.twse {
					rows = append(rows, sblRow(s.Symbol, "上市"+s.Symbol, s.Sell, s.Return, s.Balance))
				}
			}
			fields := sblFields
			if m.layoutBreak {
				fields = []string{"代號", "名稱", "foo", "bar"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stat": "OK", "date": day, "fields": fields, "data": rows,
			})
		case strings.Contains(r.URL.Path, "/exchangeReport/TWT72U"):
			m.twt72Calls.Add(1)
			if m.failTWT72 {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			day := r.URL.Query().Get("date")
			rows := [][]string{}
			if d, ok := m.days[day]; ok {
				rows = d.borrowRows()
			}
			fields := twt72uFields
			if m.layoutBreakTWT72 {
				fields = []string{"證券代號", "證券名稱", "foo"}
			}
			if len(rows) == 0 {
				// 真實上游對「無資料」的回應：stat 是提示訊息、無 fields/data。
				_ = json.NewEncoder(w).Encode(map[string]any{
					"stat": "很抱歉，沒有符合條件的資料!",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stat": "OK", "date": day, "fields": fields, "data": rows,
			})
		case strings.Contains(r.URL.Path, "/www/zh-tw/margin/sbl"):
			m.tpexCalls.Add(1)
			if m.failTPEx {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			day := strings.ReplaceAll(r.URL.Query().Get("date"), "/", "")
			rows := [][]string{}
			if d, ok := m.days[day]; ok {
				for _, s := range d.tpex {
					rows = append(rows, sblRow(s.Symbol, "上櫃"+s.Symbol, s.Sell, s.Return, s.Balance))
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stat": "ok", "date": day,
				"tables": []map[string]any{{
					"title": "信用額度總量管制餘額表", "date": day,
					// TPEx 的真實表頭（融券半部也稱「當日餘額」）。
					"fields": tpexSBLFields, "data": rows,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// newSBLProviderOnMock 建 provider 指向 mock，並把兩個共用桶換成無限速率
// （避免測試真的照 3 req/5s 政策等待）。
func newSBLProviderOnMock(t *testing.T, m *sblMockServer) *TWSESBLProvider {
	t.Helper()
	oldTPEx := SetTPExSharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTPExSharedLimiterForTest(oldTPEx) })
	p := NewTWSESBLProvider(0.5)
	p.SetHTTPClient(m.srv.Client())
	p.SetBaseURL(m.srv.URL)
	p.SetTPExBaseURL(m.srv.URL)
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	return p
}

// fmtPtr 讓 nil 指標的失敗訊息可讀（借券餘額是 *int64：nil = 不可用）。
func fmtPtr(p *int64) string {
	if p == nil {
		return "nil(不可用)"
	}
	return fmt.Sprint(*p)
}

func TestTWSESBL_FirstParty_MergesTWSEAndTPEx(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {
			twse: []SBLStatsLike{{Symbol: "2330", Sell: 11000, Return: 40000, Balance: 16804500}},
			tpex: []SBLStatsLike{{Symbol: "5206", Sell: 3000, Return: 0, Balance: 168000}},
		},
	})
	p := newSBLProviderOnMock(t, m)

	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("FetchSBLSummary: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("stats = %d, want 2 (上市 + 上櫃)", len(stats))
	}
	if stats[0].Symbol != "2330" || stats[0].SBLShortBalance != 16804500 ||
		stats[0].SBLShortVolume != 11000 || stats[0].SBLReturnVolume != 40000 ||
		stats[0].Date != "2026-09-23" {
		t.Errorf("TWSE row mapping wrong: %+v", stats[0])
	}
	if stats[1].Symbol != "5206" || stats[1].SBLShortBalance != 168000 || stats[1].SBLShortVolume != 3000 {
		t.Errorf("TPEx row mapping wrong: %+v", stats[1])
	}
	// 借券餘額（供給面）必須來自 TWT72U，且上市 + 上櫃兩邊都要拿到。
	if b := stats[0].SBLBorrowBalance; b == nil || *b != 16804500*2 {
		t.Errorf("TWSE row borrow balance = %v, want %d (from TWT72U)", fmtPtr(b), 16804500*2)
	}
	if b := stats[1].SBLBorrowBalance; b == nil || *b != 168000*2 {
		t.Errorf("TPEx row borrow balance = %v, want %d (TWT72U covers 櫃檯買賣中心 too)", fmtPtr(b), 168000*2)
	}
	if got := p.LastSource(); got != "TWSE:TWT93U+TPEx:margin/sbl+TWSE:TWT72U" {
		t.Errorf("LastSource = %q, want all three official tables", got)
	}
	if _, lastErr := p.LastFetchState(); lastErr != "" {
		t.Errorf("lastErr = %q, want empty after success", lastErr)
	}
}

// 兩個交易所的表頭都必須對到「借券半部」：TWSE 用「今日餘額/當日餘額」區分，
// TPEx 兩組同名。斷言值刻意與融券半部（1..9）不同，抓錯群組必失敗。
func TestSBLColumns_BothExchangeHeaderShapesResolveSBLHalf(t *testing.T) {
	row := sblRow("2330", "台積電", 11, 22, 33)

	twseStats, ok := parseSBLRows(sblFields, [][]string{row}, "2026-09-23")
	if !ok || len(twseStats) != 1 {
		t.Fatalf("TWSE header: ok=%v stats=%d", ok, len(twseStats))
	}
	if twseStats[0].SBLShortVolume != 11 || twseStats[0].SBLReturnVolume != 22 || twseStats[0].SBLShortBalance != 33 {
		t.Errorf("TWSE header mapped the wrong column group: %+v", twseStats[0])
	}

	tpexStats, ok := parseSBLRows(tpexSBLFields, [][]string{row}, "2026-09-23")
	if !ok || len(tpexStats) != 1 {
		t.Fatalf("TPEx header: ok=%v stats=%d", ok, len(tpexStats))
	}
	if tpexStats[0].SBLShortVolume != 11 || tpexStats[0].SBLReturnVolume != 22 || tpexStats[0].SBLShortBalance != 33 {
		t.Errorf("TPEx header (duplicate 當日餘額) mapped the wrong column group: %+v", tpexStats[0])
	}

	// 上游若把欄位換成只有融券半部（單一「當日餘額」在 index 6）→ 必須拒絕。
	marginOnly := []string{"代號", "名稱", "前日餘額", "賣出", "買進", "現券", "當日餘額", "限額"}
	if _, ok := parseSBLRows(marginOnly, [][]string{{"2330", "台積電", "1", "2", "3", "4", "5", "6"}}, "2026-09-23"); ok {
		t.Error("margin-only header must be rejected (no SBL 當日賣出/當日還券)")
	}
}

// 關鍵回歸：第一方成功時不得碰 FinMind（= 零配額消耗）。
func TestTWSESBL_FirstParty_SuccessConsumesNoFinMindQuota(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 1}}},
	})
	var finmindCalls atomic.Int64
	fmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finmindCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "data": []map[string]any{}})
	}))
	t.Cleanup(fmSrv.Close)
	fm := NewFinMindClientWithStateDir("", t.TempDir())
	fm.SetBaseURL(fmSrv.URL)
	fm.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))

	p := newSBLProviderOnMock(t, m)
	p.SetFinMindClient(fm)

	if _, err := p.FetchSBLSummary(context.Background(), "20260923"); err != nil {
		t.Fatalf("FetchSBLSummary: %v", err)
	}
	if n := finmindCalls.Load(); n != 0 {
		t.Fatalf("FinMind called %d times on the first-party path, want 0", n)
	}
	if got := fm.QuotaUsed(); got != 0 {
		t.Fatalf("FinMind quota used = %d, want 0", got)
	}
	if m.twseCalls.Load() == 0 || m.tpexCalls.Load() == 0 {
		t.Errorf("expected both official hosts to be called: twse=%d tpex=%d", m.twseCalls.Load(), m.tpexCalls.Load())
	}
}

// 假日/尚未發布：兩家都空 → 回探到有資料的交易日（契約不變）。
func TestTWSESBL_FirstParty_WalksBackToLatestReportDay(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		// 目標日 9/24 尚未發布（兩家都空）；9/23 有資料。
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	p := newSBLProviderOnMock(t, m)

	stats, err := p.FetchSBLSummary(context.Background(), "20260924")
	if err != nil {
		t.Fatalf("FetchSBLSummary walk-back: %v", err)
	}
	if len(stats) != 1 || stats[0].Date != "2026-09-23" {
		t.Fatalf("walk-back should land on the 2026-09-23 report, got %+v", stats)
	}
}

// 上市主機整日失敗、上櫃正常 → 仍回傳上櫃資料（部分覆蓋勝過全空），並記 warn。
func TestTWSESBL_FirstParty_PartialHostFailureStillReturnsData(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {
			twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}},
			tpex: []SBLStatsLike{{Symbol: "5206", Balance: 200}},
		},
	})
	m.failTWSE = true
	p := newSBLProviderOnMock(t, m)

	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("partial failure should still return TPEx data: %v", err)
	}
	if len(stats) != 1 || stats[0].Symbol != "5206" {
		t.Fatalf("stats = %+v, want only the TPEx row", stats)
	}
	if got := p.LastSource(); got != "TPEx:margin/sbl" {
		t.Errorf("LastSource = %q, want TPEx only", got)
	}
}

// 兩家都失敗且無 FinMind fallback → 回傳錯誤（真故障，不是靜默成功）。
func TestTWSESBL_FirstParty_BothHostsFailNoFallbackErrors(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.failTWSE, m.failTPEx = true, true
	p := newSBLProviderOnMock(t, m)

	if _, err := p.FetchSBLSummary(context.Background(), "20260923"); err == nil {
		t.Fatal("expected error when both official hosts fail and no FinMind fallback is wired")
	}
	if _, lastErr := p.LastFetchState(); lastErr == "" {
		t.Error("lastErr should be recorded on failure")
	}
}

// 兩家都失敗但 FinMind 可用 → 退回路徑成功，provenance = finmind-fallback。
func TestTWSESBL_FirstParty_BothHostsFailFallsBackToFinMind(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.failTWSE, m.failTPEx = true, true
	p := newSBLProviderOnMock(t, m)
	p.SetFinMindClient(finmindMock(t, sblFinMindRowsJSON))

	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("fallback fetch: %v", err)
	}
	if len(stats) != 1 || stats[0].Symbol != "2330" || stats[0].SBLShortBalance != 130 {
		t.Fatalf("fallback mapping wrong: %+v", stats)
	}
	if got := p.LastSource(); got != SBLSourceFinMind {
		t.Errorf("LastSource = %q, want %q", got, SBLSourceFinMind)
	}
}

// 配額耗盡（fallback 路徑，第一方關閉）→ 錯誤必須保留 ErrQuotaExhausted
// 語意，讓 gateway 分類成 warn、讓排程任務走「延後 + 重置後補抓」分支。
// 不得改成靜默成功（那會是「用忽略錯誤消除 warn」）。
func TestTWSESBL_FinMindFallback_QuotaExhaustedKeepsSentinel(t *testing.T) {
	p := NewTWSESBLProvider(0.5)
	p.SetFirstPartyEnabled(false)
	fm := NewFinMindClientWithStateDir("", t.TempDir())
	fm.SetQuotaLimit(0) // 額度已用完：AllowCall=false，不會發 HTTP
	p.SetFinMindClient(fm)

	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err == nil {
		t.Fatalf("expected ErrQuotaExhausted, got stats=%d", len(stats))
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("error must wrap ErrQuotaExhausted (gateway maps it to warn), got %v", err)
	}
	if _, lastErr := p.LastFetchState(); lastErr == "" {
		t.Error("lastErr should carry the quota reason for the channel health page")
	}
}

// 第一方關閉且沒有 FinMind client → 明確的「無來源」錯誤（不是空資料成功）。
func TestTWSESBL_NoSourceConfigured(t *testing.T) {
	p := NewTWSESBLProvider(0.5)
	p.SetFirstPartyEnabled(false)
	if _, err := p.FetchSBLSummary(context.Background(), "20260923"); err == nil {
		t.Fatal("expected ErrSBLNoSource")
	} else if !errors.Is(err, ErrSBLNoSource) {
		t.Fatalf("error = %v, want ErrSBLNoSource", err)
	}
	day, _ := time.Parse("2006-01-02", "2026-09-23")
	p.SetStorageDir(t.TempDir())
	if _, err := p.FetchSBLHistory(context.Background(), day, day); !errors.Is(err, ErrSBLNoSource) {
		t.Fatalf("history error = %v, want ErrSBLNoSource", err)
	}
}

// 上游改版（表頭不符）→ 必須回報錯誤，不可靜默產生空資料。
func TestTWSESBL_FirstParty_LayoutBreakIsAnError(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.layoutBreak = true
	p := newSBLProviderOnMock(t, m)

	if _, err := p.FetchSBLSummary(context.Background(), "20260923"); err == nil {
		t.Fatal("unexpected table layout must surface as an error")
	}
}

// 歷史回補走第一方：每個交易日最多 2 個官方 call、零 FinMind，且假日(空)略過。
func TestTWSESBL_FirstParty_HistoryWritesDayFiles(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}, tpex: []SBLStatsLike{{Symbol: "5206", Balance: 200}}},
		"20260924": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 110}}},
	})
	p := newSBLProviderOnMock(t, m)
	p.SetStorageDir(t.TempDir())

	start, _ := time.Parse("2006-01-02", "2026-09-22")
	end, _ := time.Parse("2006-01-02", "2026-09-25")
	written, err := p.FetchSBLHistory(context.Background(), start, end)
	if err != nil {
		t.Fatalf("FetchSBLHistory: %v", err)
	}
	if written != 2 {
		t.Fatalf("written = %d, want 2 day files (9/22 無資料=假日略過)", written)
	}
	twseBefore, tpexBefore := m.twseCalls.Load(), m.tpexCalls.Load()
	written2, err := p.FetchSBLHistory(context.Background(), start, end)
	if err != nil || written2 != 0 {
		t.Fatalf("idempotent re-run: written=%d err=%v, want 0/nil", written2, err)
	}
	// 已有日檔的交易日必須在發 HTTP 前就跳過（9/23、9/24 已寫檔）。
	// 仍需各探一次的是「沒有日檔可記」的日：9/22 假日（兩家都空）與 9/25
	// 尚未發布 —— 這類日子無法用檔案標記完成，只能重探。
	if got := m.twseCalls.Load() - twseBefore; got != 2 {
		t.Errorf("re-run TWSE calls = %d, want 2 (holiday 9/22 + unpublished 9/25 only)", got)
	}
	if got := m.tpexCalls.Load() - tpexBefore; got != 2 {
		t.Errorf("re-run TPEx calls = %d, want 2 (holiday 9/22 + unpublished 9/25 only)", got)
	}
	// 借券餘額（TWT72U）只在 TWT93U 有資料時才抓 ⇒ 重跑時兩個已寫檔的交易日都被
	// 跳過，無資料的 9/22/9/25 也不會多花這個 call。
	if got := m.twt72Calls.Load(); got != 2 {
		t.Errorf("total TWT72U calls = %d, want 2 (only the two persisted days)", got)
	}
	raw, _ := os.ReadFile(filepath.Join(p.storageDir, "20260923_sbl.json"))
	if !strings.Contains(string(raw), `"symbol": "2330"`) || !strings.Contains(string(raw), `"symbol": "5206"`) ||
		!strings.Contains(string(raw), `"sbl_short_balance": 100`) {
		t.Errorf("20260923 file content wrong: %s", raw)
	}
}

// 歷史回補：第一方失敗 + FinMind fallback 配額耗盡 → 錯誤保留 sentinel 且中止。
func TestTWSESBL_History_QuotaExhaustedFallbackPreservesSentinel(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{})
	m.failTWSE, m.failTPEx = true, true
	p := newSBLProviderOnMock(t, m)
	fm := NewFinMindClientWithStateDir("", t.TempDir())
	fm.SetQuotaLimit(0) // 配額耗盡：不發 HTTP，直接 ErrQuotaExhausted
	p.SetFinMindClient(fm)
	p.SetStorageDir(t.TempDir())

	day, _ := time.Parse("2006-01-02", "2026-09-23")
	_, err := p.FetchSBLHistory(context.Background(), day, day)
	if err == nil {
		t.Fatal("expected error when first-party fails and FinMind quota is exhausted")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("error must wrap ErrQuotaExhausted, got %v", err)
	}
}

// ── 供給面（借券餘額，TWT72U）契約測試 ──────────────────────────────────────
// FU-20260929-02：sbl_borrow_balance 曾是永遠 0 的 placeholder（沒有任何路徑
// 填過它），讓「供給面無法量測」被誤讀成「供給面為 0」。

// twt72uRealFixture 是 2026-09-16 官方回應的**逐字節錄**（表頭全部、列取 4 檔 +
// 中文彙總列），用來鎖住真實欄名而不只鎖測試自造的形狀。
const twt72uRealFixture = `{
  "stat": "OK",
  "date": "20260916",
  "title": "115年09月16日 證交所借券系統與證商/證金營業處所借券餘額合計表",
  "fields": ["證券代號", "證券名稱", "前日借券餘額(1)股", "本日異動股借券(2)", "本日異動股還券(3)", "本日借券餘額股(4)=(1)+(2)-(3)", "本日收盤價(5)單位：元", "借券餘額市值單位：元(6)=(4)*(5)", "市場別"],
  "data": [
    ["2301", "光寶科", "77,063,000", "2,232,000", "227,000", "79,068,000", "277.00", "21,901,836,000", "集中市場"],
    ["2330", "台積電", "122,610,000", "8,787,000", "71,704,000", "59,693,000", "2,380.00", "142,069,340,000", "集中市場"],
    ["8183", "精星", "18,000", "0", "18,000", "0", "28.00", "0", "櫃檯買賣中心"],
    ["合計", "　", "28,918,850,000", "810,737,000", "604,598,000", "29,124,989,000", "　", "3,302,036,995,470", "整體市場"]
  ]
}`

// 真實 fixture ⇒ 借券餘額必須逐值對上（含 0 與彙總列排除）。
func TestSBLBorrowRows_RealFixtureMapsSupplyColumn(t *testing.T) {
	var r twseSBLResponse
	if err := json.Unmarshal([]byte(twt72uRealFixture), &r); err != nil {
		t.Fatalf("fixture unmarshal: %v", err)
	}
	borrow, ok := parseSBLBorrowRows(r.Fields, r.Data)
	if !ok {
		t.Fatalf("real TWT72U header must parse: fields=%v", r.Fields)
	}
	want := map[string]int64{"2301": 79068000, "2330": 59693000, "8183": 0}
	if len(borrow) != len(want) {
		t.Fatalf("borrow rows = %v, want exactly %v (中文「合計」列必須被濾掉)", borrow, want)
	}
	for sym, w := range want {
		if got, ok := borrow[sym]; !ok || got != w {
			t.Errorf("borrow[%s] = %d (present=%v), want %d", sym, got, ok, w)
		}
	}
	// 真實表頭 index 2 是「前日借券餘額(1)股」、index 5 才是「本日借券餘額股(4)=…」；
	// 兩欄常同數字（本日 = 前日 + 借券 - 還券），所以斷言值刻意取自真實 fixture 中
	// 兩者不同的列（2330：前日 122,610,000 vs 本日 59,693,000）。抓錯欄必紅
	// （已用突變 `sblColBorrowPrefix = "前日借券餘額"` 驗證：borrow[2330] = 122610000）。
	if _, ok := borrow["合計"]; ok {
		t.Error("彙總列 must not be treated as a security")
	}
}

// 表頭契約：0 個或 2 個命中都必須拒絕（不可猜、不可靜默回空資料）。
func TestSBLBorrowColumn_HeaderContract(t *testing.T) {
	if i, ok := sblBorrowColumn(twt72uFields); !ok || i != 5 {
		t.Fatalf("real header: idx=%d ok=%v, want 5/true", i, ok)
	}
	if _, ok := sblBorrowColumn([]string{"證券代號", "證券名稱", "foo"}); ok {
		t.Error("header without 本日借券餘額 must be rejected")
	}
	dup := append(append([]string{}, twt72uFields...), "本日借券餘額股(4)=(1)+(2)+(3)")
	if _, ok := sblBorrowColumn(dup); ok {
		t.Error("duplicate 本日借券餘額 columns must be rejected (哪一欄才是借券餘額無法判斷)")
	}
}

// TWT72U 表上缺席的證券 ⇒ nil（不可用），**不是** 0。
// 實證（2026-09-16）：TWT93U 1302 檔中有 165 檔不在 TWT72U，其中借券賣出餘額非零
// 者 0 檔 ⇒ 缺席無法區分「0」與「不可得」，故不可填 0。
func TestSBLBorrowBalance_AbsentSymbolStaysNil(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {
			twse: []SBLStatsLike{
				{Symbol: "2330", Balance: 100},
				{Symbol: "1213", Balance: 700}, // 不在 TWT72U 表上
			},
			borrow: []sblBorrowLike{{Symbol: "2330", Balance: 59000000, Market: "集中市場"}},
		},
	})
	p := newSBLProviderOnMock(t, m)
	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("FetchSBLSummary: %v", err)
	}
	bySym := map[string]SBLStats{}
	for _, s := range stats {
		bySym[s.Symbol] = s
	}
	if b := bySym["2330"].SBLBorrowBalance; b == nil || *b != 59000000 {
		t.Errorf("2330 borrow = %v, want 59000000", fmtPtr(b))
	}
	if b := bySym["1213"].SBLBorrowBalance; b != nil {
		t.Errorf("1213 borrow = %d, want nil (TWT72U 表上缺席 ⇒ 不可用，不可寫 0)", *b)
	}
	raw, err := json.Marshal(bySym["1213"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"sbl_borrow_balance":null`) {
		t.Errorf("unavailable borrow balance must serialize as explicit null, got %s", raw)
	}
}

// TWT93U 有資料但 TWT72U 尚未發布 ⇒ 整日視為「日報未齊備」，回探上一日
// （與假日/未發布同一契約），而不是落地一個供給面全 null 且日後不再覆寫的日檔。
func TestSBLBorrowBalance_NotYetPublishedWalksBack(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
		"20260924": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 110}}, noBorrow: true},
	})
	p := newSBLProviderOnMock(t, m)
	stats, err := p.FetchSBLSummary(context.Background(), "20260924")
	if err != nil {
		t.Fatalf("walk-back on unpublished TWT72U: %v", err)
	}
	if len(stats) != 1 || stats[0].Date != "2026-09-23" {
		t.Fatalf("want the 2026-09-23 report, got %+v", stats)
	}
	if b := stats[0].SBLBorrowBalance; b == nil {
		t.Error("walked-back day must still carry a real borrow balance")
	}
	if got := p.LastSource(); got != "TWSE:TWT93U+TWSE:TWT72U" {
		t.Errorf("LastSource = %q, want the two TWSE tables", got)
	}
}

// 上游把 TWT72U 表頭改壞 ⇒ 必須回報錯誤（不可靜默讓供給面全 0）。
func TestSBLBorrowBalance_LayoutBreakIsAnError(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.layoutBreakTWT72 = true
	p := newSBLProviderOnMock(t, m)
	if _, err := p.FetchSBLSummary(context.Background(), "20260923"); err == nil {
		t.Fatal("unexpected TWT72U table layout must surface as an error")
	}
}

// TWT72U HTTP 失敗（同主機、TWT93U 成功）⇒ 錯誤，不可退化成「供給面 0」。
func TestSBLBorrowBalance_HTTPFailureIsAnError(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.failTWT72 = true
	p := newSBLProviderOnMock(t, m)
	_, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err == nil {
		t.Fatal("TWT72U HTTP failure must surface as an error")
	}
	if !strings.Contains(err.Error(), "TWT72U") {
		t.Errorf("error should name the failing table, got %v", err)
	}
}

// 上市主機失敗 ⇒ 同主機的 TWT72U 一併不可得：維持既有「部分覆蓋」語意（回傳上櫃
// 資料），供給面留 nil 並發 warn，而不是把整日丟掉或把供給面寫成 0。
func TestSBLBorrowBalance_TWSEHostDownKeepsPartialCoverage(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {
			twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}},
			tpex: []SBLStatsLike{{Symbol: "5206", Balance: 200}},
		},
	})
	m.failTWSE = true
	p := newSBLProviderOnMock(t, m)
	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("partial host failure must still return OTC data: %v", err)
	}
	if len(stats) != 1 || stats[0].Symbol != "5206" {
		t.Fatalf("stats = %+v, want only the TPEx row", stats)
	}
	if stats[0].SBLBorrowBalance != nil {
		t.Errorf("borrow = %d, want nil while the TWSE host is down", *stats[0].SBLBorrowBalance)
	}
	if n := m.twt72Calls.Load(); n != 0 {
		t.Errorf("TWT72U calls = %d, want 0 (same host as the failed TWT93U)", n)
	}
	if got := p.LastSource(); got != "TPEx:margin/sbl" {
		t.Errorf("LastSource = %q, want TPEx only", got)
	}
}

// FinMind fallback 沒有任何借券餘額欄位（實測 2026-10-16：dataset 只有 3 個 SBL
// 欄位）⇒ 供給面必須是 nil，不可用 0 冒充。
func TestSBLBorrowBalance_FinMindFallbackLeavesNil(t *testing.T) {
	m := newSBLMockServer(t, map[string]sblMockDay{
		"20260923": {twse: []SBLStatsLike{{Symbol: "2330", Balance: 100}}},
	})
	m.failTWSE, m.failTPEx = true, true
	p := newSBLProviderOnMock(t, m)
	p.SetFinMindClient(finmindMock(t, sblFinMindRowsJSON))

	stats, err := p.FetchSBLSummary(context.Background(), "20260923")
	if err != nil {
		t.Fatalf("fallback fetch: %v", err)
	}
	if len(stats) != 1 || stats[0].SBLBorrowBalance != nil {
		t.Fatalf("FinMind fallback has no borrow column: want nil, got %+v", stats)
	}
}
