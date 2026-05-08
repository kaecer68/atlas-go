# Operations Playbook

## Purpose

This document explains how to operate `atlas-go` correctly day to day.

## Operating Modes

### 1. Single Session Replay

Use when:

- validating one replay date
- checking agent output shape
- verifying ledger output
- testing one importer or one prompt adjustment

Core command:

```bash
go run ./cmd/atlas
```

### 2. Replay Import

Use when:

- normalizing TWSE or TPEX open-data files
- preparing replay-ready datasets
- moving from raw CSV into internal JSONL

Core command:

```bash
go run ./cmd/import-replay -source samples/replay/twse_stock_day_all_sample.csv -target data/replay/tw_open_data.jsonl
```

### 3. Window Backtest

Use when:

- evaluating agent behavior over a period
- choosing the weakest agent
- generating mutation candidates

Core command:

```bash
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

## Standard Operating Procedure

1. Verify the agent registry path and replay data path.
2. Confirm the replay date or backtest window.
3. Run import if the source data is still raw.
4. Run a replay session or window backtest.
5. Inspect:
   - session summary
   - outcomes
   - experiments
   - weakest-agent output
6. Decide whether the result is exploratory or ready for mutation design.

For mutation runs, prefer explicit mode selection:

- isolated validation mode: `--no-fallback --no-auto-pivot`
- guarded throughput mode: default auto-pivot with `--min-sample-for-rank <n>`

## Dashboard Operations

The Unified Control Tower (`web/static/index.html`) is the primary human-AI interaction surface. Use it for real-time monitoring, approval, and intervention.

### Entry point

Open `http://localhost:8080/` after starting `go run ./cmd/atlas -api`.

### Page workflow (decision logic)

Operate the dashboard left-to-right in normal conditions:

1. **總覽** -- 快速掌握系統健康度
2. **宏觀敘事** -- 理解驅動今日 regime 的宏觀故事
3. **相對趨勢** -- 確認熔斷機制、投資組合損益與總經雷達
4. **投資管線** -- 審查個別推薦並執行人為過濾
5. **AI 觀測台** -- 檢視最弱 agent 成績卡與標的池重疊
6. **模擬交易** -- 評判結果、比對 diff、晉升已通過的候選
7. **最新回測** -- 閱讀最近一次回測窗口的完整報告與下載連結
8. **控制與稽核** -- 執行覆寫、管理基線版本、閱讀稽核紀錄

### 總覽快速判讀

本頁是基於最新回測窗口計算出的**系統狀態快照**，不是即時行情，而是 Atlas 在指定 replay 資料區間內，透過多層 AI Agent 模擬與風控後匯總出的「當前體制判斷」。

7 張 KPI cards 依 top-down 順序排列：

- **資料時間** -- 回測資料的最新日期與窗口生成時間，放在首列以宣告「這組數據所對應的時間線」。
- **基線版本** -- 現行生效的 baseline policy 版本號，說明當前統計是基於哪一版政策運算出來的。
- **敘事脈絡** -- 當前最活躍的宏觀事件主題、外資出逃指數分數/等級，以及前往宏觀敘事頁籤的捷徑。宏觀脈絡決定所有下游的倉位規模與過濾條件。
- **市場狀態** -- 當前市場 regime（RISK_ON／NEUTRAL／RISK_OFF）。 regime 文字本身以色呈現：綠色代表風險偏好、積極配置（RISK_ON），黃色代表中性謹慎（NEUTRAL，倉位上限 85%），紅色代表風險趨避、建議降低曝險（RISK_OFF）。點擊卡片可查看三種體制的詳細解讀與操作建議。
- **最弱 Agent** -- 下一個突變候選人及其 Sharpe-like 指標。
- **實驗狀態** -- 待評判 / 待晉升的數量。
- **擁擠標的** -- 當 >=3 個 agents 重疊在同一標的，或 style-layer 標的池重疊過高時標示。

點擊任何一張分析卡片（除資料時間與基線版本外）會彈出說明視窗，解釋該指標的背後意義、對投資決策的影響，以及下一步該注意什麼。

### 宏觀敘事頁籤

在決定倉位規模或產業配置前閱讀：

