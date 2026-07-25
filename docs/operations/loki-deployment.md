# Loki 部署設計規格

> **狀態**: Draft(待 review)
> **對應任務**: T5 (Phase 1) + T6 (Phase 2) + T7 (Phase 3)
> **前置依賴**: PR #926 已 merge(commit `9d9a1502`,落地 `atlas_channel_health_errors_total` 等真實 metric)
> **重要**: 本文件**僅為設計規格**,**不實際 deploy** Loki/Promtail/Alertmanager 整合。所有 docker-compose / Promtail / Loki / Ruler 設定皆為**預覽**,需 reviewer 過目後另開實作 PR。

---

## 1. 目標與非目標

### 1.1 目標

- 集中化 atlas-go 所有服務 log 於 Grafana 可查詢
- 啟用 log-based alert: panic / 5xx spike / circuit breaker open
- 降低 MTTR(mean time to recovery)透過 log search 而非 SSH + grep
- 補齊 `.omo/briefs/roadmap-v2.md` 提到的「Production 環境需要 structured logging」需求

### 1.2 非目標

- 不取代 Prometheus metrics(現有 wave9 alert rules 維持)
- 不處理 client-side log(瀏覽器端、零售投資人前端)
- 不做長期歸檔(> 30 天);若需 > 30 天改走 S3 cold tier
- 不重構現有 log 格式(沿用 Go `log.Printf` 與 `slog` 既有格式)

---

## 2. 架構總覽

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  atlas-go   │  │fubon-proxy  │  │  cron jobs  │
│  (main svc) │  │  (Python)   │  │  (geo)      │
└──────┬──────┘  └──────┬──────┘  └──────┬──────┘
       │ stdout/stderr  │                │
       └────────┬───────┴────────────────┘
                │ (Docker log driver: json-file)
         ┌──────▼──────┐
         │  Promtail   │  (採 /var/lib/docker/containers)
         └──────┬──────┘
                │ (Loki push API, multi-tenant header)
         ┌──────▼──────┐
         │    Loki     │  (Single-binary mode, FS backend)
         └──────┬──────┘
                │ (LogQL query / Ruler eval)
       ┌────────┼────────┐
       │        │        │
   ┌───▼──┐ ┌──▼───┐ ┌──▼─────────┐
   │Grafana│ │ Ruler│ │Alertmanager│
   └──────┘ └──┬───┘ └──────┬─────┘
                │ (webhook)  │
                └──────┬────┘
                       │
              ┌────────▼────────┐
              │ Notification    │
              │ (Slack / PagerDuty)│
              └─────────────────┘
```

---

## 3. Phase 1 — Promtail + Loki + 4 LogQL rules(T5)

### 3.1 docker-compose 變更(預覽,**非實際 edit**)

```yaml
  loki:
    image: grafana/loki:2.9.0
    container_name: atlas-loki
    ports:
      - "3100:3100"
    volumes:
      - ./monitoring/loki:/etc/loki:ro
      - loki-data:/loki
    command: -config.file=/etc/loki/local-config.yaml
    networks:
      - atlas-internal
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://localhost:3100/ready || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: "0.5"

  promtail:
    image: grafana/promtail:2.9.0
    container_name: atlas-promtail
    volumes:
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./monitoring/promtail:/etc/promtail:ro
    command: -config.file=/etc/promtail/config.yaml
    networks:
      - atlas-internal
    depends_on:
      - loki
    restart: unless-stopped
```

> **Note**: 新增 `loki-data` named volume 需同步加在 compose 檔最底 `volumes:` 區塊。

### 3.2 Loki 本地 config(預覽)

```yaml
# monitoring/loki/local-config.yaml
auth_enabled: false
server:
  http_listen_port: 3100
common:
  instance_addr: 127.0.0.1
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2026-07-03
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
limits_config:
  retention_period: 168h  # 7 天(預設)
  # 開放問題:Production 應 14-30 天
  ingestion_rate_mb: 10
  ingestion_burst_size_mb: 20
ruler:
  storage:
    type: local
    local:
      directory: /loki/rules
  rule_path: /loki/rules-tmp
  alertmanager_url: http://alertmanager:9093  # Phase 2 啟用
  enable_api: true
