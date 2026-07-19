# Audit Manifest: CL-2 macro/snapshot/history 缺時序 API — 設計與實作

> **Audit source**: [[atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19]] + [[atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20]] §2
> **承接**: docs/manifests/2026-07-20-capital-flow-history-audit.md（CL-1 修復後未解決的 BL-CL2 範圍）
> **Goal**: 補齊 `/api/macro/snapshot/history` 的時序端點，讓 hermes 可對 80+ 已存在的 macro snapshot 做時序分析（regime score 替代、跨日 macro 對齊、T+N retrospective 對齊）
> **Scope**: MEDIUM — 新增 1 個 endpoint + 1 個 service method + MCP wrapper 修正（既有 silent bug 一起解）+ spec 新章節。明確不做：snapshot 資料層補建（已有 80+ 檔）、CL-3 regime store、CL-5 HandleHistory 程式實作 — 一律入 Backlog。
> **Created**: 2026-07-20
> **Status**: planning

---

## 證據鏈摘要

| 證據層 | 來源 | 結論 |
|--------|------|------|
| 底層資料存在 | `/Users/kaecer/workspace/atlas/data/state/macro/` | 80 個 dated snapshot 從 2026-04-21 起；非連續（週末 + 假日缺） |
| Handler 程式碼 | `internal/monitoring/api/macro/handlers.go:47-60` | `HandleMacroSnapshotHistory` 只接 `?date=YYYY-MM-DD` 單一日期 |
| Service 程式碼 | `internal/monitoring/service/macro.go:68-82` | `GetSnapshotByDate` 直接讀 `{SnapshotDir}/{date}.json`；無 list/range helper |
| MCP wrapper | `cmd/atlas-mcp/server/tools_macro.go:74-90` | `handleMacroGetSnapshotHistory` **送 `?days=N`（預設 30、上限 365）** 但 handler 不收 → 每次呼叫必然回 400（silent bug） |
| MCP wrapper test | `cmd/atlas-mcp/server/tools_macro_test.go:23-56` | 測試只驗 query 構造（days=30 / clamp 365），沒驗 handler 真的能解析 — 漏掉 silent bug |
| Whitelist | `cmd/atlas/main.go:isPublicPath` | `/api/macro` 全 prefix 已 public，新端點不需加白名單 |
| ValidateDateParam | `internal/monitoring/api/shared/paths.go:62` | regex `^\d{4}-\d{2}-\d{2}$`，可重用 |
| Module pitfalls | `internal/monitoring/AGENTS.md` | 「公開端點需同步加白名單」（本 manifest 不適用，已 public）；「JSON tag snake_case」；「defer Close 用 closure + logging」 |

**根因判定**：`HandleMacroSnapshotHistory` 設計時就只規劃單日查詢（handler/service/URL 三層都綁死 `?date=`），沒有 range/range-relative 的概念。底層 snapshot 是 dated file，物理上支援 range scan（`os.ReadDir` + prefix filter），只是沒人寫這條路徑。

**判定信心**：high（5 層獨立證據互不依賴 + MCP silent bug 是額外確證）。

---

## 設計選項

### 選項 A：在既有 `/api/macro/snapshot/history` 加 `?from=&to=` 或 `?days=N`

- 優點：向後相容（既有 `?date=` 仍 work）；零新 endpoint
- 缺點：API 表面長出兩個相似但不同的語意（date-lookup vs range）→ client 易混淆
- 影響：~15 行 handler + 30 行 service + 改 MCP wrapper（query 構造不變）+ spec 文件

### 選項 B（**採用**）：新增 `/api/macro/snapshot/timeline` 端點

- 優點：語意分離明確（point-in-time vs range）；既有 `/history` 不動，向後相容；MCP wrapper 改指向新端點即可
- 缺點：API surface 增加（1 個新路徑）；前端需新整合（但當前無 frontend consumer，僅 MCP → 0 影響）
- 影響：~30 行新 handler + 40 行新 service + 改 MCP wrapper 指向 + spec 新章節

### 選項 C：合併 A + B 與 `?date=` 廢棄

- 風險：對既有 consumer 破壞性變更；migration 路徑複雜
- 不採用 — wiki §2 建議為「短期內不廢既有 endpoint」

