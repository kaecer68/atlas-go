package marketdata

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// GovernmentBrokerAggregator fetches daily broker-level trading data from TWSE
// (bsr.twse.com.tw) and aggregates net buy/sell for the 8 core government banks'
// head offices, writing the result to the GovernmentFlowProvider's flat directory.
//
// Methodology (per docs/specs/government-force-proxy-spec.md and community practice):
//   - 8 core banks: 合庫(8060), 土銀(8030), 臺灣銀(8040), 台企銀(8010), 彰化(8064),
//     兆豐(8061), 第一金(8011), 華南永昌(8080)
//   - Head office branch codes only (not all branches)
//   - Aggregated across the top N weighted stocks (TW50 constituents)
//   - Source: TWSE bsr.twse.com.tw — Open Data, publicly accessible
//
// Note: bsr.twse.com.tw added a CAPTCHA to the query flow. The aggregator now
// posts through the ASP.NET form and reports a clear error when the CAPTCHA is
// presented, instead of silently writing a zero reading.
type GovernmentBrokerAggregator struct {
	client    *http.Client
	limiter   *rate.Limiter
	outputDir string
	baseURL   string
	symbols   []string
	// cooldown 是 per-channel CAPTCHA backoff（P0-2）：CAPTCHA 被偵測後
	// 24h 內不再打 bsr.twse.com.tw，避免重複觸發 upstream block。
	// 死碼修復 — captcha_cooldown.go 的元件此前從未被接線。
	cooldown *CaptchaCooldown
	// breaker is the aggregator-level circuit breaker (P1-7). The "no
	// stocks processed" (nil,nil) result — a holiday/no-data condition —
	// does NOT trip it; transport/parse/write failures do.
	breaker *providerBreaker
}

// GovernmentBrokerChannelID is the channel identifier used for the
// CaptchaCooldown key. It matches the adapter Metadata().ChannelID so the
// adapter and aggregator share one cooldown state.
const GovernmentBrokerChannelID = "government_broker"

// coreBankBranches maps the 8 government-controlled banks to their TWSE head-office
// branch codes per docs/specs/government-force-proxy-spec.md. Definition updated
// from 5 to 8 banks in this PR; see commit message for the mapping rationale.
var coreBankBranches = map[string]string{
	"8060": "合作金庫",
	"8030": "土地銀行",
	"8040": "臺灣銀行",
	"8010": "臺灣企銀",
	"8064": "彰化銀行",
	"8061": "兆豐證券",
	"8011": "第一金證券",
	"8080": "華南永昌證券",
}

// insuranceBrokerCodes maps major Taiwan life insurance companies' affiliated
// securities firms (used as proxy for insurance capital flow).
// Note: Insurance companies trade through their securities arms or dedicated
// brokers; these codes represent the primary trading desks.
var insuranceBrokerCodes = map[string]string{
	"8880": "國泰證券(國泰人壽)",
	"9600": "富邦證券(富邦人壽)",
	"8560": "新光證券(新光人壽)",
	"8840": "凱基證券(中國人壽/凱基人壽)",
	"9200": "群益證券(南山人壽主要券商)",
}

// tw50Symbols is the list of TWSE Taiwan 50 constituent stock symbols
// whose broker data is aggregated for the government flow proxy.
var tw50Symbols = []string{
	"2330", "2317", "2454", "2308", "2382", "2303", "2881", "2882",
	"2891", "2886", "2885", "2884", "2892", "5880", "3711", "3034",
	"3008", "2880", "1301", "1303", "1326", "2002", "2207", "2912",
	"2412", "3045", "4904", "1216", "1101", "1102", "6505", "2603",
	"2615", "2609", "2610", "5871", "5876", "2883", "2801", "2887",
	"2890", "2357", "2327", "3231", "2379", "2383", "2345", "3037",
	"3443", "5269",
}

// BrokerBranchNet records a single broker branch's daily trading on one stock.
type BrokerBranchNet struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Buy  int64  `json:"buy"`
	Sell int64  `json:"sell"`
	Net  int64  `json:"net"`
}

// BrokerDailyDetail accumulates a broker's totals across all queried stocks
// for one trading day. It is the row shape written to YYYYMMDD_brokers.json.
type BrokerDailyDetail struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type"` // "gov" or "ins"
	Buy  int64  `json:"buy"`
	Sell int64  `json:"sell"`
	Net  int64  `json:"net"`
}

