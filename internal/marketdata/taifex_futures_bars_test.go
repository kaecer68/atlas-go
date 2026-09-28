package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ─── fixtures ───────────────────────────────────────────────────────────────
//
// 全部為 2026-09-28 對期交所**實跑**取得的原始 bytes（見
// docs/specs/futures-bars-firstparty-spec.md §12）。CSV 為 MS950/Big5 原文，
// 刻意不解碼存放，讓測試同時釘住 charset 轉碼。

func readFutFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// decodeFutFixture 依官網實際回傳的 Content-Type 轉碼。
func decodeFutFixture(t *testing.T, name string) []byte {
	t.Helper()
	out, err := decodeTAIFEXCSVBody(readFutFixture(t, name), "text/html;charset=MS950")
	if err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return out
}

// fastLimiterProvider 回傳一個不限流的 provider（測試用；否則每段請求會等 3 秒）。
func fastLimiterProvider() *TAIFEXFuturesBarsProvider {
	p := NewTAIFEXFuturesBarsProvider()
	p.rateLimiter = rate.NewLimiter(rate.Inf, 1)
	return p
}

func txFutFixture(t *testing.T) []byte {
	t.Helper()
	return decodeFutFixture(t, "taifex_fut_daily_20260921_24_TX.csv")
}

func findFutBar(t *testing.T, bars []domain.FuturesBar, date, month string, session domain.FuturesSession) domain.FuturesBar {
	t.Helper()
	for _, b := range bars {
		if b.TradeDate.Format("2006-01-02") == date && b.ContractMonth == month && b.Session == session {
			return b
		}
	}
	t.Fatalf("bar not found: date=%s month=%s session=%s (have %d bars)", date, month, session, len(bars))
	return domain.FuturesBar{}
}

// TestParseFuturesCSVBars_TXSample 釘住欄位對映與值：任一欄位索引被改壞，本測試必紅。
//
// 期望值全部取自 fixture 原文：
//
//	2026/09/21,TX,202610  ,47607,48091,47496,48077,649,1.37%,37750,48053,101502,...
//	2026/09/21,TX,202610  ,47494,47584,47208,47405,-23,-0.05%,21514,-,-,...
func TestParseFuturesCSVBars_TXSample(t *testing.T) {
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(txFutFixture(t))), map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bars) == 0 {
		t.Fatal("no bars parsed")
	}

	regular := findFutBar(t, bars, "2026-09-21", "202610", domain.SessionRegular)
	assertFutPtr(t, "open", regular.Open, 47607)
	assertFutPtr(t, "high", regular.High, 48091)
	assertFutPtr(t, "low", regular.Low, 47496)
	assertFutPtr(t, "close", regular.Close, 48077)
	assertFutIntPtr(t, "volume", regular.Volume, 37750)
	assertFutPtr(t, "settlement", regular.SettlementPrice, 48053)
	assertFutIntPtr(t, "open_interest", regular.OpenInterest, 101502)
	if regular.Contract != "TX" {
		t.Errorf("contract = %q, want TX", regular.Contract)
	}
	if regular.Source != futuresBarsSourceCSV {
		t.Errorf("source = %q, want %q", regular.Source, futuresBarsSourceCSV)
	}
	if !regular.Traded() {
		t.Error("regular session bar should be Traded()")
	}

	// 盤後列：成交量有值、OI 固定缺值（"-"）⇒ nil，不是 0。
	after := findFutBar(t, bars, "2026-09-21", "202610", domain.SessionAfterHours)
	assertFutIntPtr(t, "after_hours volume", after.Volume, 21514)
	if after.OpenInterest != nil {
		t.Errorf("after_hours open_interest = %d, want nil (upstream \"-\")", *after.OpenInterest)
	}
	if after.SettlementPrice != nil {
		t.Errorf("after_hours settlement = %v, want nil (upstream \"-\")", *after.SettlementPrice)
	}
	assertFutPtr(t, "after_hours close", after.Close, 47405)
}

