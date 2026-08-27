package portfolio

import (
	"log"

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

	// 同 symbol 多筆 recommendation 先合併（max conviction 勝出並保留該筆
	// target/goal；平手保留第一筆；衝突以 log 顯式記錄），
	// totalConviction 分母與分配均以去重後的 symbol 集計算。
	deduped := dedupeRecommendations(recommendations)
	if len(deduped) == 0 {
		return allocation
	}

	totalConviction := 0.0
	for _, rec := range deduped {
		if rec.Conviction > 0 {
			totalConviction += float64(rec.Conviction)
		}
	}

	if totalConviction == 0 {
		equalShare := deployableCapital / float64(len(deduped))
		for _, rec := range deduped {
			allocation.PositionSizes[rec.Symbol] = equalShare
		}
		return allocation
	}

	for _, rec := range deduped {
		if rec.Conviction > 0 {
			weight := float64(rec.Conviction) / totalConviction
			allocation.PositionSizes[rec.Symbol] = deployableCapital * weight
		}
	}

	return allocation
}

// dedupeRecommendations merges recommendations that share the same symbol.
//
// Semantics (P0-2 fix): for a duplicated symbol, the entry with the highest
// Conviction wins and the full winning Recommendation entry (TargetPrice,
// StopLossPrice, Reason, etc.) is kept; on a conviction tie the first
// occurrence is kept (deterministic). Every merge is logged explicitly so
// conflicts are never silent. Empty-symbol entries are suppressed with a
// warning (no dedup key available) and never enter the allocation.
func dedupeRecommendations(recs []domain.Recommendation) []domain.Recommendation {
	if len(recs) <= 1 {
		return recs
	}

	bestIdx := make(map[string]int, len(recs)) // symbol -> index in deduped
	deduped := make([]domain.Recommendation, 0, len(recs))
	merged := 0

	for _, rec := range recs {
		if rec.Symbol == "" {
			log.Printf("warn: capital allocator: skipping recommendation with empty symbol (conviction %d)", rec.Conviction)
			merged++
			continue
		}
		idx, exists := bestIdx[rec.Symbol]
		if !exists {
			bestIdx[rec.Symbol] = len(deduped)
			deduped = append(deduped, rec)
			continue
		}
		merged++
		if rec.Conviction > deduped[idx].Conviction {
			log.Printf(
				"warn: capital allocator: symbol %s duplicated: replacing kept entry (conviction %d) with higher-conviction entry (conviction %d)",
				rec.Symbol, deduped[idx].Conviction, rec.Conviction,
			)
			deduped[idx] = rec
		} else {
			log.Printf(
				"warn: capital allocator: symbol %s duplicated: keeping entry with conviction %d, merging duplicate with conviction %d",
				rec.Symbol, deduped[idx].Conviction, rec.Conviction,
			)
		}
	}

	if merged > 0 {
		log.Printf(
			"warn: capital allocator: suppressed %d duplicate recommendation(s) (incl. empty-symbol), %d unique symbol(s) after dedup",
			merged, len(deduped),
		)
	}
	return deduped
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
