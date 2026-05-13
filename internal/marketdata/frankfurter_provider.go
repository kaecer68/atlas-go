package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

const frankfurterEndpoint = "https://api.frankfurter.dev/v2/latest?from=USD&to=JPY,TWD"

type FrankfurterFXProvider struct {
	client   *http.Client
	endpoint string
}

func NewFrankfurterFXProvider() *FrankfurterFXProvider {
	return &FrankfurterFXProvider{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: frankfurterEndpoint,
	}
}

func (f *FrankfurterFXProvider) Name() string {
	return "frankfurter_fx"
}

func (f *FrankfurterFXProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter read body: %w", err)
	}

	var fxResp frankfurterResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter unmarshal: %w", err)
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}

	if jpyRate, ok := fxResp.Rates["JPY"]; ok {
		snap.JPY = MacroDataPoint{
			Symbol:    "JPY=X",
			Value:     jpyRate,
			ChangePct: 0,
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Warn("frankfurter_provider", "missing_rate", "currency", "JPY")
	}

	if twdRate, ok := fxResp.Rates["TWD"]; ok {
		snap.USD_TWD = MacroDataPoint{
			Symbol:    "USD/TWD=X",
			Value:     twdRate,
			ChangePct: 0,
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Warn("frankfurter_provider", "missing_rate", "currency", "TWD")
	}

	return snap, nil
}

type frankfurterResponse struct {
	Date  string             `json:"date"`
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}
