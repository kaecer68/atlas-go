package marketdata

// FinMind TaiwanStockPrice **range** 抓取的契約測試（#2126 後續）。
//
// 這條路徑是 quotes 日線回補的地基：一次呼叫必須覆蓋整段查詢窗口
// （呼叫次數 = 檔數 × 1），且窗口外的列**不得**進入回傳值。
// 兩者各自有釘子，任一被改壞都必須紅燈。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// rangeTestServer 回傳一個 FinMind 形狀的假端點：先記錄本次請求的查詢參數與次數，
// 再回傳呼叫端給的列。query 以複本回報，避免測試在 handler 之外讀到共用 map。
func rangeTestServer(t *testing.T, rows string, queries *[]map[string]string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if queries != nil {
			q := r.URL.Query()
			copied := make(map[string]string, len(q))
			for k, v := range q {
				if len(v) > 0 {
					copied[k] = v[0]
				}
			}
			*queries = append(*queries, copied)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[` + rows + `]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newRangeTestClient(t *testing.T, srv *httptest.Server) *FinMindClient {
	t.Helper()
	c := newFinMindClientInternal("test-key", t.TempDir())
	c.SetBaseURL(srv.URL)
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return c
}

// row 造一列 TaiwanStockPrice 形狀的假資料（欄名與上游一致；整數價格）。
func row(date, stockID string, o, h, l, cl, vol float64) string {
	return `{"date":"` + date + `","stock_id":"` + stockID + `","open":` + ftoa(o) + `,"max":` + ftoa(h) +
		`,"min":` + ftoa(l) + `,"close":` + ftoa(cl) + `,"Trading_Volume":` + ftoa(vol) + `}`
}

func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// TestGetStockPriceRange_SingleRequestCarriesTheWholeWindow 是「range 形式」的釘子：
// 80 天的窗口必須只用**一次** HTTP 呼叫，且請求本身要帶完整的 start_date/end_date。
//
// 兩個突變各自會讓本測試紅燈：
//   - 改回 per-day（逐日呼叫）⇒ hits != 1。
//   - 把日期界拿掉（送空的 start_date/end_date）⇒ 參數斷言失敗。
func TestGetStockPriceRange_SingleRequestCarriesTheWholeWindow(t *testing.T) {
	var queries []map[string]string
	srv, hits := rangeTestServer(t, strings.Join([]string{
		row("2026-03-02", "2330", 1000, 1030, 995, 1020, 30000000),
		row("2026-03-03", "2330", 1020, 1040, 1010, 1035, 25000000),
	}, ","), &queries)

	c := newRangeTestClient(t, srv)
	bars, err := c.GetStockPriceRange(context.Background(), "2330.TW", "2026-03-02", "2026-06-24")
	if err != nil {
		t.Fatalf("GetStockPriceRange: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("HTTP hits = %d, want 1 — 一段窗口只能有一次呼叫（呼叫次數 = 檔數 × 1）", got)
	}
	if len(queries) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(queries))
	}
	q := queries[0]
	for _, want := range []struct{ key, value string }{
		{"dataset", "TaiwanStockPrice"},
		{"data_id", "2330"}, // .TW 後綴由 fetchDataset 正規化，上游只認裸代碼
		{"start_date", "2026-03-02"},
		{"end_date", "2026-06-24"},
	} {
		if q[want.key] != want.value {
			t.Errorf("request %s = %q, want %q（窗口必須由請求本身界定）", want.key, q[want.key], want.value)
		}
	}

	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(bars))
	}
	if bars[0].Date.Format("2006-01-02") != "2026-03-02" || bars[1].Date.Format("2006-01-02") != "2026-03-03" {
		t.Fatalf("bars not sorted by date ascending: %v %v", bars[0].Date, bars[1].Date)
	}
	first := bars[0]
	if first.Symbol != "2330.TW" {
		t.Errorf("Symbol = %q, want the caller's key form 2330.TW（quotes 的鍵由呼叫端決定）", first.Symbol)
	}
	if first.Open != 1000 || first.High != 1030 || first.Low != 995 || first.Close != 1020 {
		t.Errorf("OHLC = %v/%v/%v/%v, want 1000/1030/995/1020", first.Open, first.High, first.Low, first.Close)
	}
	if first.Volume != 30000000 {
		t.Errorf("Volume = %d, want 30000000", first.Volume)
	}
	if first.Source != stockPriceRangeSource {
		t.Errorf("Source = %q, want %q", first.Source, stockPriceRangeSource)
	}
}

