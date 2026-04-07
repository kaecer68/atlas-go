# OpenClaw 快速入門 (開箱文)

> 讀取本文件後，即可開始執行 atlas-go 策略優化。

## 第一步：確認環境

```bash
cd /Users/kaecer/.openclaw/workspace/agents/finance/atlas
ls scripts/openclaw/  # 應該有 6 個腳本
```

## 第二步：檢查狀態

```bash
./scripts/openclaw/status.sh
```

**看懂輸出**：
- `Baseline Policy` - 當前策略版本
- `Running` - 進行中的實驗 (優先處理)
- `Planned` - 計劃中的實驗
- `Recommended Next Action` - 系統建議

## 第三步：執行工作

### 場景 A：系統建議 "judge-latest"
```bash
./scripts/openclaw/judge-latest.sh --auto --json
# 分析結果 → 如果 accepted → promote
#                rejected → 跳過
```

### 場景 B：系統建議 "execute-next"
```bash
./scripts/openclaw/execute-next.sh --auto
./scripts/openclaw/judge-latest.sh --auto --json
# 然後 promote 或 skip
```

### 場景 C：系統建議 "propose-mutation"
```bash
./scripts/openclaw/propose-mutation.sh --auto
# 審查生成的 brief → 確認 → execute
```

## 第四步：Promote (需確認)

```bash
# 只有在改善時才執行
./scripts/openclaw/decide.sh --promote EXP-ID --reason "改善了X%"
```

## 關鍵檔案

- `docs/openclaw-protocol.md` - 完整協議
- `docs/SCRIPT_USAGE_GUIDE.md` - 詳細教學
- `docs/skills-map.md` - 技能定義

## 安全規則

1. **必須確認** - Promote/Revert 前暫停等待
2. **必須理由** - 每次決策都要 --reason
3. **不要修改** - configs/ 和 internal/ 不要動

## 故障排除

| 問題 | 解決 |
|------|------|
| "No experiments" | 執行 propose-mutation.sh |
| "Cannot find file" | 稍等後重試 |
| BaselineValue=0 | 先執行 backtest-window |

## 開始

執行：`./scripts/openclaw/status.sh`
