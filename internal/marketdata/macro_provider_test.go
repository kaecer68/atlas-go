package marketdata

import (
	"context"
	"errors"
	"testing"
)

// failingMacroProvider returns an error from FetchSnapshot.
type failingMacroProvider struct{ name string }

func (f *failingMacroProvider) Name() string                                         { return f.name }
func (f *failingMacroProvider) FetchSnapshot(_ context.Context) (MacroDataSnapshot, error) {
	return MacroDataSnapshot{}, errors.New("mock failure")
}

func TestCompositeMacroProvider_AllSucceed(t *testing.T) {
	p1 := &MockMacroProvider{Snapshot: MacroDataSnapshot{
		NVDA:   MacroDataPoint{Symbol: "NVDA", Value: 100, ChangePct: 2.5},
		SOXIndex: MacroDataPoint{Symbol: "^SOX", Value: 5000, ChangePct: -1.2},
	}}
	p2 := &MockMacroProvider{Snapshot: MacroDataSnapshot{
		US10Y: MacroDataPoint{Symbol: "^TNX", Value: 4.5, ChangePct: 0.1},
		SPXIndex: MacroDataPoint{Symbol: "^GSPC", Value: 5500, ChangePct: 0.5},
	}}

	composite := NewCompositeMacroProvider(p1, p2)
	merged, err := composite.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if merged.DataStatus != "ok" {
		t.Errorf("DataStatus = %q, want %q", merged.DataStatus, "ok")
	}
	if len(merged.FailedChannels) != 0 {
		t.Errorf("FailedChannels = %v, want empty", merged.FailedChannels)
	}
	if merged.NVDA.Value != 100 {
		t.Errorf("NVDA.Value = %v, want 100", merged.NVDA.Value)
	}
	if merged.SOXIndex.Value != 5000 {
		t.Errorf("SOXIndex.Value = %v, want 5000", merged.SOXIndex.Value)
	}
}

func TestCompositeMacroProvider_PartialFailure(t *testing.T) {
	p1 := &failingMacroProvider{name: "broken_provider"}
	p2 := &MockMacroProvider{Snapshot: MacroDataSnapshot{
		NVDA: MacroDataPoint{Symbol: "NVDA", Value: 100, ChangePct: 2.5},
	}}
	p3 := &MockMacroProvider{Snapshot: MacroDataSnapshot{
		TSMADR: MacroDataPoint{Symbol: "TSM", Value: 200, ChangePct: -1.0},
	}}

	composite := NewCompositeMacroProvider(p1, p2, p3)
	merged, err := composite.FetchSnapshot(context.Background())
	// Partial failure should NOT be an error — data from successful providers is still usable.
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v, want nil (partial failure is not fatal)", err)
	}
	if merged.DataStatus != "degraded" {
		t.Errorf("DataStatus = %q, want %q (1 of 3 failed)", merged.DataStatus, "degraded")
	}
	if len(merged.FailedChannels) != 1 {
		t.Errorf("FailedChannels len = %d, want 1; channels=%v", len(merged.FailedChannels), merged.FailedChannels)
	}
	if merged.FailedChannels[0] != "broken_provider" {
		t.Errorf("FailedChannels[0] = %q, want %q", merged.FailedChannels[0], "broken_provider")
	}
	// Successful providers should still have their data merged.
	if merged.NVDA.Value != 100 {
		t.Errorf("NVDA.Value = %v, want 100 (should still be merged from p2)", merged.NVDA.Value)
	}
	if merged.TSMADR.Value != 200 {
		t.Errorf("TSMADR.Value = %v, want 200 (should still be merged from p3)", merged.TSMADR.Value)
	}
}

func TestCompositeMacroProvider_AllFail(t *testing.T) {
	p1 := &failingMacroProvider{name: "broken_a"}
	p2 := &failingMacroProvider{name: "broken_b"}

	composite := NewCompositeMacroProvider(p1, p2)
	merged, err := composite.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("FetchSnapshot() error = nil, want error (all providers failed)")
	}
	if merged.DataStatus != "stale" {
		t.Errorf("DataStatus = %q, want %q (all failed)", merged.DataStatus, "stale")
	}
	if len(merged.FailedChannels) != 2 {
		t.Errorf("FailedChannels len = %d, want 2; channels=%v", len(merged.FailedChannels), merged.FailedChannels)
	}
}

func TestCompositeMacroProvider_EmptyNoProviders(t *testing.T) {
	composite := NewCompositeMacroProvider()
	merged, err := composite.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if merged.DataStatus != "ok" {
		t.Errorf("DataStatus = %q, want %q (0 of 0 failed)", merged.DataStatus, "ok")
	}
	if len(merged.FailedChannels) != 0 {
		t.Errorf("FailedChannels = %v, want empty", merged.FailedChannels)
	}
}
