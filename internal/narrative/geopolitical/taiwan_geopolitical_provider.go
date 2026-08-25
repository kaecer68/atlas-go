package geopolitical

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TaiwanRSSGeopoliticalProvider monitors Taiwan-related geopolitical news via RSS feeds.
// It extends the RSS pattern to track cross-strait tensions, military drills, and sanctions.
type TaiwanRSSGeopoliticalProvider struct {
	mu       sync.RWMutex
	client   *http.Client
	feeds    []string
	keywords []string
	limiter  *rate.Limiter
}

// NewTaiwanRSSGeopoliticalProvider creates a Taiwan geopolitical risk monitor.
// Uses mainstream financial news RSS feeds (DIGITIMES, 經濟日報, 非凡新聞) for
// market-relevant cross-strait / sanctions / macro keywords.
func NewTaiwanRSSGeopoliticalProvider() *TaiwanRSSGeopoliticalProvider {
	return &TaiwanRSSGeopoliticalProvider{
		client:  httpclient.NewFactory().NewClient(15 * time.Second),
		limiter: rate.NewLimiter(rate.Every(10*time.Second), 1),
		feeds: []string{
			"https://www.digitimes.com/rss/daily.xml",            // DIGITIMES 科技供應鏈
			"https://money.udn.com/rssfeed/lists/1001",           // 經濟日報(聯合報系)
			"https://news.ustv.com.tw/feed",                      // 非凡新聞
			"https://wwwc.twse.com.tw/rwd/zh/news/feed?type=rss", // TWSE 證交所新聞
		},
		keywords: []string{
			// Chinese keywords
			"台灣", "中國", "兩岸", "軍演", "制裁", "共機", "共艦",
			"台海", "陸委會", "海基會", "AIT", "美台", "抗中",
			"武統", "和統", "九二共識", "一中", "台獨", "維持現狀",
			// English keywords
			"taiwan", "china", "chinese", "cross-strait", "cross strait",
			"military drill", "military exercise", "naval exercise",
			"sanction", "sanctions", "blacklist",
			"PLA", "People's Liberation Army", "PLA Navy",
			"aircraft carrier", "warship", "drone",
			"Taiwan Strait", "Taiwan relations act", "TRA",
			"Tsai Ing-wen", "Lai Ching-te", "president",
			"US arms sale", "arms sale", "F-16", "missile",
		},
	}
}

// Name returns the provider name.
func (t *TaiwanRSSGeopoliticalProvider) Name() string {
	return "taiwan_rss_geopolitical"
}

// SetHTTPClient overrides the default HTTP client (testability hook).
// Locked with mu to prevent races with concurrent FetchScore readers.
func (t *TaiwanRSSGeopoliticalProvider) SetHTTPClient(client *http.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.client = client
}

// FetchScore aggregates keyword counts from Taiwan RSS feeds and maps to intensity.
func (t *TaiwanRSSGeopoliticalProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	var totalMatches int
	var succeeded []string
	var events []GeoEvent
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, url := range t.feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			matches, evs, err := t.countKeywordsInFeed(ctx, url)
			if err != nil {
				logging.Warn("taiwan_geopolitical_provider", "feed_failed", logging.FStr("url", url), logging.Err(err))
				return
			}
			mu.Lock()
			totalMatches += matches
			succeeded = append(succeeded, url)
			events = append(events, evs...)
			mu.Unlock()
		}(url)
	}

	wg.Wait()

	// G5-4: cap + dedupe event-layer trace items (newest-first not available
	// from RSS without pubDate; dedupe by title keeps the sample meaningful).
	seen := make(map[string]struct{}, len(events))
	deduped := make([]GeoEvent, 0, len(events))
	for _, ev := range events {
		if ev.Title == "" {
			continue
		}
		if _, ok := seen[ev.Title]; ok {
			continue
		}
		seen[ev.Title] = struct{}{}
		deduped = append(deduped, ev)
		if len(deduped) >= 20 {
			break
		}
	}

	// Map matches to 0-100 intensity
	// Cross-strait news is typically less frequent but more impactful than Middle East conflicts
	intensity := float64(totalMatches) * 4.0
	if intensity > 100 {
		intensity = 100
	}
	if intensity < 5 {
		intensity = 5
	}

	// Sentiment tends to be more neutral to negative depending on news content
	sentiment := -0.3
	if totalMatches > 10 {
		sentiment = -0.5
	}

	score := GeopoliticalRiskScore{
		Region:         "Taiwan",
		Intensity:      intensity,
		Sentiment:      sentiment,
		Confidence:     0.60,
		OilImpact:      0.1, // Lower oil impact than Middle East
		ShippingImpact: 0.4, // Taiwan Strait shipping risk
		Sources:        succeeded,
		Events:         deduped,
		Timestamp:      time.Now().UTC(),
	}
	return score, nil
}

