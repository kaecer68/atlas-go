package sectorallocation

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// NamespaceKind 將 sector 識別字強制分成 4 個互斥的 namespace。
// spec §3.2: equity_sector_l1 / research_theme_l2 / strategy_bucket / asset_class
// 不得混用、不得 fuzzy map、不得跨層級 union。
type NamespaceKind string

const (
	NamespaceEquityL1        NamespaceKind = "equity_sector_l1"
	NamespaceResearchThemeL2 NamespaceKind = "research_theme_l2"
	NamespaceStrategyBucket  NamespaceKind = "strategy_bucket"
	NamespaceAssetClass      NamespaceKind = "asset_class"
)

// IsValidNamespace reports whether k is exactly one of the four canonical kinds.
// 拒絕拼字漂移、單複數不一致、跨層別名。
func IsValidNamespace(k NamespaceKind) bool {
	switch k {
	case NamespaceEquityL1, NamespaceResearchThemeL2, NamespaceStrategyBucket, NamespaceAssetClass:
		return true
	}
	return false
}

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

// ThemeExposure 描述 L2 / narrative theme 對 L1 的曝險矩陣。
// 每一列 ToL1 必須只含 L1 IDs 且 sum=1±1e-9。
// spec §3.2 與 SA-INV-03:禁止 fuzzy mapping 與 L2 偽裝成 L1。
type ThemeExposure struct {
	Theme              string
	CanonicalSubsector *industry.SectorID
	ToL1               map[industry.SectorID]float64
	Source             string
	Version            string
}

// ValidateThemeExposure 對 SA-INV-03 做單一拒絕檢查。
func ValidateThemeExposure(t ThemeExposure) error {
	if t.Theme == "" {
		return fmt.Errorf("theme must declare non-empty Theme")
	}
	if len(t.ToL1) == 0 {
		return fmt.Errorf("theme %q must declare at least one L1 target", t.Theme)
	}
	s := 0.0
	for id, w := range t.ToL1 {
		if !industry.IsL1(id) {
			return fmt.Errorf("theme %q maps to non L1 key %s", t.Theme, id)
		}
		if w < 0 {
			return fmt.Errorf("theme %q has negative weight: %f", t.Theme, w)
		}
		s += w
	}
	if s < 0.999999999 || s > 1.000000001 {
		return fmt.Errorf("theme %q row sum drift: %.12f", t.Theme, s)
	}
	return nil
}
