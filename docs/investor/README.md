# atlas-go for Investors（散戶入口）

atlas-go 是給**台灣散戶**的模擬交易暨投資策略觀測輔助平台。產品定位全文：[`reference/product-positioning.md`](../reference/product-positioning.md)。

兩種使用方式，建議順序：

1. **網頁（先用，免設定）**——打開 `/client/` 儀表板：市場總覽、七維錢潮雷達（3+2+2 分層）看板、未來 5 日錢潮預測、產業地圖、投資心法與策略排名，全部白話呈現；七維語意以 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 為準。
2. **AI agent（進階）**——把 **atlas-mcp** 接進 Hermes / OpenClaw / Claude Desktop，讓 agent 用 110 個工具回答網頁上說不清楚的問題（例如「今天為什麼跌」「外資最近連買幾天」）。

## 8 Use Cases — Pick Yours

| # | Use Case | Primary Question | 網頁對應 | MCP |
|---|----------|-------------------|------|-----|
| 1 | [個股研究](use-cases/01-stock-research.md) | "2330 現在能不能買?" | 個股快查 | 待 [#1068](https://github.com/kaecer68/atlas-go/issues/1068) |
| 2 | [市場全景](use-cases/02-market-overview.md) | "現在大盤怎樣?" | 市場總覽首頁 | 待 #1068 |
| 3 | [個股深度](use-cases/03-stock-deep-dive.md) | "為什麼 2330 漲/跌?" | 個股快查 | 待 #1068 |
| 4 | [持倉健康](use-cases/04-portfolio-health.md) | "我的倉穩嗎?" | 組合持倉 | 待 #1068 |
| 5 | [策略排名](use-cases/05-strategy-ranking.md) | "哪個策略最好?" | 投資心法 | 待 #1068 |
| 6 | [資金流向](use-cases/06-capital-flow.md) | "主力在買什麼?" | 錢潮看板 | 待 #1068 |
| 7 | [每日晨報](use-cases/07-daily-briefing.md) | "今天要關注什麼?" | 市場總覽首頁 | 待 #1068 |
| 8 | [稅務規劃](use-cases/08-tax-planning.md) | "報稅怎麼算?" | 組合持倉稅務區塊 | 待 #1068 |

## Get Started：網頁

啟動 atlas 後打開瀏覽器：

```
http://localhost:18080/client/
```

首頁即市場總覽（建議方向、七維錢潮雷達 3+2+2、未來 5 日預測）；左側選單可進錢潮預測、產業地圖、投資心法、投資管線與組合持倉。註冊帳號後依 tier 解鎖更多區塊。

## Get Started：AI agent（進階）

告訴你的 AI agent：

> "請幫我安裝 atlas-mcp 並配置你的 MCP 設定"

或手動跑：

```bash
make setup-mcp-agent   # 自動安裝 + 設定 + 共用 dev key
make verify-mcp-setup  # 驗證 112 tools 連線成功
```

（詳見 [`cmd/atlas-mcp/README.md`](../../cmd/atlas-mcp/README.md)）

## Full Reference

- [`reference/tool-catalog.md`](../reference/tool-catalog.md) — 112 tool 完整 catalog
- [`query-examples.md`](query-examples.md) — 自然語言 → tool 對照 (~25 高頻範本)
