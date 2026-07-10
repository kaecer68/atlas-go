# C1 - 個股研究 (Stock Research)

從報價、估值、籌碼、技術面 4 個角度看「我該買這支股嗎?」

## 投資人會問

- 「2330 現在多少?」
- 「2330 本益比高嗎?」
- 「今天外資在買 2330 嗎?」
- 「2330 是不是超買?」

## 對應 tool

| Tool | 用途 | Tier |
|------|------|------|
| `stock_get_quote` | 報價（last、change、change_pct、open、high、low、volume） | 待 #1068 |
| `stock_get_fundamentals` | 估值（PE、PB、PS、DividendYield、Sector） | 待 #1068 |
| `stock_get_chips` | 籌碼（外資、投信、自營商淨買賣超） | 待 #1068 |
| `stock_get_technical` | 技術（SMA20、SMA50、RSI14、volume） | 待 #1068 |

## 下一步

- 詳細自然語言 → tool 範本：見 [`../query-examples.md`](../query-examples.md)（待 PR-2）
- 完整 tool schema：見 [`../api-reference.md`](../api-reference.md)
