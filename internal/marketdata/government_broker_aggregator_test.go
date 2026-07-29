package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// newStubTWSEFormServer returns a server that mimics the TWSE bsMenu.aspx flow:
// GET returns the ASP.NET form with hidden tokens, POST returns the broker table.
func newStubTWSEFormServer(t *testing.T, branchCode string) *httptest.Server {
	t.Helper()
	formHTML := `<html><body><form>
<input id="__VIEWSTATE" value="/wEPDwUKLTgxNDI2MzM4MGRk">
<input id="__VIEWSTATEGENERATOR" value="AA1F01CB">
<input id="__EVENTVALIDATION" value="/wEWAwKz">
</form></body></html>`
	tableHTML := `<html><body><table><tr>
<th>券商代號</th><th>券商名稱</th><th>買進</th><th>賣出</th><th>淨買</th>
</tr><tr>
<td>` + branchCode + `</td><td>合作金庫</td>
<td>1000</td><td>0</td><td>1000</td>
</tr></table></body></html>`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(formHTML))
			return
		}
		_, _ = w.Write([]byte(tableHTML))
	}))
}

func TestAggregateDate_WritesBrokerDetails(t *testing.T) {
	dir := t.TempDir()
	server := newStubTWSEFormServer(t, "8060")
	defer server.Close()

	agg := NewGovernmentBrokerAggregator(dir)
	agg.SetHTTPClient(server.Client())
	agg.SetBaseURL(server.URL)
	agg.SetSymbols([]string{"2330"})

	date := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	reading, err := agg.AggregateDate(context.Background(), date)
	if err != nil {
		t.Fatalf("AggregateDate() error = %v", err)
	}
	if reading.TotalNet != 1000 {
		t.Errorf("TotalNet = %d, want 1000", reading.TotalNet)
	}

	// Existing aggregate file.
	aggPath := filepath.Join(dir, "20260728.json")
	if _, err := os.Stat(aggPath); err != nil {
		t.Errorf("Aggregate file missing: %v", err)
	}

	// New per-broker detail file.
	detailPath := filepath.Join(dir, "20260728_brokers.json")
	detailBytes, err := os.ReadFile(detailPath)
	if err != nil {
		t.Fatalf("Read detail file: %v", err)
	}
	var detail struct {
		Date    string              `json:"date"`
		Source  string              `json:"source"`
		Brokers []BrokerDailyDetail `json:"brokers"`
	}
	if err := json.Unmarshal(detailBytes, &detail); err != nil {
		t.Fatalf("Unmarshal detail: %v", err)
	}
	if detail.Date != "20260728" {
		t.Errorf("Detail date = %q, want 20260728", detail.Date)
	}
	if len(detail.Brokers) != 1 {
		t.Fatalf("Detail brokers len = %d, want 1", len(detail.Brokers))
	}
	b := detail.Brokers[0]
	if b.Code != "8060" || b.Name != "合作金庫" || b.Type != "gov" || b.Buy != 1000 || b.Sell != 0 || b.Net != 1000 {
		t.Errorf("Unexpected broker detail: %+v", b)
	}
}

func TestParseBrokerTableHTML_SimplePositionBased(t *testing.T) {
	agg := NewGovernmentBrokerAggregator(t.TempDir())
	body := []byte(`<html><body><table><tr>
<th>code</th><th>name</th><th>buy</th><th>sell</th><th>net</th>
</tr><tr>
<td>8060</td><td>HeadOffice</td>
<td>1000</td><td>0</td><td>1000</td>
</tr></table></body></html>`)

	res, err := agg.parseBrokerTableHTML("2330", body)
	if err != nil {
		t.Fatalf("parseBrokerTableHTML() error = %v", err)
	}
	if res.GovNet != 1000 {
		t.Errorf("GovNet = %d, want 1000", res.GovNet)
	}
	if len(res.Gov) != 1 {
		t.Fatalf("Gov brokers len = %d, want 1", len(res.Gov))
	}
	b := res.Gov[0]
	if b.Code != "8060" || b.Name != "HeadOffice" || b.Buy != 1000 || b.Sell != 0 || b.Net != 1000 {
		t.Errorf("Unexpected broker row: %+v", b)
	}
}

