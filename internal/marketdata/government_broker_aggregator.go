package marketdata

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// (bsr.twse.com.tw) and aggregates net buy/sell for the 5 core government banks'
// head offices, writing the result to the GovernmentFlowProvider's flat directory.
//
// Methodology (per docs/specs/government-force-proxy-spec.md and community practice):
//   - 5 core banks: 合庫(8060), 土銀(8030), 臺灣銀(8040), 台企銀(8010), 彰化(8064)
//   - Head office branch codes only (not all branches)
//   - Aggregated across the top N weighted stocks (TW50 constituents)
//   - Source: TWSE bsr.twse.com.tw — Open Data, publicly accessible
type GovernmentBrokerAggregator struct {
	client    *http.Client
	limiter   *rate.Limiter
	outputDir string
	baseURL   string
	symbols   []string
}

// coreBankBranches maps the 5 core government banks to their TWSE head-office
// branch codes. These are the head office (總公司) codes sourced from TWSE
// broker code registry.
var coreBankBranches = map[string]string{
	"8060": "合作金庫",
	"8030": "土地銀行",
	"8040": "臺灣銀行",
	"8010": "臺灣企銀",
	"8064": "彰化銀行",
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

// AggregateDate fetches broker data for the given trading date, aggregates net
// buy/sell for the 5 core banks across TW50 stocks, and writes the result as
// a GovernmentFlowReading JSON file.
func (a *GovernmentBrokerAggregator) AggregateDate(ctx context.Context, date time.Time) (*GovernmentFlowReading, error) {
	dateStr := date.Format("20060102")

	var totalNet int64
	var stocksProcessed int

	for _, symbol := range a.symbols {
		if err := a.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit: %w", err)
		}

		net, err := a.fetchStockBrokerNet(ctx, symbol, date)
		if err != nil {
			continue // individual failures non-fatal
		}
		totalNet += net
		stocksProcessed++
	}

	if stocksProcessed == 0 {
		return nil, fmt.Errorf("government_broker: no stocks processed for %s", dateStr)
	}

	reading := &GovernmentFlowReading{
		Date:     dateStr,
		TotalNet: totalNet,
		Source:   "broker-aggregate",
		RawURL:   "https://bsr.twse.com.tw/bshtm/bsMenu.aspx",
	}

	if err := a.writeReading(*reading); err != nil {
		return nil, fmt.Errorf("government_broker write: %w", err)
	}

	return reading, nil
}

// fetchStockBrokerNet fetches the broker trading report for a single stock.
func (a *GovernmentBrokerAggregator) fetchStockBrokerNet(ctx context.Context, symbol string, date time.Time) (int64, error) {
	rocDate := fmt.Sprintf("%d/%02d/%02d", date.Year()-1911, date.Month(), date.Day())

	url := fmt.Sprintf(
		"%s/bsContent.aspx?v=VOLUME&p=%s_%s",
		a.baseURL,
		symbol, strings.ReplaceAll(rocDate, "/", ""),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; atlas-go/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch %s: HTTP %d", symbol, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", symbol, err)
	}

	return a.parseBrokerTable(symbol, body)
}

// DataDir returns the directory where daily readings are written.
func (a *GovernmentBrokerAggregator) DataDir() string {
	return a.outputDir
}

// brokerRowRegex matches broker rows: code name buy sell net ...
var brokerRowRegex = regexp.MustCompile(`(\d{4}[A-Za-z]?\d*)\s+(\S+)\s+([\d,]+)\s+([\d,]+)\s+([\d,-]+)`)

func (a *GovernmentBrokerAggregator) parseBrokerTable(symbol string, body []byte) (int64, error) {
	// Try CSV first if the response looks like CSV
	if strings.HasPrefix(string(body), `"`) || strings.Contains(strings.ToLower(string(body)), "text/csv") {
		return a.parseBrokerCSV(symbol, body)
	}

	// HTML parsing
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("parse HTML: %w", err)
	}

	var textBuf bytes.Buffer
	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.TextNode {
			textBuf.WriteString(n.Data)
			textBuf.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}
	extractText(doc)

	matches := brokerRowRegex.FindAllStringSubmatch(textBuf.String(), -1)
	if len(matches) == 0 {
		return a.parseBrokerCSV(symbol, body)
	}

	var totalNet int64
	for _, m := range matches {
		brokerID := strings.TrimSpace(m[1])
		if _, ok := coreBankBranches[brokerID[:4]]; !ok {
			continue
		}
		netStr := strings.ReplaceAll(strings.TrimSpace(m[5]), ",", "")
		net, err := strconv.ParseInt(netStr, 10, 64)
		if err != nil {
			continue
		}
		totalNet += net
	}

	return totalNet, nil
}

func (a *GovernmentBrokerAggregator) parseBrokerCSV(symbol string, body []byte) (int64, error) {
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1

	var totalNet int64
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
		if _, ok := coreBankBranches[brokerID[:4]]; !ok {
			continue
		}
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
		totalNet += net
	}

	return totalNet, nil
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
