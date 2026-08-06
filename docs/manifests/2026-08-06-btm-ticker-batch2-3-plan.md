# BTM Ticker Migration — Batch 2/3 方案 — Issue #1447 (最終版, 2026-08-06)

> **狀態**: 方案 (v3 — 全部 17 個 ticker 評估完畢, **0 個可遷**)
> **對應**: Issue #1447 + Batch 1 PR #1471 (merge commit 7cc83475)
> **結論**: Issue #1447 在憲章 §4.5.2 框架下沒有剩餘可遷的 ticker

---

## 0. 修正歷史

| 版本 | commit | 主要修正 |
|------|--------|---------|
| v1 | `109a8117` | 甩 6 個決策問題給 kaecer — **AI 失職** |
| v2 | `63372905` (amend) | 9 個中嚴重逐個判斷, 列 4 個可遷候選 |
| v3 (本版) | (amend) | **4 個候選全部不可遷** — 已逐個驗證 |
| 結論 | — | **Issue #1447 全部 17 個 ticker 沒有可遷的** |

---

## 1. v2 → v3 修正 — 4 個候選的真實狀態

### 1.1 #5 `mcp/anomaly/emitter.go:118` AnomalyEmitter.Run

**v2 判斷**: 可遷（簡單）

**v3 真實** (grep 證實):
- `cmd/atlas-mcp/server/server.go:118-125` 寫: `go anomalyEmitter.Run(ctx)`
- **grep `apigateway.BackgroundTaskManager` in `cmd/atlas-mcp/`: 0 結果**
- **mcp binary 完全沒有 BTM 架構**
- 改 BTM 需整個 mcp 引入 BTM + gateway = 違憲章 §4.5.2 例外「DI 元件」精神

**v3 結論**: **不可遷**

### 1.2 #15 `monitoring/rules.go:55` RuleEngine

**v2 判斷**: 可遷但需 RFC

**v3 真實** (grep + 憲章查證):
- `cmd/atlas/main.go:1554-1562` **已註冊** `rule_engine_check` BTM task:
  ```go
  _ = taskMgr.Register(&apigateway.ScheduledTask{
      Name:     "rule_engine_check",
      Interval: time.Duration(params.RuleEngineIntervalSec.Value) * time.Second,
      Enabled:  true,
      Task: func(ctx context.Context) error {
          ruleEngine.EvaluateRules(nil)
          ...
      },
  })
  ```
- `internal/apigateway/CONSTITUTION.md:262` **明文列為例外**:
  > `ruleEngine.Start(ctx, stateStore)`（live mode 專用；api mode 已透過 TaskManager `rule_engine_check` 使用 `EvaluateRules(nil)`）
- `RuleEngine.Start` 是 live-mode 狀態評估器, 與 BTM `rule_engine_check` **互補** (a2-tasks.md:116)

**v3 結論**: **不可遷** (憲章明文例外 + BTM task 已存在)

### 1.3 #16 `cmd/atlas-mcp/server/server.go:209` 24h ticker

**v2 判斷**: 可遷（簡單）

**v3 真實**: 同 #1.1 — mcp 沒 BTM 架構, 需整套引入 = 違憲章例外

**v3 結論**: **不可遷**

### 1.4 #17 `cmd/atlas-mcp/server/ratelimit.go:140` RateLimiter idleSweep

**v2 判斷**: 可遷（簡單, 有測試）

**v3 真實**: 同 #1.1 — mcp 沒 BTM 架構, 違憲章例外

**v3 結論**: **不可遷**

---

## 2. 17 個 ticker 完整真實評估