// TestParseFuturesCSVBars_IsTXOnly 驗證 want 過濾（單一契約請求）。
func TestParseFuturesCSVBars_IsTXOnly(t *testing.T) {
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(txFutFixture(t))), map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, b := range bars {
		if b.Contract != "TX" {
			t.Fatalf("unexpected contract %q (want filter applied)", b.Contract)
		}
	}
}

// TestParseFuturesCSVBars_HeaderSentinelRejectsAlertPage 是 §1.3 的**負對照**測試。
//
// fixture 是官網在「查詢區間 >31 天」時的真實回應：HTTP 200，body 卻是
// `<script>alert("日期時間錯誤")...>`。哨兵必須把它擋成 ErrTAIFEXFuturesBarsSchema，
// **不得**回 (nil slice, nil error)。
func TestParseFuturesCSVBars_HeaderSentinelRejectsAlertPage(t *testing.T) {
	body := readFutFixture(t, "taifex_fut_daily_range_limit_error.html")
	if !bytesContains(body, "alert") {
		t.Fatal("fixture is not the TAIFEX alert page; test would be vacuous")
	}
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(body)), map[string]bool{"TX": true})
	if err == nil {
		t.Fatalf("expected schema error for HTML alert page, got nil (bars=%d)", len(bars))
	}
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("error must wrap ErrSchema, got %v", err)
	}
	if !errors.Is(err, ErrTAIFEXFuturesBarsSchema) {
		t.Fatalf("error must be ErrTAIFEXFuturesBarsSchema, got %v", err)
	}
	if strings.Contains(err.Error(), "no futures rows") {
		t.Fatalf("an HTML error page must never be classified as empty data (ErrNoData world): %v", err)
	}
	if !strings.Contains(err.Error(), "header sentinel") {
		t.Fatalf("the sentinel must be the FIRST gate for a non-CSV body (got %v)", err)
	}
}

// TestVerifyFuturesCSVHeader_Sentinel 直接釘住表頭哨兵本身。
//
// 為什麼要獨立釘：只斷言 `errors.Is(err, ErrSchema)` 是不夠的 —— 把哨兵拿掉後，
// HTML 錯誤頁仍會因為「缺少必要欄位」而回 ErrSchema，測試照樣綠（= 釘子空轉，已實測）。
// 因此這裡同時斷言**錯誤訊息指出哨兵**，讓「移除哨兵」這個突變必定變紅。
func TestVerifyFuturesCSVHeader_Sentinel(t *testing.T) {
	if err := verifyFuturesCSVHeader(futuresCSVHeaderLine + "\n2026/09/21,TX,...\n"); err != nil {
		t.Fatalf("valid CSV header must pass the sentinel: %v", err)
	}
	alert := string(readFutFixture(t, "taifex_fut_daily_range_limit_error.html"))
	err := verifyFuturesCSVHeader(alert)
	if err == nil {
		t.Fatal("alert page must fail the sentinel")
	}
	if !strings.Contains(err.Error(), "header sentinel") {
		t.Fatalf("error must identify the missing header sentinel as the cause (got %v) — otherwise the gate is not pinned", err)
	}
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("sentinel failure must wrap ErrSchema, got %v", err)
	}
}

// TestParseFuturesCSVBars_HeaderOnlyIsNoData 區分「只有表頭」與「HTML 錯誤頁」。
//
// fixture 是 1998/07/20（TX 上市前一日）的真實回應：表頭正常、0 資料列。
// 這是 ErrNoData 的世界，不是 ErrSchema —— 回空且不報錯，由呼叫端轉 ErrNoData。
func TestParseFuturesCSVBars_HeaderOnlyIsNoData(t *testing.T) {
	body := decodeFutFixture(t, "taifex_fut_daily_header_only_19980720.csv")
	if !strings.HasPrefix(strings.TrimSpace(string(body)), futuresCSVHeaderSentinel) {
		t.Fatalf("fixture must start with the CSV header sentinel, got %q", firstLine(string(body)))
	}
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(body)), nil)
	if err != nil {
		t.Fatalf("header-only CSV must not be a schema error: %v", err)
	}
	if len(bars) != 0 {
		t.Fatalf("want 0 bars, got %d", len(bars))
	}
}

