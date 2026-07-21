# AGENTS.md 全盤審計報告

> **審計日期**：2026-07-11
> **審計範圍**：全部 34 個 AGENTS.md 檔案
> **審計方法**：4 批次 parallel explore agents 逐檔分析 + ACI 交叉驗證

---

## 總體統計

| 指標 | 數值 |
|------|------|
| AGENTS.md 總數 | **34** |
| 平均行數 | 71 |
| 最長 | `internal/marketdata/AGENTS.md`（187 行） |
| 最短 | `internal/db/AGENTS.md`（32 行） |
| 超過 100 行 | 5 個 |
| 內容衝突（需核對程式碼） | **5 處** |
| 大量內容應移至 docs/ | **4 個檔案** |

---

## 🔴 P0 — CRITICAL：內容衝突（必須立即核對）

### C1: Fubon URL guard 狀態矛盾

| 檔案 | 說法 |
|------|------|
| `internal/fubonproxy/AGENTS.md` | AST guard test 已 ship，`fubon_url_guard_test.go` 會抓出直接 hardcode URL |
| `internal/marketdata/AGENTS.md` | guard 尚未 ship，仍是 convention + code review 階段 |

**建議**：核對 `internal/marketdata/fubon_url_guard_test.go` 是否實際存在並通過 CI。以程式碼為準統一說法。

### C2: Industry API 新舊路徑矛盾

| 檔案 | 說法 |
|------|------|
| `internal/industry/AGENTS.md` | 新路徑：`/api/dashboard/industry-*`，舊 `/api/industry/*` 已棄用 |
| `internal/monitoring/AGENTS.md` | 仍列出 `/api/industry/cycles` 等舊路由 |

**建議**：核對 `internal/monitoring/api/industry/handlers.go` 與 `cmd/atlas/main.go` 實際路由註冊。統一兩處文件。

### C3: admin_web ↔ client_web 色彩系統大量重複但版本不一致

兩個檔案共用約 70% 內容（CSS 色彩語意系統、Canvas 繪圖橋接、色彩選擇決策樹），但：

- `admin_web/AGENTS.md`（108 行）：含 capital-flow / signal / JS color helper 分支
- `client_web/AGENTS.md`（101 行）：缺少上述 3 個分支

**建議**：色彩系統抽取到 `shared_web/` 或 `docs/guides/` 做為單一權威來源，兩處 AGENTS.md 只保留 cross-ref。

### C4: baseline policy 檔案路徑潛在不一致

`internal/baseline/AGENTS.md` 使用路徑 `data/state/baseline_policy.json`；部分資料目錄標準文件提到 `data/state/baseline/baseline_policy.json`。

**建議**：以實際 `go:embed` 路徑或 `config.go` 中的常數為準。

### C5: strategy_ranker ↔ strategy_validator 職責描述重疊

`strategy_ranker/AGENTS.md` 有 ~52%（30 行）描述 `strategy_validator.Rank`/`AssignTiers` 的行為，而非 strategy_ranker 本身。

**建議**：cut 至 5 行 cross-ref。

---

## 🟠 P1 — HIGH：應移至 docs/ 的內容

### 需拆分/搬移的檔案

| 檔案 | 行數 | Hot-path% | 問題 | 建議目標 |
|------|------|----------|------|---------|
| `internal/marketdata/AGENTS.md` | **187** | 60% | TWSE charset/deprecation RCA、ETF NAV 調查、Fubon proxy 長背景 → 移到 docs/ | `docs/investigations/`、`docs/REFERENCE/TRAPS.md` |
| `scripts/openclaw/AGENTS.md` | 112 | 15% | 90% 為操作指南（script 目錄概述、常用指令、skill 整合）→ 移到 docs/ | `docs/operations/openclaw-governance.md` |
| `cmd/experimental/AGENTS.md` | 50 | 55% | CLI 目錄清單、常用指令 → 移到 docs/ | `docs/QUICKSTART.md`、`docs/script_usage_guide.md` |
| `internal/strategy_techniques/AGENTS.md` | 52 | **25%** | 五層框架表、4 核心指標表、自我修正機制 → 移到 docs/specs/ | `docs/specs/strategy-techniques-spec.md` |

### 需精簡的部分段落

| 檔案 | 段落 | 應移至 |
|------|------|--------|
| `internal/narrative/AGENTS.md` | 滾動校準框架（7 行） | cross-ref `docs/MACRO_CALIBRATION.md` |
| `internal/baseline/AGENTS.md` | Policy lifecycle（24 行） | `docs/operations_playbook.md` |
| `internal/llm/AGENTS.md` | 環境變數 catalog（15 行） | `CLAUDE.md`、`docs/ENVIRONMENT.md` |
| `internal/risk/AGENTS.md` | 組態設定總論（17 行） | `docs/REFERENCE/PARAMETER_SYSTEM.md` |
| `internal/industry/AGENTS.md` | 供應鏈圖設計（10+ 行） | `docs/specs/` |
| `internal/live/AGENTS.md` | 通用 live 安全策略 | `.github/instructions/live-trading.guardrails.instructions.md` |
| `internal/recommender/AGENTS.md` | 認證鏈（與 subscription 重複） | 只留 X-User-Email fallback |

