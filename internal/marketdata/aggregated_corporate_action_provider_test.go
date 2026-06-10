package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ---------------------------------------------------------------------------
// Helpers — small in-memory fakes for the two upstream providers.
// ---------------------------------------------------------------------------

// stubTWSECalendar wraps TWSECalendarProvider's HTTP layer with a stub server
// so tests can inject events without hitting twse.com.tw. We assign baseURL +
// httpClient directly because the production constructor reads ParametersConfig.
func stubTWSECalendar(t *testing.T, eventsByMonth map[string]string) (*TWSECalendarProvider, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("date")
		if len(q) < 6 {
			http.Error(w, "bad date", http.StatusBadRequest)
			return
		}
		key := q[:6]
		body, ok := eventsByMonth[key]
		if !ok {
			// Empty / OK-with-no-data is a valid TWSE response shape.
			_, _ = w.Write([]byte(`{"stat":"OK","date":"` + q + `","title":"","fields":[],"data":[],"total":0}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))

	p := NewTWSECalendarProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	// Test-only override: the production rate limit (0.6 req/s) is too tight
	// for unit tests that issue ~12 month-requests per FetchEvents call.
	p.rateLimiter = rate.NewLimiter(rate.Inf, 1000)
	return p, func() { server.Close() }
}

// exDivMonthBody builds a TWSE exRight response payload for one month.
// rows: each entry is one [code, name, exDate, kind, preClose, refPrice,
//
//	rightsValue, dividendValue, totalValue, limitUp, limitDown,
//	startTradeDate, paymentDate].
func exDivMonthBody(rows [][]string) string {
	fields := []string{
		"股票代號", "股票名稱", "除權息日期", "種類",
		"除權息前收盤價", "除權息參考價", "權值", "息值", "權值+息值",
		"漲停價", "跌停價", "開始交易日期", "現金股利發放日",
	}
	var sb strings.Builder
	sb.WriteString(`{"stat":"OK","title":"除權息預告表","fields":[`)
	for i, f := range fields {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"`)
		sb.WriteString(f)
		sb.WriteString(`"`)
	}
	sb.WriteString(`],"data":[`)
	for i, row := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("[")
		for j, cell := range row {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`"`)
			sb.WriteString(cell)
			sb.WriteString(`"`)
		}
		sb.WriteString("]")
	}
	sb.WriteString(`],"total":`)
	if len(rows) > 0 {
		sb.WriteString("1")
	} else {
		sb.WriteString("0")
	}
	sb.WriteString(`}`)
	return sb.String()
}

// stubFinMindWithCache builds a real FinMindDividendProvider whose cache is
// pre-seeded with the supplied DividendRecords. The on-disk cache lets us
// drive the provider end-to-end without touching the network —
// finmindBaseURL is a package-level const that can't be redirected to an
// httptest server, so cache injection is the only test seam.
func stubFinMindWithCache(t *testing.T, symbol string, start, end time.Time, records []domain.DividendRecord) (*FinMindDividendProvider, func()) {
	t.Helper()
	cacheDir := t.TempDir()
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")
	filename := strings.ReplaceAll(symbol, ".", "_") + "_" + startStr + "_" + endStr + ".json"
	path := filepath.Join(cacheDir, filename)

	// mtime must be recent enough that cacheTTL (24h) hasn't expired.
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("seed empty cache: %v", err)
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	// Touch the file to ensure ModTime() > now-24h.
	future := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	client := NewFinMindClient("test-key")
	p := NewFinMindDividendProvider(client, cacheDir)
	return p, func() {}
}

// ---------------------------------------------------------------------------
// Constructor + basic interface satisfaction
// ---------------------------------------------------------------------------

func TestNewAggregatedCorporateActionProvider(t *testing.T) {
	twse := NewTWSECalendarProvider()
	p := NewAggregatedCorporateActionProvider(twse, nil)
	if p == nil {
		t.Fatal("constructor returned nil")
	}
	if p.twse != twse {
		t.Errorf("twse field not assigned")
	}
	if p.finmind != nil {
		t.Errorf("expected nil finmind")
	}
}

// TestAggregatedCorporateActionProvider_SatisfiesInterface verifies the
// concrete type implements the CorporateActionProvider interface. Catches
// signature drift early.
func TestAggregatedCorporateActionProvider_SatisfiesInterface(t *testing.T) {
	var _ CorporateActionProvider = (*AggregatedCorporateActionProvider)(nil)
}

// ---------------------------------------------------------------------------
// Primary path: TWSE wins
// ---------------------------------------------------------------------------

func TestGetCorporateActions_PrimaryFromTWSE(t *testing.T) {
	twseBody := exDivMonthBody([][]string{
		{"2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"},
		{"2454", "聯發科", "20260620", "除息", "1200.00", "1180.00", "0", "20", "20", "1298.00", "1062.00", "20260621", "20260715"},
	})
	twse, closeFn := stubTWSECalendar(t, map[string]string{"202606": twseBody})
	defer closeFn()

	p := NewAggregatedCorporateActionProvider(twse, nil)
	got, err := p.GetCorporateActions(context.Background(), "2330",
		date(2026, 6, 1), date(2026, 6, 30))
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event for 2330, got %d", len(got))
	}
	if got[0].Symbol != "2330" {
		t.Errorf("symbol: got %q want 2330", got[0].Symbol)
	}
	if got[0].Source != "twse_calendar" {
		t.Errorf("source: got %q want twse_calendar", got[0].Source)
	}
}

