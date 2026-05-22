# Issues & Gotchas

## [2026-05-20] Session Init
- Hardcoded debt catalog:
  - factor_weight_engine.go: base weights (8 values), regime deltas (9), RISK_ON/OFF adjustments (5), severity deltas (4), event theme→factor mappings (3 themes), strategy adjustments, clamp bounds [0.02, 0.50]
  - narrative_conviction_modulator.go: theme hit rates (5, e.g. AI_capex_surge=0.81), skill→theme mappings (9 pairs)
  - industry_cycle_modulator.go: skillToIndustry map, phaseDelta (expansion=+20, recovery=+10, mature=0, recession=-20)
  - sector_rotator.go: applyMacroAdjustments() (Yellow/Orange/Red), applyFlowAdjustments() (risk_off/carry_trade_unwind/sector_rotation)
- Base weights: momentum=0.25, value=0.20, quality=0.20, agent=0.15, inst_sent=0.10, liquidity=0.05, narrative=0.05, industry_cycle=0.00
- Clamp bounds: [0.02, 0.50]
