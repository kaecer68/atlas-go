# LLM Provider 路由策略（Provider Routing Strategy）

> **文件角色**：atlas-go LLM 多 Provider 路由 + 備援鏈的完整規格（架構藍圖 §6 抽離）。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.2）
> **Maturity 規則**：`internal/MATURITY.md` LLM 相關條目；本文件為 X 級，僅供 reference，不直接 import

---

## 六、Provider 路由策略

> **v2.0 重寫**：本章由原本的「單一 provider 預設」改為「多 provider 依 capability 路由 + 備援鏈」。每個 capability 的 primary 選擇依 §3.2 決策表，備援鏈依健康度動態降級。
>
> **v2.2 修訂（2026-09-11，ADR-012）**：路由鏈改以「任務可達成率 + 訂閱額度」選型，不再以資料管轄區（DataClass）決定 provider；ADR-010 的 DataClass 閘門已拆除，`DataClass` 降為稽核 metadata。路由表、`max_tokens` 校準與「空輸出視為失敗」行為見 §6.1、§6.1a、§6.3a。

### 6.1 路由表（每 capability 的 fallback 鏈）

**Capability column 命名約定**：本表 capability column 採用 `internal/llm/provider.go` 中 Capability 常數的字串值（即 `CapabilityFailureAttribution` 的字面值），Phase 1 開發期使用的 dotted name（如 `strategy.failure_attribution`、`narrative.rationale_translation_fallback`）僅作為 §3 capability taxonomy 的歷史參照，已不具權威性。Phase 2 整合時若 doc scope 與 code scope 不一致（如 `rationale_translation_fallback` vs `rationale_generation`），需另立 ADR 記錄決策（目前 ADR-011 處理六處落差）。

| Capability | Primary | Backup1 | Backup2 | Last Resort |
|------------|---------|---------|---------|-------------|
| `failure_attribution` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `rule_based`（`frame.Attribution`） |
| `rationale_generation` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `passthrough`（原字回傳） |
| `regime_explanation` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `passthrough` |
| `sentiment_explanation` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `passthrough` |
| `confidence_commentary` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `empty`（不產生旁註；`ErrAllProvidersFailed` 時 handler 回空回應） |
| `performance_forensics` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `passthrough` |
| `risk_surface_extraction` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `passthrough`（保留低覆蓋率原描述） |
| `scenario_simulation` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `discard`（不存） |
| `strategy_summary` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | `null`（端點回空字串） |
| `code_review_annotation` | `kimi (kimi-for-coding)` | `minimax (M3)` | `deepseek (deepseek-flash)` | `empty`（無註解） |
| `prompt_lint` | `kimi (kimi-for-coding)` | `minimax (M3)` | `deepseek (deepseek-flash)` | `pass`（CI 不擋） |
| `contra_attribution` | `minimax (M3)` | `deepseek (deepseek-flash)` | （空） | （待定；目前無 handler 實作，Phase 2 落地時定義） |

**三個群組（ADR-012）**：
- **敘事 / 解釋 JSON（9 個，primary = MiniMax M3）**：`failure_attribution`、`rationale_generation`、`regime_explanation`、`sentiment_explanation`、`confidence_commentary`、`performance_forensics`、`risk_surface_extraction`、`scenario_simulation`、`strategy_summary`。M3 走訂閱額度（邊際成本低）且繁中金融敘事為其強項；backup1 一律為 `deepseek (deepseek-flash)`。
- **程式碼（2 個，primary = kimi `kimi-for-coding`）**：`code_review_annotation`、`prompt_lint`。ADR-009 的能力 guard 使這兩個 capability 僅 kimi 可承接；未設定 kimi key 時 Router 自動落到 backup1（M3），再落到 backup2（deepseek-flash）。這是唯一使用完整三層鏈的群組。
  - **production 現況（2026-09-11 起，屬預期）**：iMac 的 `.env` **未設 `LLM_KIMI_API_KEY`**（Kimi coding plan 月額度用罄；額度恢復後才會設回）。因此 code 群組在 production 會**跳過 kimi**（出現於 span 的 `llm.skipped_providers`）並由 M3 承接，`FallbackTriggeredTotal` 每次 +1。`cmd/lint-pr` / `cmd/lint-prompts` 已在 key 存在時註冊 Kimi adapter，所以 key 一旦設回，primary 立即生效，不需改程式。
