package realtime

// fugle_ws_volume_contract_test.go — the #1987 single-unit contract on the
// WebSocket boundary: domain.Quote.Volume is ALWAYS 成交股數 (shares). The
// trades channel's volume (成交總量) follows Fugle's intraday 整股 convention
// of 成交張數 (lots), so tradeToQuote multiplies by domain.SharesPerLot.
// The conversion is a documented inference — the WS docs do not state the unit
// explicitly — so this test pins the CONVERSION, and the first production
// wiring of this provider must verify it against TWSE 成交股數.

import (
	"testing"
)

func TestContractFugleWSTradeToQuoteVolumeIsShares(t *testing.T) {
	p := &FugleWebSocketProvider{}
	q := p.tradeToQuote(fugleWSTradeData{
		Symbol: "2330",
		Price:  1000,
		Open:   990,
		High:   1005,
		Low:    985,
		Volume: 54_538, // 成交總量 in lots (Fugle intraday convention)
	})
	if q.Volume != 54_538_000 {
		t.Errorf("Quote.Volume = %d, want 54538000 (54,538 lots x domain.SharesPerLot)", q.Volume)
	}
	if q.Source != "fugle-ws" {
		t.Errorf("Source = %q, want fugle-ws", q.Source)
	}
}
