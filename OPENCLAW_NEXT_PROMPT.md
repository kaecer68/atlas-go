# OpenClaw 執行提示詞

## 當前狀態

剛剛修復了兩個關鍵問題：

1. **GrowthMomentumExecutor** (`internal/orchestrator/plugin_style.go`)
   - 添加了基線行為（價格下跌減分、低成交量減分）
   - Mutation 關鍵字現在會疊加額外懲罰

2. **AISupplyChainExecutor 和 ETFRotationExecutor** (`internal/orchestrator/plugin_sector.go`)
   - 原來這些 executor 完全沒有使用 prompt 參數！
   - 現在已添加 prompt 響應邏輯

## 你的任務

執行 atlas-go 的進化循環，目標是：

1. **清理現有實驗** - 檢查 running experiments 的狀態
2. **執行新的 mutation** - 測試修復後的系統
3. **觀察結果** - 確認 mutations 現在能產生不同的 baseline/candidate

## 執行步驟

### Step 1: 檢查狀態
```bash
./scripts/openclaw/status.sh
```

分析輸出，確認：
- Baseline 版本
- Running experiments 數量
- 是否需要清理

### Step 2: 處理 Running Experiments

如果有 running experiments，執行：
```bash
./scripts/openclaw/judge-latest.sh --auto --json
```

觀察結果：
- BaselineValue 應該 ≠ CandidateValue（這是修復的關鍵！）
- 如果有改善，考慮 promote
- 如果沒有改善，記錄原因

### Step 3: 生成新的 Mutation

當所有 experiments 都處理完後：
```bash
./scripts/openclaw/propose-mutation.sh --auto
```

檢查生成的 brief：
- Target skill 是什麼？
- Hypothesis 是否合理？
- Mutation type 是 prompt_tightening 還是其他？

### Step 4: 執行實驗

如果 brief 看起來合理：
```bash
./scripts/openclaw/execute-next.sh --auto
```

### Step 5: 判斷結果

```bash
./scripts/openclaw/judge-latest.sh --auto --json
```

**關鍵觀察指標**：
- BaselineValue 和 CandidateValue 是否不同？
- CandidateValue > BaselineValue 嗎？
- Judge checks 包含哪些項目？

### Step 6: 決策

如果 accepted：
```bash
./scripts/openclaw/decide.sh --promote <EXP-ID> --reason "修復後首次成功 mutation，[具體原因]"
```

如果 rejected：
- 記錄原因
- 等待下一輪

## 成功標準

✅ **主要目標**：看到 BaselineValue ≠ CandidateValue 的 experiment
✅ **次要目標**：成功 promote 至少一個 mutation

## 注意事項

- 修復後的系統現在應該能產生不同的 baseline/candidate
- 如果仍然看到 BaselineValue = CandidateValue，可能是其他 executor 也需要修復
- 記錄所有實驗結果供進一步分析

## 開始執行

請執行：
```bash
./scripts/openclaw/status.sh
```

然後根據輸出決策下一步。
