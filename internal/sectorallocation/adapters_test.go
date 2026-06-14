package sectorallocation

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// =====================================================================
// CycleAdapter
// =====================================================================

func TestNewCycleAdapter(t *testing.T) {
	// Non-nil tracker
	tracker := industry.NewCycleTracker()
	adapter := NewCycleAdapter(tracker)
	if adapter == nil {
		t.Fatal("NewCycleAdapter with non-nil tracker returned nil")
	}
	if adapter.tracker != tracker {
		t.Error("adapter.tracker does not match input tracker")
	}

	// Nil tracker
	nilAdapter := NewCycleAdapter(nil)
	if nilAdapter == nil {
		t.Fatal("NewCycleAdapter with nil tracker returned nil")
	}
	if nilAdapter.tracker != nil {
		t.Error("nilAdapter.tracker should be nil")
	}
}

func TestCycleAdapter_GetCycleMultiplier(t *testing.T) {
	ctx := context.Background()
	unknownID := "unknown-industry-xyz"

	t.Run("nil_tracker_returns_1.0", func(t *testing.T) {
		adapter := NewCycleAdapter(nil)
		m, err := adapter.GetCycleMultiplier(ctx, unknownID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 for nil tracker, got %f", m)
		}
	})

	t.Run("unknown_industry_returns_1.0", func(t *testing.T) {
		tracker := industry.NewCycleTracker()
		adapter := NewCycleAdapter(tracker)
		m, err := adapter.GetCycleMultiplier(ctx, unknownID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 for unknown industry, got %f", m)
		}
	})
}

// =====================================================================
// SeasonalAdapter
// =====================================================================

func TestNewSeasonalAdapter(t *testing.T) {
	// Non-nil engine
	engine := industry.NewSeasonalEngine()
	adapter := NewSeasonalAdapter(engine)
	if adapter == nil {
		t.Fatal("NewSeasonalAdapter with non-nil engine returned nil")
	}
	if adapter.engine != engine {
		t.Error("adapter.engine does not match input engine")
	}

	// Nil engine
	nilAdapter := NewSeasonalAdapter(nil)
	if nilAdapter == nil {
		t.Fatal("NewSeasonalAdapter with nil engine returned nil")
	}
	if nilAdapter.engine != nil {
		t.Error("nilAdapter.engine should be nil")
	}
}

func TestSeasonalAdapter_GetSeasonalMultiplier(t *testing.T) {
	ctx := context.Background()
	unknownID := "unknown-industry-xyz"
	now := time.Now()

	t.Run("nil_engine_returns_1.0", func(t *testing.T) {
		adapter := NewSeasonalAdapter(nil)
		m, err := adapter.GetSeasonalMultiplier(ctx, unknownID, now)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 for nil engine, got %f", m)
		}
	})

	t.Run("unknown_industry_returns_1.0", func(t *testing.T) {
		engine := industry.NewSeasonalEngine()
		adapter := NewSeasonalAdapter(engine)
		m, err := adapter.GetSeasonalMultiplier(ctx, unknownID, now)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 for unknown industry, got %f", m)
		}
	})
}

func TestSeasonalAdapter_GetActivePatternNames(t *testing.T) {
	ctx := context.Background()
	unknownID := "unknown-industry-xyz"
	now := time.Now()

	t.Run("nil_engine_returns_nil", func(t *testing.T) {
		adapter := NewSeasonalAdapter(nil)
		names := adapter.GetActivePatternNames(ctx, unknownID, now)
		if names != nil {
			t.Errorf("expected nil for nil engine, got %v", names)
		}
	})

	t.Run("returns_slice_for_real_engine", func(t *testing.T) {
		engine := industry.NewSeasonalEngine()
		adapter := NewSeasonalAdapter(engine)
		names := adapter.GetActivePatternNames(ctx, unknownID, now)
		// Should return a non-nil slice (may be empty)
		if names == nil {
			t.Error("expected non-nil slice, got nil")
		}
	})
}

// =====================================================================
// LinkageAdapter
// =====================================================================

func TestNewLinkageAdapter(t *testing.T) {
	analyzer := industry.NewLinkageAnalyzer()
	propagation := &industry.ShockPropagation{}

	adapter := NewLinkageAdapter(analyzer, propagation)
	if adapter == nil {
		t.Fatal("NewLinkageAdapter returned nil")
	}
	if adapter.analyzer != analyzer {
		t.Error("adapter.analyzer does not match input")
	}
	if adapter.propagation != propagation {
		t.Error("adapter.propagation does not match input")
	}
}

