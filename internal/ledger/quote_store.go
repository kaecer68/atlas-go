package ledger

import (
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
