# AGENTS.md — internal/narrative

本目錄負責**巨集觀敘事（Macro Narrative）**事件偵測與因果鏈（Causal Chain）推導。

---

## OVERVIEW

`internal/narrative` 透過監控全球總經指標（美債、匯率、VIX、原油、黃金）與特定產業情緒，將原始數據轉化為具備信心度與命中率的領域事件，並藉由因果範本推論對台股各板塊的潛在影響。

### 核心資料流
`MacroIngestor (MarketData) → NarrativeEvent → KnowledgeBase (Match Template) → CausalChain`

### News Sentiment 數據策略
**重要限制**：Finnhub News Sentiment API **僅支援美股公司**，台股無法直接使用。

**實作策略**：
1. **美股大盤作為代理指標**：使用 Finnhub 取得美股（NASDAQ、S&P 500）的新聞情緒與大盤漲跌
2. **外资流向推斷**：當美股下跌且 VIX 上升時，推測外资可能從台股撤離
3. **台股本地數據**：使用 TWSE 開放資料作為台股本地情緒的代理指標（公告數量、討論度）

**因果關係**：
```
美股大跌 + VIX飆升  →  外資撤離台股  →  台股壓力上升  →  RegimeChange
     ↑                                          ↑
  Finnhub API                              TWSE 開放資料
  (新聞情緒)                               (本地代理指標)
```

---

## EVENT TYPES

### NarrativeEvent 結構（擴展版）
- `Theme`: 事件主題標籤（如 `US_rates_up`, `AI_capex_surge`）。
- `Confidence`: 偵測演算法對事件成立的信心度 `[0.0, 1.0]`。
- `ConfidenceSource`: 信心度來源（預設 `heuristic_fixed_v1`）。
- `HitRate`: 該主題在歷史中的回測命中率。
- `SourceData`: 觸發事件的原始數值快照。
- `Duration`: 事件影響持續時間（time.Duration）。
- `ExpiresAt`: 事件過期時間點（*time.Time）。
- `Severity`: 事件嚴重程度（`low`/`medium`/`high`/`critical`）。
- `Status`: 事件狀態（`active`/`confirmed`/`faded`/`expired`）。

### Event 狀態機
```
active → confirmed → faded → expired
  ↓
  （可直接跳轉）
```

| 狀態 | 說明 | 轉換條件 |
|------|------|----------|
| `active` | 事件初始偵測 | 信心度 > 閾值 |
| `confirmed` | 事件已確認（多個數據源驗證） | 2+ 獨立數據源確認 |
| `faded` | 事件影響減弱 | 時間経過 > Duration × 0.8 |
| `expired` | 事件完全過期 | 時間経過 > ExpiresAt |

### Severity 等級
| 等級 | 說明 | 因子權重調整 |
|------|------|--------------|
| `low` | 輕微影響 | ±5% |
| `medium` | 中度影響 | ±10% |
| `high` | 高度影響 | ±20% |
| `critical` | 極度影響（緊急） | ±30%，立即觸發 RegimeChange |

### 內建主題與命中率 (Built-in Hit Rates)
| Theme | Hit Rate | 偵測指標 |
|-------|----------|---------|
| `AI_capex_surge` | **0.81** | AI 資本支出展望、CoWoS 需求 |
| `US_rates_up` | **0.72** | 美債 10Y 殖利率、美元指數 (DXY) |
| `JPY_carry_unwind` | **0.68** | 日圓匯率、VIX 波動率 |
| `geopolitical_risk_spike` | **0.65** | 黃金、VIX、地緣政治指數 (GPR) |
| `oil_price_shock` | **0.58** | 原油價格劇烈波動 |

### 內建事件持續時間 (Event Durations)
從學術研究與歷史經驗訂定：

| Theme | 價格發現期 | 影響持續期 | Severity 預設 |
|-------|-----------|------------|---------------|
| `AI_capex_surge` | 1-30 日 | 30-90 日 | `high` |
| `US_rates_up` | 1-4 小時 | 3-7 日 | `medium` |
| `JPY_carry_unwind` | 即時 | 7-14 日 | `medium` |
| `geopolitical_risk_spike` | 即時 | 7-30 日 | `high` |
| `oil_price_shock` | 即時 | 5-15 日 | `medium` |
| `Fed_emergency_cut` | 即時 | 1-3 日 | `critical` |
| `earnings_surprise` | 1-3 日 | 5-10 日 | `high` |

---

## EVENT LIFECYCLE MANAGEMENT

### 事件過期機制
1. 每個 `NarrativeEvent` 在建立時根據 Theme 自動設定 `Duration` 與 `ExpiresAt`。
2. `EventLifecycleManager` 負責追蹤所有活躍事件，定期更新狀態。
3. 當事件從 `active` 轉為 `faded` 時，FactorWeightEngine 收到通知，開始漸進式回調權重。
4. 當事件過期時，從 FactorWeightEngine 的活躍事件列表移除。

### RegimeChange 觸發條件
以下情況觸發 RegimeChange：
- VIX 突破 30（進入 HighVol Regime）
- VIX 突破 25 且趨勢向下（進入 Bear Regime）
- VIX 跌破 15 且趨勢向上（進入 Bull Regime）
- `critical` 等級事件觸發
- StressIndex 突破 80

---

## ANTI-PATTERNS

- **手動計算 HitRate**: `NarrativeEvent` 的 `HitRate` 必須透過 `hitRateForTheme()` 從 `DefaultTemplates` 取得，不可在 detector 中硬編碼。
- **遺漏 SourceData**: 每個 `NarrativeEvent` 必須包含觸發時的 `SourceData`（如 bps 變化或百分比），以利後續決策鏈透明化追蹤。
- **無視 Region 限制**: 因果鏈匹配時會檢查 `RequiredRegion`（如 `US_rates_up` 需為 `US`），擴充偵測邏輯時須確保地域屬性正確。
- **直接修改模型權重**: `InvestmentModel` 的權重更新應由 `UpdateModelWeights` (Inverse-error) 統一處理，避免手動干預造成權重失衡。
- **忽略 Duration/ExpiresAt**: 新增 `Duration` 和 `ExpiresAt` 後，detector 必須在建立事件時設定這些欄位，不可遺漏。
- **手動設定 Status**: Status 的狀態轉換應由 `EventLifecycleManager` 統一管理，不应由 detector 直接設定。
- **事件重複偵測**: 相同 Theme 的事件在 active 狀態時不應重複偵測，應更新現有事件的 Confidence 而非建立新事件。

---

## KEY TYPES (public 結構體)

| 結構體 | 檔案 | 用途 |
|--------|------|------|
| `NarrativeEvent` | types.go | 領域事件結構 |
| `KnowledgeBase` | knowledge_base.go | 因果範本匹配 |
| `CausalChain` | causal_chain.go | 因果鏈推導 |
| `MacroIngestor` | ingestor.go | 巨集觀數據攝入 |
| `EventLifecycleManager` | lifecycle.go | 事件生命週期管理 |
| `TaiwanStressIndex` | taiwan_stress_index.go | 台灣壓力指數計算 |

---

## 測試與驗證

- 偵測邏輯驗證：`go test -v ./internal/narrative/ingestor_test.go`
- 模板匹配驗證：`go test -v ./internal/narrative/narrative_test.go`
- 事件生命週期驗證：`go test -v ./internal/narrative/lifecycle_test.go`
- StressIndex 驗證：`go test -v ./internal/narrative/taiwan_stress_index_test.go`
