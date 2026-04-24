# Atlas Strategy Evolution Skill

**版本**: 1.0  
**日期**: 2026-04-23  
**職責**: 投資模型的動態調整、績效回饋與持續進化  

---

## 核心哲學

> **「沒有永遠有效的投資模型，只有持續進化的策略系統」**

---

## 投資模型生態系

### 現有模型

| 模型ID | 名稱 | 適用情境 | 權重範圍 |
|--------|------|---------|---------|
| `ai_supercycle_model` | AI超級週期模型 | 結構性科技趨勢強勁 | 0.1 - 0.6 |
| `hawkish_fed_model` | 鷹派Fed模型 | 高利率+緊縮環境 | 0.1 - 0.5 |
| `geopolitical_hedge_model` | 地緣政治避險模型 | 戰爭/地緣風險升溫 | 0.1 - 0.6 |
| `semiconductor_cycle_model` | 半導體週期模型 | 半導體庫存週期 | 0.1 - 0.4 |
| `seasonal_model` | 季節性輪動模型 | 春節/除權息等季節效應 | 0.0 - 0.3 |

### 模型權重動態調整

```go
func EvolveModelWeights(macro MacroRiskAssessment, trends []StructuralTrend) {
    switch macro.Level {
    case Green:
        weights["ai_supercycle_model"] = 0.6
        weights["hawkish_fed_model"] = 0.2
        weights["geopolitical_hedge_model"] = 0.2
        
    case Yellow:
        if trends["AI"].Strength > 70 {
            weights["ai_supercycle_model"] = 0.5
            weights["hawkish_fed_model"] = 0.3
        } else {
            weights["ai_supercycle_model"] = 0.3
            weights["hawkish_fed_model"] = 0.4
        }
        
    case Orange/Red:
        weights["ai_supercycle_model"] = 0.1
        weights["geopolitical_hedge_model"] = 0.6
        weights["hawkish_fed_model"] = 0.3
    }
}
```

---

## 績效回饋機制

### 評估維度

```go
type ModelPerformance struct {
    ModelID       string
    Period        string
    PredictedFlow string    // 模型預測的資金流向
    ActualFlow    string    // 實際資金流向
    Accuracy      float64   // 預測準確率
    PnLImpact     float64   // 對組合損益的影響
}
```

### 自動調整規則

```go
func EvaluateModels(performances []ModelPerformance) {
    for _, p := range performances {
        if p.Accuracy < 0.5 {
            ReduceModelWeight(p.ModelID, 0.1)
        } else if p.Accuracy > 0.8 {
            IncreaseModelWeight(p.ModelID, 0.1)
        }
    }
}
```

---

## 實驗生命週期

```
Propose → Execute → Judge → Promote/Revert
```

### 改良重點（v2.0）

1. **觀測值門檻提高**：level_1: 3→8, level_2: 8→15, level_3: 12→25
2. **統計穩定性檢查**：Sharpe ratio 標準誤差 < 0.5
3. **Out-of-sample 驗證**：Candidate 必須在第二個獨立窗口優於 Baseline
4. **透明度強化**：Darwinian 權重截斷事件記錄 + 連續截斷警告

---

## 關鍵檔案

- `internal/experiment/judge.go` - 實驗評判核心
- `internal/experiment/sharpe_stability.go` - Sharpe穩定性檢查
- `internal/experiment/oos_validator.go` - Out-of-sample驗證
- `internal/portfolio/darwinian_weights.go` - Darwinian權重管理
- `internal/narrative/knowledge_base.go` - 投資模型定義

---

*技能版本: 1.0*  
*最後更新: 2026-04-23*
