package monitoring

// universe_quote_volume_unit_test.go pins the SHARE contract of the Layer-2
// turnover floor (issue #1987).
//
// domain.Quote.Volume is ALWAYS 成交股數 (shares): providers normalise at the
// boundary (Fugle/Fubon intraday report 成交張數 and multiply by
// domain.SharesPerLot before constructing the Quote — pinned per provider in
// internal/marketdata/provider_volume_contract_test.go). The turnover floor
// (Volume × Last) therefore reads Volume at face value and does NOT inspect
// Source: a quote that still arrives lot-denominated is a provider bug and
// must FAIL the floor (visible), never be silently re-converted (invisible).
//
// The consumer-side translation table that lived here before #1987
// (quoteVolumeLotSources / quoteVolumeInShares, added in #1979) was removed:
// after the boundary fix it double-converted every Fugle/Fubon quote.

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func unitTestQuote(sym, source string, last float64, volume int64) domain.Quote {
	return domain.Quote{Symbol: sym, Source: source, Last: last, Volume: volume, AsOf: time.Now()}
}

// TestTurnoverFloor_ShareQuoteAboveFloorPasses: a normalised quote whose real
// turnover clears NT$10M must rank, regardless of which provider produced it.
func TestTurnoverFloor_ShareQuoteAboveFloorPasses(t *testing.T) {
	// 60,000 shares × NT$200 = NT$12,000,000 > NT$10M floor. Every Source —
	// including the formerly lot-denominated ones — is read as shares.
	for _, source := range []string{"twse", "finmind", "fugle", "fubon", "fugle-ws", ""} {
		quotes := map[string]domain.Quote{"2330": unitTestQuote("2330", source, 200, 60_000)}
		ss := productionScaleScreener()
		ss.TopN = 10
		if ranked := ss.Rank([]string{"2330"}, quotes); len(ranked) != 1 {
			t.Errorf("source %q: ranked %d symbols, want 1 (NT$12M turnover must clear the NT$10M floor)", source, len(ranked))
		}
	}
}

// TestTurnoverFloor_ShareQuoteBelowFloorDrops: the floor must keep its teeth —
// 5,000 shares × NT$200 = NT$1M stays below NT$10M for every provider.
func TestTurnoverFloor_ShareQuoteBelowFloorDrops(t *testing.T) {
	for _, source := range []string{"twse", "fugle", "fubon"} {
		quotes := map[string]domain.Quote{"1101": unitTestQuote("1101", source, 200, 5_000)}
		ss := productionScaleScreener()
		ss.TopN = 10
		if ranked := ss.Rank([]string{"1101"}, quotes); len(ranked) != 0 {
			t.Errorf("source %q: ranked %d symbols, want 0 (5,000 shares × NT$200 = NT$1M < NT$10M)", source, len(ranked))
		}
	}
}

// TestTurnoverFloor_DoesNotReconvertBySource pins the removal of the
// consumer-side lots→shares translation: a quote that still carries a LOT-scale
// volume (e.g. a provider that forgot the boundary conversion) must NOT be
// silently multiplied by 1000. Before #1987 a Source of "fugle"/"fubon" turned
// Volume=60 into 60,000 shares and the quote passed; now it must fail the
// floor so the provider defect stays visible in the scoring_filters log.
func TestTurnoverFloor_DoesNotReconvertBySource(t *testing.T) {
	// 60 張 of a NT$200 stock would be 60,000 shares = NT$12M if anything still
	// re-converted; read at face value it is NT$12,000 and must be dropped.
	for _, source := range []string{"fugle", "fubon", "fugle-ws"} {
		quotes := map[string]domain.Quote{"2330": unitTestQuote("2330", source, 200, 60)}
		ss := productionScaleScreener()
		ss.TopN = 10
		if ranked := ss.Rank([]string{"2330"}, quotes); len(ranked) != 0 {
			t.Errorf("source %q with lot-scale Volume=60 ranked %d symbols, want 0: "+
				"the filter must read Volume as shares at face value, not re-convert by Source", source, len(ranked))
		}
	}
}
