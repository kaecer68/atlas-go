# OpenClaw 腳本快速參考卡

## 5 分鐘快速開始

```bash
# 推薦：一鍵執行完整實驗循環（90天窗口，自動判斷，自動晉升）
./scripts/openclaw/run_validated_round.sh

# 查看系統狀態
./scripts/openclaw/status.sh
```

## Mutation 類型選擇指南

| 類型 | 效果 | 建議使用場景 |
|------|------|--------------|
| `risk_rule_change` | **+40%** 平均改進 | 首選，激進風格，降低准入門檻 |
| `portfolio_constraint` | **+26%** 平均改進 | 次選，調整倉位和現金比例 |
| `prompt_tightening` | ~0% 改進 | 不推薦，僅修改提示詞無實質影響 |

```bash
# 使用特定 mutation 類型
./scripts/openclaw/run_validated_round.sh --type risk_rule_change
./scripts/openclaw/run_validated_round.sh --type portfolio_constraint --agent value-yield-01
```

## 常用命令速查

### 🧪 治理 Gate 深度驗證（推薦）

```bash
# 一鍵驗證 G2/G3/G4：determinism + hard guard + trace/dashboard
./scripts/openclaw/verify_governance_gates.sh

# 指定 replay 與視窗
./scripts/openclaw/verify_governance_gates.sh \
	--replay-data samples/replay/twse_stock_day_all_sample.csv \
	--start 2026-03-26 \
	--end 2026-03-27

# 人工審核事件契約 + 重播驗證
./scripts/openclaw/verify_human_approval_event.sh

# M5 parallel simulation verification (base/stress/shock + determinism)
./scripts/openclaw/verify_parallel_scenarios.sh

# M5 strict mode: fail when scenarios are not distinguishable
./scripts/openclaw/verify_parallel_scenarios.sh --require-diversity

# Unified governance strict mode (G2/G3/G4 + M5 + M7)
./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity

# M8 operations gate (runbook + monitoring config + rollback drill)
./scripts/openclaw/verify_operations_gate.sh

# M8 + strict governance in one run
./scripts/openclaw/verify_operations_gate.sh --with-governance

# Guided branch protection setup (default dry-run)
./scripts/openclaw/setup_branch_protection.sh

# Apply branch protection after review
./scripts/openclaw/setup_branch_protection.sh --apply

# Verify branch protection script contract (required-reviews 0..6)
./scripts/openclaw/verify_branch_protection_script.sh

# Apply with custom snapshot backup directory
./scripts/openclaw/setup_branch_protection.sh --apply --backup-dir data/state/custom-branch-protection-backups

# Restore branch protection from snapshot (safe preview)
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json

# Restore from snapshot and apply
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json --apply
```

### 🔍 狀態檢查

```bash
./scripts/openclaw/status.sh              # 完整狀態報告
./scripts/openclaw/revert-baseline --list # 查看版本歷史
```

### 📊 資源監控與輪次管理

```bash
# 資源檢查（CPU/內存/磁盤）
./scripts/monitor/resource_guard.sh check           # 檢查資源狀態
./scripts/monitor/resource_guard.sh check --json     # JSON格式輸出

# 輪次追蹤與停止條件
./scripts/monitor/round_tracker.sh check    # 檢查是否應停止
./scripts/monitor/round_tracker.sh stats    # 查看輪次統計
./scripts/monitor/round_tracker.sh reset    # 重置追蹤器
```

**停止條件（平衡模式）**：
- 總輪次達到 20 輪
- 連續 3 輪被拒絕
- 接受率低於 15%
- CPU > 75% 或內存 > 80%

配置文件：`configs/monitor_limits.json`

### 📝 生成改進建議
```bash
# 互動模式（推薦）
./scripts/openclaw/propose_mutation.sh

# 自動模式
./scripts/openclaw/propose_mutation.sh --auto

# 指定 agent
./scripts/openclaw/propose_mutation.sh --agent growth-momentum-01
```

