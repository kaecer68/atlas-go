package marketdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// FubonETFProvider 抓取富邦投信官網「申購買回清單 (PCF)」頁面，彙總主力
// 富邦 ETF 的每日淨申購/贖回金額（TWD），供 RSI-tw subC3（ETF 申購分數）
// 消費。
//
// 背景（2026-08-17 實測）：TWSE TWT44U（全市場 ETF 申購贖回淨額彙總報表）
// 已於上游移除（HTTP 307 → 404，見 known_issues.go twse_etf_upstream_60d），
// twse_etf channel 因此停用。富邦投信官網 PCF 頁面
//
//	https://websys.fsit.com.tw/FubonETF/Trade/Pcf.aspx?stkId=<代碼>&lan=TW
//
// 免費、免 key、純 HTTP GET 可抓（無需 Playwright），每支 ETF 一頁，內含：
//   - 「與前日已發行單位差異數」＝當日申購/贖回淨額（受益權單位數，
//     正=淨申購 負=淨贖回；實測 006208→0、00900→-1,000,000、00692→-500,000）
//   - 「每受益權單位淨資產價值」(NAV/unit, TWD)
//   - 「基金淨資產價值」(基金總規模, TWD)
//
// 彙總方式：NetSubscription = Σ(差異數 × NAV/unit)（TWD 加權）。與
// RSI-tw C3 thresholds（單位 TWD，見 defaults_engine.go C3BullishThreshold
// 等）一致；若直接加總單位數會被高 NAV 的 006208/0057 稀釋失真。
// TotalNAV = Σ(基金淨資產價值)。SubscriberCount 無對應欄位，恆 0。
//
// ⚠️ 涵蓋範圍：僅富邦主力 ETF（DefaultFubonETFSymbols，8 支），非全市場
// 彙總 — 作為 subC3 的「方向性代理指標」使用（實測每日有真實非零值）。
// 單支 ETF 失敗不影響其他（best-effort）；全部失敗才回傳錯誤。
type FubonETFProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	symbols     []string
	now         func() time.Time

	cacheMu  sync.RWMutex
	cached   *ETFStats
	cachedAt time.Time
	cacheTTL time.Duration
}

// DefaultFubonETFSymbols 是富邦投信主力台股 ETF（2026-08-17 擴充至 10 支，
// 加入半導體 00892 與主動式 00405A；規模參考 fundclear 觀測站全市場清單）。
// PCF 頁面每日更新一次，10 支 × 1 頁 = 每日 10 個 GET，量級遠低於任何
// rate limit 顧慮。
var DefaultFubonETFSymbols = []string{
	"006208", // 富邦台50（規模 ~4530 億，最大）
	"0052",   // 富邦科技（~1643 億）
	"00692",  // 富邦公司治理100（~560 億）
	"00405A", // 富邦台灣龍耀主動（~312 億）
	"00900",  // 富邦特選高股息30（~300 億）
	"00892",  // 富邦台灣半導體（~134 億）
	"00733",  // 富邦臺灣中小（~44 億）
	"00730",  // 富邦臺灣優質高息（~21 億）
	"0057",   // 富邦摩台（~5 億）
	"009802", // 富邦旗艦動能50
}

// DefaultFubonETFCacheTTL 控制 provider 內部 TTL cache。PCF 每個交易日才
// 更新一次，短 TTL 只是避免每次 dashboard 刷新都重抓 8 支（網頁本身回
// Cache-Control: no-store）。
const DefaultFubonETFCacheTTL = 10 * time.Minute

const (
	// fubonPcfBaseURL 是富邦投信「申購買回清單 (PCF)」頁面。
	fubonPcfBaseURL = "https://websys.fsit.com.tw/FubonETF/Trade/Pcf.aspx"
	// fubonETFFetchTimeout 單支 ETF 頁面的 HTTP timeout（實測 ~0.6-0.8s）。
	fubonETFFetchTimeout = 10 * time.Second
	// fubonETFMaxConcurrency 限制同時對富邦網站的連線數（配合 rate limiter，
	// 讓 8 支在 RSI-tw handler 的 5s fetchCtx 內完成）。
	fubonETFMaxConcurrency = 3
	// fubonETFUserAgent 標示來源（該站無 UA 白名單，仍附上禮貌性 UA）。
	fubonETFUserAgent = "Mozilla/5.0 (compatible; atlas-go/1.0)"
)

