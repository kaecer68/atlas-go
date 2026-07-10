# atlas-go for Investors

atlas-go is a Taiwan stock investment research system. Through **atlas-mcp** (Model Context Protocol), AI agents like Hermes / OpenClaw can answer your investment questions by querying 91 tools that cover quotes, fundamentals, market regime, portfolio risk, and strategy performance.

## 8 Use Cases — Pick Yours

| # | Use Case | Primary Question | Tier |
|---|----------|-------------------|------|
| 1 | [個股研究](use-cases/01-stock-research.md) | "2330 現在能不能買?" | 待 [#1068](https://github.com/kaecer68/atlas-go/issues/1068) |
| 2 | [市場全景](use-cases/02-market-overview.md) | "現在大盤怎樣?" | 待 #1068 |
| 3 | [個股深度](use-cases/03-stock-deep-dive.md) | "為什麼 2330 漲/跌?" | 待 #1068 |
| 4 | [持倉健康](use-cases/04-portfolio-health.md) | "我的倉穩嗎?" | 待 #1068 |
| 5 | [策略排名](use-cases/05-strategy-ranking.md) | "哪個策略最好?" | 待 #1068 |
| 6 | [資金流向](use-cases/06-capital-flow.md) | "主力在買什麼?" | 待 #1068 |
| 7 | [每日晨報](use-cases/07-daily-briefing.md) | "今天要關注什麼?" | 待 #1068 |
| 8 | [稅務規劃](use-cases/08-tax-planning.md) | "報稅怎麼算?" | 待 #1068 |

## Get Started

告訴你的 AI agent：

> "請幫我安裝 atlas-mcp 並配置你的 MCP 設定"

或手動跑：

```bash
make setup-mcp-agent   # 自動安裝 + 設定 + 共用 dev key
make verify-mcp-setup  # 驗證 91 tools 連線成功
```

（詳見 [`cmd/atlas-mcp/README.md`](../../cmd/atlas-mcp/README.md)）

## Full Reference

- [`REFERENCE/tool-catalog.md`](REFERENCE/tool-catalog.md) — 91 tool 完整 catalog
- [`query-examples.md`](query-examples.md) — 自然語言 → tool 對照 (20-30 高頻範本，待 PR-2)
- [`tier-guide.md`](tier-guide.md) — public / registered / premium 差異 *(待 [#1068](https://github.com/kaecer68/atlas-go/issues/1068) 商業化確認)*
