# LLM 整合架構決策紀錄（LLM Decision Log）

> **文件角色**：atlas-go LLM 整合的 ADR（Architecture Decision Record）時序紀錄。每一條都附**理由**與**拒絕的替代方案**。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.2，本文件為其 §10 抽離）
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
- **狀態**：Superseded by ADR-012（2026-09-11）
- **後續**：本閘門已由 ADR-012 拆除；`DataClass` 降為稽核 metadata。以下內文保留為歷史紀錄（append-only，不覆寫）。
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

### ADR-011：Capability name 對齊（Phase 2）— 未收錄

> **⚠️ 待補**：`docs/specs/llm-routing-spec.md` §6.1 引用了 ADR-011（Phase 2 capability name 對齊決策，採 Option B — 對齊 code → doc），但本 ADR log 尚未收錄該條目。此處僅為預留位置（append-only 精神），**不代為臆測內容**；補齊時請依上方「新增 ADR 流程」填入完整條目。

### ADR-012（2026-09-11 新增）：拆除 MiniMax DataClass 主權閘門，改以任務可達成率與訂閱額度選模型

- **日期**：2026-09-11
- **狀態**：Accepted（取代 ADR-010；ADR-009 的能力 guard 不受影響）
- **決策**：
  1. **拆除 provider 閘門**：`internal/llm/router.go` 的 `shouldGateProvider()` 與 `Call()` 內兩處呼叫已刪除；`internal/llm/clients/kimi.go` 對 `DataClassRegulated` / `DataClassSecret` 的 `ErrIncompatibleDataClass` 拒收已刪除。**ADR-009 的 `kimiAllowedCaps` 能力 guard 保留不動。**
  2. **`DataClass` 降為稽核 / 觀測 metadata**：enum 維持 `Unmarked` / `NonRegulated` / `Regulated` / `Secret` 四類；`DataClass` 繼續隨 `Request` 傳遞、記入 metric 與 span，作為日後 redaction 的依據，但**不再阻擋任何 provider**。
  3. **改以「任務可達成率 + 訂閱額度」選模型**：新路由鏈（三個群組）見 `docs/specs/llm-routing-spec.md` §6.1 —— 敘事 / 解釋 JSON 9 個 capability 以 MiniMax M3 為 primary；程式碼 2 個 capability（`code_review_annotation`、`prompt_lint`）以 kimi-for-coding 為 primary；`contra_attribution` 歸敘事群組。`configs/llm_router.yaml` 與 Go 的 `defaultRoutingTable()` 同步同一份鏈。
  4. **空輸出視為失敗**：Router 收到 provider「成功但 `Output` trim 後為空」一律視為該 provider 失敗，續試下一個鏈成員；`AttemptedProviders` 記錄全部嘗試過的 provider，`FallbackTriggeredTotal` 遞增。詳見 `docs/specs/llm-routing-spec.md` §6.3a。
  5. **模型名設定化**：新增環境變數 `LLM_DEEPSEEK_MODEL`（預設 canonical `deepseek-flash` = DeepSeek-V4.1-Flash），取代 `cmd/lint-pr`、`cmd/atlas`、`cmd/lint-prompts` 內硬編碼的 `deepseek-v4-pro`。`internal/llm/clients/deepseek.go` 的 `DefaultModelV4Pro` / `DefaultModelV4Flash` 常數保留但標 deprecated，新增 `DefaultModelV4_1Flash = "deepseek-flash"`。**`deepseek-v4-pro` / `deepseek-pro` 已退役，不得再作為派遣或預設模型。**
  6. **`max_tokens` 重新校準**：reasoning 模型（M3 / deepseek-flash）需要最低預算，舊值過小會讓 thinking 吃光預算而回空內容。舊值 → 新值明細見 `docs/specs/llm-routing-spec.md` §6.1a。