// TestParseFuturesCSVBars_SkipsSpreadCombos 價差組合（到期月含 "/"）不得入庫。
func TestParseFuturesCSVBars_SkipsSpreadCombos(t *testing.T) {
	body := txFutFixture(t)
	if !strings.Contains(string(body), "2027") || !strings.Contains(string(body), "/") {
		t.Fatal("fixture lost its spread-combination rows; the pin would be vacuous")
	}
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(body)), map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, b := range bars {
		if strings.Contains(b.ContractMonth, "/") {
			t.Fatalf("spread combination leaked into bars: %q", b.ContractMonth)
		}
	}
}

// TestParseFuturesCSVBars_WeeklyContracts 週契約可入庫，但不得被當成月契約。
func TestParseFuturesCSVBars_WeeklyContracts(t *testing.T) {
	body := decodeFutFixture(t, "taifex_fut_daily_20260921_24_MTX.csv")
	bars, err := ParseFuturesCSVBars(strings.NewReader(string(body)), map[string]bool{"MTX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var weekly, monthly int
	for _, b := range bars {
		if domain.IsMonthlyContractMonth(b.ContractMonth) {
			monthly++
			continue
		}
		if strings.Contains(b.ContractMonth, "W") {
			weekly++
			if b.Contract != "MTX" {
				t.Fatalf("weekly bar has unexpected contract %q", b.Contract)
			}
		}
	}
	if weekly == 0 {
		t.Fatal("fixture must contain MTX weekly contracts (e.g. 202609W5); pin would be vacuous")
	}
	if monthly == 0 {
		t.Fatal("fixture must contain monthly contracts")
	}
}

// TestParseFuturesCSVBars_MissingValuesAreNilNotZero 是「缺值 ≠ 0」的釘子。
func TestParseFuturesCSVBars_MissingValuesAreNilNotZero(t *testing.T) {
	csvText := futuresCSVHeaderLine + "\n" +
		// 有成交：全部有值
		"2026/09/21,TX,202610  ,47607,48091,47496,48077,649,1.37%,37750,48053,101502,1,2,3,4,,一般,,\n" +
		// 上市但未成交：OHLC/結算/OI 全為 "-"，成交量為 0（合法值）
		"2026/09/21,TX,202706  ,-,-,-,-,-,-,0,-,-,-,-,-,-,,一般,,\n"
	bars, err := ParseFuturesCSVBars(strings.NewReader(csvText), map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("want 2 bars, got %d", len(bars))
	}
	untraded := bars[1]
	if untraded.Close != nil || untraded.Open != nil || untraded.SettlementPrice != nil || untraded.OpenInterest != nil {
		t.Fatalf("missing values must be nil, got %+v", untraded)
	}
	if untraded.Volume == nil {
		t.Fatal("volume \"0\" is a legitimate value, must NOT be nil")
	}
	if *untraded.Volume != 0 {
		t.Fatalf("volume = %d, want 0", *untraded.Volume)
	}
	if untraded.Traded() {
		t.Error("bar with no close price must not be Traded()")
	}
	// NULL（OpenAPI 的缺值表示）也要走同一條路徑。
	if v, err := parseNullableFloat("NULL"); err != nil || v != nil {
		t.Fatalf("NULL must map to nil, got v=%v err=%v", v, err)
	}
	if v, err := parseNullableInt("-"); err != nil || v != nil {
		t.Fatalf("\"-\" must map to nil, got v=%v err=%v", v, err)
	}
}

// TestParseFuturesCSVBars_HeaderDrivenMapping 欄位順序改變（表頭名稱不變）仍必須正確解析。
func TestParseFuturesCSVBars_HeaderDrivenMapping(t *testing.T) {
	// 刻意把「成交量」與「結算價」對調順序。
	header := "交易日期,契約,到期月份(週別),開盤價,最高價,最低價,收盤價,結算價,成交量,未沖銷契約數,交易時段\n"
	row := "2026/09/21,TX,202610  ,47607,48091,47496,48077,48053,37750,101502,一般\n"
	bars, err := ParseFuturesCSVBars(strings.NewReader(header+row), map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("want 1 bar, got %d", len(bars))
	}
	assertFutPtr(t, "close", bars[0].Close, 48077)
	assertFutIntPtr(t, "volume", bars[0].Volume, 37750)
	assertFutPtr(t, "settlement", bars[0].SettlementPrice, 48053)
	assertFutIntPtr(t, "open_interest", bars[0].OpenInterest, 101502)
}

// TestParseFuturesCSVBars_MissingRequiredColumn 少一個必要欄位 ⇒ ErrSchema（不可靜默）。
func TestParseFuturesCSVBars_MissingRequiredColumn(t *testing.T) {
	header := "交易日期,契約,到期月份(週別),開盤價,最高價,最低價,收盤價,成交量,結算價,交易時段\n" // 少了「未沖銷契約數」
	row := "2026/09/21,TX,202610  ,47607,48091,47496,48077,37750,48053,一般\n"
	_, err := ParseFuturesCSVBars(strings.NewReader(header+row), nil)
	if err == nil || !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema for missing column, got %v", err)
	}
}

// TestParseFuturesCSVBars_NonNumericValueIsSchemaError 非數字非缺值 ⇒ ErrSchema。
func TestParseFuturesCSVBars_NonNumericValueIsSchemaError(t *testing.T) {
	csvText := futuresCSVHeaderLine + "\n" +
		"2026/09/21,TX,202610  ,47607,48091,47496,oops,649,1.37%,37750,48053,101502,,,,一般,,\n"
	_, err := ParseFuturesCSVBars(strings.NewReader(csvText), nil)
	if err == nil || !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema for non-numeric close, got %v", err)
	}
}

