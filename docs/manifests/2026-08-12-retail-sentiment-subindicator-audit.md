# Audit Manifest: 散戶情緒子指標明細假數字盤查

> **Audit source**: kaecer 觀察 `/client/retail_sentiment` 子指標明細數值可疑（維持率 -0.30 / 當沖 0.27 / 融資餘額 0.00 / VIX 0.30 / PCR 0.90 / 零股 0.50 / 期貨 OI 0.50 / 券商 0.50 / ETF 0.00 / Part D 0.850），要求盤查是否硬編碼。
> **Goal**: 逐項確認 9 個子指標 + Part D 乘數的數值來源（真實資料 / fallback / 硬編碼），揭露前端是否誤導。
> **Scope**: 唯讀盤查（Phase A）。不修改任何程式碼。修復與否由 kaecer 決定後另開 Phase B。
> **Created**: 2026-08-12
> **Status**: Phase A 完成，等待修復決策

---

## Invariant Tracker

| ID | Problem | Root Cause | Files | Acceptance | Status | Notes |
|----|---------|-----------|-------|-----------|--------|-------|
| A01 | 子指標明細顯示的數值混合真實計算與資料缺失 fallback，前端無法區分 | fetcher channel 失敗時 calculator 回傳 fallback 值（0.5/0/0.0），`IsFallback` 標記存在但前端 `retail-sentiment-panel.js` 完全不渲染 | `internal/retail/rsi_tw_calculator.go`、`shared_web/static/js/shared/retail-sentiment-panel.js` | 前端對 fallback 子指標顯示「資料缺失」標記 | accepted | curl 實測 fetcher_status：taifex/odd_lot/etf = error |
| A02 | subC1 散戶期貨 OI 硬編碼 fallback 0.5（非參數化） | `rsi_tw_calculator.go` subC1：`if pct == 0 { score = 0.5 }` — literal，與其他 fallback（A5/A6 走 params）不一致 | `internal/retail/rsi_tw_calculator.go:subC1` | C1 fallback 改走 `params.C1FallbackScore` 或明確標記 | accepted | 用戶看到的 0.50 = taifex error → RetailFuturesPct=0 → literal 0.5 |
| A03 | subC3 ETF 申購分數永遠 0（資料源已死） | `twse_etf` channel circuit breaker open（known_issue `twse_etf_upstream_60d`：TWT44U 已移除 → 404），`netSub==0` → return 0；`docs/guides/retail-sentiment.md` 已註記 | `internal/retail/rsi_tw_calculator.go:subC3`、`internal/monitoring/known_issues.go` | 前端顯示「資料源已移除」而非 0.00 | accepted | B03 audit（2026-08-10）已記錄；ETF 欄位永遠 0 屬已知但前端未標示 |
| A04 | Part D 乘數 0.850 與「無觸發事件」矛盾 | `active_events` 從未被 handler 填充（`convertRSITwSubIndicators` 只填 AdjustmentFactor/DMultiplier），但 D1（地緣政治）乘數可被觸發（geoRisk > 0.5 → ×0.85） | `internal/monitoring/api/system/handlers.go:convertRSITwSubIndicators` | D 觸發事件與乘數一致顯示 | accepted | 用戶看到 0.850 + 無觸發事件 = D1 觸發但 UI 無事件文字 |
| A05 | 融資餘額 Z-score 0.00 為 fallback（history 不足或 std==0） | subA1：`len(history)<2` 或 `std==0` → ZScore=0, IsFallback=true。Calculator singleton 的 marginHistory 累積實際資料，但首次/常數序列時為 0 | `internal/retail/rsi_tw_calculator.go:subA1` | 前端標記「歷史不足」 | accepted | curl 實測 margin_balance_z=0 且 category_a.is_fallback=true |
| A06 | guide 過時：`handlers.go:204 GeopoliticalRisk: 0` 已不復存在 | `docs/guides/retail-sentiment.md` 描述舊代碼；實際 `HandleRetailSentiment` 已改用 `h.GeopoliticalRiskFetcher`（geoRisk 從 provider 讀） | `docs/guides/retail-sentiment.md` | guide 與代碼一致 | accepted | curl 實測 geopolitical_risk=ok，Part D 乘數可達 0.85 |
| A07 | **C2 單位錯配（鐵證）**：`ForeignInvestorNet.Value` 單位是「億股」（`twse_capital_flow_provider.go:258` `totalForeign/1e8`，T86 row[4] 為股數），但 `subC2` 的 `C2NetflowScalingFactor=1e9` 假設「TWD 元」（rationale 明寫 1B TWD） | netFlow=5.67 億股 ÷ 1e9 ≈ 5.67e-9 → score 恆等 0.5（實測 0.5000000057）→ C2 完全無鑑別力 | `internal/retail/rsi_tw_calculator.go:subC2`、`internal/config/defaults_engine.go:C2NetflowScalingFactor`、`internal/marketdata/twse_capital_flow_provider.go:258` | C2 使用正確單位的流量分數（金額 TWD 或歷史 Z-score） | accepted | 實測 `broker_flow_score: 0.5000000056738098` |
| A08 | **C2 資料源語義錯誤**：T86 row[4] 為「股數」非「金額」；金融工程標準以金額（TWD）衡量法人買賣超，股數受股價水準扭曲 | provider 註解自承 column 4 = 外陸資買賣超**股數** | `internal/marketdata/twse_capital_flow_provider.go` | 使用金額或 Z-score（需 web 驗證 T86 是否有金額欄位） | hypothesis | 待 web 驗證 TWSE T86 schema |
| A09 | **橫向重複造輪子**：散戶情緒計算存在三套並行實作（retail.Calculator / FactorBridge.computeRetailSentiment / capitalflow ForceRetail Z-score），FactorBridge 標註 ForceRetail 為「canonical, unified source」但 retail 未接入 | 架構分散，修 A 壞 B 風險高 | `internal/retail/rsi_tw_calculator.go`、`internal/portfolio/factor_bridge.go:SetForceRetailZScore`、`internal/capitalflow/forces.go` | 統一入口或明確 delegation | accepted | factor_bridge.go 註解 |
| A10 | **A4/A5 方向與指數語義矛盾**：Part A 高分 = 狂熱（frenzy），但 A4 VIX 高 → +1.0（max fear）、A5 PCR 高 → +0.9（very bearish），兩者把恐慌訊號推高狂熱分數 | `A4VixScores`/`A5PcrScores` 映射方向與 composite 語義（+frenzy/-fear）相反 | `internal/retail/rsi_tw_calculator.go:subA4,subA5`、`defaults_engine.go:A4VixScores,A5PcrScores` | A4/A5 恐慌訊號推低（負向）分數 | hypothesis | 待 web 驗證台灣 PCR/VIX 語義 |
| A11 | **A3 名稱誤導**：「維持率 Z-score」實際是「融資餘額歷史百分位」映射 `(percentile-0.5)*2`，非維持率（margin maintenance ratio）；`MarginMaintenanceRatio` 欄位存在但 TWSE MI_MARGN 不提供（provider 註解自承） | 命名與實質不符；前端標題誤導 | `internal/retail/rsi_tw_calculator.go:subA3`、`shared_web/static/js/shared/retail-sentiment-panel.js`、`internal/marketdata/twse_margin_provider.go:fetchMaintenanceRatio` | 改名為融資餘額百分位或接真實維持率 | accepted | provider 註解「expected to remain empty until a suitable data source」 |
| A12 | **A2 誤用 Z-score 命名**：`subA2` 直接以 `DayTrading.VolumeRatio`（原始比率 0.268）當 Z-score，非標準化 | 命名誤導；數值本身真實（day_trading ok） | `internal/retail/rsi_tw_calculator.go:subA2` | 正名或標準化 | accepted | 實測 0.2683 |
| A13 | **A5 PCR 單位錯配（鐵證）**：TAIFEX `PutCallVolumeRatio%` = `'110.43'`（百分比），`parseFloat64` 後 110.43 進 `subA5`，threshold 是比率（1.5/1.0/0.8）→ 110.43 > 1.5 **永遠成立** → taifex 有資料時 PCR 永遠映射 0.9（very bearish） | `internal/marketdata/taifex_provider.go` 未做 /100 轉換；`defaults_engine.go:A5PcrThresholds` 單位假設錯誤 | `internal/marketdata/taifex_provider.go:FetchPCR`、`internal/retail/rsi_tw_calculator.go:subA5`、`internal/config/defaults_engine.go` | PCR 以比率（1.1043）進計算 | accepted | 用戶看到 0.90 = taifex 正常時；curl 當下 taifex error → 0.5 fallback |
| A14 | **A6 零股失衡計算方法缺陷**：`twse_oddlot_provider.go` 用 `close > open` 猜買賣方向（heuristic）彙總 buy/sell → ImbalanceRatio 非真實買賣失衡 | BFI84U 為盤後零股成交明細，無買賣方向欄位；provider 以漲跌猜方向不合理 | `internal/marketdata/twse_oddlot_provider.go:121-140` | 改用含買賣方向的資料源（盤中零股買賣超）或標記 heuristic | accepted | 且 odd_lot fetch error → 目前 fallback 0.5 |
| A15 | **C1 量級與 threshold 不匹配**：實測 `RetailFuturesPct = RetailLongPct - RetailShortPct ≈ -0.9%`（前十大 OI 佔市場 60-70%），threshold 20/10/-10/-20 幾乎永遠不觸發 → C1 即使資料正常也恆等 0.5 | `taifex_provider.go` 的「散戶 = 市場 - 前十大」定義使 RetailLongPct/RetailShortPct 都接近 30-40%，相減量級小 | `internal/marketdata/taifex_provider.go:FetchRetailFuturesOI`、`internal/retail/rsi_tw_calculator.go:subC1` | 改用正確散戶期貨指標（小台指散戶多空比或外資淨多單反向） | accepted | 實測樣本 diff=-0.9%；MacroMicro 定義小台散戶多空比 = -1×三大法人未平倉/全體 |

