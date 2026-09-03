// Package config — period detection threshold configuration.
//
// PeriodDetectionConfig holds all threshold values used by PeriodDetector
// to classify market periods. Defaults follow ATLAS_METHODOLOGY.md §3.
//
// Maturity: E (evolving)
package config

// PeriodDetectionConfig holds thresholds for seven-period market cycle detection.
// Zero values mean "use default" — Callers should use DefaultPeriodDetectionConfig()
// or apply overrides selectively.
type PeriodDetectionConfig struct {
	// ── Black Swan ──
	BlackSwanForeignSellBillion float64 // 外資單日賣超 > N 億元
	BlackSwanVIX                float64 // VIX > N
	BlackSwanTAIEXDeclinePct    float64 // 加權指數單日跌幅 > N%
	BlackSwanTWDDepreciationPct float64 // 新台幣單日重貶 > N%
	BlackSwanGeoIntensity       float64 // 地緣（台海）風險強度 > N（0-100，憲章 §3 黑天鵝）

	// ── Turnaround Down ──
	TurnDownConsecSellDays    int     // 外資連續賣超天數
	TurnDownSingleSellBillion float64 // 單日賣超 > N 億元
	TurnDownMarginMaintRatio  float64 // 融資維持率 < N%
	TurnDownFuturesOIDecrease int     // 期貨未平倉減少 > N 口
	TurnDownGeoIntensity      float64 // 地緣（台海）風險強度 > N（0-100，憲章 §3 轉折下壓）

	// ── Downturn ──
	DownturnSellRatioToPeak    float64 // 5日均值 / 峰值 < N
	DownturnMarginReductionPct float64 // 融資較高點減少 > N%
	DownturnPublicBankBuyDays  int     // 公股連續買超 > N 日
	DownturnVIXMin             float64 // VIX > N

	// ── Turnaround Up ──
	TurnUpSingleBuyBillion  float64 // 外資單日買超 > N 億元
	TurnUpConsecBuyDays     int     // 外資連續買超天數
	TurnUpTWDApprec1DPct    float64 // 新台幣單日升值 > N% (負值)
	TurnUpTWDApprec3DPct    float64 // 新台幣3日累積升值 > N%
	TurnUpFuturesOIIncrease int     // 期貨多單增加 > N 口
	TurnUpTSMADRPct         float64 // TSM ADR 單日漲幅 > N%

	// ── Bull ──
	BullForeignBuyRatio10 float64 // 近10日買超佔比 > N
	BullFuturesOIMin      int     // 期貨多單 > N 口
	BullMarginDailyMaxPct float64 // 融資日均增幅 < N%

	// ── Plateau ──
	PlateauBuyRatio3to10     float64 // 3日均值 / 10日均值 < N
	PlateauDayTradeMinPct    float64 // 當沖佔比 > N%
	PlateauTAIEXDeviationPct float64 // 指數偏離20日線 < ±N%

	// ── Consolidation ──
	ConsolidationBuyDaysMin  int     // 近10日買超天數 > N
	ConsolidationSellDaysMin int     // 近10日賣超天數 > N
	ConsolidationTWDBandPct  float64 // 新台幣偏離月線 < ±N%
	ConsolidationVolRatioMin float64 // 成交量 / 20日均量 > N
	ConsolidationVolRatioMax float64 // 成交量 / 20日均量 < N

	// ── P1 grading + P2 state machine (PR-3b, plan v1.1) ──
	// BlackSwanMinConditions: black swan fires only when at least N of the 6
	// conditions hit, OR when a single extreme condition hits (see the two
	// multipliers below). Default 2 fixes audit P1 ("OR 邏輯過敏").
	BlackSwanMinConditions int
	// BlackSwanExtremeVIXMultiplier: a single VIX reading ≥
	// BlackSwanVIX × N counts as an extreme condition that alone fires
	// black swan. Default 1.5 (VIX ≥ 52.5 with BlackSwanVIX=35).
	BlackSwanExtremeVIXMultiplier float64
	// BlackSwanExtremeForeignSellMultiplier: a single foreign net-sell ≥
	// BlackSwanForeignSellBillion × N (億元) counts as extreme. Default 2.0.
	// NationalFundActive is treated as inherently extreme: the government
	// only officially activates the fund in genuine market crises (A1/R8),
	// so it alone still fires black swan.
	BlackSwanExtremeForeignSellMultiplier float64
	// PeriodMinStayDays: once a period is confirmed, it stays for at least
	// N trading days before a transition is allowed (audit P2 最小停留期).
	PeriodMinStayDays int
	// PeriodConfirmDays: a candidate period must be observed on N
	// consecutive days before the state machine switches (轉移遲滯).
	PeriodConfirmDays int
}

// DefaultPeriodDetectionConfig returns thresholds matching ATLAS_METHODOLOGY.md §3.
func DefaultPeriodDetectionConfig() PeriodDetectionConfig {
	return PeriodDetectionConfig{
		// Black Swan
		BlackSwanForeignSellBillion: 500,
		BlackSwanVIX:                35,
		BlackSwanTAIEXDeclinePct:    5,
		BlackSwanTWDDepreciationPct: 0.5,
		BlackSwanGeoIntensity:       60, // 地緣強度 ≥60（4 級制 ≥ 高張(3)）— 憲章 §3 黑天鵝

		// Turnaround Down
		TurnDownConsecSellDays:    3,
		TurnDownSingleSellBillion: 150,
		TurnDownMarginMaintRatio:  150,
		TurnDownFuturesOIDecrease: 10000,
		TurnDownGeoIntensity:      40, // 地緣強度 ≥40（4 級制 ≥ 升溫(2)）— 憲章 §3 轉折下壓

		// Downturn
		DownturnSellRatioToPeak:    0.30,
		DownturnMarginReductionPct: 0.15,
		DownturnPublicBankBuyDays:  5,
		DownturnVIXMin:             25,

		// Turnaround Up
		TurnUpSingleBuyBillion:  100,
		TurnUpConsecBuyDays:     3,
		TurnUpTWDApprec1DPct:    -0.3,
		TurnUpTWDApprec3DPct:    -0.5,
		TurnUpFuturesOIIncrease: 3000,
		TurnUpTSMADRPct:         2.0,

		// Bull
		BullForeignBuyRatio10: 0.7,
		BullFuturesOIMin:      30000,
		BullMarginDailyMaxPct: 1.0,

		// Plateau
		PlateauBuyRatio3to10:     0.50,
		PlateauDayTradeMinPct:    35,
		PlateauTAIEXDeviationPct: 2.0,

		// Consolidation
		ConsolidationBuyDaysMin:  3,
		ConsolidationSellDaysMin: 3,
		ConsolidationTWDBandPct:  0.5,
		ConsolidationVolRatioMin: 0.7,
		ConsolidationVolRatioMax: 1.0,

		// P1 grading + P2 state machine (PR-3b)
		BlackSwanMinConditions:                2,
		BlackSwanExtremeVIXMultiplier:         1.5,
		BlackSwanExtremeForeignSellMultiplier: 2.0,
		PeriodMinStayDays:                     3,
		PeriodConfirmDays:                     2,
	}
}
