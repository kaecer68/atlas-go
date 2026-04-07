# Atlas OpenClaw 腳本使用教學

本指南提供每個腳本的詳細使用方法、實際範例和常見問題解答。

## 目錄

1. [status.sh - 狀態報告](#statussh---狀態報告)
2. [propose-mutation.sh - Mutation 建議](#propose-mutationsh---mutation-建議)
3. [execute-next.sh - 執行實驗](#execute-nextsh---執行實驗)
4. [judge-latest.sh - 判斷實驗](#judge-latestsh---判斷實驗)
5. [decide.sh - 決策執行](#decidesh---決策執行)
6. [完整工作流程範例](#完整工作流程範例)
7. [常見問題](#常見問題)

---

## status.sh - 狀態報告

### 用途
查看系統當前完整狀態，了解該做什麼。

### 基本用法

```bash
./scripts/openclaw/status.sh
```

### 輸出說明

```
╔════════════════════════════════════════════════╗
║        Atlas OpenClaw Status Report            ║
╚════════════════════════════════════════════════╝

=== Baseline Policy Status ===
✓ Baseline policy exists (version 5)    ← 當前版本
  Version: 5
  Last updated: 2026-03-30 10:30:00

=== Experiment Status ===
  Total experiments: 15                    ← 實驗統計
  📝 Planned: 2                           ← 等待執行
  🔄 Running: 1                           ← 進行中
  ✅ Accepted: 10                         ← 已接受
  ❌ Rejected: 2                          ← 已拒絕
⚠ 1 experiment(s) currently running       ← 注意事項

=== Replay Data Status ===
✓ Replay data available (3 files)         ← 數據狀態
  tw_open_data.jsonl (1.6M)
  twse_202603.csv (2.3M)
  tpex_202603.csv (1.1M)

=== Weakest Agent Analysis ===
  Analyzing recommendation outcomes...
  ⚠ growth-momentum-01 shows lowest Sharpe  ← 最需要改進

=== Recent Activity (Last 24h) ===
  Recent modifications: 8 files
  - baseline_policy.json (Mar 30 09:15)
  - experiments.jsonl (Mar 30 09:10)

=== Recommended Next Action ===
⚠ Finish running experiment first:        ← 系統建議
  ./scripts/openclaw/judge-latest.sh

=== Quick Commands ===
  ./scripts/openclaw/status.sh
  ./scripts/openclaw/propose-mutation.sh
  ./scripts/openclaw/execute-next.sh
  ./scripts/openclaw/judge-latest.sh
  ./scripts/openclaw/decide.sh
```

### 什麼時候使用

- ✅ 每天開始工作時
- ✅ 不確定該做什麼時
- ✅ 系統異常排查時
- ✅ 想要全局概覽時

---

## propose-mutation.sh - Mutation 建議

### 用途
分析 weakest agent，生成 mutation 改進建議。

### 基本用法

**互動模式（推薦給人類）**
```bash
./scripts/openclaw/propose-mutation.sh
```

流程：
1. 系統自動找出 weakest agent
2. 顯示當前 prompt
3. 生成 mutation brief
4. 詢問是否保存

**自動模式（推薦給 OpenClaw）**
```bash
# 自動選擇 weakest agent
./scripts/openclaw/propose-mutation.sh --auto

# 指定特定 agent
./scripts/openclaw/propose-mutation.sh --auto --agent growth-momentum-01

# 預覽不保存（dry-run）
./scripts/openclaw/propose-mutation.sh --auto --dry-run
```

### 輸出範例

```
╔════════════════════════════════════════════════╗
║     OpenClaw Mutation Proposal Assistant       ║
╚════════════════════════════════════════════════╝

✓ Found prompt file: prompts/agents/growth_momentum.md

Current prompt (first 20 lines):
---
# Growth Momentum Agent

## Role
Identify stocks with strong momentum supported by earnings...
---

Generating mutation proposal...

{
  "id": "brief-growth-momentum-01-1774800459",
  "target_agent": "growth-momentum-01",
  "timestamp": "2026-03-30T10:30:00Z",
  "hypothesis": "Refine growth-momentum-01 prompt to improve...",
  "suggested_mutation_type": "prompt_tightening",
  "rationale": [
    "Agent shows lower Sharpe ratio compared to peers",
    "Recommendations may lack sufficient conviction filtering"
  ],
  "proposed_changes": [
    "Add explicit conviction threshold requirements",
    "Clarify regime-aware downgrade rules"
  ],
  "acceptance_criteria": [
    "Sharpe-like score improvement > 0.001",
    "No material drawdown degradation"
  ]
}

Save this brief? [Y/n]: y

✓ Mutation brief saved to: data/state/mutation-briefs/brief-growth-momentum-01-1774800459.json

Next Steps:
1. Review the generated brief
2. Execute experiment:
   ./scripts/openclaw/execute-next.sh
```

### 什麼時候使用

- ✅ 系統建議「Start new iteration cycle」時
- ✅ 發現某個 agent 持續表現不佳時
- ✅ 想要改進特定策略時

---

## execute-next.sh - 執行實驗

### 用途
執行下一個準備好的實驗（planned 狀態或 mutation brief）。

### 基本用法

**互動模式**
```bash
./scripts/openclaw/execute-next.sh
```

**指定 mutation brief**
```bash
./scripts/openclaw/execute-next.sh --brief data/state/mutation-briefs/brief-xxx.json
```

**自動模式**
```bash
./scripts/openclaw/execute-next.sh --auto
```

### 執行流程

```
╔════════════════════════════════════════════════╗
║     OpenClaw Experiment Executor               ║
╚════════════════════════════════════════════════╝

Found planned experiment: exec-growth-momentum-01-1774800459

Experiment Information:
  ID: exec-growth-momentum-01-1774800459
  Agent: growth-momentum-01
  Skill: growth_momentum
  Mutation Type: prompt_tightening

Execute this experiment? [Y/n]: y

Executing experiment: exec-growth-momentum-01-1774800459
Found experiment file: data/state/experiments/exec-growth-momentum-01-1774800459.json
Command: go run ./cmd/execute-experiment

[執行中...]

✓ Experiment execution started

Next steps:
  1. Wait for execution to complete
  2. Run: ./scripts/openclaw/judge-latest.sh
```

### 什麼時候使用

- ✅ status.sh 顯示有 planned experiments 時
- ✅ 剛生成 mutation brief 後
- ✅ 想要執行特定 brief 時

---

## judge-latest.sh - 判斷實驗

### 用途
判斷最新完成的實驗，提供決策建議。

### 基本用法

**互動模式**
```bash
./scripts/openclaw/judge-latest.sh
```

**JSON 輸出（供 OpenClaw 使用）**
```bash
./scripts/openclaw/judge-latest.sh --json
```

**自動模式**
```bash
./scripts/openclaw/judge-latest.sh --auto
```

### 輸出範例

```
╔════════════════════════════════════════════════╗
║       OpenClaw Experiment Judge                ║
╚════════════════════════════════════════════════╝

Latest experiment: exec-growth-momentum-01-1774800459

Experiment Results:
  Experiment ID: exec-growth-momentum-01-1774800459
  Agent: growth-momentum-01
  Mutation Type: prompt_tightening
  Status: accepted

Performance Comparison:
  Baseline:  1.2345
  Candidate: 1.2987
  Improvement: 0.0642
  % Change: 5.2%

Acceptance Checks:
  ✓ contains stronger trend confirmation rule
  ✓ contains conviction downgrade logic
  ✓ required skill policy checks preserved

Recommendation:
  Action: --promote exec-growth-momentum-01-1774800459
  Reason: Experiment passed all acceptance gates

Next step:
  ./scripts/openclaw/decide.sh --promote exec-growth-momentum-01-1774800459 --reason "Experiment passed all acceptance gates"
```

### JSON 輸出範例

```bash
$ ./scripts/openclaw/judge-latest.sh --json
{
  "recommendation": "--promote exec-growth-momentum-01-1774800459",
  "reason": "Experiment passed all acceptance gates",
  "status": "accepted",
  "experiment_id": "exec-growth-momentum-01-1774800459"
}
```

### 什麼時候使用

- ✅ execute-next.sh 執行完成後
- ✅ status.sh 顯示有 completed experiments 時
- ✅ 想要了解實驗結果時

---

## decide.sh - 決策執行

### 用途
執行最終決策：promote 或 revert，帶有安全檢查和確認流程。

### Promote 用法

**基本 promote**
```bash
./scripts/openclaw/decide.sh --promote exec-growth-momentum-01-1774800459 \
  --reason "Improved Sharpe by 5.2% with no drawdown degradation"
```

**預覽（dry-run）**
```bash
./scripts/openclaw/decide.sh --promote exec-growth-momentum-01-1774800459 \
  --reason "..." --dry-run
```

**自動確認（OpenClaw 使用）**
```bash
./scripts/openclaw/decide.sh --promote exec-growth-momentum-01-1774800459 \
  --reason "..." --yes
```

### Revert 用法

**回滾到上一版本**
```bash
./scripts/openclaw/decide.sh --revert --reason "Unexpected drawdown increase"
```

**回滾到指定版本**
```bash
./scripts/openclaw/decide.sh --revert 3 --reason "Version 3 was more stable"
```

**查看歷史**
```bash
./scripts/openclaw/revert-baseline --list
```

### 輸出範例

**Promote 成功**
```
╔════════════════════════════════════════════════╗
║      OpenClaw Decision Assistant               ║
╚════════════════════════════════════════════════╝

Experiment Information:
  ID: exec-growth-momentum-01-1774800459
  Agent: growth-momentum-01

Running safety checks...
  ✓ Experiment status: accepted
  ✓ Has candidate metrics
  ✓ Reason provided
Safety: 3/3 checks passed

Confirm promotion? [y/N]: y

Executing promotion...
baseline_policy: data/state/baseline_policy.json
version: 6
prompt_overrides: 3
promotions: 6
require_cro_pass: true
conviction_floor: 50

✓ Promotion complete
Reason logged: Improved Sharpe by 5.2% with no drawdown degradation
```

**Revert 成功**
```
╔════════════════════════════════════════════════╗
║      OpenClaw Decision Assistant               ║
╚════════════════════════════════════════════════╝

Preparing revert...
Current version: 5
Target version: 3

⚠ Warning: Revert will roll back policy changes
Confirm revert? [y/N]: y

Executing revert...
baseline_policy: data/state/baseline_policy.json
reverted_from_version: 5
reverted_to_version: 3
reverted_experiments: 2
reason: Version 3 was more stable
reverted_at: 2026-03-30T10:45:00

reverted_experiment_ids:
  - exp-growth-momentum-01-1774800300
  - exp-growth-momentum-01-1774800400

✓ Revert complete
```

### 什麼時候使用

- ✅ judge-latest.sh 建議 promote 時
- ✅ 發現問題需要回滾時
- ✅ 想要回退到穩定版本時

---

## 完整工作流程範例

### 場景 1：人類主導的改進循環

```bash
# Step 1: 查看狀態
./scripts/openclaw/status.sh
# 輸出顯示：⚠ growth-momentum-01 shows lowest Sharpe
#          Recommended: Start new iteration cycle

# Step 2: 生成 mutation 建議
./scripts/openclaw/propose-mutation.sh
# 系統找出 growth-momentum-01 是最弱的
# 顯示 mutation brief
# 確認保存

# Step 3: 執行實驗
./scripts/openclaw/execute-next.sh
# 確認執行
# 等待完成

# Step 4: 判斷結果（稍後執行）
./scripts/openclaw/judge-latest.sh
# 顯示：Candidate improved by 5.2%
#       Recommendation: promote

# Step 5: 執行 promote
./scripts/openclaw/decide.sh \
  --promote exec-growth-momentum-01-xxx \
  --reason "Improved Sharpe by 5.2%"
# 確認
# 完成！
```

### 場景 2：OpenClaw 自動化（人工確認點）

```bash
#!/bin/bash
# openclaw-daily-loop.sh

echo "=== OpenClaw Daily Loop ==="

# 1. 檢查狀態
STATUS=$(./scripts/openclaw/status.sh 2>&1)
echo "$STATUS"

# 2. 判斷是否需要新實驗
if echo "$STATUS" | grep -q "Start new iteration cycle"; then
    echo ""
    echo "🤖 OpenClaw: Proposing mutation..."
    
    # 生成 mutation brief
    ./scripts/openclaw/propose-mutation.sh --auto --dry-run
    
    # 人工確認點 1
    echo ""
    read -p "👤 Human: Approve this mutation? [y/N]: " approve
    
    if [[ "$approve" =~ ^[Yy]$ ]]; then
        # 保存並執行
        ./scripts/openclaw/propose-mutation.sh --auto
        ./scripts/openclaw/execute-next.sh --auto
        echo "✅ Experiment started"
    else
        echo "❌ Cancelled"
    fi
fi

# 3. 檢查是否有完成的實驗
if echo "$STATUS" | grep -q "completed"; then
    echo ""
    echo "🤖 OpenClaw: Judging latest experiment..."
    
    # 判斷
    JUDGMENT=$(./scripts/openclaw/judge-latest.sh --json)
    RECOMMENDATION=$(echo "$JUDGMENT" | grep -o '"recommendation": "[^"]*"' | cut -d'"' -f4)
    REASON=$(echo "$JUDGMENT" | grep -o '"reason": "[^"]*"' | cut -d'"' -f4)
    
    echo "Recommendation: $RECOMMENDATION"
    echo "Reason: $REASON"
    
    # 人工確認點 2（僅當建議是 promote 時）
    if [[ "$RECOMMENDATION" == --promote* ]]; then
        echo ""
        read -p "👤 Human: Execute recommendation? [y/N]: " execute
        
        if [[ "$execute" =~ ^[Yy]$ ]]; then
            EXP_ID=$(echo "$RECOMMENDATION" | awk '{print $2}')
            ./scripts/openclaw/decide.sh --promote "$EXP_ID" --reason "$REASON" --yes
            echo "✅ Promoted"
        else
            echo "❌ Skipped"
        fi
    fi
fi

echo ""
echo "=== Loop Complete ==="
```

---

## 常見問題

### Q: 我該如何開始第一次改進循環？

```bash
# 1. 確保有 replay 資料
ls data/replay/

# 2. 查看狀態
./scripts/openclaw/status.sh

# 3. 如果顯示 "Start new iteration cycle"
./scripts/openclaw/propose-mutation.sh
# 跟隨互動提示
```

### Q: 如何指定特定 agent 進行改進？

```bash
./scripts/openclaw/propose-mutation.sh --agent semiconductor_desk-01
```

### Q: 實驗執行後如何知道完成了？

```bash
# 方法 1：再次查看狀態
./scripts/openclaw/status.sh

# 方法 2：直接查看實驗狀態
grep '"Status":"completed"' data/state/experiments.jsonl

# 方法 3：檢查結果文件
ls -lt data/state/experiments/
```

### Q: 如果 promote 後發現問題怎麼辦？

```bash
# Step 1: 查看版本歷史
./scripts/openclaw/revert-baseline --list

# Step 2: 回滾到之前版本
./scripts/openclaw/decide.sh --revert --reason "Unexpected performance degradation"

# 或回滾到指定版本
./scripts/openclaw/decide.sh --revert 3 --reason "Version 3 was more stable"
```

### Q: OpenClaw 如何自動化執行？

使用 `--auto` 和 `--yes` 旗標：

```bash
# 自動生成 mutation（不交互）
./scripts/openclaw/propose-mutation.sh --auto

# 自動執行實驗
./scripts/openclaw/execute-next.sh --auto

# 自動判斷
./scripts/openclaw/judge-latest.sh --auto --json

# 自動 promote（謹慎使用）
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..." --yes
```

### Q: 如何預覽操作而不執行？

所有腳本都支援 dry-run：

```bash
./scripts/openclaw/propose-mutation.sh --dry-run
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..." --dry-run
./scripts/openclaw/decide.sh --revert --reason "..." --dry-run
```

### Q: 忘記加 --reason 怎麼辦？

系統會拒絕執行並提示：

```
Error: --reason is required for revert (use --dry-run to preview)
```

重新執行並加上 reason：

```bash
./scripts/openclaw/decide.sh --promote EXP-ID --reason "Your reason here"
```

---

## 總結

| 腳本 | 核心功能 | 人工確認點 |
|------|----------|------------|
| `status.sh` | 查看狀態 | 無需確認 |
| `propose-mutation.sh` | 生成改進建議 | **需要確認** |
| `execute-next.sh` | 執行實驗 | 可選確認 |
| `judge-latest.sh` | 判斷結果 | 無需確認 |
| `decide.sh` | **Promote/Revert** | **必須確認** |

**記住**：Promote 和 Revert 是單向門操作，系統強制要求 reason 和確認。