// FubonETF typed errors（比照 twse_etf_provider.go 的 A05 慣例，供
// adapter/circuit-breaker 區分上游故障與 schema 變更）。
var (
	// ErrFubonETFUpstream：transport/HTTP 層失敗（timeout、DNS、4xx/5xx）。
	ErrFubonETFUpstream = errors.New("fubon_etf: upstream failure")
	// ErrFubonETFSchema：回應無法解析（WAF 頁面、schema 改變、非預期格式）。
	ErrFubonETFSchema = errors.New("fubon_etf: schema mismatch")
	// ErrFubonETFNoData：全部主力 ETF 都抓取失敗（best-effort 收斂結果）。
	ErrFubonETFNoData = errors.New("fubon_etf: no ETF data available")
)

// ETFNetSubFetcher 抽象 ETF 淨申購/贖回資料源，供 monitoring.NewETFFetcher
// 注入與測試替換。
type ETFNetSubFetcher interface {
	FetchETFNetSubscription(ctx context.Context) (*ETFStats, error)
}

// NewFubonETFProvider 建立預設的富邦 PCF provider（真實 HTTP client +
// 寫死主力 ETF 清單 + 300ms/req 速率限制 + 10min TTL cache）。
func NewFubonETFProvider() *FubonETFProvider {
	return &FubonETFProvider{
		client:      httpclient.NewFactory().NewClient(fubonETFFetchTimeout),
		baseURL:     fubonPcfBaseURL,
		rateLimiter: rate.NewLimiter(rate.Every(300*time.Millisecond), 1),
		symbols:     append([]string(nil), DefaultFubonETFSymbols...),
		now:         time.Now,
		cacheTTL:    DefaultFubonETFCacheTTL,
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *FubonETFProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// SetRateLimiter overrides the rate limiter (for testing).
func (p *FubonETFProvider) SetRateLimiter(lim *rate.Limiter) {
	if lim != nil {
		p.rateLimiter = lim
	}
}

// SetSymbols overrides the ETF universe (for testing).
func (p *FubonETFProvider) SetSymbols(symbols []string) {
	if len(symbols) > 0 {
		p.symbols = append([]string(nil), symbols...)
	}
}

// SetCacheTTL overrides the in-memory cache TTL (TTL<=0 disables caching).
func (p *FubonETFProvider) SetCacheTTL(ttl time.Duration) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	p.cacheTTL = ttl
	if ttl <= 0 {
		p.cached = nil
	}
}

// Name returns the provider name.
func (p *FubonETFProvider) Name() string {
	return "fubon_etf_pcf"
}

// FetchETFNetSubscription 抓取全部主力 ETF 的 PCF 差異數並彙總。
// 結果以 TTL cache 快取，短時間內重複呼叫（dashboard 每 ~30s 刷新）
// 不會重複打富邦網站。
func (p *FubonETFProvider) FetchETFNetSubscription(ctx context.Context) (*ETFStats, error) {
	p.cacheMu.RLock()
	if p.cached != nil && p.cacheTTL > 0 && time.Since(p.cachedAt) < p.cacheTTL {
		cp := *p.cached
		p.cacheMu.RUnlock()
		return &cp, nil
	}
	p.cacheMu.RUnlock()

	stats, err := p.fetchAll(ctx)
	if err != nil {
		return nil, err
	}

	p.cacheMu.Lock()
	if p.cacheTTL > 0 {
		p.cached = stats
		p.cachedAt = p.now()
	}
	p.cacheMu.Unlock()
	return stats, nil
}

// fetchAll 對每支 ETF 平行抓取（有界 concurrency），best-effort 彙總。
// 至少 1 支成功即回傳結果；全部失敗回傳第一個錯誤（或 ErrFubonETFNoData）。
func (p *FubonETFProvider) fetchAll(ctx context.Context) (*ETFStats, error) {
	sem := make(chan struct{}, fubonETFMaxConcurrency)
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		netSub   int64
		totalNAV int64
		okCount  int
		date     string
		firstErr error
	)

	for _, symbol := range p.symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				mu.Unlock()
				return
			}

			diffUnits, navPerUnit, fundNAV, pageDate, err := p.fetchSymbol(ctx, sym)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", sym, err)
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			netSub += int64(math.Round(float64(diffUnits) * navPerUnit))
			totalNAV += fundNAV
			okCount++
			if date == "" && pageDate != "" {
				date = pageDate
			}
			mu.Unlock()
		}(symbol)
	}
	wg.Wait()

	if okCount == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrFubonETFNoData
	}
	if date == "" {
		// 頁面無日期欄位時以抓取日為準（PCF 每日更新，時效誤差 ≤1 交易日）。
		date = p.now().Format("20060102")
	}
	return &ETFStats{
		Date:            date,
		NetSubscription: netSub,
		TotalNAV:        totalNAV,
		SubscriberCount: 0,
	}, nil
}

