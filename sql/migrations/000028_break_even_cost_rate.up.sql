-- 000028_break_even_cost_rate.up.sql
-- Phase 4 break-even transparency: forward-return median (the round-trip
-- cost rate at which win_rate falls below 50%) persisted alongside the
-- win-rate aggregates. Output column only — no gate semantics change.
ALTER TABLE stock_win_rate ADD COLUMN IF NOT EXISTS break_even_cost_rate DOUBLE PRECISION;
ALTER TABLE stock_win_rate_by_regime ADD COLUMN IF NOT EXISTS break_even_cost_rate DOUBLE PRECISION;
