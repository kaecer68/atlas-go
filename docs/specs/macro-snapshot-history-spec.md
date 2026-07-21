# Spec: macro snapshot timeline API

> **Spec owner**: OpenCode CLI Agent (Sisyphus)
> **Audit source**: `.omo/manifests/2026-07-20-cl2-macro-snapshot-history.md`
> **承接**: docs/specs/macro-category-spec.md（不同主題；本 spec 為 macro 域 timeline 端點專屬）
> **Created**: 2026-07-20
> **Status**: Phase B 草案（待 review + Phase C 實作）

---

## 1. 範圍與動機

### 1.1 問題陳述

`/api/macro/snapshot/history?date=YYYY-MM-DD` 端點自設計之初僅支援「拿指定日期單一 snapshot」。但底層 `data/state/macro/` 自 2026-04-21 起每日（或近每日）累積 dated snapshot，累計 80+ 檔案。對使用方（hermes agent、dashboard pages、MCP `macro_get_snapshot_history` tool）的真實需求是：

- **時序分析**：跨日 macro 對齊（例如「最近 30 天 DXY 走勢」）
- **Retrospective 對齊**：T+N retrospective（例：今天回頭看 7 天前 DXY 變化）
- **Regime 替代**：CL-3 尚未修復前，可用 macro timeline 暫時替代 regime 時序研究

此 spec 定義一個**新端點** `/api/macro/snapshot/timeline`，專門服務 range 查詢。**不破壞**既有 `?date=` 端點的向後相容性。

### 1.2 不在範圍

- CL-3 regime observation store（BL-CL3）— 待 JANUS 6h 排程評估
- CL-5 capital-flow HandleHistory 程式實作（BL-CL5）— 下一輪
- SnapshotDir 容量管理（BL-MS-01）— 評估中
- frontend 整合（dashboard / MCP 之外的 client）— 本 spec 不約束

---

## 2. API Surface

### 2.1 Endpoint

```
GET /api/macro/snapshot/timeline
```

**Auth**：公開端點（沿用 `/api/macro/*` 既有的 `isPublicPath` whitelist）。

**Query Parameters**（三選一；優先順序 `from/to` > `days` > default(30)）：

| 參數 | 格式 | 預設 | 上限 | 說明 |
|------|------|------|------|------|
| `from` | `YYYY-MM-DD` | — | — | range 起始（inclusive） |
| `to` | `YYYY-MM-DD` | 今天（Asia/Taipei） | — | range 結束（inclusive） |
| `days` | int | 30 | 365 | 從今天往前推 N 天 |

**參數組合規則**：

- 若 `from` 與 `days` 同時存在 → 400（互斥）
- 若 `from > to` → 400
- 若 `days <= 0` 或 `days > 365` → 400
- 若 `from` 與 `to` 皆無 → 用 `days=30`
- 僅有 `from`（無 `to`）→ `to = today`
- 僅有 `to`（無 `from`）→ `from = to - 30 days`（行為：往回 30 天）

### 2.2 Response 200

```json
{
  "snapshots": [
    {
      "trading_date": "2026-04-21",
      "recorded_at": 1784217600,
      "snapshot": {
        "recorded_at": 1784217600,
        "DXY": { "symbol": "DX-Y.NYB", "value": 105.3, "change_pct": -0.12, "timestamp": 1784217600 },
        "VIX": { ... },
        "US10Y": { ... },
        "USD_TWD": { ... },
        "Oil": { ... },
        "Gold": { ... },
        "JPY": { ... },
        "ForeignInvestorNet": 123.45,
        "DomesticFundNet": -50.12,
        "DealerNet": -73.33,
        "TSMADRA": -2.77
      },
      "source_status": "complete"
    },
    {
      "trading_date": "2026-04-22",
      "recorded_at": 0,
      "snapshot": null,
      "source_status": "missing"
    }
  ],
  "range": {
    "from": "2026-04-21",
    "to": "2026-07-20"
  },
  "capacity_limit_hit": false,
  "missing_dates": ["2026-04-22", "2026-04-25"],
  "stats": {
    "requested_count": 91,
    "returned_count": 78,
    "missing_count": 13
  }
}
```

