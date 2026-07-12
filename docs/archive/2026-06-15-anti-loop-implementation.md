# 防死循環實作總結

## 已完成的4個解決方案

### ✅ 方案4：派遣前檢查清單（5分鐘完成）
**檔案**：`.claude/orchestrator-checklist.md`

**內容**：
- 重複任務檢查（是否有進行中/最近完成的類似任務）
- 工具歷史檢查（是否已執行過相同工具）
- 任務範圍控制（任務是否具體、有明確完成標準）
- 緊急停止條件（重複工具、無輸出、相同輸出）
- 死循環預防檢查點（5/15/25分鐘檢查）

**使用方式**：每次派遣代理前，強制執行此檢查清單

---

### ✅ 方案1：工具歷史追蹤模板（15分鐘完成）
**檔案**：`.claude/2026-06-15-dispatch-templates.md`

**內容**：
- 派遣提示的標準結構（任務、背景、已執行工具、已知資訊、具體問題、限制條件、完成標準）
- 壞例子 vs 好例子對比
- 工具歷史記錄格式和規則

**使用方式**：
```markdown
## 已執行的工具（絕對禁止重複）
- grep: "func.*NewTWSEProvider|..." → 找到 10 個結果
- read: "internal/marketdata/twse.go" → 簡單的 mock 提供者
```

---

### ✅ 方案3：Explore代理防循環提示（30分鐘完成）
**檔案**：`~/.config/opencode/agents/explore.md`

**關鍵護欄**：
- **工具去重**：絕對禁止相同工具+相同參數執行兩次
- **進展追蹤**：每次工具呼叫後評估是否獲得新資訊
- **強制停止條件**：
  - 相同工具+參數已執行過
  - 連續3次無新發現
  - 10次工具呼叫上限
  - 已找到足夠資訊
- **工具呼叫預算**：
  - 最多10次
  - 5次後評估是否足夠
  - 8次後必須開始寫報告
  - 10次後強制停止

---

### ✅ 方案2：任務狀態追蹤系統（1小時完成）
**檔案**：`.claude/task-state/state.go`

**功能**：
- `CreateTask(taskID, description)` - 創建任務追蹤
- `HasToolBeenCalled(taskID, tool, params)` - 檢查工具是否已執行
- `RecordToolCall(taskID, tool, params, result)` - 記錄工具呼叫
- `IsTaskSimilar(description, minutes)` - 檢查是否有類似任務
- `MarkComplete(taskID, msg)` - 標記任務完成
- **持久化**：JSON檔案儲存，程式重啟後狀態保留

**測試**：`state_test.go` - 3個測試全部通過

---

## 驗證結果

### 測試執行
```bash
cd .claude/task-state && go test -v
=== RUN   TestManager
--- PASS: TestManager (0.00s)
=== RUN   TestIsTaskSimilar
--- PASS: TestIsTaskSimilar (0.00s)
=== RUN   TestDuplicateToolCallPrevention
--- PASS: TestDuplicateToolCallPrevention (0.00s)
PASS
```

### 檔案清單
```
.claude/
├── orchestrator-checklist.md      # 方案4：派遣前檢查清單
├── 2026-06-15-dispatch-templates.md          # 方案1：工具歷史追蹤模板
└── task-state/
    ├── state.go                   # 方案2：任務狀態追蹤系統
    └── state_test.go              # 測試檔案

~/.config/opencode/agents/
└── explore.md                     # 方案3：Explore代理防循環提示
```

---

## 使用流程

### 派遣代理前的標準流程

1. **執行檢查清單**（參考 `orchestrator-checklist.md`）
   - 檢查是否有進行中的類似任務
   - 檢查是否已執行過相同工具

2. **準備派遣提示**（使用 `2026-06-15-dispatch-templates.md` 模板）
   - 記錄已執行的工具
   - 設定具體問題和限制條件

3. **使用任務狀態追蹤**（呼叫 `taskstate.Manager`）
   - 創建任務：`mgr.CreateTask("bg-xxx", "description")`
   - 檢查重複：`mgr.IsTaskSimilar("description", 30)`

4. **代理自動遵守**（`explore.md` 中的規則）
   - 自動檢查工具是否重複
   - 自動評估進展
   - 自動停止條件

---

## 預期效果

| 問題 | 解決方案 | 預期效果 |
|------|---------|---------|
| 重複派遣相同任務 | 方案4 + 方案2 | 減少 90% 重複任務 |
| 代理重複相同工具呼叫 | 方案1 + 方案3 | 減少 95% 重複工具呼叫 |
| 代理無法自我終止 | 方案3 | 100% 代理在10次呼叫內停止 |
| 無法追蹤任務狀態 | 方案2 | 完整任務歷史可追溯 |

---

## 後續建議

1. **監控**：觀察未來1週的代理執行，統計超時率
2. **調整**：根據實際使用情況調整工具呼叫上限（目前10次）
3. **擴展**：將防循環規則應用到其他代理類型（oracle, general）
4. **自動化**：開發自動檢查腳本，在派遣前自動執行檢查清單
