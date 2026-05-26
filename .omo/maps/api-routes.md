# API Routes Map
> Generated: 2026-05-26 04:37 UTC | Total routes: 119 | Stubs: 0

## Summary by Group
| Group | Count | Stubs | Active |
|-------|-------|-------|--------|
| dashboard | 42 | 0 | 42 |
| narrative | 8 | 0 | 8 |
| control | 9 | 0 | 9 |
| live | 5 | 0 | 5 |
| experiment | 5 | 0 | 5 |
| backtest | 4 | 0 | 4 |
| performance | 2 | 0 | 2 |
| system | 11 | 0 | 11 |
| other | 33 | 0 | 33 |

## Dashboard Routes (42 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| /api/dashboard/api-keys/update | anonymous | internal/monitoring/dashboard_api.go:705 | ✅ active |
| /api/dashboard/channels/ | anonymous | internal/monitoring/dashboard_api.go:661 | ✅ active |
| /api/dashboard/data-channels | anonymous | internal/monitoring/dashboard_api.go:592 | ✅ active |
| /api/dashboard/data-pipeline | anonymous | internal/monitoring/dashboard_api.go:639 | ✅ active |
| /api/dashboard/drawdown | a.handleDrawdown | internal/monitoring/dashboard_api.go:657 | ✅ active |
| GET /api/dashboard/agent-observatory | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:29 | ✅ active |
| GET /api/dashboard/baseline-info | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:38 | ✅ active |
| GET /api/dashboard/benchmark-comparison | *ast.CallExpr | internal/monitoring/api/live/handlers.go:103 | ✅ active |
| GET /api/dashboard/capital-phase | *ast.CallExpr | internal/monitoring/api/system/handlers.go:37 | ✅ active |
| GET /api/dashboard/circuit-breaker | *ast.CallExpr | internal/monitoring/api/circuitbreaker/handlers.go:16 | ✅ active |
| GET /api/dashboard/clamping-events | *ast.CallExpr | internal/monitoring/api/system/handlers.go:35 | ✅ active |
| GET /api/dashboard/conviction-clamping-events | *ast.CallExpr | internal/monitoring/api/system/handlers.go:36 | ✅ active |
| GET /api/dashboard/correlation-matrix | *ast.CallExpr | internal/monitoring/api/risk/handlers.go:38 | ✅ active |
| GET /api/dashboard/daily-summary | *ast.CallExpr | internal/monitoring/api/report/handlers.go:21 | ✅ active |
| GET /api/dashboard/data-quality | *ast.CallExpr | internal/monitoring/api/metrics/handlers.go:33 | ✅ active |
| GET /api/dashboard/experiment-inbox | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:61 | ✅ active |
| GET /api/dashboard/forecast-vs-reality | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:30 | ✅ active |
| GET /api/dashboard/industry-classification | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:17 | ✅ active |
| GET /api/dashboard/industry-cycle | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:20 | ✅ active |
| GET /api/dashboard/industry-detail | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:26 | ✅ active |
| GET /api/dashboard/industry-graph | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:25 | ✅ active |
| GET /api/dashboard/industry-linkage | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:21 | ✅ active |
| GET /api/dashboard/industry-overview | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:23 | ✅ active |
| GET /api/dashboard/industry-risk | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:22 | ✅ active |
| GET /api/dashboard/industry-seasonality | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:18 | ✅ active |
| GET /api/dashboard/industry-seasonality-calendar | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:19 | ✅ active |
| GET /api/dashboard/macro-radar | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:28 | ✅ active |
| GET /api/dashboard/metrics | *ast.CallExpr | internal/monitoring/api/metrics/handlers.go:31 | ✅ active |
| GET /api/dashboard/metrics/trend | *ast.CallExpr | internal/monitoring/api/metrics/handlers.go:32 | ✅ active |
| GET /api/dashboard/phase3-status | *ast.CallExpr | internal/monitoring/api/system/handlers.go:33 | ✅ active |
| GET /api/dashboard/portfolio-state | *ast.CallExpr | internal/monitoring/api/live/handlers.go:101 | ✅ active |
| GET /api/dashboard/reasoning-trace | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:34 | ✅ active |
| GET /api/dashboard/recommendation-pipeline | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:31 | ✅ active |
| GET /api/dashboard/regime-history | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:37 | ✅ active |
| GET /api/dashboard/retail-sentiment | *ast.CallExpr | internal/monitoring/api/system/handlers.go:38 | ✅ active |
| GET /api/dashboard/sessions | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:32 | ✅ active |
| GET /api/dashboard/system-health | *ast.CallExpr | internal/monitoring/api/system/handlers.go:34 | ✅ active |
| GET /api/dashboard/tax-snapshot | *ast.CallExpr | internal/monitoring/api/tax/handlers.go:112 | ✅ active |
| GET /api/dashboard/trade-history | *ast.CallExpr | internal/monitoring/api/live/handlers.go:102 | ✅ active |
| GET /api/dashboard/universe-overlap | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:33 | ✅ active |
| POST /api/dashboard/circuit-breaker/reset | *ast.CallExpr | internal/monitoring/api/circuitbreaker/handlers.go:17 | ✅ active |
| POST /api/dashboard/industry-shock-simulation | *ast.CallExpr | internal/monitoring/api/industry/handlers.go:24 | ✅ active |