### 2.3 Response 400（參數錯誤）

```json
{
  "error": "from must be before to"
}
```

錯誤情境：

- `from > to`
- `days <= 0` 或 `days > 365`
- `from` 與 `days` 同時存在（互斥）
- 日期格式不合法（不符 `^\d{4}-\d{2}-\d{2}$`）

### 2.4 Response 500（服務錯誤）

```json
{
  "error": "snapshot directory read failed: <wrapped error>"
}
```

僅在 `SnapshotDir()` 本身不可讀時觸發。**單檔損壞不算 500**（per CF-MS-03 skip）。

### 2.5 Response 200 但 `snapshots: []`

合法的空回應情境：當 range 完全落在未來，或所有日期都缺資料時。`stats` 仍正常回報，`missing_dates` 列出所有請求日期。

---

## 3. Invariants

### CF-MS-01：不補假資料

缺失日期的 `snapshot` 欄位為 `null`，`source_status: "missing"`，日期進 `missing_dates`。

**禁止行為**：

- 禁止插入零值（per AGENTS.md 公開資料陷阱）
- 禁止沿用前一交易日資料當今日值
- 禁止沿用後一交易日資料回填
- 禁止為缺失日期建立空物件 placeholder

**允許行為**：

- `snapshot` 為 `null` + `source_status: "missing"` + `missing_dates` 包含此日期

**違反偵測**：unit test 必須覆蓋（1）檔案不存在（2）檔案存在但 JSON 解析失敗（3）檔案存在但無 `recorded_at` 欄位 三種缺失情境。

### CF-MS-02：Capacity 限制

`limit` 上限 365 trading days（沿用 MCP wrapper 既有的 `days > 365 → clamp 365` 行為）。

**超量行為**：當 requested range > 365 days → `capacity_limit_hit: true` + 回傳**最近 365 筆**（不是 400 報錯）。

**理由**：hermes UX — client 可選擇接受截斷繼續分析，無需重新計算參數。

**範例**：`?days=500` → `capacity_limit_hit: true`、`returned_count: 365`、snapshots 為最近 365 個交易日。

### CF-MS-03：無資料日 skip

JSON 解析失敗 / 檔案不存在 / 非預期檔名（不符合 `^\d{4}-\d{2}-\d{2}\.json$`）→ skip，日期進 `missing_dates`，**不拋 error**。

**跳過規則**：

| 情境 | 行為 |
|------|------|
| 檔案不存在 | skip + 進 missing_dates |
| 檔案存在但非 `.json` 結尾 | 不列入 requested range（不計入 missing） |
| 檔案名稱不符 `^\d{4}-\d{2}-\d{2}\.json$` | 不列入 requested range |
| `latest.json` / `previous.json` / `_metadata.json` | **永遠排除**（即便日期格式符合也不處理） |
| JSON 解析失敗 | skip + 進 missing_dates |
| `recorded_at` 為 0 | `recorded_at: 0` + `source_status: "missing"`（per CF-MS-01） |

### CF-MS-04：Trading date 為主鍵（不混用 recorded_at）

以 snapshot **filename 的 `YYYY-MM-DD`** 作為 `trading_date` 鍵，**不**用 `recorded_at` Unix 時間。

**理由**：

- 與 wiki §「CL-6 RecordedAt vs filename date 語意分離」一致
- filename = 資料所屬日期（trading_date）
- recorded_at = provider 抓取時間（可能在週末/假日落後數日）
- 兩個欄位在 response 中**同時保留**：`trading_date`（鍵）、`recorded_at`（metadata）

---

## 4. 與既有 Endpoint 的關係

### 4.1 Endpoint 對照表