- **理由**：
  1. **閘門沒有主權收益，只有成本**：閘門只擋 MiniMax（上海 / 中國），但被擋下的資料實際落到 DeepSeek（`api.deepseek.com`，杭州）；kimi 為 Moonshot（北京）。三家同屬同一管轄區 → gate 沒有換到任何主權保障，只讓成本變約 5 倍。
  2. **ADR-010 的核心規則不可實作**：ADR-010 自己寫「`DataClass == Secret` 強制走 self-host」，但程式碼裡根本沒有 self-host provider（`internal/llm/provider.go` 無此常數、亦無任何自架實作）→ 該規則無對應程式碼可實作。
  3. **閘門的實際效果是能力退化與沉默空輸出**：gate 讓部分 capability 永久失去 primary、掉到 backup；並在 production 造成沉默空輸出 —— primary 被跳過後，backup 以過小的 `max_tokens` 讓 reasoning 吃光預算，回傳成功（success）但 `output` 為空，**呼叫端拿到空字串且無 error**。這也是本次同步校準 `max_tokens` 並將空輸出視為失敗的直接原因。
  4. **主權控制的粒度錯了**：若真的要「不讓第三方處理」，正確控制是「全供應商一致封鎖」或「self-host」，而不是挑一家廠商擋。此點列入下方 residual risk。
- **拒絕方案**：
  - **保留閘門 + 補上 self-host M3**（ADR-010 的長期路徑）：self-host 在本次決策時點未實作；在自架落地前，閘門仍會持續製造沉默空輸出，因此不能作為「不改程式碼」的理由。
  - **把閘門從 MiniMax 擴大到所有中國境內 provider（MiniMax + DeepSeek + Kimi）**：等於停用全部主力 provider，在 self-host 就緒前不可行。
  - **只改文件警告、不動程式碼**：gate 的退化行為來自程式碼（provider 被跳過、backup 預算不足），文件警告無法阻止空輸出，必須以程式碼與路由鏈修正。
- **Residual risk / 未決事項**：
  1. **主權風險未消除，只是不再由 Router 以「單一 provider 黑名單」處理**。未決：是否改採「全供應商一致封鎖」或「self-host」；self-host 目標模型與部署時程均未定。
  2. `DataClass` 現為純 metadata；若日後要作為 redaction 的觸發條件，需先確認各 capability 的 `DataClass` 填值正確（現況部分 capability 在未設定時自行預設 `Regulated`）。此屬稽核面向工作，不影響路由。
  3. **ADR-011（capability name 對齊）未收錄於本 log**：`docs/specs/llm-routing-spec.md` §6.1 已引用，待補（見上方「ADR-011」預留位置）。
  4. **`contra_attribution` 目前無 handler 實作**：路由鏈已定義（primary minimax / backup1 deepseek / last_resort mock），但無 `max_tokens` 可校準；handler 落地時需另行定義 last resort 語意。


### ADR-012 追加（2026-09-11，PR #1886 第二階段）：production 實證與後續修正

**production 實證（iMac，48 小時 log）**：`llm.scenario_simulation` 被呼叫 **663 次**，每一次 span 的
`llm.attempted_providers` 都是 `["deepseek"]`、`llm.data_class` 皆是 `2`（Regulated）——閘門確實讓 M3
從未上場，且該 hook 的輸出因此在 production 一直是空的。這證實本 ADR 的「沉默空輸出」判斷。

**後續修正**：

1. **annotator（`/api/strategies/{id}/annotate`）設定錯誤**：`cmd/atlas` 以 `llm_annotator` 預設值啟動
   （BaseURL `https://api.kimi.com/coding/v1`、model `moonshot-v1-8k`），但 key 是 MiniMax CN coding-plan →
   實測 HTTP **401**，該端點不可能成功。已改為指向 MiniMax CN（`clients.MiniMaxChatBaseV1` + `MiniMax-M3`）
   並把 token 預算由 512 提到 2048；`llm_annotator` client 與 `/annotate` handler 兩層都改為
   **空輸出視為失敗**（不再回 HTTP 200 + 空註解，改回 502 + rule-based fallback）。
2. **M3 回應正規化**：M3 把 native thinking 內嵌在 `message.content`（`<think>…</think>` + 答案）。
   `internal/llm/clients` 與 `internal/llm_annotator` 的 client 現在剝除該區塊；若 thinking 被 `max_tokens`
   截斷（有開無關），剝除後為空 → 依本 ADR 的空輸出規則視為失敗。DeepSeek V4.1-Flash 的 thinking 走獨立
   `reasoning_content`，`content` 本來就乾淨（實測）。
