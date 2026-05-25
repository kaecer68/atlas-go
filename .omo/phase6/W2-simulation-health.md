# W2: 模擬 pipeline 固化

## 目標

讓 `go run ./cmd/atlas`（simulation mode）可以**可重複、可稽核、可除錯**地跑完整場 session。

## 為什麼需要這個？

- 模擬 pipeline 是整個系統的地基。地基不穩，上面蓋什麼都倒。
- 目前：數據時有時無、執行斷裂、前端只能看到部分結果、前端不顯示錯誤層級
- 目標：一場 session 從頭到尾有完整的 step-by-step trace，哪層斷了一目瞭然

## 要產出什麼？

### 1. 模擬 trace log (`.omo/traces/sim-YYYYMMDD.jsonl`)

```
每場模擬產出一條 JSONL：
{"step":1,"layer":"data_fetch","status":"START","ts":"2026-05-26T09:00:00Z"}
{"step":1,"layer":"data_fetch","status":"OK","symbols":200,"provider":"hybrid","ts":"..."}
{"step":2,"layer":"regime_detect","status":"START","ts":"..."}
{"step":2,"layer":"regime_detect","status":"OK","regime":"risk_on","confidence":0.82,"ts":"..."}
{"step":3,"layer":"screening","status":"START","ts":"..."}
{"step":3,"layer":"screening","status":"OK","candidates":45,"filtered":155,"ts":"..."}
{"step":4,"layer":"recommend","status":"WARN","agents":12,"active":8,"muted":4,"ts":"..."}
{"step":5,"layer":"guard_filter","status":"OK","passed":23,"blocked":5,"ts":"..."}
{"step":6,"layer":"sim_exec","status":"OK","orders":8,"ts":"..."}
{"step":7,"layer":"ledger_write","status":"OK","ts":"..."}
```

如果某層 FAIL：
```json
{"step":3,"layer":"screening","status":"FAIL","error":"screener: no symbols match criteria","ts":"..."}
```
→ 前端立刻顯示哪層斷了，為什麼。

### 2. `--verbose` 旗標

```
go run ./cmd/atlas --verbose
  → 終端機即時輸出每一步的狀態
  → 紅色標記失敗的層
  → 黃色標記警告（如 provider 回退）

go run ./cmd/atlas --simulate --date 2026-03-26 --verbose
  → 強制跑指定日期的模擬
  → 產出 trace log
  → 輸出到終端機 + 寫入 .omo/traces/
```

### 3. 模擬狀態儀表板（前端）

在 Dashboard 加一個區塊（或獨立頁面）：
```
┌─────────────────────────────────────────┐
│ 模擬狀態                                 │
│                                         │
│ ● data_fetch   ✅ 200 symbols          │
│ ● regime       ✅ risk_on (0.82)       │
│ ● screening    ✅ 45/200 passed        │
│ ● recommend    ⚠️  8/12 agents active  │
│ ● guard_filter ✅ 23 passed, 5 blocked │
│ ● sim_exec     ✅ 8 orders generated   │
│ ● ledger_write ✅ saved                │
│                                         │
│ 耗時: 2.3s   |   上一場: 2026-05-25     │
│ [查看 trace] [重新模擬]                  │
└─────────────────────────────────────────┘
```

### 4. 修復模擬中斷點

根據 trace log 的 FAIL，優先修：
- 數據源回退邏輯（HybridProvider 在無數據時的 fallback）
- screener 在沒有篩選結果時的行為（目前是靜默跳過）
- agent 全 mute 時的處理（目前直接 return 0 orders，沒報錯）

## 實作順序

### Phase W2-1: Trace infrastructure
- 定義 trace log 格式 (struct)
- 在 SystemCore.RunDailySimulation 的關鍵節點插入 trace 記錄
- `--verbose` flag + 終端機輸出
- `--date` flag（覆蓋 sessionDate）

### Phase W2-2: 前端儀表板
- 讀取最新的 `.omo/traces/sim-*.jsonl`
- 渲染成狀態圖
- 點擊每層展開詳細 log

### Phase W2-3: 中斷點修復
- 從 trace log 找 FAIL pattern
- 逐個修復
- 每修一個 → 重跑模擬 → 確認該層變 OK

## 不可碰的檔案（讀取可，不寫入）

- `internal/portfolio/` — 不碰 optimizer / factor engine
- `internal/config/parameters*.go` — 不碰參數系統
- `web/static/js/pages/` — 前端可以加新頁面/組件，但不要改現有頁面的核心邏輯

## 可修改/新增的範圍

- `cmd/atlas/main.go` — 加 `--verbose`, `--date` flag 和 trace 輸出
- `internal/orchestrator/system.go` — 在 RunDailySimulation 插入 trace 點
- `internal/sim/` — 讀取模擬引擎狀態
- `web/static/js/components/` — 加 sim-health.js 組件
- `web/static/index.html` — 加模擬狀態區塊
- 新建 `.omo/traces/` — trace log 輸出目錄

## 驗證條件

```bash
go build ./cmd/atlas/...
go run ./cmd/atlas --simulate --date 2026-03-26 --verbose 2>&1 | head -30
# → 輸出每一步的狀態，不中斷
ls .omo/traces/sim-20260326.jsonl
# → 檔案存在且行數 >= 7 (每層一行)
cat .omo/traces/sim-20260326.jsonl | jq '.status'
# → 看到 OK/FAIL/WARN
go test ./internal/orchestrator/...  # 現有測試繼續通過
```

## 完成報告格式

```markdown
## W2 Completion Report

### Trace Infrastructure
- [ ] --verbose flag implemented
- [ ] --date flag implemented
- [ ] trace log format defined (struct)
- [ ] RunDailySimulation emits trace records
- [ ] Example trace output (paste terminal output)

### Frontend Dashboard
- [ ] Simulation health panel renders in browser
- [ ] Green/yellow/red status per layer
- [ ] Click to expand layer details
- [ ] Auto-refresh on new simulation

### Fixes Applied
| Layer | Issue | Fix | Verified? |
|-------|-------|-----|-----------|

### Regression Check
- [ ] go test ./internal/orchestrator/... passes
- [ ] go test ./internal/sim/... passes
- [ ] go build ./cmd/atlas/ passes

### Trace Log Sample
(paste first 10 lines of a successful trace)
```

將此報告存到 `/tmp/w2-report.md`
