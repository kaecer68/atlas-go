# LLM 整合架構決策紀錄（LLM Decision Log）

> **文件角色**：atlas-go LLM 整合的 ADR（Architecture Decision Record）時序紀錄。每一條都附**理由**與**拒絕的替代方案**。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.1，本文件為其 §10 抽離）
> **新增 ADR 流程**：建立新章節，狀態 = Proposed → Accepted / Superseded；append-only，不覆寫既有紀錄。

---

> 重要架構決策的時序紀錄。每一條都附**理由**與**拒絕的替代方案**。

### ADR-001：保留 `Annotator` 介面，包進新 Router

- **日期**：v1.0（2026-06）
- **狀態**：Accepted
- **決策**：新 `internal/llm.Provider` 介面透過 adapter 包進既有 `llm_annotator.Annotator`（`doc.go:75-80`），不改後者簽章。
- **理由**：
  - 既有介面已被 `monitoring/api/strategies/handlers.go:20`、`:40`、`:302` 多處依賴；改簽章 = 改 production wiring。
  - Adapter pattern 保留所有既有投資（`KimiClient`、`CircuitBreaker`、`AnnotationStore`、`MetricsRecorder`）。
- **拒絕方案**：
  - **直接重寫 Annotator 為 Provider**：破壞性，無對稱效益。
  - **刪除 `llm_annotator`，全部併入 `internal/llm`**：違反 §1 保留原則；且 `internal/MATURITY.md:87` 已建立 X 級獨立性。

### ADR-002：以 Capability 為中心而非 Model 為中心

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：呼叫端只認 capability ID（`string`），不知道 provider。
- **理由**：
  - 同一個 capability 跨 provider 切換不需改呼叫端。
  - 測試時只需切到 mock，不需 mock 整個 client。
- **拒絕方案**：
  - **每個 capability 直接持 provider 物件**：耦合過緊；換 provider 要改 9 處呼叫端。
  - **使用 model id 作為字串參數**：把「OpenAI-compatible 但不同 model」這種常見切換變得繁瑣。

### ADR-003：rationale_corpus.go 不引入 LLM import

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：rationale corpus 維持純 Go map；LLM fallback 在呼叫端（pipeline handlers）實作。
- **理由**：
  - `rationale_corpus.go:15-21` 明文 invariant：「MUST NOT depend on an LLM, a translation API, or any external network resource」。
  - 保持 corpus 可離線測試；可在沒有網路的環境下完整跑 unit test。
- **拒絕方案**：
  - **把 LLM fallback 直接寫進 TranslateReason**：違反既有 invariant；測試成本上升。
  - **建立獨立 translator package**：增加模組數量；呼叫端還是要寫同樣的 glue code。

### ADR-004：S 級模組透過介面注入接受 X 級能力

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：`orchestrator/prism_executor.go` 與 `spawning/gap_detector.go` 等 S/E 級模組透過「可選 Router 欄位 + Setter」接受 LLM 能力，不直接 import `internal/llm`。
- **理由**：
  - 符合 `internal/MATURITY.md:75-78`：「X 級 experimental 模組，不應被 stable/evolving 模組依賴」。
  - 介面注入是 MATURITY 規則允許的標準做法（呼叫端持有 `Router` interface，不持有具體 client）。
  - 可選注入保證既有行為不退化。
- **拒絕方案**：
  - **直接 import internal/llm**：違反 MATURITY。
  - **把 internal/llm 晉升 E 級再 import**：時程太長；現有需求（PRISM cohort insight）需要更早接入。

### ADR-005（v2.0 重寫）：Capability-Based Multi-Provider 架構

- **日期**：v2.0（2026-06）
- **狀態**：Accepted（取代 v1.0 版「單一 Provider」決策）
- **決策**：atlas-go 使用三個主力 provider（DeepSeek V4-Pro / V4-Flash、MiniMax M3、Kimi K2.6 / K2.7）+ 兩個備援通道（OpenCode-Go、OpenCode-Zen），由 Router 依 §3.2 決策表 + 動態健康度自動路由；每個 capability 有四級 fallback 鏈。
- **理由**：
  - **沒有任何單一模型在所有 9 個 capability 上都最佳**：V4-Pro 強推理但成本高，V4-Flash 便宜但推理弱，M3 強金融但有資料主權風險，K2.7 強 code 但無 general instruct。
  - **K2.7 用於敘事任務會產生結構性幻覺**：因其為純程式碼模型（見 ADR-009）。
  - **DeepSeek V4-Pro 提供最強的中文 + 推理組合**：NIST 獨立評估為接近 GPT-5 級，IMOAnswerBench 89.8%、C-Eval 93.1%。
  - **MiniMax M3 提供最佳金融 + 繁中組合**：BankerToolBench 76.12、SpreadsheetBench 89.35；且支援 self-host，可解除 §9 風險 8。
  - **OpenCode-Go 提供訂閱制多模型 failover**：成本可控，作為統一 backup2。
  - **v1.0 的「單一 provider 為預設」是基於 v1.0 時點的「沒有 day-2 需求」假設**；v2.0 的需求（PRISM cohort insight、rationale fallback、confidence commentary）已經超過單一模型能舒適承擔的範圍。
- **拒絕方案**：
  - **Day-1 全部用 V4-Pro**（最強）：成本過高，rationale 翻譯這類簡單任務用 V4-Pro 是浪費。
  - **只加 OpenAI 作為 backup**（v1.0 路線）：v1.0 §6.5 列的觸發條件在 v2.0 之前已部分成立（30 天資料累積完成、production SLA 違規已記錄）；且用戶不使用 OpenAI。
  - **只用 MiniMax 一家**：M3 在 reasoning 與 abstract 任務落後 V4-Pro；金融場景以外的能力會降級。
  - **保留單一 provider 但升級到 V4-Pro**：無法解決 K2.7 敘事幻覺與 M3 金融需求的雙重任務差異。

