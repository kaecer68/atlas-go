package monitoring

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// defaultProfitBiasRatio controls how the profit bias is derived from the
// revenue bias when building an industry.NarrativeAdjustment. It is exposed
// as a named constant rather than an inline magic number so callers can
// override it at the call site if the parameters system later requires it.
const defaultProfitBiasRatio = 0.8

// KeywordMapping links a scraped keyword to an industry and a directional bias.
type KeywordMapping struct {
	Keyword    string  `json:"keyword"`
	IndustryID string  `json:"industry_id"`
	Bias       float64 `json:"bias"`       // -0.3 to +0.3
	EventType  string  `json:"event_type"` // "order_news", "policy", "earnings", "macro"
}

// NarrativeEvent is a single keyword hit produced by the bridge. It carries
// enough metadata for exponential decay, confidence scoring, and industry
// weight attribution.
type NarrativeEvent struct {
	Keyword    string    `json:"keyword"`
	IndustryID string    `json:"industry_id"`
	EventType  string    `json:"event_type"`
	HitCount   int       `json:"hit_count"`
	Sources    []string  `json:"sources"`
	DetectedAt time.Time `json:"detected_at"`
	Bias       float64   `json:"bias"`
	Confidence float64   `json:"confidence"`
}

// defaultDecayLambdas defines the half-life for each event class in seconds.
// The lambda is expressed as 1.0 / half-life so that ApplyDecay can compute
// bias(t) = bias_0 * exp(-lambda * elapsed_seconds).
var defaultDecayLambdas = map[string]float64{
	"order_news": 1.0 / (12 * 3600), // half-life 12hr
	"policy":     1.0 / (24 * 3600), // half-life 24hr
	"earnings":   1.0 / (24 * 3600), // half-life 24hr
	"macro":      1.0 / (24 * 3600), // half-life 24hr
}