func TestLinkageAdapter_GetLinkageMultiplier(t *testing.T) {
	ctx := context.Background()
	unknownID := "unknown-industry-xyz"

	t.Run("nil_analyzer_nil_propagation_returns_1.0", func(t *testing.T) {
		adapter := NewLinkageAdapter(nil, nil)
		m, err := adapter.GetLinkageMultiplier(ctx, unknownID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0, got %f", m)
		}
	})

	t.Run("nil_propagation_with_analyzer_unknown_industry", func(t *testing.T) {
		analyzer := industry.NewLinkageAnalyzer()
		adapter := NewLinkageAdapter(analyzer, nil)
		m, err := adapter.GetLinkageMultiplier(ctx, unknownID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 for unknown industry via analyzer, got %f", m)
		}
	})
}

// =====================================================================
// NarrativeAdapter
// =====================================================================

func TestNewNarrativeAdapter(t *testing.T) {
	t.Run("nil_fn_uses_noop", func(t *testing.T) {
		provider := NewNarrativeAdapter(nil)
		if provider == nil {
			t.Fatal("NewNarrativeAdapter(nil) returned nil")
		}
		// Should return a neutral value without panicking
		m, err := provider.GetNarrativeMultiplier(context.Background(), "any-id")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0 from nil fn shim, got %f", m)
		}
	})

	t.Run("valid_fn_wrapped", func(t *testing.T) {
		fn := func(ctx context.Context, industryID string) (float64, float64, string, error) {
			return 1.2, 0.8, "test reason", nil
		}
		provider := NewNarrativeAdapter(fn)
		if provider == nil {
			t.Fatal("NewNarrativeAdapter(fn) returned nil")
		}
		m, err := provider.GetNarrativeMultiplier(context.Background(), "test-id")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.2 {
			t.Errorf("expected 1.2, got %f", m)
		}
	})
}

func TestNarrativeShim_GetNarrativeMultiplier(t *testing.T) {
	ctx := context.Background()
	shim := narrativeShim{
		fn: func(ctx context.Context, industryID string) (float64, float64, string, error) {
			return 1.3, 0.9, "shim reason", nil
		},
	}
	m, err := shim.GetNarrativeMultiplier(ctx, "any-id")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if m != 1.3 {
		t.Errorf("expected 1.3, got %f", m)
	}
}

func TestNarrativeProviderFunc_GetNarrativeMultiplier(t *testing.T) {
	ctx := context.Background()
	fn := func(ctx context.Context, industryID string) (float64, float64, string, error) {
		return 1.4, 0.7, "func reason", nil
	}
	var provider NarrativeProviderFunc = fn
	m, err := provider.GetNarrativeMultiplier(ctx, "test-id")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if m != 1.4 {
		t.Errorf("expected 1.4, got %f", m)
	}
}

// =====================================================================
// MacroProviderFunc
// =====================================================================

func TestMacroProviderFunc_GetMacroTilt(t *testing.T) {
	ctx := context.Background()
	fn := func(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error) {
		if macroLevel == "bull" {
			return 1.1, nil
		}
		return 0.9, nil
	}
	var provider MacroProviderFunc = fn

	t.Run("bull_level", func(t *testing.T) {
		m, err := provider.GetMacroTilt(ctx, "semiconductor", "bull", "foreign")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.1 {
			t.Errorf("expected 1.1, got %f", m)
		}
	})

	t.Run("bear_level", func(t *testing.T) {
		m, err := provider.GetMacroTilt(ctx, "semiconductor", "bear", "foreign")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 0.9 {
			t.Errorf("expected 0.9, got %f", m)
		}
	})
}

func TestMacroProviderFunc_GetMacroFlowDescription(t *testing.T) {
	var provider MacroProviderFunc = func(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error) {
		return 1.0, nil
	}
	desc := provider.GetMacroFlowDescription("bull", "foreign")
	if desc != "" {
		t.Errorf("expected empty string, got %q", desc)
	}
}

// =====================================================================
// FactorProviderFunc
// =====================================================================

func TestFactorProviderFunc_GetFactorTilt(t *testing.T) {
	ctx := context.Background()
	fn := func(ctx context.Context, industryID string) (float64, error) {
		if industryID == "semiconductor" {
			return 1.2, nil
		}
		return 1.0, nil
	}
	var provider FactorProviderFunc = fn

	t.Run("specific_industry", func(t *testing.T) {
		m, err := provider.GetFactorTilt(ctx, "semiconductor")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.2 {
			t.Errorf("expected 1.2, got %f", m)
		}
	})

	t.Run("unknown_industry", func(t *testing.T) {
		m, err := provider.GetFactorTilt(ctx, "unknown")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m != 1.0 {
			t.Errorf("expected 1.0, got %f", m)
		}
	})
}
