package marketdata

import (
	"context"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
	marketdatapkg "github.com/kaecer68/atlas-go/internal/marketdata"
)

type retailSentimentProvider interface {
	FetchSnapshot(ctx context.Context) (domain.RetailSentimentSnapshot, error)
}

func newRetailSentimentProvider(workDir string) retailSentimentProvider {
	return marketdatapkg.NewTWSERetailSentimentProvider(filepath.Join(workDir, "data/state/margin"))
}
