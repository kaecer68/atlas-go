package monitoring

import (
	"github.com/kaecer68/atlas-go/internal/domain"
)

type AgentUniverseView struct {
	AgentID           string                   `json:"agent_id"`
	Name              string                   `json:"name"`
	Layer             string                   `json:"layer"`
	Universe          []string                 `json:"universe"`
	ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
}

type SectorExposure struct {
	Sector      string  `json:"sector"`
	SectorLabel string  `json:"sector_label"`
	Weight      float64 `json:"weight"`
	EstValue    float64 `json:"est_value"`
}

type FactorExposureInline struct {
	Momentum float64 `json:"momentum"`
	Value    float64 `json:"value"`
	Quality  float64 `json:"quality"`
	Agent    float64 `json:"agent"`
	Total    float64 `json:"total"`
}