| # | 位置 | 真實狀態 | 證據 |
|---|------|---------|------|
| 1 | `marketdata/fubon_client.go:308` runHealthProbe | 不遷 (backoff 狀態機) | 方案 §1.2 對 |
| 2 | `marketdata/streaming.go:26` PollingAdapter | 不遷 (改 API 語意) | 影響 `StreamingProvider` interface, 9 個 callers |
| 3,4 | `realtime/router.go:104,232` | 不遷 (共用 cancelCtx) | 拆 BTM 破壞單一 cancel 語意 |
| 5 | `mcp/anomaly/emitter.go:118` | **不遷 (mcp 沒 BTM)** | v3 修正 |
| 6 | `metalearning/metalearner.go:292` | 不遷 (dead code) | `SetMetaLearner` 0 non-test caller |
| 7-10 | `monitoring/service/*` 4 個 detector | 不遷 (Wave9 supervisor) | `wave9_runtime.go:132-285` 統一 lifecycle |
| 11 | `prism_manager.go:569` autoBalancer | ✅ **已遷** | PR #1471 merge |
| 12 | `realtime/regime_adapter.go:268` | 不遷 (100ms sub-second + flag gate) | 方案 §1.2 對 |
| 13 | `spawning_manager.go:104` | 不遷 (憲章明文例外) | CONSTITUTION §4.5.2 spawning path |
| 14 | `live/scheduler.go:115,170,198` | 不遷 (憲章明文例外 + 動態 ticker) | live ruleEngine 例外 + marketTimeScheduler 動態 ticker |
| 15 | `monitoring/rules.go:55` RuleEngine | **不遷 (憲章明文例外 + BTM 已存在)** | v3 修正, CONSTITUTION.md:262 |
| 16 | `atlas-mcp/server/server.go:209` | **不遷 (mcp 沒 BTM)** | v3 修正 |
| 17 | `atlas-mcp/server/ratelimit.go:140` | **不遷 (mcp 沒 BTM)** | v3 修正 |

**結論**: 17 個 ticker 中, **15 個明確不遷, 1 個已遷, 1 個 dead code 待 deprecation**。

---

## 3. 對 Issue #1447 的真實結論

**Issue #1447 經完整評估後, 在憲章 §4.5.2 框架下沒有剩餘可遷的 ticker**。

剩餘可做:
1. **metalearner deprecation 註解** (cosmetic, 1 行) — 標明 `production dead code` 與 archive doc 引用
2. **a5-violations.md 標記為 outdated** (doc update) — 17 個 ticker 中 15 個已驗證為憲章例外, 不再是違規

**給 kaecer 的真實問題**:
- 1. Issue #1447 是否 close? (17 個 ticker 全部評估完, 沒 code change 可做)
- 2. metalearner deprecation 1 行是否開 PR? (low value but cleanup)
- 3. a5-violations.md 是否要更新反映 15 個實際例外? (文件同步)

這 3 個都是 closure 方向, 不是新實作。

---

## 4. 給下個 session 的指引

- **不要再評估「還有哪些 ticker 可遷」** — 17 個已逐個評估, 0 個可遷
- **不要再 grep 找「漏掉的 ticker」** — a5 audit 17 個清單已覆蓋
- **不要開 RFC 改憲章 §4.5.2** — 那是 design decision, 屬 kaecer
- **可做**: metalearner deprecation 1 行 + a5-violations 同步 (1 個 PR)

---

## 5. 學到的失誤 (v1 → v2 → v3 教訓)

| 階段 | 失誤 | 修正 |
|------|------|------|
| v1 | 甩 6 個決策問題 | 補 grep 證據, 給判斷 |
| v2 | 沒驗證候選 ticker 所在套件是否有 BTM 架構就說「可遷」 | 補 grep `apigateway.BackgroundTaskManager` 確認 |
| v2 | 沒查憲章 CONSTITUTION.md line 262 已明文列 RuleEngine 為例外 | 補查憲章例外清單 |
| v2 | 沒驗證 BTM `Register` 對 `ChannelID != ""` 的 contract 限制 | codegraph 看 Register 完整 body |

**核心教訓**: 「**可遷移**」的判斷需要 3 重驗證:
1. 該 ticker 所在套件**有 BTM 架構**
2. 該 ticker **不在憲章 §4.5.2 明文例外清單**
3. 該 ticker 改 BTM **不違反 Register contract** (ChannelID 必須在 gateway)

v1/v2 我都只做了第 2 項, v3 才補齊三項。

---

## 6. 參考

- 憲章: `internal/apigateway/CONSTITUTION.md §4.5.2` (line 261-265 完整例外清單)
- a5 audit: `docs/manifests/2026-07-25-channel-architecture-audit/a2-tasks.json:116` (RuleEngine 註解)
- a5 audit a4: `a2-tasks.md` line 38 (BTM task 完整清單)
- Batch 1 PR: #1471 (merge commit 7cc83475)
- 已存在的 BTM `rule_engine_check`: `cmd/atlas/main.go:1554-1562`
- BTM Register contract: `internal/apigateway/background.go:162-185`
- Wave9 supervisor: `internal/monitoring/wave9_runtime.go:132-285`
- Issue: #1447