- **總經快照** -- DXY-美元指數、US10Y-美債10年期、VIX-波動率指數、USD/TWD-匯率、原油、黃金、日圓，以及三大法人資金流數據。面板頂部有資料通道健康度燈號（🟢 正常 / 🟡 延遲 / 🔴 缺失），三大法人資金流也有自己的獨立燈號與更新時間，用於即時判斷宏觀數據採集是否及時。
- **外資出逃指數** -- 0–100 分，由 6 大子項加權組成，反映外資撤離台灣市場的壓力程度：DXY-美元指數（15%）、US10Y-美債10年期（20%）、外資流向（25%）、VIX-波動率指數（15%）、日圓-套利平倉壓力（10%）、地緣政治風險（15%）。等級標示為低壓 / 警戒 / 高壓 / 危機。在高壓 / 危機 regime 下，可考慮降低曝險或收緊控制層過濾條件。
- **敘事事件 / 因果傳導鏈 / 投資模型** -- 解釋當前 regime 為何被如此評分，以及哪些產業被看好或應迴避。因果傳導鏈中的主題 ID 與名稱均已本地化為中文，每個步驟都會標示影響的板塊標籤；負數影響力以紅字呈現。

### 相對趨勢頁籤

- 頁面頂端有控制層處置結果的解讀說明，強調這一頁呈現的是「風控長／投資長對 AI 推薦的最終處置結果」。
- **總經敘事脈絡橫幅**（雷達上方）重複顯示壓力分數、主要事件、情緒方向與壓力等級建議，並根據當前 narrative model 自動列出看多/看空板塊，讓你在監控即時執行時不需要來回切換到宏觀敘事頁籤。
- 總經雷達顯示最新回測場次的 regime、guard 處置紀錄（放行/過濾/阻擋），以及最終放行標的的前 5 檔清單（含公司名稱、信念、遠期報酬）。
- 即時狀態欄寬縮小為 170px；若系統處於 Simulation 模式，會顯示「目前以 replay 資料進行回測模擬，未連接 live broker」的說明，而非空白「無資料」。

### 投資管線頁籤

本頁呈現的是**最新回測場次中，經過控制層審核後的推薦標的清單**。這不是即時行情，而是 Atlas 在指定 replay 區間內模擬與風控後的結果。

#### 預設視圖與完整視圖

- **預設**：僅顯示 `passed_guards=true`（控制層已放行）的推薦。
- **顯示全部被過濾項目**：勾選後會額外出現紅色邊框的列，這些是被 CRO 或 CIO 擋下的推薦。

#### 欄位說明

表格包含標的、公司名稱（由前端 `STOCK_NAME_MAP` 映射 24 檔常見台股）、策略來源（Agent + Skill）、來源層、方向、收盤價、目標價、停損價、信念、隔日回測報酬、價量標籤與推薦理由。

- **信念**與**隔日回測報酬**欄位標題旁有 ℹ️，點擊可查看數值意義。
- **擁擠標籤**（`warn`）當原因包含 `[crowded:N agents]` 時出現，表示 CIO 層已對該標的套用信念懲罰。

#### 人工覆寫操作（可選）

投資管線提供三種按鈕，讓操作者對 AI 推薦進行單筆覆寫。這些動作會寫入控制稽核紀錄，並在後續回測執行時生效。**不進行任何操作不會造成錯誤。**

| 按鈕 | 出現條件 | 語義與後續影響 |
|------|----------|----------------|
| **放行** | 僅對 `passed_guards=true` 的列顯示 | 人工背書此推薦，後續回測不會將它濾除。 |
| **否決** | 僅對 `passed_guards=true` 的列顯示 | 人工拒絕此推薦，後續回測會強制排除該 (標的, Agent) 組合。 |
| **補追** | 僅對 `passed_guards=false`（被過濾）的列顯示 | 語義同「放行」——對已被控制層擋下的標的進行人工強制納入。 |

**使用時機舉例**：
- **放行**：某推薦被 CIO 因信念稍低而擋下，但你基於外部消息（如新聞、財報）認為應該進場。
- **否決**：某 Agent 推薦了你投資紀律中絕不碰的標的，或出現系統尚未反應的負面事件。
- **補追**：某標的被風控長過濾，但你判斷該過濾條件在當下過於保守，決定強制納入。

這三個按鈕本質上是同一套人工覆寫機制的三種語境表現；預設情況下（不做任何點擊），系統完全依照控制層自動規則運行。