**選項 B 採納理由**：
1. wiki §2 明列 B 為「建議」
2. `/api/macro/snapshot/timeline` 命名比 `?days=N` 更明確傳達 range 語意
3. 既有 `/api/macro/snapshot/history` 仍保留 `?date=` 給 point-in-time 使用（無 migration）
4. MCP wrapper 從「送錯 query 參數」改為「指向正確端點」，是 bug fix 而非 breaking change

---

## Blast Radius（pre-change protocol Step 1）

### 新增 endpoint `/api/macro/snapshot/timeline`

- Risk: **LOW**
- New handler: `HandleMacroSnapshotTimeline` on `Handlers` struct
- New service method: `ListSnapshotsInRange(from, to string, limit int)` on `*MacroService`
- Direct callers: 0（純新增）
- Indirect callers: 1（MCP wrapper `handleMacroGetSnapshotHistory` 改指向新路徑）
- Modules affected: 1（`monitoring`）

### 修改 MCP wrapper `cmd/atlas-mcp/server/tools_macro.go:74-90`

- Risk: **LOW**
- 既有 function `handleMacroGetSnapshotHistory` 內 1 行 `path` 改寫
- 既有 test `TestHandleMacroGetSnapshotHistory_DefaultDays` + `TestHandleMacroGetSnapshotHistory_ClampedTo365` 不需動（query 構造不變）
- 新增 test：handler 真的回傳 array of snapshots

### 修改 `Handlers` struct 註冊路由

- Risk: **LOW**
- 1 行 `mux.Handle("GET /api/macro/snapshot/timeline", shared.Get(h.HandleMacroSnapshotTimeline))`
- Whitelist 不需動（`/api/macro` prefix 已 public）

### 不變的既有函式

- `GetSnapshotByDate(date)` — **保留**，既有 `/api/macro/snapshot/history?date=` 仍用
- `HandleMacroSnapshotHistory` — **保留**，既有 `?date=` 查詢不破壞

---

## Invariant Tracker

### 線 A：CL-2 時序端點設計與實作

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| **A01** | `/api/macro/snapshot/history` 沒有時序語意（point-in-time only），底層 80+ dated snapshot 無法以 range 查詢 | **accepted**：handler/service/URL 三層綁死 `?date=` 單日，無 range helper | `docs/specs/macro-snapshot-history-spec.md`（新增 §1-§4）+ `internal/monitoring/service/macro.go`（新增 `ListSnapshotsInRange` + `parseSnapshotDate` helper）+ `internal/monitoring/api/macro/handlers.go`（新增 `HandleMacroSnapshotTimeline` + 註冊 `GET /api/macro/snapshot/timeline`）+ `cmd/atlas-mcp/server/tools_macro.go`（`handleMacroGetSnapshotHistory` 改指向 `/api/macro/snapshot/timeline` + 新增對應 test） | (1) `go build ./...` 全綠；(2) 新增 6 個 unit test 全 PASS：`TestListSnapshotsInRange_FullRange`、`TestListSnapshotsInRange_MissingDates`、`TestListSnapshotsInRange_CapacityClamp`、`TestListSnapshotsInRange_DateParseError`、`TestHandleMacroSnapshotTimeline_OK`、`TestHandleMacroSnapshotTimeline_BadDateParams`；(3) 既有 `TestHandleMacroGetSnapshotHistory_*` 改 path 驗證後仍 PASS；(4) 既有 `TestService_*` 全綠；(5) `bash scripts/ci/check_atlas_mcp_docs_consistency.sh` 全綠 | pending | **CF-MS-01/02/03** 寫入 spec §3 invariants | 影響：~70 行新程式碼 + 60 行 spec + 80 行 test；新 endpoint 1 個 |
| **A02** | MCP wrapper `macro_get_snapshot_history` 送 `?days=N` 給只接 `?date=` 的 handler → 每次呼叫必然 400（silent bug，無 log、無 alert） | **accepted**：wrapper 與 handler query contract 不一致；test 只驗 query 構造未驗 handler 解析 | （隨 A01 一起解：wrapper 改指向新 timeline endpoint，handler 真的支援 `?days=N`） | (1) A01 完成後，MCP `macro_get_snapshot_history` tool 在本地以 `days=30` 呼叫能回 200 + array；(2) test `TestHandleMacroGetSnapshotHistory_DefaultDays` 改 path 驗證後仍 PASS；(3) `cmd/atlas-mcp` 套件測試全綠 | pending | （無 spec 變更；MCP tool behavior 改進屬於 bug fix） | 屬於 A01 連帶效益；獨立驗收點 |
| **A03** | spec 缺乏「historical macro snapshot timeline」語意契約，且缺 invariants 守護 CL-2 的修法不再次退化 | **accepted**：macro 域目前沒有 timeline API 的 spec 文件；只有 `macro-category-spec.md`（不同主題） | `docs/specs/macro-snapshot-history-spec.md`（新檔案，4 個章節：query 參數 / response shape / invariants / 與既有 endpoint 關係） | (1) §3 含 3 條 invariants：CF-MS-01（不補假資料）、CF-MS-02（capacity 限制 ≤365 trading days）、CF-MS-03（無資料日 skip + 進 `missing_dates`）；(2) §2 含 response 完整 JSON shape 含 `trading_date` / `recorded_at` / `source_status` / `missing_dates`；(3) §4 引用既有 `data/state/macro/` 目錄契約與 `internal/monitoring/AGENTS.md` 「公開端點需同步加白名單」（已 public，不適用）；(4) `bash scripts/ci/check_markdown_links.sh` 全綠 | pending | **spec 新檔**：`macro-snapshot-history-spec.md` | 純文件；無 code 變更 |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| 6 條 CL 真相盤查 | done | `atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md` |
| CL-2 根因定位（單日查詢設計 / 無 range helper） | done | 4 層證據（handler/service/URL/MCP wrapper） |
| MCP silent bug 發現 | done | `tools_macro.go:74-90` + `tools_macro_test.go:23-56`（test 未涵蓋 handler 解析） |
| 影響範圍 blast radius | done | gitnexus_impact：HandleMacroSnapshotHistory 0 upstream + GetSnapshotByDate 1 caller（皆 LOW） |
| 設計選項拍板 | done | Option B（新增 timeline endpoint）— wiki §2 建議 + 語意分離考量 |

