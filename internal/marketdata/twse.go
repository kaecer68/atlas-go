package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type MockProvider struct{}

func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (p *MockProvider) Name() string {
	return "mock"
}

func (p *MockProvider) GetQuotes(_ context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes := make([]domain.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quotes = append(quotes, mockQuote(symbol, asOf, "mock"))
	}
	return quotes, nil
}

func (p *MockProvider) IsMock() bool {
	return true
}
