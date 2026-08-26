// Package marketdata: histock_broker8.go
//
// HistockBroker8Provider fetches the daily 八大公股行庫買賣超 ranking from
// HiStock (histock.tw/stock/broker8.aspx) as the machine-readable replacement
// for the CAPTCHA-blocked TWSE bsr.twse.com.tw scraper (root-cause analysis
// 2026-08-26: bsr serves a CAPTCHA to every automated session; BK-13's
// scraper never produced non-zero data).
//
// Page structure (verified 2026-08-26, server-rendered, no CAPTCHA):
//   - Two <ul class="stock-list"> blocks: buy-side Top N, then sell-side Top N.
//   - Each row is an <li> with onclick='goUrl("2330");' and ordered <span>
//     cells: [rank marker, stock name, 合庫, 土銀, 台銀, 台企銀, 彰銀,
//     第一金, 兆豐銀, 華南永昌, 合計] — amounts in 萬元 (10k TWD), negative
//     on the sell side.
//   - Historical dates via ?d=YYYY/MM/DD (verified back to 2024-06).
//   - robots.txt does not disallow this path.
//
// Coverage caveat (documented in government-force-proxy-spec.md): only the
// Top30 buy + Top30 sell rows are published per day; stocks outside that
// ranking carry negligible public-bank flow and are not counted.
package marketdata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
	"golang.org/x/net/html"
)

// ErrHistockBroker8Schema is returned when the HiStock broker8 page
// contains ranked rows (li with a goUrl symbol) but none of them parse —
// the upstream changed the cell layout. Callers must treat this as a real
// upstream failure (breaker failure / error), NOT as a holiday no-data
// condition. Mirrors ErrTAIFEXSchema (anti-regression, k3 advisor 2026-08-26).
var ErrHistockBroker8Schema = fmt.Errorf("histock broker8: schema mismatch: %w", ErrSchema)

// histockBankCodes maps the HiStock column short names to the canonical
// core-bank branch codes used by coreBankBranches / broker detail files.
var histockBankCodes = map[string]string{
	"合庫":   "8060",
	"土銀":   "8030",
	"台銀":   "8040",
	"台企銀":  "8010",
	"彰銀":   "8064",
	"第一金":  "8011",
	"兆豐銀":  "8061",
	"華南永昌": "8080",
}

// histockBankOrder is the column order of bank amounts inside each <li>.
var histockBankOrder = []string{"合庫", "土銀", "台銀", "台企銀", "彰銀", "第一金", "兆豐銀", "華南永昌"}

// HistockBroker8Row is one stock row from the daily ranking.
type HistockBroker8Row struct {
	Symbol   string
	Name     string
	Banks    map[string]int64 // bank short name → amount (萬元), signed
	TotalWan int64            // 合計 (萬元), negative = net sell
}

// HistockBroker8Provider fetches and parses the HiStock broker8 page.
type HistockBroker8Provider struct {
	client  *http.Client
	baseURL string
}

// NewHistockBroker8Provider creates a provider pointing at the live page.
func NewHistockBroker8Provider() *HistockBroker8Provider {
	return &HistockBroker8Provider{
		client:  httpclient.NewFactory().NewClient(15 * time.Second),
		baseURL: "https://histock.tw/stock/broker8.aspx",
	}
}

// SetHTTPClient overrides the HTTP client (tests only).
func (p *HistockBroker8Provider) SetHTTPClient(c *http.Client) {
	if c != nil {
		p.client = c
	}
}

// SetBaseURL overrides the page URL (tests only).
func (p *HistockBroker8Provider) SetBaseURL(u string) {
	p.baseURL = u
}

// FetchDaily retrieves the broker8 ranking for the given date. An empty
// (row-less) page — holiday or data not yet published — returns an empty
// slice with a nil error so callers can distinguish no-data from failure.
func (p *HistockBroker8Provider) FetchDaily(ctx context.Context, date time.Time) ([]HistockBroker8Row, error) {
	url := fmt.Sprintf("%s?d=%s", p.baseURL, date.Format("2006/01/02"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("histock create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("histock fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("histock read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: histock broker8 HTTP %d", ErrUpstream, resp.StatusCode)
	}
	return ParseHistockBroker8HTML(body)
}

var histockGoURLRe = regexp.MustCompile(`goUrl\("(\d+)"\)`)

// ParseHistockBroker8HTML extracts all ranked rows from the page HTML.
//
// Anti-regression (k3 advisor 2026-08-26): a page that renders ranked
// rows but parses NONE of them (upstream changed the cell layout) returns
// ErrHistockBroker8Schema so the caller records a real failure instead of
// a fake "holiday / no data" success — the exact failure mode that killed
// the bsr.twse.com.tw scraper. A genuinely empty page (no ranked rows,
// e.g. holiday) returns an empty slice with nil error. Partial pages keep
// the parseable rows and emit a warning.
func ParseHistockBroker8HTML(body []byte) ([]HistockBroker8Row, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("histock parse html: %w", err)
	}
	var rows []HistockBroker8Row
	ranked, malformed := 0, 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && histockRankedLi(n) {
			ranked++
			if r, ok := parseHistockRow(n); ok {
				rows = append(rows, r)
			} else {
				malformed++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if ranked == 0 {
		// No ranked rows: holiday / not-yet-published page.
		return nil, nil
	}
	if len(rows) == 0 {
		// Every ranked row malformed → upstream layout change.
		return nil, fmt.Errorf("%w: %d ranked rows, 0 parsed", ErrHistockBroker8Schema, ranked)
	}
	if malformed > 0 {
		logging.Warn("histock", "broker8_partial_parse",
			"ranked", ranked, "parsed", len(rows), "malformed", malformed)
	}
	return rows, nil
}

// histockRankedLi reports whether an <li> is a ranking row (has the
// goUrl("SYM") onclick attribute) vs an unrelated list item.
func histockRankedLi(li *html.Node) bool {
	for _, a := range li.Attr {
		if a.Key == "onclick" && histockGoURLRe.MatchString(a.Val) {
			return true
		}
	}
	return false
}

// parseHistockRow converts one <li> into a row. Returns ok=false for
// non-ranking <li> elements (no goUrl symbol or malformed cells).
func parseHistockRow(li *html.Node) (HistockBroker8Row, bool) {
	sym := ""
	for _, a := range li.Attr {
		if a.Key == "onclick" {
			if m := histockGoURLRe.FindStringSubmatch(a.Val); len(m) == 2 {
				sym = m[1]
			}
		}
	}
	if sym == "" {
		return HistockBroker8Row{}, false
	}
	var cells []string
	for s := li.FirstChild; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && s.Data == "span" {
			cells = append(cells, nodeText(s))
		}
	}
	if len(cells) < 3+len(histockBankOrder) { // rank + name + 8 banks (+ total)
		return HistockBroker8Row{}, false
	}
	name := strings.TrimPrefix(cells[1], "\u00a0")
	name = strings.TrimSpace(strings.ReplaceAll(name, "\u00a0", ""))
	banks := make(map[string]int64, len(histockBankOrder))
	for i, bank := range histockBankOrder {
		v, err := strconv.ParseInt(strings.ReplaceAll(cells[2+i], ",", ""), 10, 64)
		if err != nil {
			return HistockBroker8Row{}, false
		}
		banks[bank] = v
	}
	total, err := strconv.ParseInt(strings.ReplaceAll(cells[2+len(histockBankOrder)], ",", ""), 10, 64)
	if err != nil {
		return HistockBroker8Row{}, false
	}
	return HistockBroker8Row{Symbol: sym, Name: name, Banks: banks, TotalWan: total}, true
}
