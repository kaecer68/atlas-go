package marketdata

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// TraderSide is the per-trader breakdown of trading volume and open
// interest for one futures contract on the latest session.
type TraderSide struct {
	TradeLong  int64 `json:"trade_long"`
	TradeShort int64 `json:"trade_short"`
	TradeNet   int64 `json:"trade_net"`
	OILong     int64 `json:"oi_long"`
	OIShort    int64 `json:"oi_short"`
	OINet      int64 `json:"oi_net"`
}

// InstitutionalFuturesDaily holds 三大法人 positions for the TAIFEX TAIEX
// futures (ContractCode 臺股期貨) on the latest session. Values are
// raw contract counts (口數); can be negative for net short positions.
// Source: TAIFEX OpenAPI MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate.
type InstitutionalFuturesDaily struct {
	Date            string     `json:"date"`
	Foreign         TraderSide `json:"foreign"`          // 外資及陸資
	InvestmentTrust TraderSide `json:"investment_trust"` // 投信
	Dealer          TraderSide `json:"dealer"`           // 自營商
}

// rawInstitutionalTrader mirrors the TAIFEX OpenAPI JSON shape for one row.
type rawInstitutionalTrader struct {
	Date               string `json:"Date"`
	ContractCode       string `json:"ContractCode"`
	Item               string `json:"Item"`
	TradingVolumeLong  string `json:"TradingVolume(Long)"`
	TradingVolumeShort string `json:"TradingVolume(Short)"`
	TradingVolumeNet   string `json:"TradingVolume(Net)"`
	OpenInterestLong   string `json:"OpenInterest(Long)"`
	OpenInterestShort  string `json:"OpenInterest(Short)"`
	OpenInterestNet    string `json:"OpenInterest(Net)"`
}

// FetchInstitutionalFuturesDaily retrieves 三大法人 期貨 OI for the 臺股期貨
// contract from TAIFEX OpenAPI. The endpoint returns only the latest trading
// session (no date parameter), so backfill beyond the OpenAPI is not possible
// without an external source such as FinMind's TaiwanFuturesInstitutionalTraders.
func (t *TAIFEXProvider) FetchInstitutionalFuturesDaily(ctx context.Context) (*InstitutionalFuturesDaily, error) {
	if err := WaitForLimiter(ctx, t.rateLimiter); err != nil {
		return nil, fmt.Errorf("taifex institutional rate limit wait: %w", err)
	}

	url := t.baseURL + "/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("taifex institutional create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("taifex institutional http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("taifex institutional read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("taifex institutional api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var rawList []rawInstitutionalTrader
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &rawList); err != nil {
		return nil, fmt.Errorf("taifex institutional decode response: %w", err)
	}

	result := &InstitutionalFuturesDaily{}
	seen := map[string]bool{}
	for _, r := range rawList {
		if r.ContractCode != "臺股期貨" {
			continue
		}
		if seen[r.Item] {
			continue
		}
		// P0-3: a non-numeric / missing field means the upstream schema
		// changed — surface typed ErrTAIFEXSchema instead of silently
		// recording zeros (0 data + nil error looked healthy downstream).
		tradeLong, ok := parseInt64OK(r.TradingVolumeLong)
		if !ok {
			return nil, fmt.Errorf("%w: %s TradingVolume(Long)=%q not parseable", ErrTAIFEXSchema, r.Item, r.TradingVolumeLong)
		}
		tradeShort, ok := parseInt64OK(r.TradingVolumeShort)
		if !ok {
			return nil, fmt.Errorf("%w: %s TradingVolume(Short)=%q not parseable", ErrTAIFEXSchema, r.Item, r.TradingVolumeShort)
		}
		tradeNet, ok := parseInt64OK(r.TradingVolumeNet)
		if !ok {
			return nil, fmt.Errorf("%w: %s TradingVolume(Net)=%q not parseable", ErrTAIFEXSchema, r.Item, r.TradingVolumeNet)
		}
		oiLong, ok := parseInt64OK(r.OpenInterestLong)
		if !ok {
			return nil, fmt.Errorf("%w: %s OpenInterest(Long)=%q not parseable", ErrTAIFEXSchema, r.Item, r.OpenInterestLong)
		}
		oiShort, ok := parseInt64OK(r.OpenInterestShort)
		if !ok {
			return nil, fmt.Errorf("%w: %s OpenInterest(Short)=%q not parseable", ErrTAIFEXSchema, r.Item, r.OpenInterestShort)
		}
		oiNet, ok := parseInt64OK(r.OpenInterestNet)
		if !ok {
			return nil, fmt.Errorf("%w: %s OpenInterest(Net)=%q not parseable", ErrTAIFEXSchema, r.Item, r.OpenInterestNet)
		}
		side := TraderSide{
			TradeLong:  tradeLong,
			TradeShort: tradeShort,
			TradeNet:   tradeNet,
			OILong:     oiLong,
			OIShort:    oiShort,
			OINet:      oiNet,
		}
		switch r.Item {
		case "外資及陸資":
			result.Foreign = side
			result.Date = r.Date
			seen[r.Item] = true
		case "投信":
			result.InvestmentTrust = side
			seen[r.Item] = true
		case "自營商":
			result.Dealer = side
			seen[r.Item] = true
		}
	}
	if result.Date == "" {
		return nil, fmt.Errorf("taifex institutional: no 臺股期貨 rows found in response")
	}
	if !seen["外資及陸資"] || !seen["投信"] || !seen["自營商"] {
		return nil, fmt.Errorf("taifex institutional: missing trader rows (foreign=%v trust=%v dealer=%v)",
			seen["外資及陸資"], seen["投信"], seen["自營商"])
	}
	return result, nil
}
