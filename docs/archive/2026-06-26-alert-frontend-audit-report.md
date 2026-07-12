# 系統警報前端審計報告 — PR #460 前後端一致性分析

> **審計日期**：2026-06-10
> **審計範圍**：`internal/monitoring/`（後端） vs `web/static/js/pages/alerts.js` + `web/dist/`（前端）
> **審計視角**：金融工程風控閉環 + 系統運維需求者

---

## 一、執行摘要

PR #460 完成了後端 Alert Lifecycle 重構（dedup、auto-handler、regime suppression、PG columns、cleanup utility），但**前端頁面完全沒有配合更新**。這導致風控閉環在 Resolution 環節斷裂，且前端用戶對 Deduplication 與 Suppression 完全不可見。

**嚴重等級**：🔴 **高風險** — 風控人員無法正確評估警報狀態，可能導致系統性風險被低估。

---

## 二、後端能力盤點（PR #460 已交付）

### 2.1 Alert Lifecycle 狀態機

```
                    ┌─────────────┐
        ┌──────────→│  triggered  │←────────────────┐
        │           │  (觸發中)    │                 │
        │           └──────┬──────┘                 │
        │                  │ dedup (count++)        │
        │           ┌──────▼──────┐                 │
        │           │ acknowledged │                │
        │           │  (已確認)    │                │
        │           └──────┬──────┘                │
        │                  │ resolve                │
        │           ┌──────▼──────┐                │
        └───────────│  resolved   │────────────────┘
        (re-trigger)│  (已解決)    │
                    └─────────────┘
                           ▲
                    ┌──────┴──────┐
                    │   silenced  │
                    │  (已抑制)    │
                    │ regime/auto │
                    └─────────────┘
```

| 狀態 | 後端支援 | 前端顯示 |
|------|---------|---------|
| `triggered` | ✅ `monitor.go:138` 寫入 | ❌ 不顯示，只用 acknowledged bool |
| `acknowledged` | ✅ `alert_store.go:80` | ⚠️ 用 badge 顯示「已確認」 |
| `resolved` | ✅ 有 `ResolvedAt`, `ResolvedBy` 欄位 | ❌ **無 API、無 UI** |
| `silenced` | ✅ `monitor.go:124` regime suppression | ❌ **完全不可見** |

### 2.2 Deduplication 機制

- **實現**：`internal/monitoring/dedup.go`
- **窗口**：5 分鐘（`cmd/atlas/main.go:552`）
- **行為**：相同 `dedupKey = category + ":" + level` 在 5 分鐘內不新建紀錄，而是更新現有紀錄的 `count++` 與 `last_seen`
- **前端現狀**：❌ 完全不顯示 `count`, `first_seen`, `last_seen`, `dedup_key`

### 2.3 Auto-handler 機制

- **實現**：`internal/monitoring/autohandler.go`
- **行為**：
  - INFO 級別警報自動 acknowledge（`auto-handler` 標記）
  - 符合 suppression rules 的警報被靜默丟棄
- **前端現狀**：❌ 無法區分「人工確認」與「自動確認」

### 2.4 Regime Suppression

- **實現**：`monitor.go:124-129`
- **行為**：當 `currentRegime == RiskOff` 時，INFO/WARNING 級別警報不保存、不通知
- **前端現狀**：❌ 完全看不到被 regime 抑制的警報，也無法知道當前 regime 狀態

### 2.5 持久化層

| 存儲 | 狀態 |
|------|------|
| JSONL (`AlertStore`) | ✅ 保留向後相容 |
| PostgreSQL (`PostgresRepository`) | ✅ 完整 18 列，含 migration |
| DualWrite | ✅ `internal/repository/dual_write.go` |

---

## 三、後端 API 缺口（PR #460 遺漏）

### 3.1 沒有 Resolve Endpoint

```go
// alert_api.go 現有端點
GET  /api/alerts              ← 回傳全部
GET  /api/alerts/unacknowledged ← 只回未確認
POST /api/alerts/acknowledge  ← 確認

// ❌ 缺少
POST /api/alerts/resolve      ← 標記為已解決
GET  /api/alerts?status=...   ← 按狀態過濾
```

### 3.2 AlertStore 缺少 Resolve 方法

`alert_store.go` 現有方法：
- `Save`, `LoadAll`, `LoadUnacknowledged`, `Acknowledge`, `FindByDedupKey`, `Update`, `DeleteWhere`

❌ 缺少：`Resolve(alertID, user string) error`

> 註：雖然可用 `Update(id, fn)` 繞過，但沒有專屬方法違反領域語言一致性。

### 3.3 沒有 Alert 統計 API

前端無法顯示「當前 X 個觸發中、Y 個已抑制、Z 個已解決」的摘要。

