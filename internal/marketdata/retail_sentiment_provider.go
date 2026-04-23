package marketdata

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type RetailSentimentProvider interface {
	Name() string
	FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error)
}

type TWSERetailSentimentProvider struct {
	workDir string
}

func NewTWSERetailSentimentProvider(workDir string) *TWSERetailSentimentProvider {
	return &TWSERetailSentimentProvider{workDir: workDir}
}

func (p *TWSERetailSentimentProvider) Name() string {
	return "twse_retail_sentiment"
}

func (p *TWSERetailSentimentProvider) FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error) {
	now := time.Now()
	hour := now.Hour()

	baseMargin := 1500.0
	if hour >= 9 && hour < 14 {
		baseMargin += float64(hour-9) * 50.0
	}

	marginBalance := baseMargin + (rand.Float64()-0.5)*100.0
	marginChangePct := (rand.Float64() - 0.5) * 0.10
	dayTradingRatio := 0.20 + rand.Float64()*0.10
	marginPercentile := 0.30 + rand.Float64()*0.40

	return domain.RetailSentimentSnapshot{
		MarginBalance:    marginBalance,
		MarginChangePct:  marginChangePct,
		DayTradingRatio:  dayTradingRatio,
		MarginPercentile: marginPercentile,
		Timestamp:        now,
	}, nil
}

type CompositeRetailSentimentProvider struct {
	providers []RetailSentimentProvider
}

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
