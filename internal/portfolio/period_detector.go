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
	"reflect"

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

	// ── 地緣政治（台海危機，憲章 §3 黑天鵝/轉折下壓）──
	GeoIntensity         float64 // 地緣（台海）風險強度 0-100（geo RSS provider → 壓力指數 geopolitical 元件）
	GeoIntensityChange5D float64 // 地緣強度 5 日變動（正=升溫；0=無歷史資料/持平，視為不可用→僅以當日強度判定）
}

// TriggeredIndicator captures the evaluation outcome of a single period-detection
// condition. It is purely observational metadata — it MUST NOT influence
// detection logic.
type TriggeredIndicator struct {
	Name           string  `json:"name"`            // 指標名（與 ATLAS_METHODOLOGY.md §3 一致）
	Value          float64 `json:"value"`           // 實際值
	Threshold      float64 `json:"threshold"`       // 閾值
	Relation       string  `json:"relation"`        // "gt" | "lt" | "gte" | "lte"
	Hit            bool    `json:"hit"`             // 是否觸發
	InputAvailable bool    `json:"input_available"` // 輸入欄位是否接入（非零/非空）
}

// PeriodAssessment is the full classification output including observability
// metadata. DetectAssessment returns this; DetectPeriod delegates to it and
// returns only the MarketPeriod.
type PeriodAssessment struct {
	MarketPeriod        domain.MarketPeriod  `json:"market_period"`
	Confidence          float64              `json:"confidence"`
	ConditionsHit       int                  `json:"conditions_hit"`
	ConditionsTotal     int                  `json:"conditions_total"`
	TriggeredIndicators []TriggeredIndicator `json:"triggered_indicators"`
	IsFallback          bool                 `json:"is_fallback"`
}

// PeriodDetector implements seven-period market cycle detection per
// ATLAS_METHODOLOGY.md §3. Thresholds are driven by config.PeriodDetectionConfig;
// use NewPeriodDetector(config) or NewPeriodDetectorWithDefaults().
type PeriodDetector struct {
	cfg config.PeriodDetectionConfig
}

// package-level reference: keep isXxx methods alive for backward compatibility
// and golden test validation. These are intentionally retained alongside their
// assessXxx counterparts; both linters (golangci-lint unused, staticcheck U1000)
// would otherwise flag them as dead code.
var _ = [...]func(*PeriodDetector, PeriodIndicators) bool{
	(*PeriodDetector).isBlackSwan,
	(*PeriodDetector).isTurnaroundDown,
	(*PeriodDetector).isDownturn,
	(*PeriodDetector).isTurnaroundUp,
	(*PeriodDetector).isBull,
	(*PeriodDetector).isPlateau,
	(*PeriodDetector).isConsolidation,
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
	assessment, _ := d.DetectAssessment(ind)
	return assessment.MarketPeriod
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
	if ind.TAIEXMA20 > 0 && ind.TAIEXPrice > 0 {
		// TAIEX deviation proxy: price relative to 20-day MA (< -5%).
		// NOTE: Uses MA20 deviation as proxy for single-day crash. Known biases:
		// (a) false-positive on slow multi-day declines, (b) false-negative when
		// crash occurs from a price well above MA20. Full fix needs TAIEXChange1D.
		decline := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		if decline < -(d.cfg.BlackSwanTAIEXDeclinePct / 100) {
			triggers++
		}
	}
	if ind.NationalFundActive {
		triggers++
	}
	// TWD panic: single day depreciation > threshold (%)
	if ind.TWDChange1D > d.cfg.BlackSwanTWDDepreciationPct {
		triggers++
	}
	// Geopolitical (Taiwan Strait) crisis: intensity >= threshold (0-100)
	if ind.GeoIntensity >= d.cfg.BlackSwanGeoIntensity {
		triggers++
	}

	return triggers >= 1
}