// TestParseFuturesCSVBars_ShortRowIsSchemaErrorNotPanic 欄數不足的列必須回 ErrSchema。
//
// 這是一條**由測試抓到的真 bug**：初版只比對「欄數 >= 必要欄位個數」（11），
// 但表頭驅動對映下真正的邊界是最後一個必要欄位的索引（交易時段在第 18 欄），
// 因此 17 欄的列會讓索引越界 panic。修正後回 typed error。
func TestParseFuturesCSVBars_ShortRowIsSchemaErrorNotPanic(t *testing.T) {
	csvText := futuresCSVHeaderLine + "\n" +
		"2026/09/21,TX,202610  ,47607,48091,47496,48077,649,1.37%,37750,48053,101502,\n"
	_, err := ParseFuturesCSVBars(strings.NewReader(csvText), nil)
	if err == nil || !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema for short row, got %v", err)
	}
}

// TestParseFuturesCSVBars_UnknownSessionIsNotRegular 未知時段不得被歸類為一般。
func TestParseFuturesCSVBars_UnknownSessionIsNotRegular(t *testing.T) {
	csvText := futuresCSVHeaderLine + "\n" +
		"2026/09/21,TX,202610  ,47607,48091,47496,48077,649,1.37%,37750,48053,101502,,,,加掛,,\n"
	bars, err := ParseFuturesCSVBars(strings.NewReader(csvText), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if bars[0].Session != domain.SessionUnknown {
		t.Fatalf("session = %q, want %q", bars[0].Session, domain.SessionUnknown)
	}
}

// TestParseFuturesOpenAPIBars 用 OpenAPI 真實回應（TX/MTX 前 12 列）釘住 JSON 對映。
func TestParseFuturesOpenAPIBars(t *testing.T) {
	body := readFutFixture(t, "taifex_fut_daily_openapi_20260924.json")
	bars, err := ParseFuturesOpenAPIBars(body, map[string]bool{"TX": true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bars) == 0 {
		t.Fatal("no bars parsed")
	}
	regular := findFutBar(t, bars, "2026-09-24", "202610", domain.SessionRegular)
	assertFutPtr(t, "open", regular.Open, 47850)
	assertFutPtr(t, "close", regular.Close, 48123) // JSON key "Last"
	assertFutIntPtr(t, "volume", regular.Volume, 37196)
	assertFutIntPtr(t, "open_interest", regular.OpenInterest, 101311)
	if regular.Source != futuresBarsSourceOpenAPI {
		t.Errorf("source = %q, want %q", regular.Source, futuresBarsSourceOpenAPI)
	}
	after := findFutBar(t, bars, "2026-09-24", "202610", domain.SessionAfterHours)
	if after.OpenInterest != nil {
		t.Errorf("after_hours open_interest = %d, want nil", *after.OpenInterest)
	}
	// MTX 不得出現（want 過濾）。
	for _, b := range bars {
		if b.Contract != "TX" {
			t.Fatalf("unexpected contract %q", b.Contract)
		}
	}
}

// TestParseFuturesOpenAPIBars_RejectsHTML body 是 HTML ⇒ ErrSchema（不可當成空資料）。
func TestParseFuturesOpenAPIBars_RejectsHTML(t *testing.T) {
	_, err := ParseFuturesOpenAPIBars([]byte("<html><body>maintenance</body></html>"), nil)
	if err == nil || !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema, got %v", err)
	}
}

// TestChunkFuturesDateRange_MaxThirtyOneDays 分段上限釘子：每段相差 ≤ 31 天。
func TestChunkFuturesDateRange_MaxThirtyOneDays(t *testing.T) {
	loc := TaiwanLocation()
	start := time.Date(1998, 7, 21, 0, 0, 0, 0, loc)
	end := time.Date(2026, 9, 24, 0, 0, 0, 0, loc)
	chunks := ChunkFuturesDateRange(start, end)
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if got := chunks[0][0].Format("2006-01-02"); got != "1998-07-21" {
		t.Errorf("first chunk start = %s, want 1998-07-21", got)
	}
	last := chunks[len(chunks)-1]
	if got := last[1].Format("2006-01-02"); got != "2026-09-24" {
		t.Errorf("last chunk end = %s, want 2026-09-24", got)
	}
	for i, c := range chunks {
		if c[1].Before(c[0]) {
			t.Fatalf("chunk %d has end before start: %v", i, c)
		}
		diff := int(c[1].Sub(c[0]).Hours() / 24)
		if diff > futuresBarsMaxRangeDays {
			t.Fatalf("chunk %d spans %d days (> %d): %s..%s", i, diff, futuresBarsMaxRangeDays, c[0], c[1])
		}
		if i > 0 {
			prev := chunks[i-1]
			if !c[0].Equal(prev[1].AddDate(0, 0, 1)) {
				t.Fatalf("chunks %d/%d are not contiguous: %s..%s then %s..%s", i-1, i, prev[0], prev[1], c[0], c[1])
			}
		}
	}
	// 單日區間。
	single := ChunkFuturesDateRange(start, start)
	if len(single) != 1 || !single[0][0].Equal(single[0][1]) {
		t.Fatalf("single-day range should be one chunk, got %v", single)
	}
}

// TestFetchFuturesBarsRange_ChunksAndParses 以 httptest 驗證：每次請求的區間都 ≤31 天、
// 表頭哨兵生效、且分段結果被彙整。
func TestFetchFuturesBarsRange_ChunksAndParses(t *testing.T) {
	// 刻意回**原始 MS950 bytes** ＋ 官網實際的 Content-Type，
	// 讓本測試同時覆蓋「分段」「表頭哨兵」「charset 轉碼」三件事。
	txCSV := readFutFixture(t, "taifex_fut_daily_20260921_24_TX.csv")
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := r.Form.Get("down_type"); got != "1" {
			t.Errorf("down_type = %q, want 1", got)
		}
		if got := r.Form.Get("commodity_id"); got != "TX" {
			t.Errorf("commodity_id = %q, want TX", got)
		}
		start, err1 := time.ParseInLocation("2006/01/02", r.Form.Get("queryStartDate"), TaiwanLocation())
		end, err2 := time.ParseInLocation("2006/01/02", r.Form.Get("queryEndDate"), TaiwanLocation())
		if err1 != nil || err2 != nil {
			t.Errorf("bad dates: %v %v", err1, err2)
		}
		if diff := int(end.Sub(start).Hours() / 24); diff > futuresBarsMaxRangeDays {
			t.Errorf("request spans %d days (> %d)", diff, futuresBarsMaxRangeDays)
		}
		ranges = append(ranges, start.Format("2006-01-02")+".."+end.Format("2006-01-02"))
		w.Header().Set("Content-Type", "text/html;charset=MS950")
		_, _ = w.Write(txCSV)
	}))
	defer srv.Close()

	p := fastLimiterProvider()
	p.SetHTTPClient(srv.Client())
	p.csvURL = srv.URL

	loc := TaiwanLocation()
	bars, err := p.FetchFuturesBarsRange(context.Background(),
		[]string{"TX"},
		time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 9, 24, 0, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(bars) == 0 {
		t.Fatal("no bars")
	}
	if len(ranges) < 4 {
		t.Fatalf("2026-06-01..2026-09-24 must be split into >=4 chunks, got %v", ranges)
	}
}

