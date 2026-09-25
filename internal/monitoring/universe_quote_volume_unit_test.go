package monitoring

// universe_quote_volume_unit_test.go pins the provider-unit translation applied
// to the Layer-2 turnover floor (issue #1944 Batch 3, I25 production-scale
// follow-up).
//
// domain.Quote.Volume does not have one unit across providers: TWSE (first
// party) and FinMind report 成交股數 (shares) while the Fugle market-data REST
// quote — and fubon-neo, which embeds the same client — reports 成交張數 (lots).
// The turnover floor is Volume × Last, a TWD magnitude, so a lot-denominated
// quote silently understates turnover 1000×. That is the same class of silent
// failure as I25 itself, so it is translated at the one place the magnitude is
// derived and the conversion is counted and logged.

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func unitTestQuote(sym, source string, last float64, volume int64) domain.Quote {
	return domain.Quote{Symbol: sym, Source: source, Last: last, Volume: volume, AsOf: time.Now()}
}

// TestQuoteVolumeInShares_TranslatesLotsProviders pins the recognised sources.
func TestQuoteVolumeInShares_TranslatesLotsProviders(t *testing.T) {
	cases := []struct {
		source      string
		volume      int64
		wantShares  int64
		wantConvert bool
	}{
		{"fugle", 20, 20_000, true},
		{"fugle_candles", 3, 3_000, true},
		{"fubon", 15, 15_000, true},
		{"FUBON", 15, 15_000, true}, // Source is matched case-insensitively
		{"twse", 20_000, 20_000, false},
		{"finmind", 20_000, 20_000, false},
		{"", 20_000, 20_000, false}, // unknown source keeps the historical behaviour
		{"somefuture-provider", 20_000, 20_000, false},
	}
	for _, c := range cases {
		got, converted := quoteVolumeInShares(unitTestQuote("2330", c.source, 100, c.volume))
		if got != c.wantShares || converted != c.wantConvert {
			t.Errorf("source %q volume %d: got (%d, %v), want (%d, %v)",
				c.source, c.volume, got, converted, c.wantShares, c.wantConvert)
		}
	}
}

// TestTurnoverFloor_IsUnitInvariant is the payoff: the same real trade must be
// classified the same way whether the provider expressed it in lots or shares.
func TestTurnoverFloor_IsUnitInvariant(t *testing.T) {
	// 60 張 of a NT$200 stock = 60,000 shares = NT$12,000,000 turnover: above the
	// NT$10M floor.
	asLots := map[string]domain.Quote{"2330": unitTestQuote("2330", "fugle", 200, 60)}
	asShares := map[string]domain.Quote{"2330": unitTestQuote("2330", "twse", 200, 60_000)}

	ss := productionScaleScreener()
	ss.TopN = 10
	rankedLots := ss.Rank([]string{"2330"}, asLots)
	rankedShares := ss.Rank([]string{"2330"}, asShares)

	if len(rankedLots) != 1 {
		t.Fatalf("lot-denominated quote ranked %d symbols, want 1 (NT$12M turnover must clear the NT$10M floor)", len(rankedLots))
	}
	if len(rankedShares) != 1 {
		t.Fatalf("share-denominated quote ranked %d symbols, want 1", len(rankedShares))
	}
}

// TestTurnoverFloor_LotsQuoteBelowFloorStillDrops proves the translation does
// not turn the floor into a no-op: 5 張 × NT$200 = NT$1M stays below it.
func TestTurnoverFloor_LotsQuoteBelowFloorStillDrops(t *testing.T) {
	quotes := map[string]domain.Quote{"1101": unitTestQuote("1101", "fubon", 200, 5)}
	ss := productionScaleScreener()
	ss.TopN = 10
	if ranked := ss.Rank([]string{"1101"}, quotes); len(ranked) != 0 {
		t.Fatalf("ranked %d symbols, want 0 (5 張 × NT$200 = NT$1M < NT$10M)", len(ranked))
	}
}

// TestFilterStatsLotsConvertedIsRecorded keeps the conversion visible: an
// operator reading the production log must be able to tell that a run's turnover
// comparison involved lot-denominated quotes.
func TestFilterStatsLotsConvertedIsRecorded(t *testing.T) {
	quotes := map[string]domain.Quote{
		"2330": unitTestQuote("2330", "fugle", 200, 60),
		"2317": unitTestQuote("2317", "twse", 200, 60_000),
	}
	ss := productionScaleScreener()
	_, stats := ss.applyVolumeAndPriceFilters([]string{"2330", "2317"}, quotes)
	if stats.LotsConverted != 1 {
		t.Fatalf("LotsConverted = %d, want 1", stats.LotsConverted)
	}
	if stats.Passed != 2 {
		t.Fatalf("Passed = %d, want 2", stats.Passed)
	}
}

// TestQuoteVolumeUnitLotSourcesMatchProviderSources pins that every Source in
// the translation table is a value some provider actually sets, so the table
// cannot silently rot into a no-op reference.
func TestQuoteVolumeUnitLotSourcesMatchProviderSources(t *testing.T) {
	for source := range quoteVolumeLotSources {
		switch source {
		case "fugle", "fugle_candles", "fubon":
		default:
			t.Errorf("quoteVolumeLotSources contains %q, which no provider sets (see internal/marketdata/*_client.go Source: fields)", source)
		}
	}
	if len(quoteVolumeLotSources) != 3 {
		t.Errorf("quoteVolumeLotSources has %d entries, want 3", len(quoteVolumeLotSources))
	}
}