---

## Phase Tracker

### Phase A — Audit (read-only) ✅

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 追蹤前端→API→計算鏈路 | - | accepted | `retail_sentiment.js` → `GET /api/dashboard/retail-sentiment` → `internal/monitoring/api/system/handlers.go:HandleRetailSentiment` → `internal/retail/rsi_tw_calculator.go:ComputeFinal` |
| 驗證每個子指標數值來源 | A01-A06 | accepted | curl `localhost:18080` 實測數值與代碼路徑逐一對應（見下表） |
| 確認 fetcher channel 真實狀態 | A01/A03 | accepted | channel-health：taifex_daily error / twse_oddlot ok（但 handler error，schema changed known_issue）/ twse_etf error (circuit breaker) |
| 建立 audit manifest | - | done | 本文件 |

### 子指標逐一驗證（Phase A 證據）

| 顯示欄位 | curl 實測值 | 代碼路徑 | 判定 |
|---|---|---|---|
| 維持率 Z-score | -0.299 | `subA3` = `(percentile-0.5)*2`，percentile=0.351 真實計算 | ✅ 真實（來自 macro snapshot 歷史百分位） |
| 當沖 Z-score | 0.268 | `subA2` = `DayTrading.VolumeRatio`，day_trading channel ok | ✅ 真實 |
| 融資餘額 Z-score | 0.00 | `subA1` history 不足/std==0 → fallback 0 | ❌ fallback（A05） |
| VIX 風險分數 | 0.30 | `subA4` = `vixMapParam`（15<VIX≤20 → 0.3），VIX 真實 | ✅ 真實 |
| 週選擇權 PCR | 0.90（用戶）/ 0.50（curl） | `subA5`：pcr==0 → A5PcrFallback=0.5；pcr>1.5 → 0.9。taifex error 時 fallback | ⚠️ 依 channel 狀態（taifex error → 0.5 fallback） |
| 零股交易失衡 | 0.50 | `subA6`：ImbalanceRatio=0（odd_lot fetch error）→ A6OddLotFallback=0.5 | ❌ fallback |
| 散戶期貨 OI | 0.50 | `subC1`：RetailFuturesPct=0（taifex error）→ **literal 0.5** | ❌ 硬編碼 fallback（A02） |
| 券商分點流向 | 0.5000000057 | `subC2`：netFlow=5.67e8 → 0.5 + 5.67e8/1e9 → 0.5000000057（真實計算但無鑑別力） | ⚠️ 名義真實，實質近中性 |
| ETF 申購分數 | 0.00 | `subC3`：netSub=0（twse_etf circuit breaker）→ return 0 | ❌ 資料源已死（A03） |
| Part D 乘數 | 0.850（用戶）/ 1.0（curl） | `factorD1`：geoRisk>0.5 → ×0.85；「無觸發事件」因 active_events 永為 null | ⚠️ 乘數真實，UI 事件文字誤導（A04） |