// TestFetchFuturesBarsRange_SentinelPropagatesErrSchema 上游回 HTML 錯誤頁 ⇒ 整輪回 ErrSchema。
func TestFetchFuturesBarsRange_SentinelPropagatesErrSchema(t *testing.T) {
	alert := readFutFixture(t, "taifex_fut_daily_range_limit_error.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		_, _ = w.Write(alert)
	}))
	defer srv.Close()

	p := fastLimiterProvider()
	p.SetHTTPClient(srv.Client())
	p.csvURL = srv.URL

	loc := TaiwanLocation()
	day := time.Date(2026, 9, 24, 0, 0, 0, 0, loc)
	_, err := p.FetchFuturesBarsRange(context.Background(), []string{"TX"}, day, day)
	if err == nil {
		t.Fatal("want error for alert page, got nil")
	}
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema, got %v", err)
	}
}

// TestFetchFuturesBarsRange_HeaderOnlyIsNoData 「只有表頭」⇒ ErrNoData（不記 breaker 失敗）。
func TestFetchFuturesBarsRange_HeaderOnlyIsNoData(t *testing.T) {
	headerOnly := readFutFixture(t, "taifex_fut_daily_header_only_19980720.csv")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=MS950")
		_, _ = w.Write(headerOnly)
	}))
	defer srv.Close()

	p := fastLimiterProvider()
	p.SetHTTPClient(srv.Client())
	p.csvURL = srv.URL

	loc := TaiwanLocation()
	day := time.Date(1998, 7, 20, 0, 0, 0, 0, loc)
	_, err := p.FetchFuturesBarsRange(context.Background(), []string{"TX"}, day, day)
	if err == nil || !errors.Is(err, ErrNoData) {
		t.Fatalf("want ErrNoData, got %v", err)
	}
	if errors.Is(err, ErrSchema) {
		t.Fatal("header-only response must NOT be classified as a schema error")
	}
	if info := p.BreakerInfo(); info.FailureCount != 0 {
		t.Fatalf("no-data must not trip the breaker, failure count = %d", info.FailureCount)
	}
}

