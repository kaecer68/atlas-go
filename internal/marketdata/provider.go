package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Provider interface {
	Name() string
	GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}