### Phase B — Plan（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker（A01-A03） |
| 預期 commit 順序 | planned | A03（spec）→ A01+A02（code + test + MCP wrapper fix，1 PR 內） |
| Spec outline | done | 本文件 §設計細節 + 即將產出 `docs/specs/macro-snapshot-history-spec.md` |

### Phase C — Implement（pending 15:30 B4 驗證後啟動）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Spec 新檔 macro-snapshot-history-spec.md | A03 | pending | 待 commit |
| Service `ListSnapshotsInRange` + helper + tests | A01 | pending | 待 commit |
| Handler `HandleMacroSnapshotTimeline` + route + tests | A01 | pending | 待 commit |
| MCP wrapper 改 path + 新增解析測試 | A01+A02 | pending | 待 commit |

### Phase D — Close Out（pending）

| Task | Status | Evidence |
|------|--------|----------|
| 驗收：`go build ./...` + `go test ./internal/monitoring/... ./cmd/atlas-mcp/...` + markdown link check + doc consistency check 全綠 | pending | - |
| Worktree branch push + PR 開啟 + CI 綠 | pending | PR # |
| Backlog 維持 CL-3 regime observation store + CL-5 HandleHistory code | pending | - |
| Post-merge cleanup per `docs/multi-cli-protocol.md` | pending | - |

---

## 設計細節（給 A03 spec 撰寫用）

### API Surface

```
GET /api/macro/snapshot/timeline
  Query params (三選一，優先順序：from/to > days > default):
    from=YYYY-MM-DD   # range start (inclusive)
    to=YYYY-MM-DD     # range end (inclusive), default = today (Asia/Taipei)
    days=N            # range length relative to today (default 30, max 365)

  Response 200:
    {
      "snapshots": [
        {
          "trading_date": "2026-04-21",
          "recorded_at": 1784217600,        // Unix seconds; 0 if missing
          "snapshot": { ...MacroDataSnapshot... },  // null if file missing/corrupt
          "source_status": "complete" | "partial" | "missing"  // see CF-MS-03
        },
        ...
      ],
      "range": { "from": "2026-04-21", "to": "2026-07-20" },
      "capacity_limit_hit": false,          // true if requested range > 365 days
      "missing_dates": ["2026-04-25", ...],  // dates within range with no snapshot file
      "stats": {
        "requested_count": 91,
        "returned_count": 78,
        "missing_count": 13
      }
    }

  Response 400:
    { "error": "from must be before to" | "invalid date format" | "days out of range" }

  Response 500:
    { "error": "snapshot directory read failed" }
```

