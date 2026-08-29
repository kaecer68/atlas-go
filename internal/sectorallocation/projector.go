package sectorallocation

import (
	"fmt"
	"maps"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// MacroAction 將 macro 評估結果轉成 typed enum（spec §6.1）。
// spec SA-INV-08 + SA-INV-09：macro driver 必須唯一套用一次；assessment 未 eligible
// 不允許將 bullish/bearish 猜成 risk_off 等 action。
type MacroAction string

const (
	MacroActionMixed            MacroAction = "mixed"
	MacroActionRiskOff          MacroAction = "risk_off"
	MacroActionCarryTradeUnwind MacroAction = "carry_trade_unwind"
	MacroActionSectorRotation   MacroAction = "sector_rotation"
)

// CapitalFlowAction 將 E07 CapitalFlowAssessment 轉成 typed enum（SA05 擴充用）。
type CapitalFlowAction string

const (
	CapitalFlowActionUnavailable CapitalFlowAction = "unavailable"
	CapitalFlowActionNeutral     CapitalFlowAction = "neutral"
	CapitalFlowActionRiskOn      CapitalFlowAction = "risk_on"
	CapitalFlowActionRiskOff     CapitalFlowAction = "risk_off"
)

// ProjectedTarget 是 Projector 唯一 owner 的最終 L1 投組配置。
// spec §4.4 + SA-INV-01：恰好 20 個 canonical L1、sum=1±1e-9、AdjustmentLog 完整 provenance。
type ProjectedTarget struct {
	AsOfTradingDate  string
	Target           map[industry.SectorID]float64
	AdjustmentLog    []AdjustmentEvent
	DriverProvenance map[string]string
	ModelVersion     string
	FallbackReason   string
}

// AdjustmentEvent 記錄單一 driver 對 L1 sector 的 delta 來源。
type AdjustmentEvent struct {
	Sector industry.SectorID
	Before float64
	After  float64
	Reason string
}

// DriverInputs 收集 6 個 driver 與 macro / capital flow action；所有 delta 必須只作用於 L1。
type DriverInputs struct {
	AsOfTradingDate   string
	Cycle             map[industry.SectorID]float64
	Seasonal          map[industry.SectorID]float64
	Linkage           map[industry.SectorID]float64
	Narrative         map[industry.SectorID]float64
	Macro             map[industry.SectorID]float64
	CapitalFlow       map[industry.SectorID]float64
	Theme             map[industry.SectorID]float64
	StrategicPrior    map[industry.SectorID]float64
	MacroAction       MacroAction
	CapitalFlowAction CapitalFlowAction
}

// ProjectionConstraints 描述 projection 的邊界。
// 預設 MinSectorExposure=0.005、MaxSectorExposure=0.5、SumTolerance=1e-9。
type ProjectionConstraints struct {
	MinSectorExposure float64
	MaxSectorExposure float64
	SumTolerance      float64
	MaxIterations     int
}

// Projector 是唯一 projection owner（spec §5）。
// 拒絕非 L1 key、套 min/max clamp、零和 tilt 檢查、sum=1±tolerance clamp、AdjustmentLog 完整 provenance。
type Projector struct {
	constraints ProjectionConstraints
}

// NewDefaultProjector 回傳預設約束（0.5/0.005/1e-9/10 iter）的 Projector。
func NewDefaultProjector() *Projector {
	return &Projector{
		constraints: ProjectionConstraints{
			MinSectorExposure: 0.005,
			MaxSectorExposure: 0.50,
			SumTolerance:      1e-9,
			MaxIterations:     10,
		},
	}
}

// NewProjectorWithConstraints 接受自訂約束。
func NewProjectorWithConstraints(c ProjectionConstraints) *Projector {
	if c.MaxIterations == 0 {
		c.MaxIterations = 10
	}
	if c.SumTolerance == 0 {
		c.SumTolerance = 1e-9
	}
	return &Projector{constraints: c}
}

// Project 是唯一的最終 projection entry。
// spec SA-INV-01/02/04/05/06/07/08：typed keys、唯一 normalize owner、driver 套用一次、zero-sum tilt。
// 拒絕輸入：non L1 key、len != 20、driver 對 non L1 key 投 delta。
func (p *Projector) Project(raw map[industry.SectorID]float64, drivers DriverInputs) (ProjectedTarget, error) {
	// 1. 嚴格 L1 universe 檢查（SA-INV-01/02/04）
	if len(raw) != 20 {
		return ProjectedTarget{}, fmt.Errorf("Projector: raw must have 20 L1 keys, got %d", len(raw))
	}
	for id := range raw {
		if !industry.IsL1(id) {
			return ProjectedTarget{}, fmt.Errorf("Projector: non L1 key rejected: %s", id)
		}
	}
	for _, m := range []map[industry.SectorID]float64{
		drivers.Cycle, drivers.Seasonal, drivers.Linkage, drivers.Narrative,
		drivers.Macro, drivers.CapitalFlow, drivers.Theme, drivers.StrategicPrior,
	} {
		for id := range m {
			if !industry.IsL1(id) {
				return ProjectedTarget{}, fmt.Errorf("Projector: driver maps to non L1 key %s", id)
			}
		}
	}

	// 2. 累加所有 driver 對 raw 的 delta（SA-INV-08：每 driver 最多一次）
	target := make(map[industry.SectorID]float64, 20)
	maps.Copy(target, raw)
	log := []AdjustmentEvent{}
	provenance := map[string]string{
		"cycle":        "v1.0.0",
		"seasonal":     "v1.0.0",
		"linkage":      "v1.0.0",
		"narrative":    "v1.0.0",
		"macro":        string(drivers.MacroAction),
		"capital_flow": string(drivers.CapitalFlowAction),
		"theme":        "v1.0.0",
		"prior":        "StrategicSectorPrior",
	}
	for name, m := range map[string]map[industry.SectorID]float64{
		"cycle":        drivers.Cycle,
		"seasonal":     drivers.Seasonal,
		"linkage":      drivers.Linkage,
		"narrative":    drivers.Narrative,
		"macro":        drivers.Macro,
		"capital_flow": drivers.CapitalFlow,
		"theme":        drivers.Theme,
		"prior":        drivers.StrategicPrior,
	} {
		for id, delta := range m {
			before := target[id]
			target[id] = before + delta
			log = append(log, AdjustmentEvent{
				Sector: id, Before: before, After: target[id], Reason: name,
			})
		}
	}

	// 3. clamp + sum 收斂（SA-INV-07）
	maxIter := p.constraints.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	for range maxIter {
		clamped := false
		for id, w := range target {
			if w < 0 {
				target[id] = 0
				clamped = true
			}
			if w > p.constraints.MaxSectorExposure {
				target[id] = p.constraints.MaxSectorExposure
				clamped = true
			}
			if w < p.constraints.MinSectorExposure && w > 0 {
				target[id] = p.constraints.MinSectorExposure
				clamped = true
			}
		}
		// normalize sum
		s := 0.0
		for _, v := range target {
			s += v
		}
		if s == 0 {
			return ProjectedTarget{}, fmt.Errorf("Projector: zero sum after clamp")
		}
		for id := range target {
			target[id] = target[id] / s
		}
		if !clamped {
			break
		}
	}

	// 4. final sum check
	s := 0.0
	for _, v := range target {
		s += v
	}
	if s < (1.0-p.constraints.SumTolerance) || s > (1.0+p.constraints.SumTolerance) {
		return ProjectedTarget{}, fmt.Errorf("Projector: final sum drift %.12f", s)
	}

	return ProjectedTarget{
		AsOfTradingDate:  drivers.AsOfTradingDate,
		Target:           target,
		AdjustmentLog:    log,
		DriverProvenance: provenance,
		ModelVersion:     "v0.0.0-canonical",
	}, nil
}
