package geopolitical

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestTaiwanRSSGeopoliticalProvider_Name(t *testing.T) {
	p := NewTaiwanRSSGeopoliticalProvider()
	if p.Name() != "taiwan_rss_geopolitical" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestTaiwanRSSGeopoliticalProvider_Feeds(t *testing.T) {
	p := NewTaiwanRSSGeopoliticalProvider()
	if len(p.feeds) == 0 {
		t.Fatalf("feeds should not be empty")
	}

	// Verify expected financial news feeds are present
	expectedFeeds := []string{
		"https://www.digitimes.com/rss/daily.xml",
		"https://money.udn.com/rssfeed/lists/1001",
		"https://news.ustv.com.tw/feed",
		"https://wwwc.twse.com.tw/rwd/zh/news/feed?type=rss",
	}

	found := 0
	for _, feed := range p.feeds {
		if slices.Contains(expectedFeeds, feed) {
			found++
		}
	}

	if found < 2 {
		t.Fatalf("expected at least 2 known feeds, found %d", found)
	}
}

func TestTaiwanRSSGeopoliticalProvider_Keywords(t *testing.T) {
	p := NewTaiwanRSSGeopoliticalProvider()
	if len(p.keywords) == 0 {
		t.Fatalf("keywords should not be empty")
	}

	// Verify Chinese keywords
	chineseKeywords := []string{"台灣", "中國", "兩岸", "軍演", "制裁"}
	for _, kw := range chineseKeywords {
		found := slices.Contains(p.keywords, kw)
		if !found {
			t.Fatalf("expected keyword %q not found", kw)
		}
	}

	// Verify English keywords
	englishKeywords := []string{"taiwan", "china", "cross-strait", "military drill"}
	for _, kw := range englishKeywords {
		found := slices.Contains(p.keywords, kw)
		if !found {
			t.Fatalf("expected keyword %q not found", kw)
		}
	}
}

func TestTaiwanRSSGeopoliticalProvider_FetchScore(t *testing.T) {
	p := NewTaiwanRSSGeopoliticalProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	score, err := p.FetchScore(ctx)
	if err != nil {
		t.Fatalf("FetchScore failed: %v", err)
	}

	if score.Region != "Taiwan" {
		t.Fatalf("unexpected region: %s", score.Region)
	}

	// Intensity should be between 0-100
	if score.Intensity < 0 || score.Intensity > 100 {
		t.Fatalf("intensity out of range: %v", score.Intensity)
	}

	// Confidence should be between 0-1
	if score.Confidence < 0 || score.Confidence > 1 {
		t.Fatalf("confidence out of range: %v", score.Confidence)
	}

	// Sentiment should be between -1 and 1
	if score.Sentiment < -1 || score.Sentiment > 1 {
		t.Fatalf("sentiment out of range: %v", score.Sentiment)
	}

	// Oil impact should be low for Taiwan
	if score.OilImpact > 0.5 {
		t.Fatalf("oil impact too high for Taiwan: %v", score.OilImpact)
	}
}

func TestCompositeTaiwanGeopoliticalProvider_Name(t *testing.T) {
	p := NewCompositeTaiwanGeopoliticalProvider()
	if p.Name() != "composite_taiwan_geopolitical" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestCompositeTaiwanGeopoliticalProvider_FetchScore(t *testing.T) {
	p := NewCompositeTaiwanGeopoliticalProvider(
		NewTaiwanRSSGeopoliticalProvider(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	score, err := p.FetchScore(ctx)
	if err != nil {
		t.Fatalf("FetchScore failed: %v", err)
	}

	if score.Region != "Taiwan" {
		t.Fatalf("unexpected region: %s", score.Region)
	}

	if score.Intensity < 0 || score.Intensity > 100 {
		t.Fatalf("intensity out of range: %v", score.Intensity)
	}
}

// Verifies SetHTTPClient is safe under concurrent FetchScore (regression test for #534).
func TestTaiwanRSSGeopoliticalProvider_SetHTTPClient_Race(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Empty</title></channel></rss>`)
	}))
	defer server.Close()

	clientA := server.Client()
	clientB := &http.Client{Timeout: 5 * time.Second}
	p := NewTaiwanRSSGeopoliticalProvider()

	const N = 50
	var wg sync.WaitGroup

	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				p.SetHTTPClient(clientA)
			} else {
				p.SetHTTPClient(clientB)
			}
		}(i)
	}

	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = p.FetchScore(ctx)
		}()
	}

	wg.Wait()
}
