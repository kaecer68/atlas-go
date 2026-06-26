# Handoff — atlas Fin-Skills Integration

**日期**: 2026-05-30 (updated)
**分支**: `main` @ `a7ab1dc`
**階段**: Phase A ✅ + Phase B ✅ 全部完成

---

## 已完成的（全部已 merge 進 main）

### Phase A: Fin-Skills 模型 + 評估 + 穩健性
| Package | 覆蓋率 | 對應 Fin-Skills |
|---------|--------|----------------|
| `internal/ml/` | 64.7% | SK-05 OLS, SK-06 ElasticNet, SK-08 PCR, SK-09 PLS |
| `internal/eval/` | 98.0% | SK-12 R²/Sharpe, SK-13 Permutation, SK-14 PDP |
| `internal/robustness/` | 93.7% | SK-20 SizeGroup, SK-21 PennyExclusion, SK-22 Ablation |
| `internal/backtest/` | — | SK-03 RollingSplit, BacktestPipeline |
| `internal/tax/` | — | SK-19 TaiwanCostModel (0.654%+0.3%) |

### Skills 文件
- `.claude/skills/atlas-fin-*/SKILL.md` — 4 個開發者技能（v2.0，含 Go 實作骨架）
- `.claude/skills/atlas-investor-*/SKILL.md` — 5 個投資者技能
- `.claude/skills/atlas-agent-*/SKILL.md` — 5 個 AI Agent 技能
- `docs/FIN_SKILLS_GAP_ANALYSIS.md` — 32 SK 覆蓋率矩陣
- `.claude/SKILLS-MAP.md` — 已更新到 v2.0（含全部 14 個新技能）

### Bug Fixes
- `internal/ml/pls.go` — PLS computeCoefficients 改 return error（原本 silent fallback）
- `internal/backtest/backtest_pipeline.go` — 刪除 dead ValidRange 欄位

---

## 待啟動：四個整合 Worktree（已建立）

```
/Users/kaecer/workspace/
├── atlas/                     # main (已 merge 完，clean)
├── atlas-ws-backtest-cli/     # WT A: cmd/backtest-pipeline CLI
├── atlas-ws-judge-eval/       # WT B: experiment judge + eval integration
├── atlas-ws-orch-ml/          # WT C: orchestrator ML scoring
└── atlas-ws-retrain/          # WT D: ML retrain scheduler
```

每個 worktree 目錄下有 `PROMPT.md`，包含完整的任務說明。

### WT A: Backtest Pipeline CLI（最簡單，建議先跑）
- 新增 `cmd/backtest-pipeline/main.go`
- 讓 BacktestPipeline 有 CLI 入口
- 問題：sample CSV 只有 7 天 → 需要 `--synthetic` flag 生成合成資料
- 合成資料規格：2005-2022 每日 DailyBar，已知 y = 2*momentum + 3*value + noise
- 驗證 OLS 還原 β ≈ [2.0, 3.0]

### WT B: Judge eval Integration
- 改 `internal/experiment/judge.go` + `oos_validator.go`
- 引入 `internal/eval/` 的 OOSR2/SharpeRatio/MaxDrawdown
- 不破壞現有 API

### WT C: Orchestrator ML Scoring（最複雜）
- 新增 `internal/orchestrator/ml_scorer.go`
- 修改 `collectRecommendations()` 加入 ML scoring path
- 透過 `config.ParametersConfig.UseMLScoring` 控制（預設 false）
- 修改 `internal/config/parameters.go` + `parameters_defaults.go`

### WT D: ML Retrain Scheduler
- 新增 `internal/scheduler/ml_retrain.go`
- 修改 `cmd/atlas/main.go` 的 BackgroundTaskManager 註冊區
- 每 24h 自動重訓 OLS/ElasticNet/PCR/PLS

---

## 擱置（Backlog）
- ElasticNet `searchAlpha()` manual 3-fold → 應改用 `KFoldSplitter`（code cleanliness，非 bug）
- SK-07 GLM spline, SK-10 RandomForest, SK-11 NeuralNet（下一批 ML 模型）
- SK-24~28 PPO RL 全線

---

## 啟動方式

```bash
# 在新 OpenCode CLI session 中：

# 最簡單的先跑
cd /Users/kaecer/workspace/atlas-ws-backtest-cli
cat PROMPT.md   # 複製全文貼到 OpenCode

# 其他三個可以併行
cd /Users/kaecer/workspace/atlas-ws-judge-eval && cat PROMPT.md
cd /Users/kaecer/workspace/atlas-ws-orch-ml && cat PROMPT.md
cd /Users/kaecer/workspace/atlas-ws-retrain && cat PROMPT.md
```

## 合併策略
四個 worktree 改不同目錄，零 merge conflict。完成後依任意順序合併進 main。

## Fin-Skills 原始規格
`/Users/kaecer/workspace/Fin-Skills/Fin-Skills.md` — 32 個 SK 完整規格
