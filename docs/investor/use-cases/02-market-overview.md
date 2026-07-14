# C2 - 市場全景 (Market Overview)

跨市場 + regime + 宏觀指標，看「現在大盤怎樣?」

## 投資人會問

- 「現在市場狀態?」
- 「美股昨晚怎樣?」
- 「大盤方向?」
- 「現在是 risk-on 還是 risk-off?」

## 對應 tool

| Tool | 用途 | Tier |
|------|------|------|
| `macro_get_snapshot_latest` | 一次拿到當前宏觀快照（GDP、CPI、利率、匯率） | 待 #1068 |
| `regime_get_history` | 市場 regime 歷史（risk-on / risk-off / neutral） | 待 #1068 |
| `crossmarket_get_status` | 跨市場總體狀態（美股、台股、陸股） | 待 #1068 |
| `crossmarket_get_us_indices` | 美股主要指數（NASDAQ、S&P500、道瓊） | 待 #1068 |
| `macro_get_capital_flow_latest` | 跨市場資金流向 | 待 #1068 |

## 下一步

- 詳細自然語言 → tool 範本：見 [`../query-examples.md`](../query-examples.md)（待 PR-2）
- 完整 tool schema：見 [`docs/reference/tool-catalog.md`](../../reference/tool-catalog.md)