### AI 觀測台頁籤

- Agent 觀測台：完整的成績卡表格（窗口數、命中率、Sharpe、最大回撤、Darwinian 權重）。
- 標的池重疊：顯示每個 agent 的標的池與 style-layer 重疊矩陣。重疊 >=3 檔標的者以黃色高亮。

### 模擬交易頁籤

- **評判** 直接從收件匣對待評判實驗進行評判。
- **差異** 開啟並排的基線與候選 prompt 比對。
- **晉升** 將已接受的實驗移入現行基線政策（寫入 `data/state/baseline_policy.json`）。
- 晉升歷史顯示過去接受/拒絕的紀錄與版本號。

### 最新回測頁籤

- 顯示最近一次回測窗口的詳細報告、績效摘要與 Markdown 下載連結。

### 控制與稽核頁籤

- **資料採集健康度** -- 集中監控 10 項宏觀與資金流資料來源的即時狀態（DXY、US10Y、VIX、匯率、原油、黃金、日圓、三大法人）。綠燈表示 24 小時內有更新，黃燈表示延遲，紅燈表示缺失。大面積紅燈時應檢查網路連線或資料供應商（Yahoo Finance / TWSE）狀態。
- **Agent 覆寫** -- 暫停 / 恢復個別 agents。
- **產業封鎖** -- 封鎖或解除產業（例如 `半導體`）。
- **基線管理** -- 從下拉選單晉升實驗，或附帶原因回滾到先前版本。
- **人工干預紀錄** -- 完整的批准、拒絕、暫停、封鎖與回滾稽核軌跡。


## Operator Techniques

### Start small

Use one replay date first. Confirm the ledger and summary artifacts before running wider windows.

### Keep raw and normalized data separate

- raw files: source dumps
- normalized JSONL: replay-ready internal format

This keeps importer bugs from polluting analysis.

### Treat session artifacts as evidence

The files in `data/state/sessions/<session-id>/` are not logs for decoration. They are the evidence trail for why a future prompt mutation was justified.

### Respect sample-size limits

If only one or two sessions exist, use the result for orientation, not for strong model claims.

When running `today-start`, sample size also affects mutation-type ranking:

- mutation types with insufficient historical sample count are excluded from weighted ranking
- raise `--min-sample-for-rank` when you prefer conservative switching
- lower it only for exploratory search, and treat outcomes as low-confidence

### Understand guard outcomes

`today-start` can skip or switch before execution:

- `Primary mutation marked futile...`: recent runs for that mutation type are all non-improving in the same window
- `Primary cycle skipped due to futility guard...`: skip path (usually with `--no-auto-pivot`)
- `[pivot] Switching primary mutation type...`: auto-pivot picked an alternative using weighted ranking

Interpret these as control signals, not errors.

### Read the weakest-agent result with context

The weakest agent is a candidate for investigation, not automatic proof that the prompt is bad. Check:

- number of observations
- regime context
- concentration of failures
- data completeness
- required skills and forbidden actions from the registry policy

## Artifacts Checklist

For a healthy run, expect:

- `recommendation_outcomes.jsonl`
- `experiments.jsonl`
- `summary.json`
- window summary when a backtest window is run

## Failure Handling

If a run looks wrong, inspect in this order:

1. replay source path
2. session date and forward-return availability
3. registry load path
4. outcome file contents
5. weakest-agent selection logic

If mutation flow behaves unexpectedly, also inspect:

6. futility guard status in `scripts/openclaw/today_start.sh`
7. `--min-sample-for-rank` value and candidate sample counts (`n` in pivot logs)

## Baseline Promotion

After an experiment is accepted:

1. Promote the accepted result into the baseline policy store.
2. Confirm the baseline policy version and promotion history changed.
3. Re-run replay or backtest commands so the next cycle uses the promoted baseline.

This keeps runtime execution, replay compare, and future mutations aligned to the same formal baseline.

## Human Approval Workflow

Use the human-in-the-loop wrapper as the default decision entrypoint for promote/reject/revert.

### Decision Entry

```bash
# approve and promote
./scripts/openclaw/human_approval.sh --approve --experiment <exp-id> --reason "Passes replay and guard gates"

# reject (audit-only)
./scripts/openclaw/human_approval.sh --reject --experiment <exp-id> --reason "Insufficient improvement evidence"

# revert baseline
./scripts/openclaw/human_approval.sh --revert --reason "Rollback after post-promotion alert"
```