// defaultKeywordMappings returns the hard-coded keyword-to-industry table.
// Each keyword may map to one KeywordMapping entry. This table is the Layer 3
// bridge between raw RSS/news tokens and the industry identifiers used by the
// cycle/sector allocation engines.
func defaultKeywordMappings() map[string][]KeywordMapping {
	return map[string][]KeywordMapping{
		// 先進封裝 / CoWoS
		"CoWoS": {{Keyword: "CoWoS", IndustryID: "packaging_testing", Bias: 0.25, EventType: "order_news"}},
		"先進封裝":  {{Keyword: "先進封裝", IndustryID: "packaging_testing", Bias: 0.25, EventType: "order_news"}},
		"3D IC": {{Keyword: "3D IC", IndustryID: "packaging_testing", Bias: 0.25, EventType: "order_news"}},

		// AI 伺服器 / GPU 訂單
		"GB300":     {{Keyword: "GB300", IndustryID: "ai_server", Bias: 0.30, EventType: "order_news"}},
		"GB200":     {{Keyword: "GB200", IndustryID: "ai_server", Bias: 0.30, EventType: "order_news"}},
		"B300":      {{Keyword: "B300", IndustryID: "ai_server", Bias: 0.30, EventType: "order_news"}},
		"NVIDIA 訂單": {{Keyword: "NVIDIA 訂單", IndustryID: "ai_server", Bias: 0.30, EventType: "order_news"}},

		// 散熱 / 液冷
		"散熱":  {{Keyword: "散熱", IndustryID: "thermal_cooling", Bias: 0.20, EventType: "order_news"}},
		"液冷":  {{Keyword: "液冷", IndustryID: "thermal_cooling", Bias: 0.20, EventType: "order_news"}},
		"浸沒式": {{Keyword: "浸沒式", IndustryID: "thermal_cooling", Bias: 0.20, EventType: "order_news"}},

		// 矽光子 / 光通訊
		"矽光子": {{Keyword: "矽光子", IndustryID: "optical_comm", Bias: 0.20, EventType: "order_news"}},
		"CPO": {{Keyword: "CPO", IndustryID: "optical_comm", Bias: 0.20, EventType: "order_news"}},
		"光通訊": {{Keyword: "光通訊", IndustryID: "optical_comm", Bias: 0.20, EventType: "order_news"}},

		// 記憶體
		"HBM":    {{Keyword: "HBM", IndustryID: "memory", Bias: 0.20, EventType: "order_news"}},
		"高頻寬記憶體": {{Keyword: "高頻寬記憶體", IndustryID: "memory", Bias: 0.20, EventType: "order_news"}},
		"DRAM":   {{Keyword: "DRAM", IndustryID: "memory", Bias: 0.20, EventType: "order_news"}},

		// 能源 / 核能 / 再生能源
		"核能":   {{Keyword: "核能", IndustryID: "energy", Bias: 0.15, EventType: "policy"}},
		"核電":   {{Keyword: "核電", IndustryID: "energy", Bias: 0.15, EventType: "policy"}},
		"SMR":  {{Keyword: "SMR", IndustryID: "energy", Bias: 0.15, EventType: "policy"}},
		"離岸風電": {{Keyword: "離岸風電", IndustryID: "renewable_energy", Bias: 0.15, EventType: "policy"}},
		"儲能":   {{Keyword: "儲能", IndustryID: "renewable_energy", Bias: 0.15, EventType: "policy"}},

		// 自動化 / 機器人
		"人形機器人": {{Keyword: "人形機器人", IndustryID: "automation", Bias: 0.20, EventType: "order_news"}},
		"機器人":   {{Keyword: "機器人", IndustryID: "automation", Bias: 0.20, EventType: "order_news"}},

		// 地緣政治 / 出口管制（半導體負面）
		"中美關稅":     {{Keyword: "中美關稅", IndustryID: "semiconductor", Bias: -0.15, EventType: "policy"}},
		"出口管制":     {{Keyword: "出口管制", IndustryID: "semiconductor", Bias: -0.15, EventType: "policy"}},
		"chip ban": {{Keyword: "chip ban", IndustryID: "semiconductor", Bias: -0.15, EventType: "policy"}},

		// 台積電先進製程
		"台積電 2nm": {{Keyword: "台積電 2nm", IndustryID: "foundry", Bias: 0.25, EventType: "order_news"}},
		"3nm 量產":  {{Keyword: "3nm 量產", IndustryID: "foundry", Bias: 0.25, EventType: "order_news"}},

		// 被動元件
		"被動元件": {{Keyword: "被動元件", IndustryID: "passive_components", Bias: 0.15, EventType: "order_news"}},
		"MLCC": {{Keyword: "MLCC", IndustryID: "passive_components", Bias: 0.15, EventType: "order_news"}},

		// 航運 / 運價
		"航運":   {{Keyword: "航運", IndustryID: "shipping", Bias: 0.20, EventType: "macro"}},
		"SCFI": {{Keyword: "SCFI", IndustryID: "shipping", Bias: 0.20, EventType: "macro"}},
		"BDI":  {{Keyword: "BDI", IndustryID: "shipping", Bias: 0.20, EventType: "macro"}},

		// 金融 / 金控
		"金融":   {{Keyword: "金融", IndustryID: "financial", Bias: 0.15, EventType: "policy"}},
		"金控":   {{Keyword: "金控", IndustryID: "financial", Bias: 0.15, EventType: "policy"}},
		"合併":   {{Keyword: "合併", IndustryID: "financial", Bias: 0.15, EventType: "policy"}},
		"股利政策": {{Keyword: "股利政策", IndustryID: "financial", Bias: 0.15, EventType: "policy"}},

		// 生技 / 新藥
		"生技":   {{Keyword: "生技", IndustryID: "biotech", Bias: 0.25, EventType: "earnings"}},
		"新藥":   {{Keyword: "新藥", IndustryID: "biotech", Bias: 0.25, EventType: "earnings"}},
		"FDA":  {{Keyword: "FDA", IndustryID: "biotech", Bias: 0.25, EventType: "earnings"}},
		"臨床試驗": {{Keyword: "臨床試驗", IndustryID: "biotech", Bias: 0.25, EventType: "earnings"}},

		// 總經
		"CPI":  {{Keyword: "CPI", IndustryID: "macro", Bias: 0.15, EventType: "macro"}},
		"通膨":   {{Keyword: "通膨", IndustryID: "macro", Bias: 0.15, EventType: "macro"}},
		"利率決策": {{Keyword: "利率決策", IndustryID: "macro", Bias: 0.15, EventType: "macro"}},
		"聯準會":  {{Keyword: "聯準會", IndustryID: "macro", Bias: 0.15, EventType: "macro"}},

		// 營建 / 房地產
		"營建":  {{Keyword: "營建", IndustryID: "construction", Bias: 0.10, EventType: "policy"}},
		"房市":  {{Keyword: "房市", IndustryID: "construction", Bias: 0.10, EventType: "policy"}},
		"都更":  {{Keyword: "都更", IndustryID: "construction", Bias: 0.10, EventType: "policy"}},
		"預售屋": {{Keyword: "預售屋", IndustryID: "construction", Bias: 0.10, EventType: "policy"}},

		// 食品 / 農產 / 大宗物資
		"食品":   {{Keyword: "食品", IndustryID: "food_agri", Bias: 0.10, EventType: "macro"}},
		"農產品":  {{Keyword: "農產品", IndustryID: "food_agri", Bias: 0.10, EventType: "macro"}},
		"大宗物資": {{Keyword: "大宗物資", IndustryID: "food_agri", Bias: 0.10, EventType: "macro"}},

		// 塑化 / 原油
		"塑化": {{Keyword: "塑化", IndustryID: "petrochem", Bias: 0.10, EventType: "macro"}},
		"油價": {{Keyword: "油價", IndustryID: "petrochem", Bias: 0.10, EventType: "macro"}},
		"原油": {{Keyword: "原油", IndustryID: "petrochem", Bias: 0.10, EventType: "macro"}},
	}
}

