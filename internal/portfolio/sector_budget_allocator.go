package portfolio

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// SectorBudgetAllocator consumes a sector allocation policy and
// produces per-sector budget allocations as a fraction of total
// portfolio value. It implements the "next session must consume
// policy before producing orders" constraint from SA08.
//
// Budgets are proportional to the target weights in the policy
// snapshot. If the snapshot's sum does not equal 1 within tolerance,
// the allocator returns an error (fail-closed).
type SectorBudgetAllocator struct {
	// budgetTolerance is the allowed deviation from sum=1.0.
	budgetTolerance float64
}

// NewSectorBudgetAllocator creates an allocator with default tolerance.
func NewSectorBudgetAllocator() *SectorBudgetAllocator {
	return &SectorBudgetAllocator{budgetTolerance: 1e-9}
}

// AllocationResult maps L1 sector IDs to their budget fraction of
// total portfolio value.
type AllocationResult struct {
	Budgets         map[industry.SectorID]float64 `json:"budgets"`
	PortfolioValue  float64                       `json:"portfolio_value"`
	PolicyReceiptID string                        `json:"policy_receipt_id"`
	EffectiveFrom   string                        `json:"effective_from"`
}

// Allocate consumes a policy snapshot and produces budget allocations.
// It validates the policy:
//   - snapshot must not be nil
//   - target weights must sum to 1.0 within tolerance
//   - policy must have at least one target sector
//
// Returns error when the policy is unusable (degraded state).
func (a *SectorBudgetAllocator) Allocate(
	snap *sectorallocation.SectorAllocationSnapshot,
	portfolioValue float64,
) (*AllocationResult, error) {
	if snap == nil {
		return nil, fmt.Errorf("nil snapshot — no policy available")
	}
	if len(snap.Target) == 0 {
		return nil, fmt.Errorf("empty target — policy degraded")
	}

	sum := 0.0
	for _, w := range snap.Target {
		sum += w
	}
	if sum < 1.0-a.budgetTolerance || sum > 1.0+a.budgetTolerance {
		return nil, fmt.Errorf("target sum %.6f outside tolerance [1.0±%.0e]", sum, a.budgetTolerance)
	}

	budgets := make(map[industry.SectorID]float64, len(snap.Target))
	for sector, weight := range snap.Target {
		budgets[sector] = portfolioValue * weight
	}

	receiptID := ""
	if snap.MutationReceipt != nil {
		receiptID = snap.MutationReceipt.ReceiptID
	}

	return &AllocationResult{
		Budgets:         budgets,
		PortfolioValue:  portfolioValue,
		PolicyReceiptID: receiptID,
		EffectiveFrom:   snap.EffectiveFrom,
	}, nil
}