### ADR-006：Capability Label 是 Append-only，不覆寫既有 metrics

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：所有新 metric 的 `capability` label 為附加；既有 `llm_annotator_requests_total{outcome=...}` 保持原樣。
- **理由**：
  - 既有 Prometheus alert rule（`monitoring/rules/llm_annotator_alerts.yml`）依賴既有 label 集。
  - Append-only 是 Prometheus metric evolution 的標準做法。
- **v2.0 補充**：v2.0 新增 `provider` label 沿用同樣原則；既有 alert 規則不受影響，新 alert 規則基於新 label 集。
- **拒絕方案**：
  - **把 capability 寫進既有 metric 的 label 值**：破壞既有 alert 與 dashboard。
  - **新建一套 metric**：浪費 label cardinality；運維成本高。

### ADR-007：trace 預設關閉，僅在 dispute 開啟

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：`Options.Trace == true` 才寫 trace；預設關閉。
- **理由**：
  - Trace 含 raw response，可能含敏感資料。
  - Trace 容量大；開啟會讓 JSONL store 迅速 50MB rotation。
  - 既有 `AnnotationRecord`（`observability.go:209-216`）已是 sufficient dispute signal；trace 是 optional enhancement。
- **v2.0 補充**：trace 結構新增 `fallback_chain` 與 `data_class` 兩個欄位；前者記錄實際 fallback 軌跡，後者用於合規 audit。

### ADR-008：`internal/llm` 與 `llm_annotator` 同為 X 級

- **日期**：v1.0
- **狀態**：Accepted
- **決策**：新建 `internal/llm` 設為 X 級；與 `llm_annotator` 並列於 `internal/MATURITY.md:75-89` 區段。
- **理由**：
  - Router 內部仍呼叫 `llm_annotator`；同為 X 是對稱且誠實的標記。
  - 晉升 E 級需 30 天穩定期；新模組與舊模組一起觀察。
- **拒絕方案**：
  - **`internal/llm` 直接設為 E**：沒有 30 天資料支持；會被 reviewer 打回。

### ADR-009（v2.0 新增）：Kimi K2.7 限縮於 Code-Only Capability

- **日期**：v2.0（2026-06）
- **狀態**：Accepted
- **決策**：Kimi K2.7 僅用於 code-related capability（`CapabilityCodeReviewAnnotation`、`CapabilityPromptLint` 的 code path）；**禁止**用於 Failure Attribution、Translation、Summary、Headline、Commentary、PRISM Insight、Gap Description。
- **理由**：
  - K2.7 為純程式碼模型，無 general instruct variant；強行用於非程式碼任務會產生幻覺。
  - 既有 C-Eval 92.5% / CMMLU 90.9% 是程式碼 / 推理 benchmark 表現；與「敘事輸出的可控性」是兩件事。
  - thinking mode 強制 ON，無法針對敘事任務關閉；會增加成本與 latency。
- **結構性 guard**：`internal/llm/adapters/annotator_adapter.go` 的 `Supports` method 對 K2.7 與 `CapabilityFailureAttribution` 等敘事 capability 回傳 `false`；路由器自動降級到 backup1。即使有人寫死 `Options.ForceProvider = ProviderKimi`，adapter 仍會拒絕呼叫。
- **拒絕方案**：
  - **完全禁用 K2.7**：浪費其在程式碼任務的優勢；Code Review Annotation 仍需要 code 模型。
  - **把 K2.7 用於所有任務**：產生幻覺；違反 §3.2 決策表依據。
  - **不寫結構性 guard，僅在文件警告**：文件可被忽略；需要程式碼層防護。

### ADR-010（v2.0 新增）：MiniMax M3 Hosted API 的資料主權閘門

- **日期**：v2.0（2026-06）
- **狀態**：Accepted
- **決策**：MiniMax M3 hosted API（`https://api.minimax.io/v1`）**不得**接收受規範金融資料；`DataClass == Regulated` 的 capability 在路由到 M3 hosted 之前必須先嘗試 self-host M3 或降級到 backup1。
- **理由**：
  - MiniMax M3 hosted API 的伺服器位於中國境內，受 2017 年《中華人民共和國國家安全法》管轄。
  - 受規範金融資料（PRISM 結果、RiskManager VaR、StrategyFrame 細節）跨境傳輸可能違反金融監理要求。
  - 營業秘密（universe selection logic、RiskManager 演算法）若透過 hosted API 送出，可能被視為對外揭露。
- **結構性 guard**：`internal/llm` Router 在 `DataClass == Regulated` 時自動拒絕 M3 hosted 路徑（見 §6.3 步驟 2）；`DataClass == Secret` 強制走 self-host。
- **長期路徑**：Phase 4+ 評估 self-host MiniMax M3（基於 minimax-community 授權的 440GB MXFP8 量化權重）作為 M3 的預設 endpoint，解除資料主權顧慮。
- **拒絕方案**：
  - **完全不接 M3**：失去最佳金融 + 繁中模型；PRISM insight、gap description 的品質會下降。
  - **M3 hosted 與 self-host 預設共存**（讓使用者選）：增加設定複雜度；多數使用者會選預設值，違背設計意圖。
  - **完全 self-host M3**：部署成本高（440GB 權重需 GPU 叢集）；短期不可行。