---

## 四、前端現狀盤點

### 4.1 前端檔案清單

| 檔案 | 最後修改 | 內容 |
|------|---------|------|
| `web/static/js/pages/alerts.js` | PR #458 (`14f0bb68`) | 舊版，只顯示 6 個欄位 |
| `web/static/js/shared/field_types.ts` | 某個 commit（已有新欄位） | TypeScript 介面完整 |
| `web/static/index.html` | 多頁面共用 | 警報頁面結構簡陋 |
| `web/dist/chunks/alerts-KZD4JTEQ.js` | `bd36545f` 之前 | **過期編譯產物** |
| `web/static/js/main.js` | 近期 | 動態載入 alerts.js，呼叫 `renderAlerts` |

### 4.2 前端顯示欄位 vs 後端實際欄位

| 欄位 | 後端 `AlertRecord` | 前端 `alerts.js` | `field_types.ts` |
|------|-------------------|-----------------|-----------------|
| `id` | ✅ | ✅ | ✅ |
| `timestamp` | ✅ | ✅ | ✅ |
| `rule` | ✅ | ✅ | ✅ |
| `severity` | ✅ | ✅ | ✅ |
| `message` | ✅ | ✅ | ✅ |
| `value` | ✅ | ✅ | ✅ |
| `threshold` | ✅ | ❌ | ✅ |
| `breakdown` | ✅ | ❌ | ✅ |
| `acknowledged` | ✅ | ✅ | ✅ |
| `acknowledged_at` | ✅ | ❌ | ✅ |
| `acknowledged_by` | ✅ | ❌ | ✅ |
| **`status`** | ✅ **(新增)** | ❌ | ✅ |
| **`dedup_key`** | ✅ **(新增)** | ❌ | ✅ |
| **`first_seen`** | ✅ **(新增)** | ❌ | ✅ |
| **`last_seen`** | ✅ **(新增)** | ❌ | ✅ |
| **`count`** | ✅ **(新增)** | ❌ | ✅ |
| **`resolved_at`** | ✅ **(新增)** | ❌ | ✅ |
| **`resolved_by`** | ✅ **(新增)** | ❌ | ✅ |
| **`silenced_until`** | ✅ **(新增)** | ❌ | ✅ |

**結論**：前端只利用了 7/19 個欄位，**利用率僅 37%**。

### 4.3 前端 UI 結構問題

```html
<!-- index.html 警報頁面 -->
<div class="page" id="page-alerts">
  <div class="panel wide">
    <div class="flex-between mb-sm">
      <h2>系統警報</h2>
      <div class="control-group">
        <button data-action="load-alerts">🔄 重新整理</button>
        <button data-action="show-unacknowledged">僅顯示未確認</button>
      </div>
    </div>
    <div id="alertsPanel" class="empty loading">載入中…</div>
  </div>
</div>
```

**缺失的控制項**：
- ❌ 狀態過濾器（全部 / 觸發中 / 已確認 / 已解決 / 已抑制）
- ❌ 時間範圍選擇器
- ❌ 警報統計摘要面板
- ❌ Resolve 操作按鈕
- ❌ 批量操作（批量確認 / 批量解決）
- ❌ Dedup 資訊提示（count > 1 時）

---

## 五、金融工程視角：風控閉環斷裂分析

### 5.1 Basel III 操作風險管理框架對照

| Basel III 要求 | PR #460 後端 | 前端現狀 | 風險等級 |
|---------------|-------------|---------|---------|
| **Detection** | ✅ 完整 | ⚠️ 可見但資訊不全 | 中 |
| **Classification** | ✅ severity + status | ❌ 只顯示 severity | **高** |
| **Deduplication** | ✅ 5min 窗口 | ❌ 完全不可見 | **高** |
| **Routing/Suppression** | ✅ regime + auto-handler | ❌ 完全不可見 | **高** |
| **Acknowledgment** | ✅ API + store | ✅ 有按鈕 | 低 |
| **Resolution** | ⚠️ 有欄位，**無 API** | ❌ **無 UI** | **極高** |
| **Archival/Cleanup** | ✅ CLI 工具 | ❌ 無資訊 | 中 |

### 5.2 具體風險場景

#### 場景 A：重複警報被低估

```
後端實際：
  警報「資料通道 yahoo 連線失敗」count=5, last_seen=2 分鐘前

前端顯示：
  一條普通警報「資料通道 yahoo 連線失敗」，看起來像首次觸發

風險：
  運維人員以為是偶發問題，實際上已持續 20 分鐘重複觸發。
  可能導致 live trading 時使用過期資料。
```

#### 場景 B：RiskOff 抑制下的盲區

