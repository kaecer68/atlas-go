# Sprint 3 — Shadow Mode + 灰度 Rollout Runbook

> Branch: `fix/p0-sprint3-e2e-rollout`
> Triggered by: PR #997 + T13 E2E test passing
> Date: 2026-07-08
> Status: **PRE-DEPLOYMENT**

## 目標

把 Sprint 1+2 重構後的 `/api/recommendations` 從 staging 灰度到 production：
- 10% 流量 → 50% 流量 → 100% 流量
- 各階段觀察 ≥24h
- 任何錯誤率上升自動 rollback

## 前置條件

✅ Sprint 1+2 已 merge main (PR #997, 14 commits)
✅ T13 E2E test 已寫入 main (commit 0bd9dc16)
✅ 4 個 services 真實可用：narrative / capitalflow / eventdriven / strategy
✅ DataStatus enum 已就緒 (`service_unavailable` / `no_data`)

## 環境設定

### 1. Staging 環境

```bash
# 拉新 image
docker pull ghcr.io/kaecer68/atlas-go:latest

# 啟動 staging compose（已存在於 .omo/staging/）
docker compose -f .omo/staging/docker-compose.yml up -d

# 驗證 staging health
curl -fsS https://staging.atlas-go.local/api/llm/health | jq .
curl -fsS https://staging.atlas-go.local/api/recommendations -H "Authorization: Bearer $JWT_TEST" | jq .
```

### 2. Shadow mode flag

```bash
# 在 staging main.go 加 ENV flag
export ATLAS_RECOMMENDER_SHADOW_MODE=true
export ATLAS_RECOMMENDER_CANARY_PERCENT=0  # 0% 真實流量, 100% shadow 對照

# 重啟
docker compose -f .omo/staging/docker-compose.yml restart atlas
```

Shadow mode 語意：所有 `recommendations` 請求同步呼叫兩個 implementation：
- `getOldRecommendations()` (回退到 hardcoded stub)
- `getNewRecommendations()` (Sprint 2 整合版本)
回應用 `old`，但 metrics 同時記錄 `new` 結果，便於比較 diff。

## 觀察指標

### SLO (Service Level Objectives)

| 指標 | 門檻 |
|------|------|
| `/api/recommendations` p50 latency | < 200ms |
| `/api/recommendations` p99 latency | < 800ms |
| Error rate (5xx) | < 0.5% |
| `data_status=service_unavailable` 比例 | < 1% |
| `warning` field 出現率 | < 5% |

### Prometheus metrics (新增)

```yaml
- atlas_recommender_requests_total{tier, data_status}
- atlas_recommender_duration_seconds{tier}
- atlas_recommender_services_total{service, status}  # narrative/capflow/events/strategy
- atlas_recommender_warning_total{source}  # narrative/capflow/events/strategy
- atlas_recommender_regime_change_total{old, new}
```

### Log 結構

```json
{
  "ts": "2026-07-08T13:00:00Z",
  "level": "info",
  "component": "recommender",
  "tier": "premium",
  "regime": "RISK_OFF",
  "stress_index": 22.5,
  "data_status": "available",
  "warning": "",
  "services_active": ["narrative", "capitalflow", "events", "strategy"],
  "duration_ms": 145
}
```

## Rollout phases

### Phase 1 — 10% 流量 (24h observation)

```bash
export ATLAS_RECOMMENDER_CANARY_PERCENT=10
docker compose restart atlas
```

觀察 SLO 指標 24h：
- Error rate 必須 < 0.3%
- p99 必須 < 800ms
- `service_unavailable` 必須 < 0.5%

通過條件：所有 SLO 通過 + 至少 1000 次真實請求。

### Phase 2 — 50% 流量 (24h observation)

```bash
export ATLAS_RECOMMENDER_CANARY_PERCENT=50
docker compose restart atlas
```

觀察 24h 通過後進 Phase 3。

### Phase 3 — 100% 流量 (穩定)

```bash
export ATLAS_RECOMMENDER_CANARY_PERCENT=100
docker compose restart atlas
```

觀察 7 days 通過後視為完成。

## Rollback procedure

任何 phase 出現：
- Error rate > 1% sustained 5min
- p99 latency > 2s sustained 5min
- `data_status=service_unavailable` > 5%

→ 立即執行：

```bash
export ATLAS_RECOMMENDER_CANARY_PERCENT=0
docker compose restart atlas
```

回到 hardcoded stub 模式，並開 P1 incident。

## Rollback completion

回到 stub 後：
1. 確認 metrics 回歸 baseline（< 0.1% error rate, p99 < 200ms）
2. 開 incident report
3. 修失敗案例 + 新增 regression test
4. 重新 deploy (Phase 1)

## T14 Acceptance criteria

- [ ] T13 E2E test 在 staging 環境跑過
- [ ] 所有 3 phases (10% / 50% / 100%) 達成 + 通過 SLO
- [ ] 0 critical incident（during 7-day observation in 100%）
- [ ] Prometheus metrics 與 runbook 邏輯都 deploy

## Operational ownership

| 任務 | Owner |
|------|-------|
| Staging 環境維護 | DevOps |
| Shadow mode flag deploy | DevOps |
| SLO 監控 + alert | DevOps + Backend on-call |
| Rollback 決策 | Backend on-call（值班時） |
| Incident review | Backend team |