## Narrative Routes (8 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| GET /api/narrative/chains | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:21 | ✅ active |
| GET /api/narrative/events | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:20 | ✅ active |
| GET /api/narrative/models | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:22 | ✅ active |
| GET /api/narrative/seasonal | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:24 | ✅ active |
| GET /api/narrative/stress-index/current | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:25 | ✅ active |
| GET /api/narrative/stress-index/history | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:26 | ✅ active |
| GET /api/narrative/stress-index/thresholds | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:27 | ✅ active |
| GET /api/narrative/templates | *ast.CallExpr | internal/monitoring/api/narrative/handlers.go:23 | ✅ active |

## Control Routes (9 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| GET /api/agents/health | *ast.CallExpr | internal/monitoring/api/control/handlers.go:25 | ✅ active |
| GET /api/control/active-overrides | *ast.CallExpr | internal/monitoring/api/control/handlers.go:24 | ✅ active |
| GET /api/control/audit-log | *ast.CallExpr | internal/monitoring/api/control/handlers.go:23 | ✅ active |
| POST /api/control/approve-recommendation | *ast.CallExpr | internal/monitoring/api/control/handlers.go:21 | ✅ active |
| POST /api/control/pause-agent | *ast.CallExpr | internal/monitoring/api/control/handlers.go:17 | ✅ active |
| POST /api/control/reject-recommendation | *ast.CallExpr | internal/monitoring/api/control/handlers.go:22 | ✅ active |
| POST /api/control/resume-agent | *ast.CallExpr | internal/monitoring/api/control/handlers.go:18 | ✅ active |
| POST /api/control/sector-ban | *ast.CallExpr | internal/monitoring/api/control/handlers.go:20 | ✅ active |
| POST /api/control/set-model-weight | *ast.CallExpr | internal/monitoring/api/control/handlers.go:19 | ✅ active |

## Live Routes (5 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| /api/dashboard/risk-calibration | a.handleRiskCalibration | internal/monitoring/dashboard_api.go:656 | ✅ active |
| GET /api/dashboard/live-status | *ast.CallExpr | internal/monitoring/api/live/handlers.go:100 | ✅ active |
| GET /api/dashboard/pnl-attribution | *ast.CallExpr | internal/monitoring/api/live/handlers.go:98 | ✅ active |
| GET /api/dashboard/risk | *ast.CallExpr | internal/monitoring/api/risk/handlers.go:37 | ✅ active |
| GET /api/dashboard/risk-exposure | *ast.CallExpr | internal/monitoring/api/live/handlers.go:99 | ✅ active |

## Experiment Routes (5 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| GET /api/experiment/diff | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:60 | ✅ active |
| GET /api/experiment/history | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:62 | ✅ active |
| POST /api/experiment/judge | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:59 | ✅ active |
| POST /api/experiment/promote | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:57 | ✅ active |
| POST /api/experiment/revert | *ast.CallExpr | internal/monitoring/api/experiment/handlers.go:58 | ✅ active |

## Backtest Routes (4 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| GET /api/backtest/signals | *ast.CallExpr | internal/monitoring/api/backtest/handlers.go:27 | ✅ active |
| GET /api/backtest/snapshots | *ast.CallExpr | internal/monitoring/api/backtest/handlers.go:26 | ✅ active |
| GET /api/backtest/status | *ast.CallExpr | internal/monitoring/api/backtest/handlers.go:25 | ✅ active |
| POST /api/backtest/run | *ast.CallExpr | internal/monitoring/api/backtest/handlers.go:24 | ✅ active |

## Performance Routes (2 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| GET /api/dashboard/performance-report | *ast.CallExpr | internal/monitoring/api/performance/handlers.go:23 | ✅ active |
| GET /api/dashboard/performance-report/export | *ast.CallExpr | internal/monitoring/api/performance/handlers.go:24 | ✅ active |