// defaultRSSFeeds returns the placeholder RSS feed URLs that a real scraper
// would consume. These are intentionally not wired to HTTP calls yet.
func defaultRSSFeeds() []string {
	return []string{
		"https://money.udn.com/rssfeed/news/1001/5591/5612?ch=news",
		"https://www.ctee.com.tw/rss/newslist_rss.xml",
		"https://www.digitimes.com/rss/daily.xml",
	}
}

// NarrativeEventBridge is a Layer 3 adapter that translates scraped RSS/news
// keyword signals into industry-level NarrativeAdjustment values. It applies
// exponential time decay to each signal and exposes the primitives needed by
// the cycle tracker and sector allocation engines.
type NarrativeEventBridge struct {
	mu                  sync.RWMutex
	rssFeeds            []string
	keywords            map[string][]KeywordMapping
	httpClient          *http.Client
	cachePath           string
	decayLambda         map[string]float64
	confidenceThreshold int
}

// NewNarrativeEventBridge creates a bridge with sensible defaults. If cachePath
// is empty, it falls back to data/state/narrative_cache.json under the current
// working directory.
func NewNarrativeEventBridge(cachePath string) *NarrativeEventBridge {
	if cachePath == "" {
		cachePath = filepath.Join("data", "state", "narrative_cache.json")
	}
	return &NarrativeEventBridge{
		rssFeeds:            defaultRSSFeeds(),
		keywords:            defaultKeywordMappings(),
		httpClient:          &http.Client{Timeout: 30 * time.Second},
		cachePath:           cachePath,
		decayLambda:         defaultDecayLambdas,
		confidenceThreshold: 3,
	}
}

// SetHTTPClient replaces the default HTTP client. Useful for tests and for
// injecting a gateway-compatible transport later without breaking the bridge's
// public signature.
func (b *NarrativeEventBridge) SetHTTPClient(client *http.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.httpClient = client
}

// getHTTPClient returns the current HTTP client under read lock.
func (b *NarrativeEventBridge) getHTTPClient() *http.Client {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.httpClient
}

// SetConfidenceThreshold overrides the default hit-count threshold used by
// ComputeConfidence.
func (b *NarrativeEventBridge) SetConfidenceThreshold(threshold int) {
	b.confidenceThreshold = threshold
}

// SetDecayLambda overrides the per-event-type decay coefficients.
func (b *NarrativeEventBridge) SetDecayLambda(lambdas map[string]float64) {
	b.decayLambda = lambdas
}

// SetCachePath overrides the path used by SaveCache and LoadCache.
func (b *NarrativeEventBridge) SetCachePath(path string) {
	b.cachePath = path
}

// rssFeed, rssChannel, and rssItem are minimal XML structs for parsing
// RSS 2.0 feeds without importing an external library.
type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Items []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

// Scrape fetches every registered RSS feed, parses each item's title and
// description for keyword matches, and returns deduplicated NarrativeEvent
// results. Feeds that fail to fetch or parse are skipped with a warning log.
func (b *NarrativeEventBridge) Scrape(ctx context.Context) ([]NarrativeEvent, error) {
	seen := make(map[string]bool) // "keyword|industry_id" dedup
	var events []NarrativeEvent

	for _, feedURL := range b.rssFeeds {
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, feedURL, nil)
		if err != nil {
			cancel()
			logging.Warn("narrative_bridge", "rss_request_create_failed",
				logging.FStr("feed", feedURL),
				logging.Err(err))
			continue
		}

		resp, err := b.getHTTPClient().Do(req)
		cancel()
		if err != nil {
			logging.Warn("narrative_bridge", "rss_fetch_failed",
				logging.FStr("feed", feedURL),
				logging.Err(err))
			continue
		}
		defer resp.Body.Close()

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logging.Warn("narrative_bridge", "rss_read_body_failed",
				logging.FStr("feed", feedURL),
				logging.Err(readErr))
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logging.Warn("narrative_bridge", "rss_non_200",
				logging.FStr("feed", feedURL),
				logging.FStr("status", resp.Status))
			continue
		}

		var feed rssFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			logging.Warn("narrative_bridge", "rss_parse_failed",
				logging.FStr("feed", feedURL),
				logging.Err(err))
			continue
		}

		for _, item := range feed.Channel.Items {
			titleLower := strings.ToLower(item.Title)
			descLower := strings.ToLower(item.Description)

			for keyword, mappings := range b.keywords {
				kwLower := strings.ToLower(keyword)
				if !strings.Contains(titleLower, kwLower) && !strings.Contains(descLower, kwLower) {
					continue
				}

				for _, m := range mappings {
					dedupKey := m.Keyword + "|" + m.IndustryID
					if seen[dedupKey] {
						continue
					}
					seen[dedupKey] = true

					events = append(events, NarrativeEvent{
						Keyword:    m.Keyword,
						IndustryID: m.IndustryID,
						EventType:  m.EventType,
						HitCount:   1,
						Sources:    []string{feedURL},
						DetectedAt: time.Now(),
						Bias:       m.Bias,
						Confidence: 0.7,
					})
				}
			}
		}
	}

	return events, nil
}