func (t *TaiwanRSSGeopoliticalProvider) countKeywordsInFeed(ctx context.Context, url string) (int, []GeoEvent, error) {
	if err := t.limiter.Wait(ctx); err != nil {
		return 0, nil, fmt.Errorf("rate limit: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err != nil {
		return 0, nil, err
	}

	matches := 0
	var events []GeoEvent
	for _, item := range rss.Channel.Items {
		text := strings.ToLower(item.Title + " " + item.Description)
		for _, kw := range t.keywords {
			if strings.Contains(text, kw) {
				matches++
				// G5-4: keep the first matched keyword per item for tracing.
				if len(events) < 20 {
					events = append(events, GeoEvent{
						Title:   strings.TrimSpace(item.Title),
						Keyword: kw,
						Source:  url,
					})
				}
				break // one event per item is enough for tracing
			}
		}
	}
	return matches, events, nil
}

// CompositeTaiwanGeopoliticalProvider combines Taiwan RSS with other Taiwan-focused providers.
type CompositeTaiwanGeopoliticalProvider struct {
	providers []GeopoliticalRiskProvider
}

// NewCompositeTaiwanGeopoliticalProvider creates a composite Taiwan provider.
func NewCompositeTaiwanGeopoliticalProvider(providers ...GeopoliticalRiskProvider) *CompositeTaiwanGeopoliticalProvider {
	return &CompositeTaiwanGeopoliticalProvider{providers: providers}
}

// Name returns the provider name.
func (c *CompositeTaiwanGeopoliticalProvider) Name() string {
	return "composite_taiwan_geopolitical"
}

// SetHTTPClient propagates the custom HTTP client to inner providers that support it.
func (c *CompositeTaiwanGeopoliticalProvider) SetHTTPClient(client *http.Client) {
	for _, p := range c.providers {
		if setter, ok := p.(interface{ SetHTTPClient(*http.Client) }); ok {
			setter.SetHTTPClient(client)
		}
	}
}

// FetchScore averages intensity across all Taiwan providers.
func (c *CompositeTaiwanGeopoliticalProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	var totalIntensity float64
	var totalConfidence float64
	var totalOilImpact float64
	var totalShippingImpact float64
	var sources []string
	var count int

	for _, p := range c.providers {
		score, err := p.FetchScore(ctx)
		if err != nil {
			logging.Warn("taiwan_geopolitical_provider", "provider_failed", logging.FStr("provider", p.Name()), logging.Err(err))
			continue
		}
		totalIntensity += score.Intensity
		totalConfidence += score.Confidence
		totalOilImpact += score.OilImpact
		totalShippingImpact += score.ShippingImpact
		sources = append(sources, p.Name())
		count++
	}

	if count == 0 {
		return GeopoliticalRiskScore{}, fmt.Errorf("all Taiwan geopolitical providers failed")
	}

	return GeopoliticalRiskScore{
		Region:         "Taiwan",
		Intensity:      totalIntensity / float64(count),
		Sentiment:      -0.4,
		Confidence:     totalConfidence / float64(count),
		OilImpact:      totalOilImpact / float64(count),
		ShippingImpact: totalShippingImpact / float64(count),
		Sources:        sources,
		Timestamp:      time.Now().UTC(),
	}, nil
}
