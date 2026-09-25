package marketdata

// twse_stock_day_all_volume_test.go — the VOLUME UNIT and volume-parse evidence
// the production-scale universe audit needs (issue #1944 Batch 3 / I25 follow-up
// question 5: "quote 缺 Volume (=0) 時是否全滅？provider 的 Volume 單位是否為股？").
//
// Why it matters: monitoring.ScoringScreener.applyVolumeAndPriceFilters drops a
// symbol unless quote.Volume * quote.Last >= NT$10,000,000, so Volume=0 is
// indistinguishable from "thin stock" and wipes the whole universe. The unit is
// therefore part of the universe contract:
//
//	TWSE STOCK_DAY_ALL : 成交股數 (SHARES) — www.twse.com.tw/exchangeReport/STOCK_DAY_ALL
//	Fugle              : total.tradeVolume  (same share count, fugle_client.go:357)
//	Fubon proxy        : total.tradeVolume  (services/fubon-proxy/main.py:162)
//
// The hermetic tests below pin the TWSE side without a network call; the live
// test (env-gated, skipped by default) re-measures the real endpoint.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stockDayAllCSVHeader is the real CSV header TWSE started returning for
// STOCK_DAY_ALL (2026-06-30, measured again 2026-09-24).
const stockDayAllCSVHeader = "日期,證券代號,證券名稱,成交股數,成交金額,開盤價,最高價,最低價,收盤價,漲跌價差,成交筆數"

// newStockDayAllTestClient returns a client bound to baseURL with retries and
// rate limiting disabled, so CSV parsing can be tested without network or wait.
func newStockDayAllTestClient(baseURL string) *TWSEClient {
	return &TWSEClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}
}

// TestTWSEStockDayAll_VolumeIsShareCount pins the unit the universe pipeline
// depends on: 成交股數 is a SHARE count, so 22,871,974 means 22.87M shares
// (≈NT$358M turnover at the row's NT$15.66 close), not 22.87M lots.
func TestTWSEStockDayAll_VolumeIsShareCount(t *testing.T) {
	body := stockDayAllCSVHeader + "\n" +
		`"1150924","2330","台積電","22871974","35836000000","1550.00","1580.00","1545.00","1566.00","10.0000","45231"` + "\n"

	c := newStockDayAllTestClient("http://unused")
	quotes, err := c.parseStockCSV(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseStockCSV: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("parsed %d quotes, want 1", len(quotes))
	}
	q := quotes[0]
	if q.Volume != 22_871_974 {
		t.Errorf("Volume = %d, want 22871974 (成交股數 in shares)", q.Volume)
	}
	if q.Last != 1566 {
		t.Errorf("Last = %v, want 1566", q.Last)
	}
	// The turnover the universe volume floor computes must be the real
	// NT$35.8bn, i.e. the share interpretation.
	turnover := float64(q.Volume) * q.Last
	if turnover < 10_000_000 {
		t.Errorf("turnover = %.0f TWD, below the NT$10M universe floor for a mega-cap", turnover)
	}
	if got := turnover / 1e9; got < 35 || got > 36 {
		t.Errorf("turnover = %.2f bn TWD, want ~35.8bn (share count, not lots)", got)
	}
}

// TestTWSEStockDayAll_CommaFormattedVolumeStillParses pins the parse robustness
// of the STOCK_DAY_ALL path against the legacy JSON shape, which formats
// 成交股數 with thousands separators ("8,116,074"). convertToQuote silently
// ignored a parse failure and produced Volume=0, which the universe volume floor
// reads as "below NT$10M" — the same silent zero that I25 was about, reached
// through a different door.
func TestTWSEStockDayAll_CommaFormattedVolumeStillParses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		wantVol int64
	}{
		{"plain", "22871974", 22_871_974},
		{"comma separated", "22,871,974", 22_871_974},
		{"comma separated small", "8,116,074", 8_116_074},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := convertToQuoteForTest(TWSEQuote{
				Code:         "2330",
				ClosingPrice: "1566.00",
				OpeningPrice: "1550.00",
				HighestPrice: "1580.00",
				LowestPrice:  "1545.00",
				TradeVolume:  tc.raw,
			})
			if err != nil {
				t.Fatalf("convertToQuote(%q): %v", tc.raw, err)
			}
			if q.Volume != tc.wantVol {
				t.Fatalf("Volume(%q) = %d, want %d", tc.raw, q.Volume, tc.wantVol)
			}
		})
	}
}

