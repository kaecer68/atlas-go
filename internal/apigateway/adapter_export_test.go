package apigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestExportStatisticsChannelAdapter_Metadata(t *testing.T) {
	a := &ExportStatisticsChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "export_statistics" {
		t.Errorf("ChannelID = %q, want export_statistics", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "關務署" {
		t.Errorf("Platform = %q, want 關務署", m.Platform)
	}
	if m.APIFormat != "csv" {
		t.Errorf("APIFormat = %q, want csv", m.APIFormat)
	}
	if m.Path != "opendata.customs.gov.tw" {
		t.Errorf("Path = %q, want opendata.customs.gov.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestExportStatisticsChannelAdapter_RateLimit(t *testing.T) {
	a := NewExportStatisticsChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewExportStatisticsChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestExportStatisticsChannelAdapter_Fetch(t *testing.T) {
	csvBody := "年度,月份,出口總值(新臺幣千元),出口(新臺幣千元),復出口(新臺幣千元),進口總值(新臺幣千元),進口(新臺幣千元),復進口(新臺幣千元),出入超(新臺幣千元),備註\n" +
		"115,5,1000000,950000,50000,900000,850000,50000,100000,\n" +
		"115,4,950000,900000,50000,850000,800000,50000,100000,\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(csvBody))
	}))
	defer server.Close()

	provider := marketdata.ExportStatisticsProviderWithClient(withClientMockTransport(server, "opendata.customs.gov.tw"), t.TempDir())
	a := NewExportStatisticsChannelAdapter(provider)

	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
}

func TestExportStatisticsChannelAdapter_HealthCheck(t *testing.T) {
	csvBody := "年度,月份,出口總值(新臺幣千元),出口(新臺幣千元),復出口(新臺幣千元),進口總值(新臺幣千元),進口(新臺幣千元),復進口(新臺幣千元),出入超(新臺幣千元),備註\n" +
		"115,5,1000000,950000,50000,900000,850000,50000,100000,\n" +
		"115,4,950000,900000,50000,850000,800000,50000,100000,\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(csvBody))
	}))
	defer server.Close()

	provider := marketdata.ExportStatisticsProviderWithClient(withClientMockTransport(server, "opendata.customs.gov.tw"), t.TempDir())
	a := NewExportStatisticsChannelAdapter(provider)

	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestExportStatisticsChannelAdapter_HealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := marketdata.ExportStatisticsProviderWithClient(withClientMockTransport(server, "opendata.customs.gov.tw"), t.TempDir())
	a := NewExportStatisticsChannelAdapter(provider)

	status, err := a.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck() expected error")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}
