# atlas-mcp UX/UI Health Check — Manifest I：問題清單與驗收標準

> **生成日期**: 2026-07-26
> **審計來源**: Hermes Agent / OpenClaw 首次接入模擬測試
> **審計範圍**: 5 份引導文件、100+ MCP 工具、40+ 次實際呼叫
> **審計模型**: deepseek-v4-pro

## 分級說明
| 等級 | 定義 |
|------|------|
| 🔴 P0 | Agent 呼叫即報錯，用戶會直接感知「系統異常」 |
| 🟡 P1 | 引導／文件錯誤導致設定失敗或用戶困惑 |
| 🟠 P2 | 數據品質問題，agent 可能誤判系統狀態 |
| 🔵 P3 | UX 改善，提升散戶使用體驗 |

---

## 🔴 P0 — Critical Bugs（必須立即修復）

### P0-1: `stock_get_quote` client timeout
- **症狀**: `GET /api/stock/quote?symbol=2330` → `context deadline exceeded`
- **影響**: Hermes 查個股報價直接報錯
- **驗收**: `stock_get_quote("2330")` 3 次中有 2+ 次成功回傳報價
- **複現**: 100%（當前環境）

### P0-2: `llm_get_cost` 503 no KimiClient
- **症狀**: `GET /api/llm_annotator/cost` → `503: no KimiClient wired`
- **影響**: 查詢 LLM 成本永遠失敗
- **驗收**: `llm_get_cost()` 回傳實際成本數值，或明確的 "N/A" 訊息
- **複現**: 100%

### P0-3a: `detector_registry_list` JSON unmarshal error
- **症狀**: `json: cannot unmarshal array into Go value of type map[string]interface {}`
- **影響**: 完全無法查詢 detector 狀態
- **驗收**: `detector_registry_list()` 回傳 24 個 detector 的結構化清單
- **複現**: 100%

### P0-3b: `template_detector_status` JSON unmarshal + empty response
- **症狀**: `cannot unmarshal array into Go value of type map (raw=3 bytes)`
- **影響**: 完全無法查詢 detector scan 結果
- **驗收**: `template_detector_status()` 回傳最近 scan 的 DetectionResult
- **複現**: 100%

---

## 🟡 P1 — 文件與引導缺陷

### P1-1: Hermes `${ATLAS_API_KEY}` 變數未展開
- **症狀**: `~/.hermes/config.yaml` 中 `ATLAS_API_KEY: ${ATLAS_API_KEY}` 為 literal 字串
- **影響**: Hermes 無法以 API key 存取任何 admin endpoint
- **驗收**: `hermes mcp test atlas-mcp` 回傳正常工具數且 admin tools 可用
- **根因推測**: `setup-mcp-agent` 未顯式展開環境變數

### P1-2: Tool count 文件 4 處不一致
- **症狀**: 108-110 / 112 / 113+ / 116 / 118 五個數字
- **影響**: Agent 接入時無法確認設定是否正確
- **驗收**: 所有文件引用同一個數字，且 `make verify-mcp-setup` 能比對

### P1-3: 「registered N tools」訊息不存在
- **症狀**: `mcp-integration-local.md` §5 說會看到此訊息，但 stdout 沒有
- **影響**: 操作者無法完成文件描述的驗證步驟
- **驗收**: stdout 確實印出 tool count 或文件移除該描述

### P1-4: `event_calendar` / `calendar_events` 命名重複
- **症狀**: 兩個功能幾乎相同的 tool，名稱不同
- **影響**: Agent 不知道該用哪個
- **驗收**: 合併為一個或明確標記 deprecated

---

## 🟠 P2 — 數據品質／Agent 誤判風險

### P2-1: 40+ unacknowledged alerts 無過濾
- **症狀**: `alert_list_unacknowledged()` 回傳大量 simulation/experiment warning
- **影響**: Agent 誤報「系統嚴重異常」
- **驗收**: 提供 `severity` / `rule` filter 參數；已知重複告警 dedup 後 ≤ 10 條

### P2-2: `risk_get_drawdown` 回 "not available"
- **症狀**: 無 drawdown 數據
- **影響**: Agent 誤判風險模組缺失
- **驗收**: 回傳有意義的狀態（含原因說明）

### P2-3: 15 個 channel "unknown" status
- **症狀**: `data_get_channels()` 回傳 15 個 `status: "unknown"` 的通道
- **影響**: Agent 看到大量「異常」通道
- **驗收**: 未接線的通道標記為 `disabled` 而非 `unknown`

### P2-4: Simulation 持續 0 訂單 spam alert
- **症狀**: 每天 8-12 條 `"場次 session-*-daily 產生 0 筆訂單"` warning
- **影響**: Alert 淹沒於無意義的噪音
- **驗收**: 0 訂單不產生 WARNING alert（至少降級為 INFO）

### P2-5: `scheduler_get_status` 無優先級／無摘要
- **症狀**: 70+ 任務清單，`next_run: 0001-01-01` 的任務混雜其中
- **影響**: Agent 無法識別重要任務狀態
- **驗收**: 提供 `enabled` filter + summary（幾條 running / pending / disabled）

---

## 🔵 P3 — UX 改善

### P3-1: First Contact SOP 缺失
- **現狀**: Agent 接入後不知道先調用哪些工具
- **驗收**: `AGENT_QUICKSTART.md` 新增 "首次 3 call" 段落

### P3-2: 13 個 sector 無中文標籤
- **現狀**: `display_zh` 顯示英文 ID（ai_supply_chain 等）
- **驗收**: 所有 sector 有完整中文名稱

### P3-3: `parameters_get` 無摘要模式
- **現狀**: 回傳 20KB+ flat map
- **驗收**: 提供 category filter 或 summary 層級

### P3-4: `explain_market_move` emoji 渲染不穩定
- **現狀**: 回傳含 📉 emoji
- **驗收**: 可選是否含 emoji（`?format=emoji|plain`）

---

## 驗收執行 SOP

```bash
# 1. 重建全 binary
make rebuild-all && make check-binaries

# 2. P0 驗證
ATLAS_BASE_URL=http://127.0.0.1:18080 bin/atlas-mcp 2>&1 | head -3

# 3. 各 tool 逐一驗證（見 §二 的測試矩陣）
#    stock_get_quote / llm_get_cost / detector_registry_list / template_detector_status
#    必須全部 PASS

# 4. P1 驗證
hermes mcp test atlas-mcp    # 必須列出工具且 admin tools 可用
grep -r "tool.*count\|registered.*tools\|總計" docs/ --include="*.md" | sort

# 5. P2 驗證
#    alert_list_unacknowledged → alerts ≤ 10
#    data_get_channels → unknown channels = 0
#    scheduler_get_status → 無 0001-01-01 next_run 的 enabled task

# 6. 全測試矩陣重跑
go test -race ./cmd/atlas-mcp/... && go test -race ./internal/mcp/...
```