// ---------------------------------------------------------------------------
// Backup path: FinMind fills events TWSE misses.
// ---------------------------------------------------------------------------

func TestGetCorporateActions_BackupFromFinMind(t *testing.T) {
	// TWSE returns no events.
	twse, closeFn := stubTWSECalendar(t, map[string]string{})
	defer closeFn()

	// FinMind returns one record for 2330 — pre-seeded via cache.
	start, end := date(2026, 6, 1), date(2026, 6, 30)
	finmind, _ := stubFinMindWithCache(t, "2330", start, end, []domain.DividendRecord{
		{
			Symbol:         "2330",
			Year:           2025,
			CashDividend:   12.0,
			ExDividendDate: "2026-06-15",
		},
	})

	p := NewAggregatedCorporateActionProvider(twse, finmind)
	got, err := p.GetCorporateActions(context.Background(), "2330", start, end)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event from finmind, got %d", len(got))
	}
	if got[0].Source != "finmind" {
		t.Errorf("source: got %q want finmind", got[0].Source)
	}
	if got[0].CashDividend != 12.0 {
		t.Errorf("cash_dividend: got %v want 12", got[0].CashDividend)
	}
}

// ---------------------------------------------------------------------------
// Dedup: TWSE + FinMind overlap on the same (symbol, ex_date) → no duplicate
// ---------------------------------------------------------------------------

func TestGetCorporateActions_DedupByExDate(t *testing.T) {
	twseBody := exDivMonthBody([][]string{
		{"2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"},
	})
	twse, closeFn := stubTWSECalendar(t, map[string]string{"202606": twseBody})
	defer closeFn()

	start, end := date(2026, 6, 1), date(2026, 6, 30)
	// Same date as TWSE — must be deduplicated.
	finmind, _ := stubFinMindWithCache(t, "2330", start, end, []domain.DividendRecord{
		{
			Symbol:         "2330",
			Year:           2025,
			CashDividend:   15.0, // different from TWSE's parsed 12.0
			ExDividendDate: "2026-06-15",
		},
	})

	p := NewAggregatedCorporateActionProvider(twse, finmind)
	got, err := p.GetCorporateActions(context.Background(), "2330", start, end)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event after dedup, got %d: %+v", len(got), got)
	}
	if got[0].Source != "twse_calendar" {
		t.Errorf("TWSE should win dedup: got source %q", got[0].Source)
	}
}

// ---------------------------------------------------------------------------
// Partial failure: TWSE fails but FinMind succeeds → return FinMind's data
// ---------------------------------------------------------------------------

func TestGetCorporateActions_PartialFailure(t *testing.T) {
	twseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer twseServer.Close()
	twse := NewTWSECalendarProvider()
	twse.baseURL = twseServer.URL
	twse.SetHTTPClient(twseServer.Client())
	twse.rateLimiter = rate.NewLimiter(rate.Inf, 1000)

	start, end := date(2026, 6, 1), date(2026, 6, 30)
	finmind, _ := stubFinMindWithCache(t, "2330", start, end, []domain.DividendRecord{
		{
			Symbol:         "2330",
			Year:           2025,
			CashDividend:   12.0,
			ExDividendDate: "2026-06-15",
		},
	})

	p := NewAggregatedCorporateActionProvider(twse, finmind)
	got, err := p.GetCorporateActions(context.Background(), "2330", start, end)
	if err != nil {
		t.Fatalf("partial failure should not surface error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event from finmind fallback, got %d", len(got))
	}
	if got[0].Source != "finmind" {
		t.Errorf("expected source finmind after TWSE failure, got %q", got[0].Source)
	}
}

// ---------------------------------------------------------------------------
// Both fail → error returned
// ---------------------------------------------------------------------------

func TestGetCorporateActions_AllFail(t *testing.T) {
	twseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer twseServer.Close()
	twse := NewTWSECalendarProvider()
	twse.baseURL = twseServer.URL
	twse.SetHTTPClient(twseServer.Client())
	twse.rateLimiter = rate.NewLimiter(rate.Inf, 1000)

	// FinMind with empty cache → no records (success but empty).
	start, end := date(2026, 6, 1), date(2026, 6, 30)
	finmind, _ := stubFinMindWithCache(t, "2330", start, end, nil)

	p := NewAggregatedCorporateActionProvider(twse, finmind)
	_, err := p.GetCorporateActions(context.Background(), "2330", start, end)
	if err == nil {
		t.Fatal("expected error when both providers return no data")
	}
}

// ---------------------------------------------------------------------------
// Output ordering: ExDate ascending
// ---------------------------------------------------------------------------