### Service Method Signature

```go
// ListSnapshotsInRange reads dated snapshot files from SnapshotDir()
// between from and to (inclusive, YYYY-MM-DD format). Returns snapshots
// in trading_date ascending order. Missing/corrupt files are skipped
// per CF-MS-03 (not patched with zero values), and their dates are
// reported in MissingDates. limit caps the response size; if the
// requested range exceeds limit, capacityLimitHit is true and the
// response includes only the most recent `limit` snapshots.
//
// Empty from or empty to are treated as "no bound" (e.g. to == "" → today).
func (s *MacroService) ListSnapshotsInRange(
    ctx context.Context, from string, to string, limit int,
) (snapshots []TimelineEntry, missingDates []string, capacityLimitHit bool, err error)

type TimelineEntry struct {
    TradingDate string                    `json:"trading_date"`
    RecordedAt  int64                     `json:"recorded_at"`
    Snapshot    *marketdata.MacroDataSnapshot `json:"snapshot"`  // nil if missing
    SourceStatus string                   `json:"source_status"`  // complete|partial|missing
}
```

### Invariants

- **CF-MS-01（不補假資料）**：缺失日期的 `snapshot` 欄位為 `null`，`source_status: "missing"`，日期進 `missing_dates`。**禁止**插入零值或預設 snapshot。
- **CF-MS-02（capacity 限制）**：`limit` 上限 365 trading days（per wiki §2 沿用 MCP wrapper 既有的 365 上限）；超量時 `capacity_limit_hit: true` + 回傳最近 N 筆（不報錯）。
- **CF-MS-03（無資料日 skip）**：JSON 解析失敗 / 檔案不存在 → skip，日期進 `missing_dates`，**不拋 error**（避免單日 corrupt 影響整個 range 回傳）。
- **CF-MS-04（trading_date 為主鍵）**：以 snapshot filename 的 `YYYY-MM-DD` 作為 trading_date 鍵，**不**用 `recorded_at` Unix 時間（per CL-6 語意分離原則 — filename = 資料所屬日期，recorded_at = 抓取時間）。

### 與既有 endpoint 關係

| Endpoint | Query | 用途 |
|----------|-------|------|
| `GET /api/macro/snapshot/latest` | 無 | 拿最新一份 snapshot |
| `GET /api/macro/snapshot/history?date=YYYY-MM-DD` | `date` | 拿指定日期單一 snapshot |
| `GET /api/macro/snapshot/timeline` | `from`/`to`/`days` | 拿 range 內所有 snapshot（本 manifest 新增） |

---

## 預期 Commit 順序

| # | Format | 內容 |
|---|--------|------|
| 1 | `docs(manifest): #A03 add macro-snapshot-history-spec.md with CF-MS-01/02/03/04` | 純 spec 文件，無 code 變更 |
| 2 | `feat(manifest): #A01 add HandleMacroSnapshotTimeline + ListSnapshotsInRange` | service method + handler + route + tests |
| 3 | `fix(manifest): #A02 point MCP macro_get_snapshot_history to /timeline endpoint` | MCP wrapper path 修正 + 解析測試 |

PR body 必須引用：`See docs/manifests/2026-07-20-cl2-macro-snapshot-history.md`

---

