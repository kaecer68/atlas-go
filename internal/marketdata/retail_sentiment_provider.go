package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RetailSentimentProvider fetches retail investor sentiment data.
type RetailSentimentProvider interface {
	Name() string
	FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error)
}

// TWSERetailSentimentProvider uses real TWSE margin balance data.
type TWSERetailSentimentProvider struct {
	marginProvider *TWSEBalanceProvider
	storageDir     string
}

// NewTWSERetailSentimentProvider creates a provider that reads from TWSE margin data.
func NewTWSERetailSentimentProvider(storageDir string) *TWSERetailSentimentProvider {
	return &TWSERetailSentimentProvider{
		marginProvider: NewTWSEBalanceProvider(storageDir),
		storageDir:     storageDir,
	}
}

func (p *TWSERetailSentimentProvider) Name() string {
	return "twse_retail_sentiment"
}

// FetchSnapshot retrieves the latest retail sentiment from TWSE margin data.
func (p *TWSERetailSentimentProvider) FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error) {
	// Fetch latest margin balance from TWSE.
	latest, err := p.marginProvider.fetchLatestTradingDay(ctx)
	if err != nil {
		return domain.RetailSentimentSnapshot{}, fmt.Errorf("fetch margin balance: %w", err)
	}

	// Load historical data for change percentage and percentile.
	history := p.loadMarginHistory()

	marginBalance := latest.MarginBalance
	marginChangePct := 0.0
	marginPercentile := 0.5

	if len(history) >= 2 {
		// Sort by date ascending.
		sort.Slice(history, func(i, j int) bool {
			return history[i].Date < history[j].Date
		})

		// Find previous trading day (most recent before latest).
		var prevBalance float64
		for i := len(history) - 2; i >= 0; i-- {
			if history[i].Date != latest.Date {
				prevBalance = history[i].MarginBalance
				break
			}
		}

		if prevBalance > 0 {
			marginChangePct = (marginBalance - prevBalance) / prevBalance * 100
		}

		// Calculate percentile from available history (simplified).
		marginPercentile = p.calculatePercentile(marginBalance, history)
	}

	return domain.RetailSentimentSnapshot{
		MarginBalance:    marginBalance,
		MarginChangePct:  marginChangePct,
		MarginPercentile: marginPercentile,
		Timestamp:        time.Now(),
	}, nil
}

// loadMarginHistory reads all stored margin balance files.
func (p *TWSERetailSentimentProvider) loadMarginHistory() []TWSEBalance {
	entries, err := os.ReadDir(p.storageDir)
	if err != nil {
		return nil
	}

	var history []TWSEBalance
	for _, entry := range entries {
		if entry.IsDir() || !isMarginFile(entry.Name()) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(p.storageDir, entry.Name()))
		if err != nil {
			continue
		}

		var bal TWSEBalance
		if err := json.Unmarshal(data, &bal); err != nil {
			continue
		}
		history = append(history, bal)
	}
	return history
}

// isMarginFile checks if filename matches margin balance storage pattern.
func isMarginFile(name string) bool {
	return len(name) == 16 && name[8:] == "_margin.json"
}

// calculatePercentile computes the percentile of current balance in historical data.
func (p *TWSERetailSentimentProvider) calculatePercentile(current float64, history []TWSEBalance) float64 {
	if len(history) == 0 {
		return 0.5
	}

	var values []float64
	for _, h := range history {
		if h.MarginBalance > 0 {
			values = append(values, h.MarginBalance)
		}
	}

	if len(values) == 0 {
		return 0.5
	}

	sort.Float64s(values)

	// Find rank of current value.
	count := 0
	for _, v := range values {
		if v <= current {
			count++
		}
	}

	return float64(count) / float64(len(values))
}

// parseMarginDate extracts date from margin filename (e.g. "20260503_margin.json" → "20260503").
func parseMarginDate(filename string) string {
	if len(filename) >= 8 {
		return filename[:8]
	}
	return ""
}

// parseDateToTime converts "20060102" string to time.Time.
func parseDateToTime(dateStr string) time.Time {
	t, _ := time.Parse("20060102", dateStr)
	return t
}

// formatDate converts time.Time to "20060102" string.
func formatDate(t time.Time) string {
	return t.Format("20060102")
}

// CompositeRetailSentimentProvider tries multiple providers in order.
type CompositeRetailSentimentProvider struct {
	providers []RetailSentimentProvider
}

// NewCompositeRetailSentimentProvider creates a composite provider.
func NewCompositeRetailSentimentProvider(providers ...RetailSentimentProvider) *CompositeRetailSentimentProvider {
	return &CompositeRetailSentimentProvider{providers: providers}
}

func (p *CompositeRetailSentimentProvider) Name() string {
	return "composite_retail_sentiment"
}

func (p *CompositeRetailSentimentProvider) FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error) {
	for _, provider := range p.providers {
		snap, err := provider.FetchSnapshot(ctx)
		if err == nil {
			return snap, nil
		}
	}
	return domain.RetailSentimentSnapshot{}, fmt.Errorf("all retail sentiment providers failed")
}
