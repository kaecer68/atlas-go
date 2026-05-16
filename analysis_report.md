# ATLAS 策略進化系統深度診斷與修復建議報告

> ⚠️ **歷史文件**：本報告為 2026-04-25 的診斷報告，部分問題可能已被後續修復。
> 建議重新執行診斷以確認當前狀態。

**日期**: 2026-04-25
**診斷範圍**: Strategy Evolution Skill (`/atlas-strategy-evolution`)
**執行代理**: Sisyphus (OhMyOpenCode)

---

## 執行摘要

本次診斷針對 atlas-go 的策略進化系統進行深度盤查，發現 **7 個關鍵問題**，涵蓋文件與程式碼脫節、缺失模組、靜態配置、死碼、以及可觀測性缺口。所有問題均已確認根因，並附有具體修復建議與優先順序。

| 嚴重性 | 問題數 | 分類 |
|--------|--------|------|
| 🔴 高 | 2 | 文件脫節、死碼 |
| 🟡 中 | 4 | 缺失模組、靜態配置、可觀測性 |
| 🟢 低 | 1 | 架構整合 |

---

## 問題清單

### 🔴 P1: 技能文件與實際程式碼嚴重脫節

**現象**: `atlas-strategy-evolution` 技能文件聲稱 v2.0 已將觀測值門檻提升至 `8/15/25`，但實際程式碼仍為 `3/8/12`。

**根因確認**:
- `internal/experiment/judge.go:340-351` — `requiredObservationCountForMaturity()` 實際回傳:
  - `level_1_exploratory`: **3**
  - `level_2_window_validated`: **8**
  - `level_3_regime_aware`: **12**
- 技能文件聲稱: `8/15/25`
- **差距**: 完全未實作

**影響**:
- 技能文件誤導開發者與 AI 代理，導致對系統成熟度的錯誤認知
- 若依技能文件規劃實驗，會低估所需資料量
- 文件與程式碼的信任鏈斷裂

**建議修復**:
1. **立即**: 更新技能文件，將門檻改為實際值 `3/8/12`，或標註「待實作 v2.0」
2. **短期**: 決定是否真的要升級至 `8/15/25` — 若需要，修改 `judge.go` 的門檻常數
3. **長期**: 建立文件與程式碼的同步檢查機制（如 CI 驗證技能文件中的數值是否與程式碼一致）

---

### 🔴 P2: 雙重權重系統 — AgentWeightManager 為死碼

**現象**: `internal/portfolio/` 下存在兩套權重管理系統，但僅有一套在運作。

**根因確認**:

| 系統 | 檔案 | 權重範圍 | 狀態 | 呼叫者 |
|------|------|---------|------|--------|
| **DarwinianWeightManager** | `darwinian_weights.go` | [0.3, 2.5] | ✅ **運作中** | `orchestrator/executors.go`, `system.go`, `plugin_control.go` |
| **AgentWeightManager** | `agent_weights.go` | [0.05, 0.40] | ❌ **死碼** | **無** — `NewAgentWeightManager()` 定義後從未被呼叫 |

- `AgentWeightManager` 擁有完整的 `RecordSignal()`, `UpdateWeights()`, `ApplyWeightsToRecommendations()` 方法，但從未被實例化或使用
- `DarwinianWeightManager` 是實際運作的系統，負責實驗流程中的權重調整

**影響**:
- 程式碼庫中存在 438 行無用程式碼，增加維護負擔
- 新開發者可能誤以為 AgentWeightManager 是活躍系統，導致錯誤修改
- 兩套系統的設計意圖（分數權重 vs 乘數權重）未被釐清

**建議修復**:
1. **立即**: 在 `agent_weights.go` 頂部加入 `// DEPRECATED: This package is not used in production. See darwinian_weights.go for active weight management.` 註解
2. **短期**: 決定是否移除 `agent_weights.go`（若確定不再使用）或將其整合進 Darwinian 系統
3. **長期**: 若保留，需明確定義其與 Darwinian 的職責邊界（例如：AgentWeightManager 用於單一 agent 層級，Darwinian 用於投資組合層級）

---

### 🟡 P3: Sharpe 穩定性檢查模組缺失

