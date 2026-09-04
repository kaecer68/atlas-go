package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"
)

// dayTradingTestProvider builds a provider pointed at a test server serving
// the given body for every TWTB4U request.
func dayTradingTestProvider(t *testing.T, body string) *DayTradingProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	p := NewDayTradingProvider()
	p.SetHTTPClient(srv.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.SetBaseURL(srv.URL)
	return p
}

// TestDayTradingProvider_FetchDate_StatsTable verifies the live 2026-09-01..03
// shape: tables[0] = 當日沖銷交易統計資訊 (aggregate row), tables[1] = eligible
// securities. The stats row must be picked regardless of its position.
func TestDayTradingProvider_FetchDate_StatsTable(t *testing.T) {
	// Statistics table deliberately placed LAST to prove identity-based
	// selection (not position-based).
	p := dayTradingTestProvider(t, `{"stat":"OK","date":"20260903","tables":[
		{"title":"115年09月03日 當日沖銷交易標的及成交量值","fields":["證券代號","證券名稱","暫停現股賣出後現款買進當沖註記","當日沖銷交易成交股數","當日沖銷交易買進成交金額","當日沖銷交易賣出成交金額"],"data":[["2330","台積電","","1,000,000","50,000,000","50,100,000"]]},
		{"title":"115年09月03日 當日沖銷交易統計資訊","fields":["當日沖銷交易總成交股數","當日沖銷交易總成交股數占市場比重%","當日沖銷交易總買進成交金額","當日沖銷交易總買進成交金額占市場比重%","當日沖銷交易總賣出成交金額","當日沖銷交易總賣出成交金額占市場比重%"],"data":[["2,787,279,000","24.88","415,154,007,080","41.79","415,341,495,270","41.81"]]}
	]}`)

	stats, err := p.fetchDate(context.Background(), "20260903")
	if err != nil {
		t.Fatalf("fetchDate error: %v", err)
	}
	if stats.DayTradingVolume != 2_787_279_000 {
		t.Errorf("DayTradingVolume = %d, want 2787279000 (aggregate row, not 2330's 1,000,000)", stats.DayTradingVolume)
	}
	if stats.VolumeRatio != 0.2488 {
		t.Errorf("VolumeRatio = %v, want 0.2488 (decimal semantics)", stats.VolumeRatio)
	}
	if stats.DayTradingBuyValue != 415_154_007_080 || stats.DayTradingSellValue != 415_341_495_270 {
		t.Errorf("buy/sell values = %d/%d, want 415154007080/415341495270", stats.DayTradingBuyValue, stats.DayTradingSellValue)
	}
	if stats.BuyValueRatio != 0.4179 || stats.SellValueRatio != 0.4181 {
		t.Errorf("buy/sell ratios = %v/%v, want 0.4179/0.4181", stats.BuyValueRatio, stats.SellValueRatio)
	}
}

// TestDayTradingProvider_FetchDate_StatsNotPublishedYet covers the verified
// 2026-09-04 live shape: tables[0] is an EMPTY object and tables[1] is the
// securities list with only 3 columns. This is "statistics not published
// yet", so fetchDate must return ErrNoData (gateway waiting-state, calendar
// walk-back) and never mistake the securities list for statistics.
func TestDayTradingProvider_FetchDate_StatsNotPublishedYet(t *testing.T) {
	p := dayTradingTestProvider(t, `{"stat":"OK","date":"20260904","tables":[
		{},
		{"title":"115年09月04日 當日沖銷交易標的","fields":["證券代號","證券名稱","暫停現股賣出後\n現款買進當沖註記"],"data":[["00400A","主動國泰動能高息","Y"],["2330","台積電",""]]}
	]}`)

	_, err := p.fetchDate(context.Background(), "20260904")
	if err == nil {
		t.Fatal("fetchDate error = nil, want ErrNoData when statistics are absent")
	}
	if !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want ErrNoData wrap", err)
	}
	if !strings.Contains(err.Error(), "not published yet") {
		t.Errorf("error should explain the not-published-yet case, got %v", err)
	}
}

