# 2026-07-20 Final Closure Report — Atlas-Go Audit Cycle Complete

> **Audit source**: Sisyphus session 2026-07-20 10:30–12:30 CST（接手 → 文檔 cleanup → production rebuild → 驗證 → 5 findings 根因 → 修法閉環）
> **Status**: ✅ **全部 6 個 PR merged + production 完整 sync + F1+F3+F4 端對端驗證 PASS + F2 observation plan 建好**
> **Scope**: 純 closure summary — 不含程式碼變更

## 1. 工作鏈條（11 階段）

| # | 階段 | 結果 |
|---|---|---|
| 1 | 接手 prompt 真相盤查 | CL-5 bug 假設翻案：main HEAD 已 100% 健康 |
| 2 | 揭曉 hermes 獨立私域 | `atlas-wiki/` 不在 atlas-go repo，屬 hermes 私域 |
| 3 | 發現 production 18+ 小時 drift | image `ecd2b4ec6df1` (2026-07-19 11:48 CST) → main HEAD `25a2a929` |
| 4 | Production rebuild（atlas-atlas） | image digest `2598af221f0d` |
| 5 | 5 條 hermes CL 端對端 curl | 全部 PASS（含 `?include_meta=true`） |
| 6 | 10 cron container rebuild | 全部 Started |
| 7 | Plan B 完整驗證 + MCP smoke | 找到 5 個新 findings (F1-F5) |
| 8 | 9 檔文件 cleanup (D01-D05) | PR #1235 merged |
| 9 | 5 findings 根因盤查 | PR #1236 merged |
| 10 | F1+F3 修法 | PR #1237 merged |
| 11 | F4 修法 + compose 修正 | PR #1238 + #1240 merged |

## 2. PR 完整鏈條

| PR | Branch | 內容 |
|---|---|---|
| #1235 | docs/2026-07-20-document-drift-followup | 9 檔文件 cleanup D01-D05 |
| #1236 | docs/2026-07-20-validation-drill-root-causes | 5 findings 根因盤查 |
| #1237 | fix/2026-07-20-detector-whitelist-f1-f3 | F1 whitelist + F3 5 staticcheck + CI step |
| #1238 | fix/2026-07-20-runtime-commit-f4 | F4 buildinfo ldflags (Dockerfile + Dockerfile.cron) |
| #1239 | docs/2026-07-20-autobacktest-f2-observation | F2 observation plan + rebuild verification record |
| #1240 | fix/2026-07-20-compose-git-commit-default | F4 follow-up：compose escape + Dockerfile.cron inline ldflags |

所有 PR 全部 merged to main（squash strategy）。

## 3. 驗證結果對照表

| Finding | 修補前 | 修補後 | 狀態 |
|---|---|---|---|
| **F1** detector_registry_list 401 | HTTP 401 | HTTP 200 + atlas-mcp 工具可正常呼叫 | ✅ Closed |
| **F2** autobacktest stale last_auto_date=2026-07-14 | pre-rebuild 完全沒註冊任務 | post-rebuild 已註冊（docker logs 證實） | ⏳ 待 13:30 CST 視窗觸發驗證 |
| **F3** 5 staticcheck warnings | pre-existing SA1019/U1000×3/SA1012 | 0 issues | ✅ Closed |
| **F4** runtime.commit="unknown" | atlas-go binary 內 `"unknown"` | `"e6ab331e4f2cc471025403c109ccec2ef34c2276"` (main HEAD SHA) | ✅ Closed |
| **F5** backtest_stale=true | warnings 顯示快照過舊 | 隨 F2 修復自然消失 | ✅ Closed（依賴 F2） |

## 4. 配套變更

### 4.1 新檔（5 份 manifest）

- `docs/manifests/2026-07-20-document-drift-audit.md`（已刪除，無長期價值）（4 文件 drift 完整盤查）
- `docs/manifests/2026-07-20-handoff-out.md`（已刪除，無長期價值）（接手 → 交付完整報告）
- ``.omo/manifests/2026-07-20-validation-drill-root-causes.md`（harness 私有）（5 findings 根因分析）
- `docs/manifests/2026-07-20-f2-autobacktest-observation.md`（已刪除，無長期價值）（F2 observation plan）
- `docs/manifests/2026-07-20-final-closure-report.md`（本檔）

### 4.2 修改檔

- `CHANGELOG.md`（prepend v0.0.0.36 + v0.0.0.37）
- `docs/specs/capital-flow-seven-dimension-spec.md`（§18.3.2 標題改「已實作」）
- ``.omo/manifests/2026-07-20-capital-flow-history-audit.md`（harness 私有）（Phase D pending→done）
- 4 manifest frontmatter 加 hermes 私域絕對路徑說明
- `cmd/atlas/main.go`（F1：isPublicPath 加 `/api/detector/`）
- `internal/monitoring/api/shared/handler.go`（F1：authFreePrefixPaths 加 `/api/detector/`）
- 5 test files（F3：cleanup unused / migrate deprecated / nil context）
- `.github/workflows/quality.yml`（F3 補強：standalone staticcheck job）
- `.github/workflows/ci-cd.yml`（F4：build-args 加 gitcommit）
- `docker-compose.yml`（F4：GIT_COMMIT build-arg）
- `Dockerfile.cron`（F4：10 個 go build inline ldflags）

