# AGENTS.md — internal/narrative

本目錄負責**巨集觀敘事（Macro Narrative）**事件偵測與因果鏈（Causal Chain）推導。

---

## OVERVIEW

`internal/narrative` 透過監控全球總經指標（美債、匯率、VIX、原油、黃金）與特定產業情緒，將原始數據轉化為具備信心度與命中率的領域事件，並藉由因果範本推論對台股各板塊的潛在影響。

### 核心資料流
`MacroIngestor (MarketData) → NarrativeEvent → KnowledgeBase (Match Template) → CausalChain`

---

## EVENT TYPES

### NarrativeEvent 結構
- `Theme`: 事件主題標籤（如 `US_rates_up`, `AI_capex_surge`）。
- `Confidence`: 偵測演算法對事件成立的信心度 `[0.0, 1.0]`。
- `ConfidenceSource`: 信心度來源（預設 `heuristic_fixed_v1`）。
- `HitRate`: 該主題在歷史中的回測命中率。
- `SourceData`: 觸發事件的原始數值快照。

### 內建主題與命中率 (Built-in Hit Rates)
| Theme | Hit Rate | 偵測指標 |
|-------|----------|---------|
| `AI_capex_surge` | **0.81** | AI 資本支出展望、CoWoS 需求 |
| `US_rates_up` | **0.72** | 美債 10Y 殖利率、美元指數 (DXY) |
| `JPY_carry_unwind` | **0.68** | 日圓匯率、VIX 波動率 |
| `geopolitical_risk_spike` | **0.65** | 黃金、VIX、地緣政治指數 (GPR) |
| `oil_price_shock` | **0.58** | 原油價格劇烈波動 |

---

## ANTI-PATTERNS

- **手動計算 HitRate**: `NarrativeEvent` 的 `HitRate` 必須透過 `hitRateForTheme()` 從 `DefaultTemplates` 取得，不可在 detector 中硬編碼。
- **遺漏 SourceData**: 每個 `NarrativeEvent` 必須包含觸發時的 `SourceData`（如 bps 變化或百分比），以利後續決策鏈透明化追蹤。
- **無視 Region 限制**: 因果鏈匹配時會檢查 `RequiredRegion`（如 `US_rates_up` 需為 `US`），擴充偵測邏輯時須確保地域屬性正確。
- **直接修改模型權重**: `InvestmentModel` 的權重更新應由 `UpdateModelWeights` (Inverse-error) 統一處理，避免手動干預造成權重失衡。

---

## 測試與驗證

- 偵測邏輯驗證：`go test -v ./internal/narrative/ingestor_test.go`
- 模板匹配驗證：`go test -v ./internal/narrative/narrative_test.go`
