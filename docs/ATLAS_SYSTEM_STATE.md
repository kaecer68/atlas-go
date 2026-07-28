# ATLAS 系統狀態快照

> 最後更新：2026-07-28
> 維護紀律：每次 feature wave 合併後更新，維持現狀可追蹤性。

## 活躍工作區

| 工作區 | Branch | 狀態 | 說明 |
|--------|--------|------|------|
| `~/workspace/atlas` | `main` | 🟢 基準 | 主工作區，已合併 E4/E6a |
| `~/workspace/atlas/E6a-home-dedup` | `kaecer68/E6a-home-dedup` | ⚪ 待清理 | E6a 已完成合併，worktree 待移除 |
| `~/workspace/atlas/MoneyTren-E5` | `feat/20260727-strategy-three-category` | 🟡 進行中 | E5a 策略三分類 |
| `~/workspace/atlas/MoneyTrend-B4b` | `kaecer68/MoneyTrend-B4b` | 🟡 進行中 | B4b 成交量資料源接入 |
## Feature Wave 進度

| Wave | 描述 | 狀態 | PR | 合併日期 |
|------|------|------|-----|----------|
| E4 | 方法論：七時期 UI + 因果傳導鏈頁面 | ✅ 已完成 | #1397, #1398 | 2026-07-27 |
| E5a | 策略三分類（防禦/攻擊/戰術） | 🟡 進行中 | — | — |
| B4b | 成交量資料源接入（market_volume channel） | 🟡 進行中 | #1405 | — |

## E4 — 方法論頁面（已完成）

- **PR #1397**: `feat(methodology): add 七時期 UI + 因果傳導鏈頁面 (E4)`
- **PR #1398**: `fix(methodology): E4 頁面審查修復`
- **涵蓋**：七時期判斷條件展示、因果傳導鏈視覺化、方法論憲章對齊
- **前端**：`client_web/static/js/page-shells/methodology.js`
- **後端**：`internal/methodology/`、`internal/config/methodology_config.go`
- **已知限制**：無

## E5a — 策略三分類（進行中）

- **目標**：策略分類為防禦/攻擊/戰術三軸，前端 CSS variables 已定義（`--category-defensive` / `--category-aggressive` / `--category-tactical`）
- **工作區**：`~/workspace/atlas/MoneyTren-E5`，branch `feat/20260727-strategy-three-category`
- **涉及檔案**：`internal/config/methodology_config.go`、`internal/methodology/advisor.go`、`internal/dailyreport/report.go`、`cmd/atlas/main.go`、`shared_web/static/js/page-shells/methodology.js`、`shared_web/static/css/base/variables.css`
- **注意**：E6a spec 紅線禁止觸碰 E5a 的六個檔案

## E6a — 首頁去重與資訊架構重整（已完成）

- **PR #1401**: `feat(home-dedup): E6a 首頁去重 + 側邊欄修正 + 專頁入口`
- **Phase 0 結論**：`/api/events/calendar` 與 `/api/dashboard/calendar-events` 不同源 → 方案 B（兩區塊保留，標題區分）
- **涵蓋**：
  - 首頁去重：主軌加入口連結、tier 軌移除重複 fetch/區塊（-116 行）
  - 側邊欄：移除死連結、+3 入口、5 項更名、補 page-performance-report 容器
  - 測試更新：route-deep-link.spec.ts page title 預期值、移除過時 prediction-card 測試
- **前端**（7 檔案）：`client_web/static/index.html`、`client_web/static/js/main.js`、`client_web/static/js/components/home-tier-sections.js`、`shared_web/static/js/pages/home.js`、`shared_web/static/css/pages/home.css`、`client_web/tests/route-deep-link.spec.ts`、`client_web/tests/home-capital-flow.spec.ts`

- **已知限制（留給 E6b）**：趨勢表達（專頁視覺化強化）、事件日曆後端合併（P2 cleanup）

## B4b — 成交量資料源接入（進行中）

- **PR #1405**: `feat: 新增 market_volume channel（集中市場成交金額）`
- **目標**：從 TWSE MI_INDEX?type=MS 抓取集中市場大盤統計成交金額，換算為億元後填入 `MacroDataSnapshot.MarketVolume`
- **端點**：`www.twse.com.tw/exchangeReport/MI_INDEX?type=MS`
- **換算**：成交金額（元）/ 100,000,000 = 億元
- **Provider**：`internal/marketdata/market_volume_provider.go`
- **Adapter**：`internal/apigateway/adapter_market_volume.go`
- **註冊**：`internal/apigateway/register_adapters.go`、`gateway.go` channelIDs()
- **測試**：5 個單元測試（正常/空陣列/stat 異常/格式異常/非數值金額）
- **已知限制**：TWSE MI_INDEX 每日盤後更新（~14:00-15:00）；非交易日 7 天 fallback

## 更名對照（E6a 生效）

| data-page | 原名 | 新名 |
|-----------|------|------|
| home | 市場總覽 | 今日判讀 |
| capital_predictions | 錢潮預測 | 未來 5 日資金 |
| capital_board | 錢潮看板 | 七大勢力看板 |
| narrative | 宏觀視野 | 全球宏觀 |
| methodology | 方法論 | 方法論：為什麼 |
