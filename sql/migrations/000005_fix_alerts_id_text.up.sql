-- Fix alerts.id type from UUID to TEXT to match domain.AlertRecord.ID (string)
-- and monitoring.generateAlertID() which produces "alert-<nanos>" format.
-- This resolves "ERROR: invalid input syntax for type uuid" on INSERT.

ALTER TABLE alerts DROP CONSTRAINT alerts_pkey;
ALTER TABLE alerts ALTER COLUMN id DROP DEFAULT,
                        ALTER COLUMN id TYPE TEXT USING id::TEXT;
ALTER TABLE alerts ADD PRIMARY KEY (id);
