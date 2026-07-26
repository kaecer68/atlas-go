package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

func TestPeriodDetector_BlackSwan(t *testing.T) {
	d := NewPeriodDetector()

	tests := []struct {
		name string
		ind  PeriodIndicators
		want domain.MarketPeriod
	}{
		{
			name: "VIX spike alone triggers black swan",
			ind: PeriodIndicators{
				VIX: 36,
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "foreign panic sell triggers black swan",
			ind: PeriodIndicators{
				ForeignSingleDayNet: -55_000_000_000, // 550億賣超
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "national fund intervention triggers black swan",
			ind: PeriodIndicators{
				NationalFundActive: true,
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "TWD panic depreciation triggers black swan",
			ind: PeriodIndicators{
				TWDChange1D: 0.6,
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "no black swan conditions returns next period",
			ind: PeriodIndicators{
				VIX:         20,
				TAIEXPrice:  18000,
				TAIEXMA20:   18000,
				TWDChange1D: 0.1,
			},
			want: domain.PeriodConsolidation, // falls through to default
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.DetectPeriod(tt.ind)
			if got != tt.want {
				t.Errorf("DetectPeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPeriodDetector_TurnaroundDown(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not black swan (VIX below 35)
		VIX: 28,
		// Turnaround down conditions
		ForeignConsecSellDays:  4,
		ForeignSingleDayNet:    -20_000_000_000, // 200億賣超
		TWDMA20:                31.5,
		TWDChange1D:            0.4, // depreciating
		MarginMaintenanceRatio: 145,
		SOXPrice:               4200,
		SOXMA50:                4500, // below 50-day
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundDown {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodTurnaroundDown)
	}
}

func TestPeriodDetector_Downturn(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not black swan
		VIX: 28,
		// Not turnaround down (margin OK)
		MarginMaintenanceRatio: 160,
		SOXPrice:               5000,
		SOXMA50:                4500,
		// Downturn conditions
		ForeignNet5DayAvg:       -3_000_000_000,  // 30億賣超
		ForeignNetPeakSell:      -15_000_000_000, // 前波150億峰值, 30億/150億 = 20% < 30%
		MarginBalance:           1800,
		MarginBalancePeak:       2200, // down 18% > 15%
		PublicBankConsecBuyDays: 6,
		TAIEXPrice:              17000,
		TAIEXMA5:                16900,
		TAIEXMA20:               17200, // above 5-day, below 20-day
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodDownturn {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodDownturn)
	}
}

func TestPeriodDetector_TurnaroundUp(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not extreme states
		VIX: 22,
		// Turnaround up: 2+ hits
		ForeignSingleDayNet:  15_000_000_000, // 150億買超
		ForeignConsecBuyDays: 3,
		TWDChange1D:          -0.4, // 升值0.4%
		SOXPrice:             4800,
		SOXMA50:              4500, // above 50-day
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundUp {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodTurnaroundUp)
	}
}

func TestPeriodDetector_Bull(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not extreme/transition states
		VIX:                 18,
		ForeignSingleDayNet: 5_000_000_000,
		SOXPrice:            5000,
		SOXMA50:             4500,
		TWDChange1D:         -0.1,
		// Bull conditions
		ForeignBuyDays10:      8,
		ForeignFuturesOI:      35000,
		MarginBalanceChange5D: 2.0, // mild increase
		TAIEXPrice:            18500,
		TAIEXMA20:             18000,
		TAIEXMA20Slope:        0.01, // positive
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodBull {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodBull)
	}
}

func TestPeriodDetector_Plateau(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not bull (foreign buy ratio too low)
		VIX:              20,
		ForeignBuyDays10: 5,
		SOXPrice:         5000,
		SOXMA50:          4800,
		TWDChange1D:      -0.1,
		// Plateau conditions
		ForeignNet5DayAvg:      2_000_000_000,
		ForeignNet10DayAvg:     5_000_000_000, // 2B/5B = 40% < 50%
		ForeignFuturesOIDelta3: -3,
		DayTradeRatio:          38,
		TAIEXPrice:             18100,
		TAIEXMA20:              18000, // +0.55%, within ±2%
		SectorRotationFlag:     true,
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodPlateau {
		t.Errorf("DetectPeriod() = %v, want %v (got bull=%v)", got, domain.PeriodPlateau,
			d.isBull(ind))
	}
}

func TestPeriodDetector_Consolidation(t *testing.T) {
	d := NewPeriodDetector()

	ind := PeriodIndicators{
		// Not plateau (day trade low)
		VIX:                18,
		ForeignBuyDays10:   4,
		ForeignSellDays10:  5,
		TWDMA20:            31.0,
		TWDChange5D:        0.2,
		SectorRotationFlag: true,
		MarketVolume:       90_000_000_000,
		MarketVolumeMA20:   100_000_000_000, // 90%
		TAIEXPrice:         18000,
		TAIEXMA20:          18000,
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodConsolidation {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodConsolidation)
	}
}

func TestPeriodDetector_DefaultFallback(t *testing.T) {
	d := NewPeriodDetector()

	// All zero — insufficient data
	ind := PeriodIndicators{}
	got := d.DetectPeriod(ind)
	if got != domain.PeriodConsolidation {
		t.Errorf("DetectPeriod() with zero data = %v, want %v (default fallback)", got, domain.PeriodConsolidation)
	}
}

func TestPeriodToRegime(t *testing.T) {
	tests := []struct {
		period domain.MarketPeriod
		want   domain.Regime
	}{
		{domain.PeriodBull, domain.RegimeRiskOn},
		{domain.PeriodTurnaroundUp, domain.RegimeNeutral},
		{domain.PeriodPlateau, domain.RegimeNeutral},
		{domain.PeriodConsolidation, domain.RegimeNeutral},
		{domain.PeriodDownturn, domain.RegimeRiskOff},
		{domain.PeriodTurnaroundDown, domain.RegimeRiskOff},
		{domain.PeriodBlackSwan, domain.RegimeRiskOff},
	}
	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			got := PeriodToRegime(tt.period)
			if got != tt.want {
				t.Errorf("PeriodToRegime(%v) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestPeriodToRiskLevel(t *testing.T) {
	tests := []struct {
		period domain.MarketPeriod
		want   macroflow.RiskLevel
	}{
		{domain.PeriodBull, macroflow.RiskYellow},
		{domain.PeriodTurnaroundUp, macroflow.RiskYellow},
		{domain.PeriodPlateau, macroflow.RiskYellow},
		{domain.PeriodConsolidation, macroflow.RiskYellow},
		{domain.PeriodDownturn, macroflow.RiskOrange},
		{domain.PeriodTurnaroundDown, macroflow.RiskOrange},
		{domain.PeriodBlackSwan, macroflow.RiskRed},
	}
	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			got := PeriodToRiskLevel(tt.period)
			if got != tt.want {
				t.Errorf("PeriodToRiskLevel(%v) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestPeriodToRegime_AllPeriodsCovered(t *testing.T) {
	// Verify every MarketPeriod constant maps to a valid Regime
	allPeriods := []domain.MarketPeriod{
		domain.PeriodDownturn,
		domain.PeriodTurnaroundUp,
		domain.PeriodBull,
		domain.PeriodPlateau,
		domain.PeriodConsolidation,
		domain.PeriodTurnaroundDown,
		domain.PeriodBlackSwan,
	}
	for _, p := range allPeriods {
		r := PeriodToRegime(p)
		if r != domain.RegimeRiskOn && r != domain.RegimeRiskOff && r != domain.RegimeNeutral {
			t.Errorf("PeriodToRegime(%v) = %v, must map to RISK_ON/RISK_OFF/NEUTRAL", p, r)
		}
	}
}

func TestDetectionPriority_BlackSwanOverridesAll(t *testing.T) {
	d := NewPeriodDetector()

	// Indicators that would normally be bull market
	ind := PeriodIndicators{
		VIX:                   36, // black swan trigger
		ForeignBuyDays10:      9,  // bull
		ForeignFuturesOI:      40000,
		MarginBalanceChange5D: 1.0,
		TAIEXPrice:            19000,
		TAIEXMA20:             18500,
		TAIEXMA20Slope:        0.02,
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodBlackSwan {
		t.Errorf("Black swan should override bull: got %v, want %v", got, domain.PeriodBlackSwan)
	}
}

func TestDetectionPriority_TurnaroundDownOverDownturn(t *testing.T) {
	d := NewPeriodDetector()

	// Indicators that satisfy both turnaround_down and downturn
	ind := PeriodIndicators{
		VIX:                    32,
		ForeignConsecSellDays:  5,
		ForeignSingleDayNet:    -20_000_000_000,
		MarginMaintenanceRatio: 140,
		SOXPrice:               4200,
		SOXMA50:                4600,
		// Also satisfy downturn
		ForeignNet5DayAvg:       -5_000_000_000,
		ForeignNetPeakSell:      -20_000_000_000,
		MarginBalance:           1700,
		MarginBalancePeak:       2100,
		PublicBankConsecBuyDays: 6,
		TAIEXPrice:              16800,
		TAIEXMA5:                16700,
		TAIEXMA20:               17100,
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundDown {
		t.Errorf("Turnaround down should override downturn: got %v, want %v", got, domain.PeriodTurnaroundDown)
	}
}
