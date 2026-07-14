# AGENTS.md 整合稽核報告（Audit Report）

> **建立日期**：2026-06-27
> **稽核範圍**：44 個 `internal/*/AGENTS.md`（不含已保留的 13 個 — 見下方保留清單）
> **目標**：從 57 個總 AGENTS.md 縮減到 ≤15 個
> **策略**：先遷移內容到合適位置（`.claude/skills/` / `docs/specs/` / `docs/guides/` / `doc.go`），再刪除模組內 AGENTS.md

---

## 1. 背景

### 現況（2026-06-27 audit）

| 指標 | 數值 |
|------|------|
| 總 `AGENTS.md` 數 | **57 個** |
| `internal/*/AGENTS.md` | 50 個 |
| 其他位置（web/、admin_web/、client_web/、cmd/、scripts/、docs/、根）| 7 個 |
| `internal/*/doc.go` 總數 | 58 個 |
| `AGENTS.md` ∩ `doc.go` 雙存在 | **50/50（100%）** |

### 問題診斷

1. **內容大量重疊**：50 個模組每個都有 `AGENTS.md` 和 `doc.go`，後者已涵蓋 Maturity 標記與模組簡介
2. **Token 浪費**：AI 預讀 `internal/<mod>/AGENTS.md` 觸發門檻低，5-10 個檔案 × 平均 40 行 ≈ 2000 行無用上下文
3. **訊號稀釋**：每個模組都有「重要陷阱」，實際上等於沒有重點
4. **維護負擔**：每改一個模組要同步兩處文件

### 專案根 `AGENTS.md` 已規範的歸屬規則

> 跨模組全域規則 → `AGENTS.md`（本檔）
> 模組內部陷阱/API/流程 → `internal/<mod>/AGENTS.md`
> 操作程序 / playbook → `docs/`
> 憲法級強制規範 → `docs/reference/constitution.md`、`internal/apigateway/CONSTITUTION.md`
> 技能 / 子代理指引 → `.claude/skills/`

**問題在於「50 個模組都符合第二條」** — 規則本身沒問題，但實作未考慮「內容是否真的獨特」。

---

## 2. 分類結果（44 個 AGENTS.md）

### 2.1 分類方法

依每個檔案內容是否提供「根 AGENTS.md + doc.go + 既有 skill/CONSTITUTION 都沒有的獨特資訊」分為 4 類：

| 類別 | 含義 | 後續動作 |
|------|------|---------|
| **A. 純重疊 doc.go** | 內容已完全被 doc.go + 根 AGENTS.md 涵蓋 | 合併至 doc.go，刪除 AGENTS.md |
| **B. 已有 skill / CONSTITUTION 涵蓋** | 內容分散到 `.claude/skills/atlas-<x>/SKILL.md` 或 `internal/<mod>/constitution.md` | 確認 skill 涵蓋後刪除 AGENTS.md |
| **C. 獨特內容** | 有根 AGENTS.md / doc.go / 既有 skill 都沒有的細節 | 保留為 AGENTS.md 或遷移至 docs/specs/、.claude/skills/ |
| **D. 空殼** | 內容空洞（≤25 行僅列 KEY TYPES）或 X 級實驗性模組 | 直接刪除 |

### 2.2 分類統計

| 類別 | 數量 | 比例 | 後續動作 |
|------|------|------|---------|
| A. 純重疊 doc.go | 5 | 11% | 合併 doc.go → 刪除 AGENTS.md |
| B. 已有 skill / CONSTITUTION 涵蓋 | 9 | 20% | 確認涵蓋 → 刪除 AGENTS.md |
| C. 獨特內容 | 19 | 43% | 保留 15 個 / 遷移 4 個 |
| D. 空殼 | 11 | 25% | 直接刪除 |
| **總計** | **44** | **100%** | — |

### 2.3 詳細分類表

