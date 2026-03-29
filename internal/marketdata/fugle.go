package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type FugleProvider struct{}

func NewFugleProvider() *FugleProvider {
	return &FugleProvider{}
}

func (p *FugleProvider) Name() string {
	return "fugle"
}

func (p *FugleProvider) GetQuotes(_ context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes := make([]domain.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quote := mockQuote(symbol, asOf, "fugle")
		quote.Last *= 1.002
		quotes = append(quotes, quote)
	}
	return quotes, nil
}
