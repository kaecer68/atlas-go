package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// A05 typed-error contract tests for TWSEETFProvider.
//
// 背景：adapter 舊邏輯把「7 天無資料」的錯誤訊息當成假日預期（Stale），
// 無法區分 403/timeout（transport failure，應觸發 circuit breaker）與
// 正常休市。A05 引入 typed sentinel errors，只有 ErrETFNoTradingData
// 允許轉成 stale。

// etfTestServer 建立回傳固定 body 的 mock TWSE server。
func etfTestServer(t *testing.T, statusCode int, contentType, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// newETFTester 建立連到 mock server 的 provider，並繞過 rate limiter pacing。
func newETFTester(t *testing.T, ts *httptest.Server) *TWSEETFProvider {
	t.Helper()
	p := NewTWSEETFProvider()
	p.SetHTTPClient(ts.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.baseURL = ts.URL // 同 package 測試直接覆寫 baseURL 指向 mock server
	return p
}

func TestTWSEETFProvider_FetchLatest_Success(t *testing.T) {
	ts := etfTestServer(t, http.StatusOK, "application/json", `{
		"stat":"OK","date":"20260807",
		"tables":[{"fields":["名稱","申購","淨值","人數"],"data":[["0050","1000","2000","300"]]}]
	}`)
	p := newETFTester(t, ts)

	stats, err := p.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest error: %v", err)
	}
	if stats.Date == "" || stats.NetSubscription == 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestTWSEETFProvider_FetchLatest_AllNoData(t *testing.T) {
	// 7 天都回 stat!=OK（假日/休市正常無資料）→ ErrETFNoTradingData
	ts := etfTestServer(t, http.StatusOK, "application/json",
		`{"stat":"FAIL","date":"","tables":[]}`)
	p := newETFTester(t, ts)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrETFNoTradingData) {
		t.Errorf("err = %v, want ErrETFNoTradingData", err)
	}
}

func TestTWSEETFProvider_FetchLatest_Upstream403(t *testing.T) {
	// 403 = upstream 拒絕（IP rate-limit 推測的實際情況）→ ErrETFUpstream，
	// 必須能觸發 circuit breaker，不能偽裝成假日。
	ts := etfTestServer(t, http.StatusForbidden, "text/plain", "Forbidden")
	p := newETFTester(t, ts)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrETFUpstream) {
		t.Errorf("err = %v, want ErrETFUpstream", err)
	}
}

func TestTWSEETFProvider_FetchLatest_SchemaMismatch(t *testing.T) {
	// 回應不是預期 JSON（WAF 頁面 / schema 改變）→ ErrETFSchema
	ts := etfTestServer(t, http.StatusOK, "text/html", "<html>Forbidden</html>")
	p := newETFTester(t, ts)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrETFSchema) {
		t.Errorf("err = %v, want ErrETFSchema", err)
	}
}

func TestTWSEETFProvider_FetchLatest_UpstreamDominates(t *testing.T) {
	// 7 天內混雜：某天 403（hard error）優先於其他天 no-data 回傳，
	// 不能讓 hard error 被 7 天 fallback 吞掉。
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"FAIL","date":"","tables":[]}`))
	}))
	t.Cleanup(ts.Close)
	p := newETFTester(t, ts)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrETFUpstream) {
		t.Errorf("err = %v, want ErrETFUpstream (hard error must dominate)", err)
	}
	_ = time.Now
}
