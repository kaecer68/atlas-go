# 2026-07-20 Validation Drill — 5 Findings + 根因盤查

> **Audit source**: Sisyphus 接手 session 2026-07-20 13:xx CST，承接 PR #1235 之後
> **觸發條件**: Plan B 完整測試與驗證 + 10 個 cron container rebuild + MCP tool smoke test
> **Goal**: 把 Plan B / cron rebuild 過程中發現的 5 個問題附上完整證據與根因分析，列入新 PR 追蹤
> **Scope**: 純根因盤查 + 建議修復方向 — 修法本身留待下一輪 PR 評估

## 1. 已驗證 PASS（無問題）

| 驗證項 | 結果 |
|---|---|
| PR #1235 CI 全套（32+ checks） | ✅ 100% pass |
| `go test ./...` 完整 backend | ✅ exit 0，全部 packages `ok` |
| `go vet ./...` | ✅ exit 0 |
| `golangci-lint run --timeout=5m` | ✅ 0 issues |
| `scripts/ci/check_markdown_links.sh` | ✅ 316 link + 249 bare path = 565 reference 全部 valid |
| `scripts/ci/check_atlas_mcp_docs_consistency.sh` | ✅ AGENTS.md 77 lines (≤155)、`111 tools` 3 files (≥3) |
| Production image rebuild | ✅ digest `2598af221f0d` (replaced `ecd2b4ec6df1`) |
| 5 條 hermes CL 端對端 curl | ✅ 全部 PASS（含 `?include_meta=true` 回 partial + missing_dimensions） |
| 10 個 cron container rebuild + up | ✅ 全部 Started（image digest 從舊的 6d05d605 era 變為 main HEAD era） |
| MCP smoke test (15+ tools) | ✅ 絕大多數 PASS；只有 F1 一個工具失敗（見下） |

## 2. 5 個新發現的問題（含根因）

### 🔴 F1 — `atlas-mcp detector_registry_list` 回 401 unauthorized

**證據**:
```
$ atlas-mcp_detector_registry_list
HttpClient: GET http://127.0.0.1:18080/api/detector/registry/list -> 401: {"error":"unauthorized"}
```

**根因**（已完整確認）：
1. 路由**有註冊**：`cmd/atlas/template_detector.go:37` 透過 `mux.Handle("GET /api/detector/registry/list", handleDetectorRegistryList(registry))` 註冊（條件式：需 `registry != nil`）
2. `cmd/atlas/main.go:782` 啟動時 log: `[TemplateDetector] registered /api/detector/* routes (24 detectors + scan store=...)`
3. **但 `/api/detector/` prefix 沒在兩個白名單內**：
   - `cmd/atlas/main.go:142-225` `isPublicPath()` switch case 沒涵蓋 `/api/detector/`
   - `internal/monitoring/api/shared/handler.go:24-78` `authFreeExactPaths` + `authFreePrefixPaths` 也沒涵蓋
4. AuthMiddleware 預設 401 → MCP wrapper 也跟著 401（因為 cli.Get 不帶 atlas-mcp token）

**違反 invariant**：`internal/monitoring/AGENTS.md` 明確寫「公開端點需同步加白名單：任何新增的 `/api/*` 公開端點必須同步加到 `cmd/atlas/main.go isPublicPath` + `internal/monitoring/api/shared/handler.go authFreeExactPaths/authFreePrefixPaths`」— 兩個位置都漏了。

**為什麼 CI 沒抓到**：markdown-links + doc-consistency script 只驗文件合規，不驗 API endpoint 是否能公開存取；go test 不打 production HTTP。

**影響面**：
- 24 個 detector 的 enable/disable 清單無法透過 MCP 取得
- `template_detector_status` 也呼叫相同 `/api/detector/...` 路由，雖然走的是 `/api/detector/scan/status`（也是 401，但因為是 audit log endpoint，原本就不應公開）

**建議修法**：
- 修 `cmd/atlas/main.go:142` `isPublicPath()` 加 `case p == "/api/detector" || strings.HasPrefix(p, "/api/detector/"): return true`
- 修 `internal/monitoring/api/shared/handler.go:47` `authFreePrefixPaths` 加 `"/api/detector/",`
- 新增 unit test：對每個 `registerTools` 註冊的 endpoint 自動驗證兩處白名單一致

---

### 🟡 F2 — autobacktest 卡在 `last_auto_date=2026-07-14`

**證據**:
```
$ atlas-mcp_backtest_status
{"result":{"last_auto_date":"2026-07-14","last_auto_portfolio_val":3000000}}

$ atlas-mcp_system_get_health  # 同樣警告
"warnings":["自動回測快照過舊：最新 2026-07-14（replay 已達 2026-07-18）"]
```

