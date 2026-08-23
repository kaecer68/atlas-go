package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// newStubTWSEServer returns a minimal TWSE broker-table response that
// simulates the real ASP.NET form flow: GET bsMenu.aspx returns the form
// with viewstate tokens, and POST bsMenu.aspx returns the broker table.
func newStubTWSEServer(t *testing.T, branchCode string) *httptest.Server {
	t.Helper()
	formHTML := "<html><body><form><input id=\"__VIEWSTATE\" value=\"%2FwEPDwUKLTgxNDI2MzM4MGRk\">" +
		"<input id=\"__VIEWSTATEGENERATOR\" value=\"AA1F01CB\">" +
		"<input id=\"__EVENTVALIDATION\" value=\"%2FwEWAwKz\"></form></body></html>"
	tableHTML := "<html><body><table><tr>" +
		"<th>券商代號</th><th>券商名稱</th><th>買進</th><th>賣出</th><th>淨買</th>" +
		"</tr><tr>" +
		"<td>" + branchCode + "</td><td>HeadOffice</td><td>1000</td><td>0</td><td>1000</td>" +
		"</tr></table></body></html>"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(formHTML))
		case http.MethodPost:
			_, _ = w.Write([]byte(tableHTML))
		default:
			_, _ = w.Write([]byte(tableHTML))
		}
	}))
}

func TestGovernmentBrokerChannelAdapter_Fetch(t *testing.T) {
	dir := t.TempDir()

	// Use only the first symbol so the test stays fast and deterministic.
	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	agg.SetSymbols([]string{"2330"})

	// The first symbol maps to branch 8060 (合作金庫).
	server := newStubTWSEServer(t, "8060")
	defer server.Close()

	agg.SetHTTPClient(server.Client())
	agg.SetBaseURL(server.URL)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	ctx := context.Background()
	res, err := adapter.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty result")
	}
	if res.Meta.ChannelID != "government_broker" {
		t.Errorf("ChannelID = %q, want government_broker", res.Meta.ChannelID)
	}

	var reading marketdata.GovernmentFlowReading
	if err := json.Unmarshal(res.Data, &reading); err != nil {
		t.Fatalf("Unmarshal reading: %v", err)
	}
	if reading.TotalNet == 0 {
		t.Error("TotalNet should be non-zero for a matched bank branch")
	}
	if reading.Source != "broker-aggregate" {
		t.Errorf("Source = %q, want broker-aggregate", reading.Source)
	}

	// Verify the per-broker detail file is also written (PR-A).
	detailPath := filepath.Join(dir, reading.Date+"_brokers.json")
	detailBytes, err := os.ReadFile(detailPath)
	if err != nil {
		t.Fatalf("Read detail file: %v", err)
	}
	var detail struct {
		Date    string                         `json:"date"`
		Source  string                         `json:"source"`
		Brokers []marketdata.BrokerDailyDetail `json:"brokers"`
	}
	if err := json.Unmarshal(detailBytes, &detail); err != nil {
		t.Fatalf("Unmarshal detail file: %v", err)
	}
	if len(detail.Brokers) == 0 {
		t.Error("Detail file should contain at least one broker")
	}
	found8060 := false
	for _, b := range detail.Brokers {
		if b.Code == "8060" && b.Type == "gov" {
			found8060 = true
			if b.Net != reading.TotalNet {
				t.Errorf("Detail net %d != aggregate total %d", b.Net, reading.TotalNet)
			}
		}
	}
	if !found8060 {
		t.Errorf("Expected broker 8060 in detail file, got %+v", detail.Brokers)
	}
}