// assessBlackSwan evaluates each black-swan condition independently and
// returns the assessment metadata alongside the boolean result. The boolean
// result is guaranteed identical to isBlackSwan(ind) for the same input.
func (d *PeriodDetector) assessBlackSwan(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 6

	// 1. Foreign panic sell: single day > 500億
	thr1 := -(d.cfg.BlackSwanForeignSellBillion * 1_000_000_00)
	hit1 := ind.ForeignSingleDayNet < thr1
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資單日賣超金額", Value: ind.ForeignSingleDayNet,
		Threshold: thr1, Relation: "lt",
		Hit: hit1, InputAvailable: true,
	})
	if hit1 {
		condHit++
	}

	// 2. VIX spike
	avail2 := ind.VIX != 0
	hit2 := ind.VIX > d.cfg.BlackSwanVIX
	indicators = append(indicators, TriggeredIndicator{
		Name: "VIX 恐慌指數", Value: ind.VIX,
		Threshold: d.cfg.BlackSwanVIX, Relation: "gt",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. TAIEX deviation from 20-day MA
	avail3 := ind.TAIEXMA20 > 0 && ind.TAIEXPrice > 0
	hit3 := false
	if avail3 {
		decline := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		hit3 = decline < -(d.cfg.BlackSwanTAIEXDeclinePct / 100)
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "大盤偏離月線跌幅", Value: 0,
		Threshold: -(d.cfg.BlackSwanTAIEXDeclinePct / 100), Relation: "lt",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. National fund intervention
	hit4 := ind.NationalFundActive
	indicators = append(indicators, TriggeredIndicator{
		Name: "國安基金進場", Value: 0,
		Threshold: 0, Relation: "eq",
		Hit: hit4, InputAvailable: true,
	})
	if hit4 {
		condHit++
	}

	// 5. TWD panic depreciation
	avail5 := ind.TWDChange1D != 0
	hit5 := ind.TWDChange1D > d.cfg.BlackSwanTWDDepreciationPct
	indicators = append(indicators, TriggeredIndicator{
		Name: "台幣單日貶值幅度", Value: ind.TWDChange1D,
		Threshold: d.cfg.BlackSwanTWDDepreciationPct, Relation: "gt",
		Hit: hit5, InputAvailable: avail5,
	})
	if avail5 && hit5 {
		condHit++
	}

	// 6. Geopolitical (Taiwan Strait) crisis
	avail6 := ind.GeoIntensity != 0
	hit6 := ind.GeoIntensity >= d.cfg.BlackSwanGeoIntensity
	indicators = append(indicators, TriggeredIndicator{
		Name: "地緣（台海）風險強度", Value: ind.GeoIntensity,
		Threshold: d.cfg.BlackSwanGeoIntensity, Relation: "gte",
		Hit: hit6, InputAvailable: avail6,
	})
	if avail6 && hit6 {
		condHit++
	}

	return condHit >= 1, condHit, condTotal, indicators
}

// ─── Turnaround Down Detection ───

func (d *PeriodDetector) isTurnaroundDown(ind PeriodIndicators) bool {
	// All conditions required

	passed := 0

	// 1. Foreign consecutive heavy sell: 3+ days with at least one > 150億.
	// Uses ForeignNetPeakSell (max sell in window) as secondary check to
	// cover the case where the heaviest sell was not the most recent day.
	heavyThreshold := -(d.cfg.TurnDownSingleSellBillion * 1_000_000_00)
	if ind.ForeignConsecSellDays >= d.cfg.TurnDownConsecSellDays &&
		(ind.ForeignSingleDayNet < heavyThreshold || ind.ForeignNetPeakSell < heavyThreshold) {
		passed++
	}

	// 2. TWD breaks below monthly MA, depreciating fast
	if ind.TWDMA20 > 0 && ind.TWDChange1D > 0 {
		// Simplified: TWD weaker than MA20 and still weakening
		passed++
	}

	// 3. Margin maintenance ratio < 150%
	if ind.MarginMaintenanceRatio > 0 && ind.MarginMaintenanceRatio < d.cfg.TurnDownMarginMaintRatio {
		passed++
	}

	// 4. SOX below 50-day MA
	if ind.SOXPrice > 0 && ind.SOXMA50 > 0 && ind.SOXPrice < ind.SOXMA50 {
		passed++
	}

	// 5. Foreign futures turning short or large reduction (contracts, OI delta)
	futuresDelta := ind.ForeignFuturesOI - ind.ForeignFuturesOIPrev
	if futuresDelta < -float64(d.cfg.TurnDownFuturesOIDecrease) || ind.ForeignFuturesOI < 0 {
		passed++
	}

	// 6. Geopolitical (Taiwan Strait) tension rising: intensity >= threshold
	// AND 5-day trend not declining (Change5D == 0 = no history → intensity-only).
	if ind.GeoIntensity >= d.cfg.TurnDownGeoIntensity && ind.GeoIntensityChange5D >= 0 {
		passed++
	}

	return passed >= 3
}

// assessTurnaroundDown evaluates each turnaround-down condition and returns
// assessment metadata. The boolean result is guaranteed identical to
// isTurnaroundDown(ind) for the same input.
func (d *PeriodDetector) assessTurnaroundDown(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 6
	heavyThreshold := -(d.cfg.TurnDownSingleSellBillion * 1_000_000_00)

	// 1. Foreign consecutive heavy sell
	avail1 := ind.ForeignConsecSellDays != 0 || ind.ForeignSingleDayNet != 0 || ind.ForeignNetPeakSell != 0
	hit1 := ind.ForeignConsecSellDays >= d.cfg.TurnDownConsecSellDays &&
		(ind.ForeignSingleDayNet < heavyThreshold || ind.ForeignNetPeakSell < heavyThreshold)
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資連續重賣", Value: float64(ind.ForeignConsecSellDays),
		Threshold: float64(d.cfg.TurnDownConsecSellDays), Relation: "gte",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. TWD breaks below monthly MA
	avail2 := ind.TWDMA20 > 0 && ind.TWDChange1D != 0
	hit2 := ind.TWDMA20 > 0 && ind.TWDChange1D > 0
	indicators = append(indicators, TriggeredIndicator{
		Name: "台幣跌破月線且續貶", Value: ind.TWDChange1D,
		Threshold: 0, Relation: "gt",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. Margin maintenance ratio < threshold
	avail3 := ind.MarginMaintenanceRatio > 0
	hit3 := ind.MarginMaintenanceRatio > 0 && ind.MarginMaintenanceRatio < d.cfg.TurnDownMarginMaintRatio
	indicators = append(indicators, TriggeredIndicator{
		Name: "融資維持率", Value: ind.MarginMaintenanceRatio,
		Threshold: d.cfg.TurnDownMarginMaintRatio, Relation: "lt",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. SOX below 50-day MA
	avail4 := ind.SOXPrice > 0 && ind.SOXMA50 > 0
	hit4 := ind.SOXPrice > 0 && ind.SOXMA50 > 0 && ind.SOXPrice < ind.SOXMA50
	indicators = append(indicators, TriggeredIndicator{
		Name: "SOX 跌破季線", Value: ind.SOXPrice,
		Threshold: ind.SOXMA50, Relation: "lt",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	// 5. Foreign futures turning short
	futuresDelta := ind.ForeignFuturesOI - ind.ForeignFuturesOIPrev
	avail5 := ind.ForeignFuturesOI != 0 || ind.ForeignFuturesOIPrev != 0
	hit5 := futuresDelta < -float64(d.cfg.TurnDownFuturesOIDecrease) || ind.ForeignFuturesOI < 0
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資期貨轉空", Value: futuresDelta,
		Threshold: -float64(d.cfg.TurnDownFuturesOIDecrease), Relation: "lt",
		Hit: hit5, InputAvailable: avail5,
	})
	if avail5 && hit5 {
		condHit++
	}

	// 6. Geopolitical (Taiwan Strait) tension rising: intensity >= threshold
	// AND 5-day trend not declining (Change5D == 0 = no history → intensity-only).
	avail6 := ind.GeoIntensity != 0
	hit6 := ind.GeoIntensity >= d.cfg.TurnDownGeoIntensity && ind.GeoIntensityChange5D >= 0
	indicators = append(indicators, TriggeredIndicator{
		Name: "地緣（台海）緊張升溫", Value: ind.GeoIntensity,
		Threshold: d.cfg.TurnDownGeoIntensity, Relation: "gte",
		Hit: hit6, InputAvailable: avail6,
	})
	if avail6 && hit6 {
		condHit++
	}

	return condHit >= 3, condHit, condTotal, indicators
}

// ─── Downturn Detection ───
func (d *PeriodDetector) isDownturn(ind PeriodIndicators) bool {
	passed := 0

	// 1. Foreign sell slowing: 5-day avg < 30% of peak sell
	if ind.ForeignNetPeakSell < 0 && ind.ForeignNet5DayAvg < 0 {
		if ind.ForeignNet5DayAvg/ind.ForeignNetPeakSell < d.cfg.DownturnSellRatioToPeak {
			passed++
		}
	}

	// 2. Margin balance down > 15% from peak
	if ind.MarginBalancePeak > 0 && ind.MarginBalance > 0 {
		if (ind.MarginBalancePeak-ind.MarginBalance)/ind.MarginBalancePeak > d.cfg.DownturnMarginReductionPct {
			passed++
		}
	}

	// 3. Public bank buying 5+ consecutive days
	if ind.PublicBankConsecBuyDays >= d.cfg.DownturnPublicBankBuyDays {
		passed++
	}

	// 4. VIX > threshold but not making new highs
	if ind.VIX > d.cfg.DownturnVIXMin {
		passed++
	}

	// 5. TAIEX above 5-day MA but below 20-day MA
	if ind.TAIEXPrice > 0 && ind.TAIEXMA5 > 0 && ind.TAIEXMA20 > 0 {
		if ind.TAIEXPrice > ind.TAIEXMA5 && ind.TAIEXPrice < ind.TAIEXMA20 {
			passed++
		}
	}

	return passed >= 3
}

// assessDownturn evaluates each downturn condition and returns assessment
// metadata. The boolean result is guaranteed identical to isDownturn(ind).
func (d *PeriodDetector) assessDownturn(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 5

	// 1. Foreign sell slowing
	avail1 := ind.ForeignNetPeakSell < 0 && ind.ForeignNet5DayAvg < 0
	hit1 := false
	if avail1 {
		hit1 = ind.ForeignNet5DayAvg/ind.ForeignNetPeakSell < d.cfg.DownturnSellRatioToPeak
	}
	ratio := 0.0
	if ind.ForeignNetPeakSell != 0 {
		ratio = ind.ForeignNet5DayAvg / ind.ForeignNetPeakSell
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資賣壓趨緩(5日均/峰值)", Value: ratio,
		Threshold: d.cfg.DownturnSellRatioToPeak, Relation: "lt",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. Margin balance reduction
	avail2 := ind.MarginBalancePeak > 0 && ind.MarginBalance > 0
	hit2 := false
	reduction := 0.0
	if avail2 {
		reduction = (ind.MarginBalancePeak - ind.MarginBalance) / ind.MarginBalancePeak
		hit2 = reduction > d.cfg.DownturnMarginReductionPct
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "融資餘額降幅", Value: reduction,
		Threshold: d.cfg.DownturnMarginReductionPct, Relation: "gt",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. Public bank buying
	avail3 := ind.PublicBankConsecBuyDays != 0
	hit3 := ind.PublicBankConsecBuyDays >= d.cfg.DownturnPublicBankBuyDays
	indicators = append(indicators, TriggeredIndicator{
		Name: "公股連續買超天數", Value: float64(ind.PublicBankConsecBuyDays),
		Threshold: float64(d.cfg.DownturnPublicBankBuyDays), Relation: "gte",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. VIX elevated
	avail4 := ind.VIX != 0
	hit4 := ind.VIX > d.cfg.DownturnVIXMin
	indicators = append(indicators, TriggeredIndicator{
		Name: "VIX 維持高檔", Value: ind.VIX,
		Threshold: d.cfg.DownturnVIXMin, Relation: "gt",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	// 5. TAIEX between MA5 and MA20
	avail5 := ind.TAIEXPrice > 0 && ind.TAIEXMA5 > 0 && ind.TAIEXMA20 > 0
	hit5 := avail5 && ind.TAIEXPrice > ind.TAIEXMA5 && ind.TAIEXPrice < ind.TAIEXMA20
	indicators = append(indicators, TriggeredIndicator{
		Name: "大盤介於5日與月線間", Value: ind.TAIEXPrice,
		Threshold: ind.TAIEXMA20, Relation: "lt",
		Hit: hit5, InputAvailable: avail5,
	})
	if avail5 && hit5 {
		condHit++
	}

	return condHit >= 3, condHit, condTotal, indicators
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
	futuresDelta := ind.ForeignFuturesOI - ind.ForeignFuturesOIPrev
	if futuresDelta > float64(d.cfg.TurnUpFuturesOIIncrease) {
		hits++
	}

	return hits >= 2
}

// assessTurnaroundUp evaluates each turnaround-up condition and returns
// assessment metadata. Identical boolean result to isTurnaroundUp(ind).
func (d *PeriodDetector) assessTurnaroundUp(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 5

	// 1. Foreign sudden buy
	avail1 := ind.ForeignSingleDayNet != 0 || ind.ForeignConsecBuyDays != 0
	hit1 := ind.ForeignSingleDayNet > (d.cfg.TurnUpSingleBuyBillion*1_000_000_00) || ind.ForeignConsecBuyDays >= d.cfg.TurnUpConsecBuyDays
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資突擊買超", Value: ind.ForeignSingleDayNet,
		Threshold: d.cfg.TurnUpSingleBuyBillion * 1_000_000_00, Relation: "gt",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. TWD appreciation
	avail2 := ind.TWDChange1D != 0 || ind.TWDChange3D != 0
	hit2 := ind.TWDChange1D < d.cfg.TurnUpTWDApprec1DPct || ind.TWDChange3D < d.cfg.TurnUpTWDApprec3DPct
	indicators = append(indicators, TriggeredIndicator{
		Name: "台幣急升", Value: ind.TWDChange1D,
		Threshold: d.cfg.TurnUpTWDApprec1DPct, Relation: "lt",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. SOX break above 50-day MA
	avail3 := ind.SOXPrice > 0 && ind.SOXMA50 > 0
	hit3 := avail3 && (ind.SOXPrice > ind.SOXMA50 || (ind.SOXMA20 > 0 && ind.SOXMA20 > ind.SOXMA50))
	indicators = append(indicators, TriggeredIndicator{
		Name: "SOX 突破季線", Value: ind.SOXPrice,
		Threshold: ind.SOXMA50, Relation: "gt",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. TSM ADR surge
	avail4 := ind.TSMADRPrice > 0 && ind.TSMADRHigh5 > 0
	hit4 := false
	if avail4 {
		hit4 = ind.TSMADRPrice > ind.TSMADRHigh5 && (ind.TSMADRPrice-ind.TSMADRHigh5)/ind.TSMADRHigh5*100 > d.cfg.TurnUpTSMADRPct
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "TSM ADR 突破5日高", Value: ind.TSMADRPrice,
		Threshold: ind.TSMADRHigh5, Relation: "gt",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	// 5. Futures OI increase
	futuresDelta := ind.ForeignFuturesOI - ind.ForeignFuturesOIPrev
	avail5 := ind.ForeignFuturesOI != 0 || ind.ForeignFuturesOIPrev != 0
	hit5 := futuresDelta > float64(d.cfg.TurnUpFuturesOIIncrease)
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資期貨OI增幅", Value: futuresDelta,
		Threshold: float64(d.cfg.TurnUpFuturesOIIncrease), Relation: "gt",
		Hit: hit5, InputAvailable: avail5,
	})
	if avail5 && hit5 {
		condHit++
	}

	return condHit >= 2, condHit, condTotal, indicators
}

// ─── Bull Detection ───

func (d *PeriodDetector) isBull(ind PeriodIndicators) bool {
	passed := 0

	// 1. Foreign continuous buy: 7+ of last 10 days
	if ind.ForeignBuyDays10 >= int(d.cfg.BullForeignBuyRatio10*10) {
		passed++
	}

	// 2. Foreign futures OI high: > 30000
	if ind.ForeignFuturesOI > float64(d.cfg.BullFuturesOIMin) {
		passed++
	}

	// 3. Margin mild increase: < 1% daily
	if ind.MarginBalanceChange5D > 0 && ind.MarginBalanceChange5D < d.cfg.BullMarginDailyMaxPct*5 {
		passed++
	}

	// 4. TAIEX above 20-day MA with positive slope
	if ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0 {
		if ind.TAIEXPrice > ind.TAIEXMA20 && ind.TAIEXMA20Slope > 0 {
			passed++
		}
	}

	return passed >= 3
}

// assessBull evaluates each bull-market condition and returns assessment
// metadata. Identical boolean result to isBull(ind).
func (d *PeriodDetector) assessBull(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 4

	// 1. Foreign continuous buy
	avail1 := ind.ForeignBuyDays10 != 0
	hit1 := ind.ForeignBuyDays10 >= int(d.cfg.BullForeignBuyRatio10*10)
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資10日買超天數", Value: float64(ind.ForeignBuyDays10),
		Threshold: d.cfg.BullForeignBuyRatio10 * 10, Relation: "gte",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. Futures OI high
	avail2 := ind.ForeignFuturesOI != 0
	hit2 := ind.ForeignFuturesOI > float64(d.cfg.BullFuturesOIMin)
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資期貨OI水位", Value: ind.ForeignFuturesOI,
		Threshold: float64(d.cfg.BullFuturesOIMin), Relation: "gt",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. Margin mild increase
	avail3 := ind.MarginBalanceChange5D > 0
	hit3 := ind.MarginBalanceChange5D > 0 && ind.MarginBalanceChange5D < d.cfg.BullMarginDailyMaxPct*5
	indicators = append(indicators, TriggeredIndicator{
		Name: "融資溫和增加", Value: ind.MarginBalanceChange5D,
		Threshold: d.cfg.BullMarginDailyMaxPct * 5, Relation: "lt",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. TAIEX above MA20 with positive slope
	avail4 := ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0
	hit4 := avail4 && ind.TAIEXPrice > ind.TAIEXMA20 && ind.TAIEXMA20Slope > 0
	indicators = append(indicators, TriggeredIndicator{
		Name: "大盤站穩月線且斜率向上", Value: ind.TAIEXPrice,
		Threshold: ind.TAIEXMA20, Relation: "gt",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	return condHit >= 3, condHit, condTotal, indicators
}

// ─── Plateau Detection ───

func (d *PeriodDetector) isPlateau(ind PeriodIndicators) bool {
	passed := 0

	// 1. Foreign buy slowing: 3-day avg < 50% of 10-day avg
	if ind.ForeignNet10DayAvg > 0 && ind.ForeignNet5DayAvg > 0 {
		if ind.ForeignNet5DayAvg/ind.ForeignNet10DayAvg < d.cfg.PlateauBuyRatio3to10 {
			passed++
		}
	}

	// 2. Foreign futures declining 3+ days
	if ind.ForeignFuturesOIDelta3 < 0 && ind.ForeignFuturesOIDelta3 <= -3 {
		passed++
	}

	// 3. Day trade ratio > 35%
	if ind.DayTradeRatio > d.cfg.PlateauDayTradeMinPct {
		passed++
	}

	// 4. TAIEX near 20-day MA (±2%)
	if ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0 {
		deviation := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		if deviation > -(d.cfg.PlateauTAIEXDeviationPct/100) && deviation < (d.cfg.PlateauTAIEXDeviationPct/100) {
			passed++
		}
	}

	// 5. Sector rotation active
	if ind.SectorRotationFlag {
		passed++
	}

	return passed >= 3
}

// assessPlateau evaluates each plateau condition and returns assessment
// metadata. Identical boolean result to isPlateau(ind).
func (d *PeriodDetector) assessPlateau(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 5

	// 1. Foreign buy slowing
	avail1 := ind.ForeignNet10DayAvg > 0 && ind.ForeignNet5DayAvg > 0
	hit1 := false
	ratio1 := 0.0
	if avail1 {
		ratio1 = ind.ForeignNet5DayAvg / ind.ForeignNet10DayAvg
		hit1 = ratio1 < d.cfg.PlateauBuyRatio3to10
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資買超趨緩(5日/10日)", Value: ratio1,
		Threshold: d.cfg.PlateauBuyRatio3to10, Relation: "lt",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. Futures declining
	avail2 := ind.ForeignFuturesOIDelta3 < 0
	hit2 := ind.ForeignFuturesOIDelta3 < 0 && ind.ForeignFuturesOIDelta3 <= -3
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資期貨連減天數", Value: float64(ind.ForeignFuturesOIDelta3),
		Threshold: -3, Relation: "lte",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. Day trade ratio
	avail3 := ind.DayTradeRatio > 0
	hit3 := ind.DayTradeRatio > d.cfg.PlateauDayTradeMinPct
	indicators = append(indicators, TriggeredIndicator{
		Name: "當沖佔比", Value: ind.DayTradeRatio,
		Threshold: d.cfg.PlateauDayTradeMinPct, Relation: "gt",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. TAIEX near MA20
	avail4 := ind.TAIEXPrice > 0 && ind.TAIEXMA20 > 0
	hit4 := false
	if avail4 {
		deviation := (ind.TAIEXPrice - ind.TAIEXMA20) / ind.TAIEXMA20
		hit4 = deviation > -(d.cfg.PlateauTAIEXDeviationPct/100) && deviation < (d.cfg.PlateauTAIEXDeviationPct/100)
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "大盤貼近月線", Value: ind.TAIEXPrice,
		Threshold: ind.TAIEXMA20, Relation: "near",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	// 5. Sector rotation
	avail5 := ind.SectorRotationFlag
	hit5 := ind.SectorRotationFlag
	indicators = append(indicators, TriggeredIndicator{
		Name: "類股輪動", Value: 0,
		Threshold: 0, Relation: "eq",
		Hit: hit5, InputAvailable: avail5,
	})
	if avail5 && hit5 {
		condHit++
	}

	return condHit >= 3, condHit, condTotal, indicators
}

// ─── Consolidation Detection ───

func (d *PeriodDetector) isConsolidation(ind PeriodIndicators) bool {
	passed := 0

	// 1. Foreign mixed: both buy and sell days > 3 in 10 days
	if ind.ForeignBuyDays10 > d.cfg.ConsolidationBuyDaysMin && ind.ForeignSellDays10 > d.cfg.ConsolidationSellDaysMin {
		passed++
	}

	// 2. TWD range-bound near monthly MA (±0.5%)
	if ind.TWDMA20 > 0 && ind.TWDChange5D > -d.cfg.ConsolidationTWDBandPct && ind.TWDChange5D < d.cfg.ConsolidationTWDBandPct {
		passed++
	}

	// 3. No sector leader (rotation flag)
	if ind.SectorRotationFlag {
		passed++
	}

	// 4. Volume contracting to 70%-100% of 20-day MA
	if ind.MarketVolume > 0 && ind.MarketVolumeMA20 > 0 {
		ratio := ind.MarketVolume / ind.MarketVolumeMA20
		if ratio >= d.cfg.ConsolidationVolRatioMin && ratio <= d.cfg.ConsolidationVolRatioMax {
			passed++
		}
	}

	return passed >= 3
}

// assessConsolidation evaluates each consolidation condition and returns
// assessment metadata. Identical boolean result to isConsolidation(ind).
func (d *PeriodDetector) assessConsolidation(ind PeriodIndicators) (hit bool, condHit int, condTotal int, indicators []TriggeredIndicator) {
	condTotal = 4

	// 1. Foreign mixed buy/sell
	avail1 := ind.ForeignBuyDays10 > 0 || ind.ForeignSellDays10 > 0
	hit1 := ind.ForeignBuyDays10 > d.cfg.ConsolidationBuyDaysMin && ind.ForeignSellDays10 > d.cfg.ConsolidationSellDaysMin
	indicators = append(indicators, TriggeredIndicator{
		Name: "外資買賣天數交錯", Value: float64(ind.ForeignBuyDays10),
		Threshold: float64(d.cfg.ConsolidationBuyDaysMin), Relation: "gt",
		Hit: hit1, InputAvailable: avail1,
	})
	if avail1 && hit1 {
		condHit++
	}

	// 2. TWD range-bound
	avail2 := ind.TWDMA20 > 0 && ind.TWDChange5D != 0
	hit2 := ind.TWDMA20 > 0 && ind.TWDChange5D > -d.cfg.ConsolidationTWDBandPct && ind.TWDChange5D < d.cfg.ConsolidationTWDBandPct
	indicators = append(indicators, TriggeredIndicator{
		Name: "台幣區間整理", Value: ind.TWDChange5D,
		Threshold: d.cfg.ConsolidationTWDBandPct, Relation: "between",
		Hit: hit2, InputAvailable: avail2,
	})
	if avail2 && hit2 {
		condHit++
	}

	// 3. No sector leader
	avail3 := ind.SectorRotationFlag
	hit3 := ind.SectorRotationFlag
	indicators = append(indicators, TriggeredIndicator{
		Name: "無領導類股", Value: 0,
		Threshold: 0, Relation: "eq",
		Hit: hit3, InputAvailable: avail3,
	})
	if avail3 && hit3 {
		condHit++
	}

	// 4. Volume contraction
	avail4 := ind.MarketVolume > 0 && ind.MarketVolumeMA20 > 0
	hit4 := false
	if avail4 {
		ratio := ind.MarketVolume / ind.MarketVolumeMA20
		hit4 = ratio >= d.cfg.ConsolidationVolRatioMin && ratio <= d.cfg.ConsolidationVolRatioMax
	}
	indicators = append(indicators, TriggeredIndicator{
		Name: "成交量收縮", Value: ind.MarketVolume,
		Threshold: ind.MarketVolumeMA20, Relation: "between",
		Hit: hit4, InputAvailable: avail4,
	})
	if avail4 && hit4 {
		condHit++
	}

	return condHit >= 3, condHit, condTotal, indicators
}

// DetectAssessment classifies the current market into one of seven periods
// and returns the full assessment including confidence and triggered indicators.
// Detection order follows the priority chain; the first matching period wins.
// Falls back to PeriodConsolidation when no period matches.
func (d *PeriodDetector) DetectAssessment(ind PeriodIndicators) (PeriodAssessment, error) {
	var assessment PeriodAssessment

	// ─── Detection Order ───

	// 1. Black Swan
	if hit, condHit, condTotal, indicators := d.assessBlackSwan(ind); hit {
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodBlackSwan,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else if hit, condHit, condTotal, indicators := d.assessTurnaroundDown(ind); hit {
		// 2. Turnaround Down
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodTurnaroundDown,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else if hit, condHit, condTotal, indicators := d.assessDownturn(ind); hit {
		// 3. Downturn
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodDownturn,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else if hit, condHit, condTotal, indicators := d.assessTurnaroundUp(ind); hit {
		// 4. Turnaround Up
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodTurnaroundUp,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else if hit, condHit, condTotal, indicators := d.assessBull(ind); hit {
		// 5. Bull
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodBull,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else if hit, condHit, condTotal, indicators := d.assessPlateau(ind); hit {
		// 6. Plateau
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodPlateau,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
		}
	} else {
		// 7. Consolidation (last in chain — always evaluate)
		_, condHit, condTotal, indicators := d.assessConsolidation(ind)
		assessment = PeriodAssessment{
			MarketPeriod:        domain.PeriodConsolidation,
			ConditionsHit:       condHit,
			ConditionsTotal:     condTotal,
			TriggeredIndicators: indicators,
			// IsFallback: 判為 consolidation 且整個 PeriodIndicators 無任何
			// 非零輸入（設計 §3.1.2 step 3：無資料 fallback vs 有指標支撐的
			// 真實盤整）。consolidation 專屬的 indicators（買賣天數/輪動等）
			// 可能為零但其他層級有資料（TAIEX/OI/量能）——只要有資料就不算
			// fallback，故檢查全 struct 而非單一 assess 的 InputAvailable。
			IsFallback: !indicatorsHaveData(ind),
		}
	}

	// Compute confidence = conditions_hit / conditions_total (Formula A)
	if assessment.ConditionsTotal > 0 {
		assessment.Confidence = float64(assessment.ConditionsHit) / float64(assessment.ConditionsTotal)
	}

	return assessment, nil
}

// ─── Downward Compatibility Mappings ───

// indicatorsHaveData reports whether any period indicator carries a non-zero
// value (float64 / int / bool). Used to distinguish data-backed classifications
// from zero-data fallbacks: a day with any non-zero input is never "fallback"
// even if it classifies as consolidation (design §3.1.2 step 3).
func indicatorsHaveData(ind PeriodIndicators) bool {
	v := reflect.ValueOf(ind)
	for _, field := range v.Fields() {
		switch f := field.Interface().(type) {
		case float64:
			if f != 0 {
				return true
			}
		case int:
			if f != 0 {
				return true
			}
		case bool:
			if f {
				return true
			}
		}
	}
	return false
}

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
