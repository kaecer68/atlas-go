package marketdata

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 2026-09-26: replay phantom rows → date-addressed MI_INDEX source.
//
// STOCK_DAY_ALL (GetQuotes) takes NO date parameter: on a closed market it
// replays the previous trading day's rows (verified live on Saturday
// 2026-09-26: the first data column of the response was the ROC date 1150924 =
// 2026-09-24, the previous Thursday). The
// replay writer stamped those rows with time.Now(), so the CSV gained a
// phantom row set for every closed day since 2026-08-29 — and the CSV's date
// column IS the replay trading calendar.
//
// These tests pin the replacement source's contract:
//  1. one dated request (type=ALLBUT0999&date=YYYYMMDD) yields the whole market;
//  2. a closed market is reported as empty data, not as a fetch failure;
//  3. the returned DataDate is the payload's OWN date — never the requested
//     one — so a caller can refuse to store a mis-dated payload;
//  4. stat=OK with no 每日收盤行情 table is a SCHEMA change, never a silent
//     zero-row success (the cmd/fetch-historical defect).
// ---------------------------------------------------------------------------

// miIndexFields is the real 每日收盤行情 header (measured 2026-09-24).
const miIndexFields = `"證券代號","證券名稱","成交股數","成交筆數","成交金額","開盤價","最高價","最低價","收盤價","漲跌(+/-)","漲跌價差","最後揭示買價","最後揭示買量","最後揭示賣價","最後揭示賣量","本益比"`

// miIndexRow2330 is a real row (comma-grouped numbers included) so the tests
// exercise the same comma-stripping path as production.
const miIndexRow2330 = `["2330","台積電","14,557,662","75,382","36,107,476,243","2,480.00","2,490.00","2,470.00","2,475.00","<p style= color:green>-</p>","25.00","2,475.00","1,065","2,480.00","46","28.69"]`

// miIndexBody renders a minimal MI_INDEX payload: an index section first (as
// the live response has, so a positional parser would pick the wrong table)
// followed by the 每日收盤行情 section.
func miIndexBody(stat, date, title string, rows ...string) string {
	if stat != "OK" {
		return fmt.Sprintf(`{"stat":%q,"type":"ALLBUT0999"}`, stat)
	}
	return fmt.Sprintf(`{
		"stat":"OK",
		"date":%q,
		"tables":[
			{"title":"115年09月24日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數","漲跌(+/-)"],"data":[["發行量加權股價指數","48,024.60","-"]]},
			{"title":%q,"fields":[%s],"data":[%s]}
		]
	}`, date, title, miIndexFields, strings.Join(rows, ","))
}

const miIndexDailyTitle = "115年09月24日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))"