## Backlog（明確不做，入後續輪次）

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|----------------|----------------|
| **BL-CL3** | `regime_get_history` 回 simulation session 摘要 + 當下 score，非時序；缺 RegimeObservationStore infra | 2026-07-20 | 待 JANUS 6h 排程與新 store infra 評估 |
| **BL-CL4** | `universe_get_sessions` 只回 session metadata，不含 per-strategy 5 主體力值 | 2026-07-20 | 評估 drill-down session endpoint |
| **BL-CL5** | HandleHistory 對缺失 dimension 現 silent omission（spec §14 + CF-INV-17 已寫，**程式實作**待補） | 2026-07-20 | CL-2 後下一輪 |
| **BL-CL6** | `recorded_at` 與 snapshot filename 日期語意混淆 | 2026-07-20 | 評估 spec 加 §「date vs recorded_at 雙欄位語意」 |
| **BL-MS-01** | SnapshotDir 容量若無限增長是否需要 trim？目前 80+ 檔，一年後 ~250，5 年後 ~1250；長期 cap 在 5 年（~1260 trading days） | 2026-07-20 | 評估 cleanup job；短期不處理（<10K 不影響效能） |

---

## 設計決策（已從證據推出，不開放重選）

| 決策 | 結論 | 證據 |
|------|------|------|
| **端點路徑** | `GET /api/macro/snapshot/timeline` | wiki §2 選項 B 建議；語意分離（point-in-time vs range） |
| **Query 優先順序** | `from/to` > `days` > default(30) | expressiveness from 最明確；MCP wrapper 只送 `days` 仍 work |
| **時區** | 沿用 `ValidateDateParam` 既有 `YYYY-MM-DD` 格式 | 既有 regex 鎖住 `^\d{4}-\d{2}-\d{2}$`；server 端不在 URL 上做時區轉換 |
| **Sort order** | `trading_date` ascending | 既有 `applyUpsert` 與 `CalculateStressIndex` 都按時間排序；行為一致 |
| **Capacity 上限** | 365 trading days（per MCP wrapper 既有的 clamp） | MCP wrapper 已 clamp 365；新 endpoint 沿用 |
| **容量超限行為** | 不報錯，回傳最近 N 筆 + `capacity_limit_hit: true` | hermes UX：client 可選擇擴大或接受截斷 |
| **檔案損壞處理** | skip + 進 `missing_dates` | CF-MS-03；避免單檔 corrupt 影響整個 range |
| **whitelist** | 不需動（`/api/macro` prefix 已 public） | `cmd/atlas/main.go isPublicPath` 已涵蓋 |
| **MCP wrapper 修正** | 改 path 至 `/timeline` + 新增 path 驗證 test | A02 bug fix 連帶 A01 一起 ship |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- 一 ID 一 commit（A01+A02 因範圍小且連帶效益合併為 1 個 code commit）
- 驗收標準未過不得 commit
- PR body 必須引用：`See docs/manifests/2026-07-20-cl2-macro-snapshot-history.md`
- 不直接推 main；CI 綠才 merge；merge 後依 `docs/multi-cli-protocol.md` 清理分支
- 修改 function/class 前先跑 gitnexus_impact（pre-change protocol，已記錄於 §Blast Radius）

---

## 風險與緩解

| 風險 | 等級 | 緩解 |
|------|------|------|
| SnapshotDir 容量無限增長影響 range query 效能 | LOW | 短期 80+ 檔 O(n) 沒問題；BL-MS-01 評估長期 cap |
| ListSnapshotsInRange 在 SnapshotDir 有非預期檔案（如 latest.json、previous.json、_metadata.json）時誤抓 | MEDIUM | 過濾條件：`HasSuffix(name, ".json") && name != "latest.json" && name != "previous.json" && name != "_metadata.json"` + parse trading_date 前綴 `^\d{4}-\d{2}-\d{2}\.json$` |
| JSON 解析失敗時整個 range 失敗 | MEDIUM | per-file recover：try decode → fail 跳過、日期進 missing |
| MCP wrapper 修正破壞既有 test | LOW | test 只驗 query 構造，path 改寫不影響既有 assertion；新增 path 驗證 test |
| 新 endpoint 命名衝突（既有 `/api/macro/snapshot/history` vs `/timeline`） | LOW | 兩者 query 語意不同（date vs range）；MCP wrapper 顯式指向；spec §4 明確分開 |
| Manifest v1.0 review 後業主推翻設計 | MEDIUM | 設計皆從證據推導（wiki §2 沿用）；如推翻則改 §設計選項 |

---

## 不可動的事項（mission 紀律）

