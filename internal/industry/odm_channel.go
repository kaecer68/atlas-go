package industry

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// CowosUtilizationTracker maintains the current CoWoS (Chip-on-Wafer-on-
// Substrate) packaging utilization observed for TSMC, plus its direction
// of travel. It is the single source of truth for ODMChannel when computing
// the CowosUtilDelta leg of the transmission cascade (Section 4.2).
//
// The default baseline of 0.85 reflects the typical TSMC CoWoS-S/CoWoS-L
// utilization band during AI-driven demand peaks; the field is mutable
// because the real producer (TSMC monthly revenue + supply-side capacity
// disclosure) is wired up in a parallel effort and we expose the knob
// here so downstream callers can drive it without depending on that
// pipeline being live.
type CowosUtilizationTracker struct {
	mu                 sync.RWMutex
	CurrentUtilization float64
	TrendDirection     string
	LastUpdated        time.Time
}

const defaultCowosUtilization = 0.85

// NewCowosUtilizationTracker returns a tracker seeded with the default
// utilization (0.85) and a "stable" trend.
func NewCowosUtilizationTracker() *CowosUtilizationTracker {
	return &CowosUtilizationTracker{
		CurrentUtilization: defaultCowosUtilization,
		TrendDirection:     "stable",
		LastUpdated:        time.Now(),
	}
}

// Update overwrites the tracker's current utilization (clamped to [0, 1])
// and trend label.
func (t *CowosUtilizationTracker) Update(utilization float64, trend string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CurrentUtilization = clampUnit(utilization)
	t.TrendDirection = trend
	t.LastUpdated = time.Now()
}

// Snapshot returns a copy of the tracker state, safe for the caller to
// retain across goroutine boundaries.
func (t *CowosUtilizationTracker) Snapshot() CowosUtilizationTracker {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return CowosUtilizationTracker{
		CurrentUtilization: t.CurrentUtilization,
		TrendDirection:     t.TrendDirection,
		LastUpdated:        t.LastUpdated,
	}
}

// GetDeltaFromBaseline returns the signed difference between the current
// utilization and the supplied baseline.
func (t *CowosUtilizationTracker) GetDeltaFromBaseline(baseline float64) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.CurrentUtilization - baseline
}

func clampUnit(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ODMChannel orchestrates the US CSP capex → Nvidia → CoWoS → TSMC → ODM
// transmission cascade (Section 4.2) for Taiwan server ODM names.
type ODMChannel struct {
	mu           sync.RWMutex
	odmProviders map[string]marketdata.ODMDataProvider
	cowosTracker *CowosUtilizationTracker
	usCapexShock float64
}

// NewODMChannel returns an empty channel with no providers registered and
// a fresh CowosUtilizationTracker at the default 0.85 utilization.
func NewODMChannel() *ODMChannel {
	return &ODMChannel{
		odmProviders: make(map[string]marketdata.ODMDataProvider),
		cowosTracker: NewCowosUtilizationTracker(),
	}
}

// RegisterProvider binds a marketdata.ODMDataProvider to a Taiwan ODM
// symbol. Re-registering the same symbol overwrites the previous
// registration; this is intentional and matches the "last write wins"
// convention used elsewhere in the linkage layer.
func (c *ODMChannel) RegisterProvider(symbol string, provider marketdata.ODMDataProvider) {
	if symbol == "" || provider == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.odmProviders[symbol] = provider
}

// SetUSCapexShock records the most recent US CSP capex change (in percent).
// CalculateTransmission consumes this value when computing the cascade.
func (c *ODMChannel) SetUSCapexShock(shock float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usCapexShock = shock
}

// GetRevenue delegates to the provider registered for the given symbol
// and returns the latest monthly revenue (in TWD) from its ODMRevenuePoint.
func (c *ODMChannel) GetRevenue(ctx context.Context, symbol string) (float64, error) {
	c.mu.RLock()
	provider, ok := c.odmProviders[symbol]
	c.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("odm_channel: no provider registered for symbol %q", symbol)
	}
	point, err := provider.FetchODMRevenue(ctx, symbol)
	if err != nil {
		return 0, fmt.Errorf("odm_channel: fetch %s: %w", symbol, err)
	}
	return point.Revenue, nil
}

// GetAllRevenues fetches revenues for every registered symbol and
// returns the result as a symbol → revenue map. Individual provider
// errors are accumulated and the first one is returned alongside the
// partial result, letting callers log-and-continue.
func (c *ODMChannel) GetAllRevenues(ctx context.Context) (map[string]float64, error) {
	c.mu.RLock()
	symbols := make([]string, 0, len(c.odmProviders))
	for sym := range c.odmProviders {
		symbols = append(symbols, sym)
	}
	c.mu.RUnlock()
	sort.Strings(symbols)

	result := make(map[string]float64, len(symbols))
	var firstErr error
	for _, sym := range symbols {
		rev, err := c.GetRevenue(ctx, sym)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result[sym] = rev
	}
	return result, firstErr
}