```
後端實際：
  市場 regime = RiskOff，3 個 WARNING 警報被 regime suppression 攔截

前端顯示：
  「目前沒有警報」或只有 ERROR/CRITICAL

風險：
  事後審計時無法解釋為何當時沒有 WARNING 警報。
  風控長無法判斷 suppression 是否合理。
```

#### 場景 C：Acknowledge ≠ Resolve 的語義混淆

```
後端實際：
  警報 A 被 acknowledge（狀態 = acknowledged）
  但底層問題尚未修復（狀態 ≠ resolved）

前端顯示：
  「已確認」badge，看起來像已處理完畢

風險：
  運維人員確認後忘記跟進，問題持續存在。
  下次登入看到「已確認」以為沒事，實際警報仍可能重新觸發。
```

#### 場景 D：無法追蹤時間線

```
後端實際：
  first_seen = 09:00, last_seen = 09:15, count = 4

前端顯示：
  timestamp = 09:00（僅顯示首次時間）

風險：
  無法判斷問題是間歇性（09:00, 09:05, 09:10, 09:15）
  還是持續性（09:00 開始一直存在）。
  影響根因分析效率。
```

---

## 六、需求者視角：日常運維流程斷點

### 6.1 理想流程 vs 實際流程

```
【理想流程】                        【實際流程（PR #460 後）】
┌─────────────┐                    ┌─────────────┐
│ 1. 登入系統  │                    │ 1. 登入系統  │
│ 2. 看到警報  │                    │ 2. 看到警報  │
│    摘要面板  │                    │    （無摘要） │
│    「3觸發/2抑制/5已解決」│        │              │
└──────┬──────┘                    └──────┬──────┘
       ▼                                  ▼
┌─────────────┐                    ┌─────────────┐
│ 3. 點選過濾  │                    │ 3. 點選過濾  │
│    「僅顯示觸發中」│               │    「僅顯示未確認」│
│    （可逐層過濾）│                 │    （只有一種） │
└──────┬──────┘                    └──────┬──────┘
       ▼                                  ▼
┌─────────────┐                    ┌─────────────┐
│ 4. 看到 count=5 │                │ 4. 看到一條警報 │
│    「首次 09:00 │                │    （無 dedup 資訊）│
│     最後 09:15」│                │              │
└──────┬──────┘                    └──────┬──────┘
       ▼                                  ▼
┌─────────────┐                    ┌─────────────┐
│ 5. 調查根因  │                    │ 5. 調查根因  │
│    確認問題  │                    │    確認問題  │
└──────┬──────┘                    └──────┬──────┘
       ▼                                  ▼
┌─────────────┐                    ┌─────────────┐
│ 6. 點「解決」│                    │ 6. 點「確認」│
│    狀態→resolved│                 │    狀態→acknowledged│
│    （語義清晰）│                  │    （語義模糊）│
└──────┬──────┘                    └──────┬──────┘
       ▼                                  ▼
┌─────────────┐                    ┌─────────────┐
│ 7. 系統自動  │                    │ 7. ???      │
│    歸檔/清理  │                    │    手動 CLI  │
│    （可見策略）│                  │    （不可見） │
└─────────────┘                    └─────────────┘
```

### 6.2 用戶痛點總結

| 角色 | 痛點 |
|------|------|
| **風控長** | 無法判斷當前系統風險狀態，看不到被抑制的警報 |
| **運維工程師** | 無法區分「已看過」和「已修復」，容易遺漏跟進 |
| **量化研究員** | 看不到警報重複模式，無法進行系統性問題分析 |
| **審計人員** | 無法追溯警報處置歷史（acknowledge vs resolve 混淆） |

---

## 七、修復方案（建議分兩階段）

### Phase 1：後端 API 補齊（必須先完成）

#### 7.1.1 新增 AlertStore.Resolve()

```go
// internal/monitoring/alert_store.go
func (s *AlertStore) Resolve(alertID string, user string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    all, err := s.loadFromFile()
    if err != nil {
        return fmt.Errorf("load alerts: %w", err)
    }

    now := time.Now()
    found := false
    for i := range all {
        if all[i].ID == alertID {
            all[i].Status = domain.AlertStatusResolved
            all[i].ResolvedAt = &now
            all[i].ResolvedBy = user
            found = true
            break
        }
    }
    if !found {
        return fmt.Errorf("alert %q not found", alertID)
    }
    return s.rewriteAll(all)
}
```

#### 7.1.2 新增 POST /api/alerts/resolve