| # | 模組 | AGENTS.md 行數 | 分類 | 對應 skill / CONSTITUTION / 遷移目標 |
|---|------|---------------|------|--------------------------------------|
| 1 | adversarial | 22 | **D** | X 級實驗性，無獨特內容 |
| 2 | apigateway | 53 | **B** | `internal/apigateway/CONSTITUTION.md`（6 條憲法）+ `atlas-data-visibility` |
| 3 | autobacktest | 35 | **C** | SignalEngine FullStore + 15% 熔斷 + next13_30 timezone — **保留** |
| 4 | backtest | 30 | **C** | RollingWindowSplit + valid_start.Year 停止 — **保留** |
| 5 | baseline | 78 | **C** | Policy lifecycle + Trigger 執行期強制 — **保留** |
| 6 | bootstrap | 42 | **C** | Init 順序 → **遷移至 `docs/quickstart.md`** |
| 7 | config | 38 | **B** | `docs/reference/parameter-system.md` 涵蓋 .env / Magic number |
| 8 | db | 28 | **A** | 合併 DATABASE_URL / migration path → doc.go |
| 9 | domain | 56 | **C** | Scorecard OOS 同步鏈 4 位置 + CorporateAction — **保留** |
| 10 | eval | 25 | **A** | 合併 Fin-Skills 編號 → doc.go |
| 11 | eventbus | 43 | **B** | `apigateway/constitution.md` Article 4 (BackgroundTaskManager) |
| 12 | feature | 23 | **A** | 合併 Registry 簽名 → doc.go |
| 13 | fubonproxy | 82 | **B** | `atlas-fubon-supervisor-invariants` (F1~F9) 已涵蓋 |
| 14 | globalmarket | 34 | **C** | 多市場 tier 1 限制 — **遷移至 `internal/industry/AGENTS.md`** |
| 15 | importer | 20 | **D** | CLI 工具、無獨特內容 |
| 16 | industry | 100 | **C** | 9 子系統 + 7 API + 4 Experimental Files — **保留** |
| 17 | janus | 69 | **C** | Regime → Risk Gate 自校準接線 — **保留** |
| 18 | ledger | 68 | **C** | BuildScorecards OOS 計算 5 步驟 — **保留** |
| 19 | llm_annotator | 59 | **B** | `atlas-llm-provider-capability` + deprecation 計畫 |
| 20 | logging | 43 | **A** | 合併 Critical level 12 → doc.go |
| 21 | marketdata | 84 | **C** | Provider 優先級 + ETF NAV 4 Layer + providerBreaker — **保留** |
| 22 | metalearning | 35 | **D** | X 級實驗性，無獨特內容 |
| 23 | ml | 24 | **D** | X 級，Fin-Skills 編號已於 eval 記錄 |
| 24 | monitoring | 82 | **B** | `atlas-data-visibility` 涵蓋 4 層 + DriftDetector 部分 |
| 25 | prism | 62 | **C** | Synthetic flag + classifyRegime — **保留** |
| 26 | realtime | 34 | **A** | 合併 RegimeType 粒度差異 → doc.go |
| 27 | reflexivity | 20 | **D** | X 級，1 symbol |
| 28 | replay | 21 | **D** | CLI 工具 |
| 29 | reporting | 33 | **C** | BacktestWindowSummary 欄位完整性契約 — **保留** |
| 30 | repository | 49 | **C** | DualWriteRepository + ROC +1911 + TimescaleDB — **保留** |
| 31 | retail | 94 | **C** | RSI-tw 12 子指標 + 11 欄位真實性對照 — **保留** |
| 32 | risk | 61 | **B** | `atlas-risk-management` 涵蓋 4 層架構 |
| 33 | robustness | 23 | **D** | X 級，無獨特內容 |
| 34 | scheduler | 24 | **D** | X 級，無獨特內容 |
| 35 | screener | 33 | **C** | 永不回傳錯誤契約 + 零值 P/E/P/B — **保留** |
| 36 | sim | 37 | **C** | RunWithState 就地變異 + RunDay 7 步執行序列 — **保留** |
| 37 | spawning | 38 | **C** | 遷移至 `atlas-strategy-evolution` skill |
| 38 | storage | 45 | **C** | 合併至 doc.go |
| 39 | strategy | 36 | **B** | `atlas-multi-strategy` 涵蓋 Selector/Allocator |
| 40 | strategy_techniques | 52 | **B** | `atlas-strategy-techniques` 涵蓋 5 層框架 |
| 41 | stress | 20 | **D** | X 級，1 function |
| 42 | swarm | 22 | **D** | X 級，無獨特內容 |
| 43 | taskexec | 22 | **D** | CLI 工具 |
| 44 | tax | 34 | **C** | TaxAwareSizer + NHISurcharge — **保留** |

