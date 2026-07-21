# LLM Provider 路由策略（Provider Routing Strategy）

> **文件角色**：atlas-go LLM 多 Provider 路由 + 備援鏈的完整規格（架構藍圖 §6 抽離）。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.1）
> **Maturity 規則**：`internal/MATURITY.md` LLM 相關條目；本文件為 X 級，僅供 reference，不直接 import

---

## 六、Provider 路由策略

> **v2.0 重寫**：本章由原本的「單一 provider 預設」改為「多 provider 依 capability 路由 + 備援鏈」。每個 capability 的 primary 選擇依 §3.2 決策表，備援鏈依健康度動態降級。

### 6.1 路由表（每 capability 的四級鏈）

**Capability column 命名約定**：本表 capability column 採用 `internal/llm/provider.go` 中 Capability 常數的字串值（即 `CapabilityFailureAttribution` 的字面值），Phase 1 開發期使用的 dotted name（如 `strategy.failure_attribution`、`narrative.rationale_translation_fallback`）僅作為 §3 capability taxonomy 的歷史參照，已不具權威性。Phase 2 整合時若 doc scope 與 code scope 不一致（如 `rationale_translation_fallback` vs `rationale_generation`），需另立 ADR 記錄決策（目前 ADR-011 處理六處落差）。

| Capability | Primary | Backup1 | Backup2 | Last Resort |
|------------|---------|---------|---------|-------------|
| `failure_attribution` | DeepSeek V4-Pro | MiniMax M3 | OpenCode-Go | `rule_based`（`frame.Attribution`） |
| `rationale_generation` | DeepSeek V4-Flash | MiniMax M3 | OpenCode-Go | `passthrough`（原字回傳） |
| `strategy_summary` | DeepSeek V4-Pro | MiniMax M3 | OpenCode-Go | `null`（端點回空字串） |
| `prompt_lint` | DeepSeek V4-Flash | Kimi K2.7 | OpenCode-Go | `pass`（CI 不擋） |
| `scenario_simulation` | MiniMax M3 | DeepSeek V4-Pro | OpenCode-Go | `discard`（不存） |
| `risk_surface_extraction` | MiniMax M3 | DeepSeek V4-Pro | OpenCode-Go | `passthrough`（保留低覆蓋率原描述） |
| `regime_explanation` | DeepSeek V4-Flash | MiniMax M3 | OpenCode-Go | `passthrough` |
| `performance_forensics` | DeepSeek V4-Pro | MiniMax M3 | OpenCode-Go | `passthrough` |
| `code_review_annotation` | Kimi K2.7 | DeepSeek V4-Flash | OpenCode-Go | `empty`（無註解） |

> **ADR-011**（Phase 2 capability name 對齊決策）：採用 Option B — 對齊 code → doc。Phase 1 開發期間 doc 與 code 曾使用不同的命名空間（doc 用 dotted name、code 用 snake_case enum），Phase 2 統一以 code enum 為權威來源。對齊過程中發現的 scope 差異（如 `narrative.rationale_translation_fallback` 的翻譯補丁概念 vs `rationale_generation` 的 rationale 生成概念）以本 ADR 記錄，Phase 2 adapter 實作時須重新定義兩者的輸入輸出契約。

**為何每列都至少 4 級**：
- `primary` 對應 §3.2 決策表，與任務特性最佳匹配。
- `backup1` 通常為「同任務類型的次優模型」；例如 reasoning 類 capability 從 V4-Pro 降級到 M3。
- `backup2` 統一為 OpenCode-Go（generic multi-model 訂閱），作為「所有上游同時壞掉」前的最後聰明選擇。
- `last resort` 因 capability 而異：翻譯可 passthrough、摘要可空字串、PRISM 可 discard。

**為何 OpenCode-Zen 不在主鏈上**：OpenCode-Zen 是 last-mile 備援，僅在 OpenCode-Go 也不可用時由 Router 自動啟動；不寫在這張表是為了避免誤導讀者把它當成同級備援。OpenCode-Zen 啟動時所有 capability 自動降級，且會發送 alert（見 §6.5 觸發條件 3）。

### 6.2 成本與效能矩陣

| Provider | Model | Input $/1M | Output $/1M | 主要強項 | 主要弱項 |
|----------|-------|-----------|------------|----------|----------|
| Kimi | K2.7 | $0.95 | $4.00 | 純程式碼生成 | 無金融 / 敘事能力 |
| Kimi | K2.6 | $0.95 | $4.00 | 通用、K2.7 的 instruct 對應 | 仍非頂級推理 |
| MiniMax | M3 | $0.30 | $1.20 | 金融、繁中、1M context | 資料主權風險（hosted） |
| DeepSeek | V4-Pro | 市場報價 | 市場報價 | 推理、繁中、code | HLE、抽象推理仍落後 |
| DeepSeek | V4-Flash | $0.14 | $0.28 | 成本最低、速度快 | 推理弱 |
| OpenCode-Go | multi-model | 訂閱制 | 訂閱制 | 通用 failover、訂閱可控成本 | 額外 latency、模型不固定 |
| OpenCode-Zen | multi | 訂閱制 | 訂閱制 | regional fallback | 服務品質較不穩 |
| Mock | — | 0 | 0 | 測試 | 無生產價值 |

> 註：實際定價以合約為準；本框架不內嵌定價常數於 production code，所有計費由呼叫端傳入 `costPer1kTokens`（沿用 `CostReport` 既有介面，見 `observability.go:158-167`）。
>
> v2.0 新增：每筆呼叫的 `Response` 帶 `Provider` 與 `Usage`，由 metric label `capability` × `provider` 聚合後即可計算每 capability × 每 provider 的實際成本。

### 6.3 Provider 優先順序（Router 內部選擇邏輯）

Router 在收到 `Request` 後，依下列順序決定 provider：

1. **`Options.ForceProvider` 顯式指定**（測試 / sticky routing）
2. **`DataClass` 閘門**：若 `DataClass == Regulated` 且候選 provider 為 MiniMax hosted，**自動降級**到自架 M3 或 backup1
3. **Capability 預設 primary**（§3.2 決策表）
4. **健康度檢查**：
   - primary circuit breaker open → 降級到 backup1
   - backup1 circuit breaker open 或 latency > 2× baseline → 降級到 backup2
   - backup2 也不可用（OpenCode-Go）→ 啟動 OpenCode-Zen + 發 alert
5. **Mock**：僅當 `Options.Trace == true` 或測試環境變數 `ATLAS_LLM_FORCE_MOCK=1`

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
- 保留以備「V4-Pro 與 M3 同時不可用」時降級使用

**V4-Pro**：
- 複雜推理首選（歸因、信心旁註、複雜摘要）
- 若可用則優先；不可用時降級到 MiniMax M3（推理略弱但金融場景強）

**V4-Flash**：
- 簡單 + 高量 + 成本敏感任務首選（翻譯、headline、dev path）
- 推理弱，禁用於歸因 / 信心旁註

**M3**：
- 金融 / 繁中敘事首選（PRISM insight、gap description、複雜 fallback）
- **重要**：`DataClass == Regulated` 時，hosted M3 必須由自架 M3 取代（見 §9 風險 8）
- 應部署「hosted M3 為預設，自架 M3 為受規範資料 fallback」的雙路徑

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
