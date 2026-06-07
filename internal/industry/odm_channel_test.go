package industry

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// mockODMProvider is a deterministic ODMDataProvider for unit tests.
// It returns the configured revenue or error and records the symbols
// it was asked about so tests can assert call order / coverage.
type mockODMProvider struct {
	revenue float64
	err     error
	calls   []string
}

func (m *mockODMProvider) FetchODMRevenue(_ context.Context, symbol string) (marketdata.ODMRevenuePoint, error) {
	m.calls = append(m.calls, symbol)
	if m.err != nil {
		return marketdata.ODMRevenuePoint{}, m.err
	}
	return marketdata.ODMRevenuePoint{
		Symbol:  symbol,
		Revenue: m.revenue,
	}, nil
}

func (m *mockODMProvider) FetchAllODMRevenue(_ context.Context) (map[string]marketdata.ODMRevenuePoint, error) {
	return nil, nil
}

const floatTolerance = 1e-9

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestODMTransmission_HappyPath_10PercentShock(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(10.0)
	channel.RegisterProvider("2317", &mockODMProvider{revenue: 5_000_000_000})
	channel.RegisterProvider("2382", &mockODMProvider{revenue: 1_200_000_000})

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	if !almostEqual(model.USCapexShock, 10.0, floatTolerance) {
		t.Errorf("USCapexShock: want 10.0, got %v", model.USCapexShock)
	}
	if !almostEqual(model.NvidiaOrderGrowth, 7.0, floatTolerance) {
		t.Errorf("NvidiaOrderGrowth: want 7.0, got %v", model.NvidiaOrderGrowth)
	}
	if !almostEqual(model.CowosUtilDelta, 4.2, floatTolerance) {
		t.Errorf("CowosUtilDelta: want 4.2, got %v", model.CowosUtilDelta)
	}
	if !almostEqual(model.TSMCRevenueImpact, 3.36, floatTolerance) {
		t.Errorf("TSMCRevenueImpact: want 3.36, got %v", model.TSMCRevenueImpact)
	}

	for _, sym := range []string{"2317", "2382"} {
		got, ok := model.ODMOrderImpact[sym]
		if !ok {
			t.Errorf("missing ODMOrderImpact for %s", sym)
			continue
		}
		if !almostEqual(got, 1.68, floatTolerance) {
			t.Errorf("ODMOrderImpact[%s]: want 1.68, got %v", sym, got)
		}
	}
}

func TestODMTransmission_ExtremeNegativeShock(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(-30.0)
	channel.RegisterProvider("2317", &mockODMProvider{revenue: 1})

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	if model.USCapexShock != -30.0 {
		t.Errorf("USCapexShock: want -30.0, got %v", model.USCapexShock)
	}
	if model.NvidiaOrderGrowth >= 0 {
		t.Errorf("NvidiaOrderGrowth should be negative, got %v", model.NvidiaOrderGrowth)
	}
	if model.CowosUtilDelta >= 0 {
		t.Errorf("CowosUtilDelta should be negative, got %v", model.CowosUtilDelta)
	}
	if model.TSMCRevenueImpact >= 0 {
		t.Errorf("TSMCRevenueImpact should be negative, got %v", model.TSMCRevenueImpact)
	}
	if got := model.ODMOrderImpact["2317"]; got >= 0 {
		t.Errorf("ODMOrderImpact should be negative, got %v", got)
	}

	for _, v := range []float64{
		model.NvidiaOrderGrowth,
		model.CowosUtilDelta,
		model.TSMCRevenueImpact,
		model.ODMOrderImpact["2317"],
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("cascade value is NaN/Inf: %v", v)
		}
	}
}

func TestODMTransmission_ZeroShock(t *testing.T) {
	channel := NewODMChannel()
	channel.RegisterProvider("6669", &mockODMProvider{revenue: 999_999_999})

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	if model.USCapexShock != 0 {
		t.Errorf("USCapexShock: want 0, got %v", model.USCapexShock)
	}
	if model.NvidiaOrderGrowth != 0 {
		t.Errorf("NvidiaOrderGrowth: want 0, got %v", model.NvidiaOrderGrowth)
	}
	if model.CowosUtilDelta != 0 {
		t.Errorf("CowosUtilDelta: want 0, got %v", model.CowosUtilDelta)
	}
	if model.TSMCRevenueImpact != 0 {
		t.Errorf("TSMCRevenueImpact: want 0, got %v", model.TSMCRevenueImpact)
	}
	if got := model.ODMOrderImpact["6669"]; got != 0 {
		t.Errorf("ODMOrderImpact: want 0, got %v", got)
	}
}