| Endpoint | Query | 用途 | Response shape |
|----------|-------|------|----------------|
| `GET /api/macro/snapshot/latest` | 無 | 拿最新一份 snapshot | `{ ...MacroDataSnapshot... }` |
| `GET /api/macro/snapshot/history?date=YYYY-MM-DD` | `date` | 拿指定日期單一 snapshot | `{ ...MacroDataSnapshot... }` |
| `GET /api/macro/snapshot/timeline`（**新**） | `from` / `to` / `days` | 拿 range 內所有 snapshot | `{ snapshots: [...], range, capacity_limit_hit, missing_dates, stats }` |

### 4.2 Backward Compatibility

- 既有 `?date=` 端點**保留不動**（handler/service/URL 路徑全不變）
- 既有 MCP wrapper `handleMacroGetSnapshotHistory` **改指向** `/api/macro/snapshot/timeline`（per A02 bug fix）
- 既有 tests 全保留；新增對應 timeline tests

### 4.3 Consumer Migration Path

| Consumer | 現況 | 影響 |
|----------|------|------|
| `cmd/atlas-mcp/server/tools_macro.go:74-90` | 送 `?days=N` 給 `/api/macro/snapshot/history`（handler 不收 → 必然 400） | 改 path 至 `/timeline` 即解 |
| `cmd/atlas/dailyreport_provider.go` | 透過 `latest.json` 拿 latest | 不受影響（無 range query） |
| `internal/monitoring/dashboard_api_test.go` | 既有測試用 `/history?date=` | 不受影響（path 不變） |

**Migration 時間軸**：無需使用者介入。本 spec 是 additive change。

---

## 5. Service Layer Contract

### 5.1 Method Signature

```go
// ListSnapshotsInRange reads dated snapshot files from SnapshotDir()
// between `from` and `to` (inclusive, YYYY-MM-DD format). Returns
// snapshots in trading_date ascending order.
//
// Behavior:
//   - Missing/corrupt files are skipped per CF-MS-03 (NOT patched with
//     zero values), and their dates are reported in MissingDates.
//   - `limit` caps the response size; if the requested range exceeds
//     limit, capacityLimitHit=true and the response includes only the
//     most recent `limit` snapshots.
//   - `limit <= 0` is treated as no limit (use with caution).
//
// Empty from/to are interpreted as:
//   - from == ""  → no lower bound (subject to limit)
//   - to == ""    → today (Asia/Taipei)
//
// Returns:
//   - snapshots:       ordered by trading_date ASC
//   - missingDates:    dates in range with no/corrupt snapshot file
//   - capacityLimitHit: true if range > limit and truncation occurred
//   - err:             only on SnapshotDir unreadable (per-file errors
//                      are swallowed per CF-MS-03)
func (s *MacroService) ListSnapshotsInRange(
    ctx context.Context,
    from string,
    to string,
    limit int,
) (snapshots []TimelineEntry, missingDates []string, capacityLimitHit bool, err error)
```

### 5.2 TimelineEntry Struct

```go
// TimelineEntry is one slot in a macro snapshot timeline response.
//
// Per CF-MS-01 / CF-MS-04:
//   - TradingDate is the snapshot filename date (data's date).
//   - Snapshot is nil when the file is missing/corrupt (NOT zero-patched).
//   - RecordedAt is the provider's recorded_at (Unix seconds; may lag
//     TradingDate by 1-3 days for weekend/holiday ingestion).
//   - SourceStatus reflects whether Snapshot is usable.
type TimelineEntry struct {
    TradingDate  string                         `json:"trading_date"`
    RecordedAt   int64                          `json:"recorded_at"`
    Snapshot     *marketdata.MacroDataSnapshot  `json:"snapshot"`
    SourceStatus string                         `json:"source_status"` // complete | missing
}
```

---

## 6. Operational Notes

### 6.1 Performance

- `os.ReadDir(SnapshotDir())` 為 O(n)，n 為目錄下 dated snapshot 數量
- 80+ 檔當前規模效能無慮
- BL-MS-01 評估長期 cap（建議 ~1260 trading days / 5 年）