// TestFetchLatestFuturesBars_OpenAPI 走 OpenAPI JSON 路徑。
func TestFetchLatestFuturesBars_OpenAPI(t *testing.T) {
	body := readFutFixture(t, "taifex_fut_daily_openapi_20260924.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != taifexFuturesBarsOpenAPIPath {
			t.Errorf("path = %q, want %q", r.URL.Path, taifexFuturesBarsOpenAPIPath)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := fastLimiterProvider()
	p.SetHTTPClient(srv.Client())
	p.openAPIBaseURL = srv.URL

	bars, err := p.FetchLatestFuturesBars(context.Background(), []string{"TX"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(bars) == 0 {
		t.Fatal("no bars")
	}
	if got := p.Name(); got != "taifex_futures_bars" {
		t.Errorf("Name() = %q", got)
	}
}

// TestFetchLatestFuturesBars_NoMatchingRowsIsNoData want 契約不在回應中 ⇒ ErrNoData。
func TestFetchLatestFuturesBars_NoMatchingRowsIsNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Date":"20260924","Contract":"ZFF","ContractMonth(Week)":"202610","Open":"1","High":"1","Low":"1","Last":"1","Volume":"1","SettlementPrice":"1","OpenInterest":"1","TradingSession":"一般"}]`))
	}))
	defer srv.Close()

	p := fastLimiterProvider()
	p.SetHTTPClient(srv.Client())
	p.openAPIBaseURL = srv.URL

	_, err := p.FetchLatestFuturesBars(context.Background(), []string{"TX"})
	if err == nil || !errors.Is(err, ErrNoData) {
		t.Fatalf("want ErrNoData, got %v", err)
	}
}

