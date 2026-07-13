# LLM 整合遷移路徑（Migration Roadmap）— RESOLVED

> **狀態**：✅ **RESOLVED**（2026-06 v0.0.0.21）
> **文件角色**：atlas-go LLM 整合的歷史遷移計畫。所有 Phase 1-3 已完成、Phase 4+ 已展開。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.1，本文件為其 §8 抽離，已 archive）
> **保留理由**：historical record（見 `docs/documentation-standard.md` §archive 用途）

## 完成狀態

| Phase | 期間 | 狀態 | 驗證 |
|-------|------|------|------|
| **Phase 1：介面統一 + Provider 常數擴張** | 週 1-2 | ✅ Done | PR #726, #730 |
| **Phase 2：接入第二、三 provider** | 週 3-6 | ✅ Done | PR #723, #733 |
| **Phase 3：能力擴張 + 備援驗證** | 週 7-12 | ✅ Done | PR #734, #740 |
| **Phase 4+** | TBD | ⏳ Open | OpenCode-Zen 整合、self-host M3 評估 |

**完成驗證**（v0.0.0.21）：12 個 capability handler 全部上線，3 個主力 provider 接通，備援鏈自動降級驗證成功，alert 規則 fire < 3 次/30 天。

---

## Phase 1：介面統一 + Provider 常數擴張（週 1-2）

**目標**：落地 `internal/llm` Router 骨架，與既有 `llm_annotator` 並存；Provider 常數從 3 個擴張為 7 個。

**任務清單**：

1. **建立 `internal/llm` package**（X 級）
   - 新增檔案：`internal/llm/provider.go`（§4.2 介面定義）
   - 新增檔案：`internal/llm/router.go`（`Router` 預設實作）
   - 新增檔案：`internal/llm/capabilities/failure_attribution.go`
   - 新增檔案：`internal/llm/adapters/annotator_adapter.go`（§4.4）
   - `internal/llm/doc.go`：寫明 `Maturity: experimental`，引用本文件

2. **Provider 常數擴張**（v2.0 新增）
   - 將 `ProviderKimi`、`ProviderMiniMax`、`ProviderDeepSeek`、`ProviderOpenCodeGo`、`ProviderOpenCodeZen`、`ProviderMock` 加入
   - 標記 `ProviderOpenAI` 為 DEPRECATED 註解（保留以避免破壞既有 config）

3. **endpoint 修正**（v2.0 新增）
   - `internal/llm_annotator/annotator.go:48-71` 的 `BaseURL` 預設值從 `https://api.moonshot.cn/v1` 改為 `https://api.kimi.com/coding/v1`
   - 同檔的 model 預設值保留 v1.0 的 `moonshot-v1-8k` 字串，但**加 deprecation 註解**指向 `kimi-k2.6` / `kimi-k2.7`
   - 驗證：grep 整個 codebase 確認沒有任何地方仍引用 `api.moonshot.cn`

4. **Router 健康端點**
   - 新增 `internal/llm/health.go` + `internal/monitoring/api/llm/handlers.go`
   - 註冊 `GET /api/llm/health`
   - 既有 `/health`（如果有）不動

5. **驗證**：
   - `lsp_diagnostics internal/llm/` 乾淨
   - `go test ./internal/llm/...` 通過
   - `go test ./internal/monitoring/api/strategies/...` 通過（既有測試）
   - 手動：`curl -X POST /api/strategies/{id}/annotate` 行為與 Phase 1 前**完全一致**（response shape、status code、metric labels；endpoint 內部 target 從 `moonshot.cn` 改為 `kimi.com` 對外部行為不可見）

6. **完成條件**：
   - 既有 503 fallback path 仍能運作（`handlers.go:283-287` 不變）
   - 既有 metrics label 集仍能 scrape（`llm_annotator_requests_total` 等）
   - 新 capability / provider label 是**附加**而非**取代**
   - codebase 全域無 `moonshot.cn` 殘留

## Phase 2：接入第二、三 provider（週 3-6）

**目標**：把 Rationale Translation（DeepSeek V4-Flash）、PRISM Cohort Insight（MiniMax M3）兩個 capability 透過第二、三 provider 接通。