// TestDayTradingProvider_FetchDate_LegacyTopLevelData verifies the pre-tables
// dual-format fallback: top-level `data` rows with the same 6 columns.
func TestDayTradingProvider_FetchDate_LegacyTopLevelData(t *testing.T) {
	p := dayTradingTestProvider(t, `{"stat":"OK","date":"20260825","fields":["當日沖銷交易總成交股數","占市場比重%","買進成交金額","占買進比重%","賣出成交金額","占賣出比重%"],"data":[["2,473,492,000","28.0","339,363,054,910","46.85","340,684,245,090","47.03"]]}`)

	stats, err := p.fetchDate(context.Background(), "20260825")
	if err != nil {
		t.Fatalf("fetchDate error: %v", err)
	}
	if stats.DayTradingVolume != 2_473_492_000 {
		t.Errorf("DayTradingVolume = %d, want 2473492000", stats.DayTradingVolume)
	}
	if stats.VolumeRatio != 0.28 {
		t.Errorf("VolumeRatio = %v, want 0.28", stats.VolumeRatio)
	}
	if stats.DayTradingBuyValue != 339_363_054_910 {
		t.Errorf("DayTradingBuyValue = %d, want 339363054910", stats.DayTradingBuyValue)
	}
}

// TestDayTradingProvider_FetchDate_EmptyTablesNoData covers stat=OK with no
// tables and no legacy data at all (schema break).
func TestDayTradingProvider_FetchDate_EmptyTablesNoData(t *testing.T) {
	p := dayTradingTestProvider(t, `{"stat":"OK","date":"20260904","tables":[]}`)

	_, err := p.fetchDate(context.Background(), "20260904")
	if !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want ErrNoData wrap", err)
	}
}

// TestDayTradingProvider_FetchLatest_WalksBackPastUnpublishedDays verifies
// FetchLatest still recovers when the newest queried days have no published
// statistics (the verified 2026-09-04/05 live condition): the calendar
// walk-back must land on an older day that has full statistics.
func TestDayTradingProvider_FetchLatest_WalksBackPastUnpublishedDays(t *testing.T) {
	fullStats := `{"stat":"OK","date":"20260903","tables":[
		{"title":"115年09月03日 當日沖銷交易統計資訊","fields":["當日沖銷交易總成交股數","當日沖銷交易總成交股數占市場比重%","當日沖銷交易總買進成交金額","當日沖銷交易總買進成交金額占市場比重%","當日沖銷交易總賣出成交金額","當日沖銷交易總賣出成交金額占市場比重%"],"data":[["2,787,279,000","24.88","415,154,007,080","41.79","415,341,495,270","41.81"]]},
		{"title":"115年09月03日 當日沖銷交易標的及成交量值","fields":["證券代號","證券名稱"],"data":[["2330","台積電"]]}
	]}`
	unpublished := `{"stat":"OK","date":"20260904","tables":[{},{"title":"標的","fields":["證券代號","證券名稱","註記"],"data":[["2330","台積電",""]]}]}`

	var queries atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The first two queried days answer with the "statistics not
		// published yet" shape; anything older carries full statistics.
		if queries.Add(1) <= 2 {
			_, _ = w.Write([]byte(unpublished))
			return
		}
		_, _ = w.Write([]byte(fullStats))
	}))
	defer srv.Close()

	p := NewDayTradingProvider()
	p.SetHTTPClient(srv.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.SetBaseURL(srv.URL)

	stats, err := p.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest error: %v", err)
	}
	if stats.VolumeRatio != 0.2488 {
		t.Errorf("VolumeRatio = %v, want 0.2488 (walked back to the published day)", stats.VolumeRatio)
	}
	if n := queries.Load(); n < 3 {
		t.Errorf("queries = %d, want >= 3 (first two days unpublished, third has stats)", n)
	}
}