**現象**: 技能文件提及 `internal/experiment/sharpe_stability.go`，但該檔案不存在。

**根因確認**:
- `glob internal/experiment/sharpe_stability.go` → **無匹配**
- `judge.go` 中僅有 `improve_sharpe_like` 閘門名稱，無獨立的 Sharpe 穩定性檢查邏輯
- 無 TODO 或 stub 註解指向此功能

**影響**:
- 實驗評判僅依賴單一 Sharpe-like 指標，未檢查其穩定性（標準誤差）
- 高波動環境下的實驗可能被錯誤接受

**建議修復**:
1. **短期**: 決定是否需要此功能 — 若需要，建立 `sharpe_stability.go`，實作 `SharpeStabilityCheck(sharpeSeries []float64) (stable bool, stderr float64)`
2. **門檻建議**: 標準誤差 < 0.5（來自技能文件）
3. **整合點**: 在 `judge.go` 的 `passesAcceptance()` 中，於 `improve_sharpe_like` 閘門後加入穩定性檢查

---

### 🟡 P4: Out-of-Sample 驗證模組缺失

**現象**: 技能文件提及 `internal/experiment/oos_validator.go`，但該檔案不存在。

**根因確認**:
- `glob internal/experiment/oos_validator.go` → **無匹配**
- 實驗評判僅使用單一視窗資料，無第二獨立視窗驗證
- 無 TODO 或 stub 註解指向此功能

**影響**:
- 過擬合風險：實驗可能在特定視窗表現良好，但無法泛化
- 技能文件聲稱的「Candidate 必須在第二個獨立窗口優於 Baseline」未實作

**建議修復**:
1. **短期**: 建立 `oos_validator.go`，實作 `ValidateOutOfSample(candidate, baseline, secondWindow) (passed bool, improvement float64)`
2. **整合點**: 在 `judge.go` 的 `passesAcceptance()` 中，於主視窗評估後加入 OOS 驗證閘門
3. **視窗選擇**: 建議使用與主視窗不重疊的 30-90 天資料

---

### 🟡 P5: 模型命中率為靜態 Hardcoded 值

**現象**: 7 個投資模型的命中率（HitRate）為固定值，從未從實驗結果動態更新。

**根因確認**:
- `InvestmentModel` 結構體（`types.go:48-59`）**無 `HitRate` 欄位**
- `NarrativeEvent.HitRate` 來自 `hitRateForTheme()`（`ingestor.go:92-97`），從 `DefaultTemplates()` 查表取得
- 模板中的命中率為固定值：
  - `AI_capex_surge`: 0.81
  - `US_rates_up`: 0.72
  - `JPY_carry_unwind`: 0.68
  - `geopolitical_risk_spike`: 0.65
  - `oil_price_shock`: 0.58
- `EvaluateModels()` 計算 `RecentError` 並更新 `Weight`，但**不更新 HitRate**
- **無實驗結果 → 敘事模型的反饋迴路**

**影響**:
- 模型命中率無法反映實際市場環境變化
- 敘事引擎的決策品質受限於過時的歷史統計
- 實驗系統的學習成果未回流至敘事層

**建議修復**:
1. **短期**: 在 `InvestmentModel` 結構體中加入 `HitRate float64` 欄位
2. **中期**: 修改 `EvaluateModels()`，在計算 `RecentError` 的同時更新 `HitRate = 1.0 - RecentError`
3. **長期**: 建立實驗結果 → 敘事模型的反饋迴路，讓實驗的實際表現動態調整模型權重與命中率

---

### 🟡 P6: Darwinian 權重靜默夾制（Silent Clamping）

**現象**: 權重超出 [0.3, 2.5] 範圍時被靜默截斷，無日誌、無事件、無審計軌跡。

**根因確認**:
- `constrainWeight()`（`darwinian_weights.go:310-319`）:
  ```go
  func (m *DarwinianWeightManager) constrainWeight(weight float64) float64 {
      if weight < DarwinianWeightMin { return DarwinianWeightMin }
      if weight > DarwinianWeightMax { return DarwinianWeightMax }
      return weight
  }
  ```
