//go:build golden

package portfolio

import (
	"fmt"
	"testing"
)

// TestAvailabilitySemantic shows three states (available-hit, available-miss,
// unavailable) for the two Batch 3 fields. This test is for diagnostic output
// only; the underlying calculations are exercised in other tests.
func TestAvailabilitySemantic(t *testing.T) {
	sectorDir := t.TempDir()
	govDir := t.TempDir()

	// Sector: write 12 days of identical industries to ensure no rotation
	// (force rotation=false → available+miss for the false case).
	for _, d := range []string{
		"2026-07-14", "2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18",
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
		"2026-07-28", "2026-07-29",
	} {
		writeSectorIndexFile(t, sectorDir, d, map[string]float64{
			"semiconductor": 3.0, "electronics": 2.0, "financials": 1.0,
			"steel": 0.5, "energy": 0.0, "cement": -0.5, "construction": -1.0, "shipping": -2.0,
		})
	}
	// Gov: 5 days of valid (both legacy + _brokers.json) all positive.
	for _, d := range []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"} {
		writeGovFlowDate(t, govDir, d, "100")
	}

	c := NewCalculator()

	// Day 1: sufficient data → available (both fields filled)
	ind1 := PeriodIndicators{}
	c.EnrichBatch3(&ind1, "2026-07-25", sectorDir, govDir)
	fmt.Printf("\n[2026-07-25 5d+ available, both legacy+_brokers]\n")
	fmt.Printf("  SectorRotationFlag=%t (available+miss since no rotation)\n", ind1.SectorRotationFlag)
	fmt.Printf("  PublicBankConsecBuyDays=%d (available+hit since 5d all buy)\n", ind1.PublicBankConsecBuyDays)

	// Day 2: gov has only 3 valid dates (< 5) → PB unavailable
	ind2 := PeriodIndicators{}
	c.EnrichSectorRotation(&ind2, "2026-07-23", sectorDir) // sector still has 8 days
	c.EnrichGovernmentBroker(&ind2, "2026-07-23", govDir)  // 3 valid dates
	fmt.Printf("\n[2026-07-23 3d gov, 8d sector — PB unavailable]\n")
	fmt.Printf("  SectorRotationFlag=%t (available+miss)\n", ind2.SectorRotationFlag)
	fmt.Printf("  PublicBankConsecBuyDays=%d (unavailable — leaves 0)\n", ind2.PublicBankConsecBuyDays)

	// Day 3: empty sector dir → sector unavailable
	ind3 := PeriodIndicators{}
	c.EnrichSectorRotation(&ind3, "2026-07-29", "/tmp/empty-sector")
	c.EnrichGovernmentBroker(&ind3, "2026-07-25", govDir)
	fmt.Printf("\n[empty sector dir]\n")
	fmt.Printf("  SectorRotationFlag=%t (unavailable — leaves false)\n", ind3.SectorRotationFlag)
	fmt.Printf("  PublicBankConsecBuyDays=%d (available+hit since 5d)\n", ind3.PublicBankConsecBuyDays)
}