- **`contra_attribution`（對抗式敘事分析，第 12 個 capability）**：與敘事群組同鏈（primary minimax / backup1 deepseek / backup2 空 / last_resort mock）。目前**無 handler 實作**，故無 `max_tokens` 可校準。
- 全域 fallback 模型名一律用 canonical `deepseek-flash`（= DeepSeek-V4.1-Flash）；`deepseek-v4-pro` / `deepseek-pro` **已退役**，不得再作為派遣或預設模型（ADR-012）。

> **Last Resort 欄語意**：本欄記錄「非 provider 的降級行為」（`rule_based` / `passthrough` / `null` / `pass` / `discard` / `empty`）。Phase 1 實作把 last resort 一律落到 `mock` provider 並回空 `Output`；各 capability 的語意化降級屬 Phase 2 handler 範圍（見 `internal/llm/router.go` 的 `defaultRoutingTable()` 註解）。

> **ADR-011**（Phase 2 capability name 對齊決策）：採用 Option B — 對齊 code → doc。Phase 1 開發期間 doc 與 code 曾使用不同的命名空間（doc 用 dotted name、code 用 snake_case enum），Phase 2 統一以 code enum 為權威來源。對齊過程中發現的 scope 差異（如 `narrative.rationale_translation_fallback` 的翻譯補丁概念 vs `rationale_generation` 的 rationale 生成概念）以本 ADR 記錄，Phase 2 adapter 實作時須重新定義兩者的輸入輸出契約。

**鏈長與層級說明（ADR-012）**：
- `primary` = 任務可達成率最高的模型，且優先使用已付費訂閱額度（敘事群組用 M3、程式碼群組用 kimi）。
- `backup1` = 跨家族的第一備援；敘事群組由 M3 降級到 `deepseek (deepseek-flash)`，程式碼群組由 kimi 降級到 M3。
- `backup2` 只保留給程式碼群組（`deepseek (deepseek-flash)`）；其餘群組的 `backup2` 為空字串 → 三層鏈（primary → backup1 → last resort）。
- `last resort` 因 capability 而異：翻譯可 passthrough、摘要可空字串、PRISM 可 discard（語意見上表與 §6.3a）。
- **OpenCode-Go / OpenCode-Zen 不在鏈上**：兩者為 reserved 常數，`internal/llm/clients/` 內無 client 實作（Issue #720），因此路由鏈不含 opencode 成員。

### 6.1a `max_tokens` 校準（ADR-012）

> 舊值對 reasoning 模型（M3 / `deepseek-flash`）過小：native thinking 會吃光預算，provider 回傳成功但 `output` 為空（見 §6.3a）。下表為 2026-09-11 定案值。

| Capability | 舊值 | 新值 | 理由（一行） |
|------------|------|------|--------------|
| `failure_attribution` | （無設定，走 provider 預設） | 2048 | Router 路徑的 reasoning 最低預算；rule-based fallback 仍為權威。⚠️ production `/annotate` 目前不走 Router（走 `llm_annotator.KimiClient`，tokens 由其 `Config.MaxTokens` 決定），故此值僅在 Router 路徑生效 |
| `rationale_generation` | 500 | 2048 | 翻譯輸出含 JSON 外殼；M3 / `deepseek-flash` reasoning 需要預算 |
| `regime_explanation` | 300 | 2048 | 原值對 reasoning 模型過小，會回空內容；headline 短但 thinking 需預算 |
| `sentiment_explanation` | 500 | 2048 | 同上 |
| `confidence_commentary` | 400 | 2048 | 3-4 句中文 + JSON 外殼；原值過小 |
| `performance_forensics` | 600 | 4096 | VaR / CVaR / 回撤敘事 + calibration JSON，屬長輸出 |
| `risk_surface_extraction` | 600 | 3072 | gap 描述抽取 + JSON 結構 |
| `scenario_simulation` | 600 | 4096 | 訓練結果解釋 + cohort summary 兩個欄位 |
| `strategy_summary` | 300 | 2048 | 摘要 + JSON 外殼 |
| `code_review_annotation` | 800 | 4096 | diff 審查，findings JSON 可長 |
| `prompt_lint` | 800 | 4096 | lint findings JSON |
| `contra_attribution` | — | — | 無 handler，無 `max_tokens` 可校準 |

