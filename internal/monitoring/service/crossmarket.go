package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/globalmarket"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// CrossMarketStatus represents the current US-Taiwan cross-market state.
type CrossMarketStatus struct {
	RecordedAt  int64  `json:"recorded_at"`
	GeneratedAt string `json:"generated_at"`

	// US indices
	SPX CrossMarketIndex `json:"spx"`
	NDX CrossMarketIndex `json:"ndx"`
	DJI CrossMarketIndex `json:"dji"`
	SOX CrossMarketIndex `json:"sox"`

	// US tech stocks
	NVDA CrossMarketIndex `json:"nvda"`
	AAPL CrossMarketIndex `json:"aapl"`
	MSFT CrossMarketIndex `json:"msft"`

	// TSM ADR
	TSMADR CrossMarketIndex `json:"tsm_adr"`

	// Cross-market signals
	VIX     CrossMarketIndex `json:"vix"`
	DXY     CrossMarketIndex `json:"dxy"`
	USD_TWD CrossMarketIndex `json:"usd_twd"`
	US10Y   CrossMarketIndex `json:"us10y"`

	// Derived
	CrisisActive       bool    `json:"crisis_active"`
	CorrelationSPXTWSE float64 `json:"correlation_spx_twse"`
}

// CrossMarketIndex bundles an index/stock value with its metadata.
type CrossMarketIndex struct {
	Symbol    string  `json:"symbol"`
	Value     float64 `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Timestamp int64   `json:"timestamp"`
}

// CorrelationResponse carries the dynamic correlation estimate.
type CorrelationResponse struct {
	Correlation  float64 `json:"correlation"`
	WindowSize   int     `json:"window_size"`
	Observations int     `json:"observations"`
	ComputedAt   string  `json:"computed_at"`
	IsFallback   bool    `json:"is_fallback"`
}

// USIndicesResponse returns snapshot of US market indices.
type USIndicesResponse struct {
	RecordedAt  int64              `json:"recorded_at"`
	GeneratedAt string             `json:"generated_at"`
	Indices     []CrossMarketIndex `json:"indices"`
	TechStocks  []CrossMarketIndex `json:"tech_stocks"`
}

// CrossMarketService provides cross-market data from the composite macro provider.
type CrossMarketService struct {
	provider           marketdata.MacroDataProvider
	rollingCorrelation *globalmarket.RollingCorrelation
}

// NewCrossMarketService creates a cross-market service backed by the composite provider.
func NewCrossMarketService(provider marketdata.MacroDataProvider) *CrossMarketService {
	return &CrossMarketService{
		provider:           provider,
		rollingCorrelation: globalmarket.NewRollingCorrelation(20),
	}
}

// UpdateCorrelation pushes a new daily return pair (SPX, TWSE proxy = SOX)
// into the rolling correlation engine.
func (s *CrossMarketService) UpdateCorrelation(spxReturn, soxReturn float64) {
	s.rollingCorrelation.Update(spxReturn, soxReturn)
}

// GetStatus returns the full cross-market status snapshot.
func (s *CrossMarketService) GetStatus(ctx context.Context) (*CrossMarketStatus, error) {
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch macro snapshot: %w", err)
	}

	status := &CrossMarketStatus{
		RecordedAt:   snap.RecordedAt,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CrisisActive: snap.VIX.Value >= 35.0,
	}

	status.SPX = toIndex(snap.SPXIndex)
	status.NDX = toIndex(snap.NDXIndex)
	status.DJI = toIndex(snap.DJIIndex)
	status.SOX = toIndex(snap.SOXIndex)
	status.NVDA = toIndex(snap.NVDA)
	status.AAPL = toIndex(snap.AAPL)
	status.MSFT = toIndex(snap.MSFT)
	status.TSMADR = toIndex(snap.TSMADR)
	status.VIX = toIndex(snap.VIX)
	status.DXY = toIndex(snap.DXY)
	status.USD_TWD = toIndex(snap.USD_TWD)
	status.US10Y = toIndex(snap.US10Y)

	// Use the live rolling correlation from GlobalMarketManager.
	// The correlation is maintained by the realtime_feed task which
	// pushes SPX/SOX daily returns on each macro ingestion cycle.
	rho := s.rollingCorrelation.GetCurrent()
	status.CorrelationSPXTWSE = rho

	return status, nil
}

// GetCorrelation returns the current SPX-TWSE correlation estimate.
func (s *CrossMarketService) GetCorrelation() (*CorrelationResponse, error) {
	rho := s.rollingCorrelation.GetCurrent()
	obs := s.rollingCorrelation.Observations()
	return &CorrelationResponse{
		Correlation:  rho,
		WindowSize:   20,
		Observations: obs,
		ComputedAt:   time.Now().Format(time.RFC3339),
		IsFallback:   rho == 0.5 && obs < 3,
	}, nil
}

// GetUSIndices returns the current US market indices snapshot.
func (s *CrossMarketService) GetUSIndices(ctx context.Context) (*USIndicesResponse, error) {
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch macro snapshot: %w", err)
	}

	resp := &USIndicesResponse{
		RecordedAt:  snap.RecordedAt,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	resp.Indices = []CrossMarketIndex{
		toIndex(snap.SPXIndex),
		toIndex(snap.NDXIndex),
		toIndex(snap.DJIIndex),
		toIndex(snap.SOXIndex),
	}

	resp.TechStocks = []CrossMarketIndex{
		toIndex(snap.NVDA),
		toIndex(snap.AAPL),
		toIndex(snap.MSFT),
		toIndex(snap.TSMADR),
	}

	return resp, nil
}

// toIndex converts a MacroDataPoint to a CrossMarketIndex.
func toIndex(dp marketdata.MacroDataPoint) CrossMarketIndex {
	return CrossMarketIndex{
		Symbol:    dp.Symbol,
		Value:     dp.Value,
		ChangePct: dp.ChangePct,
		Timestamp: dp.Timestamp,
	}
}