// ─── domain 契約參考表 ──────────────────────────────────────────────────────

func TestFuturesContractSpecFor(t *testing.T) {
	tx, ok := domain.FuturesContractSpecFor("TX")
	if !ok || tx.Multiplier != 200 {
		t.Fatalf("TX spec = %+v ok=%v, want multiplier 200", tx, ok)
	}
	mtx, ok := domain.FuturesContractSpecFor("MTX")
	if !ok || mtx.Multiplier != 50 || !mtx.HasWeeklies {
		t.Fatalf("MTX spec = %+v ok=%v, want multiplier 50 + weeklies", mtx, ok)
	}
	if _, ok := domain.FuturesContractSpecFor("NOPE"); ok {
		t.Fatal("unknown contract must not resolve to a default spec")
	}
}

func TestFuturesLastTradingDay(t *testing.T) {
	// 2026-09 的星期三為 2/9/16/23/30 ⇒ 第三個 = 16。
	got := domain.FuturesLastTradingDay(2026, time.September)
	if got.Format("2006-01-02") != "2026-09-16" {
		t.Fatalf("2026-09 third Wednesday = %s, want 2026-09-16", got.Format("2006-01-02"))
	}
	if got.Weekday() != time.Wednesday {
		t.Fatalf("result is %s, want Wednesday", got.Weekday())
	}
	// 2026-07 的星期三為 1/8/15/22/29 ⇒ 第三個 = 15。
	if got := domain.FuturesLastTradingDay(2026, time.July); got.Format("2006-01-02") != "2026-07-15" {
		t.Fatalf("2026-07 third Wednesday = %s, want 2026-07-15", got.Format("2006-01-02"))
	}
}

func TestIsMonthlyContractMonth(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"202610", true}, {"202609W5", false}, {"202706/202709", false}, {"", false}, {"20261", false},
	} {
		if got := domain.IsMonthlyContractMonth(tc.in); got != tc.want {
			t.Errorf("IsMonthlyContractMonth(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFuturesSessionFromTAIFEX(t *testing.T) {
	if got := futuresSessionFromTAIFEX("一般"); got != domain.SessionRegular {
		t.Errorf("一般 -> %q", got)
	}
	if got := futuresSessionFromTAIFEX("盤後"); got != domain.SessionAfterHours {
		t.Errorf("盤後 -> %q", got)
	}
	if got := futuresSessionFromTAIFEX(""); got != domain.SessionUnknown {
		t.Errorf("empty -> %q, want unknown", got)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

// futuresCSVHeaderLine 是官網 CSV 的實際表頭（19 欄）。
const futuresCSVHeaderLine = "交易日期,契約,到期月份(週別),開盤價,最高價,最低價,收盤價,漲跌價,漲跌%,成交量,結算價,未沖銷契約數,最後最佳買價,最後最佳賣價,歷史最高價,歷史最低價,是否因訊息面暫停交易,交易時段,價差對單式委託成交量"

func assertFutPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", name, *got, want)
	}
}

func assertFutIntPtr(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %d", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %d, want %d", name, *got, want)
	}
}

func bytesContains(b []byte, sub string) bool { return strings.Contains(string(b), sub) }

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