// fetchSymbol 抓取單支 ETF 的 PCF 頁面並解析。
func (p *FubonETFProvider) fetchSymbol(ctx context.Context, symbol string) (diffUnits int64, navPerUnit float64, fundNAV int64, pageDate string, err error) {
	if err = p.rateLimiter.Wait(ctx); err != nil {
		return 0, 0, 0, "", fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s?stkId=%s&lan=TW", p.baseURL, symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", fubonETFUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("%w: http request: %v", ErrFubonETFUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, "", fmt.Errorf("%w: http status %d", ErrFubonETFUpstream, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("%w: read body: %v", ErrFubonETFUpstream, err)
	}

	fields, err := parseFubonPcf(body)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("%w (%s): %v", ErrFubonETFSchema, symbol, err)
	}
	return fields.diff, fields.nav, fields.fund, fields.date, nil
}

// fubonPcfFields 是單支 ETF PCF 頁面解析出的欄位。
type fubonPcfFields struct {
	diff   int64
	diffOK bool
	nav    float64
	navOK  bool
	fund   int64
	date   string // "20060102"
}

// fubonDateRe 匹配頁面 header 的日期文字節點（如 "2026/08/17"）。
var fubonDateRe = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`)

// parseFubonPcf 從富邦 PCF HTML 抽出：
//   - diff：與前日已發行單位差異數（受益權單位數，可為 0/負；0 是合法值）
//   - nav：每受益權單位淨資產價值 (TWD)
//   - fund：基金淨資產價值 (TWD)
//   - date：頁面日期（"2026/08/17" → "20260817"）
//
// 頁面結構：<li><p>標籤</p><p>數值</p></li>（數值可能含逗號/負號/NT$）。
// 差異數或 NAV 任一缺漏 → schema error（該支視為失敗）。
func parseFubonPcf(body []byte) (*fubonPcfFields, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var f fubonPcfFields
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if f.date == "" && fubonDateRe.MatchString(t) {
				f.date = strings.ReplaceAll(t, "/", "")
			}
		}
		if n.Type == html.ElementNode && n.Data == "li" {
			label, value, ok := liLabelValue(n)
			if ok {
				switch label {
				case "與前日已發行單位差異數":
					if v, parsed := parseFubonInt(value); parsed {
						f.diff, f.diffOK = v, true
					}
				case "每受益權單位淨資產價值":
					if v, parsed := parseFubonMoney(value); parsed {
						f.nav, f.navOK = v, true
					}
				case "基金淨資產價值":
					if v, parsed := parseFubonInt(value); parsed {
						f.fund = v
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if !f.diffOK || !f.navOK {
		return nil, fmt.Errorf("missing fields: diff_ok=%v nav_ok=%v", f.diffOK, f.navOK)
	}
	return &f, nil
}

// liLabelValue 回傳 <li> 的直接 <p> 子節點（label/value 成對）。PCF 頁面
// 的數值欄位都是 <li><p>標籤</p><p>數值</p></li> 兩兩成對。
func liLabelValue(li *html.Node) (label, value string, ok bool) {
	var ps []*html.Node
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "p" {
			ps = append(ps, c)
		}
	}
	if len(ps) != 2 {
		return "", "", false
	}
	label = strings.TrimSpace(nodeText(ps[0]))
	value = strings.TrimSpace(nodeText(ps[1]))
	return label, value, label != "" && value != ""
}

// parseFubonInt 解析整數（容忍 "NT$" 前綴、千分位逗號與負號）。"0" 是合法值。
// PCF 頁面的「基金淨資產價值」為 "NT$29,973,544,281" 整數格式。
func parseFubonInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "NT$")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseFubonMoney 解析金額（容忍 "NT$" 前綴、千分位逗號、小數）。
// "NT$18.97" → 18.97。
func parseFubonMoney(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "NT$")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
