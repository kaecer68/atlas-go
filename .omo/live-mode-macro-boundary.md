# Live Mode 與 Macro Pipeline 整合邊界設計

**Issue**: #424（`Live mode 與 macro pipeline 整合邊界設計`，2026-06-08 開，無 label）
**前置稽核**: PR #421（`docs/live-mode-us-market-audit.md`，2026-06-08 merged）
**日期**: 2026-06-20
**狀態**: 設計文件，**不立即實作**；僅鎖定整合邊界、給 Phase 4 production trading 前的健壯性提升當入口
**設計哲學**: Live mode 維持「精簡依賴、清單驅動、零外部訊號」

---

## 零、目的

#424 開立時提出 4 個中長期問題：regime 偵測、敘事事件觸發下單、跨市場相關性、circuit breaker 整合。本文件目的：

1. 把 4 個問題收斂成明確決策（做 / 不做 / 何時做）
2. 記錄 live mode 與 macro pipeline 的**整合邊界契約**（contract）
3. 避免 Phase 4 production trading 重啟同樣辯論

**非目標**: 本文件不改任何 Go / JS / TS 程式碼。

---

## 一、現況盤查（自 #424 開立 12 天後）

### 1.1 Live mode 自身的基礎建設（已具備）

| 元件 | 檔案 | 觸發源 | 與 macro 關係 |
|------|------|--------|---------------|
| `inferRegime()` | `internal/live/agent_runner.go` | context agent 評分 + 事件流 | **零** — 只用 `live/store` 事件 |
| `RiskGate.SetHaltTrading(bool)` | `internal/live/risk_gate.go:74` | 外部觸發（P&L 風控、連續停損） | **零** — 不接 macro 風險指標 |
| `CircuitState` (normal/paused/halted) | `internal/live/circuit_breaker.go` | `daily_loss 2.0%` / `drawdown 3.0%` / `consecutive_sl 3` | **零** — 純 P&L 規則 |
| `orchestrator.circuitBreaker` | `internal/live/orchestrator.go:27` | P&L 事件 | **零** |
| `marketData.GetQuotes(ctx, t, watchlist)` | `internal/live/orchestrator.go:461` | 唯一市場資料來源 | **零** — 個股報價，非 macro |

### 1.2 Macro 端已上線的相關能力（live 可選擇性引用）

| 元件 | 檔案 | 用途 | live 引用狀態 |
|------|------|------|-------------|
| 7 個 US market channel | `internal/marketdata/us_index_provider.go`、`us_tech_provider.go`、`tsm_adr_provider.go` + PR #416 | SPX / NDX / DJI / SOX / NVDA / AAPL / MSFT / TSM 即時報價 | **零匹配** |
| `MacroDataProvider.FetchSnapshot` | `internal/marketdata/macro_provider.go` | BDI / VIX / DXY / USDTWD / US10Y 等 macro 指標 | **零匹配** |
| `internal/narrative/`（Wave 5+） | `internal/narrative/*` | 敘事事件生成、calibration、JSONL 儲存 | **零匹配**（已接到 monitoring / llm_annotator） |
| 8 個 narrative calibration 子模組 | `internal/narrative/calibration_*` | regime 校準、信度評分 | **零匹配** |

### 1.3 已驗證前提（PR #421 + 本次再驗）

```bash
$ grep -rn "MacroDataProvider\|FetchSnapshot\|MacroDataSnapshot" internal/live/
（無輸出）

$ grep -rn "us_spx\|us_ndx\|us_dji\|us_nvda\|us_aapl\|us_msft\|tsm_adr" internal/live/
（無輸出）

$ grep -rln "narrative" internal/live/
（無輸出）

$ git log --all --oneline --grep="live.*macro\|live.*integration\|live.*regime\|live.*narrative"
9410cdd docs(audit): live mode integration check for US market channels (#421)
```

**結論**: 12 天內 Wave 4~8 把 narrative / llm_annotator / monitoring 灌了 1 萬多行新功能，但**沒有任何 commit 動到 `internal/live/` 與 macro 整合的契約**。live 維持純 watchlist 驅動。

---

## 二、4 個問題的決策矩陣

### Q1. Regime 偵測：Live mode 是否應使用 macro snapshot 判斷 regime？

