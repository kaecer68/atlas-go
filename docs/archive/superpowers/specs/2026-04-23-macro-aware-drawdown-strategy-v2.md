# Atlas 智能回撤與投資策略系統設計文件

**版本**: v2.0 - Macro-Aware Edition  
**日期**: 2026-04-23  
**狀態**: 設計階段（等待批准）  

---

## 一、設計哲學轉變

### 從 v1.0 到 v2.0 的核心進化

| 維度 | v1.0（原始設計） | v2.0（宏觀感知設計） |
|------|----------------|-------------------|
| **觸發依據** | 組合回撤百分比（15%/25%/35%） | 宏觀風險等級 + 結構性趨勢強度 |
| **行動邏輯** | 固定減倉/清倉 | 動態調整，允許結構性豁免 |
| **資金運用** | 持有現金為主 | 切換賽道（產業輪動）|
| **目標** | 防止極端虧損 | 在保護資本的同時，捕捉結構性機會 |

### 核心洞察

> **台股2020-2026年上漲155%，關鍵不是「不虧損」，而是「在對的時候承擔對的風險」**

- 2022年俄烏戰爭：應該大幅回撤（系統性風險）
- 2024年外資大賣超：不應回撤（AI結構性趨勢戰勝宏觀）
- 2026年美伊戰爭：應該切換至能源股（產業輪動，非現金為王）

---

## 二、三層決策架構

### 2.1 第一層：宏觀風險評估（Macro Risk Assessment）

**輸入**：六大維度（美元、美債、日圓、匯率、商品、地緣政治）
**輸出**：`MacroRiskLevel`（綠/黃/橙/紅）+ `CapitalFlowDirection`

```go
type MacroRiskAssessment struct {
    Level              MacroRiskLevel    // Green/Yellow/Orange/Red
    ForeignOutflowProb float64           // 0-100%
    PrimaryFlow        string            // "risk_off" / "sector_rotation" / "carry_trade_unwind"
    FavoredSectors     []string          // 資金可能流向
    AvoidedSectors     []string          // 資金可能逃離
    StructuralOverride bool              // 是否有結構性豁免條件
    Confidence         float64           // 評估置信度
}
```

**評估規則**：

| 風險等級 | 觸發條件 | 外資出逃機率 | 歷史案例 |
|---------|---------|------------|---------|
| **綠色** | 無重大風險信號 | <20% | 2023年（AI啟動+降息預期）|
| **黃色** | 單一風險信號 | 20-50% | 2024年（美元高利率但AI強勁）|
| **橙色** | 雙重風險信號 | 50-80% | 2025/4（川普關稅威脅）|
| **紅色** | 系統性風險 | >80% | 2022/2（俄烏+升息）、2024/8（Carry Trade unwind）|

### 2.2 第二層：結構性趨勢評估（Structural Trend Assessment）

**核心問題**：當前是否有強到足以戰勝宏觀逆風的產業趨勢？

**評估指標**：

```go
type StructuralTrend struct {
    Theme           string    // "AI_supercycle", "semiconductor_cycle", "energy_transition"
    Strength        float64   // 0-100（基於營收增速、資本支出、訂單能見度）
    LeadingIndicator string   // 領先指標（如台積電營收YoY）
    Threshold       float64   // 豁免閾值（如台積電YoY > 50%）
    HistoricalProof []string  // 歷史驗證案例
}
```

**豁免條件**：

```
IF MacroRiskLevel = Yellow/Orange
   AND 台積電營收YoY > 50%
   AND AI資本支出預期上調（CSP廠商上修CapEx）
   AND 內資承接力道強（投信買超 > 外資賣超）:
    
    StructuralOverride = true
    行動 = 維持核心AI持倉，僅降低邊際倉位
    歷史驗證 = 2024年（外資賣超6951億，台股漲28%）
```

### 2.3 第三層：動態回撤執行（Dynamic Drawdown Execution）

**整合前兩層輸出，決定具體行動**：

```go
type DrawdownDecision struct {
    Action          string    // "hold" / "reduce" / "rotate" / "liquidate"
    MaxPositionSize float64   // 單檔上限（動態調整）
    CashTarget      float64   // 現金目標比例
    SectorRotation  []string  // 切換目標板塊（若Action="rotate"）
    Rationale       string    // 決策理由（可審計）
}
```

**決策矩陣**：

