package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

func TestPeriodDetector_BlackSwan(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	// PR-3b (audit P1): black swan is graded — ≥ BlackSwanMinConditions (2)
	// conditions, OR a single EXTREME condition (VIX ≥ 35×1.5, foreign
	// net-sell ≥ 500億×2). Single weak signals no longer fire.
	tests := []struct {
		name string
		ind  PeriodIndicators
		want domain.MarketPeriod
	}{
		{
			name: "single weak VIX spike no longer triggers black swan",
			ind: PeriodIndicators{
				VIX: 36, // > 35 but < 35×1.5 extreme bar
			},
			want: domain.PeriodConsolidation, // falls through to default
		},
		{
			name: "two conditions trigger black swan",
			ind: PeriodIndicators{
				VIX:          36,
				GeoIntensity: 65,
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "extreme VIX alone triggers black swan",
			ind: PeriodIndicators{
				VIX: 54, // ≥ 35×1.5
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "extreme foreign panic sell alone triggers black swan",
			ind: PeriodIndicators{
				ForeignSingleDayNet: -1_050_000_000_000, // 1050億 ≥ 500×2
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "single foreign panic sell (550億) no longer triggers",
			ind: PeriodIndicators{
				ForeignSingleDayNet: -55_000_000_000, // 550億 < 500×2 extreme bar
			},
			want: domain.PeriodConsolidation,
		},
		{
			name: "national fund intervention alone still triggers (extreme, A1/R8)",
			ind: PeriodIndicators{
				NationalFundActive: true,
			},
			want: domain.PeriodBlackSwan,
		},
		{
			name: "TWD panic depreciation alone no longer triggers",
			ind: PeriodIndicators{
				TWDChange1D: 0.6,
			},
			want: domain.PeriodConsolidation,
		},
		{
			name: "geopolitical crisis intensity alone no longer triggers (G5)",
			ind: PeriodIndicators{
				GeoIntensity: 75, // 4 級制 ≥ 高張(3)；閾值 60 — weak single signal
			},
			want: domain.PeriodConsolidation,
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
	d := NewPeriodDetectorWithDefaults()

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
		SOXMA50:                5200, // below 50-day
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundDown {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodTurnaroundDown)
	}
}

// TestPeriodDetector_TurnaroundDown_Geo covers the G5 geopolitical condition:
// 地緣緊張升溫（GeoIntensity ≥ 40）可與既有條件共同觸發轉折下壓。
func TestPeriodDetector_TurnaroundDown_Geo(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		// 2 hits from classic conditions + 1 from geopolitical = 3/6
		MarginMaintenanceRatio: 145,  // 融資維持率 < 150%
		SOXPrice:               4200, // 跌破 50 日線
		SOXMA50:                5200,
		GeoIntensity:           55, // 地緣緊張升溫（≥ 40）
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundDown {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodTurnaroundDown)
	}
}

// TestPeriodDetector_GeoDecliningTrend ensures the 地緣升溫 condition requires
// a non-declining 5-day trend (GeoIntensityChange5D >= 0): high intensity with
// a falling trend must NOT fire the condition (憲章 v1.1 5 日趨勢).
func TestPeriodDetector_GeoDecliningTrend(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		MarginMaintenanceRatio: 145,  // 融資維持率 < 150%
		SOXPrice:               4200, // 跌破 50 日線
		SOXMA50:                5200,
		GeoIntensity:           55,  // ≥ 40，但...
		GeoIntensityChange5D:   -20, // ...5 日趨勢顯著下降 → 地緣條件不成立
	}

	// Only 2/6 conditions → NOT turnaround_down.
	if got := d.DetectPeriod(ind); got == domain.PeriodTurnaroundDown {
		t.Errorf("DetectPeriod() = %v, want NOT turnaround_down (地緣 5 日趨勢下降不觸發)", got)
	}
}

// TestPeriodDetector_GeoBelowThreshold ensures geopolitical intensity below
// the turnaround-down threshold does NOT fire the condition on its own.
func TestPeriodDetector_GeoBelowThreshold(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		GeoIntensity: 25, // 平靜(1) — 低於 40
	}

	got := d.DetectPeriod(ind)
	if got == domain.PeriodTurnaroundDown {
		t.Errorf("DetectPeriod() = %v, want NOT turnaround_down for calm geo", got)
	}
}

func TestPeriodDetector_Downturn(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		// Not black swan
		VIX: 28,
		// Not turnaround down (margin OK)
		MarginMaintenanceRatio: 160,
		SOXPrice:               5000,
		SOXMA50:                5200,
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
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		// Not extreme states
		VIX: 22,
		// Turnaround up: 2+ hits
		ForeignSingleDayNet:  15_000_000_000, // 150億買超
		ForeignConsecBuyDays: 3,
		TWDChange1D:          -0.4, // 升值0.4%
		SOXPrice:             4800,
		SOXMA50:              5200, // above 50-day
	}

	got := d.DetectPeriod(ind)
	if got != domain.PeriodTurnaroundUp {
		t.Errorf("DetectPeriod() = %v, want %v", got, domain.PeriodTurnaroundUp)
	}
}

