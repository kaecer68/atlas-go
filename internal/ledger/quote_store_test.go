package ledger

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Compile-time interface assertion for QuoteStore contract.
// An implementation must provide RecordQuotes, LoadQuotes, and LoadLatestQuotes.
var _ QuoteStore = (*mockQuoteStoreForCompileCheck)(nil)

type mockQuoteStoreForCompileCheck struct{}

func (m *mockQuoteStoreForCompileCheck) RecordQuotes(quotes []domain.DailyBar) error {
	return nil
}

func (m *mockQuoteStoreForCompileCheck) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	return nil, nil
}

func (m *mockQuoteStoreForCompileCheck) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	return nil, nil
}
