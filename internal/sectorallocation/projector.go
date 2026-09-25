package sectorallocation

import (
	"fmt"
	"maps"
	"slices"

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
	// SA-DET-01（issue #1961）：driver 套用順序必須固定，且每個 driver 內部必須以
	// 固定 key 順序走訪。
	// 舊寫法把 8 個 driver map 放在一個 map 內走訪，Go 對 map 的迭代順序是隨機的；
	// 同一 sector 被多個 driver 加總時，IEEE-754 加法不具結合律
	// （(a+b)+c != a+(b+c)），結果尾位會逐次漂移，AdjustmentLog 的順序也會跟著漂移。
	driversInApplyOrder := []driverApplication{
		{"cycle", drivers.Cycle},
		{"seasonal", drivers.Seasonal},
		{"linkage", drivers.Linkage},
		{"narrative", drivers.Narrative},
		{"macro", drivers.Macro},
		{"capital_flow", drivers.CapitalFlow},
		{"theme", drivers.Theme},
		{"prior", drivers.StrategicPrior},
	}
	for _, d := range driversInApplyOrder {
		for _, id := range sortedSectorIDs(d.weights) {
			delta := d.weights[id]
			before := target[id]
			target[id] = before + delta
			log = append(log, AdjustmentEvent{
				Sector: id, Before: before, After: target[id], Reason: d.name,
			})
		}
	}

	// 3. clamp + sum 收斂（SA-INV-07）
	// SA-DET-01（issue #1961）：正規化加總必須以固定 key 順序進行。走訪 map 加總會讓
	// 20 個 sector 的總和尾位逐次不同（IEEE-754 加法不具結合律），除以總和後全部
	// sector 同時位移 1 ULP，正是 issue #1961 觀測到的現象。
	// target 的 key set 在此之後不再變動，故排序一次即可重用。
	ids := sortedSectorIDs(target)
	maxIter := p.constraints.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}
	for range maxIter {
		clamped := false
		for _, id := range ids {
			w := target[id]
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
		for _, id := range ids {
			s += target[id]
		}
		if s == 0 {
			return ProjectedTarget{}, fmt.Errorf("Projector: zero sum after clamp")
		}
		for _, id := range ids {
			target[id] = target[id] / s
		}
		if !clamped {
			break
		}
	}

	// 4. final sum check
	s := 0.0
	for _, id := range ids {
		s += target[id]
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

// driverApplication 把一個 driver 名稱與其 delta map 綁在一起，作為固定套用順序的一步。
// 存在理由見 Projector.Project 的 SA-DET-01 註解：map 走訪順序隨機，浮點加法不具結合律。
type driverApplication struct {
	name    string
	weights map[industry.SectorID]float64
}

// sortedSectorIDs 回傳 map 的 key（字典序），供任何對 map 做浮點累加的迴圈使用。
// issue #1961：對 map 直接做 `s += v` 會讓結果尾位隨迭代順序漂移；先排序再累加即為決定性。
func sortedSectorIDs(m map[industry.SectorID]float64) []industry.SectorID {
	ids := make([]industry.SectorID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