```

### 3.3 Promtail config(預覽)

```yaml
# monitoring/promtail/config.yaml
server:
  http_listen_port: 9080
positions:
  filename: /tmp/positions.yaml
clients:
  - url: http://loki:3100/loki/api/v1/push
scrape_configs:
  - job_name: docker
    docker_sd_configs:
      - host: unix:///var/run/docker.sock
        refresh_interval: 5s
    relabel_configs:
      - source_labels: ['__meta_docker_container_name']
        regex: '/(.*)'
        target_label: 'container'
      - source_labels: ['__meta_docker_container_label_com_docker_compose_service']
        target_label: 'service'
      - source_labels: ['__meta_docker_container_log_stream']
        target_label: 'stream'
    pipeline_stages:
      - docker: {}
      - match:
          selector: '{service=~"atlas-go|fubon-proxy"}'
          stages:
            - regex:
                expression: '.*level=(?P<level>\w+).*'
            - labels:
                level:
```

### 3.4 4 個 LogQL alert rules(`monitoring/rules/loki/*.yml`)

#### Rule 1: `AtlasGoPanicDetected`(CRITICAL)

```yaml
groups:
  - name: loki_panic
    rules:
      - alert: AtlasGoPanicDetected
        expr: |
          sum by (service) (
            count_over_time({service=~"atlas-go|fubon-proxy"} |~ "panic:|fatal error" [1m])
          ) > 0
        for: 0m  # 立即觸發
        labels:
          severity: critical
          source: loki
        annotations:
          summary: "服務 panic 立即告警"
          description: |
            {{ $labels.service }} 在最近 1 分鐘內有 panic 或 fatal error log。
            立即檢查 service health。
          runbook_url: "https://wiki.internal/runbooks/loki-panic"
```

#### Rule 2: `AtlasGoHighErrorLogRate`(WARNING)

```yaml
groups:
  - name: loki_error_rate
    rules:
      - alert: AtlasGoHighErrorLogRate
        expr: |
          sum by (service) (
            rate({service=~"atlas-go|fubon-proxy"} |~ "level=(error|ERROR)" [5m])
          ) > 5
        for: 5m
        labels:
          severity: warning
          source: loki
        annotations:
          summary: "ERROR log rate 超過 5/sec"
          description: |
            {{ $labels.service }} 5m 平均 ERROR log rate: {{ $value | humanize }}/s。
            持續 5 分鐘,需調查。
```

#### Rule 3: `AtlasGoServiceUnavailableLog`(WARNING)

```yaml
groups:
  - name: loki_service_unavailable
    rules:
      - alert: AtlasGoServiceUnavailableLog
        expr: |
          sum by (service) (
            count_over_time({service=~"atlas-go|fubon-proxy"} |~ "service unavailable|circuit breaker open" [5m])
          ) > 1
        for: 5m
        labels:
          severity: warning
          source: loki
        annotations:
          summary: "服務不可用或 circuit breaker 開啟"
          description: |
            {{ $labels.service }} 5m 內出現 {{ $value }} 次 service unavailable 或 circuit breaker open。
```

#### Rule 4: `AtlasGoAPIGateway5xxSpike`(WARNING)

```yaml
groups:
  - name: loki_5xx_spike
    rules:
      - alert: AtlasGoAPIGateway5xxSpike
        expr: |
          sum(
            count_over_time(
              {service="atlas-go"} |~ "HTTP/.* 5[0-9]{2}" [5m]
            )
          ) > 10
        for: 5m
        labels:
          severity: warning
          source: loki
        annotations:
          summary: "API Gateway 5xx 爆量"
          description: |
            atlas-go 5m 內 5xx HTTP log 超過 10 次。當前值: {{ $value }}。
```

### 3.5 4 條 rules 設計依據

| Rule | 觸發條件 | 嚴重度 | 理由 |
|------|---------|--------|------|
| 1 Panic | panic/fatal 1m 內 | critical | 服務幾乎確定已 crash,需立即介入 |
| 2 High Error | ERROR > 5/sec 5m | warning | 異常累積但未崩潰,留時間 debug |
| 3 Service Unavailable | "service unavailable" > 1/5m | warning | 對應 apigateway/CONSTITUTION 的 circuit breaker 行為 |
| 4 5xx Spike | 5xx HTTP log > 10/5m | warning | 與業務 event-driven alert 互補 |

---

## 4. Phase 2 — Alertmanager 整合(T6)

### 4.1 兩種整合方式

| 方式 | 優點 | 缺點 |
|------|------|------|
| **A. Loki Ruler**(建議) | 少一個元件,直接從 Loki 評估 | Ruler config 較新,社群資源少 |
| B. Grafana Alerting | UI 直覺,與 dashboard 同環境 | 需 Grafana 10+,增加 stack 複雜度 |

**採 A**:降低 stack 複雜度,Loki 2.9 Ruler 已 GA。

### 4.2 接線

```
[Loki Ruler]  --webhook-->  [Alertmanager]  --route-->  [Notification]
              POST /alertmanager/api/v1/alerts
```

Alertmanager 已存在於 compose(`atlas-alertmanager` container,line 186),需在 `alertmanager.yml` 加入 Loki webhook receiver:

```yaml
# monitoring/alertmanager.yml(預覽,需新增 section)
receivers:
  - name: loki-critical
    slack_configs:
      - api_url: ${SLACK_WEBHOOK_URL}
        channel: "#atlas-alerts"
        title: "Loki CRITICAL: {{ .GroupLabels.alertname }}"
        text: "{{ .CommonAnnotations.summary }}"
  - name: loki-warning
    slack_configs:
      - api_url: ${SLACK_WEBHOOK_URL}
        channel: "#atlas-alerts-low"
        title: "Loki WARNING: {{ .GroupLabels.alertname }}"
        text: "{{ .CommonAnnotations.summary }}"

route:
  routes:
    - matchers:
        - source="loki"
        - severity="critical"
      receiver: loki-critical
    - matchers:
        - source="loki"
        - severity="warning"
      receiver: loki-warning
```

### 4.3 Phase 2 不做的事

- 不整合既有 wave9 alert rules(那些走 Prometheus + Alertmanager 已有 route,改 route 風險高)
- 不開 PagerDuty 整合(僅 Slack)

---

## 5. Phase 3 — Self-monitoring + SOP(T7)

### 5.1 Loki 自身 metrics 整合

Loki 自動 expose Prometheus metrics 於 `:3100/metrics`,需加入 `monitoring/prometheus.yml` scrape config:

```yaml
# 新增 scrape job
- job_name: loki
  static_configs:
    - targets: ['loki:3100']
  metrics_path: /metrics
```

關鍵 metrics:

| Metric | 用途 | 告警閾值(建議) |
|--------|------|---------------|
| `loki_request_duration_seconds_bucket` | query latency | p99 > 5s |
| `loki_ingester_streams_created_total` | stream 數 | rate > 100/s(疑似 label cardinality 爆) |
| `loki_distributor_bytes_received_total` | ingestion rate | < 預期 50% 持續 10m(漏 log) |
| `loki_ingester_memory_chunks` | memory 壓力 | > 100k(需 scale up) |

### 5.2 Grafana dashboard(預覽,非實際 JSON)

Panels:
1. **Ingestion rate**(by service): `rate(loki_distributor_bytes_received_total[5m])`
2. **Query latency p99**:`histogram_quantile(0.99, rate(loki_request_duration_seconds_bucket[5m]))`
3. **Active streams**:`loki_ingester_streams`
4. **Error log rate by service**:對應 Rule 2 視覺化
5. **Panic count last 24h**:對應 Rule 1

### 5.3 SOP(Standard Operating Procedure)

#### 場景 A:Loki 容器掛掉

1. `docker compose ps loki` — 確認狀態
2. `docker compose logs loki --tail 50` — 查 crash 原因
3. 若 OOM:`docker compose restart loki`,觀察 memory
4. 若 config 錯:rollback `monitoring/loki/local-config.yaml`

#### 場景 B:磁碟滿(Loki `/loki` volume)

1. `docker exec atlas-loki du -sh /loki/chunks` — 確認 chunks 大小
2. 短期:`loki_boltdb_shipper_active_queries` 暫停查詢
3. 中期:調整 `limits_config.retention_period`(7 → 3 天)
4. 長期:改 S3 backend + cold tier

#### 場景 C:查詢慢

1. 檢查 `loki_query_duration_seconds` 指標
2. 常見原因:high-cardinality label(`{session_id="..."}`)→ 改用 `{trace_id="..."}` 聚合
3. 加 `chunk_target_size` 調整

#### 場景 D:Alert 風暴(同 rule 觸發數十次)

1. Alertmanager 端:設 `repeat_interval: 4h` 抑制
2. Loki Ruler 端:rule `for:` 改 10m

---

## 6. 風險評估

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| 磁碟爆量 | 中 | 中 | 預設 7 天 retention + monitor disk usage |
| Label cardinality 爆 | 中 | 高 | Promtail 嚴格 label 白名單,file reviewer |
| 多 container 重複 log | 高 | 低 | 需 dedup stage(Phase 1.5 加) |
| Loki 自身 OOM | 低 | 中 | 512M limit + healthcheck |
| Alert 噪音 | 中 | 中 | 預設 `enabled: "false"`,operator 驗證後啟用 |

---

## 7. 開放問題(待 reviewer 決策)

1. **Retention 預設值**:7 天 / 14 天 / 30 天?
2. **Storage backend**:本地 FS(預設)或 S3?
3. **4 rules 閾值**:panic 1m 立即,ERROR > 5/sec 5m 是否合理?
4. **Fubon proxy log 納入**:Fubon Python log 格式與 Go 不同,要 dedup 嗎?
5. **是否整合 Grafana Alerting 替代 Loki Ruler**:見 §4.1
6. **現有 alert-redesign.md 的 P2 backlog**:Loki 是否替代任何 P2 項?

---

## 8. 測試計畫(實作時)

### Phase 1 完成時

- [ ] `docker compose up loki promtail` 啟動成功
- [ ] Promtail scrape 到 atlas-go log
- [ ] Grafana Explore 查詢 `{service="atlas-go"} |= "level=error"` 有結果
- [ ] 4 條 rules 在 Prometheus UI 可見

### Phase 2 完成時

- [ ] 手動製造 panic,確認 Slack `#atlas-alerts` 收到 critical
- [ ] 製造 5xx 大量 log,確認 warning 觸發
- [ ] Alertmanager UI 顯示 Loki 來源 alert

### Phase 3 完成時

- [ ] Grafana dashboard 5 個 panels 全部有資料
- [ ] Loki 自身 metrics 在 Prometheus 中可見
- [ ] 4 個 SOP 場景演練過(可手動觸發)

---

## 9. 實作時程(預估)

| Phase | 工作 | 預估 PR 數 | 工時(人天) |
|-------|------|----------|----------|
| Phase 1 | docker-compose + Loki config + Promtail config + 4 rules | 2-3 | 1-2 |
| Phase 2 | Alertmanager webhook + severity routing | 1 | 0.5 |
| Phase 3 | Self-monitor + dashboard JSON + SOP 文件 | 1-2 | 1 |
| **總計** | | **4-6** | **2.5-3.5** |

> 預估不含整合測試與文件 review。

---

## 10. 與現有系統的關聯

- **PR #926**:已落地 `atlas_channel_health_errors_total`,本 spec 4 條 LogQL rules **不**重疊(metric-based vs log-based,互補)
- **Alertmanager** (`atlas-alertmanager` container):已存在,Phase 2 只需擴充 receiver 與 route
- **Prometheus** (`atlas-prometheus` container):Phase 3 需加 scrape job
- **Grafana** (`atlas-grafana` container):Phase 3 需新增 dashboard
- **L2.4**(`docs/operations/l2-4-runbook.md`):觀察窗口期 Loki 不在 scope;完成後可考慮納管

---

## 11. 不在本文檔 scope

- 客戶端 log(瀏覽器 console)
- log-based 業務指標(如 regime change 次數)
- Loki cluster mode(本文 single-binary mode,適用本地 + 單機部署)
- 跨區域 log replication

---

**下一步**: 此 PR 過 review 後,Phase 1 實作另開 PR(從 main branch,branch name `feat/loki-phase-1`)。