### 6.2 SnapshotDir File Filtering

必須過濾掉以下非 snapshot 檔案：

```
- latest.json
- previous.json
- _metadata.json
- 任何非 .json 結尾
- 任何不符合 ^\d{4}-\d{2}-\d{2}\.json$ 的檔名
```

### 6.3 Logging

不寫 access log（既有 `/api/macro/*` 端點無 access log 慣例）。error 路徑沿用 `fmt.Errorf("context: %w", err)` 模式。

---

## 7. Open Questions（待 review 解決）

| # | 問題 | 候選解 | 預設決策 |
|---|------|--------|----------|
| Q1 | `from` 與 `days` 同時存在時應報 400 還是 `days` 優先？ | (a) 報 400；(b) `days` 優先 | (a) 報 400（明確錯誤回饋） |
| Q2 | `from` 早於 SnapshotDir 最早日期時，是否回傳空 `snapshots` + 所有日期進 `missing_dates`？ | (a) 是；(b) 報錯 | (a) 是（per CF-MS-01 honest empty） |
| Q3 | Capacity 超限時回傳「最近 N 筆」是否包含今天？ | (a) 包含；(b) 排除今天（避免 half-day snapshot） | (a) 包含（與 MCP wrapper `clamp 365` 既有行為一致） |

---

## 8. Acceptance Criteria

### 8.1 Functional

- [ ] `?days=30` 回傳最近 30 個交易日（含缺失日進 `missing_dates`）
- [ ] `?from=2026-04-21&to=2026-07-20` 回傳指定 range
- [ ] `?days=500` 回傳 365 筆 + `capacity_limit_hit: true`
- [ ] 缺失日檔案不存在時 `snapshot: null` + `source_status: "missing"`
- [ ] 缺失日檔案 JSON 損壞時行為同上（不報 500）
- [ ] 既有 `/api/macro/snapshot/history?date=2026-07-20` 仍正常回傳
- [ ] MCP wrapper `macro_get_snapshot_history` tool 改指向後能正確回傳

### 8.2 Non-Functional

- [ ] `go build ./...` 全綠
- [ ] `go test ./internal/monitoring/... ./cmd/atlas-mcp/...` 全綠
- [ ] Coverage ≥ 60%（per `docs/reference/parameter-system.md`）
- [ ] `bash scripts/ci/check_markdown_links.sh` 全綠
- [ ] `bash scripts/ci/check_atlas_mcp_docs_consistency.sh` 全綠

### 8.3 Documentation

- [ ] 本 spec §3 4 條 invariants 與 manifest §Invariant Tracker 對齊
- [ ] 本 spec §4 與 manifest §設計細節 一致
- [ ] PR body 引用 `See `.omo/manifests/2026-07-20-cl2-macro-snapshot-history.md``

---

## 9. References

- `.omo/manifests/2026-07-20-cl2-macro-snapshot-history.md`（本 spec 的 audit manifest）
- docs/specs/macro-category-spec.md（macro 域既有 spec；本 spec 為 timeline 端點專屬補充）
- docs/specs/capital-flow-seven-dimension-spec.md §14 + CF-INV-15/16/17（CL-5 鋪路參考）
- internal/monitoring/AGENTS.md（module-level 慣例）
- internal/monitoring/api/shared/paths.go:62（`ValidateDateParam` regex）
- internal/monitoring/api/macro/handlers.go:47-60（既有 `HandleMacroSnapshotHistory`）
- internal/monitoring/service/macro.go:68-82（既有 `GetSnapshotByDate`）
- cmd/atlas-mcp/server/tools_macro.go:74-90（既有 MCP wrapper）
- cmd/atlas/main.go isPublicPath（whitelist — 已涵蓋 `/api/macro`）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial spec（4 章節：API surface / Invariants / 與既有 endpoint 關係 / Service contract / Operational） | OpenCode CLI Agent (Sisyphus) |