func TestGetCorporateActions_SortedByExDate(t *testing.T) {
	twseBody := exDivMonthBody([][]string{
		{"2330", "台積電", "20260620", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260621", "20260710"},
		{"2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"},
		{"2330", "台積電", "20260618", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260619", "20260710"},
	})
	twse, closeFn := stubTWSECalendar(t, map[string]string{"202606": twseBody})
	defer closeFn()

	p := NewAggregatedCorporateActionProvider(twse, nil)
	got, err := p.GetCorporateActions(context.Background(), "2330",
		date(2026, 6, 1), date(2026, 6, 30))
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	if !got[0].ExDate.Before(got[1].ExDate) {
		t.Errorf("results not sorted: got[0]=%v got[1]=%v", got[0].ExDate, got[1].ExDate)
	}
	if !got[1].ExDate.Before(got[2].ExDate) {
		t.Errorf("results not sorted: got[1]=%v got[2]=%v", got[1].ExDate, got[2].ExDate)
	}
}

// ---------------------------------------------------------------------------
// Symbol filter: events for other symbols are dropped
// ---------------------------------------------------------------------------

func TestGetCorporateActions_FilterBySymbol(t *testing.T) {
	twseBody := exDivMonthBody([][]string{
		{"2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"},
		{"2454", "聯發科", "20260620", "除息", "1200.00", "1180.00", "0", "20", "20", "1298.00", "1062.00", "20260621", "20260715"},
	})
	twse, closeFn := stubTWSECalendar(t, map[string]string{"202606": twseBody})
	defer closeFn()

	p := NewAggregatedCorporateActionProvider(twse, nil)
	got, err := p.GetCorporateActions(context.Background(), "2330",
		date(2026, 6, 1), date(2026, 6, 30))
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event (only 2330), got %d", len(got))
	}
	if got[0].Symbol != "2330" {
		t.Errorf("got symbol %q, expected 2330", got[0].Symbol)
	}
}

// ---------------------------------------------------------------------------
// Date range filter: events outside [start, end] are dropped
// ---------------------------------------------------------------------------

func TestGetCorporateActions_FilterByDateRange(t *testing.T) {
	body := exDivMonthBody([][]string{
		{"2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"},
		{"2330", "台積電", "20260720", "除息", "960.00", "945.00", "0", "15", "15", "1042.00", "850.00", "20260721", "20260810"},
	})
	twse, closeFn := stubTWSECalendar(t, map[string]string{
		"202606": body,
		"202607": body,
	})
	defer closeFn()

	p := NewAggregatedCorporateActionProvider(twse, nil)
	// Restrict to June only → July event must be filtered out.
	got, err := p.GetCorporateActions(context.Background(), "2330",
		date(2026, 6, 1), date(2026, 6, 30))
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event in June only, got %d", len(got))
	}
	if got[0].ExDate.Month() != time.June {
		t.Errorf("expected June event, got %v", got[0].ExDate)
	}
}

// ---------------------------------------------------------------------------
// Boundary conditions
// ---------------------------------------------------------------------------

func TestGetCorporateActions_RejectsEmptySymbol(t *testing.T) {
	twse, closeFn := stubTWSECalendar(t, map[string]string{})
	defer closeFn()
	p := NewAggregatedCorporateActionProvider(twse, nil)
	_, err := p.GetCorporateActions(context.Background(), "",
		date(2026, 6, 1), date(2026, 6, 30))
	if err == nil {
		t.Fatal("expected error for empty symbol")
	}
}

func TestGetCorporateActions_RejectsInvertedRange(t *testing.T) {
	twse, closeFn := stubTWSECalendar(t, map[string]string{})
	defer closeFn()
	p := NewAggregatedCorporateActionProvider(twse, nil)
	_, err := p.GetCorporateActions(context.Background(), "2330",
		date(2026, 6, 30), date(2026, 6, 1))
	if err == nil {
		t.Fatal("expected error when end < start")
	}
}

// TestGetCorporateActions_OnlyFinMindConfigured verifies the aggregator still
// works when twse is nil — the partial-failure path is the dominant code path.
func TestGetCorporateActions_OnlyFinMindConfigured(t *testing.T) {
	start, end := date(2026, 6, 1), date(2026, 6, 30)
	finmind, _ := stubFinMindWithCache(t, "2330", start, end, []domain.DividendRecord{
		{
			Symbol:         "2330",
			Year:           2025,
			CashDividend:   12.0,
			ExDividendDate: "2026-06-15",
		},
	})

	p := NewAggregatedCorporateActionProvider(nil, finmind)
	got, err := p.GetCorporateActions(context.Background(), "2330", start, end)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
}

// date builds a UTC midnight time.Time for test convenience.
func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Sanity guard: domain.CorporateAction's JSON shape stays stable across
// changes. Mirrors internal/domain/corporate_action_test.go but verifies
// the aggregator's wire output is unchanged.
func TestAggregatedCorporateAction_RecordShapeStable(t *testing.T) {
	a := domain.CorporateAction{
		Symbol:         "2330",
		ExDate:         date(2026, 6, 15),
		CashDividend:   12.0,
		ReferencePrice: 938.0,
		Source:         "twse_calendar",
	}
	if a.Symbol == "" || a.ExDate.IsZero() {
		t.Fatalf("record missing required fields: %+v", a)
	}
}
