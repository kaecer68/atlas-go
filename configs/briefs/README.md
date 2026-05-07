# 預設實驗 Brief

本目錄包含 5 個經過篩選的預設實驗 brief，用於「人機協同」頁面的快速執行。

## Brief 清單

| 文件名 | 目標 Agent | 變異類型 | 目的 |
|--------|-----------|---------|------|
| `semiconductor-tightening.json` | 半導體產業桌 | prompt_tightening | 緊縮弱量設置過濾，保留探索性覆蓋 |
| `technical-breakout-expansion.json` | AI 供應鏈產業桌 | prompt_expansion | 擴大可接受的突破參與度 |
| `financials-credit-quality.json` | 金融產業桌 | prompt_tightening | 改善信貸品質過濾 |
| `etf-rotation-liquidity.json` | ETF 輪動 | prompt_tightening | 改善中等流動性名稱處理 |
| `value-yield-defensive.json` | ETF 輪動 | prompt_expansion | 防禦性價值/收益變異 |

## 使用方法

在「人機協同」頁面的「執行實驗」卡片中，從下拉選單選擇 brief，點擊「執行」。

## 自訂 Brief

若要新增自訂 brief，請複製本目錄中的任意 JSON 文件，修改以下欄位：
- `proposal_id`: 唯一識別碼
- `target_agent_id`: 目標 Agent ID（參考 `configs/agents.json`）
- `target_skill`: 目標 Skill
- `hypothesis`: 你的假設
- `failure_pattern`: 觀察到的失敗模式

將新文件放入 `configs/briefs/` 目錄，重新啟動 atlas 即可使用。