**任務清單**：

1. **DeepSeek V4 provider 模組**（X 級）
   - 新增 `internal/llm/providers/deepseek/v4.go`：封裝 `https://api.deepseek.com` 呼叫
   - 支援 V4-Pro 與 V4-Flash 兩個 model
   - 沿用既有的 rate limiter / circuit breaker（從 `llm_annotator` 抽出的共用元件）
   - 記住：舊 `deepseek-chat` / `deepseek-reasoner` 於 2026/07/24 棄用，新實作只支援 `deepseek-v4-pro` / `deepseek-v4-flash`

2. **MiniMax M3 provider 模組**（X 級）
   - 新增 `internal/llm/providers/minimax/m3.go`：封裝 `https://api.minimax.io/v1` 呼叫
   - 支援 hosted 與 self-host 兩個 endpoint
   - `DataClass` 閘門在 adapter 層實作：`DataClassRegulated` 自動導向 self-host

3. **Capability: Rationale Translation Fallback**（P1）
   - 新增 `internal/llm/capabilities/rationale_translation.go`
   - 修改 `internal/monitoring/api/pipeline/handlers.go`（**非 rationale_corpus.go**）：在 `TranslateReason` 回傳值判斷「是否 passthrough」後，加一層 router 呼叫（§4.5.2 偽碼）
   - 新增 metric：`llm_annotator_requests_total{capability="narrative.rationale_translation_fallback",provider="deepseek"}`
   - **不修改** `internal/narrative/rationale_corpus.go` 的 invariant

4. **Capability: PRISM Cohort Insight**（P2）
   - 修改 `internal/orchestrator/prism_executor.go:94-102`（§4.5.4 偽碼）
   - `PRISMTrainingExecutor` 新增可選欄位 `insightRouter llm.Router`
   - `NewPRISMTrainingExecutor` 新增 setter `SetInsightRouter(router)`
   - **不改 `Run` 的既有回傳 contract**
   - v2.0 補充：因 PRISM 是 regulated，**生產環境必須先用 self-host M3**

5. **Router 配置**
   - `configs/llm_router.yaml`（新檔）：定義每個 capability 的 `RoutingChain`（primary/backup1/backup2/last resort）
   - `cmd/atlas/main.go`：啟動時載入此 config；缺檔時走 defaults

6. **驗證**：
   - 既有測試 100% 通過
   - 新增 capability 的 metric 與 record 都出現於 Prometheus 與 JSONL store
   - rationale corpus miss 時新行為：先看 corpus，miss 時呼叫 DeepSeek V4-Flash，failure 時降級到 M3，再失敗到 OpenCode-Go，最後 passthrough 原字
   - PRISM 訓練結案時新行為：self-host M3 為主，hosted M3 為 fallback，failure 時降級到 DeepSeek V4-Pro

7. **Eval：台灣股市情境 A/B 評測**（v2.1 新增，Phase 2 重點任務）
   - **目的**：在 Phase 2 接入第二、三 provider 後，用台灣股市真實情境驗證各 provider 對金融領域的適用性。
   - **金標準測試集**：從既有 `AnnotationStore`（`internal/llm_annotator/persistence.go:30-33`）的 JSONL 歷史記錄中抽取 20-30 筆具代表性的 StrategyFrame 失效歸因樣本，輔以人工標註的「理想歸因」，建立 `data/eval/tw-market-golden.jsonl`。
   - **評測對象**：DeepSeek V4-Pro（歸因 primary）、MiniMax M3（金融 narrative primary）、DeepSeek V4-Flash（翻譯 primary），加上 Kimi K2.6（作為對照組）。
   - **評測維度**：
     1. 歸因正確性（會計/策略專家 blind review，0-5 分）
     2. 台股特徵覆蓋（是否提及美元、美股連動、產業集中等 §3.1a 慣性特徵）
     3. 繁體中文自然度（native speaker review）
     4. Latency（P50 / P95）
     5. Cost per call
   - **輸出**：`docs/model-performance-matrix.md`，將結果與 §1a.6 矩陣交叉比對；若實測結果與 §1a 評分有顯著落差（>1 級），則需更新 §3.2 決策表的 primary 選擇。
   - **與 Phase 2 其他任務的關係**：此評測應在 capabilities 接通後、production 正式啟用前執行；結果作為 production routing 的 final gate。