---

## 3. 遷移計畫

### 3.1 最終保留清單（13 個 `internal/*/AGENTS.md` + 4 個非 internal = 17 個 → 調整後 ≤15）

加上已保留的 8 個（llm/portfolio/narrative/live/experiment/orchestrator + marketdata/industry/ledger + apigateway CONSTITUTION）= 最終 15 個以內。

| 類型 | 檔案 | 理由 |
|------|------|------|
| 根 | `./AGENTS.md` | 唯一不變 |
| 前端（3 個）| `web/AGENTS.md`、`admin_web/AGENTS.md`、`client_web/AGENTS.md` | 前端專屬 |
| 文件索引 | ~~`docs/wave-11/README.md`~~ | Wave 11 工作目錄已於 2026-06-28 解散，內容永久化為 `docs/operations/l2-4-runbook.md` + `docs/specs/l2-4-observation-spec.md` + `docs/operations/l2-4-followup.md`（PR #824）|
| 工具（2 個）| `cmd/experimental/AGENTS.md`、`scripts/openclaw/AGENTS.md` | 跨模組工具 |
| LLM（5 個）| `internal/llm/AGENTS.md`、`internal/portfolio/AGENTS.md`、`internal/narrative/AGENTS.md`、`internal/live/AGENTS.md`、`internal/experiment/AGENTS.md` | 已保留，PR #776 後 |
| 數據 | `internal/marketdata/AGENTS.md` | Provider 優先級 + ETF NAV |
| 風險 | `internal/orchestrator/AGENTS.md` | route 流程 |
| 內部 C 類精選 | `internal/industry/AGENTS.md` | 9 子系統 + Experimental |
| 內部 C 類精選 | `internal/ledger/AGENTS.md` | BuildScorecards OOS |
| 內部 C 類精選 | `internal/domain/AGENTS.md` | Scorecard OOS 同步鏈 |
| 內部 C 類精選 | `internal/baseline/AGENTS.md` | Policy lifecycle + Trigger |
| 內部 C 類精選 | `internal/sim/AGENTS.md` | RunWithState + RunDay |
| 內部 C 類精選 | `internal/janus/AGENTS.md` | Regime → Risk Gate |
| 內部 C 類精選 | `internal/prism/AGENTS.md` | Synthetic flag |
| 內部 C 類精選 | `internal/retail/AGENTS.md` | RSI-tw |
| 內部 C 類精選 | `internal/portfolio/AGENTS.md` | Darwinian + Factor Change |
| 內部 C 類精選 | `internal/live/AGENTS.md` | 可靠性邊界 |
| 內部 C 類精選 | `internal/experiment/AGENTS.md` | Acceptance Gates |

**等等，這超過 15 個了**。需要更精簡。讓我用另一個原則：

> **保留原則**：每個 `internal/*/AGENTS.md` 必須有「**不在根 AGENTS.md、doc.go、既有 skill/CONSTITUTION 中**」的獨特內容。

最終精簡為 **12 個 `internal/*/AGENTS.md`**（加上 4 個非 internal = 16 個，但部分已有，扣除重複）：

**精簡後保留（12 個 internal + 4 個非 internal = 16 → 取目標 15）**：

| 編號 | 檔案 | 理由 |
|------|------|------|
| 1 | `./AGENTS.md` | 根 |
| 2 | `web/AGENTS.md` | 前端 SPA |
| 3 | `admin_web/AGENTS.md` | 後台 |
| 4 | `client_web/AGENTS.md` | client |
| 5 | ~~`docs/wave-11/README.md`~~ | Wave 11 工作目錄已於 2026-06-28 解散並永久化（PR #824）|
| 6 | `scripts/openclaw/AGENTS.md` | 跨模組工具 |
| 7 | `cmd/experimental/AGENTS.md` | CLI |
| 8 | `internal/llm/AGENTS.md` | DataClass 閘門 + hot-path |
| 9 | `internal/portfolio/AGENTS.md` | Darwinian + Factor Change |
| 10 | `internal/narrative/AGENTS.md` | Event 狀態機 + Calibration |
| 11 | `internal/live/AGENTS.md` | 可靠性邊界 |
| 12 | `internal/experiment/AGENTS.md` | Acceptance Gates |
| 13 | `internal/orchestrator/AGENTS.md` | route 流程 |
| 14 | `internal/marketdata/AGENTS.md` | Provider 優先級 + ETF NAV |
| 15 | `internal/industry/AGENTS.md` | 9 子系統 + Experimental |

