# C4 - 持倉健康 (Portfolio Health)

「我的倉穩嗎?」看策略 + 風險 + 歸因

## 投資人會問

- 「我的策略還活著嗎?」
- 「portfolio 風險?」
- 「上個月歸因?」
- 「Darwinian 權重穩定嗎?」

## 對應 tool

| Tool | 用途 | Tier |
|------|------|------|
| `strategy_list_active` | 線上策略清單 | 待 #1068 |
| `strategy_get_summary` | 策略摘要（勝率、Sharpe） | 待 #1068 |
| `strategy_get_attribution` | 策略歸因 | 待 #1068 |
| `risk_get_metrics` | 風險指標（VaR、波動率） | 待 #1068 |
| `risk_get_drawdown` | 最大回撤 | 待 #1068 |
| `synergy_get_darwinian_status` | Darwinian 權重狀態 | 待 #1068 |

## 下一步

- 詳細自然語言 → tool 範本：見 [`../query-examples.md`](../query-examples.md)（待 PR-2）
- 完整 tool schema：見 [`docs/reference/tool-catalog.md`](../../reference/tool-catalog.md)
