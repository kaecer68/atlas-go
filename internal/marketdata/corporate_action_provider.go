package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CorporateActionProvider returns corporate actions (cash dividends, stock
// dividends, capital reductions) for a single symbol within a date range.
//
// Implementations SHOULD aggregate from multiple sources (TWSE OpenAPI +
// FinMind) and return results sorted by ExDate ascending. See
// AggregatedCorporateActionProvider for the canonical reference implementation.
//
// Consumers:
//   - portfolio.AdjustForCorporateActions (P1-2-β)
//   - FactorEngine (P1-2-γ)
//   - Any future audit / replay path that needs ex-corporate-action prices.
//
// Date ranges are inclusive on both ends. Callers are expected to pass CST
// (UTC+8) time.Time values; providers SHOULD treat the date portion as
// authoritative and ignore sub-day clock skew.
type CorporateActionProvider interface {
	GetCorporateActions(ctx context.Context, symbol string, start, end time.Time) ([]domain.CorporateAction, error)
}
