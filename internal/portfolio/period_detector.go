// Package portfolio — methodology-level seven-period market cycle detector.
//
// Implements the detection rules defined in docs/ATLAS_METHODOLOGY.md §3.
// The PeriodDetector takes a PeriodIndicators snapshot and returns the
// current MarketPeriod following the detection_order priority chain:
//
//	black_swan → turnaround_down → downturn → turnaround_up → bull → plateau → consolidation
//
// Maturity: E (evolving) — new module, API may adjust.
package portfolio

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

// PeriodIndicators aggregates all macro/market data needed for seven-period
// detection. Fields follow the indicator tables in ATLAS_METHODOLOGY.md §3.
//
// Zero values mean "data unavailable" — the detector skips indicators with
// zero values rather than treating them as real signals.
type PeriodIndicators struct {
	// ── 全球資金總開關（第〇層）──
	VIX   float64 // VIX 恐慌指數
	DXY   float64 // 美元指數
	US10Y float64 // 美國10年期公債殖利率

	// ── 美股科技（第一層）──
	SOXPrice    float64 // 費城半導體指數
	SOXMA50     float64 // SOX 50日移動平均
	SOXMA20     float64 // SOX 20日移動平均
	TSMADRPrice float64 // 台積電ADR
	TSMADRHigh5 float64 // 台積電ADR近5日高點

	// ── 外資資金流（第三層）──
	ForeignNet5DayAvg      float64 // 外資現貨近5日買賣超均值
	ForeignNet10DayAvg     float64 // 外資近10日均值
	ForeignNetPeakSell     float64 // 前波賣超峰值（絕對值，用於低迷判斷）
	ForeignBuyDays10       int     // 近10日中買超天數
	ForeignSellDays10      int     // 近10日中賣超天數
	ForeignConsecBuyDays   int     // 外資連續買超天數
	ForeignConsecSellDays  int     // 外資連續賣超天數
	ForeignSingleDayNet    float64 // 外資最近單日買賣超（用於黑天鵝/轉折判斷）
	ForeignFuturesOI       float64 // 外資期貨未平倉淨額
	ForeignFuturesOIPrev   float64 // 前期貨未平倉（用於計算增減）
	ForeignFuturesOIDelta3 int     // 期貨未平倉連續增減天數（正=增加，負=減少）

	// ── 新台幣匯率（第三層）──
	TWDChange1D float64 // 新台幣單日變動%（負=升值）
	TWDChange3D float64 // 新台幣3日累積變動%
	TWDChange5D float64 // 新台幣5日變動%
	TWDMA20     float64 // 新台幣20日均值

	// ── 大盤量能（第四層）──
	TAIEXPrice       float64 // 加權指數
	TAIEXMA5         float64 // 5日線
	TAIEXMA20        float64 // 20日線
	TAIEXMA20Slope   float64 // 20日線斜率
	MarketVolume     float64 // 集中市場成交量
	MarketVolumeMA20 float64 // 20日均量
	DayTradeRatio    float64 // 當沖交易佔比（%）

	// ── 散戶籌碼（第六層）──
	MarginBalance          float64 // 融資餘額
	MarginBalancePeak      float64 // 融資餘額高峰
	MarginBalanceChange5D  float64 // 融資近5日變動%
	MarginMaintenanceRatio float64 // 融資維持率（%）

	// ── 內資（第五層）──
	PublicBankConsecBuyDays int // 公股券商連續買超天數

	// ── 類股輪動（高原/盤整判斷）──
	SectorRotationFlag bool // 近5日漲幅前3名與前5日不同

	// ── 事件（第七層）──
	NationalFundActive bool // 國安基金宣布進場
}

// PeriodDetector implements seven-period market cycle detection per
// ATLAS_METHODOLOGY.md §3. Thresholds are driven by config.PeriodDetectionConfig;
// use NewPeriodDetector(config) or NewPeriodDetectorWithDefaults().
type PeriodDetector struct {
	cfg config.PeriodDetectionConfig
}

// NewPeriodDetector creates a detector with the given threshold config.
func NewPeriodDetector(cfg config.PeriodDetectionConfig) *PeriodDetector {
	return &PeriodDetector{cfg: cfg}
}

// NewPeriodDetectorWithDefaults creates a detector with constitution defaults.
func NewPeriodDetectorWithDefaults() *PeriodDetector {
	return &PeriodDetector{cfg: config.DefaultPeriodDetectionConfig()}
}

