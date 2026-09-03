package industry

// 2026-09-03 per-industry fault-tolerance regression tests (auto_cycle_update):
// a single industry with no data must be skipped (warn + report.Skipped), not
// fail the whole 6h task — while genuine hard failures (quota / rate-limit)
// still fail a fully-failed round with a truthful root cause.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// emptySymbols in the mock FinMind below is the set of data_ids that have NO
// data (weekly/monthly gap, newly listed, etc.) — the per-industry no-data
// condition the 2026-09-03 fix targets (production: laser_communication).
const emptySymbols = "3426"

// faultToleranceTree builds a minimal 2-industry tree: has_data has two
// stocks that resolve, empty_industry has one stock with no upstream data.
func faultToleranceTree() *ClassificationTree {
	tree := NewClassificationTree()
	tree.AddSegment(&IndustrySegment{
		ID:                   "has_data",
		Name:                 "有資料產業",
		NameEN:               "HasData",
		Level:                Level1,
		RepresentativeStocks: []string{"1111.TW", "2222.TW"},
	})
	tree.AddSegment(&IndustrySegment{
		ID:                   "empty_industry",
		Name:                 "無資料產業",
		NameEN:               "Empty",
		Level:                Level1,
		RepresentativeStocks: []string{"3426.TW"},
	})
	return tree
}

// quarterEndDates returns the four quarter-end date strings for year.
func quarterEndDates(year string) []string {
	return []string{
		fmt.Sprintf("%s-03-31", year),
		fmt.Sprintf("%s-06-30", year),
		fmt.Sprintf("%s-09-30", year),
		fmt.Sprintf("%s-12-31", year),
	}
}

// finmindMockServer serves FinMind-style envelopes. mode:
//   - "partial": only emptySymbols lacks data (single no-data industry)
//   - "all_empty": every symbol lacks data (all-skip round)
//   - "quota": every request answers HTTP 402 (hard failure round)
func finmindMockServer(mode string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mode == "quota" {
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit","status":402,"data":[]}`))
			return
		}
		q := r.URL.Query()
		dataset := q.Get("dataset")
		dataID := q.Get("data_id")
		isEmpty := mode == "all_empty" || (mode == "partial" && dataID == emptySymbols)

		switch dataset {
		case "TaiwanStockMonthRevenue":
			if isEmpty {
				_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
				return
			}
			start := q.Get("start_date")
			_, _ = fmt.Fprintf(w, `{"msg":"success","status":200,"data":[{"date":%q,"revenue":100.0}]}`, start)
		case "TaiwanStockFinancialStatements":
			if isEmpty {
				// Rows whose origin_name extractProfit does not recognize →
				// current/prev profit both read 0 → the quarter fallback
				// exhausts and the industry reports profit no-data too.
				year := ""
				if s := q.Get("start_date"); len(s) >= 4 {
					year = s[:4]
				}
				rows := make([]string, 0, 4)
				for _, d := range quarterEndDates(year) {
					rows = append(rows, fmt.Sprintf(`{"date":%q,"origin_name":"非淨利欄位","value":1.0}`, d))
				}
				_, _ = fmt.Fprintf(w, `{"msg":"success","status":200,"data":[%s]}`, strings.Join(rows, ","))
				return
			}
			year := ""
			if s := q.Get("start_date"); len(s) >= 4 {
				year = s[:4]
			}
			rows := make([]string, 0, 4)
			for _, d := range quarterEndDates(year) {
				rows = append(rows, fmt.Sprintf(`{"date":%q,"origin_name":"NetIncome","value":200.0}`, d))
			}
			_, _ = fmt.Fprintf(w, `{"msg":"success","status":200,"data":[%s]}`, strings.Join(rows, ","))
		default:
			_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
		}
	}))
}

func newFaultToleranceAggregator(mode string) (*DataAggregator, *httptest.Server) {
	srv := finmindMockServer(mode)
	client := marketdata.NewFinMindClient("test-key")
	client.SetBaseURL(srv.URL)
	client.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	a := NewDataAggregator(NewCycleTracker(), faultToleranceTree(), client, nil)
	return a, srv
}