15 個**剛好**。其他 28 個 `internal/*/AGENTS.md` 全部刪除（**44 - 12 = 32 個**刪除）。

### 3.2 刪除清單（32 個 `internal/*/AGENTS.md`）

| Batch | 模組 | 理由 |
|-------|------|------|
| **1. 純刪除（D 類空殼）** | adversarial, importer, metalearning, ml, reflexivity, replay, robustness, scheduler, stress, swarm, taskexec | 11 個 — 內容空洞，與 doc.go 重疊 |
| **2. Skill 涵蓋（B 類）** | apigateway, config, eventbus, fubonproxy, llm_annotator, monitoring, risk, strategy, strategy_techniques | 9 個 — 對應 skill / CONSTITUTION 已涵蓋 |
| **3. 合併 doc.go（A 類）** | db, eval, feature, logging, realtime | 5 個 — 補充關鍵內容至 doc.go |
| **4. 邊界 C 類** | bootstrap, globalmarket, spawning, storage | 4 個 — 遷移至既有文件 |
| **5. 保留為精選 C 類** | ledger, domain, baseline, sim, janus, prism, retail, tax, backtest, autobacktest, screener, reporting, repository | 13 個 — 全部需保留 |

**等等 — 11+9+5+4+13 = 42，但 44-15 = 29**。重新計算：

**實際最終保留 = 15 個 `AGENTS.md`**（含 8 個 internal + 4 個非 internal + 1 個根 + 1 個 cmd/experimental；原 `docs/wave-11/` 已於 2026-06-28 解散）

但用戶說「15 個以內」是針對 `internal/*/AGENTS.md` 還是全部？看用戶原話：「**看看是不是有可能減少到 10 個以內**」。所以目標是 `internal/*/AGENTS.md`。

**最終 internal AGENTS.md 數量（目標 ≤10）**：

| 編號 | 檔案 | 行數 | 理由 |
|------|------|------|------|
| 1 | `internal/llm/AGENTS.md` | 74 | DataClass 閘門 + hot-path 護欄 |
| 2 | `internal/portfolio/AGENTS.md` | 79 | Darwinian + Factor Change |
| 3 | `internal/narrative/AGENTS.md` | 80 | Event 狀態機 + Calibration |
| 4 | `internal/live/AGENTS.md` | 75 | 可靠性邊界 + BROKER_MODE |
| 5 | `internal/experiment/AGENTS.md` | 46 | Acceptance Gates |
| 6 | `internal/orchestrator/AGENTS.md` | 78 | route 流程 |
| 7 | `internal/marketdata/AGENTS.md` | 74 | Provider 優先級 + ETF NAV |
| 8 | `internal/industry/AGENTS.md` | 76 | 9 子系統 + Experimental |
| 9 | `internal/ledger/AGENTS.md` | 68 | BuildScorecards OOS |
| 10 | `internal/baseline/AGENTS.md` | 78 | Policy lifecycle + Trigger |

剛好 10 個。其餘 32 個（44-12=32，其中 12 已保留 8 個 = 50-8=42 待處理；再扣遷移 4 個 batch 4 = 38；扣純刪除 11 個 batch 1 = 27；扣 skill 涵蓋 9 個 batch 2 = 18；扣合併 doc.go 5 個 batch 3 = 13；扣合併到產業 4 個 batch 4 = 9？）

讓我重新核算：50 - 10（保留 internal）= 40 個待處理。4 batch 合計 11+9+5+4+11=40 ✓（其中 batch 5「保留為精選 C 類」實際上是「遷移至 docs/」）。

**最終修正：10 個保留 internal AGENTS.md + 40 個遷移/刪除**：

