package sectorallocation

import "github.com/kaecer68/atlas-go/internal/industry"

// SectorAllocationSnapshot is the canonical view-model for a single
// simulation-closing sector allocation snapshot as defined in
// docs/specs/sector-allocation-simulation-closure-spec.md §4.4.
//
// SAFERepository: SnapshotReader / FileClosureStore in SA08.
type SectorAllocationSnapshot struct {
	AsOfTradingDate   string                        `json:"as_of_trading_date"`
	EffectiveFrom     string                        `json:"effective_from"`
	Target            map[industry.SectorID]float64 `json:"target"`
	Current           map[industry.SectorID]float64 `json:"current"`
	Delta             map[industry.SectorID]float64 `json:"delta"`
	ModelVersion      string                        `json:"model_version"`
	CalibrationStatus string                        `json:"calibration_status"`
	WeightSource      string                        `json:"weight_source"`
	FallbackReason    string                        `json:"fallback_reason,omitempty"`
	Applied           bool                          `json:"applied"`
	// MutationReceipt is filled by SA08 when the policy is consumed.
	MutationReceipt any `json:"mutation_receipt,omitempty"`
}

// SnapshotReader is the read-only interface that SA09 handlers consume.
// SA08 provides the production implementation.
type SnapshotReader interface {
	LatestSnapshot() *SectorAllocationSnapshot
}
