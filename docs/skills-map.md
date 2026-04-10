# Skills Map (Current)

本文件是 atlas-go 的「現行技能地圖」，只描述目前程式與設定真正在用的技能、規則與流程。

## 1. 文件用途

- 對齊技能定義與執行行為
- 降低文件與程式脫節風險
- 讓 propose/execute/judge/decide 的責任邊界清楚可查

## 2. 真實來源（Source of Truth）

當本文與執行結果衝突時，以下內容優先：

1. `configs/agents.json`
2. `internal/orchestrator/*`
3. `internal/experiment/*`
4. `scripts/openclaw/today-start.sh`
5. `data/state/experiments/*.json`

## 3. 目前啟用技能（來自 agents.json）

| Layer | Skill |
|---|---|
| context | `taiwan_macro` |
| context | `foreign_flow` |
| sector | `semiconductor_desk` |
| sector | `ai_supply_chain_desk` |
| sector | `financials_desk` |
| sector | `shipping_desk` |
| sector | `etf_rotation_desk` |
| style | `growth_momentum` |
| style | `value_yield` |
| style | `earnings_quality` |
| style | `technical_breakout` |
| superinvestor | `druckenmiller_macro` |
| superinvestor | `aschenbrenner_ai_compute` |
| superinvestor | `baker_deep_tech` |
| superinvestor | `ackman_quality` |
| control | `cro_risk` |
| control | `cio_portfolio` |

## 4. 技能分層與責任

### 4.1 Domain Skills（研究與觀點）

| Group | Skills | 主要責任 |
|---|---|---|
| Context | `taiwan_macro`, `foreign_flow` | 給市場狀態與風險偏向 |
| Sector | `semiconductor_desk`, `ai_supply_chain_desk`, `financials_desk`, `shipping_desk`, `etf_rotation_desk` | 產業面候選與敘事約束 |
| Style | `growth_momentum`, `value_yield`, `earnings_quality`, `technical_breakout` | 進出場品質與風格濾鏡 |
| Superinvestor | `druckenmiller_macro`, `aschenbrenner_ai_compute`, `baker_deep_tech`, `ackman_quality` | 高信念補充觀點 |

### 4.2 Control Skills（風控與整合）

| Skill | 責任 |
|---|---|
| `cro_risk` | 後置風險過濾（不可產生 alpha 敘事） |
| `cio_portfolio` | 最終聚合與排序（不可繞過風控） |

### 4.3 Operating Skills（流程運作）

以下屬於流程能力，主要由 scripts/cmd 與 orchestration 實作，不是 agents.json 內的投研技能：

- replay_operator
- backtest_operator
- ledger_operator
- data_import_operator
- monitoring_operator

### 4.4 Evolution Skills（演化能力）

以下屬於實驗演化能力，主要落在 experiment/evolution 腳本與程式：

- weak_agent_selector
- prompt_mutator
- experiment_designer
- experiment_judge

## 5. Mutation Profiles（現行）

支援的 mutation type：

- `prompt_tightening`
- `risk_rule_change`
- `portfolio_constraint_revision`

### 5.1 Prompt Tightening（重點）

目前有 skill-aware 模板：

- `financials_desk`：`credit quality gate`, `spread sensitivity downgrade`, `capital adequacy premium`
- `technical_breakout`：`catch-up momentum`, `volume participation acceptance`, `close-strength tolerance`, `breakout confirmation bonus`
- 其他 skill：通用 trend/conviction  tightening 模板

實作位置：`internal/experiment/executor.go`

### 5.2 Risk Rule Change（現行參數）

`financials_desk`：

- conviction_floor: 48
- liquidity_floor: 3,500,000
- max_position_weight: 0.20
- reserve_cash_fraction: 0.12
- require_cro_pass: true

其他 skill：

- conviction_floor: 42
- liquidity_floor: 3,000,000
- max_position_weight: 0.22
- reserve_cash_fraction: 0.10
- require_cro_pass: true

實作位置：`internal/experiment/executor.go`

## 6. Judge 規則（現行）

實作位置：`internal/experiment/judge.go`

### 6.1 接受條件

- 必須 `candidate > baseline`
- 需達最小改善門檻：
  - `prompt_tightening`: 0.0005
  - `risk_rule_change`: 0.001
  - `portfolio_constraint_revision`: 0.001
- 需達觀測數門檻：
  - `level_3_regime_aware`: 12（風險/約束型 +1）
  - `level_2_window_validated`: 8（風險/約束型 +1）
  - default: 3（風險/約束型 +1）

### 6.2 檢查項（JudgeChecks）

會依 mutation type 與 target skill 檢查候選內容是否包含必要控制語句與政策欄位。

## 7. today-start Guard 與選擇器（現行）

實作位置：`scripts/openclaw/today-start.sh`

### 7.1 連敗 futility guard

- 同 `agent + window + mutation_type`
- 最近 3 筆皆 `candidate <= baseline`
- 判定為 futile

### 7.2 最小樣本門檻

- 參數：`--min-sample-for-rank N`
- 預設：`2`
- 若候選 mutation type 的歷史樣本數 `n < N`，不參與加權排名

### 7.3 加權排名公式

- 先算 `avg_delta = avg(candidate - baseline)`（lookback=5）
- 再算 `weighted = avg_delta * min(1, n/5)`
- auto-pivot 優先選 weighted 較佳者

### 7.4 執行行為

- `--no-auto-pivot`：primary 判定 futile 時直接跳過
- 預設 auto-pivot：先輸出候選排名明細，再切到較佳 mutation type
- fallback 因 futility 被跳過且開啟 auto-pivot 時：會再補跑一次 primary mutation 的 auto-agent cycle

## 8. 目前已移除的過時描述（本版不再採用）

- 「prompt_tightening 一律 no-op」：已不成立
- 舊版 aggressive risk_rule patch（conviction_floor=35 等）：已不成立
- 以 Phase 編號混合 Layer 編號的雙軌敘事：已移除

## 9. 文件維護規範

每次調整以下任一項時，必須同步更新本文件：

1. `configs/agents.json` 的技能集合
2. `executor.go` 的 mutation 模板語句
3. `judge.go` 的接受門檻或檢查項
4. `today-start.sh` 的 guard、pivot、排名參數

建議在 PR 說明附上：

- 變更前後門檻值
- 影響的 mutation type
- 一次隔離實驗（no-fallback/no-auto-pivot）結果
- 是否影響 promotion 判定