| Batch | 模組 | 數量 | 動作 |
|-------|------|------|------|
| **1. 純刪除** | adversarial, importer, metalearning, ml, reflexivity, replay, robustness, scheduler, stress, swarm, taskexec | 11 | 直接刪除 AGENTS.md |
| **2. Skill 涵蓋** | apigateway, config, eventbus, fubonproxy, llm_annotator, monitoring, risk, strategy, strategy_techniques | 9 | 確認 skill/CONSTITUTION 涵蓋後刪除 |
| **3. 合併 doc.go** | db, eval, feature, logging, realtime | 5 | 補充內容至 doc.go → 刪除 |
| **4. 邊界 C 類遷移** | bootstrap, globalmarket, spawning, storage | 4 | 遷移至 docs/quickstart.md、industry/AGENTS.md、atlas-strategy-evolution skill、doc.go |
| **5. 精選 C 類保留** | ledger, baseline, domain, sim, janus, prism, retail, tax, backtest, autobacktest, screener, reporting, repository | 13 | **最終決定：合併到 10 個保留清單中（見下）** |

**Batch 5 的 13 個模組需進一步精簡為 3 個**（讓 internal 保留清單 = 10 個 + 已存在的 8 個 - 已有重複 = 實際目標 10 個）：

| 評估 | 模組 | 理由 |
|------|------|------|
| **保留** | ledger | BuildScorecards OOS 5 步驟細節是 UNIQUE |
| **保留** | baseline | Policy lifecycle + Trigger 評估規則表 |
| **保留** | sim | RunWithState + RunDay 序列（hot-path） |
| **併入其他** | domain | Scorecard OOS 同步鏈已於 root AGENTS.md + traps.md 提及，保留**精簡版**（56→30 行）或併入 sim/AGENTS.md |
| **併入 industry** | janus | Regime → Risk Gate 接線 — 併入 `industry/AGENTS.md` §週期卡章節 |
| **併入 industry** | prism | Synthetic flag — 併入 `industry/AGENTS.md` §實驗檔案 |
| **併入 monitoring 內容** | retail | RSI-tw 12 子指標 + 11 欄位真實性對照 — 併入 `docs/guides/retail-sentiment.md`（新建） |
| **併入 config** | tax | NHISurcharge 邏輯 — 併入 `internal/config/AGENTS.md` §Tax 章節（config AGENTS.md 將保留，與 apigateway 一樣） |
| **併入 portfolio** | backtest | WindowRun 契約 — 併入 `portfolio/AGENTS.md` §backtest 章節 |
| **併入 portfolio** | autobacktest | SignalEngine FullStore — 併入 `portfolio/AGENTS.md` §autobacktest 章節 |
| **併入 root AGENTS.md** | screener | 永不回傳錯誤契約已在 `docs/reference/traps.md` 提及 |
| **併入 root AGENTS.md** | reporting | 欄位完整性契約屬跨模組 |
| **併入 root AGENTS.md** | repository | DualWriteRepository 是 generic infra 概念 |

**最終保留清單 = 10 個 `internal/*/AGENTS.md`**：

1. `internal/llm/AGENTS.md` ✅
2. `internal/portfolio/AGENTS.md` ✅
3. `internal/narrative/AGENTS.md` ✅
4. `internal/live/AGENTS.md` ✅
5. `internal/experiment/AGENTS.md` ✅
6. `internal/orchestrator/AGENTS.md` ✅
7. `internal/marketdata/AGENTS.md` ✅
8. `internal/industry/AGENTS.md` ✅（合併 janus + prism 內容）
9. `internal/ledger/AGENTS.md` ✅
10. `internal/baseline/AGENTS.md` ✅（合併 sim 部分內容）

**`internal/domain`、`sim`、`janus`、`prism`、`retail`、`tax`、`backtest`、`autobacktest`、`screener`、`reporting`、`repository` 全部刪除** — 內容已合併到上述 10 個 AGENTS.md 或遷移至 docs/。

---

## 4. 完整 Batch 計畫

### Batch 1：純刪除（11 個 D 類）

無內容遷移。直接 `git rm internal/<mod>/AGENTS.md`。

### Batch 2：Skill / CONSTITUTION 涵蓋（9 個 B 類）

對每個 skill/CONSTITUTION，先 **verify 涵蓋**，再刪除 AGENTS.md。

### Batch 3：合併 doc.go（5 個 A 類）

