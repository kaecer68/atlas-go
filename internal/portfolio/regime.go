package portfolio

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Style 投资风格
type Style string

const (
	StyleGrowth   Style = "growth"
	StyleValue    Style = "value"
	StyleMomentum Style = "momentum"
	StyleQuality  Style = "quality"
)

// StyleAllocation 风格配置
type StyleAllocation struct {
	Growth   float64
	Value    float64
	Momentum float64
	Quality  float64
}

// Validate 验证配置总和为 1
func (s StyleAllocation) Validate() bool {
	sum := s.Growth + s.Value + s.Momentum + s.Quality
	return sum >= 0.99 && sum <= 1.01
}

// RegimeConfig 市场状态配置
type RegimeConfig struct {
	Allocation  StyleAllocation
	MaxExposure float64 // 最大仓位比例
	CashReserve float64 // 现金储备比例
	RiskOn      bool    // 是否风险偏好
	Description string
}

// DefaultRegimeConfigs 默认市场状态配置
func DefaultRegimeConfigs() map[domain.Regime]RegimeConfig {
	return map[domain.Regime]RegimeConfig{
		domain.RegimeRiskOn: {
			Allocation: StyleAllocation{
				Growth:   0.40,
				Value:    0.20,
				Momentum: 0.30,
				Quality:  0.10,
			},
			MaxExposure: 0.95,
			CashReserve: 0.05,
			RiskOn:      true,
			Description: "风险偏好 - 偏好成长股和动量股",
		},
		domain.RegimeNeutral: {
			Allocation: StyleAllocation{
				Growth:   0.25,
				Value:    0.25,
				Momentum: 0.25,
				Quality:  0.25,
			},
			MaxExposure: 0.80,
			CashReserve: 0.20,
			RiskOn:      false,
			Description: "中性状态 - 均衡配置",
		},
		domain.RegimeRiskOff: {
			Allocation: StyleAllocation{
				Growth:   0.10,
				Value:    0.40,
				Momentum: 0.15,
				Quality:  0.35,
			},
			MaxExposure: 0.50,
			CashReserve: 0.50,
			RiskOn:      false,
			Description: "风险规避 - 偏好价值股和质量股",
		},
	}
}

// DefaultPeriodConfigs returns six-strategy allocation configs for each
// of the seven market periods per ATLAS_METHODOLOGY.md §5.
func DefaultPeriodConfigs() map[domain.MarketPeriod]RegimeConfig {
	return map[domain.MarketPeriod]RegimeConfig{
		domain.PeriodBull: {
			Allocation:  StyleAllocation{Growth: 0.40, Momentum: 0.35, Quality: 0.15, Value: 0.10},
			MaxExposure: 0.95, CashReserve: 0.05, RiskOn: true,
			Description: "多頭 — 跟隨聰明錢，偏好成長+動能",
		},
		domain.PeriodTurnaroundUp: {
			Allocation:  StyleAllocation{Growth: 0.40, Momentum: 0.30, Quality: 0.15, Value: 0.15},
			MaxExposure: 0.85, CashReserve: 0.15, RiskOn: true,
			Description: "轉折開高 — 聰明錢進場，成長為主、動能輔助",
		},
		domain.PeriodPlateau: {
			Allocation:  StyleAllocation{Growth: 0.20, Value: 0.30, Quality: 0.25, Momentum: 0.25},
			MaxExposure: 0.75, CashReserve: 0.25, RiskOn: false,
			Description: "高原 — 事件套利為主，降低攻擊部位",
		},
		domain.PeriodConsolidation: {
			Allocation:  StyleAllocation{Growth: 0.20, Value: 0.30, Quality: 0.30, Momentum: 0.20},
			MaxExposure: 0.65, CashReserve: 0.35, RiskOn: false,
			Description: "盤整 — 防禦+事件套利，保留現金",
		},
		domain.PeriodDownturn: {
			Allocation:  StyleAllocation{Growth: 0.10, Value: 0.45, Quality: 0.35, Momentum: 0.10},
			MaxExposure: 0.55, CashReserve: 0.45, RiskOn: false,
			Description: "低迷 — 跟隨公股布局，偏好價值+品質",
		},
		domain.PeriodTurnaroundDown: {
			Allocation:  StyleAllocation{Growth: 0.05, Value: 0.35, Quality: 0.50, Momentum: 0.10},
			MaxExposure: 0.40, CashReserve: 0.60, RiskOn: false,
			Description: "轉折下壓 — 全面防禦，現金為主",
		},
		domain.PeriodBlackSwan: {
			Allocation:  StyleAllocation{Growth: 0.00, Value: 0.30, Quality: 0.70, Momentum: 0.00},
			MaxExposure: 0.10, CashReserve: 0.90, RiskOn: false,
			Description: "黑天鵝 — 現金為王，僅保留極防禦部位",
		},
	}
}

// RegimeAllocator 市场状态配置器
type RegimeAllocator struct {
	currentRegime domain.Regime
	configs       map[domain.Regime]RegimeConfig
	periodConfigs map[domain.MarketPeriod]RegimeConfig
	mu            sync.RWMutex
}

func NewRegimeAllocator() *RegimeAllocator {
	return &RegimeAllocator{
		currentRegime: domain.RegimeNeutral,
		configs:       DefaultRegimeConfigs(),
		periodConfigs: DefaultPeriodConfigs(),
	}
}

// SetRegime 设置当前市场状态
func (r *RegimeAllocator) SetRegime(regime domain.Regime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentRegime = regime
}

// GetCurrentRegime 获取当前市场状态
func (r *RegimeAllocator) GetCurrentRegime() domain.Regime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentRegime
}

