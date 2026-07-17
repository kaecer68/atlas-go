package sectorallocation

import (
	"fmt"
	"regexp"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// StrategicSectorPrior 是唯一的 L1 sector strategic base distribution。
// spec §4.1：weights 必須恰好 20 個 canonical L1 sector IDs、sum=1±1e-9、
// 全 ≥ 0、Source ∈ {heuristic, calibrated}、CalibrationStatus ∈ {calibrating, calibrated, warming_up}。
//
// SA02 期間 SA-INV-05 鎖死：Source 起始為 "heuristic"、CalibrationStatus 起始為 "calibrating"；
// PromotionGate() 必須為 false 除非 source=calibrated 且 status=calibrated（升 empirical 不在 plan scope）。
type StrategicSectorPrior struct {
	Weights           map[industry.SectorID]float64
	Source            string
	ModelVersion      string
	CalibrationStatus string
	AsOfDate          string
}

// LoadStrategicPrior 從 ParametersConfig 讀 strategic prior 欄位，
// 對 SA-INV-05 做 validate。
// 若欄位缺值（pre-SA02 舊 config），仍可由 default 補齊（參數系統保證）。
func LoadStrategicPrior(cfg *config.ParametersConfig) (*StrategicSectorPrior, error) {
	if cfg == nil {
		return nil, fmt.Errorf("LoadStrategicPrior: nil config")
	}
	sp := cfg.Engine.SectorRotation.StrategicPrior
	weights := make(map[industry.SectorID]float64, 20)
	for k, v := range sp.Weights.Value {
		weights[industry.SectorID(k)] = v
	}
	prior := &StrategicSectorPrior{
		Weights:           weights,
		Source:            sp.Source.Value,
		ModelVersion:      sp.ModelVersion.Value,
		CalibrationStatus: sp.CalibrationStatus.Value,
		AsOfDate:          sp.AsOfDate.Value,
	}
	if err := ValidatePrior(prior); err != nil {
		return nil, fmt.Errorf("LoadStrategicPrior: %w", err)
	}
	return prior, nil
}

// PromotionGate 報告 prior 是否達到 GA 條件。
// SA02 期間：永遠 false，直到 source 與 status 雙雙升 empirical。
func (p *StrategicSectorPrior) PromotionGate() bool {
	return p.Source == "calibrated" && p.CalibrationStatus == "calibrated"
}

// semverRe 接受 semver 2.0 含 pre-release (alphanumeric + hyphen) 與 build metadata。
// 用字串前綴檢查避免 Go RE2 在 non-capturing group 內 `+?`/`*?` 的限制。
var semverRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z][0-9A-Za-z.-]*)?$`)

// ValidatePrior 對 SA-INV-05 做嚴格驗證：
// 20 keys、non L1 拒絕、negative 拒絕、sum drift > 1e-9 拒絕、non semver 拒絕。
func ValidatePrior(p *StrategicSectorPrior) error {
	if len(p.Weights) != 20 {
		return fmt.Errorf("prior must have 20 L1 keys, got %d", len(p.Weights))
	}
	s := 0.0
	for id, v := range p.Weights {
		if !industry.IsL1(id) {
			return fmt.Errorf("prior must not contain non L1 key %s", id)
		}
		if v < 0 {
			return fmt.Errorf("prior must not contain negative weight: %f for %s", v, id)
		}
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		return fmt.Errorf("prior sum drift: %.12f", s)
	}
	if !semverRe.MatchString(p.ModelVersion) {
		return fmt.Errorf("prior ModelVersion must be semver: %s", p.ModelVersion)
	}
	return nil
}
