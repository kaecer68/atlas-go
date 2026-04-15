package narrative

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TaiwanStressIndex represents a composite market pressure score for Taiwan.
type TaiwanStressIndex struct {
	Score      float64            `json:"score"`  // 0 - 100
	Regime     string             `json:"regime"` // low / alert / high / crisis
	Components map[string]float64 `json:"components"`
	Timestamp  int64              `json:"timestamp"`
}

// TaiwanStressCalculator computes the stress index from macro and capital flow data.
type TaiwanStressCalculator struct {
	geoProvider GeopoliticalRiskProvider
	mu          sync.RWMutex
	cache       *TaiwanStressIndex
	cachedAt    time.Time
	cacheTTL    time.Duration
}

// NewTaiwanStressCalculator creates a calculator with an optional geopolitical provider.
func NewTaiwanStressCalculator(geoProvider GeopoliticalRiskProvider) *TaiwanStressCalculator {
	if geoProvider == nil {
		geoProvider = NewRSSGeopoliticalProvider()
	}
	return &TaiwanStressCalculator{
		geoProvider: geoProvider,
		cacheTTL:    5 * time.Minute,
	}
}

// Calculate computes the stress index from the given snapshot and geopolitical score.
func (c *TaiwanStressCalculator) Calculate(snap marketdata.MacroDataSnapshot, geoScore GeopoliticalRiskScore) TaiwanStressIndex {
	components := make(map[string]float64)

	// DXY component (weight 15%): absolute change pct scaled to 0-100.
	dxyComponent := math.Abs(snap.DXY.ChangePct) * 5.0
	if dxyComponent > 100 {
		dxyComponent = 100
	}
	components["dxy"] = dxyComponent * 0.15

	// US10Y component (weight 20%): change in bps proxy scaled.
	us10yChange := snap.US10Y.Value
	if us10yChange < 0 {
		us10yChange = -us10yChange
	}
	us10yComponent := us10yChange * 2.0
	if us10yComponent > 100 {
		us10yComponent = 100
	}
	components["us10y"] = us10yComponent * 0.20

	// Foreign investor net sell component (weight 25%): negative flow scaled.
	foreignFlow := -snap.ForeignInvestorNet.Value // net sell is positive stress
	if foreignFlow < 0 {
		foreignFlow = 0
	}
	foreignComponent := foreignFlow * 10.0
	if foreignComponent > 100 {
		foreignComponent = 100
	}
	components["foreign_flow"] = foreignComponent * 0.25

	// VIX component (weight 15%): raw VIX level scaled (VIX 30 -> 50, VIX 40 -> 100).
	vixComponent := (snap.VIX.Value / 40.0) * 100.0
	if vixComponent > 100 {
		vixComponent = 100
	}
	components["vix"] = vixComponent * 0.15

	// JPY component (weight 10%): JPY appreciation or carry unwind increases Taiwan stress.
	jpyChange := math.Abs(snap.JPY.ChangePct)
	jpyComponent := jpyChange * 10.0
	if jpyComponent > 100 {
		jpyComponent = 100
	}
	components["jpy"] = jpyComponent * 0.10

	// Geopolitical risk component (weight 15%).
	geoComponent := geoScore.Intensity
	components["geopolitical"] = geoComponent * 0.15

	score := components["dxy"] + components["us10y"] + components["foreign_flow"] +
		components["vix"] + components["jpy"] + components["geopolitical"]

	regime := "low"
	switch {
	case score >= 70:
		regime = "crisis"
	case score >= 50:
		regime = "high"
	case score >= 30:
		regime = "alert"
	}

	return TaiwanStressIndex{
		Score:      score,
		Regime:     regime,
		Components: components,
		Timestamp:  snap.RecordedAt,
	}
}

// CalculateFromSnapshot fetches the geopolitical score and computes the index.
// Results are cached for 5 minutes to avoid repeated slow external calls on every dashboard refresh.
func (c *TaiwanStressCalculator) CalculateFromSnapshot(ctx context.Context, snap marketdata.MacroDataSnapshot) (TaiwanStressIndex, error) {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cachedAt) < c.cacheTTL {
		idx := *c.cache
		c.mu.RUnlock()
		return idx, nil
	}
	c.mu.RUnlock()

	geoScore, err := c.geoProvider.FetchScore(ctx)
	if err != nil {
		return TaiwanStressIndex{}, fmt.Errorf("fetch geopolitical score: %w", err)
	}
	idx := c.Calculate(snap, geoScore)

	c.mu.Lock()
	c.cache = &idx
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return idx, nil
}
