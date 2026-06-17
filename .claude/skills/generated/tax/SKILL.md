---
name: tax
description: "Skill for the Tax area of atlas. 23 symbols across 7 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Tax

23 symbols | 7 files | Cohesion: 94%

## When to Use

- Working with code in `internal/`
- Understanding how TestTaxAwareSizerReducesSize, TestTaxAwareSizerEdgeCases, TestTaxAwareSizerLotRounding work
- Modifying tax-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/tax/taiwan_tax.go` | NewTaiwanTaxCalculator, Config, CalculateDividendTax, CalculateTransactionTax, CalculatePositionTax (+1) |
| `internal/tax/tax_aware_sizing_test.go` | TestTaxAwareSizerReducesSize, TestTaxAwareSizerEdgeCases, TestTaxAwareSizerLotRounding, TestTaxAwareSizerBaseSizer, TestTaxAwareSizerWithCustomTaxRate |
| `internal/tax/taiwan_tax_test.go` | TestCalculateDividendTax, TestCalculateTransactionTax, TestCalculatePositionTax, TestCalculatePortfolioTax, TestConfig |
| `internal/tax/tax_aware_sizing.go` | NewTaxAwareSizer, SizePosition, BaseSizer |
| `internal/portfolio/sizing.go` | DefaultRiskParameters, NewSizer |
| `internal/sim/engine.go` | computeTaxAdjustedResults |
| `internal/domain/types.go` | DefaultTaiwanTaxConfig |

## Entry Points

Start here when exploring this area:

- **`TestTaxAwareSizerReducesSize`** (Function) — `internal/tax/tax_aware_sizing_test.go:9`
- **`TestTaxAwareSizerEdgeCases`** (Function) — `internal/tax/tax_aware_sizing_test.go:33`
- **`TestTaxAwareSizerLotRounding`** (Function) — `internal/tax/tax_aware_sizing_test.go:63`
- **`TestTaxAwareSizerBaseSizer`** (Function) — `internal/tax/tax_aware_sizing_test.go:83`
- **`TestTaxAwareSizerWithCustomTaxRate`** (Function) — `internal/tax/tax_aware_sizing_test.go:93`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestTaxAwareSizerReducesSize` | Function | `internal/tax/tax_aware_sizing_test.go` | 9 |
| `TestTaxAwareSizerEdgeCases` | Function | `internal/tax/tax_aware_sizing_test.go` | 33 |
| `TestTaxAwareSizerLotRounding` | Function | `internal/tax/tax_aware_sizing_test.go` | 63 |
| `TestTaxAwareSizerBaseSizer` | Function | `internal/tax/tax_aware_sizing_test.go` | 83 |
| `TestTaxAwareSizerWithCustomTaxRate` | Function | `internal/tax/tax_aware_sizing_test.go` | 93 |
| `NewTaxAwareSizer` | Function | `internal/tax/tax_aware_sizing.go` | 15 |
| `TestCalculateDividendTax` | Function | `internal/tax/taiwan_tax_test.go` | 9 |
| `TestCalculateTransactionTax` | Function | `internal/tax/taiwan_tax_test.go` | 33 |
| `TestCalculatePositionTax` | Function | `internal/tax/taiwan_tax_test.go` | 57 |
| `TestCalculatePortfolioTax` | Function | `internal/tax/taiwan_tax_test.go` | 131 |
| `TestConfig` | Function | `internal/tax/taiwan_tax_test.go` | 196 |
| `NewTaiwanTaxCalculator` | Function | `internal/tax/taiwan_tax.go` | 17 |
| `DefaultRiskParameters` | Function | `internal/portfolio/sizing.go` | 22 |
| `NewSizer` | Function | `internal/portfolio/sizing.go` | 53 |
| `DefaultTaiwanTaxConfig` | Function | `internal/domain/types.go` | 175 |
| `SizePosition` | Method | `internal/tax/tax_aware_sizing.go` | 26 |
| `BaseSizer` | Method | `internal/tax/tax_aware_sizing.go` | 42 |
| `Config` | Method | `internal/tax/taiwan_tax.go` | 22 |
| `CalculateDividendTax` | Method | `internal/tax/taiwan_tax.go` | 27 |
| `CalculateTransactionTax` | Method | `internal/tax/taiwan_tax.go` | 36 |

## How to Explore

1. `gitnexus_context({name: "TestTaxAwareSizerReducesSize"})` — see callers and callees
2. `gitnexus_query({query: "tax"})` — find related execution flows
3. Read key files listed above for implementation details
