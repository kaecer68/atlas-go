package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// newTestMarketVolumeProvider wires a provider to a stub upstream.
func newTestMarketVolumeProvider(t *testing.T, handler http.HandlerFunc) (*MarketVolumeProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL
	return provider, srv
}

// marketStatsTitle renders the ROC-dated title TWSE prefixes onto the
// 大盤統計資訊 section, e.g. "115年09月24日 大盤統計資訊". An unusable date yields
// the empty title (the schema-change shape) rather than failing: the stub has to
// survive the malformed-input cases.
func marketStatsTitle(compact string) string {
	d, err := time.Parse("20060102", compact)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d年%02d月%02d日 大盤統計資訊", d.Year()-1911, int(d.Month()), d.Day())
}

// marketStatsResponse builds a type=MS answer whose 大盤統計資訊 section carries
// the given title date and 一般股票 成交金額(元).
func marketStatsResponse(titleDateCompact, amount string) twseMIIndexResponse {
	tables := make([]twseMITable, 7)
	tables[6] = twseMITable{
		Title:  marketStatsTitle(titleDateCompact),
		Fields: []string{"成交統計", "成交金額(元)", "成交股數(股)", "成交筆數"},
		Data: [][]string{
			{"1.一般股票", amount, "3,609,044,470", "3,037,959"},
		},
	}
	return twseMIIndexResponse{Stat: "OK", Tables: tables}
}