**為何 OpenCode-Zen 不在主鏈上**：OpenCode-Zen 是 last-mile 備援，僅在 OpenCode-Go 也不可用時由 Router 自動啟動；不寫在這張表是為了避免誤導讀者把它當成同級備援。OpenCode-Zen 啟動時所有 capability 自動降級，且會發送 alert（見 §6.5 觸發條件 3）。

> **ADR-012 現況註記**：OpenCode-Go / OpenCode-Zen 目前皆為 reserved 常數（`internal/llm/clients/` 無 client 實作，Issue #720），路由鏈不含兩者；`backup2` 只有程式碼群組使用（`deepseek (deepseek-flash)`）。

### 6.2 成本與效能矩陣

| Provider | Model | Input $/1M | Output $/1M | 主要強項 | 主要弱項 |
|----------|-------|-----------|------------|----------|----------|
| Kimi | K2.7 | $0.95 | $4.00 | 純程式碼生成 | 無金融 / 敘事能力 |
| Kimi | K2.6 | $0.95 | $4.00 | 通用、K2.7 的 instruct 對應 | 仍非頂級推理 |
| MiniMax | M3 | $0.30 | $1.20 | 金融、繁中、1M context | 資料主權風險（hosted） |
| DeepSeek | ~~V4-Pro~~ | — | — | — | **已退役（2026-09-11，ADR-012）**：不再派遣；品質與 `deepseek-flash` 打平、貴約 5×、慢 5.7–10.6×、不支援圖像輸入 |
| DeepSeek | V4-Flash（canonical：`deepseek-flash` = V4.1-Flash） | $0.14 | $0.28 | 成本最低、速度快；**deepseek 預設**、全域 fallback | 舊定價；推理弱（V4.1-Flash 已改善，見 ADR-012） |
| OpenCode-Go | multi-model | 訂閱制 | 訂閱制 | 通用 failover、訂閱可控成本 | 額外 latency、模型不固定 |
| OpenCode-Zen | multi | 訂閱制 | 訂閱制 | regional fallback | 服務品質較不穩 |
| Mock | — | 0 | 0 | 測試 | 無生產價值 |

> 註：實際定價以合約為準；本框架不內嵌定價常數於 production code，所有計費由呼叫端傳入 `costPer1kTokens`（沿用 `CostReport` 既有介面，見 `observability.go:158-167`）。
>
> v2.0 新增：每筆呼叫的 `Response` 帶 `Provider` 與 `Usage`，由 metric label `capability` × `provider` 聚合後即可計算每 capability × 每 provider 的實際成本。

### 6.3 Provider 優先順序（Router 內部選擇邏輯）

Router 在收到 `Request` 後，依下列順序決定 provider：

1. **`Options.ForceProvider` 顯式指定**（測試 / sticky routing）
2. **Capability 預設 primary**（§6.1 路由表 / §3.2 決策表）
3. **健康度檢查**：
   - primary circuit breaker open → 降級到 backup1
   - backup1 circuit breaker open 或 latency > 2× baseline → 降級到 backup2（若 `backup2` 為空字串則直接到 last resort）
   - 全部鏈成員失敗 → 執行 last resort（§6.1 表；Phase 1 統一為 `mock`）
4. **Mock**：僅當 `Options.Trace == true` 或測試環境變數 `ATLAS_LLM_FORCE_MOCK=1`

> **已廢止（ADR-012）**：原步驟 2「`DataClass` 閘門 —— `DataClass == Regulated` 且候選 provider 為 MiniMax hosted 時自動降級到自架 M3 或 backup1」已隨 ADR-010 拆除。`DataClass` 僅為稽核 metadata，不再影響 provider 選擇。

### 6.3a 空輸出視為失敗（ADR-012）

Router 收到 provider「**呼叫成功但 `Output` trim 後為空**」時，一律視為該 provider **失敗**，並續試下一個鏈成員。理由：reasoning 模型（M3 / `deepseek-flash`）在 `max_tokens` 不足時，會把預算全花在 native thinking，回傳成功但 `output` 為空字串——呼叫端若只檢查 `error` 會拿到空氣，卻無從察覺。

