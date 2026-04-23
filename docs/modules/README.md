# 模組操作手冊

本目錄包含 atlas-go v1.3 新增模組的操作手冊。

## 模組列表

| 模組 | 說明 | 路徑 |
|------|------|------|
| [Screening Layer](screening.md) | 宣告式個股篩選 | `internal/screener/` |
| [FactorEngine](factor-engine.md) | 多因子評分引擎 | `internal/portfolio/factor_engine.go` |
| [Tax Module](tax.md) | 台灣稅務計算 | `internal/tax/` |
| [Capital Management](capital-management.md) | 階段式資金管理 | `internal/risk/` |
| [Alert System](alert-system.md) | 即時警報通知 | `internal/monitoring/` |

## 快速開始

1. 先閱讀 [Screening Layer](screening.md) 了解篩選邏輯
2. 參考 [FactorEngine](factor-engine.md) 了解評分機制
3. 根據需要配置 [Tax](tax.md)、[Capital](capital-management.md)、[Alert](alert-system.md)
