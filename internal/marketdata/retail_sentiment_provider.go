package marketdata

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RetailSentimentProvider fetches retail investor sentiment data.
type RetailSentimentProvider interface {
	Name() string
	FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error)
}

// TWSERetailSentimentProvider fetches retail data from TWSE APIs.
type TWSERetailSentimentProvider struct {
	storageDir               string
	marginHistory            []float64
	fetchMarginBalanceFunc   func(ctx context.Context) (float64, error)
	fetchDayTradingRatioFunc func(ctx context.Context) (float64, error)
}

// NewTWSERetailSentimentProvider creates a new provider.
func NewTWSERetailSentimentProvider(storageDir string) *TWSERetailSentimentProvider {
	p := &TWSERetailSentimentProvider{
		storageDir: storageDir,
	}
	p.fetchMarginBalanceFunc = p.fetchMarginBalance
	p.fetchDayTradingRatioFunc = p.fetchDayTradingRatio
	return p
}

// Name returns the provider name.
func (t *TWSERetailSentimentProvider) Name() string {
	return "twse_retail_sentiment"
}

// FetchSnapshot retrieves the latest retail sentiment snapshot.
func (t *TWSERetailSentimentProvider) FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error) {
	margin, err := t.fetchMarginBalanceFunc(ctx)
	if err != nil {
		return domain.RetailSentimentSnapshot{}, fmt.Errorf("fetch margin balance: %w", err)
	}

	dtRatio, err := t.fetchDayTradingRatioFunc(ctx)
	if err != nil {
		dtRatio = 0
	}

	percentile := t.calculatePercentile(margin, t.marginHistory)

	snap := domain.RetailSentimentSnapshot{
		MarginBalance:    int64(margin),
		DayTradingRatio:  dtRatio,
		MarginPercentile: percentile,
		Timestamp:        time.Now().UTC(),
	}
	snap.CalculateSentimentScore()

	return snap, nil
}

func (t *TWSERetailSentimentProvider) calculatePercentile(value float64, history []float64) float64 {
	if len(history) == 0 {
		return 0.5
	}

	sorted := make([]float64, len(history))
	copy(sorted, history)
	sort.Float64s(sorted)

	count := 0
	for _, v := range sorted {
		if v <= value {
			count++
		}
	}

	return float64(count) / float64(len(sorted))
}

func (t *TWSERetailSentimentProvider) fetchMarginBalance(ctx context.Context) (float64, error) {
	return 0, fmt.Errorf("not implemented: TWSE T13 API")
}

func (t *TWSERetailSentimentProvider) fetchDayTradingRatio(ctx context.Context) (float64, error) {
	return 0, fmt.Errorf("not implemented: TWSE T23 API")
}

// RetailSentimentMacroAdapter adapts RetailSentimentProvider to MacroDataProvider.
type RetailSentimentMacroAdapter struct {
	provider RetailSentimentProvider
}

// NewRetailSentimentMacroAdapter creates an adapter.
func NewRetailSentimentMacroAdapter(provider RetailSentimentProvider) *RetailSentimentMacroAdapter {
	return &RetailSentimentMacroAdapter{provider: provider}
}

// Name returns the adapter name.
func (a *RetailSentimentMacroAdapter) Name() string {
	return a.provider.Name() + "_macro"
}

// FetchSnapshot converts retail sentiment data into MacroDataSnapshot format.
func (a *RetailSentimentMacroAdapter) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	rs, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return MacroDataSnapshot{}, err
	}

	return MacroDataSnapshot{
		RecordedAt: rs.Timestamp.Unix(),
		RetailSentiment: MacroDataPoint{
			Symbol:    "RETAIL_SENTIMENT",
			Value:     rs.SentimentScore,
			Timestamp: rs.Timestamp.Unix(),
		},
	}, nil
}