| 行為 | 說明 |
|------|------|
| 失敗判定 | provider error，或 `Output` trim 後為空且無 `ToolCalls` |
| 續試 | 依鏈序嘗試下一個成員；`Backup2` 為空字串時跳過 |
| `ForceProvider` 例外 | 強制指定 provider 時**不套用**空輸出判定（沒有下一鏈成員可續試；該路徑供測試/sticky routing 使用） |
| `AttemptedProviders` | 只記錄**實際被呼叫**的 provider（依序，含失敗者）。這讓該欄位成為可信的 audit 依據 —— 出現在名單裡就代表該 provider 真的收到過這筆請求 |
| `llm.skipped_providers`（span 專用） | 鏈上「無法被呼叫」的成員（未註冊、或該 capability 不支援）**只記在 span**，不進 `AttemptedProviders`：鏈設定錯誤仍可見，但不會污染 audit |
| `FallbackTriggeredTotal` | 每次**實際呼叫**非 primary 鏈成員時遞增（primary 被跳過而 backup 被呼叫也算一次，因為 fallback 真的發生了；被跳過的成員本身不計數） |
| 與 `DataClass` 的關係 | 無關；`DataClass` 不再影響 provider 選擇（ADR-012） |
| 搭配條件 | 所有 capability 的 `max_tokens` 必須 ≥ reasoning 模型最低預算（見 §6.1a） |

### 6.4 模型專屬路由規則

> 不同模型有不同本性，路由器需認識這些本性才能正確選擇。

**K2.7（Kimi Code Plan）**：
- **唯一**允許用途：code-related capability（`CapabilityCodeReviewAnnotation`、`CapabilityPromptLint` 的 code path）
- 任何「敘事、歸因、翻譯、摘要、信心旁註」場景若 `Options.ForceProvider` 沒寫死，K2.7 的 `Supports` 必須回 `false`（見 §4.4 adapter 程式碼）
- 為何如此嚴格：K2.7 沒有 general instruct variant，強行用於非程式碼任務會產生幻覺
- ADR-009 為此規則的決策來源

**K2.6（Kimi Code Plan 通用變體）**：
- 與 K2.7 同一 endpoint（`api.kimi.com/coding/v1`），但 model id 切換
- 用於 K2.7 不適用的非程式碼任務；目前 v2.0 路由表未將任何 capability 的 primary 指定為 K2.6
- 保留以備 M3 不可用時降級使用（`V4-Pro` 已退役，ADR-012）

**V4-Pro（已退役，2026-09-11）**：
- **不再派遣、不再作為任何 capability 的 primary 或預設模型**（ADR-012）。實測品質與 `deepseek-flash` 打平、貴約 5×、慢 5.7–10.6×、且不支援圖像輸入。
- 名稱保留僅為 deprecated alias；新程式碼與設定一律用 canonical `deepseek-flash`。

**V4-Flash（canonical `deepseek-flash` = DeepSeek-V4.1-Flash）**：
- 現為 deepseek 的**預設與 model 名**，也是全域 fallback：敘事 / 解釋群組的 backup1（§6.1）。
- 多模態（可讀圖）與 1M context；`max_tokens` 需 ≥ reasoning 最低預算（§6.1a），否則回空輸出（§6.3a）。
- 模型名可由環境變數 `LLM_DEEPSEEK_MODEL` 覆寫（預設 `deepseek-flash`）。

**回應正規化（所有 provider client）**：
- MiniMax M3 的 CN OpenAI-compatible endpoint 把 native thinking **內嵌在 `message.content`**（`<think>…</think>` + 真正答案），已在 `internal/llm/clients` 與 `internal/llm_annotator` 的 client 剝除；未剝除會讓 JSON-first capability 解析失敗並落到 raw-string fallback（使用者看到推理文字）。
- 若 thinking 被 `max_tokens` 截斷（有 `<think>` 無 `</think>`），剝除後為空 → 依 §6.3a 視為該 provider 失敗，續試下一鏈成員。
- DeepSeek V4.1-Flash 的 thinking 走獨立的 `reasoning_content` 欄位，`content` 本來就乾淨，不需剝除（2026-09-11 實測）。
- 「成功但空輸出」現在在 provider client 層也會被視為失敗（`llm_annotator` 不再回 200 + 空註解）。

