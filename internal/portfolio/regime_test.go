package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestStyleAllocation_Validate(t *testing.T) {
	tests := []struct {
		name  string
		alloc StyleAllocation
		want  bool
	}{
		{"valid", StyleAllocation{0.4, 0.2, 0.3, 0.1}, true},
		{"under 1", StyleAllocation{0.1, 0.1, 0.1, 0.1}, false},
		{"over 1", StyleAllocation{0.5, 0.5, 0.5, 0.5}, false},
		{"just over", StyleAllocation{0.4, 0.3, 0.3, 0.02}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.alloc.Validate(); got != tt.want {
				t.Errorf("Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultRegimeConfigs(t *testing.T) {
	cfgs := DefaultRegimeConfigs()
	if len(cfgs) != 3 {
		t.Fatalf("expected 3 regimes, got %d", len(cfgs))
	}
	for _, r := range []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral, domain.RegimeRiskOff} {
		if _, ok := cfgs[r]; !ok {
			t.Errorf("missing config for regime %s", r)
		}
	}
	if !cfgs[domain.RegimeRiskOn].Allocation.Validate() {
		t.Error("RiskOn allocation must validate")
	}
	if !cfgs[domain.RegimeNeutral].Allocation.Validate() {
		t.Error("Neutral allocation must validate")
	}
	if !cfgs[domain.RegimeRiskOff].Allocation.Validate() {
		t.Error("RiskOff allocation must validate")
	}
}

func TestRegimeAllocator(t *testing.T) {
	a := NewRegimeAllocator()

	t.Run("initial neutral", func(t *testing.T) {
		if r := a.GetCurrentRegime(); r != domain.RegimeNeutral {
			t.Errorf("initial regime = %s, want NEUTRAL", r)
		}
	})

	t.Run("set and get regime", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		if r := a.GetCurrentRegime(); r != domain.RegimeRiskOn {
			t.Errorf("regime = %s, want RISK_ON", r)
		}
	})

	t.Run("allocation per regime", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		alloc := a.GetAllocation()
		if alloc.Growth < 0.3 {
			t.Errorf("RiskOn growth = %f, expected >= 0.3", alloc.Growth)
		}
		a.SetRegime(domain.RegimeRiskOff)
		alloc = a.GetAllocation()
		if alloc.Value < 0.3 {
			t.Errorf("RiskOff value = %f, expected >= 0.3", alloc.Value)
		}
	})

	t.Run("max exposure", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		if e := a.GetMaxExposure(); e < 0.9 {
			t.Errorf("RiskOn exposure = %f, expected >= 0.9", e)
		}
		a.SetRegime(domain.RegimeRiskOff)
		if e := a.GetMaxExposure(); e > 0.6 {
			t.Errorf("RiskOff exposure = %f, expected <= 0.6", e)
		}
	})

	t.Run("cash reserve", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOff)
		if c := a.GetCashReserve(); c < 0.3 {
			t.Errorf("RiskOff cash = %f, expected >= 0.3", c)
		}
	})

	t.Run("is risk on", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		if !a.IsRiskOn() {
			t.Error("expected RiskOn = true")
		}
		a.SetRegime(domain.RegimeRiskOff)
		if a.IsRiskOn() {
			t.Error("expected RiskOn = false for RiskOff")
		}
	})

	t.Run("style weight", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		if w := a.GetStyleWeight(StyleGrowth); w <= 0 {
			t.Errorf("growth weight = %f, expected positive", w)
		}
		if w := a.GetStyleWeight(Style("unknown")); w != 0 {
			t.Errorf("unknown style weight = %f, want 0", w)
		}
	})

	t.Run("description", func(t *testing.T) {
		a.SetRegime(domain.RegimeNeutral)
		if d := a.GetStyleDescription(); d == "" {
			t.Error("description is empty")
		}
	})

	t.Run("update config", func(t *testing.T) {
		a.UpdateConfig(domain.RegimeNeutral, RegimeConfig{
			Allocation:  StyleAllocation{0.25, 0.25, 0.25, 0.25},
			MaxExposure: 0.85, CashReserve: 0.15, RiskOn: false, Description: "custom",
		})
		if d := a.GetStyleDescription(); d != "custom" {
			t.Errorf("description = %q, want custom", d)
		}
	})

	t.Run("get regime config for unknown", func(t *testing.T) {
		cfg := a.GetRegimeConfig("nonexistent")
		if cfg.Description == "" {
			t.Error("should fall back to neutral config")
		}
	})

	t.Run("get current config", func(t *testing.T) {
		a.SetRegime(domain.RegimeRiskOn)
		cfg := a.GetCurrentConfig()
		if !cfg.RiskOn {
			t.Error("RiskOn config should have RiskOn=true")
		}
	})
}

func TestDefaultRegimeThresholds(t *testing.T) {
	th := DefaultRegimeThresholds()
	if th.RiskOnRSI <= th.RiskOffRSI {
		t.Error("RiskOnRSI must be > RiskOffRSI")
	}
	if th.VIXHigh <= th.VIXLow {
		t.Error("VIXHigh must be > VIXLow")
	}
}

func TestRegimeDetector(t *testing.T) {
	d := NewRegimeDetector()

	tests := []struct {
		name string
		ind  MarketIndicators
		want domain.Regime
	}{
		{"risk_on", MarketIndicators{RSI: 70, VIX: 10}, domain.RegimeRiskOn},
		{"risk_off_rsi", MarketIndicators{RSI: 30, VIX: 20}, domain.RegimeRiskOff},
		{"risk_off_vix", MarketIndicators{RSI: 50, VIX: 30}, domain.RegimeRiskOff},
		{"neutral", MarketIndicators{RSI: 50, VIX: 20}, domain.RegimeNeutral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.Detect(tt.ind); got != tt.want {
				t.Errorf("Detect(%+v) = %s, want %s", tt.ind, got, tt.want)
			}
		})
	}

	t.Run("set thresholds", func(t *testing.T) {
		d.SetThresholds(RegimeThresholds{RiskOnRSI: 80, RiskOffRSI: 20, VIXHigh: 30, VIXLow: 10})
		if got := d.Detect(MarketIndicators{RSI: 85, VIX: 8}); got != domain.RegimeRiskOn {
			t.Errorf("custom threshold: got %s, want RISK_ON", got)
		}
	})
}

func TestIntegratedAllocator(t *testing.T) {
	i := NewIntegratedAllocator()

	t.Run("update and detect", func(t *testing.T) {
		r := i.UpdateAndDetect(MarketIndicators{RSI: 70, VIX: 10})
		if r != domain.RegimeRiskOn {
			t.Errorf("got %s, want RISK_ON", r)
		}
		if i.GetAllocator().GetCurrentRegime() != domain.RegimeRiskOn {
			t.Error("allocator should be updated")
		}
	})

	t.Run("get allocator", func(t *testing.T) {
		if i.GetAllocator() == nil {
			t.Error("GetAllocator returned nil")
		}
	})

	t.Run("get detector", func(t *testing.T) {
		if i.GetDetector() == nil {
			t.Error("GetDetector returned nil")
		}
	})

	t.Run("current allocation", func(t *testing.T) {
		alloc := i.GetCurrentAllocation()
		if !alloc.Validate() {
			t.Error("allocation should be valid")
		}
	})
}
