# Wave 9 Observability — 操作手冊

> **對應規格**: `docs/specs/wave9-observability-spec.md`
> **適用對象**: Operator、On-call SRE
> **環境**: live mode(`ATLAS_ENV` ≠ `replay`)

---

## 1. 啟動檢查清單

部署後(或 atlas-go 重啟後)30 秒內,確認:

```bash
# 1. 容器 healthcheck 過
docker ps --format '{{.Names}}: {{.Status}}' | grep atlas-go
# 期望: Up <N> seconds (healthy)

# 2. Prometheus scrape 成功(2 個 PR-926 metric)
curl -s http://localhost:18080/metrics | grep -E "^atlas_(db_init|channel_health)"
# 期望: 2 個 HELP/TYPE 區塊 + 對應 counter

# 3. 5 個 detector 都 emit event(從 event log / Grafana logs 查)
docker logs atlas-go 2>&1 | grep "wave9_observability.*started"
# 期望: 看到 1 行 "started" log
docker logs atlas-go 2>&1 | grep -E "ingestion_lag|channel_health|factor_weight|drift_detected|regime_change" | tail -20
# 期望: 各 detector 都有 event emit(若市場無波動可能空,屬正常)
```

如果 1-3 任一失敗,見 §3 Troubleshoot。

---

## 2. 啟用 / 關閉 Detector

### 2.1 預設行為(全部啟用)

`Wave9Observability` 在 `cmd/atlas/main.go` live mode 自動啟動 5 個 detector。`Enabled` 與否由下列程式碼控制:

```go
// 簡化版;實際在 main.go bootstrap 路徑
if os.Getenv("ATLAS_ENV") != "replay" {
    w9, err := monitoring.NewWave9Observability(bus, opts...)
    if err != nil { /* fatal */ }
    if err := w9.Start(ctx); err != nil { /* fatal */ }
}
```

### 2.2 關閉整個 Wave 9

設定環境變數:

```bash
export ATLAS_ENV=replay   # 或 staging/dev 但不啟用 w9(目前無此 flag,需新增)
```

**注意**:此 flag 還沒實作(2026-07-03 為止),需手動 comment 掉 `Wave9Observability.Start()` 呼叫。

### 2.3 關閉單一 detector

**不可行**。`Wave9Observability` 是 all-or-nothing 設計。如需關閉單一 detector,需修改 `internal/monitoring/wave9_runtime.go` 並 fork 一個 fork-friendly config 介面(本 spec 建議 P1 改進)。

---

## 3. Troubleshoot

### 3.1 容器啟動後 `Up <N> seconds (health: starting)` 持續超過 60 秒

**症狀**:healthcheck 一直 `starting`。

**可能原因**:
1. `EventBus` 尚未就緒(bootstrap 順序問題)
2. `ChannelHealthProvider` 或 `IngestionLagProvider` 注入 nil
3. PostgreSQL / Redis 連線失敗,某些 detector 依賴 DB 查詢

**處置**:
```bash
docker logs atlas-go --tail 100 | grep -E "wave9_observability|start.*detector|startup_failed"
# 看 partial failure log
```

若看到 `errors.New("ChannelHealthProvider is required")` → 檢查 `cmd/atlas/main.go` 的 provider 注入路徑。

### 3.2 `/metrics` 沒看到 PR-926 的 2 個 metric

**症狀**:`curl /metrics | grep atlas_` 無輸出。

