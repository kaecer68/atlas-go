-- 000028_break_even_cost_rate.down.sql
ALTER TABLE stock_win_rate DROP COLUMN IF EXISTS break_even_cost_rate;
ALTER TABLE stock_win_rate_by_regime DROP COLUMN IF EXISTS break_even_cost_rate;
