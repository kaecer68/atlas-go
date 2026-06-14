package sectorallocation

import "time"

// SectorWeight represents the weight allocation for a single industry/sector.
type SectorWeight struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	BaseWeight       float64       `json:"base_weight"`
	AdjustedWeight   float64       `json:"adjusted_weight"`
	DerivationFactors []WeightFactor `json:"derivation_factors,omitempty"`
	AdjustmentLog    []string      `json:"adjustment_log,omitempty"`
}

// SectorAllocationPlan represents the complete sector allocation plan.
type SectorAllocationPlan struct {
	Allocations  []SectorWeight `json:"allocations"`
	PrimaryFlow string         `json:"primary_flow"`
	Rationale   string         `json:"rationale"`
	Timestamp   time.Time      `json:"timestamp"`
	ConfigSource string         `json:"config_source,omitempty"`
}

// WeightDerivation explains how an industry's weight is determined.
type WeightDerivation struct {
	BaseWeight        float64        `json:"base_weight"`
	DerivationFactors []WeightFactor `json:"derivation_factors"`
	Interpretation    string         `json:"interpretation"`
	RiskFactors       []string       `json:"risk_factors"`
	Opportunities     []string       `json:"opportunities"`
}

// WeightFactor represents a single factor contributing to industry weight.
type WeightFactor struct {
	Factor      string  `json:"factor"`
	Contribution float64 `json:"contribution"`
	Source      string  `json:"source"`
	Evidence    string  `json:"evidence"`
}