func TestTWSEMITable_TitleDate(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
		ok    bool
	}{
		{"trading day", "115年09月24日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))", "2026-09-24", true},
		{"older ROC year", "113年01月02日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))", "2024-01-02", true},
		{"3-digit ROC year 109", "109年08月03日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))", "2020-08-03", true},
		{"no date in title", "價格指數(跨市場)", "", false},
		{"impossible date is rejected, not normalised", "115年13月40日 每日收盤行情", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := twseMITable{Title: tc.title}.TitleDate()
			if ok != tc.ok || got != tc.want {
				t.Errorf("TitleDate(%q) = (%q, %v), want (%q, %v)", tc.title, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestTWSEMIIndexResponse_DailyQuotesTable(t *testing.T) {
	resp := twseMIIndexResponse{Tables: []twseMITable{
		{Title: "115年09月24日 價格指數(臺灣證券交易所)", Fields: []string{"指數", "收盤指數"}},
		{Title: "115年09月24日 大盤統計資訊", Fields: []string{"成交統計", "成交金額(元)"}},
		{Title: miIndexDailyTitle, Fields: []string{"證券代號", "收盤價"}},
	}}
	got, ok := resp.DailyQuotesTable()
	if !ok || got.Title != miIndexDailyTitle {
		t.Fatalf("DailyQuotesTable() = (%q, %v), want the 每日收盤行情 table", got.Title, ok)
	}

	// Index-only payload (a table index reorder, or a truncated report) must
	// NOT be mistaken for the per-symbol table.
	indexOnly := twseMIIndexResponse{Tables: []twseMITable{{Fields: []string{"指數", "收盤指數"}}}}
	if _, ok := indexOnly.DailyQuotesTable(); ok {
		t.Error("index-only payload resolved to a daily-quotes table")
	}
}

func TestTWSEMITable_QuoteRowsMapsByHeaderNotByPosition(t *testing.T) {
	table := twseMITable{
		Fields: []string{"證券代號", "證券名稱", "成交股數", "成交筆數", "成交金額", "開盤價", "最高價", "最低價", "收盤價", "漲跌(+/-)", "漲跌價差", "最後揭示買價", "最後揭示買量", "最後揭示賣價", "最後揭示賣量", "本益比"},
		Data:   [][]string{{"2330", "台積電", "14,557,662", "75,382", "36,107,476,243", "2,480.00", "2,490.00", "2,470.00", "2,475.00", "-", "25.00", "2,475.00", "1,065", "2,480.00", "46", "28.69"}},
	}
	want := TWSEQuote{Code: "2330", Name: "台積電", TradeVolume: "14,557,662", TradeValue: "36,107,476,243", OpeningPrice: "2,480.00", HighestPrice: "2,490.00", LowestPrice: "2,470.00", ClosingPrice: "2,475.00", Change: "25.00", Transaction: "75,382"}

	rows := table.QuoteRows()
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("QuoteRows() = %+v, want %+v", rows, want)
	}

	// Upstream inserts a new leading column and reorders two others: the
	// mapping must follow the header, not the index.
	reordered := twseMITable{
		Fields: []string{"本益比", "收盤價", "證券代號", "證券名稱", "成交股數", "成交筆數", "成交金額", "開盤價", "最高價", "最低價", "漲跌(+/-)", "漲跌價差", "最後揭示買價", "最後揭示買量", "最後揭示賣價", "最後揭示賣量"},
		Data:   [][]string{{"28.69", "2,475.00", "2330", "台積電", "14,557,662", "75,382", "36,107,476,243", "2,480.00", "2,490.00", "2,470.00", "-", "25.00", "2,475.00", "1,065", "2,480.00", "46"}},
	}
	if got := reordered.QuoteRows(); len(got) != 1 || got[0] != want {
		t.Errorf("reordered header QuoteRows() = %+v, want %+v", got, want)
	}

	// Rows without a 證券代號 carry no symbol and are dropped.
	withBlank := twseMITable{
		Fields: []string{"證券代號", "證券名稱", "收盤價"},
		Data:   [][]string{{"", "", "1"}, {"2330", "台積電", "2,475.00"}},
	}
	if got := withBlank.QuoteRows(); len(got) != 1 || got[0].Code != "2330" {
		t.Errorf("QuoteRows() with a blank code row = %+v, want only 2330", got)
	}
}

func TestGetQuotesForDate_OneDatedRequestReturnsTheWholeMarket(t *testing.T) {
	var calls int64
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		gotURL = r.URL.String()
		if ua := r.Header.Get("User-Agent"); ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			t.Errorf("User-Agent = %q, want the full browser UA (TWSE WAF 403s short UAs)", ua)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(miIndexBody("OK", "20260924", miIndexDailyTitle, miIndexRow2330)))
	}))
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 2*time.Second, 1)
	res, err := c.GetQuotesForDate(t.Context(), "20260924")
	if err != nil {
		t.Fatalf("GetQuotesForDate: %v", err)
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("HTTP requests = %d, want exactly 1 (one request covers the whole market)", got)
	}
	for _, want := range []string{"type=ALLBUT0999", "date=20260924", "response=json"} {
		if !strings.Contains(gotURL, want) {
			t.Errorf("request URL %q is missing %q", gotURL, want)
		}
	}
	if res.DataDate != "2026-09-24" {
		t.Errorf("DataDate = %q, want 2026-09-24", res.DataDate)
	}
	if len(res.Quotes) != 1 {
		t.Fatalf("quotes = %d, want 1", len(res.Quotes))
	}
	q := res.Quotes[0]
	if q.Symbol != "2330" || q.Last != 2475 || q.Open != 2480 || q.High != 2490 || q.Low != 2470 {
		t.Errorf("quote = %+v, want 2330 with O=2480 H=2490 L=2470 C=2475 (comma-grouped fields)", q)
	}
	if q.Volume != 14557662 {
		t.Errorf("volume = %d, want 14557662 (成交股數, comma-stripped)", q.Volume)
	}
	// Date-addressed fetch: the only meaningful timestamp is the trading date,
	// pinned to the 13:30 TWSE close.
	wantAsOf := time.Date(2026, 9, 24, 13, 30, 0, 0, TaiwanLocation())
	if !q.AsOf.Equal(wantAsOf) {
		t.Errorf("AsOf = %s, want %s", q.AsOf, wantAsOf)
	}
}