**現況**: live 已有 `inferRegime()` → `RegimeRiskOn / RiskOff / Neutral` 三態。**但只讀 `live/store` 事件，不讀 macro**。`internal/narrative/calibration_regime.go` 有 macro-based regime 校準，但沒接到 live。

| 選項 | 內容 | 改動面 | 風險 | 結論 |
|------|------|--------|------|------|
| A. 維持不變 | live regime 仍由 context agent + 事件流驅動 | 0 | — | ✅ **當前採用** |
| B. 雙源融合 | live regime = max(agent_score, macro_regime_score) | `internal/live/agent_runner.go` 新增 `macroRegimeProvider` 介面；新增測試 mock | medium：regime 翻轉可能觸發既有 position 行為改變 | ❌ 不建議（與「精簡依賴」衝突） |
| C. Macro 覆寫 | macro regime 為最終裁決 | 同 B，但 source of truth 換成 macro | high：把 macro 變成 live 必須依賴的單點 | ❌ 拒絕 |

**決策**: **A**。Live mode 不讀 macro regime。
**觸發重評估的條件**: Phase 4 production trading 上線後 6 個月內，若 live 表現顯著低於 backtest 且歸因分析指向 regime 誤判，再啟動 B。

---

### Q2. 敘事事件觸發下單：narrative events 是否應在 live mode 觸發停損/停利？

**現況**: `internal/narrative/` 已具備事件生成、JSONL 持久化、calibration、annotation 完整鏈路（Wave 5.1~7.2）。`internal/llm_annotator/` 已接到 narrative。**但 narrative events 對 live 是隱形的**。

| 選項 | 內容 | 改動面 | 風險 | 結論 |
|------|------|--------|------|------|
| A. 維持不變 | narrative 只用於 backtest / 監控 / 標註訓練 | 0 | — | ✅ **當前採用** |
| B. 敘事事件觸發強制 close | 重大負面事件 → live 立刻 close position | 新增 `internal/live/narrative_listener.go` + EventBus 訂閱 + `RiskGate.SetHaltTrading(true)` | high：narrative 誤判直接造成損失；需要 LLM 校準到位 | ❌ 不建議（narrative 校準尚未達 production 等級） |
| C. 敘事事件觸發 reduce-only | 重大負面事件 → 拒絕新倉、僅允許平倉 | 同 B，但只改 `RiskGate` 不直接下單 | medium：仍需可信度閾值 | ❌ 延後到 narrative 校準 KPI 達標後 |

**決策**: **A**。
**觸發重評估的條件**:
1. `internal/narrative/calibration_validation.go` 的 precision ≥ 0.85（目前未量測）
2. `internal/llm_annotator/` SLO 上線 ≥ 1 個月（PR #595 進度）
3. 至少有 1 個季度的 backtest 對照組證明 narrative-driven reduce-only 勝率優於 baseline

三項全達標才考慮 C。

---

### Q3. 跨市場相關性：SPX/NDX/DJI 與台股大盤的 lag 相關性是否影響 live trading timing？

**現況**: PR #416（2026-06-08 merged）已上線 7 個 US market channel + SPX / NDX / DJI 即時報價。`internal/monitoring/gateway_adapter.go` 已串接。**但 live 完全不讀這些 channel**。

| 選項 | 內容 | 改動面 | 風險 | 結論 |
|------|------|--------|------|------|
| A. 維持不變 | US market data 只服務 monitoring / dashboard | 0 | — | ✅ **當前採用** |
| B. Live 開盤前讀 SPX/NDX/DXY 判斷當日 risk posture | `orchestrator.go` 開盤 hook 讀 snapshot | 新增 `macroSnapshotProvider` 介面 + 開盤 hook | low：純唯讀、不進交易決策 | ⚠️ 可選（小） |
| C. Live intraday 監控 SPX 即時變動 | 持續讀 SPX 報價，>2% 跌幅觸發 pause | 新增 listener + `RiskGate.SetHaltTrading(true)` 條件擴充 | medium：需要 calibration 找 threshold | ❌ 延後到 Q1/Q2/Q4 解決後 |

**決策**: **A**。B 可作 future enhancement、C 延後。
**觸發重評估的條件**: Phase 4 production trading 上線後，backtest 對照組證明 SPX 開盤前信號對 live performance 有 ≥ 5% 改善。

---

### Q4. Circuit Breaker 整合：Macro 風險指標（如 VIX 突升）是否應在 live mode 觸發 trading halt？

