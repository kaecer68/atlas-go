# Tax Module 操作手冊

## 概述

Tax Module 處理台灣股市稅務計算，包含股利稅和交易稅。

## 稅率設定

### 預設稅率（台灣）

| 稅種 | 稅率 | 說明 |
|------|------|------|
| 股利所得稅 | 28% | 單一稅率，分離課稅 |
| 證券交易稅 | 0.3% | 賣方負擔 |

### 配置方式

```go
import "github.com/kaecer68/atlas-go/internal/domain"

cfg := domain.TaxConfig{
    DividendTaxRate:    0.28,
    TransactionTaxRate: 0.003,
    IncludeNHI:         true,  // 預設 true：包含二代健保補充保費
}

calculator := tax.NewTaiwanTaxCalculator(cfg)
```

### IncludeNHI（2026-06-05 起生效）

`TaxConfig.IncludeNHI` 控制股利稅率是否含二代健保補充保費：

| IncludeNHI | 有效股利稅率（預設） | 說明 |
|------------|----------------------|------|
| `true`（預設） | `0.28` | 28% 已內含 2.11% 二代健保補充保費 |
| `false` | `0.2589` | 28% − 2.11% = 25.89%，用於非居住民帳戶、雇主配息、情境分析等場景 |

實作：`taiwan_tax.go:NHISurchargeRate` (const 0.0211) 與 `effectiveDividendTaxRate()` helper；`CalculateDividendTax` 與 `TaxSnapshot` 計算皆透過此 helper。詳見 [`docs/specs/taiwan-tax-spec.md`](../specs/taiwan-tax.md)（Wave 11 從原 tax 模組的 AGENTS.md 抽離）。

## 使用方法

### 計算股利稅

```go
divTax := calculator.CalculateDividendTax(10000.0)
// 結果: 2800.0 (10000 * 0.28)
```

### 計算交易稅

```go
txTax := calculator.CalculateTransactionTax(50000.0)
// 結果: 150.0 (50000 * 0.003)
```

### 計算部位稅務

```go
snapshot := calculator.CalculatePositionTax(position, sellPrice, dividendReceived)
```

## TaxAwareSizer

自動考慮稅務的倉位計算器：

```go
baseSizer := portfolio.NewSizer(...)
taxSizer := tax.NewTaxAwareSizer(baseSizer, cfg)

shares := taxSizer.SizePosition(symbol, price, capital, conviction)
```

## 注意事項

1. **零值和負值輸入**會返回 0，不會報錯
2. 交易稅僅在**賣出時**計算
3. TaxAwareSizer 會**減少建議倉位**以預留稅款

## 測試

```bash
go test ./internal/tax/... -v
go test ./internal/tax/... -cover
```
