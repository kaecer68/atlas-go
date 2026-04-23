package portfolio

import (
	"github.com/kaecer68/atlas-go/internal/domain"
)

type CapitalAllocation struct {
	TotalCapital    float64            `json:"total_capital"`
	TotalDeployable float64            `json:"total_deployable"`
	ReserveCash     float64            `json:"reserve_cash"`
	PositionSizes   map[string]float64 `json:"position_sizes"`
}

type CapitalAllocator struct{}

func NewCapitalAllocator() *CapitalAllocator {
	return &CapitalAllocator{}
}

func (a *CapitalAllocator) Allocate(
	phaseConfig domain.CapitalPhaseConfig,
	recommendations []domain.Recommendation,
	totalCapital float64,
	reserveFraction float64,
) CapitalAllocation {
	limit := phaseConfig.CapitalLimits[string(phaseConfig.CurrentPhase)]
	if limit == 0 {
		limit = 1.0
	}

	reserveCash := totalCapital * reserveFraction
	deployableCapital := (totalCapital - reserveCash) * limit

	allocation := CapitalAllocation{
		TotalCapital:    totalCapital,
		TotalDeployable: deployableCapital,
		ReserveCash:     reserveCash,
		PositionSizes:   make(map[string]float64),
	}

	if len(recommendations) == 0 || deployableCapital <= 0 {
		return allocation
	}

	totalConviction := 0.0
	for _, rec := range recommendations {
		if rec.Conviction > 0 {
			totalConviction += float64(rec.Conviction)
		}
	}

	if totalConviction == 0 {
		equalShare := deployableCapital / float64(len(recommendations))
		for _, rec := range recommendations {
			allocation.PositionSizes[rec.Symbol] = equalShare
		}
		return allocation
	}

	for _, rec := range recommendations {
		if rec.Conviction > 0 {
			weight := float64(rec.Conviction) / totalConviction
			allocation.PositionSizes[rec.Symbol] = deployableCapital * weight
		}
	}

	return allocation
}

func (a *CapitalAllocator) ReallocateWithTax(
	phaseConfig domain.CapitalPhaseConfig,
	recommendations []domain.Recommendation,
	totalCapital float64,
	reserveFraction float64,
	taxSnapshots []domain.TaxSnapshot,
) CapitalAllocation {
	base := a.Allocate(phaseConfig, recommendations, totalCapital, reserveFraction)

	if len(taxSnapshots) == 0 {
		return base
	}

	taxBySymbol := make(map[string]float64)
	for _, ts := range taxSnapshots {
		taxBySymbol[ts.Symbol] = ts.TotalTax
	}

	for symbol, size := range base.PositionSizes {
		if tax, ok := taxBySymbol[symbol]; ok {
			base.PositionSizes[symbol] = size - tax
			if base.PositionSizes[symbol] < 0 {
				base.PositionSizes[symbol] = 0
			}
		}
	}

	var newDeployable float64
	for _, size := range base.PositionSizes {
		newDeployable += size
	}
	base.TotalDeployable = newDeployable

	return base
}
