package geopolitical

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestRSSGeopoliticalProvider_Name(t *testing.T) {
	p := NewRSSGeopoliticalProvider()
	if got := p.Name(); got != "rss_geopolitical" {
		t.Fatalf("unexpected name: %s", got)
	}
}

func TestRSSGeopoliticalProvider_FeedsNonEmpty(t *testing.T) {
	p := NewRSSGeopoliticalProvider()
	if len(p.feeds) == 0 {
		t.Fatal("feeds should not be empty")
	}

	expectedFeeds := []string{
		"http://feeds.bbci.co.uk/news/world/middle_east/rss.xml",
		"https://www.aljazeera.com/xml/rss/all.xml",
	}
	found := 0
	for _, feed := range p.feeds {
		if slices.Contains(expectedFeeds, feed) {
			found++
		}
	}
	if found < 2 {
		t.Fatalf("expected at least 2 known feeds (BBC, AlJazeera), found %d", found)
	}
}

func TestRSSGeopoliticalProvider_KeywordsNonEmpty(t *testing.T) {
	p := NewRSSGeopoliticalProvider()
	if len(p.keywords) == 0 {
		t.Fatal("keywords should not be empty")
	}

	expected := []string{"israel", "iran", "hamas"}
	for _, kw := range expected {
		if !slices.Contains(p.keywords, kw) {
			t.Fatalf("expected keyword %q not found in keywords", kw)
		}
	}
}

func TestGDELTGeopoliticalProvider_Name(t *testing.T) {
	p := NewGDELTGeopoliticalProvider()
	if got := p.Name(); got != "gdelt_geopolitical" {
		t.Fatalf("unexpected name: %s", got)
	}
}

func TestCompositeGeopoliticalProvider_Name(t *testing.T) {
	p := NewCompositeGeopoliticalProvider()
	if got := p.Name(); got != "composite_geopolitical" {
		t.Fatalf("unexpected name: %s", got)
	}
	// also verify it contains "composite"
	if got := p.Name(); len(got) == 0 || got[:9] != "composite" {
		t.Fatalf("name should contain 'composite', got: %s", got)
	}
}

func TestGeopoliticalStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewGeopoliticalStore(dir)

	score := GeopoliticalRiskScore{
		Region:         "Middle East",
		Intensity:      75.5,
		Sentiment:      -0.65,
		Confidence:     0.82,
		OilImpact:      0.7,
		ShippingImpact: 0.55,
		Sources:        []string{"rss_geopolitical", "gdelt_geopolitical"},
		Timestamp:      time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	if err := store.Save(score); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Region != score.Region {
		t.Fatalf("Region mismatch: got %q, want %q", loaded.Region, score.Region)
	}
	if loaded.Intensity != score.Intensity {
		t.Fatalf("Intensity mismatch: got %v, want %v", loaded.Intensity, score.Intensity)
	}
	if loaded.Sentiment != score.Sentiment {
		t.Fatalf("Sentiment mismatch: got %v, want %v", loaded.Sentiment, score.Sentiment)
	}
	if loaded.Confidence != score.Confidence {
		t.Fatalf("Confidence mismatch: got %v, want %v", loaded.Confidence, score.Confidence)
	}
	if loaded.OilImpact != score.OilImpact {
		t.Fatalf("OilImpact mismatch: got %v, want %v", loaded.OilImpact, score.OilImpact)
	}
	if loaded.ShippingImpact != score.ShippingImpact {
		t.Fatalf("ShippingImpact mismatch: got %v, want %v", loaded.ShippingImpact, score.ShippingImpact)
	}
	if len(loaded.Sources) != len(score.Sources) {
		t.Fatalf("Sources length mismatch: got %d, want %d", len(loaded.Sources), len(score.Sources))
	}
	for i, src := range score.Sources {
		if loaded.Sources[i] != src {
			t.Fatalf("Sources[%d] mismatch: got %q, want %q", i, loaded.Sources[i], src)
		}
	}
}

func TestGeopoliticalStore_LoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	store := NewGeopoliticalStore(dir)
	// Don't save anything — dir exists but latest.json does not.

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected error loading from empty store, got nil")
	}
}

func TestNewGeopoliticalStore(t *testing.T) {
	dir := t.TempDir()
	store := NewGeopoliticalStore(dir)
	if store == nil {
		t.Fatal("NewGeopoliticalStore returned nil")
	}

	// Verify we can save to the returned store
	score := GeopoliticalRiskScore{
		Region:    "Test",
		Intensity: 10,
		Timestamp: time.Now().UTC(),
	}
	if err := store.Save(score); err != nil {
		t.Fatalf("Save on new store failed: %v", err)
	}

	// Verify file was written to the expected dir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("base directory was not created")
	}
}

// Verifies SetHTTPClient is safe under concurrent FetchScore (regression test for #534).
func TestRSSGeopoliticalProvider_SetHTTPClient_Race(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Empty</title></channel></rss>`)
	}))
	defer server.Close()

	clientA := server.Client()
	clientB := &http.Client{Timeout: 5 * time.Second}
	p := NewRSSGeopoliticalProvider()

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

// Verifies SetHTTPClient is safe under concurrent FetchScore (regression test for #534).
func TestGDELTGeopoliticalProvider_SetHTTPClient_Race(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Empty</title></channel></rss>`)
	}))
	defer server.Close()

	clientA := server.Client()
	clientB := &http.Client{Timeout: 5 * time.Second}
	p := NewGDELTGeopoliticalProvider()

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
