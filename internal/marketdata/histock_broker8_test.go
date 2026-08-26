package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// histockFixtureHTML mimics the server-rendered structure of
// https://histock.tw/stock/broker8.aspx: two <ul class="stock-list"> blocks
// (buy-side Top N, sell-side Top N), each row an <li> with onclick='goUrl("SYM")'
// and ordered <span> cells: [rank, name, 8 bank amounts, total] in 萬元.
const histockFixtureHTML = `<html><body>
<div class="grid-body p7 mb10">
<table class="gvTB">
<tr><th>★</th><th>股票</th><th>合庫</th><th>土銀</th><th>台銀</th><th>台企銀</th><th>彰銀</th><th>第一金</th><th>兆豐銀</th><th>華南永昌</th><th>合計(萬)</th></tr>
</table>
<ul class="stock-list"><li class="alt-row" onclick='goUrl("2308");' title="2026-08-25 八大公股行庫合計買超 Top.1 : 台達電 (2308) 合計：81,585萬元"><span class="w20"><span class="top">&nbsp;</span></span><span class="w100 name">&nbsp;台達電</span><span class="w70">7,615</span><span class="w70">1,739</span><span class="w70">6,532</span><span class="w70">1,394</span><span class="w70">17,285</span><span class="w70">16,977</span><span class="w70">16,237</span><span class="w70">13,806</span><span class="w70">81,585</span></li><li onclick='goUrl("2317");' title="2026-08-25 八大公股行庫合計買超 Top.2 : 鴻海 (2317) 合計：58,030萬元"><span class="w20"></span><span class="w100 name">&nbsp;鴻海</span><span class="w70">6,153</span><span class="w70">1,287</span><span class="w70">4,133</span><span class="w70">2,487</span><span class="w70">760</span><span class="w70">14,474</span><span class="w70">13,767</span><span class="w70">14,969</span><span class="w70">58,030</span></li></ul>
<ul class="stock-list"><li onclick='goUrl("2454");' title="2026-08-25 八大公股行庫合計賣超 Top.1 : 聯發科 (2454) 合計：-142,202萬元"><span class="w20"></span><span class="w100 name">&nbsp;聯發科</span><span class="w70">-30,000</span><span class="w70">-10,202</span><span class="w70">-20,000</span><span class="w70">-12,000</span><span class="w70">-18,000</span><span class="w70">-22,000</span><span class="w70">-15,000</span><span class="w70">-15,000</span><span class="w70">-142,202</span></li></ul>
</div>
</body></html>`

func TestParseHistockBroker8HTML(t *testing.T) {
	rows, err := ParseHistockBroker8HTML([]byte(histockFixtureHTML))
	if err != nil {
		t.Fatalf("ParseHistockBroker8HTML() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows len = %d, want 3 (2 buy + 1 sell)", len(rows))
	}

	first := rows[0]
	if first.Symbol != "2308" || first.Name != "台達電" {
		t.Errorf("row0 = %s/%s, want 2308/台達電", first.Symbol, first.Name)
	}
	if first.TotalWan != 81585 {
		t.Errorf("row0 TotalWan = %d, want 81585", first.TotalWan)
	}
	if got := first.Banks["合庫"]; got != 7615 {
		t.Errorf("row0 合庫 = %d, want 7615", got)
	}
	if got := first.Banks["華南永昌"]; got != 13806 {
		t.Errorf("row0 華南永昌 = %d, want 13806", got)
	}

	sell := rows[2]
	if sell.Symbol != "2454" {
		t.Errorf("sell row symbol = %s, want 2454", sell.Symbol)
	}
	if sell.TotalWan != -142202 {
		t.Errorf("sell TotalWan = %d, want -142202 (negative for sell)", sell.TotalWan)
	}
}

func TestParseHistockBroker8HTML_EmptyIsNoData(t *testing.T) {
	// Holiday / not-yet-published page renders the header but no rows.
	empty := `<html><body><ul class="stock-list"></ul><ul class="stock-list"></ul></body></html>`
	rows, err := ParseHistockBroker8HTML([]byte(empty))
	if err != nil {
		t.Fatalf("ParseHistockBroker8HTML(empty) error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows len = %d, want 0", len(rows))
	}
}

func newStubHistockServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

