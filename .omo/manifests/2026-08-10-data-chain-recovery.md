# Audit Manifest: 2026-08-10 Data Chain Recovery

> **Audit source**: kaecer 要求盤查 atlas-mcp 運作 + atlas 整體 degraded（tw_vol / twse_etf / auto_cycle_update 三條資料鏈）
> **Goal**: 修復三條資料鏈根因：FinMind symbol 契約錯誤、quarter parser 錯誤、auto_cycle_update 批次容量不足、tw_vol freshness 誤判、twse_etf 錯誤分類過寬
> **Scope**: internal/marketdata、internal/industry、internal/apigateway、cmd/atlas 的資料取得/聚合/健康判定相關程式碼；不含 frontend、不含 experiment/evolution 流程、不含 twse_etf 上游替代源開發（待 upstream 實測後另案）
> **Created**: 2026-08-10
> **Status**: in-progress

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | FinMind 對 `.TW` suffix 的 stock ID 未正規化，`auto_cycle_update` 對全部 representative stocks 請求失敗 | `DataAggregator` 原封不動傳 `1513.TW` 給 `GetMonthRevenue`/`GetFinancialStatements` → `fetchDataset` 設 `data_id=1513.TW`；FinMind 只用裸碼 `1513`。MCP 個股工具會先 trim 所以成功 | `internal/marketdata/finmind_client.go` | `auto_cycle_update` 對 industrial (1513/1590) 聚合成功；新增契約測試驗證 `data_id` 無 `.TW` | pending | none | 證據: `stock_get_monthly_revenue(1513/1590)` 成功；aggregator 失敗 |
| A02 | `GetFinancialStatements` 用 `dateStr[5]` 當 quarter，實際是月份十位數 | `q := int(dateStr[5] - '0')`；`2026-03-31`→0、`2026-12-31`→1，不是 quarter | `internal/marketdata/finmind_client.go:289-315` | 新測試：3月→Q1、6月→Q2、9月→Q3、12月→Q4；現有錯誤測試更新 | pending | none | 證據: 測試註解自承「quarter=1 matches December」 |
| A03 | `auto_cycle_update` 60 秒總 timeout 無法容納全批次（~25 stocks × 多 fallback × FinMind 6s token） | `context.WithTimeout(ctx, 60*time.Second)` 包住 `AggregateAllIndustries`；每 symbol 10s sub-ctx 也常撞 rate limiter | `cmd/atlas/calibration_tasks.go:206-209`、`internal/industry/data_aggregator.go` | 批次拆 per-industry 或 bounded；task data-health 欄位填寫；不卡 timeout | pending | none | 證據: `finmind_daily_quota` 低用量但 `industrial` 仍失敗 |
| A04 | `tw_vol` freshness 判定把「平日」當「Yahoo 必須有今日 bar」，假日/盤前誤判 stale，且 transport error 與 stale-data 混計 | `isTaiwanTradingDay` 只排週末；無收盤寬限；Yahoo DNS 失敗時 breaker 因 data-freshness 判定加速開啟 | `internal/marketdata/taiwan_index_cache.go`、`internal/marketdata/taiwan_volatility_provider.go` | freshness 容許最近交易日；transport error 與 stale-data 分流；`LastDataAt` 記錄 | pending | none | 證據: 11 個 Yahoo 通道同段時間失敗（DNS）；tw_vol 無 TWSE fallback |
| A05 | `twse_etf` adapter 把所有「7 天無資料」當正常假日 stale，403/timeout/schema 改變無法區分 | `adapter_twse_etf.go:38-41` 對含 "no TWSE"/"no data" 的任何 error 回 `Stale:true` | `internal/marketdata/twse_etf_provider.go`、`internal/apigateway/adapter_twse_etf.go` | typed outcome（NoTradingData/UpstreamForbidden/TransportTimeout/SchemaMismatch）；只有 NoTradingData 轉 stale | pending | none | 證據: known_issues.go 已登錄 60+ 天 upstream 失敗 |

---

## Phase Tracker