| 宏觀風險 | 結構性趨勢 | 決策 | 單檔上限 | 現金目標 | 行動 |
|---------|-----------|------|---------|---------|------|
| 綠色 | 強 | **HOLD** | 22% | 8% | 維持正常策略 |
| 黃色 | 強 | **REDUCE** | 15% | 15% | 降低邊際倉位，保留核心 |
| 黃色 | 弱 | **ROTATE** | 15% | 20% | 切換至防禦/受益板塊 |
| 橙色 | 強 | **REDUCE** | 12% | 25% | 顯著降倉，但保留核心 |
| 橙色 | 弱 | **ROTATE** | 10% | 30% | 大幅切換至防禦 |
| 紅色 | 任何 | **LIQUIDATE** | 0% | 50%+ | 強制清倉或極低倉位 |

---

## 三、情境化案例

### 情境A：系統性風險（2022年俄烏戰爭重現）

**宏觀信號**：
- 歐洲戰爭（俄烏）
- 油價暴漲（+50%）
- 黃金大漲（+15%）
- Fed激進升息（0→5.25%）
- 10Y-2Y倒掛

**系統推導**：
```
MacroRiskLevel = RED（系統性風險）
ForeignOutflowProb = 90%+
PrimaryFlow = "risk_off"
StructuralOverride = false（無AI超級週期，當時AI尚未爆發）
```

**決策**：
```
Action = LIQUIDATE
MaxPositionSize = 0%（或極低5%）
CashTarget = 70%
Rationale = "系統性風險：戰爭+升息+衰退預警，無結構性趨勢可抵銷"
```

### 情境B：結構性趨勢戰勝宏觀（2024年實況）

**宏觀信號**：
- 美元維持高利率（5.25%）
- 外資大賣超（6,951億，史上第2大）
- 地緣政治緊張（以哈戰爭）

**結構性信號**：
- 台積電營收YoY +40%
- AI資本支出超預期（輝達、微軟、Google上修CapEx）
- 內資強力承接（投信買超8,320億）

**系統推導**：
```
MacroRiskLevel = YELLOW（單一風險：高利率）
ForeignOutflowProb = 50%
PrimaryFlow = "sector_rotation"
StructuralOverride = true（AI超級週期強度=85/100）
```

**決策**：
```
Action = REDUCE（非清倉）
MaxPositionSize = 18%（略降）
CashTarget = 12%（略增）
SectorRotation = []（維持AI持倉，不輪動）
Rationale = "宏觀風險存在但結構性趨勢更強：台積電營收+40%，內資承接8,320億，歷史證明外資賣超可被內資抵銷"
```

### 情境C：產業輪動（2026年美伊戰爭）

**宏觀信號**：
- 中東戰爭（美伊）
- 油價暴漲（破100美元）
- 黃金**不漲反跌**（資金湧入能源期貨）

**系統推導**：
```
MacroRiskLevel = ORANGE（雙重風險：戰爭+通脹）
ForeignOutflowProb = 60%
PrimaryFlow = "sector_rotation"（非全面risk_off）
FavoredSectors = ["energy", "shipping", "alternative_energy"]
AvoidedSectors = ["high_valuation_tech"]
```

**決策**：
```
Action = ROTATE（切換賽道，非現金為王）
MaxPositionSize = 15%
CashTarget = 20%
SectorRotation = ["台塑化", "航運股", "太陽能"]
Rationale = "中東戰爭引發能源危機：油價暴漲+黃金失效，資金湧入能源板塊。AI股短期承壓但中長期趨勢不變，故部分切換而非全面清倉"
```

---

## 四、系統架構

### 4.1 新增/修改的檔案

```
internal/
├── narrative/
│   ├── macro_assessment.go          # NEW: 宏觀風險評估引擎
│   ├── capital_flow_inference.go    # NEW: 資金流向推導
│   └── context_aware_template.go    # NEW: 情境感知因果模板
├── risk/
│   ├── drawdown_guard.go            # MODIFY: 整合宏觀風險
│   └── macro_aware_drawdown.go      # NEW: 宏觀感知回撤決策器
├── portfolio/
│   ├── sector_rotator.go            # NEW: 產業輪動執行器
│   └── dynamic_position_sizer.go    # NEW: 動態倉位調整
└── orchestrator/
    └── strategy_evolver.go          # NEW: 投資策略進化器
```

### 4.2 資料流