**可能原因**:
1. Image 太舊(PR #926 未 merge)— `docker inspect atlas-go --format '{{.Created}}'` 應為 PR #926 merge 後
2. Image 沒被 rebuild — `docker compose up -d atlas` 需先 `docker compose build atlas`
3. `bootstrap.go` 沒呼叫 `RecordDBInitFailure` 或 `RecordChannelHealthError`

**處置**:
```bash
# 確認 image 含 PR-926
docker exec atlas-go sh -c 'ls -la /app/atlas-go' | head -3
# rebuild + restart
cd /path/to/atlas-go && git pull
docker compose build atlas && docker compose up -d atlas
```

### 3.3 `atlas_channel_health_errors_total{channel="us_yahoo"}` 持續增加

**症狀**:us_yahoo 通道 counter 一直漲。

**可能原因**(由高到低機率):
1. **us_yahoo rate limit**:Yahoo Finance API 對非商業用戶限制嚴格
2. network 不穩:從 atlas container `ping` 或 `curl` yahoo 確認
3. `atlas-apigateway` 內部 circuit breaker 開啟:查 `internal/apigateway/`

**處置**:
```bash
# 1. 直接測 us_yahoo 連通
docker exec atlas-go sh -c 'curl -sS --max-time 5 https://query1.finance.yahoo.com/v7/finance/quote | head -3'
# 2. 查 circuit breaker 狀態(需登入)
curl -s -H "X-API-Key: $ATLAS_API_KEY" http://localhost:18080/api/dashboard/cross-market | jq '.data_status'
# 3. 暫時解:啟用 wave9 alert rule
#    改 monitoring/rules/wave9_channel_individual_health.yml 的 enabled: "false" -> "true"
#    重新部署
```

**已知問題**:us_yahoo 是 PR #926 後已確認的高錯誤通道(196 → 1 errors 之間);**長期需評估替代源**(見 `docs/audit/2026-...` 與 T10 us_yahoo 替代源評估)。

### 3.4 LLM health 回 401

**症狀**:`curl /api/llm/health` 回 `{"error":"unauthorized"}`。

**這是 PR #931 修正的問題**;若重現表示修正被覆蓋或缺步驟。

**處置**(見 `docs/reference/traps.md` 對應 entry):
1. 確認 `internal/monitoring/api/shared/handler.go:24-29` 有 `"/api/llm/health": true`
2. 確認 `cmd/atlas/main.go isPublicPath()` 有 `case p == "/api/llm/health":` 條目
3. **兩處都要有**,只改一處會 401
4. 修正後 `docker compose build atlas && docker compose up -d atlas`

### 3.5 DriftDetector v2 沒運作

**症狀**:`EventDriftDetected` 沒 `target_drift` 欄位。

**原因**:`TargetWeightsProvider` 沒注入。`Wave9Observability` 會自動 fallback 到 v1(無 target_drift)。

**處置**:
```bash
# 查 main.go bootstrap 路徑
grep -n "WithTargetWeightsProvider\|targetWeightsProvider" cmd/atlas/main.go
# 確認呼叫 WithTargetWeightsProvider(...) 傳入非 nil provider
```

---

## 4. 與 Loki 整合驗證(待 PR #929 落地)

PR #929 規劃 Loki 上線後:

1. 確認 Loki 4 條 LogQL rule 都從 `monitoring/rules/loki/*.yml` 載入
2. 觀察 Loki ruler 對應 alert(透過 Alertmanager → Slack `#atlas-alerts`)
3. Loki dashboard panels 5 個都顯示資料

**對應 detector 失敗 → 對應 LogQL rule 觸發**,見 spec §8。

---

## 5. 升級 / 降級 Detector

### 5.1 加入新 detector

修改 `internal/monitoring/wave9_runtime.go`:
1. 在 `Wave9Observability` struct 加新 detector field
2. 在 `detectorFactory` interface 加 newXxxDetector 方法
3. 在 `defaultDetectorFactory` 實作
4. 在 `Start()` 決定啟動順序(更新 start order 文件)
5. 在 `Stop()` 加 LIFO stop
6. 寫 unit test

### 5.2 取代現有 detector

`defaultDetectorFactory` 是可替換的(透過 `withDetectorFactory`),但**`detectorFactory` interface 簽名改變是 breaking change**。需在 PR description 標示 major version bump。

### 5.3 移除 detector

在 `Wave9Observability` struct、factory、Start/Stop 全部移除該 field,並更新 spec/runbook 對應段落。

---

## 6. 升級至新 Wave 9 版本

當 `Wave9Observability` 內部版本 bump(breaking change):

1. 確認 `cmd/atlas/main.go` 仍能編譯
2. `go test ./internal/monitoring/...` 通過
3. 部署後跑 §1 啟動檢查清單
4. 若新增 metric,加進 PR-926 索引(本 spec §5)
5. 通知 on-call:啟動順序改變可能影響 startup time SLA

---

## 7. 監控 Wave 9 自身

| 監控對象 | 查詢方式 |
|---------|---------|
| Wave 9 啟動時間 | `docker logs atlas-go 2>&1 \| grep -E "wave9_observability.*started"` 看時間戳 |
| 5 detector 持續 running | `curl /health` + 觀察 `started_at` 不變 |
| EventBus 流量 | Prometheus query:`rate(eventbus_published_total[5m])` |
| Detector goroutine count | `docker exec atlas-go sh -c 'cat /proc/$(pgrep atlas-go)/status \| grep Threads'` |

---

## 8. 緊急處置 Runbook

### 8.1 Wave 9 完全卡住

若 5 個 detector 全部 panic / hang,導致 atlas-go 不回應:

```bash
# 1. 重啟 container
docker compose restart atlas

# 2. 若是 bug 觸發,快速蒐集證據
docker logs atlas-go --since 10m > /tmp/atlas-incident.log
# 保留最近 10 分鐘 log 給事後分析

# 3. 若重啟無效,降級(只跑核心服務)
#    暫時註解掉 Wave9Observability.Start() 呼叫,部署 hotfix
```

### 8.2 資料源(us_yahoo)長期故障

詳見 §3.3。**短期**:啟用 `wave9_channel_individual_health.yml` rule,讓 on-call 收到告警。**長期**:啟動 T10 us_yahoo 替代源評估工作。

---

## 9. 相關文件

- `docs/specs/wave9-observability-spec.md` — 設計規格(姊妹文件)
- `docs/operations/loki-deployment.md` — 集中式 log
- `docs/reference/traps.md` — `/api/llm/health` 401 防回歸
- `internal/monitoring/AGENTS.md` — 模組 hot-path
- `internal/monitoring/wave9_runtime.go` — coordinator 原始碼

---

## 10. Alert Rules 啟用狀態（2026-07-05）

| Rule | Metric | Severity | 當前狀態 | 啟用條件 |
|------|--------|----------|---------|---------|
| `wave9_channel_individual_health` | `atlas_channel_health_errors_total` | warning | `enabled: false`（PD-W9-1 placeholder） | Wave 9.1 `EventChannelIndividualHealth` emit 確認後手動啟用 |
| `wave9_factor_weight_regression` | —（event-driven） | warning | `enabled: false` | `FactorWeightRegressionDetector` emit 驗證完成 |
| `wave9_drift_detected` | —（event-driven） | warning | `enabled: false` | `DriftDetector` v2 target drift 驗證完成 |
| `wave9_regime_change_confirmed` | —（event-driven） | warning | `enabled: false` | `RegimeDebouncer` confirm 邏輯驗證完成 |
| `wave9_ingestion_lag_spike` | —（event-driven） | warning | `enabled: false` | `IngestionLagMonitor` 30min lag threshold 確認有效觸發 |

### 啟用順序建議

1. **`channel_individual_health`**（優先啟動）— 此 metric 已在 PR #926/PR #948 完成 emit，對應 8 個 Yahoo 通道的錯誤計數。low-noise、可量化、false-positive 極低。
2. **`ingestion_lag_spike`** — 已有 `IngestionLagMonitor`，不依賴市場事件。確認 30min threshold 在正常運作下不會誤觸。
3. **`factor_weight_regression`** — 依賴 event-driven，需觀察 histogram baseline。
4. **`drift_detected`** — v2 target drift 需更多 replay 驗證。
5. **`regime_change_confirmed`** — regime shift 為低頻事件，驗證週期較長。

### 啟用流程（SOP）

```bash
# Step 1: 確認 metric/service emit
curl http://localhost:18080/metrics | grep atlas_channel_health_errors_total

# Step 2: Alertmanager routing 加抑制標籤（避免歷史噪音誤觸）
# 在 Alertmanager config 加:
#   match:
#     enabled: "false"
#   receiver: "null"

# Step 3: 改 alert rule enabled="true"
# 編輯 monitoring/rules/wave9_channel_individual_health.yml

# Step 4: 試跑 severity: info 1 週，觀察 false positive rate
# Step 5: 若 1 週內 false positive < 1/week，升為 severity: warning
```
