# Atlas 現有頁面複用審計 — 投資人 UI 遷移規劃

> **狀態**: 審計完成（2026-06-02）  
> **審計範圍**: `web/static/` 下全部 22 個頁面 + 共用元件 + API 路由  
> **評估標準**: 對投資人的價值、內容可理解性、修改成本

> **⏪ 歷史文件（2026-06-24 已歸檔）**：本文提及的 Graphify 視覺化頁面（`graphify.html`）已於 v0.0.0.8 全數退役（PR #690），改以 GitNexus 為單一架構探索工具（見 [CLAUDE.md](../CLAUDE.md)）。投資人 UI 遷移時該頁面無需搬入。

---

## 總覽

| 等級 | 定義 | 數量 |
|------|------|------|
| **A** — 可直接複用（需微調） | 內容已適合投資人，僅需移除開發者術語、調整語言 | 6 |
| **B** — 後端可複用（前端需重寫）| 後端 API 數據價值高，但前端完全不適合投資人 | 3 |
| **C** — 純開發者頁面（不複用） | 開發/維運專用，對投資人零價值 | 13 |

---

## A 級：可直接複用（6 頁面）

### A1. 宏觀敘事 (`narrative.html` → `/api/narrative/events`)
| 面向 | 評估 |
|------|------|
| **現有價值** | 已展示 NarrativeEvent 時間軸、因果鏈、命中率、信心度來源 — 與投資人儀表板「今日重要事件」100% 對齊 |
| **保留** | 事件列表、因果鏈圖、命中率（NarrativeTheme.HitRate）、ConfidenceSource |
| **調整** | 移除技術術語（`ingestor.go`、`detectUSRatesEvent()`），改為「外資動向：連續 3 日買超」 |
| **新增** | 篩選：僅顯示命中率 > 60% 的事件 |
| **後端** | `GET /api/narrative/events` — 直接可用 |

### A2. 產業生態系 (`industry.html` → `/api/industry/cycles`, `/api/industry/seasonality`, `/api/industry/supply-chain`)
| 面向 | 評估 |
|------|------|
| **現有價值** | 供應鏈連動圖、季節性日曆、產業週期羅盤 — 三大板塊都是投資人理解市場結構的核心 |
| **保留** | 供應鏈圖譜（簡化版）、產業週期位置、季節性模式列表 |
| **調整** | 移除內部參數名稱（`downstream_decay_factor`），改為「連動強度」 |
| **新增** | 圖表精簡：只顯示當前 regime 相關產業，非全部 20+ 產業 |
| **後端** | `GET /api/industry/cycles`, `GET /api/industry/seasonality`, `GET /api/industry/seasonality/calendar`, `GET /api/industry/supply-chain` — 全部可用 |

### A3. 投資管線 (`pipeline.html` → `/api/dashboard/recommendation-pipeline`)
| 面向 | 評估 |
|------|------|
| **現有價值** | 展示從篩選到最終推薦的完整管線，投資人需要看到「推薦從哪來」 |
| **保留** | 推薦列表 + 篩選通過/拒絕原因 |
| **調整** | 簡化為：推薦層級摘要（不要原始 JSON 輸出） |
| **新增** | 每筆推薦加入「一句話理由」（從 FactorScoreBreakdown 提取最強因子） |
| **後端** | `GET /api/dashboard/recommendation-pipeline` — 直接可用 |

### A4. 決策鏈 (`decision-chain.html` → `/api/decision-chain`)
| 面向 | 評估 |
|------|------|
| **現有價值** | FactorScoreBreakdown、ConvictionBreakdown 完整透明 — 是「信任」的核心 |
| **保留** | 因子分數長條圖、信心度分解步驟 |
| **調整** | 圖表格式：從表格 → 水平長條圖 + 顏色編碼（綠=正向、紅=負向） |
| **新增** | 投資人語言摘要（NLG 層渲染） |
| **後端** | `GET /api/dashboard/decision-chain` — 直接可用 |

### A5. 組合持倉 (`portfolio.html` → `/api/dashboard/positions`, `/api/dashboard/performance`)
| 面向 | 評估 |
|------|------|
| **現有價值** | 組合配置、持倉明細、Darwinian 權重 — 投資人最需要的核心頁面 |
| **保留** | 組合配置圖（圓餅圖）、持倉列表、權重分配 |
| **調整** | 術語轉換：「Darwinian 權重」→「動態配置權重」、移除 Agent ID 等內部識別 |
| **新增** | 加入 Benchmark 對比（TAIEX / 0050 同期表現） |
| **後端** | `GET /api/dashboard/positions`, `GET /api/dashboard/performance` — 直接可用 |

### A6. 績效報告 (`performance.html` → `/api/dashboard/performance-report`)
| 面向 | 評估 |
|------|------|
| **現有價值** | Sharpe、最大回撤、勝率、月報酬 — 投資人績效驗證的核心 |
| **保留** | 累積報酬曲線、Sharpe 時間序列、回撤圖 |
| **調整** | 加入 Benchmark 曲線疊加（TAIEX / 0050） |
| **新增** | 階段摘要文字：「過去 30 天表現優於大盤 1.4%」 |
| **後端** | `GET /api/dashboard/performance-report` — 需擴展加入 Benchmark 對比 |