// TestGetStockPriceRange_DropsRowsOutsideWindow 是第 2 層日期界的釘子。
//
// 動機（已實證的上游行為）：本平台看過 FinMind 的 dataset 忽略窗口
// （twse_sbl_provider.go：full-market query 只回 START 那天的列），所以呼叫端
// 不能只靠請求參數。突變：把 parseStockPriceRange 的日期界拿掉 ⇒ 本測試紅燈。
func TestGetStockPriceRange_DropsRowsOutsideWindow(t *testing.T) {
	srv, _ := rangeTestServer(t, strings.Join([]string{
		row("2026-02-20", "2330", 900, 910, 895, 905, 1000), // 窗口前
		row("2026-03-02", "2330", 1000, 1030, 995, 1020, 2000),
		row("2026-03-03", "2330", 1020, 1040, 1010, 1035, 3000),
		row("2026-07-01", "2330", 1100, 1110, 1090, 1105, 4000), // 窗口後
	}, ","), nil)

	c := newRangeTestClient(t, srv)
	bars, err := c.GetStockPriceRange(context.Background(), "2330", "2026-03-02", "2026-06-24")
	if err != nil {
		t.Fatalf("GetStockPriceRange: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2 — 窗口外的列必須被丟棄", len(bars))
	}
	for _, b := range bars {
		ds := b.Date.Format("2006-01-02")
		if ds != "2026-03-02" && ds != "2026-03-03" {
			t.Fatalf("窗口外的列進入了回傳值: %s", ds)
		}
	}
}

// TestGetStockPriceRange_DropsRowsOfOtherSymbols：逐檔查詢仍明確比對 stock_id。
// 上游若忽略 data_id（回全市場），別的股票的價格絕不能寫進這條序列。
func TestGetStockPriceRange_DropsRowsOfOtherSymbols(t *testing.T) {
	srv, _ := rangeTestServer(t, strings.Join([]string{
		row("2026-03-02", "2330", 1000, 1030, 995, 1020, 2000),
		row("2026-03-02", "2317", 200, 205, 198, 204, 9000),
		row("2026-03-03", "2330.TW", 1020, 1040, 1010, 1035, 3000), // 後綴形式也要視為同一檔
	}, ","), nil)

	c := newRangeTestClient(t, srv)
	bars, err := c.GetStockPriceRange(context.Background(), "2330.TW", "2026-03-02", "2026-06-24")
	if err != nil {
		t.Fatalf("GetStockPriceRange: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2（2317 的列必須被丟棄）", len(bars))
	}
	if bars[0].Close != 1020 {
		t.Fatalf("close = %v, want 1020 (2330 自己的收盤價)", bars[0].Close)
	}
}

// TestGetStockPriceRange_DedupesSameDayKeepingLast：同日重複列只留最後一筆
// （上游以日期遞增回傳，最後一筆是最新修訂）。
func TestGetStockPriceRange_DedupesSameDayKeepingLast(t *testing.T) {
	srv, _ := rangeTestServer(t, strings.Join([]string{
		row("2026-03-02", "2330", 1000, 1030, 995, 1020, 2000),
		row("2026-03-02", "2330", 1000, 1030, 995, 1025, 2100),
	}, ","), nil)

	c := newRangeTestClient(t, srv)
	bars, err := c.GetStockPriceRange(context.Background(), "2330", "2026-03-02", "2026-06-24")
	if err != nil {
		t.Fatalf("GetStockPriceRange: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars = %d, want 1（同日去重）", len(bars))
	}
	if bars[0].Close != 1025 || bars[0].Volume != 2100 {
		t.Fatalf("close/volume = %v/%d, want 1025/2100（後出現者為準）", bars[0].Close, bars[0].Volume)
	}
}

// TestGetStockPriceRange_EmptyResultIsTypedNoData：整段窗口都沒有列 ⇒
// ErrNoDataForSymbol（下市／代碼錯誤），呼叫端據此與傳輸錯誤分流。
func TestGetStockPriceRange_EmptyResultIsTypedNoData(t *testing.T) {
	srv, _ := rangeTestServer(t, "", nil)
	c := newRangeTestClient(t, srv)

	_, err := c.GetStockPriceRange(context.Background(), "9999", "2026-03-02", "2026-06-24")
	if !errors.Is(err, ErrNoDataForSymbol) {
		t.Fatalf("err = %v, want errors.Is(err, ErrNoDataForSymbol)", err)
	}
}

