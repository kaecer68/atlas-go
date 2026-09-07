-- 000022_channel_secrets.up.sql
-- Issue #1776 Phase 1: persistent storage for data-channel API keys.
-- Keys are stored AES-256-GCM encrypted (nonce || ciphertext, see
-- internal/channelsecrets/crypto.go); the master key lives in
-- ATLAS_SECRET_STORE_KEY and is NEVER stored in the database. Plaintext
-- API keys must never be written to this table.
CREATE TABLE IF NOT EXISTS channel_secrets (
    provider TEXT PRIMARY KEY,
    api_key_enc BYTEA NOT NULL,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
