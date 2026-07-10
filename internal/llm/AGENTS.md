# AGENTS.md — internal/llm

LLM 整合基礎設施層：capability-based 多 Provider 路由 + DataClass 治理閘門。

> 設計權威：`docs/llm-integration-strategy-framework.md`  
> 路由表：`configs/llm_router.yaml`  
> 完整 Provider/Capability SOP：`.claude/skills/atlas-llm-provider-capability/SKILL.md`

## 公共 API 速查

- `NewDefaultRouter(impls ...ProviderImpl)`：啟動時 wiring。
- `NewDefaultRouterFromConfig(cfg, impls)`：載入 `configs/llm_router.yaml` 後呼叫。
- `DefaultRouter.Call(ctx, Request)`：**所有 LLM 呼叫的唯一入口**。
- `DefaultRouter.Health()`：`/api/llm/health` 資料來源。

12 個 `Capability` 常數列於 `provider.go:30-41`。新增 capability 必須同步 4 個位置（常數、schema、handler、router table），詳見 `.claude/skills/atlas-llm-provider-capability/SKILL.md`。

## 核心陷阱

1. **繞過 Router 直接呼叫 Provider**：會跳過 DataClass 閘門與 fallback chain。一律透過 `DefaultRouter.Call()` 或 `capabilities/*Handler`。
2. **S/E 模組直接 import**：會破壞 replay 可重現性。hot-path 呼叫必須 async 或 fallback-safe。
3. **MiniMax DataClass 繞過**：regulated 資料走 MiniMax 會回 `ErrProviderDisabled`（`router.go:110`）—— 有意設計（ADR-010）。
4. **新增 Capability**：必須同步 4 個位置（常數、schema、handler、router table），見 skill。

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
