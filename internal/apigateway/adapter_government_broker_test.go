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

// newStubTWSEServer returns a minimal TWSE broker table response that contains
// one matching government-bank branch code.
func newStubTWSEServer(t *testing.T, branchCode string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The parser expects rows of: code name buy sell net
		_, _ = w.Write([]byte("<html><body><table><tr>" +
			"<td>" + branchCode + "</td><td>HeadOffice</td><td>1000</td><td>0</td><td>1000</td>" +
			"</tr></table></body></html>"))
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
}

func TestGovernmentBrokerChannelAdapter_HealthCheck_OK(t *testing.T) {
	dir := t.TempDir()

	// Write a recent reading file.
	date := previousTradingDay(time.Now())
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