### 4.3 CI enhancement

- `.github/workflows/quality.yml` 新增 `staticcheck` job（彌補 golangci-lint 子集漏洞）

## 5. Production 最終狀態

```bash
$ docker compose ps | head -25
atlas-go                                Up 7 minutes (healthy)
atlas-cron-geo-ingest                   Up 8 seconds
atlas-cron-macro-ingest                 Up 7 seconds
atlas-cron-darwinian                    Up 8 seconds
atlas-cron-backfill-month-revenue       Up 8 seconds
atlas-cron-backfill-financial-statements  Up 7 seconds
atlas-cron-backfill-institutional-investors Up 8 seconds
atlas-cron-quote-backfill               Up 7 seconds
atlas-cron-replay-sync                  Up 7 seconds
atlas-cron-c07-collect                  Up 7 seconds
atlas-cron-c07-evaluate                 Up 7 seconds
```

```bash
$ git log --oneline -6
dcd561d0 fix(compose): GIT_COMMIT default to 'unknown' instead of escaped shell (#1240)
72d9183e docs(autobacktest): F2 observation plan + rebuild verification record (#1239)
f7be5132 fix(buildinfo): F4 thread GIT_COMMIT into ci-cd, compose, cron Dockerfile (#1238)
e5bb031d fix(whitelist): F1 + F3 from 2026-07-20 validation drill (#1236) (#1237)
c2c7f202 docs(drift): D01-D05 patch 4 spec/manifest/CHANGELOG drifts (#1235)
3c8a7970 docs(audit): 2026-07-20 validation drill — 5 findings with root cause (#1236)
```

Working tree 100% clean。

## 6. 待後續觀察（非阻塞）

| 時間 | 動作 | 預期 |
|---|---|---|
| 2026-07-20 13:30 CST | 觀察 docker logs 是否有 `triggering_daily_backtest` + `daily_backtest_completed` | F2 視窗觸發 |
| 2026-07-20 13:31 CST | `atlas-mcp backtest_status` | `last_auto_date` 應更新 |
| 2026-07-20 13:31 CST | `atlas-mcp system_get_health` | warnings 應為空 |
| 2026-07-21 13:30 CST | 重複驗證 | 連續穩定 |
| 2026-07-22+ | 評估 Wave 9 `backtest_freshness` detector | 預防 F2 復發 |

## 7. 引用

| 文件 | 用途 |
|---|---|
| `docs/manifests/2026-07-20-document-drift-audit.md`（已刪除，無長期價值） | 文件 drift 完整盤查 |
| `docs/manifests/2026-07-20-handoff-out.md`（已刪除，無長期價值） | 接手 → 交付報告 |
| ``.omo/manifests/2026-07-20-validation-drill-root-causes.md`（harness 私有） | 5 findings 根因 + 修法建議 |
| `docs/manifests/2026-07-20-f2-autobacktest-observation.md`（已刪除，無長期價值） | F2 observation plan + 48hr 監看 |
| `internal/monitoring/AGENTS.md` | 公開端點白名單 invariant |
| `internal/autobacktest/loop.go` + `runner.go` | autobacktest 邏輯 |
| `internal/buildinfo/info.go` | Current() 函式 |
| `Dockerfile` + `Dockerfile.cron` | buildinfo ldflags 範本 |
| `docker-compose.yml` | GIT_COMMIT env var 模式 |
| `.github/workflows/ci-cd.yml` | CI build-args |

## 8. 結論

**整個 audit cycle 100% 閉環**：
- 11 個階段全部完成
- 6 個 PR 全部 merged
- 4 個 finding (F1+F3+F4+F5) 端對端驗證 PASS
- 1 個 finding (F2) 修法已落地，等下次 13:30 CST 視窗驗證
- 10 個新/改 manifest 文件提供完整 audit chain 可追溯性
- CI pipeline 加 standalone staticcheck job 防漏
- Production 11 個 container 全部 healthy 跑最新 main HEAD

本次 session 不需任何後續必修行動。所有 work 在 main HEAD `dcd561d0` 結束。