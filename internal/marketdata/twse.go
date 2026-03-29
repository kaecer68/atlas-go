package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type TWSEProvider struct{}

func NewTWSEProvider() *TWSEProvider {
	return &TWSEProvider{}
}

func (p *TWSEProvider) Name() string {
	return "twse"
}

func (p *TWSEProvider) GetQuotes(_ context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes := make([]domain.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quotes = append(quotes, mockQuote(symbol, asOf, "twse"))
	}
	return quotes, nil
}