對每個模組：
1. 讀 AGENTS.md 抓 3-5 個關鍵內容
2. 編輯 `doc.go` 補充
3. `git rm AGENTS.md`

### Batch 4：邊界 C 類遷移（4 個）

| 模組 | 遷移目標 |
|------|---------|
| bootstrap | 併入 `docs/quickstart.md` §啟動流程 |
| globalmarket | 併入 `internal/industry/AGENTS.md` §多市場擴展章節 |
| spawning | 併入 `.claude/skills/atlas-strategy-evolution/SKILL.md` |
| storage | 合併至 doc.go（保留為內部註解） |

### Batch 5：精選 C 類合併（13 → 0）

| 模組 | 遷移目標 |
|------|---------|
| ledger | **保留** AGENTS.md（10 個保留清單之一） |
| baseline | **保留** AGENTS.md |
| sim | **保留** AGENTS.md |
| domain | 精簡後併入 ledger/AGENTS.md（56 → 30 行）|
| janus | 併入 industry/AGENTS.md §週期卡章節 |
| prism | 併入 industry/AGENTS.md §實驗檔案章節 |
| retail | 遷移至 `docs/guides/retail-sentiment.md`（新建）|
| tax | 併入 `internal/config/AGENTS.md`（config 將保留，與 apigateway 一樣）|
| backtest | 併入 portfolio/AGENTS.md §backtest 章節 |
| autobacktest | 併入 portfolio/AGENTS.md §autobacktest 章節 |
| screener | 內容已於 `docs/reference/traps.md` 提及 |
| reporting | 欄位完整性契約屬跨模組內容 |
| repository | DualWriteRepository 是 generic infra |

### Batch 6：保留清單整合

確保 10 個保留的 `internal/*/AGENTS.md` 內容完整、cross-reference 正確、文件大小 ≤ 80 行（已在 PR #776 完成）。

### Batch 7：路由表更新

更新：
- 根 `AGENTS.md` 模組路由表（移除已刪除的 32 個）
- `docs/documentation-map.md`（反映新結構）
- `.claude/SKILLS-MAP.md`（新增 `atlas-retail-sentiment` skill）

---

## 5. 預期結果

| 指標 | Before | After | 變化 |
|------|--------|-------|------|
| 總 `AGENTS.md` 數 | 57 | 15 | -74% |
| `internal/*/AGENTS.md` | 50 | 10 | -80% |
| `.claude/skills/` 數 | 15 | 16 | +1（atlas-retail-sentiment）|
| `docs/specs/` + `docs/guides/` | 8 | 9 | +1（retail-sentiment.md）|
| 內容丟失 | 0 | 0 | 全部遷移至 skill/specs/guides/doc.go |

---

## 6. 風險評估

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| doc.go 補充後失精簡 | 中 | 低 | 每個 doc.go 補充控制在 5-10 行 |
| 遷移時漏內容 | 中 | 高 | 採用「先建立新檔，再刪除舊檔」順序，附 diff 驗證 |
| AI 不再預讀模組 AGENTS.md 而錯過陷阱 | 中 | 中 | 保留清單 10 個 + 既有 skill 涵蓋大部分 hot-path 陷阱 |
| skill 內容未涵蓋全部原始 AGENTS.md 細節 | 中 | 中 | 每個 skill 建立時做 cross-check |

---

## 7. 後續動作

待用戶審核本報告後，依下列順序執行：

1. **PR-1**：本 audit 報告（單獨 commit）
2. **PR-2**（Batch 1）：純刪除 11 個 D 類
3. **PR-3**（Batch 2）：刪除 9 個 B 類（附 skill 涵蓋驗證）
4. **PR-4**（Batch 3）：合併 5 個 A 類至 doc.go
5. **PR-5**（Batch 4）：遷移 4 個邊界 C 類
6. **PR-6**（Batch 5）：13 個精選 C 類合併至 10 個保留 + 新建 `docs/guides/retail-sentiment.md`
7. **PR-7**（Batch 6-7）：更新根 AGENTS.md + documentation-map.md + SKILLS-MAP.md

每個 PR 執行 `bash scripts/ci/check_markdown_links.sh` + `go build ./...` + `go vet ./...` + `staticcheck ./...` 驗證。