// GetAllocation 获取当前风格配置
func (r *RegimeAllocator) GetAllocation() StyleAllocation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, ok := r.configs[r.currentRegime]
	if !ok {
		return r.configs[domain.RegimeNeutral].Allocation
	}

	return config.Allocation
}

// GetRegimeConfig 获取指定市场状态的配置
func (r *RegimeAllocator) GetRegimeConfig(regime domain.Regime) RegimeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if config, ok := r.configs[regime]; ok {
		return config
	}

	return r.configs[domain.RegimeNeutral]
}

// GetPeriodConfig returns the allocation config for a given market period.
// Falls back to the Neutral config for unknown periods.
func (r *RegimeAllocator) GetPeriodConfig(period domain.MarketPeriod) RegimeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if config, ok := r.periodConfigs[period]; ok {
		return config
	}
	return r.configs[domain.RegimeNeutral]
}

// GetCurrentConfig 获取当前配置
func (r *RegimeAllocator) GetCurrentConfig() RegimeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.configs[r.currentRegime]
}

// GetMaxExposure 获取当前最大仓位
func (r *RegimeAllocator) GetMaxExposure() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, ok := r.configs[r.currentRegime]
	if !ok {
		return 0.80
	}

	return config.MaxExposure
}

// GetCashReserve 获取当前现金储备比例
func (r *RegimeAllocator) GetCashReserve() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, ok := r.configs[r.currentRegime]
	if !ok {
		return 0.20
	}

	return config.CashReserve
}

// IsRiskOn 是否风险偏好状态
func (r *RegimeAllocator) IsRiskOn() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, ok := r.configs[r.currentRegime]
	if !ok {
		return false
	}

	return config.RiskOn
}

// UpdateConfig 更新指定市场状态的配置
func (r *RegimeAllocator) UpdateConfig(regime domain.Regime, config RegimeConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[regime] = config
}

// GetStyleWeight 获取特定风格的权重
func (r *RegimeAllocator) GetStyleWeight(style Style) float64 {
	allocation := r.GetAllocation()

	switch style {
	case StyleGrowth:
		return allocation.Growth
	case StyleValue:
		return allocation.Value
	case StyleMomentum:
		return allocation.Momentum
	case StyleQuality:
		return allocation.Quality
	default:
		return 0
	}
}

// GetStyleDescription 获取当前配置的描述
func (r *RegimeAllocator) GetStyleDescription() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, ok := r.configs[r.currentRegime]
	if !ok {
		return "Unknown"
	}

	return config.Description
}

// RegimeDetector 市场状态检测器 (简化版)
type RegimeDetector struct {
	thresholds RegimeThresholds
	mu         sync.RWMutex
}

// RegimeThresholds 市场状态阈值
type RegimeThresholds struct {
	RiskOnRSI  float64 // RSI > 阈值 -> RISK_ON
	RiskOffRSI float64 // RSI < 阈值 -> RISK_OFF
	VIXHigh    float64 // VIX > 阈值 -> RISK_OFF
	VIXLow     float64 // VIX < 阈值 -> RISK_ON
}

// DefaultRegimeThresholds 默认阈值
func DefaultRegimeThresholds() RegimeThresholds {
	return RegimeThresholds{
		RiskOnRSI:  60,
		RiskOffRSI: 40,
		VIXHigh:    25,
		VIXLow:     15,
	}
}

// NewRegimeDetector 创建检测器
func NewRegimeDetector() *RegimeDetector {
	return &RegimeDetector{
		thresholds: DefaultRegimeThresholds(),
	}
}

// MarketIndicators 市场指标
type MarketIndicators struct {
	RSI      float64 // RSI 指标
	VIX      float64 // VIX 波动率指数
	SPXTrend float64 // 标普趋势 (20日收益)
	Volume   float64 // 成交量趋势
}

// Detect 检测市场状态
func (d *RegimeDetector) Detect(indicators MarketIndicators) domain.Regime {
	d.mu.RLock()
	thresholds := d.thresholds
	d.mu.RUnlock()

	// 基于 RSI 判断
	if indicators.RSI > thresholds.RiskOnRSI && indicators.VIX < thresholds.VIXLow {
		return domain.RegimeRiskOn
	}

	if indicators.RSI < thresholds.RiskOffRSI || indicators.VIX > thresholds.VIXHigh {
		return domain.RegimeRiskOff
	}

	return domain.RegimeNeutral
}

// SetThresholds 设置阈值
func (d *RegimeDetector) SetThresholds(thresholds RegimeThresholds) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.thresholds = thresholds
}

// IntegratedAllocator 整合配置器 (结合 Allocator 和 Detector)
type IntegratedAllocator struct {
	allocator *RegimeAllocator
	detector  *RegimeDetector
}

// NewIntegratedAllocator 创建整合配置器
func NewIntegratedAllocator() *IntegratedAllocator {
	return &IntegratedAllocator{
		allocator: NewRegimeAllocator(),
		detector:  NewRegimeDetector(),
	}
}

// UpdateAndDetect 更新指标并检测市场状态
func (i *IntegratedAllocator) UpdateAndDetect(indicators MarketIndicators) domain.Regime {
	newRegime := i.detector.Detect(indicators)
	i.allocator.SetRegime(newRegime)
	return newRegime
}

// GetAllocator 获取底层配置器
func (i *IntegratedAllocator) GetAllocator() *RegimeAllocator {
	return i.allocator
}

// GetDetector 获取底层检测器
func (i *IntegratedAllocator) GetDetector() *RegimeDetector {
	return i.detector
}

// GetCurrentAllocation 获取当前配置
func (i *IntegratedAllocator) GetCurrentAllocation() StyleAllocation {
	return i.allocator.GetAllocation()
}