### 🚀 執行實驗
```bash
# 執行下一個
./scripts/openclaw/execute_next.sh

# 執行特定 brief
./scripts/openclaw/execute_next.sh --brief path/to/brief.json
```

### ⚖️ 判斷結果
```bash
# 互動判斷
./scripts/openclaw/judge_latest.sh

# JSON 輸出
./scripts/openclaw/judge_latest.sh --json
```

### ✅ Promote（接受改進）
```bash
# 基本用法
./scripts/openclaw/decide.sh --promote EXP-ID --reason "Improved Sharpe by X%"

# 人工審核入口（推薦）
./scripts/openclaw/human_approval.sh --approve --experiment EXP-ID --reason "Passes G2/G3/G4"

# 預覽
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..." --dry-run
```

### ⏪ Revert（回滾）
```bash
# 回滾到上一版本
./scripts/openclaw/decide.sh --revert --reason "Unexpected drawdown"

# 人工審核入口（推薦）
./scripts/openclaw/human_approval.sh --revert --reason "Rollback after post-promo alert"

# 回滾到指定版本
./scripts/openclaw/decide.sh --revert 3 --reason "Version 3 more stable"

# 查看歷史
./scripts/openclaw/revert-baseline --list
```

## 標準工作流程

### 人類主導模式
```bash
# Step 1: 查看狀態
./scripts/openclaw/status.sh

# Step 2: 生成 mutation（互動）
./scripts/openclaw/propose_mutation.sh

# Step 3: 執行實驗
./scripts/openclaw/execute_next.sh

# Step 4: 判斷結果（稍後）
./scripts/openclaw/judge_latest.sh

# Step 5: Promote/Revert
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..."
```

### OpenClaw 輔助模式
```bash
# Step 1: 狀態檢查
./scripts/openclaw/status.sh

# Step 2: OpenClaw 生成建議
./scripts/openclaw/propose_mutation.sh --auto --dry-run
# 👤 人工確認

# Step 3: 執行
./scripts/openclaw/propose_mutation.sh --auto
./scripts/openclaw/execute_next.sh --auto

# Step 4: OpenClaw 判斷
./scripts/openclaw/judge_latest.sh --json
# 👤 人工確認 promote

# Step 5: 執行決策
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..." --yes

# 或使用人工審核入口（會寫入 approvals 稽核事件）
./scripts/openclaw/human_approval.sh --approve --experiment EXP-ID --reason "..." --yes

# 從稽核事件重播（建議先 dry-run）
./scripts/openclaw/replay_approval_event.sh --event data/state/approvals/<decision-file>.json --dry-run
```

## 故障排除

| 問題 | 解決方法 |
|------|----------|
| 不知道該做什麼 | `./scripts/openclaw/status.sh` |
| 實驗卡住 | `grep '"Status":"running"' data/state/experiments.jsonl` |
| 忘記版本號 | `./scripts/openclaw/revert-baseline --list` |
| promote 錯誤 | 加上 `--dry-run` 預覽 |

## 檔案位置

```
data/
├── state/
│   ├── baseline_policy.json      # 當前策略版本
│   ├── experiments.jsonl         # 實驗記錄
│   └── experiments/              # 實驗結果
├── replay/                       # 回放數據
└── mutation-briefs/              # Mutation 建議
```

## 安全機制

- ✅ **Promote/Revert 必須提供 `--reason`**
- ✅ **預設需要確認**（可加 `--yes` 跳過）
- ✅ **支援 `--dry-run` 預覽**
- ✅ **所有操作可追蹤**（記錄在 experiments.jsonl）

## 需要幫助？

```bash
# 任何腳本加上 --help
./scripts/openclaw/status.sh --help
./scripts/openclaw/propose_mutation.sh --help
./scripts/openclaw/decide.sh --help

# 詳細教學
# script-usage-guide.md 已移入 .omo/handoffs/（OpenClaw 退役）

# 協議文件
# 2026-06-15-openclaw-protocol.md 已刪除（OpenClaw 退役）
```

---

**記住**：Promote 和 Revert 是單向門，系統強制要求 reason 和確認！
