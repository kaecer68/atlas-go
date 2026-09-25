package sectorallocation

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// L1FinalTarget 描述一次 sector 投資組合最終投組配置。
// 必須恰好 20 個 canonical L1 sector IDs、sum=1±1e-9、全 ≥ 0。
// spec §3.2 與 SA-INV-01:不得含 L2 / theme / asset / strategy key。
type L1FinalTarget struct {
	Weights           map[industry.SectorID]float64
	CalibrationStatus string
	ModelVersion      string
}

// ValidateL1FinalTarget 對 SA-INV-01 與 SA-INV-02 做單一拒絕檢查。
// 拒絕條件：非 20 keys、含非 L1 key、含負值、sum 漂移 > 1e-9。
func ValidateL1FinalTarget(t L1FinalTarget) error {
	if len(t.Weights) != 20 {
		return fmt.Errorf("L1 final target must have 20 keys, got %d", len(t.Weights))
	}
	s := 0.0
	for id, v := range t.Weights {
		if !industry.IsL1(id) {
			return fmt.Errorf("non canonical L1 key: %s", id)
		}
		if v < 0 {
			return fmt.Errorf("negative L1 weight: %f", v)
		}
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		return fmt.Errorf("L1 sum drift: %.12f", s)
	}
	return nil
}
