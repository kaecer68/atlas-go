# 時序資料庫評估報告

**日期**: 2026-04-25
**狀態**: 評估建議
**觸發原因**: 監控與指標系統從記憶體計數器升級為 JSONL 持久化後，面臨查詢效能與規模化瓶頸

---

## 1. 現況分析

### 當前儲存架構

| 元件 | 儲存方式 | 檔案位置 | 特性 |
|------|----------|----------|------|
| 指標收集 (MetricsCollector) | 記憶體計數器 | — | 重啟歸零，無歷史 |
| 指標歷史 (MetricsHistory) | 記憶體 Ring Buffer | — | 最多 1000 筆，重啟遺失 |
| 警報 (AlertStore) | JSONL append-only | `data/state/alerts/alerts.jsonl` | 持久化，但無索引 |
| 實驗結果 | JSON 檔案 | `data/state/experiments/` | 持久化，手動管理 |
| Session 記錄 | JSON 檔案 | `data/state/sessions/` | 持久化，按日期分割 |
| Ledger 結果 | JSONL append-only | `data/state/ledger/` | 持久化，大檔案 |
| 產業連動歷史 | 記憶體 | — | 新增中，尚未持久化 |
| 季節性表現 | 記憶體 | — | 新增中，尚未持久化 |

### 痛點

1. **無時間範圍查詢能力**：JSONL 需全檔掃描才能過濾日期區間
2. **無聚合運算**：無法直接計算「過去 7 天平均通過率」這類查詢
3. **重啟遺失**：MetricsCollector/MetricsHistory 純記憶體，系統重啟後歸零
4. **無即時串流**：Dashboard 輪詢靠 HTTP polling，無 push 機制
5. **規模化瓶頸**：單一 JSONL 檔案超過 100MB 後讀取效能急遽下降

---

## 2. 候選方案評估

### 2.1 方案總覽

| 方案 | 類型 | 複雜度 | 成本 | 適合場景 |
|------|------|--------|------|----------|
| **繼續 JSONL + 索引** | 檔案式 | 低 | 零 | 小型專案、快速原型 |
| **SQLite + WAL** | 嵌入式 RDBMS | 低 | 零 | 中小型、單機部署 |
| **TimescaleDB** | PostgreSQL 擴充 | 中 | 中（需 PG） | 已用 PG 的團隊 |
| **InfluxDB OSS** | 專用時序庫 | 中 | 零（OSS） | 純指標監控場景 |
| **Prometheus + Grafana** | 拉取式監控 | 高 | 中（維運） | 基礎設施監控 |

---

### 2.2 詳細評估

#### A. 繼續 JSONL + 自訂索引

**架構**：維持現有 JSONL 格式，新增 `.idx` 索引檔（日期 → 檔案偏移量）

```
data/state/metrics/
├── metrics_2026-04.jsonl    # 按月分割
├── metrics_2026-04.idx      # 日期偏移索引
└── metrics_manifest.json    # 檔案清單與統計
```

| 維度 | 評分 | 說明 |
|------|------|------|
| 實作成本 | ⭐⭐⭐⭐⭐ | 純 Go，無外部依賴 |
| 查詢效能 | ⭐⭐ | 僅支援線性掃描 + 索引跳轉 |
| 聚合能力 | ⭐ | 需手動實作 SUM/AVG/COUNT |
| 維運成本 | ⭐⭐⭐⭐⭐ | 零維運 |
| 規模上限 | ⭐⭐ | 單月約 100MB 後效能下降 |
| 團隊熟悉度 | ⭐⭐⭐⭐⭐ | 已使用 JSONL |

**結論**：適合短期（3 個月內）過渡，不建議長期使用。

---

#### B. SQLite + WAL Mode

**架構**：使用 `modernc.org/sqlite`（純 Go SQLite，無需 CGO）

```sql
CREATE TABLE metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recorded_at DATETIME NOT NULL,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    tags TEXT  -- JSON 標籤
);
CREATE INDEX idx_metrics_time ON metrics(recorded_at);
CREATE INDEX idx_metrics_name_time ON metrics(metric_name, recorded_at);
```

| 維度 | 評分 | 說明 |
|------|------|------|
| 實作成本 | ⭐⭐⭐⭐ | Go 生態系成熟，但需設計 schema |
| 查詢效能 | ⭐⭐⭐⭐ | B-tree 索引，毫秒級查詢 |
| 聚合能力 | ⭐⭐⭐⭐⭐ | 完整 SQL（AVG, SUM, GROUP BY, window functions） |
| 維運成本 | ⭐⭐⭐⭐ | 單一檔案，自動 WAL |
| 規模上限 | ⭐⭐⭐⭐ | 單檔可達 TB 級（但建議按月分割） |
| 團隊熟悉度 | ⭐⭐⭐⭐ | SQL 普及 |

**結論**：**最佳 CP 值方案**。零外部依賴，完整 SQL 能力，適合 atlas-go 的單機部署模式。

---

#### C. TimescaleDB (PostgreSQL 擴充)

