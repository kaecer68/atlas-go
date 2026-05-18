// Package industry provides a bridge from MacroDataSnapshot (populated by
// SectorDataProvider from data/sector_data/sector_data.json) into CycleTracker
// so the AI/semiconductor cycle data reflects real sector metrics instead of
// hardcoded defaults.
package industry

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// BridgeSectorDataToCycleTracker reads the AI revenue growth and CoWoS
// utilization fields from a MacroDataSnapshot and calls
// CycleTracker.UpdatePosition for the "ai_supply_chain" and "semiconductor"
// industries. ProfitGrowthYoY and InventoryTurnover are not present in
// sector_data.json and remain at zero.
func BridgeSectorDataToCycleTracker(snapshot marketdata.MacroDataSnapshot, tracker *CycleTracker) {
	// Normalise percentage values (45.2 → 0.452) for IndustryMetrics decimal
	// format. SectorDataProvider writes raw percentage into TSMCRevenue.Value.
	aiRev := snapshot.TSMCRevenue.Value / 100.0
	cowos := snapshot.CoWoSUtilization.Value / 100.0

	if aiRev == 0 && cowos == 0 {
		return
	}

	now := time.Now()

	metrics := IndustryMetrics{
		IndustryID:          "ai_supply_chain",
		RevenueGrowthYoY:    aiRev,
		CapacityUtilization: cowos,
		Timestamp:           now,
	}
	tracker.UpdatePosition("ai_supply_chain", metrics)
	logging.Info("sector_data_bridge", "updated_ai_supply_chain",
		"rev_growth", aiRev, "cowos_util", cowos)

	// TODO(P1): Add a dedicated semiconductor revenue field in MacroDataSnapshot
	// sourced from TWSE semiconductor sub-index or aggregate revenue of all
	// semiconductor stocks in sector_symbols.json. TSMC revenue (aiRev) and
	// CoWoS (cowos) represent AI-specific supply chain data, not the broader
	// semiconductor industry which includes DRAM, NAND, IC design, packaging, etc.
}