---

## B 級：後端可複用（3 頁面/模組）

### B1. Agent 績效 (`agents.html` → `/api/dashboard/agent-status`)
| 面向 | 評估 |
|------|------|
| **後端價值** | AgentHealth（healthy/degraded/muted/recovering）、命中率 → 用於 TrustScore |
| **前端** | 完全開發者導向（agent ID、layer、screening criteria），不適合投資人 |
| **複用方式** | 後端數據匯入 TrustScore 的「推薦命中率」維度，前端新建「Agent 績效排行榜」（最強/最弱 agent） |

### B2. 風險儀表板 (`risk.html` → `/api/dashboard/risk`, `/api/dashboard/risk-calibration`)
| 面向 | 評估 |
|------|------|
| **後端價值** | VaR、壓力測試、最大回撤、風險參數校準報告 → 用於投資人風險報告頁面 |
| **前端** | 過於技術化（threshold 調整介面），投資人只需要「數字和解釋」 |
| **複用方式** | 後端數據直接呈現為：VaR 95%（數字）+「這意思是在 95% 的情況下…」（NLG） |

### B3. 演化透視 (`evolution.html` → `/api/dashboard/mutations`, `/api/dashboard/experiment-history`)
| 面向 | 評估 |
|------|------|
| **後端價值** | 實驗歷史、策略演化記錄 → 用於「Atlas 的自進化能力」信任證明 |
| **前端** | 純開發者頁面（mutation brief、實驗判決） |
| **複用方式** | 提取「最近 30 天策略改良摘要」→ 在投資人信任分數頁面顯示「策略正在持續進化」 |

---

## C 級：純開發者頁面（13 頁面，不複用）

以下頁面僅服務於開發/維運，對投資人無價值，留在 `admin_web/` 即可：

| 頁面 | 理由 |
|------|------|
| 系統健康 (`system.html`) | 開發者監控 |
| 參數管理 (`parameters.html`) | `ParametersConfig` 內部調校 |
| 部署配置 (`deployment.html`) | 部署工具設定 |
| 事件邏輯 (`events.html`) | 內部事件監控 |
| Swarm 模擬 (`swarm.html`) | 研究工具 |
| Agent 生成 (`spawning.html`) | Agent lifecycle 管理 |
| 配置管理 (`config.html`) | 內部 config 編輯 |
| 任務佇列 (`tasks.html`) | TaskExec 監控 |
| API 測試 (`api-test.html`) | 開發者工具 |
| 資料匯入 (`import.html`) | 資料管理 |
| 日誌檢視 (`logs.html`) | 內部 log |
| 稽核軌跡 (`audit.html`) | 內部審計 |
| Graphify (`graphify.html`) | 知識圖譜視覺化 |

---

## 共用元件複用清單

以下 `web/static/` 下的共用元件可直接搬入 `client_web/`：

| 元件 | 路徑 | 用途 |
|------|------|------|
| 日期選擇器 | 全域共用 | 投資人儀表板選擇日期範圍查看 |
| 狀態指示燈 | 全域共用 | 升級為 regime 狀態指示（綠/黃/紅） |
| 表格排序 + 篩選 | `web/static/css/` | 推薦列表、持倉列表 |
| 主題變數 | `web/static/css/` | 調整色調後直接用（開發者深色 → 投資人明亮色） |

---

## 遷移策略

```
web/static/              目前位置（混合開發者+可複用）
     │
     ├──→ admin_web/     13 個 C 級頁面 + 3 個 B 級原始頁面
     │
     └──→ client_web/    6 個 A 級頁面（改造後）+ 全新投資人頁面
                          + 共用元件（CSS 主題調整後）
```

### 遷移建議
1. **第一階段**：先複製 A1-A6 頁面到 `client_web/`，調整術語、移除開發者元素
2. **第二階段**：新建投資人專屬頁面（儀表板摘要、每日摘要、TrustScore），引用調整後的 A 級頁面作為子區塊
3. **第三階段**：B 級後端 API 封裝為 `/api/client/` 路由
4. **最後階段**：將 C 級頁面移入 `admin_web/`，原 `web/static/` 最終廢棄

### API 路由隔離
- 現有：`/api/dashboard/*` → 保留，用於 `admin_web/` 和內部使用
- 新增：`/api/client/*` → 投資人專用，包裝 dashboard API，加入 NLG 轉換層

---

## 相關技能文件

- `atlas-investor-ui/SKILL.md` — 核心架構與設計原則
- `atlas-investor-pages/SKILL.md` — 6 頁面 wireframe 規格（應參照本審計的 A 級頁面）
- `atlas-investor-roadmap/SKILL.md` — Phase A/B 實作路線圖
