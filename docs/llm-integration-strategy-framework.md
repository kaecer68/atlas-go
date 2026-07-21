# LLM 整合策略框架（LLM Integration Strategy Framework）

> 文件性質：架構藍圖（architecture blueprint），定義 atlas-go 內所有 LLM 相關能力的統一演進路徑。
> 適用範圍：`internal/llm_annotator`、`internal/narrative`、`internal/spawning`、`internal/orchestrator`、所有未來新增的 LLM 消費者模組。
> 取代對象：本文件**不取代**任何現有實作；它定義一個會把現有介面包進去的更上層合約。

> **Wave 11 交叉引用** (Issue #711 + v0.0.0.21):
> - `Tool.Handler` 的 trust boundary 在 PR1 已明確化 — 「LLM validation 是 hint,handler 必須自行驗證」。建議透過 `SafeInvokeHandler` 呼叫(支援 panic recovery)。詳見 `internal/llm/provider.go` 的 `Tool.Handler` docstring。
> - `Request.Validate()` 在 PR3 新增,統一驗證 `ToolChoice`(reserved keywords 或 registered tool names)。Provider adapter 在 dispatch 前必須呼叫,trust valid input。詳見 `internal/llm/provider.go` 的 `Request.Validate()` docstring。
> - L2.3 PoC(`SemiconductorLLMAgent` + `DriverAdapter`)是本框架的**第一個 sector agent consumer**,驗證了 end-to-end flow。詳見 [`docs/specs/llm-sector-agent-spec.md`](specs/llm-sector-agent.md)。
> 維護者：core architecture
> 版本：v2.1（與 `internal/MATURITY.md` 對齊於 2026-06）
>
> **v2.1 修訂**：
> 1. 新增 §3.1a「領域上下文：台灣股市慣性特徵」——定義六大台股慣性特徵及其對 capability 輸出的評估影響。
> 2. Phase 2 新增「台灣股市情境 A/B 評測」任務——用真實台股失效歸因樣本驗證各 provider 對金融領域的適用性。
> 3. 明確邊界：台灣市場特徵是 capability 輸出評估框架，不影響 Provider 介面/路由/模型選擇。
>
> **v2.1.1 結構抽離**（2026-06）：§4.2-4.5、§6 路由策略、§8 遷移路徑、§10 決策紀錄抽離至獨立文件以降低本檔行數（1770 → 972）：
> - §4.2-4.5 → [`docs/specs/llm-interface-contract-spec.md`](specs/llm-interface-contract.md)（356 行）
> - §6 → [`docs/specs/llm-routing-spec.md`](specs/llm-routing.md)（161 行）
> - §8 → [`docs/archive/2026-06-27-llm-migration-roadmap-resolved.md`](archive/2026-06-27-llm-migration-roadmap-resolved.md)（RESOLVED，164 行）
> - §10 → [`docs/llm-adr-log.md`](llm-adr-log.md)（142 行）
>
> §4.2-4.5/6/8/10 內 cross-reference 章節仍可在原檔被引用，引用位置改為「見 `specs/llm-interface-contract.md` §X」、「見 `specs/llm-routing.md` §X」等格式。
>
> **v2.0 重大修訂**：
> 1. 補齊三主力模型（Kimi K2.6/K2.7、MiniMax-M3、DeepSeek-V4-Pro/V4-Flash）。
> 2. 修正所有 API endpoint：移除錯誤的 `api.moonshot.cn` 引用，補上正確的 `api.kimi.com/coding/v1`。
> 3. 新增「主力模型能力矩陣（Model Capability Matrix）」章節（§1a）。
> 4. 「能力-模型匹配決策」由全部指向 Kimi 改為依 capability 路由（§3.2）。
> 5. 重寫 §6 為 multi-provider 路由策略與備援機制。
> 6. 新增「資料主權與法規合規」風險（§9 風險 8）。
> 7. ADR-005 由「單一 Provider」改為「Capability-Based Multi-Provider」；新增 ADR-009（K2.7 限縮）、ADR-010（資料主權閘門）。

---

## 目錄

- [一、現狀診斷（As-Is Assessment）](#一現狀診斷as-is-assessment)
- [一a、主力模型能力矩陣（Model Capability Matrix）](#一a主力模型能力矩陣model-capability-matrix)
- [二、設計原則（Design Principles）](#二設計原則design-principles)
- [三、能力分類學（Capability Taxonomy）](#三能力分類學capability-taxonomy)
- [四、統一介面合約（Unified Interface Contract）](#四統一介面合約unified-interface-contract)
- [五、成熟度與依賴地圖（Maturity & Dependency Map）](#五成熟度與依賴地圖maturity--dependency-map)
- **六、Provider 路由策略** → [`docs/specs/llm-routing-spec.md`](specs/llm-routing.md)
- [七、資料流與審計軌跡（Data Flow & Audit Trail）](#七資料流與審計軌跡data-flow--audit-trail)
- **八、遷移路徑** → [`docs/archive/2026-06-27-llm-migration-roadmap-resolved.md`](archive/2026-06-27-llm-migration-roadmap-resolved.md)
- [九、風險與緩解（Risks & Mitigations）](#九風險與緩解risks--mitigations)
- **十、決策紀錄** → [`docs/llm-adr-log.md`](llm-adr-log.md)

---

## 一、現狀診斷（As-Is Assessment）

> **⚠️ 關鍵警示**：atlas-go 目前唯一接入的 LLM 後端為 Kimi/Moonshot（`api.moonshot.cn`），但實際生產主力模型為 kimi-k2.7（kimi.com 的 coding plan）。本文件 v2.0 將所有 Provider 定義修正為正確的 API endpoint，並補齊 MiniMax-M3、DeepSeek-V4 兩個主力模型。

atlas-go 目前**只有一條** production runtime LLM 呼叫路徑，其餘所有與「智慧輸出」沾邊的行為都是 rule-based、靜態映射、或人工排程。本章先盤點事實，再指出斷裂點。

### 1.1 事實盤點

| 項目 | 現況 | 證據位置 |
|------|------|----------|
| 唯一 runtime LLM 呼叫點 | `internal/llm_annotator/annotator.go:48-71` 的 `KimiClient`，呼叫 Kimi Code Plan chat-completions（v2.0 修正：endpoint 為 `https://api.kimi.com/coding/v1`，**非** `api.moonshot.cn`） | `internal/llm_annotator/annotator.go:48-71`、`internal/llm_annotator/annotator.go:130-171`（`NewKimiClient`） |
| 唯一 HTTP endpoint | `POST /api/strategies/{id}/annotate`，回傳自然語言失效歸因 | `internal/monitoring/api/strategies/handlers.go:261-352`（`annotate` + `buildFailureContext`） |
| Production 啟用方式 | opt-in，由 `LLM_ANNOTATOR_API_KEY` 環境變數驅動；缺金鑰回 503 | `cmd/atlas/main.go:1607-1629` |
| API Key 取得路徑 | `config.GetSecret` → `envOrKeychain`，符合 apigateway 憲法「不繞過 gateway 直接讀 os.Getenv」 | `internal/config/config.go:204-208`、`internal/llm_annotator/doc.go:11-19` |
| Annotator 介面 | 兩個 method：`Annotate(ctx, FailureContext) (string, error)` + `Name() string` | `internal/llm_annotator/doc.go:75-80` |
| Config 結構 | APIKey、BaseURL、Model、Timeout、MaxTokens、BudgetThreshold、BudgetCallback、Metrics、Breaker | `internal/llm_annotator/doc.go:99-109` |
| Circuit Breaker | 自有實作（閾值 3、復原 5 分鐘、half-open 2 探測），避免 import cycle 不共用 apigateway 版本 | `internal/llm_annotator/circuit_breaker.go:1-161`、同檔 `:14-18`、與 `internal/apigateway/circuitbreaker.go:9-16` 對比 |
| Observability | `MetricsRecorder` 介面（Counter/Gauge 兩 method）、`CostReport` 結構、`AnnotationRecord` 1000 筆 ring buffer | `internal/llm_annotator/observability.go:21-24`、`:158-167`、`:209-216`、`:233` |
| 持久化 | `AnnotationStore` 介面、JSONL 預設 50MB/檔、保留 3 個 rotated copy | `internal/llm_annotator/persistence.go:12-15`、`:30-33` |
| Prometheus alerts | 5 個 alert group：fast/medium burn rate、circuit breaker、category spike、traffic cessation | `monitoring/rules/llm_annotator_alerts.yml:1-144` |
| Prometheus recording | 9 條 recording rule（5m/30m/1h/6h/24h 5 個視窗 × success/cache/error/token） | `monitoring/rules/llm_annotator_recording.yml:1-63` |
| 套件成熟度 | X (Experimental)，文件明令「不可被 stable/evolving 模組依賴」 | `internal/MATURITY.md:87` |
| 外部 LLM SDK | `go.sum` 無 openai/anthropic/tiktoken/sashabaranov；frontend 無 LLM import；Python fubon-proxy 也無 LLM | codebase 全域 grep 驗證 |

> **v2.0 新增**：`KimiClient` 的 `BaseURL` 預設值在 `annotator.go:48-71` 內部需從 `https://api.moonshot.cn/v1` 改為 `https://api.kimi.com/coding/v1`。`Moonshot Platform`（企業級 API）與 `Kimi Code Plan`（訂閱制 API）是兩套獨立系統，API key 不可互通。詳見 §1a 對 K2.7 的說明。

### 1.2 已被明文標記的擴張點（Telegraphed Expansion Points）

下列位置在程式碼註解中**明確點出**未來會引入 LLM，這是本框架最優先服務的對象：

1. **`internal/narrative/rationale_corpus.go:17-19`**：
   > 「This file MUST NOT depend on an LLM, a translation API, or any external network resource. The corpus is a pure Go map. **Future waves may add a fallback LLM translator for unmatched strings**…」

   目前實作是 32 筆靜態英文→繁中對映（`reasonCorpus`，同檔 `:48-97`），呼叫點在 `TranslateReason`（`:149-210`）。當 corpus miss 時走 step 6 passthrough（`:208-209`），把未匹配的英文原封不動回傳——這正是「fallback LLM translator」的明確插入點。v2.0 將此 fallback 路由到 DeepSeek V4-Flash（最便宜的模型，見 §3.2 決策表）。

2. **`docs/ai-agent-architecture.md:1-213`**：5 層代理階層 + 17 specialists + Agent Spawning System + PRISM Training + Reflexivity Engine + MiroFish Swarm。

   對應程式碼：
   - `internal/spawning/agent_factory.go:33-78`（`CreateAgentForGap`：gap → AgentSpec + prompt content）
   - `internal/spawning/gap_detector.go:17-40`（`KnowledgeGap` + 6 種 `GapType`）
   - `internal/spawning/spawning_manager.go:14-46`（`SpawningManager` + `SpawningConfig`）
   - `internal/orchestrator/prism_executor.go:14-27`（`PRISMTrainingExecutor` 跑純 replay backtest）

   **注意**：`configs/agents.json` 內 17 個 agent 的 `promptFile` 指向 `prompts/agents/*.md`（例如 `prompts/agents/semiconductor_desk.md:1-10`），這些 prompt 是**開發期 AI 編碼助理的人格描述**，不是 production runtime 呼叫 LLM 的入口——這點必須在遷移前先釐清，避免把「agent prompt」誤讀成「production LLM 呼叫端點」。

### 1.3 結構性斷裂（Structural Gaps）

把現有拼圖鋪開後，可以看見六條主要裂縫：

| 斷裂 | 現象 | 後果 |
|------|------|------|
| **介面過窄** | `Annotator` 只有「失敗歸因」一種能力，`Name()` 只回傳 provider 字串 | narrative rationale fallback、agent prompt 評分、PRISM cohort 摘要等場景全部無法共用，必須各自重新接線 |
| **無 Capability 路由器** | 沒有 `Capability → Provider` 的映照表 | 每個新場景都要重複決定「用哪個 model、要不要走 cache、要什麼 circuit breaker 閾值」 |
| **Provider 單一鎖定** | `KimiClient.Name()` hardcode `"kimi"`（`annotator.go:250`）；config 無 provider id 欄位 | 換 provider 要動 client 結構；multi-provider fallback 沒有設計位置 |
| **審計軌跡割裂** | Annotate 有 `AnnotationRecord`（`observability.go:209-216`）+ JSONL store；但 rationale 翻譯、agent prompt 評估完全沒有對等軌跡 | 任何想跨場景做 cost / drift 分析的人都得從零開始拼 |
| **依賴方向倒錯** | `internal/MATURITY.md:87` 禁止 S/E 模組依賴 `llm_annotator`（X 級）；但 monitoring API handler (`strategies/handlers.go:12`) 已是 S 級，卻直接 import llm_annotator——這條邊界目前靠「handler 不在 hot path」「503 fallback」維繫，不是靠結構保證 | 一旦有人在 `cmd/atlas/main.go` 開 hot path 直接 call KimiClient，就會打破 MATURITY 規定但 CI 不會擋 |
| **模型選用未對齊** | v1.0 預設所有 capability 都走 Kimi；沒有依「任務特性」分配模型 | 例如 K2.7 是純程式碼模型，被誤用於財務敘事會產生幻覺 |

### 1.4 目前架構圖（Before）

```
+--------------------------------------------------+
|                 monitoring API (S)               |
|   /api/strategies/{id}/annotate                  |
|   strategies/handlers.go:261-352                |
+-------------------------+------------------------+
                          | 唯一一條線
                          v
+--------------------------------------------------+
|             llm_annotator (X)                    |
|   Annotator 介面  doc.go:75-80                  |
|   KimiClient 實作  annotator.go:48-71           |
|   CircuitBreaker   circuit_breaker.go            |
|   Observability    observability.go              |
|   Persistence      persistence.go                |
|   Health handler   health.go                     |
+--------------------------------------------------+
                          |
                          v
              POST https://api.kimi.com/coding/v1/chat/completions
              (v2.0 修正：原 v1.0 寫的 api.moonshot.cn 為錯誤 endpoint)

   旁路（無 LLM）：
     narrative/rationale_corpus.go       ← 純 map，無網路
     spawning/agent_factory.go           ← 純 string 生成
     orchestrator/prism_executor.go      ← 純 replay backtest
     configs/agents.json + prompts/agents/*.md   ← dev-time 編碼助理 prompt，非 runtime
```

**孤立模組**：narrative / spawning / orchestrator 目前**完全沒有**通往 LLM 的路徑——這與「atlas-go is an AI-driven multi-agent system」的設計願景（`docs/ai-agent-architecture.md:1-5`）之間存在顯著張力。

---

## 一a、主力模型能力矩陣（Model Capability Matrix）

> 本章定義 atlas-go v2.0 起採用的三個主力模型 + 兩個備援通道。每一個能力（capability）的路由決策都以此矩陣為依據；詳細路由表見 §3.2 與 §6。

### 一a.1 模型清單與角色定位

| 模型 | 廠商 | 角色 | 適用場景 |
|------|------|------|----------|
| **Kimi K2.7** | Moonshot AI | 純程式碼主力 | 程式碼生成、PR review、prompt lint、code review 註解 |
| **Kimi K2.6** | Moonshot AI | 通用主力（K2.7 的 instruct 對應） | 一般任務；K2.7 不適用的非程式碼場景 |
| **MiniMax M3** | MiniMax | 金融主力 + 自架備援 | PRISM cohort insight、gap description、繁中金融敘事 |
| **DeepSeek V4-Pro** | DeepSeek | 推理主力 | 失效歸因、信心校準旁註、策略摘要 |
| **DeepSeek V4-Flash** | DeepSeek | 成本優主力 | rationale 翻譯、敘事 headline、prompt lint dev path |
| **OpenCode-Go** | 第三方 | 主要備援（multi-model 訂閱） | 主力 provider 不可用時的 generic failover |
| **OpenCode-Zen** | 第三方 | 次要備援（regional fallback） | OpenCode-Go 也不可用時的 last-mile 通道 |
| **MockAnnotator** | 內部 | 測試樁 | 測試 + `ATLAS_LLM_FORCE_MOCK=1` |

### 一a.2 Kimi K2.7（純程式碼模型）

| 欄位 | 值 |
|------|----|
| **endpoint** | `https://api.kimi.com/coding/v1` |
| **endpoint 類型** | OpenAI-compatible chat completions |
| **模型類型** | 純程式碼模型（無通用 Instruct 版本） |
| **context window** | 256K |
| **訂閱定價** | $15–$159 / 月（依用量級距） |
| **API 定價** | input $0.95 / 1M tokens、output $4.00 / 1M tokens |
| **授權** | MIT 開源 |
| **繁中能力** | C-Eval 92.5%、CMMLU 90.9% |
| **thinking mode** | 強制 ON，**無法關閉** |

**使用範圍（白名單）**：
- 程式碼生成、code review 註解
- prompt lint（CI 階段掃 `prompts/agents/*.md`）
- 與 PRISM code-related 路徑（若未來引入）

**禁止使用**：
- 任何 financial / strategy / 敘事 / 翻譯任務
- 任何信心校準旁註、信心度量化
- 任何對外可見的繁中敘事輸出

**已知限制**：
- 256K context 遠小於 Claude 1M、DeepSeek V4 1M、MiniMax M3 1M
- thinking mode 強制 ON，會增加 token 成本與 latency
- 沒有 general instruct variant；強行用於非程式碼任務風險高

**與 Moonshot Platform 的差異**：Moonshot 另有企業級 Platform（endpoint `api.moonshot.cn`），但**那不是** atlas-go 使用的方案。Kimi Code Plan 是 Moonshot 為 coding subscription 開的獨立通道，API key 不互通。

### 一a.3 MiniMax M3（金融主力 + 自架備援）

| 欄位 | 值 |
|------|----|
| **endpoint（hosted）** | `https://api.minimax.io/v1`（OpenAI-compatible） |
| **endpoint（self-hosted）** | 由內部推論叢集對外，OpenAI-compatible 介面 |
| **模型類型** | 通用多模態 MoE |
| **架構** | 428B 總參數 / 23B 啟用參數 |
| **context window** | 1M（保證 512K 滿血） |
| **訂閱定價** | $20–$120 / 月（Token Plan 級距） |
| **API 定價** | input $0.30 / 1M tokens、output $1.20 / 1M tokens（**永久 50% 折扣**） |
| **開源權重** | minimax-community 授權；MXFP8 量化後 440GB（可自架） |
| **繁中金融** | SegmentFault 評測 75.8%、BankerToolBench 76.12、SpreadsheetBench 89.35 |
| **價格優勢** | 約為 Claude Opus 4.7 的 1/18 |

**使用範圍**：
- PRISM cohort insight（金融領域）
- gap description enrichment（金融敘事）
- 需要繁中金融表達的所有 capability
- **自架 M3** 可處理受規範金融資料（見 §9 風險 8）

**已知限制**：
- 長程 agent 任務會出現疲勞（long-range agent fatigue）
- SciCode 評測從 47% 退步到 45%
- AA-Omniscience 高拒答率 30.9%
- Hosted API 受中國 2017 國家安全法管轄（見 §9 風險 8）

**自架選項**：權重採 minimax-community 授權釋出，MXFP8 量化 440GB 適合內部 GPU 叢集自架；自架後可解除資料主權顧慮（詳見 §9 風險 8 緩解 1）。

### 一a.4 DeepSeek V4-Pro / V4-Flash（推理主力 + 成本優主力）

| 欄位 | V4-Pro | V4-Flash |
|------|--------|----------|
| **endpoint** | `https://api.deepseek.com` | `https://api.deepseek.com` |
| **endpoint 相容性** | OpenAI + Anthropic 雙相容 | OpenAI + Anthropic 雙相容 |
| **架構** | 1.6T 總 / 49B 啟用 | 284B 總 / 13B 啟用 |
| **context window** | 1M | 1M |
| **API 定價** | 依市場報價（接近旗艦水準） | input $0.14 / 1M、output $0.28 / 1M（**極低**） |
| **授權** | MIT 開源 | MIT 開源 |
| **程式碼** | LiveCodeBench 93.5%（全部模型排名第一） | LiveCodeBench 略低於 V4-Pro |
| **推理** | IMOAnswerBench 89.8% | 略低於 V4-Pro |
| **繁中** | Chinese-SimpleQA 84.4%、C-Eval 93.1% | 略低於 V4-Pro |

**V4-Pro 使用範圍**：
- 失效歸因（Failure Attribution）：高推理 + 高繁中
- 信心校準旁註：風險/法規鄰接，需最強推理
- 策略摘要：複雜摘要需強推理

**V4-Flash 使用範圍**：
- rationale 翻譯：簡單翻譯任務 + 成本敏感
- 敘事 headline：簡單一行濃縮
- prompt lint（dev path）：成本敏感、量大

**NIST 獨立評估**：V4 與 GPT-5 約有 8 個月 capability gap（接近 GPT-5 級，距 GPT-5.4 / Opus 4.6 仍有距離）。

**已知限制**：
- HLE（Huge Legal Eval）37.7%，落後 Gemini 44.4%
- 抽象推理與資安場景明顯落後旗艦
- 舊 `deepseek-chat` / `deepseek-reasoner` 於 **2026/07/24** 棄用；新實作必須用 `deepseek-v4-pro` / `deepseek-v4-flash`

### 一a.5 備援通道

#### OpenCode-Go（主要備援）

- **角色**：當三個主力 provider 全部不可用時的 generic failover
- **定位**：multi-model 訂閱服務；單一 endpoint 可路由到多個上游模型
- **使用時機**：circuit breaker open on primary、latency > 2x baseline 超過 5 分鐘、error rate > 5% 超過 15 分鐘
- **缺點**：latency 較高（多一層 routing）、價格為訂閱制而非 pay-as-you-go

#### OpenCode-Zen（次要備援）

- **角色**：regional / datacenter failover
- **定位**：獨立於 OpenCode-Go 的備援平台；地理分散
- **使用時機**：OpenCode-Go 也不可用時的 last-mile 通道
- **缺點**：服務品質不一致；僅作為「別全死」的最後保險

### 一a.6 矩陣總覽（評分比較）

> 評分以最低（最差）到最高（最佳）表示。分數依 §一a.2 至 §一a.4 的客觀指標。

| 能力 | K2.7 | K2.6 | M3 | V4-Pro | V4-Flash |
|------|------|------|----|--------|----------|
| 純程式碼生成 | 最高 | 高 | 中 | 最高 | 高 |
| 通用繁中生成 | 低 | 中 | 高 | 最高 | 高 |
| 繁中金融表達 | 低 | 低 | 最高 | 高 | 中 |
| 複雜推理 | 低 | 中 | 高 | 最高 | 中 |
| 長 context（≥ 256K） | 中 | 中 | 最高 | 最高 | 最高 |
| 成本效益 | 低 | 低 | 高 | 中 | 最高 |
| 自架可行性 | 中 | 中 | 最高 | 高 | 高 |
| 資料主權風險 | 最低 | 最低 | 低（hosted）/ 最高（self-host） | 中 | 中 |
| 抽象推理 / 資安 | 最低 | 低 | 中 | 中 | 低 |

> K2.7 在「通用繁中生成」「複雜推理」的低分反映其為純程式碼模型的本質，並非訓練不足。**這正是 §3.2 與 ADR-009 限縮 K2.7 用途的依據**。

---

## 二、設計原則（Design Principles）

下列原則**全部同時生效**，任何未來的 LLM 整合決策都必須能對應到其中至少一條。

### 原則 1：介面包裝（Wrap, don't replace）

> 新的統一介面**只包進去**現有的 `Annotator`，不得刪除或改動既有介面簽章。

理由：
- 既有 `Annotator` 已被 `monitoring/api/strategies/handlers.go:20`、`handlers.go:40`、`handlers.go:302` 廣泛依賴。
- `Annotate(ctx, FailureContext) (string, error)` 已是 `cmd/atlas/main.go:1613-1625` 的 production wiring 入口。
- 改介面 = 改 main.go 啟動序列 = 改 production 部署；風險不對稱。

### 原則 2：Capability > Model（以能力為中心，不是以模型為中心）

呼叫端不該問「我該用哪個 model」，而該問「我需要什麼 capability（翻譯、摘要、評分、生成…）」。Provider 路由在內部決定。

理由：避免 narrative 模組 import `kimi` 字串、避免 spawning 模組寫死 model id。v2.0 起，Provider 常數由原本三個（Kimi、OpenAI 預留、Mock）擴張為七個，**但呼叫端仍只認 capability ID**。

### 原則 3：X 級模組不可被 S/E 級依賴（邊界守恆）

任何新增的 LLM 呼叫點若需在 stable 路徑上使用（例如 narrative 翻譯 fallback），必須透過**事件 / 介面 / 注入**三種手段之一跨越邊界，**不得直接 import**。

理由：`internal/MATURITY.md:75-78` 明確禁止。

### 原則 4：Fallback First（任何 LLM 輸出都有非 LLM 對照組）

每個新場景在引入 LLM 之前，必須先有可用的靜態 / rule-based 結果。LLM 只能「增強」，不能「創造」。

理由：這是現有 Annotator 的設計精神（`internal/llm_annotator/doc.go:21-23`：「callers MUST treat this as a fallback signal: the registry's rule_based attribution remains authoritative」）。

### 原則 5：可審計、可回放、可否決

每次 LLM 呼叫都必須留下**三項 trail**：
1. 結構化 metric（counter / gauge / label）
2. 結構化 record（input hash、output、tokens、provider、capability）
3. 適用於 dispute 的 line-by-line trace（輸出對應回 prompt section、reasoning chain）

理由：監控規則已存在（`monitoring/rules/llm_annotator_alerts.yml`、`monitoring/rules/llm_annotator_recording.yml`），新場景必須複用同樣的格式。

### 原則 6：成本與速率是 first-class concern

每個 capability 必須有：estimated tokens、per-1k price、budget threshold、rate limit。不得有「無限制 LLM 呼叫」的設計。

理由：`KimiClient.NewKimiClient` 已有完整預設（`annotator.go:130-171`），包含 rate limiter `rate.NewLimiter(rate.Every(time.Second), 4)`（`:162`）、circuit breaker、budget callback。新場景必須繼承這套。v2.0 起，每個 capability 還需在 metric 裡標記實際收費的 provider（`capability` 與 `provider` 兩個 label），以便做 cross-provider 成本對齊。

### 原則 7：觀望式擴張（Observe before extending）

> 在添加新 capability 之前，先用 Observability 資料驗證「現有能力是否已飽和 / 是否值得擴張」。

理由：`llm_annotator` 的 `RecentAnnotations(n)`（`observability.go:221-231`）與 `AnnotationRecord` ring buffer（cap 1000）已經能看出調用熱區；新能力前先看這份資料。

### 原則 8（v2.0 新增）：模型適配（Model-Capability Match）

> **每個 capability 必須綁定與其任務特性相符的模型；不得將純程式碼模型用於敘事任務，亦不得將推理弱模型用於風險/法規旁註。**

理由：K2.7 是純程式碼模型，無 general instruct 變體。將其用於 financial / strategy / 翻譯會產生幻覺。詳細對應見 §3.2 決策表與 ADR-009。

### 原則 9（v2.0 新增）：資料主權分級（Data Sovereignty Gate）

> **任何含有受規範金融資料的 capability，在路由到中國境內 hosted provider 之前，必須先通過資料分類層的閘門。**

理由：MiniMax M3 hosted API 受中國 2017 國家安全法管轄。詳見 §9 風險 8 與 ADR-010。

---

## 三、能力分類學（Capability Taxonomy）

> 表頭為必填欄位：**能力名稱 | 觸發時機 | 目前狀態 | 路由優先順序 | 消費者模組 | 優先級 | 成熟度目標**
> v2.0 起「目標 Provider」改為**路由優先順序**（primary + backup1 + backup2 + last resort），不再是單一 provider。

| 能力名稱 | 觸發時機 | 目前狀態 | 路由優先順序（primary → last） | 消費者模組 | 優先級 | 成熟度目標 |
|----------|----------|----------|-------------------------------|------------|--------|------------|
| **Failure Attribution**（策略失效歸因） | HTTP `POST /api/strategies/{id}/annotate` 由人工觸發 | 已上線（X 級，opt-in via `LLM_ANNOTATOR_API_KEY`） | DeepSeek V4-Pro → MiniMax M3 → OpenCode-Go → rule_based fallback | `internal/monitoring/api/strategies/handlers.go:261-352` → `llm_annotator.Annotator` | **P0** | X → E |
| **Rationale Translation Fallback**（rationale 翻譯補丁） | corpus miss 後啟動（見 `rationale_corpus.go:160-163`） | **未實作** | DeepSeek V4-Flash → MiniMax M3 → OpenCode-Go → passthrough 原字 | `internal/monitoring/api/pipeline/handlers.go`（非 `rationale_corpus.go`） | **P1** | X → E |
| **Strategy Frame Summarization**（StrategyFrame 摘要） | Operator dashboard 點「summarize」 | **未實作** | DeepSeek V4-Pro → MiniMax M3 → OpenCode-Go → 空字串 | `internal/monitoring/api/strategies`（新增 `GET /api/strategies/{id}/summary`） | P2 | X |
| **Agent Prompt Lint**（prompt 檔靜態審查） | CI 階段掃 `prompts/agents/*.md` | **未實作** | DeepSeek V4-Flash（dev）→ Kimi K2.7（code path） → OpenCode-Go → pass（CI 不擋） | 新增 `cmd/lint-prompts`（U 級 utility） | P3 | U |
| **PRISM Cohort Insight**（PRISM cohort 結案摘要） | `prism_executor.go:30-103` 結束時 | **未實作** | MiniMax M3 → DeepSeek V4-Pro → OpenCode-Go → discard | `internal/orchestrator/prism_executor.go`（訓練結束 hook） | P2 | X |
| **Gap Description Enrichment**（知識缺口描述增強） | `gap_detector.go:17-28` 偵測到 gap | **未實作** | MiniMax M3 → DeepSeek V4-Pro → OpenCode-Go → passthrough 原描述 | `internal/spawning/gap_detector.go`、`agent_factory.go:33-78` | P2 | X |
| **Narrative Event Headline**（敘事事件標題） | narrative ingestor 偵測新事件 | **未實作** | DeepSeek V4-Flash → MiniMax M3 → OpenCode-Go → passthrough | `internal/narrative` | P3 | X |
| **Confidence Calibration Commentary**（信心校準旁註） | `RiskManager` 跑完 VaR 後 | **未實作** | DeepSeek V4-Pro → MiniMax M3 → OpenCode-Go → passthrough | `internal/risk` | P3 | X |
| **Code Review Annotation**（code review 註解，v2.0 新增） | CI 階段掃描 PR diff | **未實作** | Kimi K2.7 → DeepSeek V4-Flash → OpenCode-Go → 無註解 | 新增 `cmd/lint-pr`（U 級 utility） | P3 | U |
| **Mock Annotator**（測試樁） | 測試任何 LLM 消費者時注入 | **已上線** | `MockAnnotator`（`internal/llm_annotator/annotator.go:23-46`） | 所有使用 `Annotator` 介面的測試 | P0 | S（永遠穩定） |

> **v2.0 變更說明**：v1.0 的「目標 Provider」欄位全部指向 Kimi；v2.0 改為四級路由鏈，呼叫端只認 capability，路由細節由 `internal/llm` Router 處理（見 §6.1）。

### 3.1 為何這十個能力是「全部」

判定標準（任一條符合即收錄）：
1. 程式碼註解明文提到 LLM 擴張點（rationale_corpus.go、ai-agent-architecture.md）。
2. 已存在「為何還沒有 LLM」的設計張力（例如 PRISM 訓練結果缺 summary）。
3. 已有呼叫介面但無 LLM（例如 Failure Attribution 已實作、可複用同一介面到其他 capability）。
4. **v2.0 新增**：程式碼生成 / review 場景明確需要 LLM，但必須隔離於敘事/金融路徑（Code Review Annotation、Agent Prompt Lint 的 code path）。

不在表內的（例如翻譯以外的 i18n、語音轉文字）目前**沒有消費者**，不在本框架承諾範圍內。新增 capability 必須先證明有呼叫端。

### 3.1a 領域上下文：台灣股市慣性特徵（v2.1 新增）

atlas-go 的核心應用場景為**台灣股票市場的智慧策略與智慧推薦系統**。台灣股市具有以下結構性慣性特徵，這些特徵構成 LLM capability 輸出的**評估背景**（非輸入參數），並影響 capability 的「正確性」校驗基準：

| 慣性特徵 | 對 LLM capability 的影響 | 關聯 capability |
|----------|--------------------------|----------------|
| **美元依賴**（USD dependency） | 策略失效歸因時，宏觀成因若忽略美元/台幣匯率波動則為不完整歸因 | Failure Attribution |
| **美股連動**（US market correlation） | 前一交易日美股走勢是台股開盤的主要方向訊號；LLM 產生的策略摘要或旁註若未參照美股收盤將顯著降低可信度 | Strategy Frame Summary、Confidence Commentary |
| **外匯操控慣性**（FX intervention） | 央行尾盤干預造成技術指標失真；LLM 若僅以價格為基礎做歸因會誤判支撐/壓力位 | Failure Attribution |
| **外貿過度依賴**（trade over-dependence） | 出口數據與電子產業景氣為台股核心驅動力；PRISM cohort insight 若未考量出口連動則洞見價值下降 | PRISM Cohort Insight |
| **單一產業集中**（single-sector concentration） | 半導體 / 電子權重過高（約 50% 權值）；策略失效常因產業輪動而非個股基本面；gap description 需能區分「個股失效」與「產業失效」 | Gap Description Enrichment |
| **淺碟市場效應**（shallow market） | 外資進出對指數影響放大；策略摘要需區分「結構性失效」與「資金面失效」 | Strategy Frame Summary |

> **重要邊界說明**：上述特徵是 **capability 輸出的評估框架（evaluation rubric）**，而非 Router 或 Provider 的設計輸入。Provider 介面、路由策略、模型選擇與市場特徵無關——無論分析台股或美股、無論考量產業集中度與否，Router 的運作邏輯完全相同。這些特徵的實際作用點是：**(a) prompt template 設計時納入情境變數**（如 macro snapshot 含美元指數與美股收盤）、**(b) Eval 策略的金標準測試集必須納入台股情境**（見 §8 Phase 2 新增任務）、**(c) 人工 review 時作為輸出品質的判斷依據**。

> 本節為 v2.1 新增，不改變任何既有 capability 定義、路由表、或介面合約。

### 3.2 能力-模型匹配決策（Capability-Model Matching）

> 本節是 §3 表格的「為什麼」展開。每個 capability 的 primary 選擇都有**對應 §1a 矩陣的評分依據**，不是任意指定。

| Capability | Primary | 評分依據（§1a） | 拒絕的替代 |
|------------|---------|------------------|------------|
| **Failure Attribution** | DeepSeek V4-Pro | IMOAnswerBench 89.8% + Chinese-SimpleQA 84.4% + 強推理；NIST 評為接近 GPT-5 級 | K2.7（純程式碼）✗、K2.6（無 K2.7 強）✗、M3（推理略弱）△、V4-Flash（推理弱）△ |
| **Rationale Translation** | DeepSeek V4-Flash | 簡單翻譯 + 成本敏感；V4-Flash 價格 $0.14/$0.28 最低；Chinese-SimpleQA 84.4% 足夠 | K2.7（無繁中能力）✗、V4-Pro（過度）△、M3（價格約 2×）△ |
| **Strategy Frame Summary** | DeepSeek V4-Pro | 複雜摘要需強推理；多條件收斂 | K2.7 ✗、V4-Flash △、M3（仍可，但推理略弱）△ |
| **Agent Prompt Lint** | DeepSeek V4-Flash | dev-only + 成本敏感 + 量可能大 | K2.7（雖是 code，但無 prompt lint 專項訓練）△、V4-Pro（過度）✗ |
| **PRISM Cohort Insight** | MiniMax M3 | 金融領域 + 繁中表達需求；BankerToolBench 76.12 強 | V4-Pro（金融表達略弱於 M3）△、K2.7 ✗ |
| **Gap Description** | MiniMax M3 | 同上：金融上下文 + 繁中敘事 | 同上 |
| **Narrative Event Headline** | DeepSeek V4-Flash | 一行 headline；簡單任務 + 成本敏感 | 同 Rationale Translation |
| **Confidence Commentary** | DeepSeek V4-Pro | 風險/法規鄰接，需最強推理；任何幻覺都可能誤導 operator | K2.7 ✗、V4-Flash（推理弱）✗、M3（推理中等）△ |
| **Code Review Annotation** | Kimi K2.7 | 純程式碼任務；K2.7 為此而設計；LiveCodeBench 競爭力 | V4-Pro（可，但成本 5×）△、V4-Flash（可，但 K2.7 為此而生）△ |
| **Mock** | MockAnnotator | 測試需求 | — |

**核心規則重申**：
- **K2.7 限縮於 code-related capability**（Code Review Annotation、Agent Prompt Lint 的 code path）。**禁止** 用於 Failure Attribution、Translation、Summary、Headline、Commentary、PRISM Insight、Gap Description。詳見 ADR-009。
- **DeepSeek V4-Pro 為複雜推理首選**；適用於歸因、旁註、複雜摘要。
- **DeepSeek V4-Flash 為成本優首選**；適用於翻譯、headline、dev path。
- **MiniMax M3 為金融/繁中敘事首選**；當資料含受規範金融欄位時應走自架 M3。詳見 ADR-010。
- **OpenCode-Go 為通用備援**；所有 capability 的 backup2 都是它。
- **OpenCode-Zen 為 last-mile 備援**；當 OpenCode-Go 也不可用時啟動，但品質保證較低。
- **Last resort 因 capability 而異**：見 §3 表格「last」欄位。

---

## 四、統一介面合約（Unified Interface Contract）

> **核心想法**：把現有 `Annotator`（`internal/llm_annotator/doc.go:75-80`）當成「Failure Attribution 這個 capability 的一個實作」，外面再包一層以 capability 為索引的路由器。v2.0 起，這個路由器支援多 provider，並依 §3.2 決策表路由。

### 4.1 概念模型

```
Capability（能力，例如 "strategy.failure_attribution"）
   ↓ 由 capability ID 決定 primary
Provider（提供者，例如 "deepseek" / "minimax" / "kimi" / "opencode-go"）
   ↓ 由 provider 決定
Concrete Client（DeepSeekV4Client / MiniMaxM3Client / KimiClient / OpenCodeGoClient / MockClient…）
```

呼叫端永遠只持有**介面**，從不持有具體 client。

### 4.2 介面定義（提案）

> **📦 已抽離**：本節完整內容（含 Go 介面程式碼）移至 [`docs/specs/llm-interface-contract-spec.md`](specs/llm-interface-contract.md) §4.2。

**核心概念速查**：`ProviderImpl`（4 個 method：`Name` / `Supports` / `Call` / `Health`）、`Router`（`Call` + `Health`）、`Capability` / `DataClass` / `Provider` 三類字串常數、`Request` / `Response` 結構。
### 4.6 架構對比（Before / After）

#### Before（孤立，v1.0 狀態）

```
+-------------------+        +-------------------+
|  strategies       |        |  narrative        |
|  handlers.go:302  |        |  rationale_corpus |
+--------+----------+        +---------+---------+
         |                            |
         | (唯一一條線)                | (無 LLM)
         v                            v
+-------------------+        +-------------------+
|  llm_annotator    |        |  (靜態 map)       |
|  KimiClient       |        +-------------------+
+--------+----------+
         |
         v
    POST https://api.kimi.com/coding/v1/chat/completions
    (v2.0 修正：v1.0 寫的 moonshot 為錯誤 endpoint)


+-------------------+        +-------------------+
|  spawning         |        |  orchestrator     |
|  gap_detector     |        |  prism_executor   |
|  agent_factory    |        |  (純統計 / 回測)   |
+--------+----------+        +---------+---------+
         |                            |
         | (無 LLM)                    | (無 LLM)
         v                            v
   string template             scorecard JSON
```

#### After（Multi-Provider Hub-and-Spoke，v2.0 目標）

```
              ┌─────────────────────────────┐
              │   monitoring / pipeline /   │
              │   spawning / orchestrator   │
              │   / risk / narrative        │
              │   (呼叫端，全部 S/E)         │
              └──────────────┬──────────────┘
                             │ Router 介面
                             │ (X 級，nullable 注入)
                             v
              ┌─────────────────────────────┐
              │   llm.Router (Hub)          │
              │   Capability → RoutingChain │
              │   (四級：primary/back1/      │
              │    back2/last)              │
              │                             │
              │   Policies:                 │
              │     - per-cap cost cap      │
              │     - per-cap rate          │
              │     - per-cap timeout       │
              │     - DataClass 閘門        │
              │                             │
              │   內部仍透過 adapter        │
              │   包進既有 Annotator        │
              └──────────────┬──────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
  │ DeepSeek V4  │    │ MiniMax M3   │    │ Kimi K2.6/7  │
  │ (reasoning)  │    │ (finance /   │    │ (code tasks) │
  │              │    │  Chinese)    │    │              │
  │ V4-Pro:      │    │ hosted +     │    │ K2.6:        │
  │  complex     │    │ self-host    │    │  general     │
  │ V4-Flash:    │    │ 選項         │    │ K2.7:        │
  │  cost-opt    │    │              │    │  code only   │
  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘
         │                   │                    │
         ▼                   ▼                    ▼
   api.deepseek.com   api.minimax.io       api.kimi.com
                       (hosted) /             /coding/v1
                       self-host

        ┌────────────────────┴────────────────────┐
        │                                         │
        ▼                                         ▼
  ┌──────────────┐                        ┌──────────────┐
  │ OpenCode-Go  │                        │ OpenCode-Zen │
  │ (主要備援)   │                        │ (次要備援)   │
  │ multi-model  │                        │ regional     │
  │ 訂閱         │                        │ fallback     │
  └──────────────┘                        └──────────────┘

   既有 rule_based / corpus / backtest 路徑
   （fallback 永遠存在，與 v1.0 一致）
```

關鍵差異（v1.0 → v2.0）：
- **Provider 從 1 個變 5 個**（Kimi + M3 + DeepSeek V4-Pro/V4-Flash + 兩個備援），依 capability 路由。
- **Kimi 內部分 K2.6 / K2.7**：K2.6 為 general，K2.7 為 code only。
- **新增備援鏈**：每個 capability 有四級 fallback 鏈（primary → backup1 → backup2 → last resort）。
- **新增資料主權閘門**：`DataClass` 欄位 + Router 內的閘門邏輯。
- **既有 Annotator 仍為基礎**：`llm_annotator.KimiClient` 仍存在，新介面以 adapter 包裹；不破壞既有投資。
- **Router 是新的 X 級模組**（`internal/llm`，與 `llm_annotator` 同層級，遵守 MATURITY 規則）。
- **呼叫端不直接 import** llm_annotator；它們 import `internal/llm`（同為 X 級，但 Router 介面是契約，可透過介面注入給 S 級模組——這是 MATURITY 規則允許的標準做法）。

---

## 五、成熟度與依賴地圖（Maturity & Dependency Map）

### 5.1 現況矩陣（引用 `internal/MATURITY.md`）

| 模組 | 目前 Tier | 與 LLM 關係 | 引用 |
|------|----------|------------|------|
| `llm_annotator` | **X** | 唯一 LLM 實作；`Maturity: experimental`（`doc.go:24`） | `internal/MATURITY.md:87` |
| `internal/llm`（v2.0 新增） | **X**（建立時即為 X） | Router 層；介面包裝 | — |
| `monitoring/api/strategies` | S | import llm_annotator（`handlers.go:12`） | `internal/MATURITY.md:36` |
| `narrative` | S | 無 LLM import；corpus 為純 Go map | `internal/MATURITY.md:38` |
| `spawning` | E | 無 LLM import | `internal/MATURITY.md:66` |
| `orchestrator` | S | 無 LLM import | `internal/MATURITY.md:39` |
| `risk` | S | 無 LLM import | `internal/MATURITY.md:42` |

### 5.2 違規點分析

`monitoring/api/strategies`（S）import `llm_annotator`（X）——這違反 `internal/MATURITY.md:75-78` 的明文規則：「X 級 experimental 模組，**不應被 stable/evolving 模組依賴**」。

為何目前能跑：
1. 該 import 僅用於 handler 結構性持有（`handlers.go:20` 的 `annotator llm_annotator.Annotator` 欄位）。
2. 透過 `SetAnnotator(nil)`（`handlers.go:40`）保證 nil-safe。
3. Hot path 從未觸發（`/annotate` 為人工觸發，failure 時 fallback `frame.Attribution`，見 `handlers.go:283-309`）。

但這是**靠紀律維繫的脆弱平衡**，沒有結構保證。本框架的 `internal/llm` Router 層正是為了把這條邊界**結構化**：
- `internal/llm`（新）也為 X 級。
- `monitoring/api/strategies` 改 import `internal/llm`（仍為 X，依賴 X），符合「X→X 是被允許的」（MATURITY 只禁 S/E → X）。
- `internal/llm` 內部仍呼叫 `internal/llm_annotator`（X → X）。
- v2.0 補充：`internal/llm` 同時持有其他 provider（MiniMax M3、DeepSeek V4、OpenCode-Go、OpenCode-Zen），這些新 client 模組也是 X 級，**也只透過 internal/llm 暴露給上層**。

### 5.3 晉升路徑（Promotion Path）

| 模組 | 現在 | 下一階段 | 達標條件 |
|------|------|---------|---------|
| `llm_annotator` | X | **E** | (a) 連續 30 天 production 無 circuit breaker 長期 open；(b) `AnnotationRecord` ring buffer 與 JSONL store 雙寫 loss 率 = 0；(c) 監控規則覆蓋率 = 5/5 alert + 9/9 recording；(d) 至少 3 個 capability 透過 adapter 接入 |
| `internal/llm`（新） | n/a | **X**（建立時即為 X） | 建立時設定；30 天無重大事故後 → E |
| `internal/llm` E | X | **E** | 與 `llm_annotator` 同步晉升；多 capability 路由穩定；**且** 至少 2 個 provider 通過 production 流量驗證 |
| `internal/llm`（含 M3、V4、備援）| X | **E** | (a) 各 provider 至少 1 個 capability 走 production 30 天；(b) 備援鏈實際觸發 ≥ 1 次（被驗證可運作） |
| `llm_annotator` E | E | **S**（長期目標，6+ 個月） | production 連續 90 天無重大事故；任何 breaking change 須有 migration plan；破壞性介面變更凍結 |

> 註：`S` 級晉升標準取自 `internal/MATURITY.md:9-13`（S 行：「API 穩定、生產執行路徑中、breaking change 需 migration plan」）。

### 5.4 依賴規則矩陣（Dependency Rules）

```
                可依賴（allowed）
from \ to       U      X      E      S
-------------------------------------
U               ✓      ✓      ✓      ✓
X               ✗      ✓      ✓      ✓   ← llm_annotator, internal/llm
E               ✗      ✗      ✓      ✓   ← spawning
S               ✗      ✗      ✗      ✓   ← narrative, orchestrator, risk, monitoring/api

            不可依賴（forbidden）
```

**新增內部規則**：
- **`internal/llm`（Router 層）為 X 級**：可被同為 X 的 `llm_annotator` 依賴、可被 S 級**透過介面注入**使用（`SetRouter(router)`），但不得被任何 S/E 模組的**常規 import 路徑**使用 hot path。
- **Hot path 守門**：`cmd/atlas/main.go` 是唯一允許直接呼叫 Router 構建函式的地方。S/E 模組只能持有 `Router` 介面欄位，呼叫點必須為 async / fallback-safe。
- **v2.0 新增**：Provider client 模組（`internal/llm/providers/kimi`、`internal/llm/providers/minimax`、`internal/llm/providers/deepseek`、`internal/llm/providers/opencode_go`）皆為 X 級；**僅** `internal/llm` 可 import 它們。

### 5.5 晉升時程建議

| 里程碑 | 時間點 | 動作 |
|--------|--------|------|
| M0 | 啟動 | 落地 `internal/llm` Router 骨架（X 級）；不改任何現有呼叫；Provider 常數補齊為七個 |
| M1 | +30 天 | `llm_annotator` 累積 30 天 production 資料；審查 metrics |
| M2 | +60 天 | `internal/llm` 接入 DeepSeek V4-Pro（第一個非 Kimi provider）；rationale translation fallback 上線 |
| M3 | +90 天 | 接入 MiniMax M3 與 DeepSeek V4-Flash；`llm_annotator` 與 `internal/llm` 評估 X → E |
| M4 | +120 天 | 接入 OpenCode-Go 備援；驗證備援鏈真實觸發 |
| M5 | +180 天 | 評估 `llm_annotator` E → S；考慮 self-host MiniMax M3 以解除 §9 風險 8 |

---

## 六、Provider 路由策略

> **📦 已抽離**：本節完整內容移至 [`docs/specs/llm-routing-spec.md`](specs/llm-routing.md)（§6 路由表、§6.1-6.5 備援策略）。

**核心概念速查**：atlas-go 採用 3 個主力 provider（DeepSeek V4-Pro/V4-Flash、MiniMax M3、Kimi K2.6/K2.7）+ 2 個備援通道（OpenCode-Go、OpenCode-Zen），每個 capability 都有四級 fallback 鏈。

**本節中的 cross-reference**（§6.3、§6.5）：當本檔其他章節引用「§6 步驟 2」、「§6.5 觸發條件 3」等，請改為「`specs/llm-routing.md` §6.3 步驟 2」、「`specs/llm-routing.md` §6.5 觸發條件 3」。
## 七、資料流與審計軌跡（Data Flow & Audit Trail）

### 7.1 單次呼叫的資料流（以 Failure Attribution 為例）

```
[Operator]
   │ POST /api/strategies/{id}/annotate
   ▼
[monitoring/api/strategies/handlers.go:271 annotate]
   │ 建構 FailureContext（handlers.go:321 buildFailureContext）
   ▼
[Router.Call(ctx, Request{Capability: FailureAttribution, Payload: fc, DataClass: Regulated})]
   │
   ├── 1. 決定 provider 鏈（capability 預設 = DeepSeek V4-Pro）
   ├── 2. DataClass 閘門檢查（Regulated → V4-Pro 通過，無降級）
   ├── 3. 計算 cache key（沿用 cacheKey, annotator.go:114-119）
   ├── 4. 檢查 response cache
   │      └─ 命中 → 立即回傳，outcome="cache_hit"
   ├── 5. 等 rate limiter（沿用 KimiClient.limiter, annotator.go:162）
   ├── 6. 檢查 circuit breaker（沿用 llm_annotator/circuit_breaker.go:14-18）
   │      └─ open → 立即回 ErrCircuitOpen，轉 backup1
   ├── 7. 選擇 provider 實作
   │      ├─ 7a. primary = DeepSeek V4-Pro
   │      │     ├─ 自動 retry（maxAttempts=3, annotator.go:163）
   │      │     ├─ 寫 Prometheus counter（capability + provider 兩個 label）
   │      │     ├─ 寫 ring buffer（appendAnnotation, observability.go:235-247）
   │      │     └─ 寫 JSONL store
   │      └─ 7b. 失敗 → 7c. backup1 = MiniMax M3
   │                    └─ 同上流程
   │      └─ 7c. 失敗 → 7d. backup2 = OpenCode-Go
   │                    └─ 同上流程
   │      └─ 7d. 失敗 → last resort = rule_based fallback
   └── 8. 回傳 Response{Output, Provider, Usage, TraceID, CacheHit, AttemptedProviders}
```

### 7.2 結構化審計三軌

#### 軌跡 1：Metric（aggregated）

輸出位置：`monitoring/rules/llm_annotator_recording.yml` 已有的 9 條 recording rule（`:1-63`）+ `monitoring/rules/llm_annotator_alerts.yml` 5 個 alert group（`:1-144`）。

新增維度（v2.0 補）：每條 metric 加 `provider` label：

```yaml
# 既有：llm_annotator_requests_total{outcome=~"success|cache_hit"}
# v1.0 新增：llm_annotator_requests_total{capability="strategy.failure_attribution",outcome=~"success|cache_hit"}
# v2.0 新增：llm_annotator_requests_total{capability="...",provider="deepseek",outcome=~"success|cache_hit"}
```

> **不要覆寫既有 label 集**；append-only 擴張，避免破壞現有 dashboard。

#### 軌跡 2：Record（per-call）

每個 capability 必須定義 `CapabilityRecord`，由 Router 統一寫入 `AnnotationStore`：

```go
type CapabilityRecord struct {
    ID                string                 `json:"id"`         // 沿用 nextAnnotationID 格式（observability.go:259-264）
    Timestamp         time.Time              `json:"timestamp"`
    Capability        Capability             `json:"capability"`
    Provider          Provider               `json:"provider"`    // 實際執行者
    LatencyMs         int64                  `json:"latency_ms"`
    Tokens            int64                  `json:"tokens"`
    Outcome           string                 `json:"outcome"`    // "success" | "cache_hit" | "rate_limited" | "breaker_open" | "timeout" | "5xx" | "4xx" | "protocol_error"
    InputHash         string                 `json:"input_hash"` // sha256 of normalized payload（沿用 cacheKey 演算法）
    Trace             map[string]any         `json:"trace,omitempty"` // capability-specific
    DataClass         DataClass              `json:"data_class"` // v2.0 新增
    AttemptedProviders []Provider            `json:"attempted_providers,omitempty"` // v2.0 新增
}
```

寫入路徑：
1. **記憶體** ring buffer cap 1000（沿用 `annotationBufferCap`，`observability.go:233`）。
2. **JSONL** `data/state/llm_annotations/annotations.jsonl`，50MB/檔，3 rotated copies（沿用 `persistence.go:30-33`）。
3. **Prometheus** counter/gauge（既有 metrics）。

#### 軌跡 3：Trace（disputable）

當 `Options.Trace == true`（僅在 dispute / 開發模式開啟），Router 額外記錄：

```json
{
  "request_id": "ann-1718738400123456789-42",
  "prompt_template_version": "v3.2",
  "prompt_sections": [
    {"name": "system", "content_hash": "sha256:..."},
    {"name": "macro_snapshot", "content_hash": "sha256:..."},
    {"name": "conditions", "content_hash": "sha256:..."}
  ],
  "raw_response": "...",
  "reasoning_chain": ["step1: identified regime=risk_off", "step2: ..."],
  "model_metadata": {
    "provider": "deepseek",
    "model": "v4-pro",
    "temperature": 0.0,
    "top_p": 1.0
  },
  "fallback_chain": ["deepseek", "minimax", "opencode-go"],
  "data_class": "regulated"
}
```

> Trace 預設關閉；當 dispute 發生時可從 ring buffer / JSONL store 重新生成（input hash + capability + provider 足以重放 prompt）。

### 7.3 跨模組審計聚合

Router 的 `Health()` 與 `/healthz/llm` 端點（提案）提供：
- 每個 provider 的 breaker state。
- 每個 capability 的 24h error rate（依 §6.5 備援指標）。
- 每月 token 用量與估計成本（沿用 `CostReport`，`observability.go:158-197`）。
- v2.0 新增：每 capability × provider 的 cost breakdown（從 metric label 聚合）。

```yaml
# 提案：monitoring/rules/llm_router_recording.yml
groups:
  - name: llm_router_recording
    interval: 30s
    rules:
      - record: llm:capability:requests_total:rate1h
        expr: sum by (capability, provider, outcome) (rate(llm_annotator_requests_total[1h]))

      - record: llm:capability:cost_usd:rate24h
        expr: sum by (capability, provider) (rate(llm_annotator_tokens_total[24h]) * 0.0012 / 1000)

      - record: llm:provider:health
        expr: llm_router_provider_health

      - record: llm:fallback:triggered:rate1h
        expr: sum by (capability, from_provider, to_provider) (rate(llm_router_fallback_triggered_total[1h]))
```

### 7.4 資料保留政策

| 資料類型 | 保留期 | 理由 |
|----------|--------|------|
| 記憶體 ring buffer | 即時 | 容量限制（1000 筆）；不做長期保留 |
| JSONL store | 90 天 hot + 1 年 cold | 沿用既有 rotation（`persistence.go:30-33`），可調 |
| Prometheus metrics | 30 天 high-res + 1 年 low-res | 沿用 `monitoring/rules/llm_annotator_recording.yml` 既有 policy |
| Trace（若開啟） | 30 天 | 容量敏感，僅 dispute 用 |
| v2.0 新增：AttemptedProviders 欄位 | 與 record 同週期 | dispute 時用於「為什麼走到 backup2」 |

---

## 八、遷移路徑

> **📦 已歸檔（RESOLVED）**：本節完整內容移至 [`docs/archive/2026-06-27-llm-migration-roadmap-resolved.md`](archive/2026-06-27-llm-migration-roadmap-resolved.md)。

**完成狀態**：Phase 1-3 全部完成（v0.0.0.21 verified）；Phase 4+ 持續進行（OpenCode-Zen 整合、self-host M3 評估）。

**本節中的 cross-reference**（Phase 1-3 細節）：改為「`archive/llm-migration-roadmap-resolved.md` Phase X」格式。
## 九、風險與緩解（Risks & Mitigations）

### 風險 1：幻覺（Hallucination）

**描述**：LLM 生成看似合理但實際錯誤的歸因 / 翻譯 / 摘要，operator 可能據此下單。

**影響**：
- 金流決策（Failure Attribution 是「為何這次策略沒中」的事後解釋，影響 operator 後續決策）
- 對外敘事（rationale 翻譯會進入 audit log 與 dashboard）

**緩解**：
1. **Fallback First**（原則 4）：任何 LLM 輸出旁必有一個 rule_based / corpus 對照組，UI 同時顯示。
2. **Confidence gate**：rationale translation fallback 要求 confidence > 0.7 才採用，否則 passthrough（`§4.5.2`）。
3. **Human-in-the-loop**：所有 capability 預設為「建議」而非「自動採用」。
4. **定期 corpus 收斂**：每週把 LLM 通過的高信心翻譯回填進 `reasonCorpus`（`rationale_corpus.go:48-97`），逐步降低對 LLM 的依賴。
5. **v2.0 新增**：模型選用本身是 hallucination 風險的源頭——K2.7 用於敘事任務就是結構性幻覺源。對應緩解：ADR-009 限縮 K2.7 用途。

### 風險 2：Prompt 漂移（Prompt Drift）

**描述**：同一個 prompt 在不同 model 版本 / 同一 model 不同時間，輸出風格不一致。

**緩解**：
1. **Prompt 版本化**：每個 capability 的 prompt template 帶 `version` 標籤；記錄於 trace。
2. **回歸測試**：CI 跑「golden output」測試——固定 input 比對固定 output snapshot；snapshot 變更需 PR review。
3. **Capability 鎖定 model**：每個 capability 綁定特定 model id（config 內）；不允許 runtime 自動切換 model。
4. **v2.0 新增**：備援鏈可能切換 provider；切換時必須把新 provider 與 model 都記錄於 trace，確保 dispute 時可定位實際使用的 model。

### 風險 3：成本爆炸（Cost Explosion）

**描述**：rationale translation fallback 觸發頻率超預期（每個 pipeline tick 數十筆 corpus miss），月底帳單暴增。

**緩解**：
1. **MonthlyBudgetUSD**（`§6.3`）：超限自動開 breaker（沿用 `llm_annotator/doc.go:91-94` 的 `BudgetThreshold` + `BudgetCallback`）。
2. **Per-capability metric + alert**：新增 alert `LLMRouterCapabilityBudgetExceeded`，threshold 80% 月預算。
3. **Rate limit**：`RatePerSecond` 限制（沿用 `annotator.go:162`）。
4. **Cache TTL**：rationale translation fallback TTL 24h（同一英文短句 24h 內只送一次）。
5. **CostReport**：每月產出報告（沿用 `CostReport`，`observability.go:158-197`）。
6. **v2.0 新增**：每 capability × provider 的 cost breakdown（沿用 §7.3 新 metric），可看出「翻譯用 V4-Flash 還是 M3 哪個便宜」、「歸因走 V4-Pro 還是 M3 哪個 cost-efficient」。

### 風險 4：模型棄用（Model Deprecation）

**描述**：Kimi 棄用 K2.6 / K2.7、DeepSeek 棄用 V4-Pro / V4-Flash、MiniMax 棄用 M3 整個系列。

**緩解**：
1. **OpenAI-compatible abstraction**：所有 provider 都用 OpenAI 相容介面（DeepSeek 還有 Anthropic 相容），換 provider 只需改 `BaseURL` 與 `APIKey`。
2. **Multi-provider 規劃**（`§6.5`）：備援鏈設計讓單一 provider 棄用的影響可被吸收。
3. **Deprecation monitoring**：監控各 provider 的 deprecation 公告；設定 `model availability` alert（HTTP 200 但 body 帶 deprecation 訊息）。
4. **v2.0 新增**：DeepSeek 舊 `deepseek-chat` / `deepseek-reasoner` 已宣布於 2026/07/24 棄用。Phase 1 已切換為 `deepseek-v4-pro` / `deepseek-v4-flash`；既有任何引用舊 model id 的 config 需在 Phase 1 一次性 migration 完畢。

### 風險 5：Privacy 與 Secrets 外洩

**描述**：prompt 含 macro snapshot（`ConditionSnapshot`、`MacroSnapshot`，`doc.go:38-73`），若 LLM provider 把資料用於訓練，可能造成隱私問題。

**緩解**：
1. **Contract review**：與各 provider 確認 inference-time 資料不進入訓練集。
2. **No PII**：現有 `FailureContext`（`doc.go:38-73`）不含個資；但 rationale 翻譯若引進 operator 輸入需另外過濾。
3. **Redaction layer**：新增 `internal/llm/redact.go`，所有 input 經過 regex 過濾（電話、統編、email）後再送 LLM。

### 風險 6：Circuit Breaker 永遠 Open

**描述**：provider 持續故障，breaker open，所有 LLM 呼叫 fail，但 rule_based fallback 正常運作——operator 沒注意到 LLM 路徑已死。

**緩解**：
1. **Traffic cessation alert**：`monitoring/rules/llm_annotator_alerts.yml` 既有 `LLMAnnotatorTrafficCessation` 規則（驗證存在於 144 行檔案中）。
2. **每週 health report**：Router 自動每週發送 `RouterHealth` snapshot 到 ops 頻道。
3. **Prometheus `up` metric**：以 5 分鐘為單位確認 `rate(llm_annotator_requests_total[5m]) > 0`。
4. **v2.0 新增**：每 provider 各自有 health gauge（`llm_router_provider_health`）；某 provider 連續 30 分鐘 unhealthy 觸發 alert（即使其他 provider 仍正常）。

### 風險 7：S 級模組意外依賴 LLM

**描述**：有人不小心在 `internal/risk` 或 `internal/orchestrator` 加了 `import llm_annotator`，CI 沒擋。

**緩解**：
1. **CI gate**：新增 `scripts/check-maturity-imports.sh`，掃描 S/E 級 package 是否有 `import .../internal/llm_annotator` 或 `import .../internal/llm`，fail build。
2. **Code review checklist**：PR 模板加入「本 PR 是否新增 LLM 依賴？」欄位。

### 風險 8（v2.0 新增）：資料主權與法規合規（Data Sovereignty & Regulatory Compliance）

**描述**：MiniMax M3 hosted API 的伺服器位於中國境內，受 2017 年《中華人民共和國國家安全法》管轄。當 regulator 要求 audit、特定 LLM provider 收到 subpoena、或 inference 結果被視為「資料傳輸出境」，可能違反金融監理要求（例如台灣金融主管機關對「敏感金融資料不得跨境傳輸」的解釋）。此外，營業秘密（universe selection logic、RiskManager 演算法）一旦透過 hosted API 送出，可能被視為對外揭露。

**影響**：
- 法規風險：受規範金融資料（如 PRISM 結果、RiskManager VaR、StrategyFrame 細節）若透過 hosted M3 傳輸，可能構成跨境資料傳輸違規。
- 營業秘密風險：universe selection、risk model 等 IP 透過 LLM inference 送出可能被視為揭露。
- 聲譽風險：若 LLM provider 的訓練資料 / inference log 被迫配合當地執法，atlas-go 客戶資料可能被存取。

**緩解**：
1. **Self-host MiniMax M3**（最徹底）：M3 開源權重以 minimax-community 授權釋出，MXFP8 量化後 440GB 可部署於內部 GPU 叢集。自架後 inference 全在自有資料中心，無跨境傳輸。建議路徑：先在測試環境驗證 self-host M3 的 inference quality 與 hosted 相當，再將 `ProviderMiniMax` 預設值從 hosted URL 切換到 self-host URL。**此為 ADR-010 的強制要求**。
2. **DataClass 閘門**（結構性）：Router 內 `DataClass == Regulated` 自動拒絕 MiniMax hosted 路徑，強制走 self-host 或 backup1（DeepSeek V4-Pro）。設計細節見 §4.2 的 `DataClass` 定義與 §6.3 路由優先順序。
3. **Redaction layer 加強**（沿用風險 5 緩解 3）：`internal/llm/redact.go` 在 `DataClass == Regulated` 時啟用更嚴格的規則（regex 黑名單擴大、欄位層級遮罩）。
4. **資料分類政策**：上線前明確定義哪些欄位屬於 `Regulated`、哪些是 `NonRegulated`、哪些是 `Secret`。`Regulated` 預設包含：所有 StrategyFrame 的 condition、RiskManager 的 VaR 與 marginal contribution、PRISM 訓練結果。`Secret` 預設包含：universe selection 邏輯、RiskManager 演算法細節（只允許走 self-host）。
5. **Audit log**：`AnnotationRecord`（沿用 `observability.go:209-216`）必須記錄 `DataClass` 與實際執行的 provider（`AttemptedProviders`）；合規 audit 時可回答「這個資料送給了誰」。
6. **Provider 合約審查**：與 DeepSeek、Kimi、OpenCode 簽訂的合約需明文排除「inference 資料用於訓練」、「第三方 subpoena 配合義務」條款；若無法排除則列入禁用清單。
7. **定期 review**：每季 review 一次資料分類政策與 provider 適用性；新公布的法規變更需在 30 天內評估影響。

**為何這是 v2.0 必須處理的風險**：v1.0 單一 provider（Kimi/Moonshot）沒有這個顧慮；v2.0 引入 MiniMax M3 後，hosted M3 的管轄風險就成為架構層面的現實問題。**只在文件章節列為「未來考慮」並不足夠**——必須有結構性閘門（DataClass 與 self-host fallback）才能讓 LLM 整合在合規框架下運作。

---

## 十、決策紀錄

> **📦 已抽離**：本節完整內容（ADR-001 至 ADR-010）移至 [`docs/llm-adr-log.md`](llm-adr-log.md)。

**ADR 新增流程**：建立新章節於 `docs/llm-adr-log.md`，狀態 = Proposed → Accepted / Superseded；append-only，不覆寫既有紀錄。
## 附錄 A：本框架未涵蓋的議題（Out of Scope）

下列議題**明確不在**本框架承諾範圍；新增需另開 ADR。

1. **Agent runtime LLM**（PRISM 訓練時即時呼叫 LLM 評分個股建議）：需要「設計藍圖 → 實作」需求強度評估；本框架僅承諾 `prism_executor` 結案摘要。
2. **Multi-language prompt engineering**：本框架假設所有 prompt 為繁中 + 英文混合；新增語系需另案。
3. **Frontend LLM**：web frontend 0 LLM import（驗證於 codebase 全域 grep）；不擴張至瀏覽器端 LLM。
4. **Streaming response**：現有 KimiClient 採 blocking single-shot；不擴張至 SSE / WebSocket streaming。
5. **Fine-tuning**：本框架不涵蓋 model fine-tuning pipeline；如需此能力需獨立設計。
6. **v2.0 新增**：Self-host 部署細節（M3 440GB MXFP8 量化權重的 GPU 叢集配置、inference latency 調校、模型版本管理）。Phase 4+ 才開始評估。
7. **v2.0 新增**：OpenCode-Zen 整合細節。當 OpenCode-Go 連續失敗時的自動切換邏輯、Zen 的 regional 選擇策略、Zen 的服務品質監控。

---

## 附錄 B：關鍵檔案索引（Quick Reference）

| 檔案 | 角色 |
|------|------|
| `internal/llm_annotator/doc.go` | 既有 Annotator 介面 + Config 定義 |
| `internal/llm_annotator/annotator.go` | KimiClient 實作（v2.0 起 BaseURL 預設值改為 `api.kimi.com/coding/v1`） |
| `internal/llm_annotator/circuit_breaker.go` | 自有 circuit breaker（避免 import cycle） |
| `internal/llm_annotator/observability.go` | MetricsRecorder / CostReport / AnnotationRecord |
| `internal/llm_annotator/persistence.go` | JSONL store + rotation |
| `internal/llm_annotator/health.go` | /healthz handler |
| `internal/narrative/rationale_corpus.go` | 32 筆靜態英中對映 + LLM 擴張點（保留） |
| `internal/monitoring/api/strategies/handlers.go` | /annotate endpoint，唯一現有 LLM 入口 |
| `internal/monitoring/api/pipeline/handlers.go` | rationale 翻譯的呼叫端（fallback hook 位置） |
| `internal/spawning/agent_factory.go` | gap → AgentSpec（無 LLM） |
| `internal/spawning/gap_detector.go` | KnowledgeGap 偵測（LLM hook 位置） |
| `internal/spawning/spawning_manager.go` | SpawningManager 編排 |
| `internal/orchestrator/prism_executor.go` | PRISM 訓練（LLM hook 位置） |
| `cmd/atlas/main.go:1607-1629` | production wiring（唯一啟動點） |
| `internal/config/config.go:204-208` | `GetSecret`：API key 取得 |
| `internal/MATURITY.md:75-89` | X 級定義與 `llm_annotator` 條目 |
| `monitoring/rules/llm_annotator_alerts.yml` | 5 個 alert group |
| `monitoring/rules/llm_annotator_recording.yml` | 9 條 recording rule |
| `docs/ai-agent-architecture.md` | 17-agent 設計藍圖（非實作狀態） |
| `configs/agents.json` | 17 agents 註冊（dev-time prompt 對應） |
| `prompts/agents/*.md` | agent prompt 範例（非 runtime） |
| `internal/llm/provider.go`（v2.0 新檔） | Provider / Capability / Request / Response 介面 |
| `internal/llm/router.go`（v2.0 新檔） | Router 預設實作 |
| `internal/llm/adapters/annotator_adapter.go`（v2.0 新檔） | 既有 Annotator 的 Provider adapter |
| `internal/llm/providers/deepseek/v4.go`（v2.0 新檔） | DeepSeek V4-Pro / V4-Flash client |
| `internal/llm/providers/minimax/m3.go`（v2.0 新檔） | MiniMax M3 client（hosted + self-host） |
| `internal/llm/providers/opencode_go/`（v2.0 新檔） | OpenCode-Go multi-model 訂閱 client |
| `internal/llm/providers/opencode_zen/`（v2.0 新檔） | OpenCode-Zen regional fallback client |
| `internal/llm/redact.go`（v2.0 新檔） | PII / 受規範欄位 redaction layer |
| `configs/llm_router.yaml`（v2.0 新檔） | 每 capability 的 RoutingChain 設定 |

---

## 附錄 C：詞彙表

| 詞 | 定義 |
|----|------|
| **Capability** | 語意穩定的能力 ID，例如 `strategy.failure_attribution` |
| **Provider** | 具體 LLM 後端的識別字串，例如 `kimi` / `deepseek` / `minimax` / `opencode-go` |
| **Router** | capability → provider 的路由器 |
| **Adapter** | 把既有介面包成新介面的薄層；不改既有簽章 |
| **Fallback** | LLM 失敗時的 rule-based / corpus / passthrough 對照組 |
| **RoutingChain** | v2.0 新增：每個 capability 對應的完整四級 fallback 鏈（primary / backup1 / backup2 / last resort） |
| **DataClass** | v2.0 新增：資料主權分級（`unmarked` / `non_regulated` / `regulated` / `secret`） |
| **Audit trail** | 三軌：metric（aggregated）、record（per-call）、trace（disputable） |
| **X 級** | Experimental；API 不穩定；不可被 S/E 級依賴（`internal/MATURITY.md:75-78`） |
| **Kimi Code Plan** | kimi.com 的 coding subscription 服務；endpoint `api.kimi.com/coding/v1`；與 Moonshot Platform（`api.moonshot.cn`）獨立 |
| **Moonshot Platform** | Moonshot 的企業級 API；atlas-go **不使用**此通道；列為避免誤用的對照 |
| **OpenCode-Go** | 第三方 multi-model 訂閱服務；atlas-go 將其作為通用 backup2 |
| **OpenCode-Zen** | 第三方備援平台；atlas-go 將其作為 last-mile 備援 |

---

## 附錄 D：版本與維護

- **v1.0**（前版文件）：定義 capability 分類學、介面合約、遷移路徑、決策紀錄；採用單一 provider（Kimi/Moonshot）。
- **v2.0**：補齊三主力模型（Kimi K2.6/K2.7、MiniMax M3、DeepSeek V4-Pro/V4-Flash）+ 兩個備援通道；新增 §1a Model Capability Matrix；§3 改為四級路由鏈；§6 重寫為 multi-provider 路由策略與備援機制；新增 §9 風險 8（資料主權）；ADR-005 重寫，新增 ADR-009（K2.7 限縮）、ADR-010（資料主權閘門）。
- **v2.1**（本文件）：新增 §3.1a 台灣股市慣性特徵領域上下文；Phase 2 新增台股情境 A/B 評測任務；明確台股特徵為輸出評估框架非 Provider 設計輸入。
- **後續版本觸發條件**：
  - `llm_annotator` 晉升 E 級 → 需更新 `§5.3` 晉升路徑與 `internal/MATURITY.md`
  - 新增 capability → 需更新 `§3` 分類學與 `§3.2` 決策表
  - 新增 provider → 需更新 `§1a` 矩陣與 `§6.1` 路由表
  - 新法規影響資料分類 → 需更新 `§9 風險 8` 與 `ADR-010`
  - Self-host M3 部署完成 → 需更新 `§1a.3`、`§6.1` 路由表預設值
  - 多 capability 跨模組整合 → 需更新 `§7` 審計軌跡
- **本文件所有「應」字皆對應 MATURITY 規則或既有 production 行為的對齊承諾；無空泛措辭。**