**現況**: live 已有 `CircuitState` 三態（normal/paused/halted），規則是 `daily_loss 2.0%` / `drawdown 3.0%` / `consecutive_sl 3`。**完全是 P&L 規則，沒有 macro 風險指標**。

| 選項 | 內容 | 改動面 | 風險 | 結論 |
|------|------|--------|------|------|
| A. 維持不變 | circuit breaker 純 P&L 規則 | 0 | — | ✅ **當前採用** |
| B. VIX 突升觸發 paused | VIX intraday > +20% 觸發 `CircuitPaused` | 新增 `vixMonitor` + `circuit_breaker.go` 新增 `VIXRule` | medium：VIX 來源要 macro_provider；threshold 需要 calibration | ❌ 延後到 Phase 4 + backtest 驗證 |
| C. Macro 風險面板（唯讀） | 開新 SSE channel 推 VIX / DXY / US10Y 給 dashboard，不接 live | 只動 monitoring | low | ⚠️ 可選（小） |

**決策**: **A**。
**觸發重評估的條件**: 任何一次 P&L drawdown > 5% 的事件發生，且歸因分析指向 macro 風險指標失效，啟動 B。

---

## 三、整合邊界契約（Contract）

### 3.1 不變式（Invariants）

以下規則**永久**有效，除非本文件被新決策明確撤銷：

1. **`internal/live/` 不得 import `internal/marketdata` 的 macro provider 套件**。驗證指令：
   ```bash
   $ grep -rn "kaecer68/atlas-go/internal/marketdata\"" internal/live/
   （必須為空）
   ```
2. **`internal/live/` 不得 import `internal/narrative/`**。驗證指令：
   ```bash
   $ grep -rn "kaecer68/atlas-go/internal/narrative" internal/live/
   （必須為空）
   ```
3. **Live mode 唯一市場資料來源維持 `o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)`**。任何新增 macro 資料必須透過 `internal/monitoring/` 或 `internal/llm_annotator/`，不得直接餵入 live。
4. **Live circuit breaker 規則不得引入 macro 指標**（除非 Q4 重評估觸發）。

### 3.2 例外申請流程

任何對 §3.1 不變式的違反，必須：
1. 在 issue 內引用本文件 §二對應問題的「觸發重評估條件」
2. 附上 backtest 對照組證據（最少 1 個季度）
3. 通過 code review 中至少一位 SRE + 一位 quant 雙重 sign-off

### 3.3 文件生命週期

- 本文件**每半年 review 一次**（下次 review：2026-12-20）
- 任一 §二問題的「觸發重評估條件」達成，**必須**更新本文件並關聯新 issue
- 若 live mode 設計哲學改變（「精簡依賴、清單驅動」被廢止），本文件需重寫

---

## 四、相關文件與引用

- PR #416（merged 2026-06-08）— `feat(apigateway): wire 7 new US market channels (SPX/NDX/DJI/NVDA/AAPL/MSFT/TSM)`
- PR #421（merged 2026-06-08）— `docs(audit): live mode integration check for US market channels`
- PR #526（merged 2026-06-14）— `fix(pipeline): expose degraded status to frontend and fix session selector`
- PR #584（merged 2026-06-18）— `feat: serialize access to global ParametersConfig (Wave 4.4)`（live UpdateDailyLoss trigger 接線）
- PR #595~#606（merged 2026-06-19~20）— Wave 7.1~7.5（llm_annotator 觀測性 + 監控）
- `.github/instructions/live-trading.guardrails.instructions.md`
- `docs/live-mode-us-market-audit.md`（PR #421 對應文件，本文件的稽核前作）

---

## 五、Issue 處理建議

| 動作 | 理由 |
|------|------|
| **保留 #424 open** 或 **改為「已記錄於 `docs/live-mode-macro-boundary.md`」並 close** | 4 個問題已收斂為 A/A/A/A 決策 + 觸發條件；設計文件本身就是交付物 |
| **新增 milestone「Phase 4 Live Trading Readiness」** | 把本文件 §三契約 + #424 4 個觸發條件納入 review checklist |
| **每半年 reminder issue** | 強制 review §一現況與 §三不變式是否仍適用 |

**建議**: close #424，引用本文件作為 closure 依據，commit hash 寫進 issue close 訊息。

---

*本文件不修改任何程式碼；變更以 commit 形式留存在 `docs/live-mode-macro-boundary.md` 本身。*