// GovernmentBrokerDailyResult is the parsed outcome for one stock on one day.
type GovernmentBrokerDailyResult struct {
	GovNet int64
	InsNet int64
	Gov    []BrokerBranchNet
	Ins    []BrokerBranchNet
}

// ErrCaptchaRequired (fix/20260731-govflow-cadence) is the typed sentinel
// returned when the upstream TWSE bsr endpoint serves a CAPTCHA page. It
// is wrapped by every "captcha required" error path inside this file and
// is detected by marketdata.CaptchaCooldown.IsCaptchaErr to drive the
// per-channel cooldown timer.
//
// Use errors.Is(err, marketdata.ErrCaptchaRequired) at call sites that
// need a stable signal — string-matching the message is brittle across
// refactors and is exactly the failure mode that motivated this typed
// sentinel.
var ErrCaptchaRequired = errors.New("captcha required for government broker")

// detailKey is the merge key for aggregating broker details across stocks.
type detailKey struct {
	Code string
	Name string
	Type string
}

// detailAccumulator is used internally by AggregateDate to merge per-stock
// broker rows into a single daily file.
type detailAccumulator struct {
	Code string
	Name string
	Type string
	Buy  int64
	Sell int64
	Net  int64
}

// NewGovernmentBrokerAggregator creates an aggregator that writes to outputDir.
// Uses the shared httpclient factory (C06: replaces raw &http.Client{} to
// remove the last direct HTTP client creation outside Gateway/ProviderRegistry).
func NewGovernmentBrokerAggregator(outputDir string) *GovernmentBrokerAggregator {
	return &GovernmentBrokerAggregator{
		client:    httpclient.NewFactory().NewClient(30 * time.Second),
		limiter:   rate.NewLimiter(rate.Every(2*time.Second), 1),
		outputDir: outputDir,
		baseURL:   "https://bsr.twse.com.tw/bshtm",
		symbols:   tw50Symbols,
		cooldown:  NewCaptchaCooldown(),
		breaker:   newProviderBreaker("government_broker", defaultCircuitBreakerConfig()),
	}
}

// SetHTTPClient overrides the HTTP client (tests only).
func (a *GovernmentBrokerAggregator) SetHTTPClient(client *http.Client) {
	a.client = client
}

// SetBaseURL overrides the TWSE base URL (tests only).
func (a *GovernmentBrokerAggregator) SetBaseURL(baseURL string) {
	a.baseURL = baseURL
}

// SetSymbols overrides the symbol list (tests only).
func (a *GovernmentBrokerAggregator) SetSymbols(symbols []string) {
	a.symbols = symbols
}

// SetCaptchaCooldown overrides the CAPTCHA backoff state (tests only; pass
// CaptchaCooldownWith(d, fakeClock) for deterministic cooldown tests).
func (a *GovernmentBrokerAggregator) SetCaptchaCooldown(cd *CaptchaCooldown) {
	a.cooldown = cd
}

// CaptchaCooldown returns the aggregator's CAPTCHA backoff state so the
// channel adapter can RecordCaptcha when an ErrCaptchaRequired surfaces
// outside AggregateDate (defensive belt-and-suspenders — AggregateDate
// already records internally). May be nil for hand-constructed aggregators.
func (a *GovernmentBrokerAggregator) CaptchaCooldown() *CaptchaCooldown {
	return a.cooldown
}

// breakerRecordSuccess / breakerRecordFailure are nil-safe breaker wrappers.
func (a *GovernmentBrokerAggregator) breakerRecordSuccess() {
	if a.breaker != nil {
		a.breaker.recordSuccess()
	}
}

func (a *GovernmentBrokerAggregator) breakerRecordFailure() {
	if a.breaker != nil {
		a.breaker.recordFailure()
	}
}

// BreakerInfo exposes the breaker state for tests and observability.
func (a *GovernmentBrokerAggregator) BreakerInfo() ProviderBreakerInfo {
	if a.breaker == nil {
		return ProviderBreakerInfo{Name: "government_broker", State: ProviderCircuitClosed}
	}
	return a.breaker.stateSnapshot()
}