func TestODMTransmission_MultipleODMs_DistributeCorrectly(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(20.0)

	foxconn := &mockODMProvider{revenue: 6_000_000_000}
	quanta := &mockODMProvider{revenue: 1_500_000_000}
	wiwynn := &mockODMProvider{revenue: 800_000_000}

	channel.RegisterProvider("2317", foxconn)
	channel.RegisterProvider("2382", quanta)
	channel.RegisterProvider("6669", wiwynn)

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	expectedImpact := 20.0 * 0.7 * 0.6 * 0.8 * 0.5
	if !almostEqual(model.NvidiaOrderGrowth, 14.0, floatTolerance) {
		t.Errorf("NvidiaOrderGrowth: want 14.0, got %v", model.NvidiaOrderGrowth)
	}
	if !almostEqual(model.TSMCRevenueImpact, 6.72, floatTolerance) {
		t.Errorf("TSMCRevenueImpact: want 6.72, got %v", model.TSMCRevenueImpact)
	}

	for _, sym := range []string{"2317", "2382", "6669"} {
		got, ok := model.ODMOrderImpact[sym]
		if !ok {
			t.Errorf("missing impact for %s", sym)
			continue
		}
		if !almostEqual(got, expectedImpact, floatTolerance) {
			t.Errorf("ODMOrderImpact[%s]: want %v, got %v", sym, expectedImpact, got)
		}
	}

	if len(foxconn.calls) != 1 || foxconn.calls[0] != "2317" {
		t.Errorf("Foxconn provider should have been called once for 2317, got %v", foxconn.calls)
	}
	if len(quanta.calls) != 1 || quanta.calls[0] != "2382" {
		t.Errorf("Quanta provider should have been called once for 2382, got %v", quanta.calls)
	}
}