// TestGetStockPriceRange_RowsAllOutsideWindowIsNoData：上游回了列但全在窗口外
// （忽略窗口的行為）⇒ 結果為空，仍必須是可分類的 no-data，而不是「成功但 0 列」。
func TestGetStockPriceRange_RowsAllOutsideWindowIsNoData(t *testing.T) {
	srv, _ := rangeTestServer(t, row("2026-01-05", "2330", 1, 1, 1, 1, 1), nil)
	c := newRangeTestClient(t, srv)

	_, err := c.GetStockPriceRange(context.Background(), "2330", "2026-03-02", "2026-06-24")
	if !errors.Is(err, ErrNoDataForSymbol) {
		t.Fatalf("err = %v, want ErrNoDataForSymbol", err)
	}
	if !strings.Contains(err.Error(), "dropped_out_of_window=1") {
		t.Errorf("err = %v, want the dropped-row count echoed (可稽核，不靜默)", err)
	}
}

// TestGetStockPriceRange_UpstreamErrorPropagates：上游失敗要原樣傳出
// （含 typed sentinel），不能被誤判成 no-data。
func TestGetStockPriceRange_UpstreamErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"msg":"boom","status":500}`))
	}))
	t.Cleanup(srv.Close)
	c := newRangeTestClient(t, srv)

	_, err := c.GetStockPriceRange(context.Background(), "2330", "2026-03-02", "2026-06-24")
	if err == nil {
		t.Fatal("want error on upstream 500")
	}
	if errors.Is(err, ErrNoDataForSymbol) {
		t.Fatalf("err = %v must NOT be classified as no-data", err)
	}
}

// TestGetStockPriceRange_DateIsUTCMidnight 釘住日期語意。
//
// quotes 的三個 store 對日期的格式化方式不一致：sqlite/jsonl 用
// q.Date.Format(...)（時間自帶的時區），postgres 用 q.Date.UTC().Format(...)。
// 若 Date 是「台北午夜」，那兩個格式化會差一天（台北午夜 = 前一日 16:00Z），
// 同一份資料寫進 sqlite 與 postgres 會落在不同日。本 package 因此固定回傳
// **UTC 午夜**：兩個格式化都得到同一天。
//
// 突變：把 time.Parse 改成 time.ParseInLocation(..., TaiwanLocation()) ⇒ 本測試紅燈。
func TestGetStockPriceRange_DateIsUTCMidnight(t *testing.T) {
	srv, _ := rangeTestServer(t, row("2026-03-02", "2330", 1, 1, 1, 1, 1), nil)
	c := newRangeTestClient(t, srv)

	bars, err := c.GetStockPriceRange(context.Background(), "2330", "2026-03-02", "2026-06-24")
	if err != nil {
		t.Fatalf("GetStockPriceRange: %v", err)
	}
	day := bars[0].Date
	if day.UTC() != day {
		t.Fatalf("Date = %v (%v), want UTC midnight so every store formats the same day", day, day.Location())
	}
	if h, m, s := day.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("Date = %v, want midnight", day)
	}
	// 兩個 store 的格式化路徑都必須得到同一天（這才是要防的失效）。
	utcFormatted := day.UTC().Format("2006-01-02")
	localFormatted := day.Format("2006-01-02")
	if utcFormatted != "2026-03-02" || localFormatted != "2026-03-02" {
		t.Fatalf("store formatters disagree: UTC=%s Local=%s, want both 2026-03-02", utcFormatted, localFormatted)
	}
	// 反向對照（負控制）：台北午夜確實會差一天 —— 證明本測試不是恆真。
	taipeiMidnight, err := time.ParseInLocation("2006-01-02", "2026-03-02", TaiwanLocation())
	if err != nil {
		t.Fatalf("parse taipei midnight: %v", err)
	}
	if taipeiMidnight.UTC().Format("2006-01-02") == "2026-03-02" {
		t.Fatal("負控制失效：台北午夜的 UTC 日期竟然同一天，本測試失去意義")
	}
}