8. **完成條件**：
   - rationale_corpus.go 檔案**無變更**（git diff 為空）
   - prism_executor.go 的 Run method signature **不變**
   - 兩個新 capability 都有可觀測的成功 / 失敗 metric
   - DataClass 閘門單元測試覆蓋率 = 100%
   - `docs/model-performance-matrix.md` 產出，且至少 3 個評測維度有量化數據

## Phase 3：能力擴張 + 備援驗證（週 7-12）

**目標**：把 Strategy Frame Summary、Prompt Lint、Confidence Commentary、Code Review Annotation 四個 capability 接通，並實際驗證備援鏈觸發。

**任務清單**：

1. **Capability: Strategy Frame Summary**（P2）
   - 新增 endpoint `GET /api/strategies/{id}/summary`
   - handler 透過 router 呼叫，failure 回空字串 + `backend` 欄位
   - primary = DeepSeek V4-Pro

2. **Capability: Prompt Lint**（P3，dev-only）
   - 新增 `cmd/lint-prompts`（U 級 utility）
   - CI 階段呼叫；非 runtime
   - 不寫 production JSONL store，僅 stderr
   - primary = DeepSeek V4-Flash；K2.7 為 backup1（因 K2.7 為 code model 對 prompt lint 程式碼部分有加分）

3. **Capability: Confidence Commentary**（P3）
   - `internal/risk` 整合 hook
   - **不修改** `RiskManager` 既有方法；只在旁路加 `EnrichedCommentary string` 欄位

4. **Capability: Code Review Annotation**（P3，v2.0 新增）
   - 新增 `cmd/lint-pr`（U 級 utility）
   - primary = Kimi K2.7（純程式碼模型）
   - **重要**：adapter 的 `Supports(CapabilityCodeReviewAnnotation)` 只在 `modelID == "kimi-k2.7"` 時回 true

5. **OpenCode-Go 備援驗證**
   - 啟動時刻意把所有 primary 標 unhealthy
   - 觸發一次 Rationale Translation
   - 驗證 metric 顯示 `from_provider="deepseek",to_provider="opencode-go"`
   - 驗證 alert `LLMRouterFallbackActivated` 在 fallback 觸發時 fire

6. **晉升 E 級評估包**
   - 收集 30 天 production metrics
   - 撰寫 `docs/llm-promotion-evaluation.md`：
     - circuit breaker 開啟累計時數（每 provider 各自）
     - JSONL store loss 率
     - 監控規則覆蓋率
     - 三 capability 接入清單
     - 備援鏈實際觸發次數
   - 走 PR review；通過則把 `internal/llm/doc.go` 的 `Maturity:` 改為 `evolving`，並更新 `internal/MATURITY.md:75-89`

7. **驗證**：
   - 所有 capability 都有對應 metric、record、trace
   - cost report 月度報表可生成（沿用 `CostReport`）
   - alert 規則（`monitoring/rules/llm_annotator_alerts.yml`）在 30 天內 fire < 3 次
   - OpenCode-Zen 從未被觸發（健康指標）

8. **完成條件**：
   - `llm_annotator` 與 `internal/llm` 同時晉升 E 級
   - `internal/MATURITY.md` 更新並通過一致性檢查（`maturity.md:120-122`）
   - 備援鏈至少觸發過 1 次（被驗證可運作）

## Phase 4+（不在本版本承諾範圍）

- **OpenCode-Zen 整合**：在 OpenCode-Go 連續失敗時自動啟動；需 Phase 3 備援驗證資料支持
- **Self-host MiniMax M3**：解除 §9 風險 8 的資料主權顧慮；需 M3 開源權重（440GB MXFP8 量化）部署可行性評估
- **`llm_annotator` E → S**：需 90 天穩定期
- **Agent runtime LLM**（PRISM 訓練時即時呼叫 LLM 評分個股建議）：需要「設計藍圖 → 實作」需求強度評估