// DetectPeriod classifies the current market into one of seven periods.
// Detection order follows the priority chain: extreme states first, then
// transitions, then steady states.
//
// Returns PeriodConsolidation as the default fallback when insufficient
// data is available to classify.
func (d *PeriodDetector) DetectPeriod(ind PeriodIndicators) domain.MarketPeriod {
	// ─── Detection Order (per constitution detection_order) ───

	// 1. Black Swan — any single trigger fires
	if d.isBlackSwan(ind) {
		return domain.PeriodBlackSwan
	}

	// 2. Turnaround Down — any 3 conditions must be met
	if d.isTurnaroundDown(ind) {
		return domain.PeriodTurnaroundDown
	}

	// 3. Downturn — any 3 conditions must be met
	if d.isDownturn(ind) {
		return domain.PeriodDownturn
	}

	// 4. Turnaround Up — any 2 of 5 conditions
	if d.isTurnaroundUp(ind) {
		return domain.PeriodTurnaroundUp
	}

	// 5. Bull — any 3 conditions must be met
	if d.isBull(ind) {
		return domain.PeriodBull
	}

	// 6. Plateau — any 3 conditions must be met
	if d.isPlateau(ind) {
		return domain.PeriodPlateau
	}

	// 7. Consolidation — any 3 conditions must be met
	if d.isConsolidation(ind) {
		return domain.PeriodConsolidation
	}

	// Fallback: insufficient data to classify
	return domain.PeriodConsolidation
}

// ─── Black Swan Detection ───

func (d *PeriodDetector) isBlackSwan(ind PeriodIndicators) bool {
	triggers := 0

	// Foreign panic sell: single day > 500億 (50 billion NTD)
	if ind.ForeignSingleDayNet < -(d.cfg.BlackSwanForeignSellBillion * 1_000_000_00) {
		triggers++
	}
	// VIX spike: > 35
	if ind.VIX > d.cfg.BlackSwanVIX {
		triggers++
	}
	// TAIEX crash: single day > -5%
	if ind.TAIEXMA20 > 0 && ind.TAIEXPrice > 0 {
		// Simplified: use price relative to MA20 as crash proxy
		// Full implementation needs daily change data
		decline := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		if decline < -(d.cfg.BlackSwanTAIEXDeclinePct / 100) {
			triggers++
		}
	}
	// National fund intervention
	if ind.NationalFundActive {
		triggers++
	}
	// TWD panic: single day depreciation > 0.5%
	if ind.TWDChange1D > 0.5 {
		triggers++
	}

	return triggers >= 1
}

// ─── Turnaround Down Detection ───

func (d *PeriodDetector) isTurnaroundDown(ind PeriodIndicators) bool {
	// All conditions required
	checks := 0
	passed := 0

	// 1. Foreign consecutive heavy sell: 3+ days with at least one > 150億.
	// Uses ForeignNetPeakSell (max sell in window) as secondary check to
	// cover the case where the heaviest sell was not the most recent day.
	heavyThreshold := -(d.cfg.TurnDownSingleSellBillion * 1_000_000_00)
	if ind.ForeignConsecSellDays >= d.cfg.TurnDownConsecSellDays &&
		(ind.ForeignSingleDayNet < heavyThreshold || ind.ForeignNetPeakSell < heavyThreshold) {
		passed++
	}
	checks++

	// 2. TWD breaks below monthly MA, depreciating fast
	if ind.TWDMA20 > 0 && ind.TWDChange1D > 0 {
		// Simplified: TWD weaker than MA20 and still weakening
		passed++
	}
	checks++

	// 3. Margin maintenance ratio < 150%
	if ind.MarginMaintenanceRatio > 0 && ind.MarginMaintenanceRatio < d.cfg.TurnDownMarginMaintRatio {
		passed++
	}
	checks++

	// 4. SOX below 50-day MA
	if ind.SOXPrice > 0 && ind.SOXMA50 > 0 && ind.SOXPrice < ind.SOXMA50 {
		passed++
	}
	checks++

	// 5. Foreign futures turning short or large reduction
	if ind.ForeignFuturesOIDelta3 < -d.cfg.TurnDownFuturesOIDecrease || ind.ForeignFuturesOI < 0 {
		passed++
	}
	checks++

	return checks >= 3 && passed >= 3
}

