package marketdata

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteComplete 判定測試（manifest Phase B1）：無資料與 closePrice-only
// 殘缺都視為 incomplete；完整 OHLC 才算 complete。
func TestQuoteComplete(t *testing.T) {
	cases := []struct {
		name string
		q    domain.Quote
		want bool
	}{
		{"all-zero → incomplete", domain.Quote{}, false},
		{"closePrice-only 殘缺 (Last>0, OHLC=0)", domain.Quote{Last: 2395}, false},
		{"Last>0 OHLC=0 Volume>0 殘缺", domain.Quote{Last: 2395, Volume: 11002}, false},
		{"OHLC 齊全但 Last=0", domain.Quote{Open: 2390, High: 2410, Low: 2385, Volume: 11002}, false},
		{"完整 OHLC", domain.Quote{Last: 2395, Open: 2390, High: 2410, Low: 2385, Volume: 11002}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := QuoteComplete(tc.q); got != tc.want {
				t.Errorf("QuoteComplete(%+v) = %v, want %v", tc.q, got, tc.want)
			}
		})
	}
}