func TestODMChannel_NoProviderRegistered(t *testing.T) {
	channel := NewODMChannel()

	_, err := channel.GetRevenue(context.Background(), "9999")
	if err == nil {
		t.Fatal("expected error for unregistered symbol, got nil")
	}
	if !strings.Contains(err.Error(), "no provider registered") {
		t.Errorf("error message should mention missing provider, got: %v", err)
	}

	all, err := channel.GetAllRevenues(context.Background())
	if err != nil {
		t.Errorf("GetAllRevenues with no providers should return nil error (empty map), got: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("GetAllRevenues with no providers should return empty map, got %d entries", len(all))
	}
}

func TestODMChannel_ZeroRevenueProvider_ExcludedFromCascade(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(10.0)
	channel.RegisterProvider("2317", &mockODMProvider{revenue: 1_000_000_000})
	channel.RegisterProvider("0000", &mockODMProvider{revenue: 0})

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	if _, ok := model.ODMOrderImpact["2317"]; !ok {
		t.Error("expected ODMOrderImpact for 2317 with non-zero revenue")
	}
	if _, ok := model.ODMOrderImpact["0000"]; ok {
		t.Error("ODM with zero revenue should not appear in ODMOrderImpact map")
	}
}

func TestODMChannel_PartialProviderAvailability(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(10.0)
	channel.RegisterProvider("2317", &mockODMProvider{revenue: 1_000_000_000})
	channel.RegisterProvider("2382", &mockODMProvider{err: errors.New("finmind timeout")})

	revenues, err := channel.GetAllRevenues(context.Background())
	if err == nil {
		t.Fatal("expected error from one failing provider, got nil")
	}
	if _, ok := revenues["2317"]; !ok {
		t.Error("expected 2317 revenue in partial result despite 2382 failure")
	}
	if _, ok := revenues["2382"]; ok {
		t.Error("2382 should be absent from partial result after provider error")
	}
}

func TestODMCowosUtilizationTracker_ClampsExtremeValues(t *testing.T) {
	tracker := NewCowosUtilizationTracker()

	tracker.Update(1.5, "increasing")
	if got := tracker.Snapshot().CurrentUtilization; got != 1.0 {
		t.Errorf("utilization should clamp to 1.0, got %v", got)
	}

	tracker.Update(-0.3, "decreasing")
	if got := tracker.Snapshot().CurrentUtilization; got != 0.0 {
		t.Errorf("utilization should clamp to 0.0, got %v", got)
	}

	tracker.Update(0.7, "stable")
	if got := tracker.Snapshot().CurrentUtilization; got != 0.7 {
		t.Errorf("utilization should record 0.7, got %v", got)
	}

	tracker.Update(math.NaN(), "unknown")
	if got := tracker.Snapshot().CurrentUtilization; got != 0 {
		t.Errorf("NaN should clamp to 0, got %v", got)
	}
}

func TestODMCowosUtilizationTracker_DeltaFromBaseline(t *testing.T) {
	tracker := NewCowosUtilizationTracker()
	tracker.Update(0.90, "increasing")

	if got := tracker.GetDeltaFromBaseline(0.85); !almostEqual(got, 0.05, floatTolerance) {
		t.Errorf("delta from 0.85: want 0.05, got %v", got)
	}
	if got := tracker.GetDeltaFromBaseline(0.90); !almostEqual(got, 0.0, floatTolerance) {
		t.Errorf("delta from 0.90: want 0.0, got %v", got)
	}
}

func TestODMTransmissionModel_Summary_Format(t *testing.T) {
	channel := NewODMChannel()
	channel.SetUSCapexShock(10.0)
	channel.RegisterProvider("2317", &mockODMProvider{revenue: 1})
	channel.RegisterProvider("2382", &mockODMProvider{revenue: 1})

	model, err := channel.CalculateTransmission(context.Background())
	if err != nil {
		t.Fatalf("CalculateTransmission returned error: %v", err)
	}

	summary := model.Summary()

	if !strings.Contains(summary, "US CSP capex +10.0%") {
		t.Errorf("summary should contain US capex shock, got: %q", summary)
	}
	if !strings.Contains(summary, "Nvidia +7.00%") {
		t.Errorf("summary should contain Nvidia growth, got: %q", summary)
	}
	if !strings.Contains(summary, "CoWoS +4.20%") {
		t.Errorf("summary should contain CoWoS delta, got: %q", summary)
	}
	if !strings.Contains(summary, "TSMC +3.36%") {
		t.Errorf("summary should contain TSMC revenue impact, got: %q", summary)
	}
	if !strings.Contains(summary, "2317=+1.68%") {
		t.Errorf("summary should contain 2317 impact, got: %q", summary)
	}
	if !strings.Contains(summary, "2382=+1.68%") {
		t.Errorf("summary should contain 2382 impact, got: %q", summary)
	}

	if !strings.HasPrefix(summary, "US CSP capex") {
		t.Errorf("summary should start with 'US CSP capex', got: %q", summary)
	}
}

func TestODMTransmissionModel_Summary_NilSafe(t *testing.T) {
	var m *ODMTransmissionModel
	if got := m.Summary(); got == "" {
		t.Error("Summary on nil receiver should return non-empty sentinel")
	}
}

func TestNewODMChannel_Defaults(t *testing.T) {
	channel := NewODMChannel()
	if channel == nil {
		t.Fatal("NewODMChannel returned nil")
	}
	if channel.cowosTracker == nil {
		t.Fatal("fresh channel should have a non-nil cowos tracker")
	}
	if got := channel.cowosTracker.Snapshot().CurrentUtilization; got != defaultCowosUtilization {
		t.Errorf("default Cowos utilization: want %v, got %v", defaultCowosUtilization, got)
	}
	if got := channel.cowosTracker.Snapshot().TrendDirection; got != "stable" {
		t.Errorf("default Cowos trend: want \"stable\", got %q", got)
	}
	if got := channel.usCapexShock; got != 0 {
		t.Errorf("default US capex shock should be 0, got %v", got)
	}
	if len(channel.odmProviders) != 0 {
		t.Errorf("fresh channel should have no providers, got %d", len(channel.odmProviders))
	}
}

func TestODMChannel_RegisterProvider_NilSafe(t *testing.T) {
	channel := NewODMChannel()
	channel.RegisterProvider("", &mockODMProvider{revenue: 1})
	channel.RegisterProvider("2317", nil)
	if len(channel.odmProviders) != 0 {
		t.Errorf("nil/empty registrations should be ignored, got %d providers", len(channel.odmProviders))
	}
}

func TestODMChannel_RefreshCowosUtilization(t *testing.T) {
	channel := NewODMChannel()
	channel.cowosTracker.Update(0.42, "decreasing")

	if err := channel.RefreshCowosUtilization(context.Background()); err != nil {
		t.Fatalf("RefreshCowosUtilization returned error: %v", err)
	}

	snap := channel.CowosTracker().Snapshot()
	if snap.CurrentUtilization != defaultCowosUtilization {
		t.Errorf("RefreshCowosUtilization should reset to default %v, got %v",
			defaultCowosUtilization, snap.CurrentUtilization)
	}
	if snap.TrendDirection != "stable" {
		t.Errorf("RefreshCowosUtilization should set trend to \"stable\", got %q", snap.TrendDirection)
	}
}