func TestGetQuotesForDate_ClosedMarketIsEmptyDataNotFailure(t *testing.T) {
	// Live behaviour on 2026-09-26 (Sat) and 2026-09-25 (中秋節): HTTP 200 with
	// stat = 很抱歉，沒有符合條件的資料! and no tables.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"很抱歉，沒有符合條件的資料!","type":"ALLBUT0999"}`))
	}))
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 2*time.Second, 2)
	_, err := c.GetQuotesForDate(t.Context(), "20260926")
	if err == nil {
		t.Fatal("GetQuotesForDate on a closed market = nil error, want empty-data error")
	}
	if !errors.Is(err, ErrTWSEEmptyData) {
		t.Errorf("error = %v, want ErrTWSEEmptyData (a closed market is expected, not a failure)", err)
	}
	if errors.Is(err, ErrUpstream) {
		t.Error("a closed market must not look like an upstream/circuit-breaker failure")
	}
	if !strings.Contains(err.Error(), "20260926") {
		t.Errorf("error %q must name the requested date", err.Error())
	}
}

func TestGetQuotesForDate_ReturnsThePayloadsOwnDateNotTheRequestedOne(t *testing.T) {
	// THE negative proof for the mismatch guard: an upstream that ignores the
	// date parameter (exactly what STOCK_DAY_ALL does) must be detectable from
	// the return value, because DataDate comes from the payload's own title —
	// never from our request.
	const payloadTitle = "115年09月24日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(miIndexBody("OK", "20260925", payloadTitle, miIndexRow2330)))
	}))
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 2*time.Second, 1)
	res, err := c.GetQuotesForDate(t.Context(), "20260925")
	if err != nil {
		t.Fatalf("GetQuotesForDate: %v", err)
	}
	if res.DataDate != "2026-09-24" {
		t.Errorf("DataDate = %q, want 2026-09-24 (the payload's own date, so the caller can refuse to store it)", res.DataDate)
	}
	if res.DataDate == "2026-09-25" {
		t.Error("DataDate must never be the requested date when the payload describes another day")
	}
}

func TestGetQuotesForDate_StatOKWithoutDailyTableIsSchemaChange(t *testing.T) {
	// The cmd/fetch-historical defect: a changed envelope decoded as 0 rows and
	// exited 0. It must be loud.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260924","tables":[{"title":"115年09月24日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數"],"data":[["發行量加權股價指數","48,024.60"]]}]}`))
	}))
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 2*time.Second, 1)
	_, err := c.GetQuotesForDate(t.Context(), "20260924")
	if err == nil {
		t.Fatal("stat=OK without a 每日收盤行情 table = nil error, want a schema-change failure")
	}
	if !strings.Contains(err.Error(), "schema change") {
		t.Errorf("error = %v, want it to name the schema change", err)
	}
	if errors.Is(err, ErrTWSEEmptyData) {
		t.Error("a schema change must not be reported as empty data — that is how a silent zero-row success happens")
	}
}

func TestGetQuotesForDate_HTTPErrorNamesTheStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	defer srv.Close()

	c := newTWSERetryTestClient(srv, 2*time.Second, 1)
	_, err := c.GetQuotesForDate(t.Context(), "20260924")
	if err == nil {
		t.Fatal("503 = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "status 503") || !strings.Contains(err.Error(), "maintenance") {
		t.Errorf("error = %v, want the status and a bounded body for channel LastError", err)
	}
}

func TestGetQuotesForDate_RejectsUnparseableDate(t *testing.T) {
	c := &TWSEClient{}
	if _, err := c.GetQuotesForDate(t.Context(), "2026-09-24"); err == nil {
		t.Error("a non-YYYYMMDD date = nil error, want a rejection before any HTTP call")
	}
}

func TestParseMIIndexDailyQuotes_SharedEnvelopeForNonClientCallers(t *testing.T) {
	// cmd/fetch-historical parses MI_INDEX with its own http client; this
	// helper is the single understanding of the `tables` envelope it shares
	// with TWSEClient.GetQuotesForDate.
	date, rows, err := ParseMIIndexDailyQuotes([]byte(miIndexBody("OK", "20260924", miIndexDailyTitle, miIndexRow2330)))
	if err != nil {
		t.Fatalf("ParseMIIndexDailyQuotes: %v", err)
	}
	if date != "2026-09-24" || len(rows) != 1 || rows[0].Code != "2330" {
		t.Errorf("got (%q, %+v), want 2026-09-24 with the 2330 row", date, rows)
	}

	// The legacy flat shape (what the pre-fix parser expected) must fail
	// loudly instead of returning 0 rows with a nil error.
	if _, _, err := ParseMIIndexDailyQuotes([]byte(`{"stat":"OK","date":"20260924","fields":["Code"],"data":[["2330"]]}`)); err == nil {
		t.Error("legacy fields/data shape parsed without error — that is the silent 0-row success being fixed")
	}

	if _, _, err := ParseMIIndexDailyQuotes([]byte(`{"stat":"很抱歉，沒有符合條件的資料!","type":"ALLBUT0999"}`)); !errors.Is(err, ErrTWSEEmptyData) {
		t.Errorf("closed market error = %v, want ErrTWSEEmptyData", err)
	}
}