**根因**（部分釐清）：
1. 排程**有註冊**：`cmd/atlas/main.go:1784-1805` 註冊 autobacktest 為 1h interval TaskManager task
2. Task handler 呼叫 `autobacktest.RunScheduledBacktest(ctx, btRunner)`（main.go:1788）
3. `RunScheduledBacktest`（`internal/autobacktest/loop.go:78-104`）：
   - 必須在 13:30 ± 30m Taipei 時間窗口內
   - 必須是 weekday（跳過週末）
   - 窗口外回傳 `ErrNotInWindow` → apigateway map 成 `ErrTaskSkipped`（不重置 failure counter）
4. `RunAndStore`（`internal/autobacktest/runner.go:38-89`）：
   - `targetDate = mostRecentTradingDay()`
   - 檢查 `LatestN(1)` 是否有當日 snapshot → 若有則 `snapshot_exists_skip` 直接 return nil（**不重新跑**）
5. **問題點**：replay 已達 2026-07-18 但 autobacktest 還停在 2026-07-14。代表 7/15-7/18 期間：
   - 要麼 task 完全沒觸發（TaskManager 問題）
   - 要麼觸發了但 `snapshot_exists_skip` 邏輯把 7/15-7/18 都誤判為已有 snapshot
   - 要麼觸發且進入 RunAndStore 但實際寫入失敗

**完整根因待追**：
- 需查 atlas-go container 在 7/15-7/18 期間的 log（`./logs/` 或 `docker logs atlas-go`）
- 找 `"autobacktest"` 關鍵字的日誌：
  - `"next_scheduled_run"` → 是否每 1h 有 log
  - `"triggering_scheduled_backtest"` → 是否進入窗口
  - `"snapshot_exists_skip"` → 是否誤判
  - `"backtest_run_failed"` → 是否失敗

**建議追查順序**：
1. `docker logs atlas-go --since="2026-07-15T00:00:00+08:00" --until="2026-07-20T23:59:59+08:00" 2>&1 | grep autobacktest`
2. 若 `next_scheduled_run` 完全沒出現 → TaskManager 1h interval 沒啟動（須查 scheduled_tasks table）
3. 若 `snapshot_exists_skip` 持續出現 → `LatestN(1)` 邏輯或 `mostRecentTradingDay()` 有 bug
4. 若有 `backtest_run_failed` → 查具體錯誤

**影響面**：
- 自動回測結果過舊（4-6 天），影響 portfolio rebalance 決策品質
- 系統健康 warning banner 持續顯示，降低 user 對系統信心

---

### 🟢 F3 — 5 個 pre-existing staticcheck 警告（test files）

**證據**:
```
$ staticcheck ./...
cmd/atlas/storage_route_test.go:15:7: monitoring.NewDashboardAPI is deprecated (SA1019)
internal/eventdriven/narrative_inject_test.go:19:6: func narrativeTestHandler is unused (U1000)
internal/monitoring/service/data_channels_g05_test.go:23:45: nil Context (SA1012)
internal/portfolio/sector_exposure_test.go:22:6: func makeAll20L1WeightsZero is unused (U1000)
internal/recommender/adapters_test.go:295:2: field err is unused (U1000)
```

**根因**（已確認）：
| File | 來源 commit | 時間 |
|---|---|---|
| cmd/atlas/storage_route_test.go | 07a3f85f | 2026-06-15 10:06 |
| internal/eventdriven/narrative_inject_test.go | 8888f01f | 2026-07-17 19:15 |
| internal/monitoring/service/data_channels_g05_test.go | 8888f01f | 2026-07-17 19:15 |
| internal/portfolio/sector_exposure_test.go | 3a0bc353 | 2026-07-18 21:52 |
| internal/recommender/adapters_test.go | 8254fa8f | 2026-07-19 04:53 |

全部早於 main HEAD `25a2a929`（2026-07-20 09:25）。非本次 PR #1235 引入的 regression。

**為什麼 CI 沒抓到**：golangci-lint 規則集合與 staticcheck 不同（golangci-lint 0 issues，staticcheck 5 警告）。CI 只跑 `golangci-lint run`，沒跑 `staticcheck`。

**影響面**：極低（test files only，unused func/field/deprecation）

**建議修法**：
- 補加 `staticcheck ./...` 至 CI pipeline（與 `golangci-lint` 並行）
- 個別修正：
  - SA1019 storage_route_test.go → migrate 到 `monitoring.NewDashboardAPIWithGateway`
  - U1000 → 刪除 unused function/field
  - SA1012 → `context.TODO()` 取代 nil

---

### 🟡 F4 — Production runtime commit 顯示 `"unknown"`

**證據**:
```
$ atlas-mcp_system_get_health | jq .info.runtime
{"build_time":"2026-07-20T03:13:55Z","commit":"unknown","go_version":"go1.26.4","version":"dev"}
```

