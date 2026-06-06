package marketdata

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type mockQuoteFetcher struct {
	mu     sync.Mutex
	quotes map[string]domain.Quote
	calls  int
}

func (m *mockQuoteFetcher) GetQuotes(_ context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	result := make([]domain.Quote, 0, len(symbols))
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result = append(result, q)
		}
	}
	return result, nil
}

func (m *mockQuoteFetcher) setQuote(symbol string, price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.quotes == nil {
		m.quotes = make(map[string]domain.Quote)
	}
	m.quotes[symbol] = domain.Quote{Symbol: symbol, Last: price, Market: "TW"}
}

func TestETFNAVProvider_FetchNAV_Cached(t *testing.T) {
	fetcher := &mockQuoteFetcher{}
	fetcher.setQuote("0050.TW", 195.50)

	provider := NewETFNAVProvider(fetcher)
	ctx := context.Background()

	nav, err := provider.FetchNAV(ctx, "0050.TW")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if nav != 195.50 {
		t.Fatalf("expected 195.50, got %.2f", nav)
	}

	// Second call should use cache (no additional API call)
	nav2, err := provider.FetchNAV(ctx, "0050.TW")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if nav2 != 195.50 {
		t.Fatalf("cached value mismatch: %.2f", nav2)
	}

	fetcher.mu.Lock()
	calls := fetcher.calls
	fetcher.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected 1 API call, got %d (cache not used)", calls)
	}
}

func TestETFNAVProvider_FetchNAV_NotFound(t *testing.T) {
	fetcher := &mockQuoteFetcher{}
	provider := NewETFNAVProvider(fetcher)
	ctx := context.Background()

	_, err := provider.FetchNAV(ctx, "9999.TW")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
}

func TestETFNAVProvider_FetchNAVBatch(t *testing.T) {
	fetcher := &mockQuoteFetcher{}
	fetcher.setQuote("0050.TW", 195.50)
	fetcher.setQuote("0056.TW", 42.80)
	fetcher.setQuote("00878.TW", 25.30)

	provider := NewETFNAVProvider(fetcher)
	ctx := context.Background()

	result, err := provider.FetchNAVBatch(ctx, []string{"0050.TW", "0056.TW", "00878.TW"})
	if err != nil {
		t.Fatalf("batch fetch: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result["0050.TW"] != 195.50 {
		t.Errorf("0050.TW: expected 195.50, got %.2f", result["0050.TW"])
	}
	if result["0056.TW"] != 42.80 {
		t.Errorf("0056.TW: expected 42.80, got %.2f", result["0056.TW"])
	}
	if result["00878.TW"] != 25.30 {
		t.Errorf("00878.TW: expected 25.30, got %.2f", result["00878.TW"])
	}
}

func TestETFNAVProvider_CacheExpiry(t *testing.T) {
	fetcher := &mockQuoteFetcher{}
	fetcher.setQuote("0050.TW", 195.50)

	provider := NewETFNAVProvider(fetcher)
	provider.SetCacheTTL(1 * time.Millisecond)
	ctx := context.Background()

	_, _ = provider.FetchNAV(ctx, "0050.TW")

	time.Sleep(5 * time.Millisecond)

	fetcher.setQuote("0050.TW", 196.00) // updated quote
	nav, err := provider.FetchNAV(ctx, "0050.TW")
	if err != nil {
		t.Fatalf("fetch after expiry: %v", err)
	}
	if nav != 196.00 {
		t.Fatalf("expected refreshed 196.00, got %.2f", nav)
	}
}

func TestETFNAVProvider_ClearCache(t *testing.T) {
	fetcher := &mockQuoteFetcher{}
	fetcher.setQuote("0050.TW", 195.50)

	provider := NewETFNAVProvider(fetcher)
	ctx := context.Background()

	_, _ = provider.FetchNAV(ctx, "0050.TW")
	provider.ClearCache()

	fetcher.setQuote("0050.TW", 197.00)
	nav, err := provider.FetchNAV(ctx, "0050.TW")
	if err != nil {
		t.Fatalf("fetch after clear: %v", err)
	}
	if nav != 197.00 {
		t.Fatalf("expected 197.00 after cache clear, got %.2f", nav)
	}
}