**M3**：
- 金融 / 繁中敘事首選（PRISM insight、gap description、複雜 fallback）；現為敘事 / 解釋群組 9 個 capability 的 primary（§6.1）。
- **`DataClass` 不再影響 M3 的路由**：原「`DataClass == Regulated` 時 hosted M3 必須由自架 M3 取代」的規則已隨 ADR-010 一併廢止；hosted M3 的主權顧慮改列為 residual risk，由 ADR-012 說明（不再有 provider 層閘門）。
- 若日後要落實主權控制，方向是「全供應商一致處理或 self-host」，而非僅挑 M3 一家（見 ADR-012 residual risk）。

**OpenCode-Go**：
- 通用備援；不參與 primary 競爭
- 訂閱制成本可控；缺點是模型不固定（每次 routing 結果可能不同），需在 trace 內記錄實際上游 model

**OpenCode-Zen**：
- last-mile 備援；品質保證較低
- 啟動時必須發 alert（`LLMRouterOpenCodeZenActivated`）

### 6.5 備援策略（Backup Strategy，v2.0 新章節）

> 取代 v1.0 的「Multi-provider 引入時機」段落。v2.0 起備援是預設設計，不是未來選項。

#### 觸發條件（任一成立即啟動降級鏈）

1. **Circuit breaker open**：provider 的 `BreakerThreshold`（預設 3 次連續失敗）觸發 → 標記該 provider unhealthy
2. **Latency 異常**：該 provider 的 P95 latency > baseline × 2，持續 ≥ 5 分鐘
3. **Error rate 異常**：該 provider 在 15 分鐘內的 error rate > 5%
4. **HTTP 4xx 異常**：5 分鐘內 4xx rate > 20%（可能是 API key 失效 / 配額用盡）
5. **手動開關**：`ATLAS_LLM_DISABLE_PROVIDER_<name>=1` 環境變數強制標記某 provider 不可用

#### 降級流程

```
[Capability 請求]
      │
      ▼
[檢查 primary provider 健康度]
      │
      ├── 健康 → 執行 primary，回傳 Response{Provider: "primary"}
      │
      └── 不健康 → 記錄 AttemptedProviders = ["primary"]
                    │
                    ▼
              [檢查 backup1 健康度]
                    │
                    ├── 健康 → 執行 backup1，回傳 Response{Provider: "backup1"}
                    │
                    └── 不健康 → 記錄 AttemptedProviders += ["backup1"]
                                  │
                                  ▼
                            [檢查 backup2（OpenCode-Go）健康度]
                                  │
                                  ├── 健康 → 執行 OpenCode-Go
                                  │
                                  └── 不健康 → 記錄 AttemptedProviders += ["opencode-go"]
                                                │
                                                ▼
                                          [啟動 OpenCode-Zen]
                                                │
                                                ├── 成功 → 執行（品質降級）
                                                │
                                                └── 失敗 → 執行 last resort
                                                           （passthrough / 空字串 / rule_based / discard）
```

#### 備援監控指標

- `llm_router_provider_health{provider="..."}` (gauge: 0=unhealthy, 1=healthy)
- `llm_router_fallback_triggered_total{capability="...",from_provider="...",to_provider="..."}` (counter)
- `llm_router_backup_chain_exhausted_total{capability="..."}` (counter; 任何 last resort 觸發時計數)
- `llm_router_opencode_zen_activated_total` (counter; OpenCode-Zen 啟動時計數 + 發 alert)

#### 復原流程

- 任何 provider 標記 unhealthy 後，每 30 秒由 health-check goroutine 試探一次（沿用既有 `CircuitBreaker` half-open 機制，見 `circuit_breaker.go:14-18`）
- 連續 2 次試探成功 → 標記 healthy，重新加入候選
- 試探失敗 → 延長 backoff（指數遞增，上限 5 分鐘）

#### 與 v1.0 差異

v1.0 §6.5 將 multi-provider 列為「未來選項，需 30 天資料 + 3 次 SLA 違規才考慮」。v2.0 反轉這個預設：備援是設計的一部分，所有 capability 從 day-1 就有四級鏈。代價是設定複雜度上升，但這是必要的——單一 provider 鎖定在 v1.0 已證明是脆弱設計（見 §1.3 表格第三列）。