**根因**（部分釐清）：
- `cmd/atlas/cmd_universe.go:239` `BuildTime string` field 存在
- runtime struct（system_get_health 用的）有 build_time / commit / go_version / version 四個欄位
- `version="dev"` 對應 `docker-compose.yml:11` `VERSION: ${ATLAS_VERSION:-dev}` — 沒傳入時 fallback 到 dev
- `commit="unknown"` 對應 image 沒 inject git commit hash（Dockerfile 沒在 build 時讀 `git rev-parse HEAD` 並 embed）
- `build_time="2026-07-20T03:13:55Z"` 對應 image build timestamp（docker image metadata）

**為什麼 CI 沒抓到**：無 lint rule 驗 production binary 必須 embed git commit

**影響面**：低 — 僅影響 SRE 對 production 版本的可追溯性；不影響功能

**建議修法**：
- Dockerfile 改：
  ```dockerfile
  ARG GIT_COMMIT=unknown
  ARG BUILD_TIME
  RUN echo "version=${GIT_COMMIT}" > /app/build_info.txt
  RUN echo "build_time=${BUILD_TIME}" >> /app/build_info.txt
  ```
- main.go 啟動時讀 `/app/build_info.txt` 注入 runtime
- CI workflow 傳入 `GIT_COMMIT=${GITHUB_SHA}` + `BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)`

---

### 🟢 F5 — Atlas-mcp `system_get_health` `backtest_stale=true`

**證據**:
```
$ atlas-mcp_system_get_health | jq '.info | {backtest_stale, last_window_generated_at, regime, replay_data_latest_date}'
{"backtest_stale":true,"last_window_generated_at":"2026-07-20T00:08:09.444769378Z","regime":"RISK_ON","replay_data_latest_date":"2026-07-18"}
```

**根因**（F2 的衍生）：
- `backtest_stale=true` 直接源自 `last_auto_date=2026-07-14` 與 replay 2026-07-18 比較
- 一旦 F2 修好，F5 自然消失
- **不算獨立問題**

**建議修法**：併入 F2 一起追蹤與修復。

---

## 3. 修復優先順序

| 優先 | ID | 標題 | 預估工時 | 修法難度 |
|---|---|---|---|---|
| 🔴 P0 | F1 | detector_registry_list 401 | 30 min | 低（兩處白名單加一行） |
| 🟡 P1 | F2 | autobacktest stale | 1-2 hr（含日誌分析） | 中（要查日誌確認根因） |
| 🟢 P3 | F3 | 5 staticcheck warnings | 30 min | 低（5 個 trivial edit） |
| 🟢 P4 | F4 | runtime commit="unknown" | 1 hr | 中（Dockerfile + main.go + CI） |
| 🟢 P5 | F5 | backtest_stale (F2 子問題) | 0 (併入 F2) | — |

**建議修法時程**：F1 + F3 可塞同一個 PR（都是 trivial）。F2 + F5 需單獨 PR（需日誌分析）。F4 可單獨或併入 F2。

---

## 4. 本次 session 沒動的東西

- ❌ 不修任何 .go 程式碼（依用戶指示只盤查根因）
- ❌ 不 commit + push（保留給下一輪 PR）
- ❌ 不動 production cron container（已 rebuild 但 cron 邏輯沒改）
- ❌ 不動任何 spec / doc / manifest

## 5. 下一輪 PR 評估建議

1. **PR #1236**（trivial）：F1 + F3 一次修完
   - 改 `cmd/atlas/main.go:142` + `internal/monitoring/api/shared/handler.go:47` 各加一行
   - 改 5 個 test file 各刪除 unused / migrate deprecated / 改 nil context
   - 加 `staticcheck` 至 CI pipeline

2. **PR #1237**（中）：F2 根因分析與修法
   - 先查 `docker logs atlas-go` 確認是 7/15-7/18 哪一類失敗
   - 修對應的 task scheduling / snapshot_exists_skip 邏輯 / RunAndStore 寫入
   - F5 自動消失

3. **PR #1238**（小）：F4
   - Dockerfile + main.go + CI 傳入 git commit
   - 補 build_info.txt 機制

## 6. 參考文件

- PR #1235 — 9 檔文件 drift cleanup（已 merged 候選）
- `docs/manifests/2026-07-20-document-drift-audit.md` — 文件 drift 完整盤查
- `docs/manifests/2026-07-20-handoff-out.md` — 交付報告
- `internal/monitoring/AGENTS.md` — 公開端點白名單 invariant 來源
- `internal/autobacktest/loop.go` — autobacktest 排程實作
- `internal/autobacktest/runner.go` — autobacktest runner 實作
- `cmd/atlas/main.go:142-225` — isPublicPath 白名單
- `internal/monitoring/api/shared/handler.go:24-78` — authFree 白名單
- `cmd/atlas/template_detector.go:35-37` — detector routes 註冊

## 7. 在工作樹外的東西

本次 session 不寫到 `~/workspace/atlas-notes/` 或 `~/workspace/atlas-wiki/` — 全部在 `~/workspace/atlas/` 內（atlas-go repo）。所有變更遵循用戶規則「只有寫入 `~/workspace/atlas/` 目錄下的內容才需要 commit + push + PR」。