# AGENTS.md — internal/llm

LLM 整合基礎設施層：capability-based 多 Provider 路由（選型依任務可達成率 + 訂閱額度）。`DataClass` 自 ADR-012 起為稽核／觀測 metadata，**不再閘門任何 provider**。

> 設計權威：`docs/llm-integration-strategy-framework.md`（v2.2）  
> 決策紀錄：`docs/llm-adr-log.md` ADR-012（取代 ADR-010 的 DataClass 主權閘門）  
> 路由表：runtime 來源是 `internal/llm/router.go` 的 `defaultRoutingTable()`；`configs/llm_router.yaml` 為鏡像，一致性由 `internal/llm/config_test.go` 把關（目前無 cmd 呼叫 `TryLoadRouterConfig`）  
> 完整 Provider/Capability SOP：`.claude/skills/atlas-llm-provider-capability/SKILL.md`

## 公共 API 速查

- `NewDefaultRouter(impls ...ProviderImpl)`：啟動時 wiring。
- `NewDefaultRouterFromConfig(cfg, impls)`：載入 `configs/llm_router.yaml` 後呼叫。
- `DefaultRouter.Call(ctx, Request)`：**所有 LLM 呼叫的唯一入口**。
- `DefaultRouter.Health()`：`/api/llm/health` 資料來源。

12 個 `Capability` 常數列於 `provider.go:30-41`。新增 capability 必須同步 4 個位置（常數、schema、handler、router table），詳見 `.claude/skills/atlas-llm-provider-capability/SKILL.md`。

## 核心陷阱

1. **繞過 Router 直接呼叫 Provider**：會跳過 routing table 與 fallback chain。一律透過 `DefaultRouter.Call()` 或 `capabilities/*Handler`。
2. **S/E 模組直接 import**：會破壞 replay 可重現性。hot-path 呼叫必須 async 或 fallback-safe。
3. **把「成功但 output 為空」當成功**：Router 自 ADR-012 起將 trim 後為空的 `Output`（且無 `ToolCalls`）視為該 provider 失敗並續試下一鏈成員。provider 端若因 `max_tokens` 不足被 thinking 吃光預算，會回 HTTP 200 + 空內容 —— 不要只看 `err == nil`。各 capability 的 `max_tokens` 最低 2048。
4. **以為 DataClass 還會擋 provider**：`ErrProviderDisabled` 已不再由 `DefaultRouter.Call` 回傳；`DataClass` 只記 metric/span 與 `Response.AttemptedProviders` 可供稽核。
5. **新增 Capability**：必須同步 4 個位置（常數、schema、handler、router table），見 skill。

DeepSeek 模型名由 `LLM_DEEPSEEK_MODEL` 決定（預設 canonical `deepseek-flash` = V4.1-Flash）；`deepseek-v4-pro` / `deepseek-pro` 已退役。

## 觀察窗口

- `SectorAgentLLM.LLM == nil` → runner 回 `ErrNotImplemented`，**預期行為**。
- `UseLLMSectorAgents` 預設關閉；backtest 須用 deterministic 路徑保證可重現性。

## 與 `internal/llm_annotator` 的分工

`internal/llm/adapters/` + `capabilities/` 是 canonical 介面；`internal/llm_annotator` 為早期 narrow 介面（保留相容）。新程式碼用 `capabilities/*Handler`。

## 測試

```bash
go test -run Integration ./internal/llm/...
```

新增 capability 必寫 handler 單元測試（mock router），確認 typed payload 轉換、fallback 與錯誤不吞。