// convertToQuoteForTest exposes the STOCK_DAY_ALL row→quote conversion (the
// STOCK_DAY_ALL JSON + CSV paths both go through convertToQuote) without an
// HTTP round trip.
func convertToQuoteForTest(q TWSEQuote) (domain.Quote, error) {
	c := newStockDayAllTestClient("http://unused")
	return c.convertToQuote(q)
}

// TestTWSEStockDayAll_LiveVolumeUnitStats measures the real endpoint. It is
// env-gated so CI never depends on upstream availability:
//
//	ATLAS_TEST_MARKETDATA_LIVE=1 go test ./internal/marketdata -run LiveStockDayAll -v
//
// It reports the numbers the production-scale audit needs: how many listed rows
// come back, and how many of them clear the universe's NT$10M turnover floor.
func TestTWSEStockDayAll_LiveVolumeUnitStats(t *testing.T) {
	if os.Getenv("ATLAS_TEST_MARKETDATA_LIVE") == "" {
		t.Skip("set ATLAS_TEST_MARKETDATA_LIVE=1 to hit the real TWSE endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	quotes, err := GetSharedTWSEClient().GetQuotes(ctx)
	if err != nil {
		t.Fatalf("live STOCK_DAY_ALL: %v", err)
	}
	var withVolume, zeroVolume, aboveFloor int
	var maxVol int64
	for _, q := range quotes {
		if q.Volume == 0 {
			zeroVolume++
			continue
		}
		withVolume++
		if q.Volume > maxVol {
			maxVol = q.Volume
		}
		if float64(q.Volume)*q.Last >= 10_000_000 {
			aboveFloor++
		}
	}
	t.Logf("live STOCK_DAY_ALL: rows=%d volume>0=%d volume=0=%d max_volume=%d above_10M_turnover=%d",
		len(quotes), withVolume, zeroVolume, maxVol, aboveFloor)
	if len(quotes) == 0 {
		t.Fatal("live endpoint returned no rows")
	}
	if withVolume == 0 {
		t.Fatal("every live row has Volume=0: the share-count unit assumption is broken")
	}
	// A share count for the largest listed name is in the millions; a lot count
	// would be in the thousands. This is the unit sanity check.
	if maxVol < 1_000_000 {
		t.Errorf("max volume = %d: looks like a LOT count, not 成交股數 (shares)", maxVol)
	}
}

// ── Cross-provider unit gap (verified 2026-09-25) ───────────────────────────
//
// TWSE reports SHARES, Fugle/Fubon report LOTS. The two providers therefore
// disagree by 1000x on domain.Quote.Volume, and
// monitoring.ScoringScreener.applyVolumeAndPriceFilters multiplies that field
// by the last price to compare against its NT$10M turnover floor.
//
// Official sources:
//
//	Fugle  Candles: data[0].volume — "Ｋ線成交量（整股：成交張數；興櫃股票及盤中
//	        零股：成交股數；指數：成交金額）"
//	         https://developer.fugle.tw/docs/data/http-api/intraday/candles/
//	Fugle  Quote example: total.tradeValue=31,019,803,000,
//	         total.tradeVolume=54,538, avgPrice=568.77 → tradeValue/tradeVolume
//	         = avgPrice x 1000, i.e. tradeVolume counts LOTS.
//	Fubon  富邦新一代 API docs are the same Fugle marketdata spec
//	         (SDK path fubon_neo.fugle_marketdata...): identical field list,
//	         identical example, same "整股：成交張數" note.
//	         https://www.fbs.com.tw/TradeAPI/docs/market-data/http-api/intraday/quote/
//
// The tests below pin the CURRENT, unnormalised behaviour so the gap cannot
// change silently. They are tripwires, not endorsements.

// TestFugleTradeVolumeIsLots_DocumentedUnitGap decodes the official Fugle quote
// example and shows, arithmetically, that total.tradeVolume is a LOT count.
func TestFugleTradeVolumeIsLots_DocumentedUnitGap(t *testing.T) {
	// Verbatim fields from the official example (2330, 2023-05-29).
	const (
		exampleTradeValue  = 31_019_803_000.0
		exampleTradeVolume = 54_538
		exampleAvgPrice    = 568.77
	)

	impliedShareCount := exampleTradeValue / exampleAvgPrice
	ratio := impliedShareCount / float64(exampleTradeVolume)
	t.Logf("official example: tradeValue=%.0f avgPrice=%.2f → implied shares=%.0f; tradeVolume=%d; ratio=%.1f",
		exampleTradeValue, exampleAvgPrice, impliedShareCount, exampleTradeVolume, ratio)

	if ratio < 999 || ratio > 1001 {
		t.Fatalf("tradeValue/tradeVolume/avgPrice = %.3f, want 1000 (lots): the upstream unit convention changed — "+
			"re-read the Fugle/Fubon docs before touching the providers", ratio)
	}

	// And that is what the client hands to domain.Quote.Volume: the raw field.
	const officialExampleBody = `{"date":"2023-05-29","type":"EQUITY","exchange":"TWSE","market":"TSE",` +
		`"symbol":"2330","name":"台積電","closePrice":568,"avgPrice":568.77,"lastPrice":568,` +
		`"total":{"tradeValue":31019803000,"tradeVolume":54538,"transaction":9530}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(officialExampleBody))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	q, err := c.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Volume != exampleTradeVolume {
		t.Fatalf("Quote.Volume = %d, want the raw tradeVolume %d. If the client was changed to normalise to shares, "+
			"do the same for the Fubon proxy (fubon_client.go) and TWSE (twse_openapi.go) and update every consumer "+
			"(screener VolumeIntraday, ledger quote history, dashboard display) in the same change.",
			q.Volume, exampleTradeVolume)
	}
	// The payload's own money field settles it: 54,538 lots x 1000 shares x
	// NT$568 = NT$31.0bn = tradeValue. Reading Volume as shares would give
	// NT$31M, three orders of magnitude below the payload's own tradeValue.
	if normalised := float64(q.Volume) * 1000 * q.Last; normalised < 30.5e9 || normalised > 31.5e9 {
		t.Errorf("normalised (x1000) turnover = %.0f, want ~NT$31bn (tradeValue from the same payload)", normalised)
	}
	if asShares := float64(q.Volume) * q.Last; asShares >= 31e6 {
		t.Errorf("share-read turnover = %.0f, want ~NT$31M — far from the payload's tradeValue", asShares)
	}
	t.Logf("Quote.Volume=%d is LOTS; normalising to shares for the universe turnover floor needs x1000 (TWSE path already returns shares)",
		q.Volume)
}

// TestFubonClientTradeVolumeIsLots_DocumentedUnitGap pins the same gap on the
// proxy path: services/fubon-proxy/main.py maps the Fubon SDK's
// total.tradeVolume into volume, and fubon_client.go puts it into
// domain.Quote.Volume unchanged.
func TestFubonClientTradeVolumeIsLots_DocumentedUnitGap(t *testing.T) {
	const proxyBody = `{"symbol":"2330","last":568,"open":574,"high":574,"low":564,` +
		`"volume":54538,"is_open":false,"is_close":true,"source":"fubon"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(proxyBody))
	}))
	defer srv.Close()

	c := NewFubonClient()
	c.proxyURL = srv.URL
	c.intradayLimiter = rate.NewLimiter(rate.Inf, 1)

	q, err := c.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Volume != 54_538 {
		t.Fatalf("Quote.Volume = %d, want the proxy's raw 54538 (lots)", q.Volume)
	}
	// As lots this mega-cap still clears the NT$10M floor (NT$31M), so the
	// gap does not announce itself on 台積電; it wipes the mid and small caps,
	// whose true turnover sits between NT$10M and NT$10bn.
	asLots := float64(q.Volume) * q.Last
	asShares := float64(q.Volume) * 1000 * q.Last
	t.Logf("same payload read as lots = NT$%.0f, as shares = NT$%.0f (1000x gap)", asLots, asShares)
	if asShares/asLots != 1000 {
		t.Errorf("unit gap = %v, want exactly 1000", asShares/asLots)
	}
}

// TestUniverseTurnoverFloorIsUnitSensitive quantifies the consequence: the
// NT$10M floor includes a symbol at NT$20M turnover as shares but excludes it
// as lots.
func TestUniverseTurnoverFloorIsUnitSensitive(t *testing.T) {
	const (
		floorTWD = 10_000_000.0
		last     = 100.0
		// 200,000 shares = 200 張 = NT$20M turnover.
		shares = 200_000
		lots   = 200
	)
	t.Logf("NT$%.0f floor at NT$%.0f/share: shares=%d → NT$%.0f (pass); lots=%d → NT$%.0f (fail)",
		floorTWD, last, shares, float64(shares)*last, lots, float64(lots)*last)

	if float64(shares)*last < floorTWD {
		t.Error("share-denominated turnover should clear the floor")
	}
	if float64(lots)*last >= floorTWD {
		t.Error("lot-denominated turnover should NOT clear the floor")
	}
}
