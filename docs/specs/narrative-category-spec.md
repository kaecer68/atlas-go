# Narrative Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_narrative.go`
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Narrative](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a

## 1. 目的

對外暴露敘事(narrative)引擎輸出:事件、因果鏈、模型列表、模板、季節性、批次 bundle,給 agent 與 web narrative page 取用。

## 2. Tool 清單(7 個)

| Tool | Description | Handler |
|------|-------------|---------|
| `narrative_get_events` | 最新敘事事件 | `handleNarrativeGetEvents` |
| `narrative_get_chains` | 因果鏈(latest detected event) | `handleNarrativeGetChains` |
| `narrative_get_models` | 敘事模型清單 | `handleNarrativeGetModels` |
| `narrative_get_templates` | 因果模板 | `handleNarrativeGetTemplates` |
| `narrative_get_seasonal` | 季節性敘事 packet | `handleNarrativeGetSeasonal` |
| `narrative_get_bundle` | 編譯好的 briefing bundle(macro+regime+narrative) | `handleNarrativeGetBundle` |
| `narrative_stress_index_thresholds` | Stress index 門檻值 | `handleNarrativeStressIndexThresholds` |

## 3. 規格重點

- **Engine**: `internal/narrative/`,詳見 [`docs/specs/agent-mcp-server-spec.md`](../specs/agent-mcp-server.md) §3
- **Briefing bundle** 與 `mcp_quickstart` 互補(後者放在 briefing.go,動機來源同)
- **Stress index thresholds**:見 `internal/narrative/stress_thresholds.go`

## 4. 已知限制

- `narrative_get_bundle` 回傳 9KB+,適合單次 fetch,不建議 high-frequency polling
- 因果鏈 topology 由 latest detected event 決定(單一時間切面)

## 5. 測試

- `tools_narrative_test.go`
- e2e: `curl /api/narrative/events?limit=10` 看 envelope shape

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版 |