- 不改 manifest E05 / production 權重 / parameters.json
- 不補假資料（缺失日進 `missing_dates`，不插入零值）
- 不動 `internal/monitoring/service/macro.go` 既有的 `GetSnapshotByDate`、`GetLatestSnapshot`、`CalculateStressIndex`
- 不動 `internal/monitoring/api/macro/handlers.go` 既有的 `HandleMacroSnapshotHistory`（保留 `?date=` 向後相容）
- 不動既有的 `internal/monitoring/AGENTS.md` 「公開端點需同步加白名單」（已 public）
- 不直接 push main（必須 worktree + PR + CI 綠）
- 不在 Phase C 啟動前動 code（等 B4 15:30 CST 實測驗證 A01 通過後再啟動）

---

## Session-End State

- **Done this session**: Phase A 真相盤查（沿用 wiki §2）+ Phase B manifest 撰寫 + Phase A0 spec outline（本文件 §設計細節）
- **Remaining**: 
  1. 寫 `docs/specs/macro-snapshot-history-spec.md`（A03 — 下一步立即可做）
  2. 跑 B4 驗證（2026-07-20 15:30 CST 後檢查 `data/state/capital_flow_rolling.json` 是否開始累積 7/20 樣本）
  3. Phase C 程式實作（A01 + A02）— 等 B4 通過後啟動
  4. Phase D close out（開 PR + CI 綠 + merge + cleanup）
- **Uncommitted code**: none（本 manifest 為純設計文件）
- **Branch / PR**: `feat/cl2-macro-snapshot-history` / 未開 PR（尚無 code）
- **Worktree**: `.worktrees/feat-cl2-macro-snapshot-history`（d1ebe39a base）
- **Paused because**: 等 B4 實測驗證 A01 + 等業主 review manifest §設計細節
- **Uncommitted spec**: `docs/specs/macro-snapshot-history-spec.md`（待 A03 commit）

---

## 給下一個 session 的具體行動指令

1. **立即可做**：寫 `docs/specs/macro-snapshot-history-spec.md`（§設計細節已在本 manifest §設計細節；commit `docs(manifest): #A03`）
2. **15:30 CST 之後**：跑 `cat data/state/capital_flow_rolling.json | python3 -c "import json,sys; d=json.load(sys.stdin); print({k: sorted({s[\"trading_date\"] for s in v}) for k,v in d.get('samples', {}).items()})"` 看 store 是否開始累積新樣本
3. **若 B4 通過**：啟動 A01+A02 程式實作，4 個 commit（spec / service / handler+MCP wrapper / test）
4. **若 B4 失敗**：開新 manifest（命名格式 docs/manifests/YYYY-MM-DD-capital-flow-cl1-verify-failure.md），暫停 CL-2 等根因釐清

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest（3 IDs A01-A03，承接 wiki §2 + 真相盤查） | OpenCode CLI Agent (Sisyphus) |

---

## 附錄：3 個產出檔案路徑預覽

| 檔案 | 內容 | 預估行數變動 |
|------|------|------------|
| `docs/specs/macro-snapshot-history-spec.md` | 新檔：query params / response shape / 4 條 invariants / 與既有 endpoint 關係 | +200（新檔） |
| `internal/monitoring/service/macro.go` | 新增 `TimelineEntry` struct + `ListSnapshotsInRange` method + `parseSnapshotDate` helper | +80 |
| `internal/monitoring/api/macro/handlers.go` | 新增 `HandleMacroSnapshotTimeline` + 註冊 `GET /api/macro/snapshot/timeline` | +30 |
| `cmd/atlas-mcp/server/tools_macro.go` | `handleMacroGetSnapshotHistory` path 改 `/timeline` | +5 / -5 |
| `internal/monitoring/service/macro_test.go`（若存在） | 新增 4 個 ListSnapshotsInRange test | +120 |
| `internal/monitoring/api/macro/handlers_test.go` | 新增 2 個 HandleMacroSnapshotTimeline test | +60 |
| `cmd/atlas-mcp/server/tools_macro_test.go` | 新增 1 個 MCP wrapper path 驗證 test | +20 |

**總計**：~300 行新程式碼 + 200 行新 spec，分散在 6-7 個檔案，3 個 atomic commits（spec → code → test/MCP fix 為 1 commit）。