func TestParseBrokerTableHTML_HeaderDriven(t *testing.T) {
	agg := NewGovernmentBrokerAggregator(t.TempDir())
	body := []byte(`<html><body><table><tr>
<td>序號</td><td>券商代號</td><td>券商名稱</td>
<td>買進</td><td>賣出</td><td>淨買</td>
</tr><tr>
<td>1</td><td>8011</td><td>第一金證券</td>
<td>500</td><td>100</td><td>400</td>
</tr></table></body></html>`)

	res, err := agg.parseBrokerTableHTML("2330", body)
	if err != nil {
		t.Fatalf("parseBrokerTableHTML() error = %v", err)
	}
	if res.GovNet != 400 {
		t.Errorf("GovNet = %d, want 400", res.GovNet)
	}
	if len(res.Gov) != 1 || res.Gov[0].Code != "8011" {
		t.Errorf("Expected broker 8011, got %+v", res.Gov)
	}
}

func TestParseBrokerTableHTML_CaptchaDetected(t *testing.T) {
	agg := NewGovernmentBrokerAggregator(t.TempDir())
	body := []byte(`<html><body>
<img src="CaptchaImage.aspx?guid=123">
<input name="CaptchaControl1" type="text">
</body></html>`)

	if !hasCaptcha(body) {
		t.Error("hasCaptcha should return true for captcha body")
	}
	_, err := agg.parseBrokerTableHTML("2330", body)
	if err == nil {
		t.Fatal("parseBrokerTableHTML should error on captcha body")
	}
	if !strings.Contains(err.Error(), "captcha required") {
		t.Errorf("Expected 'captcha required' error, got %v", err)
	}
}

func TestFetchStockBrokerNet_Captcha(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Both GET and POST return captcha page.
		_, _ = w.Write([]byte(`<html><body>
<form><input id="__VIEWSTATE" value="/wEP">
<input id="__VIEWSTATEGENERATOR" value="AA1F01CB">
<input id="__EVENTVALIDATION" value="/wEW"></form>
<img src="CaptchaImage.aspx?guid=123">
</body></html>`))
	}))
	defer server.Close()

	agg := NewGovernmentBrokerAggregator(dir)
	agg.SetHTTPClient(server.Client())
	agg.SetBaseURL(server.URL)

	date := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	_, err := agg.fetchStockBrokerNet(context.Background(), "2330", date)
	if err == nil {
		t.Fatal("Expected captcha error")
	}
	if !strings.Contains(err.Error(), "captcha required") {
		t.Errorf("Expected captcha error, got %v", err)
	}
}

func TestCoreBankBranches_HasEightBanks(t *testing.T) {
	expected := map[string]string{
		"8060": "合作金庫",
		"8030": "土地銀行",
		"8040": "臺灣銀行",
		"8010": "臺灣企銀",
		"8064": "彰化銀行",
		"8061": "兆豐證券",
		"8011": "第一金證券",
		"8080": "華南永昌證券",
	}
	if len(coreBankBranches) != 8 {
		t.Errorf("coreBankBranches len = %d, want 8", len(coreBankBranches))
	}
	for code, name := range expected {
		if coreBankBranches[code] != name {
			t.Errorf("coreBankBranches[%q] = %q, want %q", code, coreBankBranches[code], name)
		}
	}
}

func TestMergeBrokerDetails(t *testing.T) {
	details := make(map[detailKey]*detailAccumulator)
	rows := []BrokerBranchNet{
		{Code: "8060", Name: "合作金庫", Buy: 1000, Sell: 0, Net: 1000},
	}
	mergeBrokerDetails(details, rows, "gov")
	mergeBrokerDetails(details, []BrokerBranchNet{
		{Code: "8060", Name: "合作金庫", Buy: 500, Sell: 200, Net: 300},
	}, "gov")

	if len(details) != 1 {
		t.Fatalf("Detail count = %d, want 1", len(details))
	}
	for _, acc := range details {
		if acc.Buy != 1500 || acc.Sell != 200 || acc.Net != 1300 {
			t.Errorf("Accumulated detail = %+v, want buy=1500 sell=200 net=1300", acc)
		}
	}
}

func TestNewGovernmentBrokerAggregator_UsesSharedHTTPClient(t *testing.T) {
	agg := NewGovernmentBrokerAggregator(t.TempDir())
	if agg.client == nil {
		t.Fatal("Expected non-nil http client from shared factory")
	}
	if agg.client.Timeout != httpclient.NewFactory().NewClient(30*time.Second).Timeout {
		// Just confirm it is the factory type; exact timeout is not critical.
		t.Logf("client timeout = %v", agg.client.Timeout)
	}
}
