# OpenClaw Operations Protocol

本文件定義 OpenClaw（或人類操作者）與 atlas-go 系統的互動協議。

## 快速開始

```bash
# 1. 查看系統狀態
./scripts/openclaw/status.sh

# 2. 生成 mutation 建議
./scripts/openclaw/propose_mutation.sh

# 3. 執行實驗
./scripts/openclaw/execute_next.sh

# 4. 判斷實驗結果
./scripts/openclaw/judge_latest.sh

# 5. 決策 promote/revert
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..."
```

## 完整工作流程

### Phase 1: 狀態評估

```bash
./scripts/openclaw/status.sh
```

輸出包含：
- 當前 baseline 版本
- 實驗狀態統計
- replay 資料可用性
- weakest agent 識別
- 建議的下一步動作

### Phase 2: Mutation 設計

互動模式：
```bash
./scripts/openclaw/propose_mutation.sh
```

自動模式（供 OpenClaw 使用）：
```bash
./scripts/openclaw/propose_mutation.sh --auto --agent growth-momentum-01
```

輸出：
- Mutation brief JSON
- 建議的 hypothesis
- Acceptance criteria
- 人工確認提示

### Phase 3: 實驗執行

```bash
# 使用建議的 brief
./scripts/openclaw/execute_next.sh

# 或直接指定
./scripts/openclaw/execute_next.sh --brief path/to/brief.json
```

### Phase 4: 實驗判斷

```bash
./scripts/openclaw/judge_latest.sh
```

自動分析：
- Baseline vs candidate 比較
- Acceptance gates 檢查
- 建議決策（accept/reject）

### Phase 5: 決策執行

Promote：
```bash
./scripts/openclaw/decide.sh \
  --promote exp-growth-momentum-01-xxx \
  --reason "Improved Sharpe by 5% with no drawdown degradation"
```

Revert：
```bash
./scripts/openclaw/decide.sh \
  --revert 2 \
  --reason "Unexpected drawdown increase in volatile regime"
```

Dry-run（預覽）：
```bash
./scripts/openclaw/decide.sh --promote EXP-ID --reason "..." --dry-run
```

## OpenClaw 自主模式

OpenClaw 可以自動化執行完整循環：

```bash
#!/bin/bash
# openclaw-auto-loop.sh

while true; do
    # 1. 檢查狀態
    STATUS=$(./scripts/openclaw/status.sh --json)
    
    # 2. 如果沒有進行中的實驗，生成 mutation
    if ! echo "$STATUS" | grep -q "running"; then
        ./scripts/openclaw/propose_mutation.sh --auto --dry-run
        
        # 人工確認點
        read -p "Proceed with mutation? [y/N]: " confirm
        if [[ $confirm =~ ^[Yy]$ ]]; then
            ./scripts/openclaw/propose_mutation.sh --auto
            ./scripts/openclaw/execute_next.sh
        fi
    fi
    
    # 3. 檢查完成的實驗
    if echo "$STATUS" | grep -q "completed"; then
        ./scripts/openclaw/judge_latest.sh
        
        # 獲取建議
        RECOMMENDATION=$(./scripts/openclaw/judge_latest.sh --recommendation)
        
        # 人工確認點
        echo "Recommendation: $RECOMMENDATION"
        read -p "Execute recommendation? [y/N]: " confirm
        if [[ $confirm =~ ^[Yy]$ ]]; then
            ./scripts/openclaw/decide.sh --yes $RECOMMENDATION
        fi
    fi
    
    sleep 3600  # 每小時檢查一次
done
```

## 人工介入點

設計原則：**關鍵決策點保留人工確認**

| 步驟 | OpenClaw | 人工 | 說明 |
|------|----------|------|------|
| 數據收集 | 自動 | 無需 | 純技術操作 |
| Backtest | 自動 | 無需 | 純技術操作 |
| Weakest agent | 自動 | 可選查看 | OpenClaw 建議，人類可覆蓋 |
| **Mutation 設計** | 生成 | **確認** | 關鍵創意步驟，人工把關 |
| Experiment 執行 | 自動 | 無需 | 純技術操作 |
| Judge | 自動 | 可選查看 | OpenClaw 分析，人類可覆蓋 |
| **Promote/Revert** | 建議 | **最終確認** | 單向門決策，必須人工確認 |

