package marketdata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── helpers ───

// fubonPcfPage 產生一份與富邦官網同結構的 PCF HTML 測試樣本。
// 結構：<li><p>標籤</p><p>數值</p></li>，header 有日期文字節點。
func fubonPcfPage(date, diff, nav, fundNAV string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>申購買回清單 | 富邦投信ETF投資網</title></head>
<body>
<div class="w3-row mb20">
  <ul class="w3-row mb20">
    <li class="w3-col s10"><h6 class="top mt0 mb0">現金申購買回清單</h6></li>
    <li class="w3-col s2 f14 txt_black_777 tar">%s</li>
  </ul>
</div>
<div class="fund_big_box mb40"><div class="fund_box_2 p4 w100 boxstyle1"><ul>
  <li><p>基金淨資產價值</p><p>NT$%s</p></li>
  <li><p>已發行受益權單位總數</p><p>1,000,000</p></li>
  <li><p>與前日已發行單位差異數</p><p>%s</p></li>
  <li><p>每受益權單位淨資產價值</p><p>NT$%s</p></li>
  <li><p>每現金申購/買回基數之受益權單位數</p><p>500,000</p></li>
</ul></div></div>
</body></html>`, date, fundNAV, diff, nav)
}

// newFubonTester 建立連到 mock server 的 provider，繞過 rate limiter pacing
// 與 cache（每個 test case 用全新實例，無共享狀態）。
func newFubonTester(t *testing.T, ts *httptest.Server, symbols []string) *FubonETFProvider {
	t.Helper()
	p := NewFubonETFProvider()
	p.SetHTTPClient(ts.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.SetCacheTTL(0) // 測試關閉 cache，避免跨 case 污染
	if len(symbols) > 0 {
		p.SetSymbols(symbols)
	}
	p.baseURL = ts.URL // 同 package 測試直接覆寫 baseURL 指向 mock server
	return p
}

// fubonSymbolServer 建立依 symbol 回傳不同 body 的 mock server。
func fubonSymbolServer(t *testing.T, pages map[string]string, statusFor string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sym := r.URL.Query().Get("stkId")
		if statusFor != "" && sym == statusFor {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, ok := pages[sym]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ─── parseFubonPcf ───

func TestParseFubonPcf_Success(t *testing.T) {
	// 真實格式：差異數帶逗號與負號、NAV 帶 NT$ 前綴、日期在 header。
	body := fubonPcfPage("2026/08/17", "-1,000,000", "18.97", "29,973,544,281")
	fields, err := parseFubonPcf([]byte(body))
	if err != nil {
		t.Fatalf("parseFubonPcf error: %v", err)
	}
	if fields.diff != -1_000_000 {
		t.Errorf("diff = %d, want -1000000", fields.diff)
	}
	if fields.nav != 18.97 {
		t.Errorf("nav = %v, want 18.97", fields.nav)
	}
	if fields.fund != 29_973_544_281 {
		t.Errorf("fund = %d, want 29973544281", fields.fund)
	}
	if fields.date != "20260817" {
		t.Errorf("date = %q, want 20260817", fields.date)
	}
}

func TestParseFubonPcf_ZeroDiffIsValid(t *testing.T) {
	// 006208 實測差異數 = 0：0 是合法值，不能視為 schema 失敗。
	body := fubonPcfPage("2026/08/17", "0", "243.58", "453,069,123,242")
	fields, err := parseFubonPcf([]byte(body))
	if err != nil {
		t.Fatalf("parseFubonPcf error: %v", err)
	}
	if !fields.diffOK {
		t.Error("diffOK = false, want true (0 is a valid difference)")
	}
	if fields.diff != 0 {
		t.Errorf("diff = %d, want 0", fields.diff)
	}
}

func TestParseFubonPcf_MissingFields(t *testing.T) {
	// 頁面缺「與前日已發行單位差異數」（schema 變更/WAF 頁）→ error。
	body := strings.ReplaceAll(fubonPcfPage("2026/08/17", "-1,000,000", "18.97", "29,973,544,281"),
		"與前日已發行單位差異數", "某個新欄位")
	if _, err := parseFubonPcf([]byte(body)); err == nil {
		t.Fatal("expected schema error for missing 差異數 label")
	}
}

func TestParseFubonPcf_NonHTML(t *testing.T) {
	// WAF/錯誤頁（非 HTML）→ error。
	if _, err := parseFubonPcf([]byte("<html>Forbidden</html>")); err == nil {
		t.Fatal("expected error for non-PCF HTML")
	}
}

// ─── FetchETFNetSubscription ───

func TestFetchETFNetSubscription_Success(t *testing.T) {
	// 兩支：一支淨贖回 -1,000,000 單位 × NAV 18.97，一支 0 單位。
	pages := map[string]string{
		"00900":  fubonPcfPage("2026/08/17", "-1,000,000", "18.97", "29,973,544,281"),
		"006208": fubonPcfPage("2026/08/17", "0", "243.58", "453,069,123,242"),
	}
	ts := fubonSymbolServer(t, pages, "")
	p := newFubonTester(t, ts, []string{"00900", "006208"})

	stats, err := p.FetchETFNetSubscription(context.Background())
	if err != nil {
		t.Fatalf("FetchETFNetSubscription error: %v", err)
	}
	// NetSubscription = -1,000,000×18.97 + 0 = -18,970,000（TWD 加權）
	if stats.NetSubscription != -18_970_000 {
		t.Errorf("NetSubscription = %d, want -18970000", stats.NetSubscription)
	}
	// TotalNAV = 兩個基金規模加總
	if stats.TotalNAV != 29_973_544_281+453_069_123_242 {
		t.Errorf("TotalNAV = %d, want %d", stats.TotalNAV, 29_973_544_281+453_069_123_242)
	}
	if stats.Date != "20260817" {
		t.Errorf("Date = %q, want 20260817", stats.Date)
	}
}

func TestFetchETFNetSubscription_BestEffort(t *testing.T) {
	// 一支 HTTP 500、一支正常 → 正常那支的資料仍回傳（單支失敗不拖垮全部）。
	pages := map[string]string{
		"00900": fubonPcfPage("2026/08/17", "-1,000,000", "18.97", "29,973,544,281"),
	}
	ts := fubonSymbolServer(t, pages, "006208")
	p := newFubonTester(t, ts, []string{"006208", "00900"})

	stats, err := p.FetchETFNetSubscription(context.Background())
	if err != nil {
		t.Fatalf("FetchETFNetSubscription error: %v", err)
	}
	if stats.NetSubscription != -18_970_000 {
		t.Errorf("NetSubscription = %d, want -18970000 (best-effort must keep the healthy symbol)", stats.NetSubscription)
	}
}

func TestFetchETFNetSubscription_AllFail(t *testing.T) {
	ts := fubonSymbolServer(t, map[string]string{}, "")
	p := newFubonTester(t, ts, []string{"00900", "006208"}) // 兩支都 404

	_, err := p.FetchETFNetSubscription(context.Background())
	if err == nil {
		t.Fatal("expected error when all symbols fail")
	}
	if !errors.Is(err, ErrFubonETFUpstream) && !errors.Is(err, ErrFubonETFNoData) {
		t.Errorf("err = %v, want ErrFubonETFUpstream or ErrFubonETFNoData", err)
	}
}

func TestFetchETFNetSubscription_Non200(t *testing.T) {
	// 上游 5xx → typed ErrFubonETFUpstream（需能觸發 circuit breaker 語意）。
	ts := fubonSymbolServer(t, map[string]string{}, "00900")
	p := newFubonTester(t, ts, []string{"00900"})

	_, err := p.FetchETFNetSubscription(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrFubonETFUpstream) {
		t.Errorf("err = %v, want ErrFubonETFUpstream", err)
	}
}

func TestFetchETFNetSubscription_CacheHit(t *testing.T) {
	// TTL cache：第二次呼叫不打 HTTP（server 只被 hit 一次）。
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(fubonPcfPage("2026/08/17", "-1,000,000", "18.97", "29,973,544,281")))
	}))
	t.Cleanup(ts.Close)

	p := newFubonTester(t, ts, []string{"00900"})
	p.SetCacheTTL(DefaultFubonETFCacheTTL) // 測試 cache 行為時重新啟用

	for range 3 {
		if _, err := p.FetchETFNetSubscription(context.Background()); err != nil {
			t.Fatalf("Fetch error: %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("HTTP hits = %d, want 1 (TTL cache must serve repeat calls)", got)
	}
}

func TestFetchETFNetSubscription_RespectsContext(t *testing.T) {
	// ctx 已取消 → 不應 hang，直接回傳 ctx 錯誤。
	ts := fubonSymbolServer(t, map[string]string{"00900": fubonPcfPage("2026/08/17", "0", "18.97", "1")}, "")
	p := newFubonTester(t, ts, []string{"00900"})
	p.SetRateLimiter(rate.NewLimiter(rate.Every(time.Hour), 1)) // 讓 rate limiter 立刻封鎖

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	time.Sleep(30 * time.Millisecond) // 確保 timeout 已觸發

	_, err := p.FetchETFNetSubscription(ctx)
	if err == nil {
		t.Fatal("expected error for expired context")
	}
}
