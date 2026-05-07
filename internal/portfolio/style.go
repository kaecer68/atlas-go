package portfolio

import (
	"math"
	"sync"
	"time"
)

// StyleMomentumData 风格动量数据
type StyleMomentumData struct {
	Style     Style
	Return20D float64 // 20日收益
	Return60D float64 // 60日收益
	Strength  float64 // 相对强度
}

// StyleRotationDetector 风格轮动检测器
type StyleRotationDetector struct {
	styleReturns  map[Style][]float64 // 各风格历史收益
	threshold     float64             // 切换阈值
	holdingPeriod int                 // 最小持有期
	lastSwitch    map[Style]time.Time // 上次切换时间
	currentLeader Style               // 当前领先风格
	mu            sync.RWMutex
}

// NewStyleRotationDetector 创建检测器
func NewStyleRotationDetector() *StyleRotationDetector {
	return &StyleRotationDetector{
		styleReturns:  make(map[Style][]float64),
		threshold:     0.05, // 5% 差异阈值
		holdingPeriod: 5,    // 5天最小持有期
		lastSwitch:    make(map[Style]time.Time),
		currentLeader: StyleGrowth,
	}
}

// SetParameters 设置参数
func (d *StyleRotationDetector) SetParameters(threshold float64, holdingDays int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.threshold = threshold
	d.holdingPeriod = holdingDays
}

// UpdateStyleReturn 更新风格收益
func (d *StyleRotationDetector) UpdateStyleReturn(style Style, dailyReturn float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.styleReturns[style] = append(d.styleReturns[style], dailyReturn)

	// 只保留最近 60 天
	if len(d.styleReturns[style]) > 60 {
		d.styleReturns[style] = d.styleReturns[style][len(d.styleReturns[style])-60:]
	}
}

// GetStyleMomentum 获取风格动量
func (d *StyleRotationDetector) GetStyleMomentum(style Style) StyleMomentumData {
	d.mu.RLock()
	defer d.mu.RUnlock()

	returns := d.styleReturns[style]
	if len(returns) == 0 {
		return StyleMomentumData{Style: style}
	}

	// 计算 20 日收益
	var return20D float64
	start20 := len(returns) - 20
	if start20 < 0 {
		start20 = 0
	}
	for i := start20; i < len(returns); i++ {
		return20D += returns[i]
	}

	// 计算 60 日收益
	var return60D float64
	for _, r := range returns {
		return60D += r
	}

	return StyleMomentumData{
		Style:     style,
		Return20D: return20D,
		Return60D: return60D,
	}
}

// DetectRotation 检测风格轮动
func (d *StyleRotationDetector) DetectRotation() *RotationSignal {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 计算各风格动量
	momentums := make(map[Style]StyleMomentumData)
	for _, style := range []Style{StyleGrowth, StyleValue, StyleMomentum, StyleQuality} {
		returns := d.styleReturns[style]
		mom := StyleMomentumData{Style: style}
		if len(returns) > 0 {
			start20 := len(returns) - 20
			if start20 < 0 { start20 = 0 }
			for i := start20; i < len(returns); i++ { mom.Return20D += returns[i] }
			for _, r := range returns { mom.Return60D += r }
		}
		momentums[style] = mom
	}

	// 找出最强风格
	var strongest Style
	var maxStrength float64 = -math.MaxFloat64

	for style, mom := range momentums {
		// 综合 20 日和 60 日收益
		strength := mom.Return20D*0.7 + mom.Return60D*0.3
		momentums[style] = StyleMomentumData{
			Style:     style,
			Return20D: mom.Return20D,
			Return60D: mom.Return60D,
			Strength:  strength,
		}

		if strength > maxStrength {
			maxStrength = strength
			strongest = style
		}
	}

	// 检查是否需要切换
	if strongest == d.currentLeader {
		return nil // 无需切换
	}

	// 检查持有期
	if lastSwitch, ok := d.lastSwitch[strongest]; ok {
		if time.Since(lastSwitch).Hours() < float64(d.holdingPeriod*24) {
			return nil // 持有期未满
		}
	}

	// 计算当前领先风格的强度差距
	currentStrength := momentums[d.currentLeader].Strength
	diff := maxStrength - currentStrength

	if diff < d.threshold {
		return nil // 差距不够大
	}

	// 触发切换
	d.lastSwitch[strongest] = time.Now()
	d.currentLeader = strongest

	return &RotationSignal{
		FromStyle: d.currentLeader,
		ToStyle:   strongest,
		Strength:  maxStrength,
		Diff:      diff,
		Timestamp: time.Now(),
	}
}