- **無任何 logging** — `grep log` 於 `darwinian_weights.go` 無匹配
- `PerformDailyAdjustment()` 回傳的 `adjustments` map 僅包含 delta（new - old），無法從 delta 判斷是否發生夾制
- 測試覆蓋僅有邊界值測試（`darwinian_weights_test.go:174-192`），無整合場景測試

**影響**:
- 無法追蹤何時、哪些 agent 的權重被夾制
- 夾制可能掩蓋系統性問題（如某類 agent 持續表現極差/極好）
- 審計需求無法滿足（投資決策的權重調整必須可追蹤）

**建議修復**:
1. **立即**: 在 `constrainWeight()` 中加入日誌記錄：
   ```go
   if weight < DarwinianWeightMin {
       log.Printf("[Darwinian] Weight clamped to min: agent=%s, raw=%.4f, clamped=%.4f", agentID, weight, DarwinianWeightMin)
       return DarwinianWeightMin
   }
   ```
2. **短期**: 修改 `PerformDailyAdjustment()` 回傳值，加入夾制事件列表：
   ```go
   type AdjustmentResult struct {
       Adjustments map[string]float64
       ClampedEvents []ClampingEvent  // 新增
   }
   ```
3. **中期**: 在 Dashboard API 中暴露夾制事件查詢端點，供前端顯示警告

---

### 🟢 P7: MetaLearner 為死碼，未整合實驗流程

**現象**: `internal/metalearning/metalearner.go` 存在但從未被呼叫。

**根因確認**:
- `metalearner.go` 僅在自身檔案與測試檔案中被引用
- `NewMetaLearner()`, `Start()`, `SubmitSwarmData()` 無任何生產環境呼叫者
- 設計為「學習如何學習」的優化器，維護 20+ 策略族群，但從未接入實驗或 swarm 流程

**影響**:
- 796 行死碼，增加編譯時間與認知負荷
- 設計意圖（元學習優化）未實現

**建議修復**:
1. **立即**: 標記為 `// DEPRECATED: Not integrated into production flow`
2. **短期**: 決定是否保留 — 若保留，需設計整合點（如：在 `experiment/executor.go` 的 `Execute()` 中呼叫 `metaLearner.SubmitSwarmData()`）
3. **長期**: 若移除，需確保無其他文件引用此功能

---

## 修復優先順序總覽

| 優先級 | 問題 | 修復類型 | 預估工作量 |
|--------|------|---------|-----------|
| **P0 (本日)** | P6 靜默夾制 — 加日誌 | 簡單修改 | 30 分鐘 |
| **P1 (本週)** | P1 文件脫節 — 對齊或更新 | 文件修改 | 1 小時 |
| **P1 (本週)** | P2 AgentWeightManager 死碼 — 標記或移除 | 程式碼清理 | 1 小時 |
| **P2 (下週)** | P5 命中率靜態 — 加入動態更新 | 功能擴充 | 4 小時 |
| **P2 (下週)** | P3 Sharpe 穩定性 — 建立模組 | 新功能 | 6 小時 |
| **P2 (下週)** | P4 OOS 驗證 — 建立模組 | 新功能 | 6 小時 |
| **P3 (下月)** | P7 MetaLearner — 整合或移除 | 架構決策 | 8 小時 |

---

## 風險評估

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| 修復 P1 門檻時影響既有實驗結果 | 中 | 高 | 先備份 baseline_policy.json，逐步驗證 |
| 移除 P2 AgentWeightManager 影響未來整合 | 低 | 中 | 標記 deprecated 而非立即移除，保留 1 個版本 |
| P5 動態命中率引入不穩定 | 中 | 中 | 加入平滑機制（EMA），避免劇烈跳動 |
| P6 夾制日誌造成效能問題 | 低 | 低 | 使用結構化日誌，batch 寫入 |

---

## 下一步行動

1. **確認優先順序**: 請確認上述優先順序是否符合業務需求
2. **產出修復計劃**: 將本報告轉為可執行的 `plan.md`，包含具體檔案、行號、修改內容
3. **多代理執行**: 以並行代理方式執行各修復任務

---

*報告產出時間: 2026-04-25*
*診斷技能: /atlas-strategy-evolution v1.0*
