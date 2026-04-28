package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type FubonProvider struct {
	client *FubonClient
}

func NewFubonProviderWithClient(client *FubonClient) *FubonProvider {
	return &FubonProvider{client: client}
}

func (p *FubonProvider) Name() string {
	return "fubon"
}

func (p *FubonProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	return p.client.GetQuotes(ctx, symbols)
}

func (p *FubonProvider) GetClient() *FubonClient {
	return p.client
}