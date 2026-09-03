-- 000021_period_detector_version.up.sql
-- capital-flow Phase 3 PR-3b: period_history gains a detector-semantics
-- stamp. PR-3b changes PeriodDetector behavior (P1 black-swan grading:
-- ≥2 conditions or a single extreme condition instead of plain OR;
-- P2 optional debounce state machine). Rows written after PR-3b carry
-- detector_version='v2'; pre-PR-3b rows keep 'v1' via the column default.
-- Phase 2's period×strategy matrix splits on this column.
ALTER TABLE period_history ADD COLUMN IF NOT EXISTS detector_version TEXT NOT NULL DEFAULT 'v1';