// AggregateDate fetches broker data for the given trading date, aggregates net
// buy/sell for the 8 core government banks across TW50 stocks, and writes the
// result as both a GovernmentFlowReading JSON file and a per-broker detail file.
func (a *GovernmentBrokerAggregator) AggregateDate(ctx context.Context, date time.Time) (*GovernmentFlowReading, error) {
	dateStr := date.Format("20060102")

	// P1-7: aggregator-level breaker — open 時不發 50-symbol 的請求串。
	if a.breaker != nil && !a.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: government_broker circuit breaker open", ErrUpstream)
	}

	// P0-2: CAPTCHA cooldown gate — a recent CAPTCHA response means the
	// upstream has blocked us; skip the whole run instead of hammering all
	// 50 symbols. Same nil,nil contract as the "no stocks processed" path
	// so the adapter surfaces a no_data stub (not a channel error).
	if a.cooldown != nil && a.cooldown.ShouldSkip(GovernmentBrokerChannelID) {
		return nil, nil
	}

	var totalGovNet, totalInsNet int64
	var stocksProcessed int
	var runFailed bool // any per-symbol transport/parse failure this run
	details := make(map[detailKey]*detailAccumulator)

	for _, symbol := range a.symbols {
		if err := a.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit: %w", err)
		}

		res, err := a.fetchStockBrokerNet(ctx, symbol, date)
		if err != nil {
			runFailed = true
			// P0-2: CAPTCHA means the upstream is actively blocking us —
			// record the cooldown and STOP (previously the loop continued
			// into the remaining ~49 symbols, re-triggering the block).
			// P1-7: CAPTCHA/upstream failure counts against the breaker.
			if a.cooldown != nil && errors.Is(err, ErrCaptchaRequired) {
				a.cooldown.RecordCaptcha(GovernmentBrokerChannelID)
				a.breakerRecordFailure()
				break
			}
			a.breakerRecordFailure()
			continue
		}
		if a.cooldown != nil {
			// Any successful non-CAPTCHA fetch clears a stale cooldown so a
			// future CAPTCHA starts a fresh window (captcha_cooldown.go doc).
			a.cooldown.RecordSuccess(GovernmentBrokerChannelID)
		}
		totalGovNet += res.GovNet
		totalInsNet += res.InsNet
		stocksProcessed++

		mergeBrokerDetails(details, res.Gov, "gov")
		mergeBrokerDetails(details, res.Ins, "ins")
	}

	// No stocks processed for this date is not an error: it can happen on
	// non-trading days (Taiwanese national holidays), after-hours runs, or
	// when the upstream TWSE broker page is temporarily unavailable but
	// didn't return a hard error. We return (nil, nil) so the caller can
	// distinguish "no data" from "real failure" and avoid surfacing a false
	// channel error on the dashboard (regression: 2026-08-03 channel-health
	// "no stocks processed for 20260803" — was a holiday, not a fault).
	if stocksProcessed == 0 {
		if runFailed {
			// Every symbol failed at the transport/parse layer — this is an
			// upstream outage, NOT a quiet holiday. Keep the breaker failure
			// count (do NOT reset it via recordSuccess) so the breaker can
			// open and gate the next run, and surface the failure to the
			// caller instead of a misleading no_data stub (k3 audit 2026-08-24:
			// the previous unconditional recordSuccess masked the outage).
			return nil, fmt.Errorf("%w: government_broker fetch failed for all %d symbols", ErrUpstream, len(a.symbols))
		}
		// True no-data (holiday / upstream empty page): expected outcome, not
		// a breaker failure. Leave counts untouched (no-op) so prior real
		// failures are not masked by a quiet day.
		return nil, nil
	}
	// Write government bank reading (existing format).
	govReading := &GovernmentFlowReading{
		Date:     dateStr,
		TotalNet: totalGovNet,
		Source:   "broker-aggregate",
		RawURL:   "https://bsr.twse.com.tw/bshtm/bsMenu.aspx",
	}
	if err := a.writeReading(*govReading); err != nil {
		a.breakerRecordFailure()
		return nil, fmt.Errorf("government_broker write: %w", err)
	}

	// Write insurance company reading (existing format).
	insReading := &GovernmentFlowReading{
		Date:     dateStr,
		TotalNet: totalInsNet,
		Source:   "broker-aggregate",
		RawURL:   "https://bsr.twse.com.tw/bshtm/bsMenu.aspx",
	}
	if err := a.writeInsuranceReading(*insReading); err != nil {
		a.breakerRecordFailure()
		return nil, fmt.Errorf("insurance_broker write: %w", err)
	}

	// Write per-broker detail file (new: PR-A).
	if err := a.writeBrokerDetails(dateStr, details); err != nil {
		a.breakerRecordFailure()
		return nil, fmt.Errorf("broker_details write: %w", err)
	}

	a.breakerRecordSuccess()
	return govReading, nil
}

