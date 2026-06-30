CREATE TABLE atlas_mcp_tokens (
  token_id UUID PRIMARY KEY,
  token_hash VARCHAR(64) NOT NULL UNIQUE,
  tenant_id VARCHAR(64) NOT NULL,
  agent_id VARCHAR(128) NOT NULL,
  scopes JSONB NOT NULL DEFAULT '[]',
  rate_limit_per_min INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ
);
CREATE INDEX idx_atlas_mcp_tokens_hash ON atlas_mcp_tokens(token_hash);
CREATE INDEX idx_atlas_mcp_tokens_tenant ON atlas_mcp_tokens(tenant_id);
