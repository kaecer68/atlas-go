package narrative

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GeopoliticalRiskScore represents a computed geopolitical risk reading.
type GeopoliticalRiskScore struct {
	Region         string    `json:"region"`
	Intensity      float64   `json:"intensity"`  // 0 - 100
	Sentiment      float64   `json:"sentiment"`  // -1.0 to +1.0 (negative = risk-off)
	Confidence     float64   `json:"confidence"` // 0.0 to 1.0
	OilImpact      float64   `json:"oil_impact"` // estimated impact direction
	ShippingImpact float64   `json:"shipping_impact"`
	Sources        []string  `json:"sources"`
	Timestamp      time.Time `json:"timestamp"`
}

// GeopoliticalRiskProvider fetches geopolitical risk indicators.
type GeopoliticalRiskProvider interface {
	Name() string
	FetchScore(ctx context.Context) (GeopoliticalRiskScore, error)
}

// RSSGeopoliticalProvider computes risk intensity by scanning RSS feeds for conflict keywords.
type RSSGeopoliticalProvider struct {
	client   *http.Client
	feeds    []string
	keywords []string
}

// NewRSSGeopoliticalProvider creates a default RSS-based Middle-East conflict monitor.
func NewRSSGeopoliticalProvider() *RSSGeopoliticalProvider {
	return &RSSGeopoliticalProvider{
		client: &http.Client{Timeout: 15 * time.Second},
		feeds: []string{
			"http://feeds.bbci.co.uk/news/world/middle_east/rss.xml",
			"https://www.aljazeera.com/xml/rss/all.xml",
		},
		keywords: []string{
			"israel", "hamas", "iran", "hezbollah", "houthi", "houthis",
			"gaza", "red sea", "middle east", "palestine", "lebanon",
			"syria", "yemen", "airstrike", "missile", "attack",
		},
	}
}

// Name returns the provider name.
func (r *RSSGeopoliticalProvider) Name() string {
	return "rss_geopolitical"
}

// FetchScore aggregates keyword counts from RSS feeds and maps to intensity.
func (r *RSSGeopoliticalProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	var totalMatches int
	var succeeded []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, url := range r.feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			matches, err := r.countKeywordsInFeed(ctx, url)
			if err != nil {
				log.Printf("[RSSGeopoliticalProvider] feed failed %s: %v", url, err)
				return
			}
			mu.Lock()
			totalMatches += matches
			succeeded = append(succeeded, url)
			mu.Unlock()
		}(url)
	}

	wg.Wait()

	// Map matches to 0-100 intensity (heuristic: 0-20 matches -> linear scale, >20 saturates towards 100)
	intensity := float64(totalMatches) * 3.0
	if intensity > 100 {
		intensity = 100
	}

	score := GeopoliticalRiskScore{
		Region:         "Middle East",
		Intensity:      intensity,
		Sentiment:      -0.7,
		Confidence:     0.55,
		OilImpact:      0.6,
		ShippingImpact: 0.5,
		Sources:        succeeded,
		Timestamp:      time.Now().UTC(),
	}
	return score, nil
}

func (r *RSSGeopoliticalProvider) countKeywordsInFeed(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err != nil {
		return 0, err
	}

	matches := 0
	for _, item := range rss.Channel.Items {
		text := strings.ToLower(item.Title + " " + item.Description)
		for _, kw := range r.keywords {
			if strings.Contains(text, kw) {
				matches++
			}
		}
	}
	return matches, nil
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

// GDELTGeopoliticalProvider fetches conflict event counts from GDELT 2.0 API.
type GDELTGeopoliticalProvider struct {
	client *http.Client
}

// NewGDELTGeopoliticalProvider creates a GDELT-based provider.
func NewGDELTGeopoliticalProvider() *GDELTGeopoliticalProvider {
	return &GDELTGeopoliticalProvider{
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name returns the provider name.
func (g *GDELTGeopoliticalProvider) Name() string {
	return "gdelt_geopolitical"
}

// FetchScore queries GDELT for violent events in the Middle East over the last 24h.
func (g *GDELTGeopoliticalProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	// GDELT Global Knowledge Graph (GKG) API query example for Middle East conflict keywords
	// We use a simple query against the GKG for "Israel" + "Gaza" in the last 24 hours.
	// GDELT 2.0 GKG API: https://blog.gdeltproject.org/gdelt-2-0-our-global-world-in-real-time/
	// For a lightweight MVP we use the GKG Summary API for the last 24h.
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	startDate := yesterday.Format("20060102") + "000000"
	endDate := now.Format("20060102") + "235959"

	url := fmt.Sprintf(
		"https://api.gdeltproject.org/api/v2/summary/summary?query=israel+gaza+hamas+iran&mode=artlist&maxrecords=50&startdatetime=%s&enddatetime=%s&format=json",
		startDate, endDate,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GeopoliticalRiskScore{}, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return GeopoliticalRiskScore{}, err
	}
	defer resp.Body.Close()

	// GDELT summary artlist JSON structure is variable; for MVP we treat non-empty response as positive signal.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GeopoliticalRiskScore{}, err
	}

	// Simple heuristic: length of response as proxy for activity volume.
	intensity := float64(len(body)) / 500.0
	if intensity > 100 {
		intensity = 100
	}
	if intensity < 5 {
		intensity = 5
	}

	score := GeopoliticalRiskScore{
		Region:         "Middle East",
		Intensity:      intensity,
		Sentiment:      -0.6,
		Confidence:     0.50,
		OilImpact:      0.5,
		ShippingImpact: 0.4,
		Sources:        []string{"GDELT 2.0"},
		Timestamp:      now,
	}
	return score, nil
}

// CompositeGeopoliticalProvider merges scores from multiple geopolitical providers.
type CompositeGeopoliticalProvider struct {
	providers []GeopoliticalRiskProvider
}

// NewCompositeGeopoliticalProvider creates a composite provider.
func NewCompositeGeopoliticalProvider(providers ...GeopoliticalRiskProvider) *CompositeGeopoliticalProvider {
	return &CompositeGeopoliticalProvider{providers: providers}
}

// Name returns the provider name.
func (c *CompositeGeopoliticalProvider) Name() string {
	return "composite_geopolitical"
}

// FetchScore averages intensity across all providers.
func (c *CompositeGeopoliticalProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	var totalIntensity float64
	var totalConfidence float64
	var sources []string
	var count int

	for _, p := range c.providers {
		score, err := p.FetchScore(ctx)
		if err != nil {
			log.Printf("[CompositeGeopoliticalProvider] provider %s failed: %v", p.Name(), err)
			continue
		}
		totalIntensity += score.Intensity
		totalConfidence += score.Confidence
		sources = append(sources, p.Name())
		count++
	}

	if count == 0 {
		return GeopoliticalRiskScore{}, fmt.Errorf("all geopolitical providers failed")
	}

	return GeopoliticalRiskScore{
		Region:         "Middle East",
		Intensity:      totalIntensity / float64(count),
		Sentiment:      -0.65,
		Confidence:     totalConfidence / float64(count),
		OilImpact:      0.55,
		ShippingImpact: 0.45,
		Sources:        sources,
		Timestamp:      time.Now().UTC(),
	}, nil
}