// mergeBrokerDetails merges per-stock broker rows into the daily accumulator.
func mergeBrokerDetails(details map[detailKey]*detailAccumulator, rows []BrokerBranchNet, typ string) {
	for _, r := range rows {
		key := detailKey{Code: r.Code, Name: r.Name, Type: typ}
		acc, ok := details[key]
		if !ok {
			acc = &detailAccumulator{Code: r.Code, Name: r.Name, Type: typ}
			details[key] = acc
		}
		acc.Buy += r.Buy
		acc.Sell += r.Sell
		acc.Net += r.Net
	}
}

// fetchStockBrokerNet fetches the broker trading report for a single stock
// and returns both government bank and insurance company details.
func (a *GovernmentBrokerAggregator) fetchStockBrokerNet(ctx context.Context, symbol string, date time.Time) (*GovernmentBrokerDailyResult, error) {
	dateStr := date.Format("20060102")

	menuURL, vs, vg, ev, err := a.fetchMenuTokens(ctx, symbol, dateStr)
	if err != nil {
		return nil, fmt.Errorf("fetch menu tokens %s/%s: %w", symbol, dateStr, err)
	}

	body, err := a.postMenuQuery(ctx, menuURL, vs, vg, ev, symbol)
	if err != nil {
		return nil, fmt.Errorf("post menu query %s/%s: %w", symbol, dateStr, err)
	}

	if hasCaptcha(body) {
		return nil, fmt.Errorf("%w %s/%s", ErrCaptchaRequired, symbol, dateStr)
	}

	res, err := a.parseBrokerTableHTML(symbol, body)
	if err != nil {
		return nil, fmt.Errorf("parse broker table %s/%s: %w", symbol, dateStr, err)
	}
	return res, nil
}

