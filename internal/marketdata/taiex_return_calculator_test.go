package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTAIEXReturnCalculator_Success(t *testing.T) {
	var callCount int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if callCount == 2 {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":19000,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000,19100]}]}}]}}`))
		} else {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":20000,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000,20000]}]}}]}}`))
		}
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ret, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err != nil {
		t.Fatalf("Get1MonthReturn failed: %v", err)
	}

	expected := (20000.0 - 19000.0) / 19000.0
	if ret < expected-0.0001 || ret > expected+0.0001 {
		t.Errorf("return = %v, want ~%v", ret, expected)
	}
}

func TestTAIEXReturnCalculator_CurrentPriceFromCloses(t *testing.T) {
	var callCount int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if callCount == 2 {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":19000,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000]}]}}]}}`))
		} else {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":0,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000,20000]}]}}]}}`))
		}
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ret, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err != nil {
		t.Fatalf("Get1MonthReturn failed: %v", err)
	}

	expected := (20000.0 - 19000.0) / 19000.0
	if ret < expected-0.0001 || ret > expected+0.0001 {
		t.Errorf("return = %v, want ~%v", ret, expected)
	}
}

func TestTAIEXReturnCalculator_APIFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

func TestTAIEXReturnCalculator_HTMLResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>Rate Limited</body></html>`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err == nil {
		t.Fatal("expected error on HTML response")
	}
}

func TestTAIEXReturnCalculator_InvalidJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestTAIEXReturnCalculator_HostFallback(t *testing.T) {
	var callCount int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		if callCount == 3 {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":19000,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000,19100]}]}}]}}`))
		} else {
			w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":20000,"regularMarketTime":1700000000},"indicators":{"quote":[{"close":[19000,20000]}]}}]}}`))
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	origHosts := yahooHosts
	yahooHosts = []string{host, host}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ret, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err != nil {
		t.Fatalf("Get1MonthReturn failed after fallback: %v", err)
	}

	expected := (20000.0 - 19000.0) / 19000.0
	if ret < expected-0.0001 || ret > expected+0.0001 {
		t.Errorf("return = %v, want ~%v", ret, expected)
	}
}

func TestTAIEXReturnCalculator_NoValidPrice(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":0,"regularMarketTime":0},"indicators":{"quote":[{"close":[null,null]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err == nil {
		t.Fatal("expected error when no valid price data")
	}
}

func TestTAIEXReturnCalculator_EmptyResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXReturnCalculator().Get1MonthReturn(context.Background())
	if err == nil {
		t.Fatal("expected error on empty result")
	}
}