## 安全機制

### Dry-run 模式

所有操作都支援 `--dry-run`：

```bash
./scripts/openclaw/propose_mutation.sh --dry-run
./scripts/openclaw/decide.sh --promote EXP --reason "..." --dry-run
./scripts/openclaw/decide.sh --revert 2 --reason "..." --dry-run
```

### Reason 強制要求

所有 promote/revert 必須提供 `--reason`：

```bash
# 錯誤
./scripts/openclaw/decide.sh --promote EXP-001

# 正確
./scripts/openclaw/decide.sh --promote EXP-001 --reason "Improved Sharpe by 8%"
```

### 確認提示

預設情況下需要確認：

```bash
./scripts/openclaw/decide.sh --promote EXP-001 --reason "..."
# 提示：Confirm promotion? [y/N]:
```

自動確認（僅供 OpenClaw 在人工預授權後使用）：

```bash
./scripts/openclaw/decide.sh --promote EXP-001 --reason "..." --yes
```

## 故障排除

### 狀態異常

```bash
# 檢查實驗狀態
cat data/state/experiments.jsonl | tail -5

# 檢查 baseline
cat data/state/baseline_policy.json | jq .Version

# 檢查 replay 資料
ls -la data/replay/
```

### 實驗卡住

```bash
# 查看運行中的實驗
grep '"Status":"running"' data/state/experiments.jsonl

# 手動判斷
./scripts/openclaw/judge_latest.sh --force
```

### 需要回滾

```bash
# 查看歷史
./scripts/openclaw/revert-baseline --list

# 回滾到上一版本
./scripts/openclaw/decide.sh --revert --reason "Reason for revert"

# 回滾到指定版本
./scripts/openclaw/decide.sh --revert 3 --reason "Version 3 was more stable"
```

## 檔案結構

```
atlas/
├── scripts/openclaw/
│   ├── status.sh           # 狀態報告
│   ├── propose_mutation.sh # Mutation 建議
│   ├── execute_next.sh     # 執行下一個實驗
│   ├── judge_latest.sh     # 判斷最新實驗
│   └── decide.sh           # Promote/Revert 決策
├── docs/
│   └── 2026-06-15-openclaw-protocol.md # 本文件
└── data/state/
    ├── baseline_policy.json   # 當前 baseline
    ├── experiments.jsonl      # 實驗記錄
    └── mutation-briefs/       # Mutation 建議
```

## 與現有 CLI 的對照

| OpenClaw 腳本 | 對應 Go 命令 | 說明 |
|--------------|------------|------|
| `status.sh` | 無 | 新增：統一狀態報告 |
| `propose_mutation.sh` | 無 | 新增：Mutation 建議生成 |
| `execute_next.sh` | `go run ./cmd/run-experiment` | 包裝層 |
| `judge_latest.sh` | `go run ./cmd/judge-experiment` | 包裝層 |
| `decide.sh --promote` | `go run ./cmd/promote-baseline` | 包裝層 + 安全檢查 |
| `decide.sh --revert` | `go run ./cmd/revert-baseline` | 包裝層 + 確認流程 |

## 開發計劃

### Phase 1: 核心腳本（已完成）
- ✅ status.sh
- ✅ propose_mutation.sh
- ✅ decide.sh
- ✅ Revert CLI

### Phase 2: 自動化強化
- ⏳ execute_next.sh
- ⏳ judge_latest.sh
- ⏳ --json 輸出模式
- ⏳ OpenClaw 整合模式

### Phase 3: 進階功能
- ⏳ Multi-baseline comparison
- ⏳ Automatic guardrails
- ⏳ Web dashboard
- ⏳ Slack/Discord 通知

## 注意事項

1. **Mutation 設計**：雖然 OpenClaw 可以生成建議，但 prompt 修改建議人工確認
2. **Promote/Revert**：這兩個操作是單向門，必須人工最終確認
3. **Reason 要求**：所有決策必須記錄原因，用於審計和學習
4. **Dry-run**：建議在自動化流程中加入 dry-run 預覽步驟

## 支援與反饋

如有問題或建議，請：
1. 查看本文件的故障排除章節
2. 檢查 `docs/` 目錄下的其他文件
3. 查看 `data/state/` 的狀態檔案
4. 提交 issue 或改進建議
