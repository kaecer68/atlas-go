package ledger

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteStore handles market quote persistence.
type QuoteStore interface {
	// RecordQuotes persists a batch of daily bars for a symbol.
	RecordQuotes(quotes []domain.DailyBar) error
	// LoadQuotes retrieves daily bars for a symbol within a time window.
	LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error)
	// LoadLatestQuotes retrieves the most recent bar for each symbol.
	LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error)
}

// QuoteSymbolLister is an optional interface for QuoteStore implementations
// that can enumerate the distinct symbols currently stored. It is intentionally
// separate from QuoteStore so existing implementations and callers are not
// broken by the addition of a listing method.
type QuoteSymbolLister interface {
	// QuoteSymbols returns the distinct raw symbols (e.g. "2330.TW") stored in
	// the quotes table, ordered ascending.
	QuoteSymbols(ctx context.Context) ([]string, error)
}