// RefreshCowosUtilization is the seam where a real CoWoS utilization feed
// will eventually land. The current implementation updates the tracker
// with the default baseline reading and a "stable" trend label.
func (c *ODMChannel) RefreshCowosUtilization(_ context.Context) error {
	if c.cowosTracker == nil {
		c.cowosTracker = NewCowosUtilizationTracker()
	}
	c.cowosTracker.Update(defaultCowosUtilization, "stable")
	return nil
}

// CowosTracker exposes the underlying CowosUtilizationTracker for callers
// that need to inspect or override the reading directly.
func (c *ODMChannel) CowosTracker() *CowosUtilizationTracker {
	return c.cowosTracker
}

// CalculateTransmission gathers the current US CSP capex shock, the live
// CoWoS utilization, and the per-symbol ODM revenues, then runs them
// through the linear cascade encoded by ODMTransmissionModel.Calculate.
func (c *ODMChannel) CalculateTransmission(ctx context.Context) (*ODMTransmissionModel, error) {
	if c.cowosTracker == nil {
		return nil, fmt.Errorf("odm_channel: cowos tracker not initialized")
	}

	c.mu.RLock()
	usCapexShock := c.usCapexShock
	c.mu.RUnlock()

	trackerSnap := c.cowosTracker.Snapshot()
	cowosUtil := map[string]float64{
		"cowos": trackerSnap.CurrentUtilization,
	}

	odmRevenues, _ := c.GetAllRevenues(ctx)

	usCapexShockMap := map[string]float64{
		"us_csp": usCapexShock,
	}

	return ODMTransmissionModel{}.Calculate(usCapexShockMap, cowosUtil, odmRevenues), nil
}

// ODMTransmissionModel is the result of running the Section 4.2 cascade
// over a single observation window.
type ODMTransmissionModel struct {
	USCapexShock      float64
	NvidiaOrderGrowth float64
	CowosUtilDelta    float64
	TSMCRevenueImpact float64
	ODMOrderImpact    map[string]float64
}

const (
	cowosNvidiaPassThrough = 0.7
	cowosUtilSensitivity   = 0.6
	tsmcRevenuePassThrough = 0.8
	odmOrderPassThrough    = 0.5
)

// Calculate runs the linear cascade over the supplied inputs. It is a
// pure function: no I/O, no shared state, no clock.
//
//   - uscapexShock["us_csp"] is the US CSP capex percentage change.
//   - cowosUtil["cowos"] is the current CoWoS utilization in [0, 1].
//     The observed reading is accepted for forward compatibility — the
//     linear cascade computes CowosUtilDelta from NvidiaOrderGrowth
//     rather than from the observed-vs-baseline difference, leaving the
//     observed reading available for future overlay models.
//   - odmRevenues maps each registered ODM symbol to its current revenue.
//     Each non-zero revenue entry produces a corresponding ODMOrderImpact.
func (ODMTransmissionModel) Calculate(
	uscapexShock map[string]float64,
	cowosUtil map[string]float64,
	odmRevenues map[string]float64,
) *ODMTransmissionModel {
	model := &ODMTransmissionModel{
		ODMOrderImpact: make(map[string]float64),
	}

	usShock := uscapexShock["us_csp"]
	model.USCapexShock = usShock

	model.NvidiaOrderGrowth = usShock * cowosNvidiaPassThrough
	model.CowosUtilDelta = model.NvidiaOrderGrowth * cowosUtilSensitivity
	model.TSMCRevenueImpact = model.CowosUtilDelta * tsmcRevenuePassThrough

	odmImpact := model.TSMCRevenueImpact * odmOrderPassThrough
	_ = cowosUtil["cowos"]

	for symbol, rev := range odmRevenues {
		if rev == 0 {
			continue
		}
		model.ODMOrderImpact[symbol] = odmImpact
	}

	return model
}

// Summary returns a one-line human-readable rendering of the cascade,
// e.g. "US CSP capex +10.0% → Nvidia +7.00% → CoWoS +4.20% → TSMC
// +3.36% → ODM order impact: 2317=+1.68%, 2382=+1.68%".
func (m *ODMTransmissionModel) Summary() string {
	if m == nil {
		return "ODM transmission: <nil model>"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "US CSP capex %+.1f%%", m.USCapexShock)

	if m.NvidiaOrderGrowth != 0 || m.CowosUtilDelta != 0 || m.TSMCRevenueImpact != 0 {
		fmt.Fprintf(&b, " → Nvidia %+.2f%% → CoWoS %+.2f%% → TSMC %+.2f%%",
			m.NvidiaOrderGrowth, m.CowosUtilDelta, m.TSMCRevenueImpact)
	}

	if len(m.ODMOrderImpact) == 0 {
		return b.String()
	}

	symbols := make([]string, 0, len(m.ODMOrderImpact))
	for sym := range m.ODMOrderImpact {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	parts := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		parts = append(parts, fmt.Sprintf("%s=%+.2f%%", sym, m.ODMOrderImpact[sym]))
	}
	fmt.Fprintf(&b, " → ODM order impact: %s", strings.Join(parts, ", "))
	return b.String()
}