**架構**：atlas-go 已使用 PostgreSQL 15，可直接啟用 TimescaleDB 擴充

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;
SELECT create_hypertable('metrics', 'recorded_at');
```

| 維度 | 評分 | 說明 |
|------|------|------|
| 實作成本 | ⭐⭐⭐ | 需確認 PG 伺服器有 timescaledb 擴充 |
| 查詢效能 | ⭐⭐⭐⭐⭐ | 自動分區、壓縮、列式儲存 |
| 聚合能力 | ⭐⭐⭐⭐⭐ | 完整 SQL + 時序專屬函數 |
| 維運成本 | ⭐⭐⭐ | 需管理 PG 擴充，但已用 PG |
| 規模上限 | ⭐⭐⭐⭐⭐ | 無限（叢集可擴展） |
| 團隊熟悉度 | ⭐⭐⭐⭐ | 已用 PG，學習曲線低 |

**結論**：**長期最佳方案**。與現有 PostgreSQL 基礎設施無縫整合，但需要伺服器端安裝 timescaledb 擴充。

---

#### D. InfluxDB OSS

**架構**：獨立 InfluxDB 2.x 程序，透過 HTTP API 寫入/查詢

| 維度 | 評分 | 說明 |
|------|------|------|
| 實作成本 | ⭐⭐ | 需引入 influxdb3-go 客戶端 |
| 查詢效能 | ⭐⭐⭐⭐⭐ | 專為時序優化 |
| 聚合能力 | ⭐⭐⭐⭐ | Flux 查詢語言（需學習） |
| 維運成本 | ⭐⭐ | 額外程序、額外備份 |
| 規模上限 | ⭐⭐⭐⭐⭐ | 專為大規模設計 |
| 團隊熟悉度 | ⭐⭐ | Flux 語言學習曲線 |

**結論**：不推薦。引入過多維運負擔，且與現有 PG 基礎設施重複。

---

#### E. Prometheus + Grafana

**架構**：Prometheus 拉取 `/metrics` 端點，Grafana 儀表板視覺化

| 維度 | 評分 | 說明 |
|------|------|------|
| 實作成本 | ⭐⭐ | 需暴露 Prometheus metrics 端點 |
| 查詢效能 | ⭐⭐⭐⭐⭐ | 專為指標設計 |
| 聚合能力 | ⭐⭐⭐⭐ | PromQL 強大但需學習 |
| 維運成本 | ⭐ | 兩個額外服務 |
| 規模上限 | ⭐⭐⭐⭐⭐ | 叢集可擴展 |
| 團隊熟悉度 | ⭐⭐⭐ | Prometheus 普及但需學習 |

**結論**：適合基礎設施監控（CPU、記憶體、HTTP 延遲），但不適合業務指標（產業分析、實驗結果）。

---

## 3. 推薦策略

### 階段式遷移路徑

```
現在 (JSONL)
    ↓ 1-2 週
Phase A: SQLite + WAL（純 Go，零依賴）
    ↓ 1-2 月
Phase B: TimescaleDB（若需要叢集/壓縮/長期儲存）
```

### Phase A: SQLite 遷移（推薦立即執行）

**目標**：將 MetricsCollector、MetricsHistory、AlertStore 迁移至 SQLite

**實作步驟**：

1. 引入 `modernc.org/sqlite`（純 Go，無需 CGO）
2. 建立 `internal/monitoring/metrics_db.go`：
   ```go
   type MetricsDB struct {
       db *sql.DB
   }
   func (m *MetricsDB) Record(metric string, value float64, tags map[string]string) error
   func (m *MetricsDB) QueryRange(metric string, start, end time.Time) ([]MetricPoint, error)
   func (m *MetricsDB) Aggregate(metric string, start, end time.Time, agg string) (float64, error)
   ```
3. 修改 `DashboardAPI` 使用 `MetricsDB` 替代 `MetricsHistory`
4. 保留 JSONL 作為備份匯出格式

**預估工時**：2-3 天

### Phase B: TimescaleDB 升級（視需求決定）

**觸發條件**：
- 單一月份 metrics 資料超過 500MB
- 需要即時 dashboard（Grafana 整合）
- 需要資料壓縮（超過 30 天的資料自動壓縮）

**實作步驟**：
1. 確認 PostgreSQL 伺服器安裝 timescaledb 擴充
2. 建立 hypertable
3. 修改 `MetricsDB` 介面實作，切換後端為 PG
4. 設定資料保留策略（自動刪除超過 1 年的原始資料）

**預估工時**：3-5 天

---

## 4. 成本效益分析

| 方案 | 實作成本 | 維運成本 | 查詢效能提升 | 總分 |
|------|----------|----------|-------------|------|
| JSONL + 索引 | 0.5 天 | 0 | 20% | ⭐⭐ |
| **SQLite + WAL** | **2-3 天** | **0** | **500%** | **⭐⭐⭐⭐⭐** |
| TimescaleDB | 3-5 天 | 中 | 1000% | ⭐⭐⭐⭐ |
| InfluxDB | 3-5 天 | 高 | 1000% | ⭐⭐⭐ |
| Prometheus | 5-7 天 | 高 | 1000% | ⭐⭐⭐ |

---

## 5. 風險評估

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|----------|
| SQLite 檔案損毀 | 低 | 高 | WAL mode + 定期備份 |
| TimescaleDB 未安裝 | 中 | 中 | Phase A 先用 SQLite 過渡 |
| 遷移期間資料遺失 | 低 | 高 | 雙寫模式（JSONL + DB）過渡 1 週 |
| 查詢效能未達預期 | 低 | 中 | 預先建立 benchmark 測試 |

---

## 6. 結論

**推薦方案**：Phase A — SQLite + WAL

**理由**：
1. atlas-go 是單機部署架構，不需要分散式資料庫
2. `modernc.org/sqlite` 是純 Go 實作，無需 CGO，與現有工具鏈完全相容
3. 完整 SQL 能力，可處理所有聚合查詢需求
4. 零額外維運成本（單一檔案）
5. 可作為未來遷移至 TimescaleDB 的過渡層（介面抽象化）

**不推薦**：
- InfluxDB：維運負擔過重，與 PG 基礎設施重複
- Prometheus：適合基礎設施監控，不適合業務指標
- 繼續 JSONL：無法解決根本問題，僅是拖延