// serveMarketStats answers every request with an honest upstream shape: the
// section title repeats the date the caller asked for, while the envelope date
// echoes the request (TWSE does that even for dates it cannot answer).
func serveMarketStats(t *testing.T, amount string, mutate func(*twseMIIndexResponse, string)) (*MarketVolumeProvider, *httptest.Server) {
	t.Helper()
	return newTestMarketVolumeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		resp := marketStatsResponse(date, amount)
		resp.Date = date
		if mutate != nil {
			mutate(&resp, date)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func TestMarketVolumeProvider_ParseNormalData(t *testing.T) {
	provider, _ := serveMarketStats(t, "687,230,800,745", nil)

	result, err := provider.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result.MarketVolume <= 0 {
		t.Fatalf("expected positive MarketVolume, got %f", result.MarketVolume)
	}
	// 687,230,800,745 元 / 1e8 = 6872.30800745 億元
	expected := 687230800745.0 / 100_000_000
	if result.MarketVolume != expected {
		t.Errorf("MarketVolume = %f, want %f", result.MarketVolume, expected)
	}
	// FetchLatest walks back over expected trading days only
	// (#1767 calendar-aware scan), so on weekends the first hit is
	// the most recent expected trading day (e.g. Friday), not the
	// calendar today. Compare against the scan window instead of a
	// single "today": the test can straddle a UTC midnight boundary,
	// where the provider's own time.Now() and this assertion's may
	// differ by one day (flaky on CI). Three trading days covers any
	// such crossing.
	window := map[string]bool{}
	var windowKeys []string
	for _, day := range RecentTradingDays(time.Now().UTC(), 3) {
		key := day.Format("20060102")
		window[key] = true
		windowKeys = append(windowKeys, key)
	}
	if !window[result.Date] {
		t.Errorf("Date = %q, want one of recent expected trading days %v", result.Date, windowKeys)
	}
}

// TestMarketVolumeProvider_UsesThePayloadsOwnDataDate pins E29-2 on the REAL
// payload: the day the numbers describe comes from the section title, not from
// the request, and the VALUE is bit-for-bit the one production stored for that
// day (TSE_VOLUME 20260924 = 7188.80491672 億 ⇒ the fix changed provenance
// only, not any trading-day number).
func TestMarketVolumeProvider_UsesThePayloadsOwnDataDate(t *testing.T) {
	body, err := os.ReadFile("testdata/mi_index_ms_20260924.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	result, err := provider.FetchDate(context.Background(), "20260924")
	if err != nil {
		t.Fatalf("FetchDate(20260924): %v", err)
	}
	if result.Date != "20260924" {
		t.Errorf("Date = %q, want 20260924 (the payload's own日期)", result.Date)
	}
	// 718,880,491,672 元 ÷ 1e8. Exactly the production TSE_VOLUME for 20260924.
	if want := 7188.80491672; result.MarketVolume != want {
		t.Errorf("MarketVolume = %v, want %v (production TSE_VOLUME for 20260924)", result.MarketVolume, want)
	}
}

// TestMarketVolumeProvider_RefusesPayloadForAnotherDate is the E29-2 defect
// itself: the real 2026-09-18 payload requested as 2026-09-24. Before the fix
// the provider stamped the REQUEST date onto it (Date=20260924 with 09-18's
// 10604.0117475 億 — a phantom trading day); now the mismatch is refused so the
// caller can walk back to a day the exchange really published.
func TestMarketVolumeProvider_RefusesPayloadForAnotherDate(t *testing.T) {
	body, err := os.ReadFile("testdata/mi_index_ms_20260918.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	result, err := provider.FetchDate(context.Background(), "20260924")
	if err == nil {
		t.Fatalf("mis-dated payload accepted: Date=%q MarketVolume=%v (want a refusal)", result.Date, result.MarketVolume)
	}
	for _, want := range []string{"2026-09-18", "2026-09-24", "mis-dated"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %q", err.Error(), want)
		}
	}

	// FetchLatest must not fall for it either: every candidate date gets the
	// same 09-18 payload, so all of them mismatch.
	if _, err := provider.FetchLatest(context.Background()); err == nil {
		t.Error("FetchLatest accepted a payload whose own date never matches the request")
	}
}

// TestMarketVolumeProvider_RefusesUntitledTable: a payload that carries the
// market-stats section but no dated title has no provable data date — refuse
// (schema change), never fall back to the request date.
func TestMarketVolumeProvider_RefusesUntitledTable(t *testing.T) {
	provider, _ := serveMarketStats(t, "687,230,800,745", func(resp *twseMIIndexResponse, _ string) {
		resp.Tables[6].Title = "大盤統計資訊" // no ROC date
	})

	if _, err := provider.FetchDate(context.Background(), "20260924"); err == nil {
		t.Fatal("untitled market-stats table accepted (no provable data date)")
	} else if !strings.Contains(err.Error(), "no data date") {
		t.Errorf("refusal %q does not explain the missing data date", err.Error())
	}
}

// TestMarketVolumeProvider_SchemaChangeWithoutTheStatsSection: the section is
// selected by field names, so a body that no longer carries 成交統計/成交金額(元)
// is reported as a schema change instead of being read positionally.
func TestMarketVolumeProvider_SchemaChangeWithoutTheStatsSection(t *testing.T) {
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		resp := twseMIIndexResponse{Stat: "OK", Date: r.URL.Query().Get("date"), Tables: make([]twseMITable, 7)}
		resp.Tables[6] = twseMITable{Title: marketStatsTitle(r.URL.Query().Get("date")), Fields: []string{"成交統計"}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	if _, err := provider.FetchDate(context.Background(), "20260924"); err == nil {
		t.Fatal("body without a 大盤統計資訊 section accepted")
	} else if !strings.Contains(err.Error(), "大盤統計資訊") {
		t.Errorf("error %q does not name the missing section", err.Error())
	}
}

func TestMarketVolumeProvider_EmptyTable(t *testing.T) {
	// 休市日：tables 存在但 data 為空
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		resp := marketStatsResponse("20260726", "0")
		resp.Tables[6].Data = [][]string{}
		resp.Date = r.URL.Query().Get("date")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 空資料 → 應該要 fallback 到下一天，最終回傳 error
	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for empty market stats, got nil")
	}
}

func TestMarketVolumeProvider_NonOKStat(t *testing.T) {
	// The closed-market answer, measured 2026-09-27 for 20260925 (中秋節): 10
	// empty tables, a prose stat, and an envelope date echoing the request.
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Stat   string `json:"stat"`
			Date   string `json:"date"`
			Tables []twseMITable
		}{
			Stat:   "很抱歉，沒有符合條件的資料!",
			Date:   r.URL.Query().Get("date"),
			Tables: make([]twseMITable, 10),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for non-OK stat, got nil")
	}
	if !strings.Contains(err.Error(), "沒有符合條件的資料") {
		t.Errorf("error %q should quote the upstream stat", err.Error())
	}
}

func TestMarketVolumeProvider_NoPanicOnMalformedJSON(t *testing.T) {
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	})

	// Should not panic — returns error gracefully.
	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestMarketVolumeProvider_InvalidAmountRow(t *testing.T) {
	// Row 0 col 1 is non-numeric → parseTWSEFloat returns 0 → treated as non-positive
	provider, _ := serveMarketStats(t, "--", nil)

	_, err := provider.FetchDate(context.Background(), "20260924")
	if err == nil {
		t.Fatal("expected error for non-positive amount, got nil")
	}
}

func TestMarketVolumeProvider_FetchDate(t *testing.T) {
	// FetchDate must request exactly the given date and parse the response.
	var gotUA string
	provider, _ := newTestMarketVolumeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		resp := marketStatsResponse(r.URL.Query().Get("date"), "520,025,000,000")
		resp.Date = r.URL.Query().Get("date")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	result, err := provider.FetchDate(context.Background(), "20260427")
	if err != nil {
		t.Fatalf("FetchDate: %v", err)
	}
	if gotUA != twseBrowserUserAgent {
		t.Errorf("User-Agent = %q, want full browser UA (TWSE WAF rejects short UAs)", gotUA)
	}
	if result.Date != "20260427" {
		t.Errorf("result date = %q, want 20260427", result.Date)
	}
	expected := 520025000000.0 / 100_000_000 // 5200.25 億元
	if result.MarketVolume != expected {
		t.Errorf("MarketVolume = %f, want %f", result.MarketVolume, expected)
	}
}

// TestMarketVolumeProvider_InvaliddateRejected: the requested date is validated
// before it is used as a provenance comparison.
func TestMarketVolumeProvider_InvaliddateRejected(t *testing.T) {
	provider, _ := serveMarketStats(t, "687,230,800,745", nil)
	if _, err := provider.FetchDate(context.Background(), "2026-09-24"); err == nil {
		t.Fatal("malformed requested date accepted")
	}
}
