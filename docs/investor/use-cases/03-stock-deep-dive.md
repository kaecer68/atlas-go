# C3 - 個股深度 (Stock Deep Dive)

「為什麼這支股漲/跌?」從事件 + 敘事 + 法人找原因

## 投資人會問

- 「2330 今天為什麼漲?」
- 「最近有什麼消息?」
- 「法人最近動向?」
- 「這是技術反彈還是基本面?」

## 對應 tool

| Tool | 用途 | Tier |
|------|------|------|
| `stock_get_quote` | 報價基準線（比較漲跌） | 待 #1068 |
| `event_calendar` | 個股相關事件日曆 | 待 #1068 |
| `narrative_get_events` | 個股事件敘事 | 待 #1068 |
| `narrative_get_chains` | 因果鏈（事件 → 敘事） | 待 #1068 |
| `stock_get_chips` | 法人買賣超 | 待 #1068 |

## 下一步

- 詳細自然語言 → tool 範本：見 [`../query-examples.md`](../query-examples.md)（待 PR-2）
- 完整 tool schema：見 [`docs/reference/tool-catalog.md`](../../reference/tool-catalog.md)
