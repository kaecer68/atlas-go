# atlas-mcp

`atlas-mcp` is the Model Context Protocol (MCP) server for [atlas-go](https://github.com/kaecer68/atlas-go). It exposes atlas-go's HTTP API as MCP tools so external AI agents (Claude Desktop, Cursor, OpenCode, etc.) can query and lightly trigger atlas-go through a standard JSON-RPC 2.0 interface.

## Phase 1 (this release)

Phase 1 ships **stdio transport only** with five core tools. Phase 2 will add
SSE and streamable-HTTP transports (see [`docs/plans/agent-interface-roadmap.md`](../../docs/plans/agent-interface-roadmap.md)).

| Tool | Trigger | Action |
|------|---------|--------|
| `regime_get_history` | Agent asks about past market regimes | GET `/api/dashboard/regime-history` |
| `strategy_list_active` | Agent asks which strategies are live | GET `/api/strategies/active` |
| `experiment_judge` | Agent wants to score a candidate experiment | POST `/api/experiment/judge` (side-effect) |
| `alert_list_unacknowledged` | Agent asks about open alerts | GET `/api/alerts/unacknowledged` |
| `system_get_health` | Agent asks about overall system health | GET `/api/dashboard/system-health` |

Reference catalog (70 tools total in the long run, with rationale + decision flow): [`docs/AGENT_TOOLS.md`](../../docs/AGENT_TOOLS.md).

## Configuration

All configuration via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `ATLAS_BASE_URL` | `http://127.0.0.1:8080` | atlas-go HTTP base URL |
| `ATLAS_API_KEY` | (unset) | Forwarded as `X-API-Key` header to atlas-go admin endpoints |
| `ATLAS_MCP_TOKEN` | (unset) | Required token presented by MCP clients. **Unset = dev mode (no auth check)** |
| `ATLAS_MCP_AUDIT_LOG` | `/tmp/atlas-mcp-audit.log` | JSONL audit log path. Parent dir auto-created with mode 0700 |

## Build & Run

```bash
go build -o bin/atlas-mcp ./cmd/atlas-mcp/
ATLAS_BASE_URL=http://127.0.0.1:8080 ATLAS_API_KEY=xxx ./bin/atlas-mcp
```

The server reads JSON-RPC requests from stdin and writes responses to stdout.

## Client Configuration Examples

### Claude Desktop (`~/.config/Claude Desktop/claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "atlas-go": {
      "command": "/absolute/path/to/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:8080",
        "ATLAS_API_KEY": "xxx",
        "ATLAS_MCP_TOKEN": "yyy",
        "ATLAS_MCP_AUDIT_LOG": "/var/log/atlas-mcp/audit.log"
      }
    }
  }
}
```

### Cursor (Settings → MCP)

Same shape. Add via `+ Add new MCP server`.

## Audit Log Format (JSONL)

Each line is one tool call:

```json
{"ts":"2026-06-30T08:00:13Z","tool":"regime_get_history","arg_keys":["days"],"status":"ok","duration_ms":42}
{"ts":"2026-06-30T08:00:14Z","tool":"experiment_judge","arg_keys":["experiment_id"],"status":"error","duration_ms":120,"error":"..."}
```

Required fields: `ts`, `tool`, `status` (`ok` | `error` | `unauthorized`), `duration_ms`. `arg_keys` is the list of input keys but values are never logged. `error` is included only when `status != "ok"`.

## License

Apache 2.0 — same as atlas-go.