A Score 驗證（用戶版本）：subA2 0.268×0.20 + subA3 -0.299×0.20 + subA4 0.30×0.15 + subA5 0.90×0.10 + subA6 0.50×0.10 = 0.0537 - 0.0598 + 0.045 + 0.09 + 0.05 = 0.179 ✅ 吻合（subA1 fallback 0）
C Score 驗證：subC1 0.5×0.40 + subC2 0.5000000057×0.35 + subC3 0×0.25 = 0.2 + 0.175 + 0 = 0.375 ✅ 吻合
最終 Score（curl 版本）：(0.1389×0.40 + 0.375×0.25)×1.0 = 0.1493 ✅ 與 API `sentiment_score: 0.1493` 一致

### Phase B — Plan ✅（2026-08-12）

| ID | 修復方案 | Acceptance Criteria | Status |
|----|---------|---------------------|--------|
| A13 | `taifex_provider.go:FetchPCR` 將 `PutCallVolumeRatio`/`PutCallOIRatio` 除以 100（TAIFEX 回傳百分比 110.43 → 比率 1.1043），threshold 維持 1.5/1.0/0.8 | `subA5` 測試：pcr=1.1043 → score 0.7（bearish）；pcr=0.5 → 0.1（bullish）；不再恆等 0.9 | planned |
| A07+A08 | `subC2` scaling 修正：`C2NetflowScalingFactor` 由 1e9（假設 TWD 元）改為匹配「億股」量級（±10 → 鑑別力）；rationale 更新；前端標題「券商分點流向」→「外資+投信淨買超」 | 測試：netFlow=5 億股 → score ≠ 0.5（有鑑別力）；netFlow=-8 → score < 0.5 | planned |
| A15 | `subC1` threshold 匹配實際量級（RetailFuturesPct ≈ ±10）：`C1VeryBullishThreshold` 20→5、`C1BullishThreshold` 10→2、`C1BearishThreshold` -10→-2、`C1VeryBearishThreshold` -20→-5；正名註解「非前十大多空差」；小台散戶多空比列 backlog | 測試：diff=3% → 0.7；diff=-3% → 0.25（不再恆等 0.5） | planned |
| A02 | `subC1` fallback 參數化：新增 `C1FallbackScore` param（0.5），移除 literal | 測試：pct==0 → 走 param | planned |
| A10 | `A4VixScores`/`A5PcrScores` 改負向：恐慌訊號（VIX 高、PCR 高）推低分數（與 composite 語義 +frenzy/-fear 一致）：A4 scores [-0.1,-0.3,-0.5,-0.7,-0.85,-1.0]（低 VIX 輕負/高 VIX 恐慌）……保留低 VIX 微正或全負需再校準；A5 scores [-0.9,-0.7,-0.5,-0.1] | 測試：VIX=40 → subA4 貢獻 < 0；pcr=2.0 → subA5 貢獻 < 0 | planned |
| A11 | 前端「維持率 Z-score」→「融資餘額百分位」；`subA3` 註解同步 | UI 文字修正 | planned |
| A12 | 前端「當沖 Z-score」→「當沖比率」 | UI 文字修正 | planned |
| A03 | 前端 ETF 申購欄位顯示「資料源已移除」badge（非 0.00） | UI 顯示 badge | planned |
| A04 | `convertRSITwSubIndicators` 填充 `catD.ActiveEvents`（D1/D2/D3 觸發 → 事件文字） | API 測試：geoRisk>0.5 → active_events 含「地緣政治風險」 | planned |
| A01 | 前端渲染 `is_fallback`：fallback 子指標顯示「資料缺失」標記 | UI 顯示標記 | planned |
| A05 | 前端 `margin_balance_z` fallback（history 不足）顯示「歷史不足」 | UI 標記 | planned |
| A09 | 橫向統一：retail.Calculator 與 FactorBridge/capitalflow 的散戶情緒入口統一——本輪僅記錄（避免修 A 壞 B），列 backlog | 不變更 | deferred |
| A06 | guide 更新（GeopoliticalRisk 已接線、C1/C2 修正、A3 命名） | guide 與代碼一致 | planned |
| A14 | 零股 buy/sell heuristic 標記為已知限制（無買賣方向資料源）；channel 修復屬 B01 另開 PR | 註解標記 | planned |