### Audit Artifact Check

Each decision writes one event file under `data/state/approvals/`.
Validate that the event contains required fields:

- `decision_id`
- `timestamp`
- `actor`
- `action`
- `reason`
- `dry_run`

### Event Replay (Dry-Run First)

Use approval event replay to verify the decision can be reconstructed from audit artifacts:

```bash
# replay one stored approval/reject/revert event without state mutation
./scripts/openclaw/replay_approval_event.sh --event data/state/approvals/<decision-file>.json --dry-run
```

### One-Command Verification

Run the dedicated checker when changing decision scripts or event schema:

```bash
./scripts/openclaw/verify_human_approval_event.sh
```

This verifies:

- event JSON schema fields are present and correctly typed
- event file persistence matches emitted decision payload
- replay wrapper can reconstruct and execute a dry-run decision from stored event

### CI Gate Requirement

Governance and operations verifiers are enforced in CI as dedicated jobs in `.github/workflows/ci.yml`:

- workflow: `ci`
- job: `governance`
- job: `operations`

For branch protection, require both status checks `ci / governance` and `ci / operations` so promote/reject/revert logic, replay determinism, M5 scenario verification, and M8 operations drills cannot regress silently.

### Branch Protection Setup (GitHub)

Preferred path (automation + guided approval):

```bash
# default: dry-run, show current config, options, and risk notes
./scripts/openclaw/setup_branch_protection.sh

# apply after reviewing prompts and confirmation phrase
./scripts/openclaw/setup_branch_protection.sh --apply
```

The setup script includes anti-misconfiguration checks:

- always starts in dry-run mode
- shows current protection config before proposing changes
- explains option-level trade-offs and risk consequences
- requires explicit final confirmation before apply
- creates a pre-apply snapshot under `data/state/branch-protection-snapshots/`

Optional snapshot location override:

```bash
./scripts/openclaw/setup_branch_protection.sh --apply --backup-dir data/state/custom-branch-protection-backups
```

Restore from a previous snapshot:

```bash
# preview restore payload and risk notes (dry-run)
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json

# apply restore (requires explicit confirmation phrase)
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json --apply
```

Restore mode anti-misconfiguration checks:

- snapshot file must exist and include `owner/repo/branch`
- snapshot target must match current repository and branch
- snapshot must contain a valid `protection` object
- restore mode still requires explicit human confirmation before apply

Recommended repository setting path:

1. GitHub repository -> Settings -> Branches
2. Add or edit branch protection rule for `main`
3. Enable `Require status checks to pass before merging`
4. Select required checks:
   - `ci / governance`
   - `ci / operations`
5. Optional but recommended:
   - Enable `Require branches to be up to date before merging`
   - Enable `Require conversation resolution before merging`
6. Save rule and verify by opening a test PR

Quick verification checklist after saving:

- PR Checks tab shows both `ci / governance` and `ci / operations`
- Merge button stays blocked until both checks pass
- Failed operations or governance jobs block merge as expected

The CI governance job runs strict mode by default:

```bash
./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
```

Use this strict mode after scenario design is calibrated for your replay window.

## Operations Gate (M8)

Use the operations gate verifier for staging-safe production-readiness checks:

```bash
./scripts/openclaw/verify_operations_gate.sh
```

What it checks:

- runbook command coverage for rollback and replay workflow
- Prometheus config sanity for atlas metrics scraping
- dry-run rollback drill via human approval event + replay
- human approval event schema/replay contract

Optional deep mode:

```bash
./scripts/openclaw/verify_operations_gate.sh --with-governance
```

Use `--with-governance` when you want to chain M8 checks with strict governance verification in one run.

## Rollback and Replay Workflow

### Revert Decision

To record a revert decision for an experiment:

```bash
./scripts/openclaw/human-approval.sh --revert <experiment-id> --reason "performance regression"
```

### Approval Event Replay

To replay an approval event for testing:

```bash
./scripts/openclaw/replay-approval-event.sh --event <event-file>
```

### Approval Event Verification

To verify approval event schema and replay contract:

```bash
./scripts/openclaw/verify-human-approval-event.sh
```

### Strict Governance Gate

To run strict governance verification with scenario diversity:

```bash
./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity
```