func TestHistockBroker8Provider_FetchDaily(t *testing.T) {
	server := newStubHistockServer(t, histockFixtureHTML)
	defer server.Close()

	p := NewHistockBroker8Provider()
	p.SetHTTPClient(server.Client())
	p.SetBaseURL(server.URL)

	date := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	rows, err := p.FetchDaily(context.Background(), date)
	if err != nil {
		t.Fatalf("FetchDaily() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows len = %d, want 3", len(rows))
	}
}

func TestHistockBroker8Provider_FetchDaily_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := NewHistockBroker8Provider()
	p.SetHTTPClient(server.Client())
	p.SetBaseURL(server.URL)

	_, err := p.FetchDaily(context.Background(), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("FetchDaily() on HTTP 503 = nil error, want error")
	}
}

func TestHistockBankCodes_CoverEightBanks(t *testing.T) {
	// Every histock column name must map to one of the 8 core bank codes.
	if len(histockBankCodes) != 8 {
		t.Fatalf("histockBankCodes len = %d, want 8", len(histockBankCodes))
	}
	for name, code := range histockBankCodes {
		want, ok := coreBankBranches[code]
		if !ok {
			t.Errorf("histockBankCodes[%q] code %q not in coreBankBranches", name, code)
			continue
		}
		if want != nameToExpectedDisplayName[name] {
			continue // display names differ (short vs full); only codes matter here
		}
	}
}

// nameToExpectedDisplayName documents the histock short-name → full-name pairing.
var nameToExpectedDisplayName = map[string]string{
	"合庫":   "合作金庫",
	"土銀":   "土地銀行",
	"台銀":   "臺灣銀行",
	"台企銀":  "臺灣企銀",
	"彰銀":   "彰化銀行",
	"第一金":  "第一金證券",
	"兆豐銀":  "兆豐證券",
	"華南永昌": "華南永昌證券",
}

func TestAggregateDate_HistockSource_WritesReadingAndDetails(t *testing.T) {
	dir := t.TempDir()
	server := newStubHistockServer(t, histockFixtureHTML)
	defer server.Close()

	agg := NewGovernmentBrokerAggregator(dir)
	agg.SetHTTPClient(server.Client())
	agg.Histock().SetBaseURL(server.URL)

	date := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	reading, err := agg.AggregateDate(context.Background(), date)
	if err != nil {
		t.Fatalf("AggregateDate() error = %v", err)
	}
	// total = (81585 + 58030 - 142202) 萬 × 10000 = -2587 × 10000 TWD.
	wantTotal := (81585 + 58030 - 142202) * 10000
	if reading.TotalNet != int64(wantTotal) {
		t.Errorf("TotalNet = %d, want %d", reading.TotalNet, wantTotal)
	}
	if reading.Source != "media-curated" {
		t.Errorf("Source = %q, want media-curated", reading.Source)
	}

	if _, err := os.Stat(filepath.Join(dir, "20260825.json")); err != nil {
		t.Errorf("reading file missing: %v", err)
	}

	detailBytes, err := os.ReadFile(filepath.Join(dir, "20260825_brokers.json"))
	if err != nil {
		t.Fatalf("brokers detail file missing: %v", err)
	}
	var detail struct {
		Brokers []BrokerDailyDetail `json:"brokers"`
	}
	mustUnmarshal(t, detailBytes, &detail)
	if len(detail.Brokers) != 8 {
		t.Fatalf("brokers detail rows = %d, want 8 banks", len(detail.Brokers))
	}
	var sumNet int64
	for _, b := range detail.Brokers {
		sumNet += b.Net
	}
	if sumNet != int64(wantTotal) {
		t.Errorf("sum of per-bank Net = %d, want %d", sumNet, wantTotal)
	}
}

func TestAggregateDate_HistockSource_EmptyIsNoData(t *testing.T) {
	dir := t.TempDir()
	server := newStubHistockServer(t, `<html><body><ul class="stock-list"></ul></body></html>`)
	defer server.Close()

	agg := NewGovernmentBrokerAggregator(dir)
	agg.SetHTTPClient(server.Client())
	agg.Histock().SetBaseURL(server.URL)

	reading, err := agg.AggregateDate(context.Background(), time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AggregateDate() holiday = error %v, want (nil, nil)", err)
	}
	if reading != nil {
		t.Errorf("holiday reading = %+v, want nil", reading)
	}
}

func mustUnmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