### Phase C — Implement（進行中）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| （見 todo 逐項） | A13→A15→A02→A10→A11/A12/A03/A04/A01/A05 | in-progress | - |

### Phase D — Close Out

（未開始）

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B01 | `twse_oddlot` channel health=ok 但 handler fetch error（schema changed，known_issue `twse_oddlot_upstream_60d`）→ 零股失衡永遠 fallback | 2026-08-12 | 修復 upstream parser 或標記已知 |
| B02 | `taifex_daily` 連線逾時（openapi.taifex.com.tw context deadline exceeded）→ PCR + 期貨 OI 同時失效 | 2026-08-12 | channel 層修復 |
| B03 | `twse_etf` circuit breaker open（TWT44U 移除）→ ETF 欄位永久 0 | 2026-08-12 | 已在 known_issues 追蹤，前端需標示 |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- PR body must reference this manifest: `See docs/manifests/2026-08-12-retail-sentiment-subindicator-audit.md`

---

## Session-End State

- **Done this session**: Phase A（A01-A06 全部 accepted，唯讀盤查）
- **Remaining**: Phase B/C/D（修復與否待 kaecer 決策）
- **Next action**: 向 kaecer 回報盤查結論，等待修復範圍決定
- **Uncommitted code**: 無（唯讀）
- **Branch / PR**: `kaecer68/fix-retail_sentiment_subindex_detail` / 無
- **Paused because**: 等待修復決策

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-12 | 1.0 | Initial manifest | agent |