3. **鏈成員記帳語意**：`Response.AttemptedProviders` 只記**實際被呼叫**的 provider（使其成為可信的
   「這筆資料送給了誰」依據）；無法被呼叫的成員（未註冊、不支援該 capability）改記在 span 的
   `llm.skipped_providers`；`FallbackTriggeredTotal` 只在**實際呼叫**非 primary 成員時遞增。
4. **路由表改由設定檔載入**：`llm.ResolveRouterConfig()` 讀 `configs/llm_router.yaml`
   （`ATLAS_LLM_ROUTER_CONFIG_PATH` 可覆寫；檔案缺失、格式錯誤或**未涵蓋全部 12 個 capability** 時回退內建
   `defaultRoutingTable()`），三個 cmd 都改走此路徑並記錄來源。這修掉「YAML 是死的鏡像」的落差。

**未決事項（已開 issue）**：

- #1887 — `CapabilityFailureAttribution` 的 Router 路徑契約不合（`FailureContext` vs `[]byte`；`AnnotatorAdapter`
  僅以 `ProviderKimi` 註冊且不在該鏈）→ 死路徑，需決定修契約、給 legacy annotator 獨立 provider 槽，或刪除。
- #1888 — `risk.PerformanceForensics` 在 production **永遠不會被觸發**（`returnHistory ≥ 30` 且該欄位從未持久化／還原；
  405,710 行 log 內 `llm.performance_forensics` span = 0）→ 需決定還原歷史、改 gate、加驗證入口，或關閉 flag。
- #1889 — 資料主權 residual risk 的處置（全供應商一致封鎖 / redaction 層 / self-host M3）。

**主權事實補充**：三個上游供應商與端點分別為 MiniMax（`api.minimaxi.com`）、DeepSeek（`api.deepseek.com`，
杭州）、Kimi/Moonshot（`api.kimi.com`，北京），同屬一管轄區；目前送出的 payload 為風險指標與訓練統計
（`RiskSnapshot` 的 VaR/CVaR/回撤、`TrainingResult` 的勝率/Sharpe 等），不含個資或使用者識別資訊——這降低但
不消除 #1889 的風險。

#### 追加二（2026-09-11 晚）：#1887 / #1888 的處置

- **#1887 `failure_attribution` Router 路徑契約（已修）**：根因是雙軌抽象下的 payload 契約不合——handler 送 `FailureContext` struct，而 MiniMax/DeepSeek adapter 只吃 `[]byte`（messages JSON），唯一能吃 struct 的 `AnnotatorAdapter` 又只用 `ProviderKimi` 註冊且不在該鏈上。
  修法（不新增任何 prompt 語意）：把 legacy 的 prompt 文字抽成單一來源（`llm_annotator.FailureAttributionSystemPrompt` / `FailureContextPrompt` /
  `FailureAttributionTemperature`），讓 capability handler 用它組出 messages payload；`RouterAnnotator` 也改走同一個 handler。新增 handler→adapter→httptest 的端到端測試。
  仍未做（另案）：把 production `/annotate` 由 legacy `KimiClient` 遷移到 Router（`dashboard.SetStrategiesAnnotator` 目前對 `*llm_annotator.KimiClient` 有具體型別斷言）。
- **#1888 risk forensics hook 觸發不到（已修，方案 A）**：`returnHistory` / `portfolioHistory` 是 per-process 累積，而 `simulation_state.json` 的
  `EquityCurve` / `DailyReturns` 就是同一組序列（同引擎、同定義）→ 載入持久化狀態時補齊，gate 才可能成立。新增 `RiskForensicsMinSamples = 30` 常數與
  端到端測試（補齊 35 筆 → 單次 `RunDailySimulation` → hook 被呼叫）。
  ⚠️ 生產時程：iMac 目前 `daily_returns=18`，每天一次 daily simulation → 約 **12 個交易日**後才會首次產生風險鑑識敘事（此後自癒）。
- **#1889 主權決策**：仍待決（見下方 residual risk）。


#### 追加三（2026-09-12）：資料主權處置決策 = A（接受 + 稽核 + 最小化）

**決策**：業主裁定「策略內部邏輯外流」為**可接受風險** → 採 **A 案**：維持現行 provider 配置（不因資料分級封鎖任何 provider），並以下列機制把風險維持在可稽核狀態：

