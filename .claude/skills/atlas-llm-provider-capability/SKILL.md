---
name: atlas-llm-provider-capability
description: "SOP for adding a new LLM provider client or capability handler in atlas-go. Covers BaseClient embedding, ProviderImpl interface, capability constant registration, routing table updates, handler wiring, and required tests. Trigger: user asks to add a new LLM provider, capability, or routing chain entry."
version: "1.0"
category: feature
auto_load: false
load_policy: manual_only
created: "2026-06-26"
updated: "2026-06-26"
target_audience: developer
---

# Atlas LLM Provider / Capability 新增 SOP

本技能規範在 `atlas-go` 新增 LLM **Provider client** 或 **Capability handler** 的標準步驟，確保 DataClass 閘門、fallback chain、型別契約與測試一併到位。

## 何時觸發

- 使用者要求「新增一個 LLM provider」（例如 Kimi、MiniMax、DeepSeek 以外的模型端點）
- 使用者要求「新增一個 LLM capability」（例如新的歸因/摘要/偵測能力）
- 需要修改 `configs/llm_router.yaml` 的 routing chain
- 在 `internal/llm/` 新增 `clients/`、`capabilities/`、`schemas/` 程式碼時

## 核心概念

### Provider client

- 定義：實作 `internal/llm.ProviderImpl` 介面的 HTTP client，負責把 `llm.Request` 序列化為 provider 原生格式並回傳 `llm.Response`。
- 實作位置：`internal/llm/clients/<name>.go`
- 關鍵：必須嵌入 `BaseClient` 以繼承 retry、rate-limit、circuit breaker、metrics。

### Capability handler

- 定義：位於 `internal/llm/capabilities/<name>/` 的 typed facade，把領域 payload 轉成 `llm.Request`，透過 `DefaultRouter.Call()` 發送，再將結果 unmarshal 回 typed response。
- 實作位置：`internal/llm/capabilities/<name>/handler.go`
- 關鍵：所有 LLM 呼叫必須經由 `DefaultRouter`，禁止直接呼叫 provider client。

### Routing table

- 定義：Primary → Backup1 → Backup2 → LastResort 的 4 層 fallback chain，決定某 capability 使用哪些 provider。
- 來源：`configs/llm_router.yaml`（runtime）與 `router.go:defaultRoutingTable()`（fallback）。
- 關鍵：兩者必須同步，否則 YAML 載入失敗時行為會漂移。

## 實作位置

| 概念 | 檔案路徑 | 關鍵函數 / 結構 |
|------|---------|----------------|
| Provider 介面 | `internal/llm/provider.go` | `ProviderImpl`、`Capability`、`DataClass`、`Provider` 常數 |
| Router 實作 | `internal/llm/router.go` | `DefaultRouter.Call()`、`defaultRoutingTable()`、`isKnownProvider()` |
| 設定驗證 | `internal/llm/config.go` | `LoadRouterConfig()`、`isKnownCapability()` |
| Provider client 基底 | `internal/llm/clients/base.go` | `BaseClient` |
| Routing table YAML | `configs/llm_router.yaml` | `routing_chains` |
| Maturity 登錄 | `internal/MATURITY.md` | LLM 相關條目 |

## 新增 Provider client

1. 在 `internal/llm/clients/<name>.go` 新建檔案，**嵌入 `BaseClient`** 取得 retry / rate-limit / circuit breaker / metrics。
2. 實作 `ProviderImpl.Supports(cap Capability) bool` — 列出支援的 capability 清單。
3. 實作 `ProviderImpl.Call(ctx, req Request) (Response, error)` — 序列化 request 到 provider 原生格式，回傳 `Response{Provider, Usage, Latency}`。
4. 在 `internal/llm/router.go:isKnownProvider()` 加入新 provider。
5. 在 `internal/llm/provider.go` Provider 常數區塊宣告。
6. **禁止**硬編碼 API key — 一律透過 `apigateway.Fetch(channelID)` 或啟動時注入。

## 新增 Capability handler

1. 在 `internal/llm/capabilities/<name>/` 新建 `handler.go`，定義 `<Name>Handler` struct + `New<Name>Handler(router llm.Router)` constructor。
2. 實作 typed `<Name>Payload` 與 `<Name>Response`（放在 `internal/llm/schemas/<name>.go`）。
3. handler 內部把 payload 包成 `llm.Request{Capability, Payload, DataClass, Options}`，呼叫 `router.Call()`，unmarshal 回 typed response。
4. 在 `internal/llm/provider.go` 新增 `Capability<Name>` 常數。
5. 在 `internal/llm/router.go:defaultRoutingTable()` 加入 routing chain。
6. 在 `internal/llm/config.go:isKnownCapability()` 加入 switch case。
7. 在 `configs/llm_router.yaml` 加入 `routing_chains` entry。
8. 在 `internal/llm/capabilities/<name>/handler_test.go` 寫 mock router 測試。

## 修改 routing table

- 改 `configs/llm_router.yaml`（runtime 來源）— **首選**。
- 改 `internal/llm/router.go:defaultRoutingTable()`（fallback 預設，僅在 YAML 不可用時生效）。
- 兩者**必須同步**，否則重啟時行為可能漂移。

## 驗證規則

- [ ] 新增 provider 有對應的 `_test.go`，測試 `Supports()` 與錯誤處理路徑
- [ ] 新增 capability 有 mock router 單元測試，驗證 typed payload → Request → typed response
- [ ] `provider.go`、`router.go`、`config.go`、`configs/llm_router.yaml` 四處同步
- [ ] `go test ./internal/llm/...` 通過
- [ ] `gofmt`、`go vet`、`staticcheck` 無錯誤
- [ ] 未直接 import provider client到業務邏輯（須經由 `DefaultRouter` 或 capability handler）

## 相關技能

| 技能 | 關聯 |
|------|------|
| `atlas-pre-change-protocol` | 修改 `internal/llm/` 前必須執行 7 步檢查 |

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-06-26 | 從 `internal/llm/AGENTS.md` 抽出成獨立 skill |