// TestAggregateAllIndustriesReport_SingleNoDataIndustrySkipped is the core
// regression for the production symptom: auto_cycle_update failing 4 runs in
// a row with last_error "no valid data for industry laser_communication"
// while every other industry had data. A no-data industry must be skipped
// (warn + Skipped++), the rest must update, and the task must NOT fail.
func TestAggregateAllIndustriesReport_SingleNoDataIndustrySkipped(t *testing.T) {
	a, srv := newFaultToleranceAggregator("partial")
	defer srv.Close()

	report, err := a.AggregateAllIndustriesReport(context.Background())
	if err != nil {
		t.Fatalf("single no-data industry must not fail the round, got %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Attempted != 2 {
		t.Errorf("Attempted = %d, want 2", report.Attempted)
	}
	if report.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1 (has_data must still update)", report.Succeeded)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (empty_industry must be counted as skipped)", report.Skipped)
	}
	// The empty industry shows up in the per-industry detail as failed with
	// the no-data reason (still visible for dashboards), never as succeeded.
	foundEmpty := false
	for _, st := range report.Industries {
		if st.IndustryID == "empty_industry" {
			foundEmpty = true
			if st.Succeeded {
				t.Error("empty_industry must not be marked succeeded")
			}
			if !strings.Contains(st.Error, "no valid data") {
				t.Errorf("empty_industry error = %q, want no-data reason", st.Error)
			}
		}
	}
	if !foundEmpty {
		t.Error("empty_industry missing from per-industry detail")
	}
	// has_data updated the tracker.
	if _, ok := a.tracker.GetPosition("has_data"); !ok {
		t.Error("has_data position missing — successful industry should have updated CycleTracker")
	}
}

// TestAggregateAllIndustriesReport_AllNoDataSkipped: when every industry is
// no-data this round (e.g. pre-publication day), the round reports all-skipped
// and returns nil — nothing was wrong, there was simply nothing to update.
func TestAggregateAllIndustriesReport_AllNoDataSkipped(t *testing.T) {
	a, srv := newFaultToleranceAggregator("all_empty")
	defer srv.Close()

	report, err := a.AggregateAllIndustriesReport(context.Background())
	if err != nil {
		t.Fatalf("all-no-data round must not be an error, got %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Succeeded != 0 {
		t.Errorf("Succeeded = %d, want 0", report.Succeeded)
	}
	if report.Skipped != report.Attempted || report.Attempted == 0 {
		t.Errorf("Skipped = %d, Attempted = %d — want all attempted skipped", report.Skipped, report.Attempted)
	}
}

// TestAggregateAllIndustriesReport_QuotaStillFailsRound: quota exhaustion is a
// hard failure, not a skip — when every industry is blocked by quota the round
// must still return an error, and the error chain must carry the typed
// ErrQuotaExhausted sentinel (fixes the misleading "no valid data for industry
// X" last_error that masked quota as no_data).
func TestAggregateAllIndustriesReport_QuotaStillFailsRound(t *testing.T) {
	a, srv := newFaultToleranceAggregator("quota")
	defer srv.Close()

	report, err := a.AggregateAllIndustriesReport(context.Background())
	if err == nil {
		t.Fatal("expected error when every industry is quota-blocked")
	}
	if !errors.Is(err, marketdata.ErrQuotaExhausted) {
		t.Errorf("err = %v, want chain wrapping marketdata.ErrQuotaExhausted (typed root cause preserved)", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Succeeded != 0 || report.Skipped != 0 {
		t.Errorf("Succeeded/Skipped = %d/%d, want 0/0 for a hard-failure round", report.Succeeded, report.Skipped)
	}
}

// TestAggregateIndustry_QuotaErrorNotMaskedAsNoData verifies the per-industry
// root-cause chain: when every symbol fails on quota, AggregateIndustry's
// error classifies as quota (not no_data) so telemetry and the aggregate
// last_error tell the truth.
func TestAggregateIndustry_QuotaErrorNotMaskedAsNoData(t *testing.T) {
	a, srv := newFaultToleranceAggregator("quota")
	defer srv.Close()

	err := a.AggregateIndustry(context.Background(), "has_data")
	if err == nil {
		t.Fatal("expected error from quota-blocked industry")
	}
	if kind := classifyFinMindError(err); kind != "quota" {
		t.Errorf("kind = %q, want quota (quota root cause must not degrade to no_data)", kind)
	}
	if !errors.Is(err, marketdata.ErrQuotaExhausted) {
		t.Errorf("err = %v, want chain wrapping marketdata.ErrQuotaExhausted", err)
	}
}

// TestAggregateIndustry_NoDataErrorStillClassifiesNoData guards the opposite
// edge: a genuinely data-less industry keeps kind=no_data so the round can
// skip it.
func TestAggregateIndustry_NoDataErrorStillClassifiesNoData(t *testing.T) {
	a, srv := newFaultToleranceAggregator("partial")
	defer srv.Close()

	err := a.AggregateIndustry(context.Background(), "empty_industry")
	if err == nil {
		t.Fatal("expected no-data error from empty industry")
	}
	if kind := classifyFinMindError(err); kind != "no_data" {
		t.Errorf("kind = %q, want no_data", kind)
	}
}
