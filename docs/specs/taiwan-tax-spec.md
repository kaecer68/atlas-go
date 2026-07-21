# Taiwan Tax Calculation 規格

> **文件角色**：atlas-go 台灣股市稅務計算（股息稅、證交稅）規格。
> **取代對象**：原 internal/tax/AGENTS.md（已遷移至此）。

**成熟度**: evolving  
**模組職責**: 台灣股市稅務計算（股息所得稅、證交稅）與稅後部位規模調整。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `TaiwanTaxCalculator` | `taiwan_tax.go` | 計算股息稅（28%）與證交稅（賣出方 0.3%） |
| `TaxAwareSizer` | `tax_aware_sizing.go` | 以稅率調整後的資本計算買入股數，確保總成本在預算內 |
| `TaxConfig` | `internal/domain` | 稅率配置（DividendTaxRate、TransactionTaxRate、NHISurchargeRate、IncludeNHI） |
| `NHISurchargeRate` | `taiwan_tax.go` | 常數 0.0211，從預設 `DividendTaxRate` (0.28) 拆分出的二代健保補充保費（實際費率 2.11%） |
| `TaxSnapshot` | `internal/domain` | 單一標的稅務快照（含稅後損益） |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **證交稅只收賣方** | `CalculateTransactionTax()` 僅在賣出時課徵，買入無稅。 |
| **零值回傳空 snapshot** | `CalculatePositionTax()` 在 `Quantity <= 0` 或 `sellPrice <= 0` 時回傳零值 TaxSnapshot（非錯誤）。 |
| **TaxAwareSizer 取整到 1000 股** | 台股以「張」為單位（1 張 = 1000 股），`SizePosition()` 會向下取整到千股倍數。 |
| **有效資本公式** | `effectiveCapital = capital / (1 + transactionTaxRate)`，預留證交稅空間。 |
| **PortfolioTax fallback 到 CurrentPrice** | `CalculatePortfolioTax()` 在 sellPrices 缺少某標的時，使用 `pos.CurrentPrice` 作為賣出價。 |
| **IncludeNHI 控制 NHI 補充費** | `effectiveDividendTaxRate()` 在 `cfg.IncludeNHI=false` 時回傳 `DividendTaxRate - NHISurchargeRate`（預設 0.28−0.0211≈0.2589）。`CalculateDividendTax` 與 `TaxSnapshot` 共用此 helper。預設 `IncludeNHI=true` 行為不變。 |
| **稅率參數透過 ParametersConfig** | `config.GetParametersConfig().Tax.ToConfig()` 是 production 中 tax config 的權威來源，禁止在 composition.go 或 handlers.go 硬編碼 `domain.DefaultTaiwanTaxConfig()`（已被 P2-7 移除）。 |

---

## 測試

```bash
go test ./internal/tax/...
```

涵蓋股息稅計算、證交稅計算、部位稅務快照、投組彙總、TaxAwareSizer 取整邏輯。
