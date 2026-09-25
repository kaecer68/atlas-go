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

// SectorDataBridgeWired reports whether any production path calls
// BridgeSectorDataToCycleTracker. It does not, deliberately (issue #1944
// Batch 2, Q6 I14 — decided "明示未啟用", not wired):
//
//  1. The bridge's only input is data/sector_data/sector_data.json, which no
//     production job refreshes: the sector_data backfill channel is declared
//     Enabled:false (cmd/atlas/backfill_tasks.go), the committed file carries
//     updated_at 2026-05-12 and the channel contract treats a file older than
//     72h as degraded. Wiring it would push a months-old manual file into the
//     cycle tracker that the factor engine and cycle card consume.
//  2. UpdatePosition appends to the tracker history, which flips
//     CycleTracker.EvidenceTier() from "estimated" (config seed, 1 entry) to
//     "empirical" (2+ entries). Bridging a stale manual file would therefore
//     launder it into "empirical" evidence for external readers.
//
// Callers exist in tests only; if this constant is flipped, update
// CycleTracker.EvidenceTier semantics, the channel freshness gate and
// docs/specs/industry-allocation-inert-audit-20260924.md together.
const SectorDataBridgeWired = false

// BridgeSectorDataToCycleTracker reads the AI revenue growth and CoWoS
// utilization fields from a MacroDataSnapshot and calls
// CycleTracker.UpdatePosition for the "ai_supply_chain" and "semiconductor"
// industries. ProfitGrowthYoY and InventoryTurnover are not present in
// sector_data.json and remain at zero.
//
// NOT wired in production — see SectorDataBridgeWired.
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

	metrics.IndustryID = "semiconductor"
	tracker.UpdatePosition("semiconductor", metrics)
	logging.Info("sector_data_bridge", "updated_semiconductor",
		"rev_growth", aiRev, "cowos_util", cowos)
}