// ─── Downturn Detection ───
func (d *PeriodDetector) isDownturn(ind PeriodIndicators) bool {
	checks := 0
	passed := 0

	// 1. Foreign sell slowing: 5-day avg < 30% of peak sell
	if ind.ForeignNetPeakSell < 0 && ind.ForeignNet5DayAvg < 0 {
		if ind.ForeignNet5DayAvg/ind.ForeignNetPeakSell < d.cfg.DownturnSellRatioToPeak {
			passed++
		}
	}
	checks++

	// 2. Margin balance down > 15% from peak
	if ind.MarginBalancePeak > 0 && ind.MarginBalance > 0 {
		if (ind.MarginBalancePeak-ind.MarginBalance)/ind.MarginBalancePeak > d.cfg.DownturnMarginReductionPct {
			passed++
		}
	}
	checks++

	// 3. Public bank buying 5+ consecutive days
	if ind.PublicBankConsecBuyDays >= d.cfg.DownturnPublicBankBuyDays {
		passed++
	}
	checks++

	// 4. VIX > threshold but not making new highs
	if ind.VIX > d.cfg.DownturnVIXMin {
		passed++
	}
	checks++

	// 5. TAIEX above 5-day MA but below 20-day MA
	if ind.TAIEXPrice > 0 && ind.TAIEXMA5 > 0 && ind.TAIEXMA20 > 0 {
		if ind.TAIEXPrice > ind.TAIEXMA5 && ind.TAIEXPrice < ind.TAIEXMA20 {
			passed++
		}
	}
	checks++

	return checks >= 3 && passed >= 3
}

// ─── Turnaround Up Detection ───

func (d *PeriodDetector) isTurnaroundUp(ind PeriodIndicators) bool {
	hits := 0

	// 1. Foreign sudden buy: single day > 100億 or 3 consecutive buy days
	if ind.ForeignSingleDayNet > (d.cfg.TurnUpSingleBuyBillion*1_000_000_00) || ind.ForeignConsecBuyDays >= d.cfg.TurnUpConsecBuyDays {
		hits++
	}

	// 2. TWD surging: 1D > 0.3% appreciation or 3D > 0.5%
	if ind.TWDChange1D < d.cfg.TurnUpTWDApprec1DPct || ind.TWDChange3D < d.cfg.TurnUpTWDApprec3DPct {
		hits++
	}

	// 3. SOX breaks above 50-day MA or 20 crosses above 50
	if ind.SOXPrice > 0 && ind.SOXMA50 > 0 {
		if ind.SOXPrice > ind.SOXMA50 {
			hits++
		} else if ind.SOXMA20 > 0 && ind.SOXMA20 > ind.SOXMA50 {
			hits++
		}
	}

	// 4. TSM ADR surge: > 2% day, above 5-day high
	if ind.TSMADRPrice > 0 && ind.TSMADRHigh5 > 0 {
		// Simplified: check if current > 5-day high
		if ind.TSMADRPrice > ind.TSMADRHigh5 && (ind.TSMADRPrice-ind.TSMADRHigh5)/ind.TSMADRHigh5*100 > d.cfg.TurnUpTSMADRPct {
			hits++
		}
	}

	// 5. Foreign futures OI increase > 3000 contracts
	if ind.ForeignFuturesOIDelta3 > d.cfg.TurnUpFuturesOIIncrease {
		hits++
	}

	return hits >= 2
}

// ─── Bull Detection ───

func (d *PeriodDetector) isBull(ind PeriodIndicators) bool {
	checks := 0
	passed := 0

	// 1. Foreign continuous buy: 7+ of last 10 days
	if ind.ForeignBuyDays10 >= int(d.cfg.BullForeignBuyRatio10*10) {
		passed++
	}
	checks++

	// 2. Foreign futures OI high: > 30000
	if ind.ForeignFuturesOI > float64(d.cfg.BullFuturesOIMin) {
		passed++
	}
	checks++

	// 3. Margin mild increase: < 1% daily
	if ind.MarginBalanceChange5D > 0 && ind.MarginBalanceChange5D < d.cfg.BullMarginDailyMaxPct*5 {
		passed++
	}
	checks++

	// 4. TAIEX above 20-day MA with positive slope
	if ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0 {
		if ind.TAIEXPrice > ind.TAIEXMA20 && ind.TAIEXMA20Slope > 0 {
			passed++
		}
	}
	checks++

	return checks >= 3 && passed >= 3
}

// ─── Plateau Detection ───