// fetchMenuTokens performs the initial GET to bsMenu.aspx and extracts the
// ASP.NET form tokens needed for the POST query. The date-symbol pair is
// encoded in the query string so the server knows which trading day to serve.
func (a *GovernmentBrokerAggregator) fetchMenuTokens(ctx context.Context, symbol, dateStr string) (string, string, string, string, error) {
	menuURL := fmt.Sprintf("%s/bsMenu.aspx?p=%s_%s", a.baseURL, dateStr, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, menuURL, nil)
	if err != nil {
		return "", "", "", "", fmt.Errorf("request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", "", fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", "", "", "", fmt.Errorf("read: %w", err)
	}

	vs, vg, ev, err := extractFormTokens(body)
	if err != nil {
		return "", "", "", "", err
	}

	return menuURL, vs, vg, ev, nil
}

// postMenuQuery submits the ASP.NET form that triggers the broker table response.
func (a *GovernmentBrokerAggregator) postMenuQuery(ctx context.Context, menuURL, vs, vg, ev, symbol string) ([]byte, error) {
	form := url.Values{}
	form.Set("__VIEWSTATE", vs)
	form.Set("__VIEWSTATEGENERATOR", vg)
	form.Set("__EVENTVALIDATION", ev)
	form.Set("TextBox_Stkno", symbol)
	form.Set("RadioButton_Normal", "RadioButton_Normal")
	form.Set("btnOK", "查詢")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, menuURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("post: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return body, nil
}

// hasCaptcha detects whether the TWSE response is asking for a CAPTCHA.
func hasCaptcha(body []byte) bool {
	return bytes.Contains(body, []byte("CaptchaImage.aspx")) ||
		bytes.Contains(body, []byte("CaptchaControl1"))
}

// DataDir returns the directory where daily readings are written.
func (a *GovernmentBrokerAggregator) DataDir() string {
	return a.outputDir
}

// writeInsuranceReading writes a GovernmentFlowReading to a suffixed file
// (<date>_insurance.json) so GovernmentFlowProvider can distinguish
// insurance company flow from government bank flow.
func (a *GovernmentBrokerAggregator) writeInsuranceReading(r GovernmentFlowReading) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal insurance: %w", err)
	}
	path := filepath.Join(a.outputDir, r.Date+"_insurance.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// writeBrokerDetails writes the daily per-broker detail file.
func (a *GovernmentBrokerAggregator) writeBrokerDetails(dateStr string, details map[detailKey]*detailAccumulator) error {
	rows := make([]BrokerDailyDetail, 0, len(details))
	for _, acc := range details {
		rows = append(rows, BrokerDailyDetail{
			Code: acc.Code,
			Name: acc.Name,
			Type: acc.Type,
			Buy:  acc.Buy,
			Sell: acc.Sell,
			Net:  acc.Net,
		})
	}

	payload := struct {
		Date    string              `json:"date"`
		Source  string              `json:"source"`
		Brokers []BrokerDailyDetail `json:"brokers"`
	}{
		Date:    dateStr,
		Source:  "broker-aggregate",
		Brokers: rows,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal broker details: %w", err)
	}
	path := filepath.Join(a.outputDir, dateStr+"_brokers.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

var (
	viewStateRegex       = regexp.MustCompile(`id="__VIEWSTATE"\s+value="([^"]+)"`)
	viewStateGenRegex    = regexp.MustCompile(`id="__VIEWSTATEGENERATOR"\s+value="([^"]+)"`)
	eventValidationRegex = regexp.MustCompile(`id="__EVENTVALIDATION"\s+value="([^"]+)"`)
)

// extractFormTokens pulls the ASP.NET hidden fields from the menu page.
func extractFormTokens(body []byte) (string, string, string, error) {
	vs := extractToken(viewStateRegex, body)
	vg := extractToken(viewStateGenRegex, body)
	ev := extractToken(eventValidationRegex, body)
	if vs == "" || vg == "" || ev == "" {
		return "", "", "", fmt.Errorf("missing form token: viewstate=%t generator=%t eventvalidation=%t", vs != "", vg != "", ev != "")
	}
	return vs, vg, ev, nil
}

func extractToken(re *regexp.Regexp, body []byte) string {
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// parseBrokerTableHTML parses the first HTML table that looks like a broker
// branch table. It supports header-driven column mapping and falls back to
// position-based parsing for simple test fixtures.
func (a *GovernmentBrokerAggregator) parseBrokerTableHTML(symbol string, body []byte) (*GovernmentBrokerDailyResult, error) {
	if hasCaptcha(body) {
		return nil, fmt.Errorf("%w %s", ErrCaptchaRequired, symbol)
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	rows := findBrokerTableRows(doc)
	if len(rows) < 2 {
		// Fallback to CSV parser for plain CSV responses.
		govNet, insNet, err := a.parseBrokerCSV(symbol, body)
		if err != nil {
			return nil, fmt.Errorf("no broker table for %s", symbol)
		}
		return &GovernmentBrokerDailyResult{GovNet: govNet, InsNet: insNet}, nil
	}

	colMap := mapBrokerColumns(rows[0])
	result := &GovernmentBrokerDailyResult{}
	for _, row := range rows[1:] {
		cells := extractCells(row)
		if len(cells) < 3 {
			continue
		}
		code, name, buy, sell, net, ok := parseBrokerCells(cells, colMap)
		if !ok {
			continue
		}
		if _, ok := coreBankBranches[code]; ok {
			result.GovNet += net
			result.Gov = append(result.Gov, BrokerBranchNet{Code: code, Name: name, Buy: buy, Sell: sell, Net: net})
		}
		if _, ok := insuranceBrokerCodes[code]; ok {
			result.InsNet += net
			result.Ins = append(result.Ins, BrokerBranchNet{Code: code, Name: name, Buy: buy, Sell: sell, Net: net})
		}
	}

	return result, nil
}

// findBrokerTableRows walks the HTML document and returns the rows of the first
// table that looks like a broker table (header contains "券商" or rows begin
// with 4-digit broker codes).
func findBrokerTableRows(doc *html.Node) []*html.Node {
	var bestRows []*html.Node
	var bestScore int

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			rows := tableRows(n)
			if len(rows) < 2 {
				return
			}
			score := brokerTableScore(rows)
			if score > bestScore {
				bestScore = score
				bestRows = rows
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return bestRows
}

func tableRows(table *html.Node) []*html.Node {
	var rows []*html.Node
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "tr" {
			rows = append(rows, c)
		}
		// Also recurse into tbody/thead/tfoot.
		if c.Type == html.ElementNode && (c.Data == "tbody" || c.Data == "thead" || c.Data == "tfoot") {
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				if cc.Type == html.ElementNode && cc.Data == "tr" {
					rows = append(rows, cc)
				}
			}
		}
	}
	return rows
}

func brokerTableScore(rows []*html.Node) int {
	if len(rows) == 0 {
		return 0
	}
	score := 0
	headerText := strings.Join(extractCells(rows[0]), " ")
	if strings.Contains(headerText, "券商") {
		score += 10
	}
	if strings.Contains(headerText, "買進") || strings.Contains(headerText, "賣出") || strings.Contains(headerText, "淨買") {
		score += 5
	}
	for _, row := range rows[1:] {
		cells := extractCells(row)
		if len(cells) > 0 && isBrokerCode(cells[0]) {
			score += 1
		}
	}
	return score
}

func isBrokerCode(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return false
	}
	prefix := s[:4]
	_, err := strconv.Atoi(prefix)
	return err == nil
}

// columnMapping holds the indices of the columns we care about.
type columnMapping struct {
	Code  int
	Name  int
	Buy   int
	Sell  int
	Net   int
	Found bool
}

// mapBrokerColumns maps header keywords to column indices. If no header is
// recognized, it returns an empty mapping and the caller falls back to position
// parsing.
func mapBrokerColumns(header *html.Node) columnMapping {
	cells := extractCells(header)
	m := columnMapping{Code: -1, Name: -1, Buy: -1, Sell: -1, Net: -1}
	for i, cell := range cells {
		text := strings.TrimSpace(cell)
		switch {
		case strings.Contains(text, "代號") && m.Code < 0:
			m.Code = i
		case strings.Contains(text, "名稱") || strings.Contains(text, "券商") && m.Name < 0:
			m.Name = i
		case strings.Contains(text, "買進") && m.Buy < 0:
			m.Buy = i
		case strings.Contains(text, "賣出") && m.Sell < 0:
			m.Sell = i
		case strings.Contains(text, "淨買") && m.Net < 0:
			m.Net = i
		}
	}
	if m.Code >= 0 && m.Name >= 0 && m.Buy >= 0 && m.Sell >= 0 && m.Net >= 0 {
		m.Found = true
	}
	return m
}

func extractCells(row *html.Node) []string {
	var cells []string
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			cells = append(cells, nodeText(c))
		}
	}
	return cells
}

func nodeText(n *html.Node) string {
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(buf.String())
}

func parseBrokerCells(cells []string, m columnMapping) (code, name string, buy, sell, net int64, ok bool) {
	idx := func(i int) string {
		if i < 0 || i >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[i])
	}

	if m.Found {
		code = idx(m.Code)
		name = idx(m.Name)
		buy = parseAmount(idx(m.Buy))
		sell = parseAmount(idx(m.Sell))
		net = parseAmount(idx(m.Net))
	} else {
		// Position-based fallback: code, name, buy, sell, net.
		if len(cells) < 5 {
			return "", "", 0, 0, 0, false
		}
		code = strings.TrimSpace(cells[0])
		name = strings.TrimSpace(cells[1])
		buy = parseAmount(cells[2])
		sell = parseAmount(cells[3])
		net = parseAmount(cells[4])
	}

	code = strings.TrimSpace(code)
	if len(code) < 4 {
		return "", "", 0, 0, 0, false
	}
	code = code[:4]
	if buy == 0 && sell == 0 && net == 0 {
		// Still valid if the row is a zero row; keep the code.
	}
	return code, name, buy, sell, net, true
}

func parseAmount(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "+", "")
	s = strings.ReplaceAll(s, " ", "")
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseBrokerCSV parses a plain CSV body that contains broker rows. It is a
// fallback used when the upstream returns a CSV payload instead of an HTML table.
func (a *GovernmentBrokerAggregator) parseBrokerCSV(symbol string, body []byte) (govNet, insNet int64, err error) {
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 4 {
			continue
		}
		brokerID := strings.TrimSpace(record[0])
		if len(brokerID) < 4 {
			continue
		}
		code := brokerID[:4]
		netStr := ""
		if len(record) >= 5 {
			netStr = record[4]
		} else {
			netStr = record[3]
		}
		netStr = strings.ReplaceAll(netStr, ",", "")
		net, err := strconv.ParseInt(netStr, 10, 64)
		if err != nil {
			continue
		}
		if _, ok := coreBankBranches[code]; ok {
			govNet += net
		}
		if _, ok := insuranceBrokerCodes[code]; ok {
			insNet += net
		}
	}

	return govNet, insNet, nil
}

func (a *GovernmentBrokerAggregator) writeReading(r GovernmentFlowReading) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(a.outputDir, r.Date+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
