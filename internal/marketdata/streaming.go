package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteHandler is a callback for receiving real-time quotes.
type QuoteHandler func(quote domain.Quote)

// StreamingProvider supports real-time quote subscription.
type StreamingProvider interface {
	Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error
	Unsubscribe(ctx context.Context, symbols []string) error
}

// PollingAdapter wraps a polling Provider to implement StreamingProvider.
type PollingAdapter struct {
	Base     Provider
	Interval int
}

func (p *PollingAdapter) Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error {
	ticker := time.NewTicker(time.Duration(p.Interval) * time.Second)
	defer ticker.Stop()

	quotes, err := p.Base.GetQuotes(ctx, time.Now(), symbols)
	if err == nil {
		for _, q := range quotes {
			handler(q)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			quotes, err := p.Base.GetQuotes(ctx, time.Now(), symbols)
			if err != nil {
				continue
			}
			for _, q := range quotes {
				handler(q)
			}
		}
	}
}

func (p *PollingAdapter) Unsubscribe(ctx context.Context, symbols []string) error {
	return nil
}
