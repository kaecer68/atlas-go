package marketdata

import (
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func mockQuote(symbol string, asOf time.Time, source string) domain.Quote {
	base := 50.0
	switch {
	case strings.HasPrefix(symbol, "2330"):
		base = 785
	case strings.HasPrefix(symbol, "2317"):
		base = 162
	case strings.HasPrefix(symbol, "2382"):
		base = 268
	case strings.HasPrefix(symbol, "0050"):
		base = 192
	case strings.HasPrefix(symbol, "2603"):
		base = 215
	}

	return domain.Quote{
		Symbol:     symbol,
		Last:       base,
		Open:       base * 0.995,
		High:       base * 1.01,
		Low:        base * 0.99,
		Volume:     15000000,
		Market:     "TW",
		AsOf:       asOf,
		IsTradable: true,
		Source:     source,
	}
}
