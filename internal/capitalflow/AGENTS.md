# AGENTS.md — finance cluster

> 合併 `capitalflow` / `eventdriven` / `recommender` / `subscription` 的模組陷阱。完整 API 與流程見 `docs/`。

---

## capitalflow（資金流）

模組職責：台股 **七維錢潮雷達（3+2+2 分層）** 日內流量 + 共振強度計算 + API 輸出。

> **分類語意權威**：`docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 / §6。
>  - 官方法人 `official_actor`：外資 / 投信 / 自營商（T86 第一方）。
>  - 行為代理 `behavioral_proxy`：官股 / 散戶（proxy）。
>  - 領先／跨市場訊號 `positioning_indicator` + `cross_market_signal`：期貨 / TSM ADR。
>  - actor 共識只計算官方actor；行為代理與訊號層不影響機構共識敘事。
>  - `weight_deprecated=true` 為 0；不進入 UI / 不影響自動化。

| 陷阱 | 說明 |
|------|------|
| **共振計算公式變更** | `ComputeResonance`（`resonance.go`）若改，需同步 `parameters.json` 並呼叫 SelfCalibrate 重新校準；actor 共識只計入官方actor 三項。 |
| **TWSE 假日不發布** | 週末/假日無資料；`Service.Refresh` 已內建 `IsTaiwanTradingDay` guard（skip-and-log，CF-INV-16），不會在非交易日寫入樣本；前端如有獨立補圖邏輯，仍應 fallback 至上週五資料。上週五若也無資料（e.g. 伺服器初次啟動），handler 回 `data_available: false`（不補 0）。 |
| **PublicBank 欄位歷史較短** | 公股行庫資料 TWSE 約 2018+ 才完整；早期資料空值（data_available=false），**不補 0**。 |
| **Service 為 pipeline 入口** | `Service.LatestDaily` / `Service.Summary` 可直接被 `internal/recommender` adapter 呼叫，繞過 `Handler` 需 `*http.Request` 的限制。 |

---

## eventdriven（事件預測）

模組職責：5 日事件驅動資金流預測（ETF 換股 / MSCI 調整 / 月營收 / 季底作帳 / 國定假日）。

| 陷阱 | 說明 |
|------|------|
| **假日效應 lag** | 假日前/後一日的特殊流動模式需要 historical window ≥ 3 年才穩定，目前可能未達。 |
| **MSCI pre-positioning** | 公告當日才反映，但 smart money 通常前一週就 position；可考慮加上 pre-window。 |
| **月營收解盲差** | 電子/傳產/金融的營收截止日不同，需用 calendar 區分產業別。 |
| **Confidence 範圍** | 預測信心度為 `(0.5, 1.0]`；計算細節見 [`docs/specs/eventdriven-spec.md`](../../docs/specs/eventdriven-spec.md)。 |

---

## recommender（推薦 API）

模組職責：為 `/api/recommendations` 提供 tier-aware 投資建議（Free / Registered / Premium）。

| 陷阱 | 說明 |
|------|------|
| **NIL deps 回 hardcoded fallback** | `Narrative` / `CapitalFlow` / `EventPredictor` / `ComparisonEngine` 任一為 nil 時回傳安全值（如 `Regime=NEUTRAL`、`Score=0`），不會 panic。 |
| **X-User-Email 僅 dev mode** | Production 預設 401（`devMode=false`）；dev mode 需 `ATLAS_DEV_MODE=true` 且必須透過 `config.GetSecret()` 讀取，**不可直接 `os.Getenv`**。 |
| **`lastSeenRegime` race** | regime-change listener 的 `lastSeenRegime` 讀寫無 mutex；並發請求可能丟/多觸發。 |
| **StrategyScoreInfo 暫為 float** | `ComparisonEngine.GetScore()` 只回 float；`EntrySignal` / `StopLoss` 目前由 handler 模板寫死。 |

---

## subscription（認證與 tier）

模組職責：使用者註冊/登入 + JWT 簽發/驗證 + tier 解析。

| 陷阱 | 說明 |
|------|------|
| **JWT secret 環境變數** | Production 必須設置 `ATLAS_JWT_SECRET`；dev mode 未設定時會降級為 unsign token（不安全）。 |
| **Trial 試用期** | `Premium` 有試用期邏輯，到期自動降級；`EffectiveTier` 處理這層。 |
| **無 rate limit** | `Register` endpoint 沒有爆破防護；生產部署需 reverse-proxy 層補。 |
| **Store 只接收已 hash 密碼** | `Store.Register` 接收 `passwordHash`；呼叫端必須先用 argon2/bcrypt hash，store 再 bcrypt 二次 hash。 |

---

## 整合速查

- `capitalflow.Service` → `recommender` adapter 直接呼叫。
- `eventdriven.Predictor` → `recommender` adapter 的 `PredictToday()` / `NextNDays()`。
- `subscription.EffectiveTier()` → `recommender` tier 判定；`subscription.JWTManager` → MCP auth。
- 路由註冊：`cmd/atlas/main.go` 的 `capitalflow.RegisterRoutes`、`eventdriven.RegisterRoutes`、`subHandler.RegisterRoutes`、`recommender.RegisterRoutesWithDeps`。

---

## 測試

- `capitalflow`：HandleDaily / HandleSummary 回應格式、`dimension_role` 完整性（官方actor / 行為代理 / 訊號）、共振範圍 `[0.5, 1.5]`。
- `eventdriven`：事件 → flow 映射、confidence 範圍、calendar edge cases。
- `recommender`：handler_test.go（13 tests）、e2e_test.go、adapters_test.go（nil-safety）。
- `subscription`：handler_test.go（Register/Login/ExtractToken/Verify）、trial expiry tests。