1. **可歸屬稽核**：`Response.AttemptedProviders`（只列實際被呼叫的 provider）+ span `llm.skipped_providers` + `llm.data_class` / `llm.data_class_name` → 任何時候都能回答「這筆資料送給哪一家」。
2. **最小化**：送出的 payload 僅為衍生／彙總資料（風險指標、訓練統計、事件情緒、策略名稱與失敗原因），不含個資、帳號、憑證或客戶資料。
3. **不可自動擴大**：新增 capability 或新增 provider 時，必須回到本 ADR 檢視 payload 內容；`DataClass` 仍隨請求傳遞（不作為閘門）。

**升級條件（不是「永遠不做」，而是「達到下列條件才做」）**：

| 觸發條件 | 應採取 | 具體做法 |
|---|---|---|
| 某個 capability 的 payload 被判定含**關鍵營業秘密**（最可能是 `failure_attribution`：frame_id + 條件 + 門檻 + 巨集值） | **D：針對性去識別化** | 只送推導結果（hit/miss、z-score 級距）而非絕對門檻值；以測試鎖住 payload 不含原始門檻 |
| 業主判定策略內部邏輯**必須完全不出境**，且要保留敘事功能 | **C：self-host** | 為 9 個敘事 capability 部署本地模型（品質低於 M3）；需 GPU 與維運預算，屬專案級工作 |
| 法規要求「不得讓第三方處理」時 | **B：全供應商一致封鎖** | 只在此時才停用相關 capability（本 ADR 已記錄：這會直接失去功能） |

**為何不是 B**：三家 provider（MiniMax CN / DeepSeek 杭州 / Kimi 北京）同屬一管轄區，逐家封鎖沒有主權收益（ADR-010 的教訓）；且本系統送出的資料不含受監理的個資，「合規」並非真正的約束，真正的約束是營業秘密 —— 而業主已明示接受。


#### 追加四（2026-09-12）：Kimi 不再納入 atlas 的 provider 設定（決策，非缺陷）

**決策**：atlas 的執行期**不使用 Kimi provider**，且**不補 `LLM_KIMI_API_KEY`**。理由（業主裁定 + 實測）：

- 本部署僅有 Kimi **coding plan** 訂閱，該 key **無法用於 app-level HTTP 呼叫**（實測 `api.kimi.com/coding/v1` 回 **401**；`api.minimaxi.com` 用同一把 key 則 200）—— 補了也無法生效。
- 既有的 `code_review_annotation` / `prompt_lint` 以 kimi 為 primary 的鏈，等於**永久不可達**：每次呼叫都會在 span 留下 `llm.skipped_providers=[kimi]`，並讓 `FallbackTriggeredTotal` 無意義地 +1，也讓「要不要補 key」這個問題反覆出現。

**處置**：

1. `defaultRoutingTable()` 與 `configs/llm_router.yaml` 的兩個 code capability 改為 **`minimax` → `deepseek` →（空）→ `mock`**（與敘事群組同型，2 層鏈）。
2. `cmd/atlas`、`cmd/lint-pr`、`cmd/lint-prompts` **移除 Kimi 的 provider 註冊**（先前在 key 存在時註冊）。
3. **保留** `clients.KimiClient`、`adapters.KimiAdapter` 與 ADR-009 的 `kimiAllowedCaps` 能力 guard（程式碼與測試不動）—— 若日後取得可用於應用流量的 Kimi/Moonshot API key，只需在 `router.go` + `configs/llm_router.yaml` 各加回一行，並恢復註冊。
4. `docs/specs/llm-routing-spec.md` §6.1 更新；`~/.config/atlas-go/.env`（兩台機器）註解同步為「本部署不使用 Kimi」。

**與 ADR-009 的關係**：ADR-009（K2.7 僅限 code capability）仍然有效且**未被推翻** —— 本追加只是說「本部署沒有可用的 Kimi key，因此鏈上不放它」；若未來有可用 key，K2.7 仍只應服務 code capability。

**開發環境（prime-agent / LiteLLM）**：Kimi 月額度用罄，約兩週後自動恢復；那與 atlas 執行期無關（不同 key/用途）。