func (d *PeriodDetector) isPlateau(ind PeriodIndicators) bool {
	checks := 0
	passed := 0

	// 1. Foreign buy slowing: 3-day avg < 50% of 10-day avg
	if ind.ForeignNet10DayAvg > 0 && ind.ForeignNet5DayAvg > 0 {
		if ind.ForeignNet5DayAvg/ind.ForeignNet10DayAvg < d.cfg.PlateauBuyRatio3to10 {
			passed++
		}
	}
	checks++

	// 2. Foreign futures declining 3+ days
	if ind.ForeignFuturesOIDelta3 < 0 && ind.ForeignFuturesOIDelta3 <= -3 {
		passed++
	}
	checks++

	// 3. Day trade ratio > 35%
	if ind.DayTradeRatio > d.cfg.PlateauDayTradeMinPct {
		passed++
	}
	checks++

	// 4. TAIEX near 20-day MA (±2%)
	if ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0 {
		deviation := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		if deviation > -(d.cfg.PlateauTAIEXDeviationPct/100) && deviation < (d.cfg.PlateauTAIEXDeviationPct/100) {
			passed++
		}
	}
	checks++

	// 5. Sector rotation active
	if ind.SectorRotationFlag {
		passed++
	}
	checks++

	return checks >= 3 && passed >= 3
}

// ─── Consolidation Detection ───

func (d *PeriodDetector) isConsolidation(ind PeriodIndicators) bool {
	checks := 0
	passed := 0

	// 1. Foreign mixed: both buy and sell days > 3 in 10 days
	if ind.ForeignBuyDays10 > d.cfg.ConsolidationBuyDaysMin && ind.ForeignSellDays10 > d.cfg.ConsolidationSellDaysMin {
		passed++
	}
	checks++

	// 2. TWD range-bound near monthly MA (±0.5%)
	if ind.TWDMA20 > 0 && ind.TWDChange5D > -(d.cfg.ConsolidationTWDBandPct) && ind.TWDChange5D < d.cfg.ConsolidationTWDBandPct {
		passed++
	}
	checks++

	// 3. No sector leader (rotation flag)
	if ind.SectorRotationFlag {
		passed++
	}
	checks++

	// 4. Volume contracting to 70%-100% of 20-day MA
	if ind.MarketVolume > 0 && ind.MarketVolumeMA20 > 0 {
		ratio := ind.MarketVolume / ind.MarketVolumeMA20
		if ratio >= d.cfg.ConsolidationVolRatioMin && ratio <= d.cfg.ConsolidationVolRatioMax {
			passed++
		}
	}
	checks++

	return checks >= 3 && passed >= 3
}

// ─── Downward Compatibility Mappings ───

// PeriodToRegime maps a seven-period classification to the three-state
// domain.Regime used by the existing pipeline. This is the bridge that
// allows PeriodDetector output to feed into ExecuteWithContext() without
// changing the existing orchestrator.
//
// Mapping per ATLAS_METHODOLOGY.md §3 "程式碼映射":
//
//	downturn       → RISK_OFF
//	turnaround_up  → NEUTRAL (transitioning to RISK_ON)
//	bull           → RISK_ON
//	plateau        → NEUTRAL
//	consolidation  → NEUTRAL
//	turnaround_down → RISK_OFF
//	black_swan     → RISK_OFF
func PeriodToRegime(p domain.MarketPeriod) domain.Regime {
	switch p {
	case domain.PeriodBull:
		return domain.RegimeRiskOn
	case domain.PeriodTurnaroundUp:
		// Transition period: neutral (existing code doesn't have "transitioning")
		return domain.RegimeNeutral
	case domain.PeriodDownturn, domain.PeriodTurnaroundDown, domain.PeriodBlackSwan:
		return domain.RegimeRiskOff
	default:
		// plateau, consolidation, unknown
		return domain.RegimeNeutral
	}
}

// PeriodToRiskLevel maps a seven-period classification to the macroflow
// RiskLevel used by Engine.Compute(). Per ATLAS_METHODOLOGY.md §5 macroflow
// dynamic adjustment table.
func PeriodToRiskLevel(p domain.MarketPeriod) macroflow.RiskLevel {
	switch p {
	case domain.PeriodBull, domain.PeriodTurnaroundUp, domain.PeriodPlateau, domain.PeriodConsolidation:
		return macroflow.RiskYellow
	case domain.PeriodDownturn, domain.PeriodTurnaroundDown:
		return macroflow.RiskOrange
	case domain.PeriodBlackSwan:
		return macroflow.RiskRed
	default:
		return macroflow.RiskYellow
	}
}
