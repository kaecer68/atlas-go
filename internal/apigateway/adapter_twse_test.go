package apigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTWSEChannelAdapter_Metadata(t *testing.T) {
	a := &TWSEChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "twse_replay" {
		t.Errorf("ChannelID = %q, want twse_replay", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE 證交所" {
		t.Errorf("Platform = %q, want TWSE 證交所", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw" {
		t.Errorf("Path = %q, want www.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEChannelAdapter_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exchangeReport/STOCK_DAY_ALL" {
			t.Errorf("path = %q, want /exchangeReport/STOCK_DAY_ALL", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260101","title":"","fields":["證券代號","證券名稱","成交股數","成交金額","開盤價","最高價","最低價","收盤價","漲跌價差","成交筆數"],"data":[["2330","台積電","1000","600000","599","601","598","600","1","100"]]}`))
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewTWSEClient()
	client.SetHTTPClient(withClientMockTransport(server, "www.twse.com.tw"))

	adapter := NewTWSEChannelAdapter(client)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "twse_replay" {
		t.Errorf("ChannelID = %q, want twse_replay", res.Meta.ChannelID)
	}
}

func TestTWSEChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260101","data":[["2330","台積電","1000","600000","599","601","598","600","1","100"]]}`))
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewTWSEClient()
	client.SetHTTPClient(withClientMockTransport(server, "www.twse.com.tw"))

	adapter := NewTWSEChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestTWSEChannelAdapter_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewTWSEClient()
	client.SetHTTPClient(withClientMockTransport(server, "www.twse.com.tw"))

	adapter := NewTWSEChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestTWSEChannelAdapter_RateLimit(t *testing.T) {
	writeParametersJSON(t, nil)
	client := marketdata.NewTWSEClient()
	adapter := NewTWSEChannelAdapter(client)
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