// RotationSignal 轮动信号
type RotationSignal struct {
	FromStyle Style
	ToStyle   Style
	Strength  float64
	Diff      float64
	Timestamp time.Time
}

// IsValid 检查信号是否有效
func (r *RotationSignal) IsValid() bool {
	return r != nil && r.Diff > 0
}

// GetCurrentLeader 获取当前领先风格
func (d *StyleRotationDetector) GetCurrentLeader() Style {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentLeader
}

// GetAllMomentums 获取所有风格动量
func (d *StyleRotationDetector) GetAllMomentums() []StyleMomentumData {
	var result []StyleMomentumData
	for _, style := range []Style{StyleGrowth, StyleValue, StyleMomentum, StyleQuality} {
		result = append(result, d.GetStyleMomentum(style))
	}
	return result
}

// Reset 重置检测器
func (d *StyleRotationDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.styleReturns = make(map[Style][]float64)
	d.lastSwitch = make(map[Style]time.Time)
	d.currentLeader = StyleGrowth
}

// StyleRotationStrategy 风格轮动策略
type StyleRotationStrategy struct {
	detector      *StyleRotationDetector
	allocator     *RegimeAllocator
	rotationCount int
	lastRotation  time.Time
	mu            sync.RWMutex
}

// NewStyleRotationStrategy 创建策略
func NewStyleRotationStrategy() *StyleRotationStrategy {
	return &StyleRotationStrategy{
		detector:  NewStyleRotationDetector(),
		allocator: NewRegimeAllocator(),
	}
}

// UpdateAndRotate 更新数据并执行轮动
func (s *StyleRotationStrategy) UpdateAndRotate(
	styleReturns map[Style]float64,
) *RotationResult {
	// 更新各风格收益
	for style, ret := range styleReturns {
		s.detector.UpdateStyleReturn(style, ret)
	}

	// 检测轮动
	signal := s.detector.DetectRotation()
	if signal == nil {
		return nil
	}

	s.mu.Lock()
	s.rotationCount++
	s.lastRotation = time.Now()
	s.mu.Unlock()

	// 根据切换到的风格调整配置
	var allocation StyleAllocation
	switch signal.ToStyle {
	case StyleGrowth:
		allocation = StyleAllocation{
			Growth:   0.50,
			Value:    0.15,
			Momentum: 0.25,
			Quality:  0.10,
		}
	case StyleValue:
		allocation = StyleAllocation{
			Growth:   0.15,
			Value:    0.50,
			Momentum: 0.10,
			Quality:  0.25,
		}
	case StyleMomentum:
		allocation = StyleAllocation{
			Growth:   0.30,
			Value:    0.10,
			Momentum: 0.50,
			Quality:  0.10,
		}
	case StyleQuality:
		allocation = StyleAllocation{
			Growth:   0.15,
			Value:    0.25,
			Momentum: 0.10,
			Quality:  0.50,
		}
	}

	return &RotationResult{
		Signal:     *signal,
		Allocation: allocation,
		Count:      s.rotationCount,
	}
}

// RotationResult 轮动结果
type RotationResult struct {
	Signal     RotationSignal
	Allocation StyleAllocation
	Count      int
}

// GetStats 获取统计
func (s *StyleRotationStrategy) GetStats() RotationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return RotationStats{
		TotalRotations: s.rotationCount,
		LastRotation:   s.lastRotation,
		CurrentLeader:  s.detector.GetCurrentLeader(),
	}
}

// RotationStats 轮动统计
type RotationStats struct {
	TotalRotations int
	LastRotation   time.Time
	CurrentLeader  Style
}

// GetDetector 获取底层检测器
func (s *StyleRotationStrategy) GetDetector() *StyleRotationDetector {
	return s.detector
}

// GetAllocator 获取配置器
func (s *StyleRotationStrategy) GetAllocator() *RegimeAllocator {
	return s.allocator
}

// StyleExposures 风格敞口
type StyleExposures struct {
	Current   map[Style]float64
	Target    map[Style]float64
	Deviation map[Style]float64
}

// CalculateStyleExposures 计算当前风格敞口
func CalculateStyleExposures(
	positions []PositionWithStyle,
	totalValue float64,
) StyleExposures {
	exposures := make(map[Style]float64)

	for _, pos := range positions {
		for _, style := range pos.Styles {
			exposures[style] += pos.Value / totalValue / float64(len(pos.Styles))
		}
	}

	return StyleExposures{
		Current: exposures,
	}
}

// PositionWithStyle 带风格标签的持仓
type PositionWithStyle struct {
	Symbol string
	Value  float64
	Styles []Style
}