---

## 🟡 P2 — MEDIUM：跨檔重複矩陣

### 重複熱點

| 重複主題 | 出現位置 | 建議權威來源 |
|----------|---------|-------------|
| CSS 色彩語意系統 | admin_web、client_web、CLAUDE.md | `docs/guides/frontend-architecture.md` 或 `shared_web/static/css/base/variables.css` |
| Constitution 禁令 | apigateway、config、AGENTS.md、TRAPS.md、CONVENTIONS_CHECKLIST | `docs/REFERENCE/CONSTITUTION.md` |
| Baseline 優先 / 實驗安全 | experiment、baseline、AGENTS.md、.github/instructions | `.github/instructions/experiments-guardrails.instructions.md` |
| Live trading 安全 | live、AGENTS.md、SECURITY.md、.github/instructions | `.github/instructions/live-trading.guardrails.instructions.md` |
| 環境變數 | llm、CLAUDE.md、config | `docs/ENVIRONMENT.md` 或 `CLAUDE.md` |
| FactorType 變更協議 | portfolio、skill | `.claude/skills/atlas-factor-change-protocol/SKILL.md` |
| Tool 計數 | cmd/atlas-mcp/server、AGENTS.md、tool-catalog、README | `cmd/atlas-mcp/server/AGENTS.md`（code assertion） |
| API 路由 | industry、monitoring、apigateway | 以 `main.go` + `handlers.go` 為準 |
| 認證鏈 | recommender、subscription、monitoring/api/shared | `monitoring/api/shared/AGENTS.md`（middleware 權威） |

---

## 🟢 P3 — LOW：格式瑕疵

| 檔案 | 問題 |
|------|------|
| `internal/portfolio/AGENTS.md` | 標題格式：`# internal/portfolio AGENTS.md`（應為 `# AGENTS.md — internal/portfolio`） |
| `internal/strategy_techniques/AGENTS.md` | 標題格式：`# Strategy Techniques AGENTS.md`（非標準） |
| `admin_web/AGENTS.md`、`client_web/AGENTS.md` | 標題使用小寫 `agents.md`（其他全用大寫） |
| `internal/db/AGENTS.md` | 末行殘留舊 footer：`(End of file - total 28 lines)`（實際 32 行） |
| `internal/logging/AGENTS.md` | 核心型別表格重複表頭（line 10-13） |
| `AGENTS.md` lines 102/103/107 | Markdown 連結使用 `` [`path`](path) `` 語法（leading backtick），渲染可能異常 |

---

## 📊 Token 效率評估

當前全部 34 個 AGENTS.md **總行數約 2,400 行**。若每次修改相關模組時需載入：
- 根 AGENTS.md：151 行（必載）
- 模組 AGENTS.md：平均 71 行
- 相關模組 AGENTS.md：約 2-3 個
- **每次修改場景載入量**：~300-400 行

### 最佳化建議

| 措施 | 預計節省 |
|------|---------|
| admin_web + client_web 去重（合併到 shared_web/doc） | -100 行 |
| marketdata 拆分到 docs/ | -90 行 |
| strategy_techniques 搬到 docs/specs/ | -35 行 |
| 各檔 cross-ref 取代長篇敘述 | -150 行 |
| scripts/openclaw 搬到 docs/operations/ | -90 行 |
| **合計預計節省** | **~465 行（-19%）** |

---

## 後續行動優先級

### 立即（P0）
1. 核對 C1-C5 五處內容衝突（以程式碼為準）
2. 修正衝突後同步更新受影響檔案

### 短期（P1）
3. 拆分 `internal/marketdata/AGENTS.md`（187→~90 行）
4. 遷移 `scripts/openclaw/AGENTS.md` 至 `docs/operations/`
5. 遷移 `internal/strategy_techniques/AGENTS.md` 至 `docs/specs/`
6. admin_web + client_web CSS 色彩系統合併至 `docs/guides/frontend-architecture.md`

### 中期（P2）
7. 各檔環境變數/Constitution/安全策略段落統一使用 cross-ref
8. 建立 AGENTS.md 內容歸屬 checklist（hot-path vs docs/ 判斷準則）

### 長期（P3）
9. 格式統一：標題、小寫 agents.md、markdown 連結語法
10. db/AGENTS.md 殘留 footer 清理、logging 重複表頭修復
