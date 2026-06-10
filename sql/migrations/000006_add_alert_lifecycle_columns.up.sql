-- Add alert lifecycle columns for Phase 2A (dedup, status, silence, count tracking).
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'triggered';
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS dedup_key TEXT DEFAULT '';
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS resolved_by TEXT DEFAULT '';
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS silenced_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_alerts_dedup_key ON alerts(dedup_key);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