func TestPeriodDetector_Bull(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	ind := PeriodIndicators{
		// Not extreme/transition states
		VIX:                 18,
		ForeignSingleDayNet: 5_000_000_000,
		SOXPrice:            5000,
		SOXMA50:             5200,
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
	d := NewPeriodDetectorWithDefaults()

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
	d := NewPeriodDetectorWithDefaults()

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
	d := NewPeriodDetectorWithDefaults()

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
	d := NewPeriodDetectorWithDefaults()

	// Indicators that would normally be bull market. PR-3b: two graded
	// black-swan conditions (VIX 36 + foreign sell 550億) still override.
	ind := PeriodIndicators{
		VIX:                   36,              // black swan trigger 1
		ForeignSingleDayNet:   -55_000_000_000, // 550億賣超 — trigger 2
		ForeignBuyDays10:      9,               // bull
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
	d := NewPeriodDetectorWithDefaults()

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

func TestDetectAssessment_IsFallback(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()

	// 零資料：所有欄位 0 → consolidation + IsFallback=true
	empty := PeriodIndicators{}
	a1, err := d.DetectAssessment(empty)
	if err != nil {
		t.Fatalf("DetectAssessment empty: %v", err)
	}
	if a1.MarketPeriod != domain.PeriodConsolidation {
		t.Errorf("empty → want consolidation, got %s", a1.MarketPeriod)
	}
	if !a1.IsFallback {
		t.Error("empty indicators → IsFallback should be true (no data fallback)")
	}

	// 有資料：TAIEX 存在 → 不應是 fallback
	withData := PeriodIndicators{TAIEXPrice: 23000, TAIEXMA20: 22000, ForeignFuturesOI: 30000, MarketVolume: 4500}
	a2, err := d.DetectAssessment(withData)
	if err != nil {
		t.Fatalf("DetectAssessment withData: %v", err)
	}
	if a2.IsFallback {
		t.Errorf("with data → IsFallback should be false (got period=%s)", a2.MarketPeriod)
	}

	// 部分資料（只有 VIX）→ 非 fallback
	partial := PeriodIndicators{VIX: 18.5}
	a3, err := d.DetectAssessment(partial)
	if err != nil {
		t.Fatalf("DetectAssessment partial: %v", err)
	}
	if a3.IsFallback {
		t.Error("partial data (VIX) → IsFallback should be false")
	}
}

// ===========================================================================
// PR-3b — P2 state machine: minimal stay + transition hysteresis.
// A jitter sequence (bull → plateau → bull) must be smoothed; a candidate
// period confirmed on PeriodConfirmDays consecutive days after
// PeriodMinStayDays transitions.
// ===========================================================================

func TestDetectAssessmentWithState_JitterSmoothed(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()
	bullInd := PeriodIndicators{
		VIX:                   18,
		ForeignBuyDays10:      9,
		ForeignFuturesOI:      40000,
		MarginBalanceChange5D: 1.0,
		TAIEXPrice:            19000,
		TAIEXMA20:             18500,
		TAIEXMA20Slope:        0.02,
	}
	plateauInd := PeriodIndicators{
		VIX:                    20,
		ForeignNet5DayAvg:      2_000_000_000,
		ForeignNet10DayAvg:     5_000_000_000, // 40% < 50% → plateau cond 1
		ForeignFuturesOIDelta3: -3,            // cond 2
		DayTradeRatio:          40,            // cond 3
		TAIEXPrice:             18100,
		TAIEXMA20:              18000, // within ±2% → cond 4
		SectorRotationFlag:     true,  // cond 5
	}

	var state PeriodDetectorState
	// Day 1: bull adopted immediately.
	ass, s, err := d.DetectAssessmentWithState(state, bullInd)
	if err != nil {
		t.Fatal(err)
	}
	state = s
	if ass.MarketPeriod != domain.PeriodBull {
		t.Fatalf("day1 = %v, want bull", ass.MarketPeriod)
	}
	// Day 2: single plateau reading is smoothed back to bull.
	ass, s, _ = d.DetectAssessmentWithState(state, plateauInd)
	state = s
	if ass.MarketPeriod != domain.PeriodBull {
		t.Errorf("day2 (jitter) = %v, want held bull", ass.MarketPeriod)
	}
	// Day 3: back to bull — candidate cleared, still bull.
	ass, s, _ = d.DetectAssessmentWithState(state, bullInd)
	state = s
	if ass.MarketPeriod != domain.PeriodBull {
		t.Errorf("day3 = %v, want bull", ass.MarketPeriod)
	}
}

func TestDetectAssessmentWithState_ConfirmedTransitionAfterHysteresis(t *testing.T) {
	d := NewPeriodDetectorWithDefaults()
	bullInd := PeriodIndicators{
		VIX:                   18,
		ForeignBuyDays10:      9,
		ForeignFuturesOI:      40000,
		MarginBalanceChange5D: 1.0,
		TAIEXPrice:            19000,
		TAIEXMA20:             18500,
		TAIEXMA20Slope:        0.02,
	}
	plateauInd := PeriodIndicators{
		VIX:                    20,
		ForeignNet5DayAvg:      2_000_000_000,
		ForeignNet10DayAvg:     5_000_000_000, // 40% < 50% → plateau cond 1
		ForeignFuturesOIDelta3: -3,            // cond 2
		DayTradeRatio:          40,            // cond 3
		TAIEXPrice:             18100,
		TAIEXMA20:              18000, // within ±2% → cond 4
		SectorRotationFlag:     true,  // cond 5
	}

	var state PeriodDetectorState
	ass, s, _ := d.DetectAssessmentWithState(state, bullInd)
	state = s
	if ass.MarketPeriod != domain.PeriodBull {
		t.Fatalf("day1 = %v, want bull", ass.MarketPeriod)
	}
	// Day 2: plateau observed once — held (confirm needs 2 consecutive days,
	// min-stay needs 3 days in bull).
	ass, s, _ = d.DetectAssessmentWithState(state, plateauInd)
	state = s
	if ass.MarketPeriod != domain.PeriodBull {
		t.Errorf("day2 = %v, want held bull (min-stay)", ass.MarketPeriod)
	}
	// Day 3: plateau observed twice (confirm satisfied) AND bull held for
	// 3 days (min-stay satisfied) → confirmed transition.
	ass, s, _ = d.DetectAssessmentWithState(state, plateauInd)
	state = s
	if ass.MarketPeriod != domain.PeriodPlateau {
		t.Errorf("day3 = %v, want confirmed plateau transition", ass.MarketPeriod)
	}
}

func TestStatefulPeriodDetector_DebrisCrossCalls(t *testing.T) {
	d := NewStatefulPeriodDetectorWithDefaults()
	bullInd := PeriodIndicators{
		VIX:                   18,
		ForeignBuyDays10:      9,
		ForeignFuturesOI:      40000,
		MarginBalanceChange5D: 1.0,
		TAIEXPrice:            19000,
		TAIEXMA20:             18500,
		TAIEXMA20Slope:        0.02,
	}
	plateauInd := PeriodIndicators{
		VIX:                    20,
		ForeignNet5DayAvg:      2_000_000_000,
		ForeignNet10DayAvg:     5_000_000_000, // 40% < 50% → plateau cond 1
		ForeignFuturesOIDelta3: -3,            // cond 2
		DayTradeRatio:          40,            // cond 3
		TAIEXPrice:             18100,
		TAIEXMA20:              18000, // within ±2% → cond 4
		SectorRotationFlag:     true,  // cond 5
	}
	if _, err := d.DetectAssessmentDebounced(bullInd); err != nil {
		t.Fatal(err)
	}
	ass, err := d.DetectAssessmentDebounced(plateauInd)
	if err != nil {
		t.Fatal(err)
	}
	if ass.MarketPeriod != domain.PeriodBull {
		t.Errorf("stateful detector jitter = %v, want held bull", ass.MarketPeriod)
	}
}