## System Routes (11 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| /api/health/data-integrity | *ast.CallExpr | internal/monitoring/dashboard_api.go:571 | ✅ active |
| GET /api/scheduler/status | *ast.CallExpr | internal/monitoring/api/scheduler/handlers.go:58 | ✅ active |
| GET /api/tasks | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:26 | ✅ active |
| GET /api/tasks/{id} | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:27 | ✅ active |
| GET /api/tasks/{id}/events | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:31 | ✅ active |
| GET /health | *ast.CallExpr | internal/monitoring/api/health/handlers.go:12 | ✅ active |
| POST /api/scheduler/toggle | *ast.CallExpr | internal/monitoring/api/scheduler/handlers.go:59 | ✅ active |
| POST /api/tasks | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:25 | ✅ active |
| POST /api/tasks/{id}/cancel | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:28 | ✅ active |
| POST /api/tasks/{id}/confirm | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:30 | ✅ active |
| POST /api/tasks/{id}/retry | *ast.CallExpr | internal/monitoring/api/taskexec/handlers.go:29 | ✅ active |

## Other Routes (33 routes)

| Pattern | Handler | File:Line | Status |
|---------|---------|-----------|--------|
| / | *ast.CallExpr | cmd/atlas/main.go:496 | ✅ active |
| /admin/reload-config | *ast.CallExpr | cmd/atlas/main.go:371 | ✅ active |
| /admin/trigger-simulation | *ast.CallExpr | cmd/atlas/main.go:404 | ✅ active |
| /api/admin/calibrate-thresholds | *ast.CallExpr | cmd/atlas/main.go:385 | ✅ active |
| /api/alerts | a.handleListAlerts | internal/monitoring/alert_api.go:21 | ✅ active |
| /api/alerts/unacknowledged | a.handleUnacknowledged | internal/monitoring/alert_api.go:22 | ✅ active |
| /api/events/stream | sseHandler.ServeHTTP | internal/monitoring/dashboard_api.go:458 | ✅ active |
| /api/traces/sim-latest | a.handleSimLatest | internal/monitoring/dashboard_api.go:658 | ✅ active |
| /metrics | *ast.CallExpr | cmd/atlas/main.go:472 | ✅ active |
| /static/ | *ast.CallExpr | cmd/atlas/main.go:502 | ✅ active |
| GET /api/docs | *ast.CallExpr | internal/monitoring/api/swagger/handlers.go:20 | ✅ active |
| GET /api/docs/swagger.json | *ast.CallExpr | internal/monitoring/api/swagger/handlers.go:21 | ✅ active |
| GET /api/macro/capital-flow/latest | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:19 | ✅ active |
| GET /api/macro/snapshot/history | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:22 | ✅ active |
| GET /api/macro/snapshot/latest | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:21 | ✅ active |
| GET /api/metrics/storage | *ast.CallExpr | internal/monitoring/api/metrics/handlers.go:35 | ✅ active |
| GET /api/parameters | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:30 | ✅ active |
| GET /api/parameters/audit-log | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:36 | ✅ active |
| GET /api/parameters/categories | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:32 | ✅ active |
| GET /api/parameters/snapshots | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:35 | ✅ active |
| GET /api/report/latest | *ast.CallExpr | internal/monitoring/api/report/handlers.go:19 | ✅ active |
| GET /api/report/list | *ast.CallExpr | internal/monitoring/api/report/handlers.go:20 | ✅ active |
| GET /api/synergy/darwinian-status | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:35 | ✅ active |
| GET /api/synergy/darwinian-trend | *ast.CallExpr | internal/monitoring/api/pipeline/handlers.go:36 | ✅ active |
| GET /api/taiwan/stress-index | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:20 | ✅ active |
| POST /api/alerts/acknowledge | *ast.CallExpr | internal/monitoring/alert_api.go:23 | ✅ active |
| POST /api/channels/ingest | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:18 | ✅ active |
| POST /api/macro/ingest | *ast.CallExpr | internal/monitoring/api/macro/handlers.go:17 | ✅ active |
| POST /api/parameters | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:31 | ✅ active |
| POST /api/parameters/infer-garch | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:33 | ✅ active |
| POST /api/parameters/reload | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:38 | ✅ active |
| POST /api/parameters/rollback | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:37 | ✅ active |
| POST /api/parameters/sweep | *ast.CallExpr | internal/monitoring/api/parameters/handlers.go:34 | ✅ active |