// TestGovernmentBrokerChannelAdapter_Fetch_NoStocksOk verifies the contract
// for non-trading-day outcomes: when the upstream TWSE page returns no
// broker data (holiday, weekend, upstream temporarily empty), the adapter
// must NOT surface an error — it returns a stub payload with status="no_data"
// so the dashboard sees a successful fetch and the channel-health page
// does not page on-call (regression: 2026-08-03 "no stocks processed"
// false-positive error in channel_health.govbroker).
func TestGovernmentBrokerChannelAdapter_Fetch_NoStocksOk(t *testing.T) {
	dir := t.TempDir()
	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	agg.SetSymbols([]string{"2330"})

	// Server returns 500 on GET so fetchMenuTokens fails — simulating an
	// upstream TWSE outage where the page is unreachable but no broker data
	// was returned. AggregateDate should treat this as "no stocks processed"
	// (nil, nil) rather than an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	agg.SetHTTPClient(server.Client())
	agg.SetBaseURL(server.URL)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() should NOT error on no-data (was a regression for 2026-08-03), got: %v", err)
	}
	if res == nil {
		t.Fatal("Fetch() returned nil result — expected stub no_data payload")
	}
	var payload struct {
		Date   string `json:"date"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(res.Data, &payload); err != nil {
		t.Fatalf("Unmarshal stub payload: %v", err)
	}
	if payload.Status != "no_data" {
		t.Errorf("payload.Status = %q, want no_data", payload.Status)
	}
	if res.Meta.ChannelID != "government_broker" {
		t.Errorf("Meta.ChannelID = %q, want government_broker", res.Meta.ChannelID)
	}
}

func TestGovernmentBrokerChannelAdapter_HealthCheck_OK(t *testing.T) {
	dir := t.TempDir()

	// Write a recent reading file.
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	path := filepath.Join(dir, date.Format("20060102")+".json")
	if err := os.WriteFile(path, []byte(`{"date":"`+date.Format("20060102")+`","total_net":1234}`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.CheckType != "readiness" {
		t.Errorf("CheckType = %q, want readiness", status.CheckType)
	}
}

func TestGovernmentBrokerChannelAdapter_HealthCheck_Missing(t *testing.T) {
	dir := t.TempDir()
	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck() expected error for missing file")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestGovernmentBrokerChannelAdapter_Metadata(t *testing.T) {
	agg := marketdata.NewGovernmentBrokerAggregator(t.TempDir())
	adapter := NewGovernmentBrokerChannelAdapter(agg)
	m := adapter.Metadata()
	if m.ChannelID != "government_broker" {
		t.Errorf("ChannelID = %q, want government_broker", m.ChannelID)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

// TestGovernmentBrokerChannelAdapter_DataState_Missing verifies DataState
// reports Present=false when no reading file exists for the previous trading
// day — the "ok 假象" condition (fetch succeeded, no data landed).
func TestGovernmentBrokerChannelAdapter_DataState_Missing(t *testing.T) {
	dir := t.TempDir()
	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	ds, err := adapter.DataState(context.Background())
	if err != nil {
		t.Fatalf("DataState() error = %v", err)
	}
	if ds.Present {
		t.Error("Present = true, want false (no reading file written)")
	}
	if ds.NonZero {
		t.Error("NonZero = true, want false")
	}
	if ds.Detail == "" {
		t.Error("Detail should explain which date/file is missing")
	}
}

// TestGovernmentBrokerChannelAdapter_DataState_NonZero verifies DataState
// reports Present + NonZero for a real reading with total_net != 0.
func TestGovernmentBrokerChannelAdapter_DataState_NonZero(t *testing.T) {
	dir := t.TempDir()
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	path := filepath.Join(dir, date.Format("20060102")+".json")
	if err := os.WriteFile(path, []byte(`{"date":"`+date.Format("20060102")+`","total_net":1234,"source":"broker-aggregate"}`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	ds, err := adapter.DataState(context.Background())
	if err != nil {
		t.Fatalf("DataState() error = %v", err)
	}
	if !ds.Present {
		t.Error("Present = false, want true")
	}
	if !ds.NonZero {
		t.Error("NonZero = false, want true (total_net=1234)")
	}
}

// TestGovernmentBrokerChannelAdapter_DataState_Zero verifies DataState
// reports Present but NonZero=false when the reading file has total_net=0 —
// the value_nonzero contract treats this as failed data.
func TestGovernmentBrokerChannelAdapter_DataState_Zero(t *testing.T) {
	dir := t.TempDir()
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	path := filepath.Join(dir, date.Format("20060102")+".json")
	if err := os.WriteFile(path, []byte(`{"date":"`+date.Format("20060102")+`","total_net":0,"source":"broker-aggregate"}`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	ds, err := adapter.DataState(context.Background())
	if err != nil {
		t.Fatalf("DataState() error = %v", err)
	}
	if !ds.Present {
		t.Error("Present = false, want true (file exists)")
	}
	if ds.NonZero {
		t.Error("NonZero = true, want false (total_net=0 fails value_nonzero)")
	}
}

// TestGovernmentBrokerChannelAdapter_DataState_NoDataStub verifies a no_data
// stub file (written by the adapter on the "no stocks processed" path) is
// reported as present-but-empty, which fails value_nonzero.
func TestGovernmentBrokerChannelAdapter_DataState_NoDataStub(t *testing.T) {
	dir := t.TempDir()
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	path := filepath.Join(dir, date.Format("20060102")+".json")
	if err := os.WriteFile(path, []byte(`{"date":"`+date.Format("20060102")+`","status":"no_data"}`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	agg := marketdata.NewGovernmentBrokerAggregator(dir)
	adapter := NewGovernmentBrokerChannelAdapter(agg)

	ds, err := adapter.DataState(context.Background())
	if err != nil {
		t.Fatalf("DataState() error = %v", err)
	}
	if !ds.Present {
		t.Error("Present = false, want true (stub file exists)")
	}
	if ds.NonZero {
		t.Error("NonZero = true, want false (no_data stub has zero total_net)")
	}
}