```go
// internal/monitoring/alert_api.go
mux.Handle("POST /api/alerts/resolve", shared.Post(a.handleResolve))

func (a *AlertAPI) handleResolve(r *http.Request) (int, any) {
    var req struct {
        AlertID string `json:"alert_id"`
        User    string `json:"user"`
    }
    // ... 驗證 ...
    if err := a.store.Resolve(req.AlertID, req.User); err != nil {
        return http.StatusNotFound, map[string]string{"error": ...}
    }
    return http.StatusOK, map[string]any{"success": true, "alert_id": req.AlertID}
}
```

#### 7.1.3 新增按狀態過濾（可選但強烈建議）

```go
// GET /api/alerts?status=triggered
// GET /api/alerts?status=resolved
// GET /api/alerts?status=silenced
```

#### 7.1.4 PostgreSQL 層同步

```go
// internal/repository/postgres_alerts.go
func (r *PostgresRepository) ResolveAlert(ctx context.Context, alertID string, user string) error
```

### Phase 2：前端更新

#### 7.2.1 重構 `alerts.js`

**新增顯示欄位**：
- `status` → 狀態 badge（觸發中🔴 / 已確認🟡 / 已解決🟢 / 已抑制⚫）
- `count` → 重複次數（count > 1 時高亮顯示）
- `first_seen` / `last_seen` → 時間線提示
- `acknowledged_by` → 區分「人工確認」vs「auto-handler」

**新增操作**：
- 「解決」按鈕（僅對 acknowledged 顯示）
- 「重新觸發」按鈕（可選，用於測試）

**新增過濾器**：
```html
<button data-filter="all">全部</button>
<button data-filter="triggered">🔴 觸發中</button>
<button data-filter="acknowledged">🟡 已確認</button>
<button data-filter="resolved">🟢 已解決</button>
<button data-filter="silenced">⚫ 已抑制</button>
```

#### 7.2.2 更新 `index.html`

```html
<div id="page-alerts">
  <!-- 新增：統計摘要 -->
  <div class="alert-summary" id="alertSummary">
    <span class="badge err">觸發中: <strong id="countTriggered">-</strong></span>
    <span class="badge warn">已確認: <strong id="countAcknowledged">-</strong></span>
    <span class="badge ok">已解決: <strong id="countResolved">-</strong></span>
    <span class="badge muted">已抑制: <strong id="countSilenced">-</strong></span>
  </div>

  <!-- 新增：過濾器 -->
  <div class="alert-filters" id="alertFilters">
    <!-- 按鈕群 -->
  </div>

  <!-- 原有面板 -->
  <div id="alertsPanel">...</div>
</div>
```

#### 7.2.3 更新 `event-listeners.js`

綁定新的過濾器按鈕與 resolve 操作。

#### 7.2.4 重新建置

```bash
cd web && npm run build
```

### Phase 3：測試驗證

1. **單元測試**：`alert_api_test.go` 補上 resolve endpoint 測試
2. **整合測試**：前後端欄位對齊測試（確保 19/19 欄位都能正確序列化/反序列化）
3. **煙霧測試**：瀏覽器手動測試 alert lifecycle 完整流程

---

## 八、修復後的風控閉環評估

| 環節 | 修復前 | 修復後 |
|------|-------|-------|
| Detection | ✅ | ✅ |
| Classification | ⚠️ | ✅ |
| Deduplication | ❌ | ✅ |
| Suppression | ❌ | ✅ |
| Acknowledgment | ✅ | ✅ |
| **Resolution** | ❌ | ✅ |
| Archival/Cleanup | ⚠️ | ✅ |

**預期收益**：
- 運維人員可正確評估警報嚴重性（count + 時間線）
- 風控長可審視被抑制的警報（regime transparency）
- 審計軌跡完整（acknowledge → resolve 兩階段）
- 系統性問題可被識別（重複模式可視化）

---

## 九、附錄：相關程式碼索引

| 檔案 | 用途 |
|------|------|
| `internal/monitoring/monitor.go` | 警報觸發、regime suppression、dedup 呼叫 |
| `internal/monitoring/dedup.go` | 去重邏輯（5min 窗口） |
| `internal/monitoring/autohandler.go` | 自動處理 INFO + suppression rules |
| `internal/monitoring/alert_store.go` | JSONL 持久化 |
| `internal/monitoring/alert_api.go` | HTTP API（缺 resolve） |
| `internal/repository/postgres_alerts.go` | PostgreSQL 持久化 |
| `internal/domain/types.go:138` | AlertRecord struct 定義 |
| `web/static/js/pages/alerts.js` | 前端渲染（舊版） |
| `web/static/js/shared/field_types.ts` | TypeScript 介面（已有新欄位） |
| `cmd/cleanup-channel-health/main.go` | 清理 CLI |
| `cmd/atlas/main.go:552` | dedup 5min 窗口設定 |