### Phase A — Audit (read-only) — 已完成於 2026-08-10 盤查

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 盤查 tw_vol 全鏈 | - | accepted | 見 `history://TraceTwVol` + MCP channel health；Yahoo DNS failure |
| 盤查 twse_etf 全鏈 | - | accepted | 見 `history://TraceTwseEtf`；TWT44U 上游失敗、無獨立 producer |
| 盤查 auto_cycle_update 全鏈 | - | accepted | `1513.TW`/`1590.TW` MCP 驗證成功 vs aggregator 失敗；`.TW` 契約差異 |
| 驗證 FinMind quarter parser | A02 | accepted | `finmind_client_extra_test.go:260-293` 自承 heuristic |
| 驗證排程與回填鏈 | - | accepted | `background.go` / `calibration_tasks.go` / 無 CycleTracker persistence |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Impact 盤查 FinMind client | A01/A02 | in-progress | gitnexus_impact / codegraph |
| Impact 盤查 data_aggregator | A03 | pending | gitnexus_impact / codegraph |
| Impact 盤查 tw_vol provider | A04 | pending | gitnexus_impact / codegraph |
| Impact 盤查 twse_etf adapter | A05 | pending | gitnexus_impact / codegraph |
| 定義各 ID acceptance | 全部 | pending | manifest 上表 |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| PR1: FinMind symbol 正規化 + quarter parser | A01+A02 | pending | commit hash |
| PR2: auto_cycle_update ingestion 重構 | A03 | pending | commit hash |
| PR3: tw_vol freshness 修正 | A04 | pending | commit hash |
| PR4: twse_etf typed error | A05 | pending | commit hash |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 更新 manifest status | - | pending | - |
| `make ci-gate` / `make ci-full` | - | pending | exit 0 |
| Push branch / open PR | - | pending | PR # |
| `make check-binaries` + `git cleanup-tools` | - | pending | exit 0 |
| 決定 manifest 歸屬 | - | pending | docs/documentation-standard.md |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B06 | 資料通道 breaker（apigateway.CircuitBreakerManager）無 HTTP/MCP 暴露 | 2026-08-10（B04 實作中發現） | observability 增強，另案 |

> **已完成**：A01-A05（#1502）、B05+B04（#1503）、B03（#1504）+ 表述修正（#1505）、B02+B01（#1506）。Backlog 核心項全數關閉。

## B03 實測記錄（2026-08-10，容器內 curl）

| 檢查 | 結果 |
|------|------|
| `TWT44U?response=json&date=20260810` | **HTTP 307 → page-not-found.html (404)** |
| `TWT44U` 其他日期/無參數/CSV | 同樣 307 → 404 |
| 對照組 `STOCK_DAY_ALL` | **HTTP 200 CSV**（出站 IP 正常 → 非 rate-limit） |
| 替代候選 TWT85U/TWT84U/TWTB7U | 200 JSON 但內容是變更交易/升降幅度/面額，非 ETF 申贖 |
| FinMind dataset enum | 僅 `TaiwanStockActiveETFInfo/Holding`（ETF 持股），**無申購贖回** |
| 2026 ETF 申贖平台 | 2026-07 新增「整批上傳」功能優化（B2B 平台自 2004 年存在，非黑箱化）；ETFortune 仍公開 NAV/PCF/折溢價 |

**結論**：TWT44U 彙總報表（全市場 ETF 申購贖回淨額）移除；ETF 投資人資訊（NAV/PCF/折溢價）仍公開於 ETFortune，但申購贖回淨額無等價公開替代（OpenAPI opendata 44 個無此項、FinMind 僅 ETF 持股）。known_issues 舊「403/likely IP rate-limit」描述為錯誤推測，已更新。ETFNetSubscription 因子停用（subC3 資料缺失不貢獻）— 因替代源語義不符（PCF/NAV ≠ 申贖淨額），功能決定維持。

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- No commit without acceptance criteria passing
- PR body must reference this manifest: `See docs/manifests/2026-08-10-data-chain-recovery.md`

---

## Session-End State

- **Done this session**: A01+A02（FinMind 正規化 + quarter parser）、A03（批次容量 + data-health）、A04（tw_vol freshness + 分流）、A05（twse_etf typed error）— 全部 commit 並過 CI
- **Remaining**: docker images 對齊（需 kaecer 主 worktree `make rebuild-all`）；B01-B05 backlog
- **Next action**: PR #1502 review → merge → post-merge cleanup（branch 刪除）
- **Uncommitted code**: 無（docs/archive 修改為既有遺留，非本 session 工作）
- **Branch / PR**: fix/20260810-data-chain-recovery / https://github.com/kaecer68/atlas-go/pull/1502
- **Paused because**: -

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-10 | 1.0 | Initial manifest（A01-A05 from audit） | agent |
| 2026-08-10 | 1.1 | A01-A05 全部實作 + commit 7daee92e..4b3eb9b9 + PR #1502 | agent |
| 2026-08-10 | 1.2 | B05+B04 實作（PR #1503）；B06 加入 backlog | agent |
| 2026-08-10 | 1.3 | B03 實測定案（TWT44U 移除）+ ETF 因子停用（PR #1504） | agent |
| 2026-08-10 | 1.4 | B03 表述修正（PR #1505）；B02+B01 history store（PR #1506）— backlog 核心項關閉 | agent |
