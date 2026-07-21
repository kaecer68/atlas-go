---
name: atlas-monitoring-observability
description: "Use when modifying or troubleshooting the Wave 9 observability system (5 detectors), Prometheus metric emission (PR-926 naming convention), /api/llm/health probe path, or wave9 alert rules. Triggers: Wave9Observability, wave9_runtime, atlas_db_init_failures_total, atlas_channel_health_errors_total, /api/llm/health 401, wave9_channel_individual_health, IsAuthFreePath, isPublicPath, MetricDBInitFailures, MetricChannelHealthErrors"
---

## 問題背景

atlas-go 啟動流程曾有兩個系統性盲點:

1. **Metrics 端點空 body**:`/metrics` 回 200 但無內容,因業務邏輯端從未 increment counter,Prometheus scrape 0 個 metric,alert rule 引用「不存在的 metric」永遠不觸發(dead code,Issue #927 經典案例)
2. **5 個 detector 啟動失敗難追溯**:Wave 9 的 RegimeDebouncer / IngestionLagMonitor / ChannelHealthSynthesizer / FactorWeightRegressionDetector / DriftDetector 啟動失敗時,oncall 不知哪個出問題

本 skill 整合 PR #926 (metric emission)、PR #928 (Issue #927 修正)、PR #929 (Loki 設計)、PR #930 (RSS 替換)、PR #931 (auth 修復) 的決策,提供一站式操作指南。

---

## 索引地圖

### 設計文件(spec)

- `docs/specs/wave9-observability-spec.md` — **5 偵測器架構 + PR-926 metric 索引 + PR-931 auth 規則**(本 skill 對應的設計規格)
- `docs/specs/llm-routing-spec.md` — LLM router 設計(`/api/llm/health` 由此暴露)
- `docs/operations/loki-deployment.md` — 集中式 log 部署(4 條 LogQL rules 對應 5 偵測器失敗模式)

### 操作手冊(runbook)

- `docs/operations/wave9-runbook.md` — **啟動檢查清單 + 4 種 troubleshoot 場景 + 緊急處置**
- `internal/monitoring/AGENTS.md` — 模組 hot-path 規則(78 行)

### 決策記錄(brief)

- `docs/operations/rss-feed-replacement.md` — PR-930 RSS feed 替換決策
- `.omo/briefs/alert-redesign-v2.md` — Wave 9 alert rule 設計與 P0-P2 backlog（v2 active draft,參照 `.omo/briefs/ALERT_SYSTEM_REdesign.md` v1）

### 規範文件(normative)

- `docs/reference/traps.md` — **必讀**:`/api/llm/health` 401 防回歸 + Prometheus metric 命名空間
- `AGENTS.md`(root) — 高頻陷阱速查表(LLM health 401 + Metric 命名空間 兩條)
- `internal/apigateway/CONSTITUTION.md` — 6 條數據源治理憲法

### 模組程式碼

- `internal/monitoring/wave9_runtime.go` — 5 偵測器 coordinator(333 行)
- `internal/monitoring/startup_metrics.go` — PR-926 metric helper
- `internal/monitoring/service/drift_detector.go` — DriftDetector v1/v2
- `internal/monitoring/api/shared/handler.go` — `authFreeExactPaths` 定義處
- `cmd/atlas/main.go` — `isPublicPath` 定義處(PR-931 同步更新)

### 對應 PR(時序)

| PR | commit | 內容 |
|----|--------|------|
| #926 | `9d9a1502` | 落地 `atlas_db_init_failures_total` + `atlas_channel_health_errors_total` |
| #928 | `552b0822` | 修正 `wave9_channel_individual_health.yml` 引用 dead metric(Issue #927) |
| #929 | `ce744cd6` | Loki 部署 spec(3 階段) |
| #930 | `9c2ebb24` | RSS feed 替換為 4 個財經主流源 |
| #931 | `82e26982` | `/api/llm/health` 401 修復(雙路徑 auth-free sync) |

---

## 核心架構

### 5 偵測器啟動順序

```
RegimeDebouncer (1st)
  ↓
{ IngestionLagMonitor, ChannelHealthSynthesizer, FactorWeightRegression } (並行 2nd)
  ↓
DriftDetector (5th, last)
```

Stop 為 LIFO:Drift → Factor → Channel → Ingestion → Regime。

任一啟動失敗 → 已啟動的偵測器 LIFO stop + 清空 reference(避免 stale bus subscription)。

### Prometheus Metric 命名規範

| 類型 | 範例 |
|------|------|
| ✅ 正確 | `atlas_db_init_failures_total`, `atlas_channel_health_errors_total` |
| ❌ 錯誤(Issue #927) | `channel_errors_total`(無前綴,被 Prometheus default metric 衝突或 dead code) |
| ❌ 錯誤 | `db_init_failures_total`(無前綴) |

**helper 必須 nil collector 安全**(bootstrap 早期 collector 可能未建):

```go
func RecordDBInitFailure(c *MetricsCollector) {
    if c == nil { return }
    c.RecordCounter(MetricDBInitFailures, 1, map[string]string{"phase": "startup"})
}
```

### Health Endpoint Auth Rule

`/api/llm/health` 是觀測型端點,**必須**在兩處**同步**列入 auth-free path(只改一處仍 401):

```go
// 檔案 1: internal/monitoring/api/shared/handler.go
var authFreeExactPaths = map[string]bool{
    "/health":          true,
    "/metrics":         true,
    "/admin":           true,
    "/client":          true,
    "/api/llm/health":  true,  // ← 必加
}

// 檔案 2: cmd/atlas/main.go
func isPublicPath(p string) bool {
    switch {
    case p == "/" || ...:
        return true
    case p == "/api/llm/health":  // ← 必加
        return true
    case p == "/admin" || ...:
        return true
    }
}
```

---

## 常見任務

### 新增 Prometheus Metric

1. 在 `internal/monitoring/startup_metrics.go` 加 const:`const MetricXxx = "atlas_<feature>_<measurement>_total"`
2. 寫 helper 函式(nil collector 安全)
3. 在業務邏輯端呼叫 helper
4. 在 `docs/specs/wave9-observability-spec.md` §5.1 補索引條目
5. 在 `monitoring/rules/*.yml` 加 alert rule(若需告警)
6. 跑 `docker compose build atlas && docker compose up -d atlas` 驗證

### 啟用 Wave 9 Alert Rule

`monitoring/rules/wave9_*.yml` 預設 `enabled: "false"`(PD-W9-1)。啟用步驟:

```bash
# 1. 確認 metric 在 /metrics 有輸出
curl http://localhost:18080/metrics | grep <metric_name>

# 2. 改 enabled: "false" -> "true"
sed -i 's/enabled: "false"/enabled: "true"/' monitoring/rules/wave9_<rule>.yml

# 3. 重新部署(Prometheus 會 reload rules 或重啟)
docker compose restart prometheus
```

### 修復 `/api/llm/health` 401

若重現 PR-931 修復的 401:

```bash
# 1. 確認兩處都加(只改一處仍 401)
grep '/api/llm/health' internal/monitoring/api/shared/handler.go
grep '/api/llm/health' cmd/atlas/main.go

# 2. 缺一就補
# 3. rebuild + restart
docker compose build atlas && docker compose up -d atlas

# 4. 驗證
curl -s -o /dev/null -w "code=%{http_code}\n" http://localhost:18080/api/llm/health
# 期望: code=200
```

---

## 警示訊號

| 訊號 | 可能原因 | 處置 |
|------|---------|------|
| `curl /metrics \| grep atlas_` 無輸出 | 容器跑舊 image(PR-926 未 merge) | `docker compose build atlas && up -d` |
| `atlas_channel_health_errors_total{channel="us_yahoo"}` 持續增加 | us_yahoo rate limit 或 network 不穩 | 啟用 wave9_channel_individual_health rule + 查 network |
| `/api/llm/health` 回 401 | PR-931 修正被覆蓋或缺步驟 | 見上文「修復 401」 |
| Wave 9 啟動日誌缺 `wave9_observability started` | `EventBus` 未就緒或 provider 注入 nil | 查 main.go provider 注入路徑 |
| DriftDetector 沒 `target_drift` 事件 | `TargetWeightsProvider` 未設定 | fallback v1 行為;若需 v2 須注入 provider |

---

## 反模式(絕對不要做)

- **手動 emit `channel_errors_total` 或任何無 `atlas_` 前綴 metric**:會被 Prometheus default metric 衝突,且無 alert rule 可用
- **修改 `wave9_runtime.go` 而不同步更新 spec**:Start 順序是契約,改了不更新文件會誤導
- **只改 `handler.go authFreeExactPaths` 不改 `main.go isPublicPath`(或反之)**:rebuild 後仍 401
- **把 `/api/llm/health` 移到 auth-protected**:atlas-mcp 客戶端無 API token 時會 401,Loki alert 整合會失敗
- **跳過 nil collector 檢查**:bootstrap 早期崩潰會觸發 panic,而不是「沒 metrics」這個靜默失敗

---

## 測試

```bash
# 單元測試
go test ./internal/monitoring/...

# E2E 驗證(本機 Docker)
docker compose up -d atlas
sleep 30
curl -s -o /dev/null -w "code=%{http_code}\n" http://localhost:18080/health           # 200
curl -s -o /dev/null -w "code=%{http_code}\n" http://localhost:18080/api/llm/health  # 200(PR-931 修復後)
curl -s http://localhost:18080/metrics | grep -E "^atlas_"                          # 2+ metric

# 整合測試(需實際 RSS/事件流)
go test -run TestTaiwanRSSGeopoliticalProvider_FetchScore ./internal/narrative/  # PR-930 RSS
```

---

## 相關 Skills(其他 19 個)

- `atlas-data-visibility` — 4 層 cross-market data visibility(L1-L4)
- `atlas-pre-change-protocol` — 改 code 前必跑的 overlap + GitNexus blast radius
- `atlas-fubon-supervisor-invariants` — F1-F9 ProcessManager invariants
- `atlas-factor-change-protocol` — FactorType 變更 8 位置同步
- `atlas-risk-management` — 4-tier risk architecture
- `atlas-strategy-evolution` — darwinian weight evolution

---

## 版本歷史

- **2026-07-03**:初版建立,整合 PR-926 / 928 / 929 / 930 / 931 決策
