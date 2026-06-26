# AGENTS.md — internal/llm

本目錄是 `atlas-go` 的 **LLM 整合基礎設施層**，提供 capability-based 多 Provider 路由 + DataClass 治理閘門 + 熱路徑護欄。

> **設計權威**：`docs/llm-integration-strategy-framework.md`（4-digit semver 規範）
> **路由表來源**：`configs/llm_router.yaml`（runtime）；fallback 預設見 `router.go:defaultRoutingTable()`
> **成熟度規則**：`internal/MATURITY.md` LLM 相關條目

---

## 子模組佈局

| 子目錄 | 行數規模 | 內容 |
|--------|---------|------|
| `clients/` | ~3K | 3 個 HTTP Provider client（DeepSeek / MiniMax / 共享 `BaseClient`）+ `metrics.go` |
| `schemas/` | ~2K | 9 個 capability 的 typed I/O contract（JSON Schema） |
| `capabilities/` | ~3K | 10 個 capability handler（typed payload → Router → typed response） |
| `adapters/` | ~1K | `Annotator` ↔ `llm.ProviderImpl` 雙向 bridge |

修改前先讀子目錄對應檔案，或用 GitNexus 查符號關係。

---

## 公共 API 速查

完整清單見 `doc.go:15-30`。最常用入口：

| 函數 | 用途 |
|------|------|
| `NewDefaultRouter(impls ...ProviderImpl) *DefaultRouter` | 啟動時 wiring，附帶 default routing table |
| `NewDefaultRouterFromConfig(cfg, impls)` | 載入 `configs/llm_router.yaml` 後呼叫 |
| `DefaultRouter.Call(ctx, Request) (Response, error)` | 所有 LLM 呼叫的唯一入口 |
| `DefaultRouter.Health() map[Provider]HealthStatus` | `/api/llm/health` |

> 12 個 `Capability` 常數列於 `provider.go:28-41`，新增 capability 必須同步 4 個位置（見 skill）。

---

## 核心陷阱

1. **繞過 Router 直接呼叫 Provider**：會跳過 DataClass 閘門與 fallback chain。一律透過 `DefaultRouter.Call()` 或 `capabilities/*Handler`。
2. **S/E 模組直接 import**：會破壞 replay 可重現性。hot-path 呼叫必須 async 或 fallback-safe。
3. **MiniMax DataClass 繞過**：regulated 資料走 MiniMax 會回 `ErrProviderDisabled`（`router.go:110`）——這是有意設計（ADR-010）。

**完整陷阱與 Provider/Capability 開發 SOP**見 **`.claude/skills/atlas-llm-provider-capability/SKILL.md`**。

---

## 環境變數

| 變數 | 用途 | 預設 |
|------|------|------|
| `LLM_MINIMAX_API_KEY` | MiniMax M3 API key | — |
| `LLM_DEEPSEEK_API_KEY` | DeepSeek V4-Pro / V4-Flash | — |
| `LLM_ANNOTATOR_API_KEY` | **向後相容** — 等同 `LLM_MINIMAX_API_KEY` | — |
| `LLM_RATIONALE_TRANSLATION_ENABLED` | rationale 翻譯 hook | `false` |
| `LLM_PRISM_SCENARIO_ENABLED` | PRISM scenario 說明 hook | `false` |
| `LLM_NARRATIVE_EXPLAIN_ENABLED` | regime + sentiment 解釋 hook | `false` |
| `LLM_RISK_FORENSICS_ENABLED` | performance forensics hook | `false` |

---

## 觀察窗口（Feature Flag）

- `SectorAgentLLM.LLM == nil` → runner 回 `ErrNotImplemented`，**這是預期行為**。
- `UseLLMSectorAgents` flag 預設關閉；backtest 必須用 deterministic 路徑保證可重現性。

---

## 測試慣例

```bash
go test ./internal/llm/...
go test -run Integration ./internal/llm/...   # 含 router + clients 端對端
```

新增 capability 必寫 handler 單元測試（mock router），確認 typed payload 轉換、router fallback 行為、錯誤不吞。

---

## 與 `internal/llm_annotator` 的分工

`internal/llm/adapters/` + `internal/llm/capabilities/` 是 Phase 2 canonical 介面；`internal/llm_annotator` 是早期 narrow 介面（保留向後相容）。新程式碼用 `capabilities/*Handler`，既有呼叫端保留 `llm_annotator`。

---

## 相關文件

- `docs/llm-integration-strategy-framework.md` — 設計權威
- `docs/llm-promotion-evaluation.md` — LLM 晉升評估流程
- `configs/llm_router.yaml` — runtime routing table
- `.claude/skills/atlas-llm-provider-capability/SKILL.md` — Provider/Capability 開發 SOP
