# W1: 系統地圖自動化

## 目標

建立一個 **commit hook 觸發的自動化系統地圖產生器**，讓 AI 每次接手工作時都能看到最新的系統藍圖，不用重複花幾小時盤點。

## 為什麼需要這個？

- GitNexus index 經常過期（`last indexed: 7f46276`），查不到新 code
- 每次 AI 接手新任務都要花 30-60 分鐘重新探索 codebase
- SKILLS-MAP.md 是手動維護的，和實際 code 不同步
- 後端 handler 數量（52+）和前端頁面（15）的對照關係沒人維護

## 要產出什麼？

### 地圖 1: 模組架構圖 (`.omo/maps/architecture.md`)

```
自動生成內容：
  ├── 模組列表（從 internal/ 目錄掃描）
  ├── 每個模組的檔案數 + 總 LOC
  ├── 模組角色說明（從 AGENTS.md 萃取第一段）
  ├── Import 依賴圖（誰 import 誰）
  └── 最後更新時間
```

### 地圖 2: API 路由全景 (`.omo/maps/api-routes.md`)

```
自動生成內容：
  ├── 所有 mux.HandleFunc / Register*Routes 呼叫點
  ├── 路由 pattern → handler 函數名 → 檔案:行號
  ├── 標記哪些 handler 是 stub（return nil / not implemented）
  └── 分組：dashboard / industry / narrative / control / live / experiment / backtest / performance
```

### 地圖 3: 模組完整度報告 (`.omo/maps/module-completeness.md`)

```
自動生成內容：
  ├── 每個 internal/* 模組
  ├── return nil 計數（stub 指標）
  ├── TODO/FIXME 計數
  ├── 測試覆蓋率（從 go test -cover 擷取）
  ├── 完整度評分（0-100%）
  └── 變化趨勢（與上次比較）
```

### 地圖 4: 前後端對照表 (`.omo/maps/frontend-backend.md`)

```
自動生成內容：
  ├── 前端頁面列表（從 web/static/js/pages/ 掃描）
  ├── 每個頁面呼叫哪些 API（從 JS fetch/XHR 萃取）
  ├── 哪些 API 沒有對應的前端頁面（孤兒 API）
  ├── 哪些前端頁面呼叫了不存在的 API（斷鏈）
  └── API handler → 前端頁面 1:N 對照矩陣
```

## 實作方式

### 觸發機制
```
git pre-commit hook → 掃描變更檔案
  → 只更新受影響的地圖（不重掃整個 repo）
  → 寫入 .omo/maps/
  → git add .omo/maps/（自動 staged）
```

### 技術方案
```
全部用 Go 實作（與專案語言一致）：
  cmd/mapgen/main.go          ← 掃描 CLI 入口
  cmd/mapgen/architecture.go  ← 模組架構圖生成
  cmd/mapgen/api_routes.go    ← API 路由萃取（用 AST 分析，非 regex）
  cmd/mapgen/completeness.go  ← stub/TODO 掃描 + 覆蓋率
  cmd/mapgen/frontend_backend.go ← JS import 掃描 + 路由對照
```

### 參考現有工具
- GitNexus clusters: `gitnexus://repo/atlas-go/clusters`
- AST-grep: `ast_grep_search` 工具可以準確找到 HandleFunc/Register 呼叫
- go/packages: Go 官方 package 掃描

## 不可碰的檔案

- `cmd/atlas/main.go` — 不要改現有邏輯
- `internal/` — 只讀不寫
- `web/` — 只讀不寫

## 驗證條件

```bash
# W1 完成後，以下必須全部通過：
go build ./cmd/mapgen/...           # ✅ 編譯成功
go run ./cmd/mapgen                  # ✅ 生成不報錯
ls .omo/maps/*.md                    # ✅ 四個地圖都產出
cat .omo/maps/module-completeness.md # ✅ 包含所有 internal/ 模組
cat .omo/maps/api-routes.md          # ✅ 路由數量 >= 50
cat .omo/maps/frontend-backend.md    # ✅ 有孤兒 API 和斷鏈報告

# pre-commit hook 驗證：
touch internal/orchestrator/system.go  # 模擬一次變更
git add internal/orchestrator/system.go
git commit -m "test: trigger map update"
# → .omo/maps/architecture.md 的 orchestrator 區塊應自動更新
```

## 完成報告格式

```markdown
## W1 Completion Report

### Generated Maps
| Map | Path | Lines | Last Updated |
|-----|------|-------|-------------|

### Discovered Issues
| Module | Stubs | TODOs | Coverage | Completeness % |
|--------|-------|-------|----------|---------------|

### Orphan APIs (no frontend consumer)
| Route | Handler | File |
|-------|---------|------|

### Broken Links (frontend calls non-existent API)
| Page | Called URL | Status |
|------|-----------|--------|

### Hook Verification
- [ ] pre-commit hook installs correctly
- [ ] Hook auto-stages .omo/maps/ on commit
- [ ] Hook skips when no source changes

### To Investigate Next
(list any anomalies found during map generation)
```

將此報告存到 `/tmp/w1-report.md`
