package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFinMindClient_GetMarginMaintenanceLatest_Success verifies the whole-market
// margin maintenance ratio fetch picks the latest published row (PR-2).
func TestFinMindClient_GetMarginMaintenanceLatest_Success(t *testing.T) {
	var capturedDataset string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDataset = r.URL.Query().Get("dataset")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"success","status":200,"data":[
			{"date":"2026-09-01","TotalExchangeMarginMaintenance":193.95},
			{"date":"2026-09-02","TotalExchangeMarginMaintenance":190.138},
			{"date":"2026-09-03","TotalExchangeMarginMaintenance":184.437}
		]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("test-key")
	c.SetBaseURL(ts.URL)

	rowDate, ratio, err := c.GetMarginMaintenanceLatest(context.Background(), "2026-09-04")
	if err != nil {
		t.Fatalf("GetMarginMaintenanceLatest error: %v", err)
	}
	if capturedDataset != "TaiwanTotalExchangeMarginMaintenance" {
		t.Errorf("dataset = %q, want TaiwanTotalExchangeMarginMaintenance", capturedDataset)
	}
	if rowDate != "2026-09-03" {
		t.Errorf("rowDate = %q, want 2026-09-03 (latest row)", rowDate)
	}
	if ratio != 184.437 {
		t.Errorf("ratio = %v, want 184.437", ratio)
	}
}

// TestFinMindClient_GetMarginMaintenanceLatest_NoData covers the legitimate
// "not published yet" case (ratio released after TWSE evening processing).
func TestFinMindClient_GetMarginMaintenanceLatest_NoData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("test-key")
	c.SetBaseURL(ts.URL)

	_, _, err := c.GetMarginMaintenanceLatest(context.Background(), "2026-09-04")
	if !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want ErrNoData wrap", err)
	}
	if !strings.Contains(err.Error(), "2026-09-04") {
		t.Errorf("error should mention the requested end date, got %v", err)
	}
}