```
每日開盤前：
┌─────────────────────────────────────────────────────────────┐
│  MacroIngestor                                               │
│  ├── 讀取六大維度數據（美元、美債、日圓、匯率、商品、地緣）    │
│  └── 輸出：MacroDataSnapshot                                  │
└──────────────────────────┬──────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│  MacroRiskAssessmentEngine                                   │
│  ├── 計算外資出逃機率（0-100%）                               │
│  ├── 判斷風險等級（綠/黃/橙/紅）                              │
│  └── 輸出：MacroRiskAssessment                                │
└──────────────────────────┬──────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│  StructuralTrendEngine                                       │
│  ├── 評估AI/半導體/能源趨勢強度                               │
│  ├── 檢查豁免條件（台積電營收、CapEx預期）                    │
│  └── 輸出：StructuralOverride（true/false）                   │
└──────────────────────────┬──────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│  MacroAwareDrawdownEngine                                    │
│  ├── 整合宏觀風險 + 結構性趨勢                                │
│  ├── 查詢決策矩陣（Hold/Reduce/Rotate/Liquidate）            │
│  └── 輸出：DrawdownDecision                                   │
└──────────────────────────┬──────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│  StrategyEvolver                                             │
│  ├── 若Action=Rotate：執行產業輪動                            │
│  ├── 若Action=Reduce：調降倉位上限                            │
│  ├── 若Action=Liquidate：強制清倉                            │
│  └── 輸出：PortfolioAdjustment                                │
└─────────────────────────────────────────────────────────────┘
```

---

## 五、投資模型進化機制

### 5.1 模型權重動態調整

現有模型（`knowledge_base.go`）：
- `hawkish_fed_model`（鷹派Fed模型）
- `ai_supercycle_model`（AI超級週期模型）
- `geopolitical_hedge_model`（地緣政治避險模型）

**進化邏輯**：

```go
func EvolveModelWeights(macro MacroRiskAssessment, trends []StructuralTrend) {
    // 基於宏觀環境調整模型權重
    switch macro.Level {
    case Green:
        weights["ai_supercycle_model"] = 0.6
        weights["hawkish_fed_model"] = 0.2
        weights["geopolitical_hedge_model"] = 0.2
        
    case Yellow:
        if trends["AI"].Strength > 70 {
            // 結構性趨勢強，維持AI模型主導
            weights["ai_supercycle_model"] = 0.5
            weights["hawkish_fed_model"] = 0.3
        } else {
            // 無強趨勢，增加防禦權重
            weights["ai_supercycle_model"] = 0.3
            weights["hawkish_fed_model"] = 0.4
        }
        
    case Orange/Red:
        // 高風險環境，避險模型主導
        weights["ai_supercycle_model"] = 0.1
        weights["geopolitical_hedge_model"] = 0.6
        weights["hawkish_fed_model"] = 0.3
    }
}
```

### 5.2 模型績效回饋

```go
type ModelPerformance struct {
    ModelID       string
    Period        string
    PredictedFlow string    // 模型預測的資金流向
    ActualFlow    string    // 實際資金流向
    Accuracy      float64   // 預測準確率
    PnLImpact     float64   // 對組合損益的影響
}

// 每月評估模型績效，自動調整權重
func EvaluateModels(performances []ModelPerformance) {
    for _, p := range performances {
        if p.Accuracy < 0.5 {
            // 準確率低，降低權重
            ReduceModelWeight(p.ModelID, 0.1)
        } else if p.Accuracy > 0.8 {
            // 準確率高，增加權重
            IncreaseModelWeight(p.ModelID, 0.1)
        }
    }
}
```

---

## 六、驗收標準

### 6.1 功能驗收

- [ ] 輸入2022/2數據（俄烏戰爭），系統輸出紅色風險+清倉建議
- [ ] 輸入2024全年數據（外資大賣超+AI強勁），系統輸出黃色風險+減倉但保留核心
- [ ] 輸入2026/3數據（美伊戰爭+油價暴漲），系統輸出橙色風險+切換至能源股
- [ ] 輸入2024/8/5數據（日圓Carry Trade unwind），系統輸出紅色風險+清倉建議

### 6.2 效能驗收

- [ ] 宏觀風險評估延遲 < 100ms
- [ ] 每日批次處理時間 < 5分鐘
- [ ] 系統可處理並發的多情境模擬

### 6.3 稽核驗收

- [ ] 每個決策都有完整的 `Rationale` 記錄
- [ ] 模型權重調整歷史可追溯
- [ ] 結構性豁免條件觸發記錄完整

---

## 七、實施優先順序

| 優先級 | 項目 | 預估時間 | 依賴 |
|--------|------|---------|------|
| P0 | MacroRiskAssessmentEngine | 3天 | 無 |
| P0 | StructuralTrendEngine | 2天 | 無 |
| P1 | MacroAwareDrawdownEngine | 3天 | P0 |
| P1 | SectorRotator | 2天 | P0 |
| P2 | StrategyEvolver | 2天 | P1 |
| P2 | ModelPerformanceFeedback | 2天 | P1 |
| P3 | Dashboard整合 | 2天 | P2 |

---

*設計文件版本: 2.0*  
*最後更新: 2026-04-23*  
*狀態: 等待用戶審查與批准*