// ApplyDecay returns the time-decayed bias for an event.
//
//	bias(t) = bias_0 * exp(-lambda * elapsed_seconds)
//
// Events detected in the future are clamped to elapsed=0 to avoid amplification.
func (b *NarrativeEventBridge) ApplyDecay(event NarrativeEvent, now time.Time) float64 {
	lambda, ok := b.decayLambda[event.EventType]
	if !ok {
		lambda = b.decayLambda["macro"]
	}
	elapsed := now.Sub(event.DetectedAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return event.Bias * math.Exp(-lambda*elapsed)
}

// MapIndustries expands a list of matched keywords into their industry
// mappings. The same (keyword, industry_id) pair is only returned once.
func (b *NarrativeEventBridge) MapIndustries(matchedKeywords []string) []KeywordMapping {
	seen := make(map[string]struct{})
	result := make([]KeywordMapping, 0, len(matchedKeywords))
	for _, kw := range matchedKeywords {
		mappings, ok := b.keywords[kw]
		if !ok {
			continue
		}
		for _, m := range mappings {
			key := m.Keyword + "|" + m.IndustryID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IndustryID != result[j].IndustryID {
			return result[i].IndustryID < result[j].IndustryID
		}
		return result[i].Keyword < result[j].Keyword
	})
	return result
}

// ComputeConfidence maps a hit count to a [0, 1] confidence score. It saturates
// at 1.0 once hitCount reaches confidenceThreshold.
func (b *NarrativeEventBridge) ComputeConfidence(hitCount int) float64 {
	if b.confidenceThreshold <= 0 {
		return 1.0
	}
	ratio := float64(hitCount) / float64(b.confidenceThreshold)
	if ratio >= 1.0 {
		return 1.0
	}
	return ratio
}

// StockWeightBias converts an industry-level bias into a per-stock bias using
// the stock's score share within its industry.
func (b *NarrativeEventBridge) StockWeightBias(industryBias, stockScore, totalIndustryScore float64) float64 {
	if totalIndustryScore == 0 {
		return 0
	}
	return industryBias * (stockScore / totalIndustryScore)
}

// BuildAdjustment aggregates decayed keyword signals for a single industry
// into the NarrativeAdjustment consumed by industry.CycleTracker.
func (b *NarrativeEventBridge) BuildAdjustment(industryID string, events []NarrativeEvent, now time.Time) industry.NarrativeAdjustment {
	var totalBias float64
	var maxConfidence float64
	var activeTheme string

	for _, e := range events {
		if e.IndustryID != industryID {
			continue
		}
		decayed := b.ApplyDecay(e, now)
		confidence := b.ComputeConfidence(e.HitCount)
		weighted := decayed * confidence
		totalBias += weighted
		if confidence > maxConfidence {
			maxConfidence = confidence
			activeTheme = e.Keyword
		}
	}

	if maxConfidence == 0 {
		return industry.NarrativeAdjustment{}
	}

	return industry.NarrativeAdjustment{
		RevenueBias: totalBias,
		ProfitBias:  totalBias * defaultProfitBiasRatio,
		Confidence:  maxConfidence,
		ActiveTheme: activeTheme,
	}
}

// SaveCache persists the given events to cachePath as JSON.
func (b *NarrativeEventBridge) SaveCache(events []NarrativeEvent) error {
	dir := filepath.Dir(b.cachePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("narrative bridge: create cache directory %q: %w", dir, err)
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("narrative bridge: marshal cache: %w", err)
	}
	tmpPath := b.cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		return fmt.Errorf("narrative bridge: write cache tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, b.cachePath); err != nil {
		return fmt.Errorf("narrative bridge: commit cache %q: %w", b.cachePath, err)
	}
	return nil
}

// LoadCache reads events previously saved to cachePath.
func (b *NarrativeEventBridge) LoadCache() ([]NarrativeEvent, error) {
	data, err := os.ReadFile(b.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []NarrativeEvent{}, nil
		}
		return nil, fmt.Errorf("narrative bridge: read cache %q: %w", b.cachePath, err)
	}
	var events []NarrativeEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("narrative bridge: unmarshal cache: %w", err)
	}
	return events, nil
}
