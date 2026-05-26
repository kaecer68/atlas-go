# Frontend-Backend Mapping
> Generated: 2026-05-26 01:44 UTC | Frontend pages: 22 | API calls: 86 | Backend routes: 119

## Frontend Pages

| Page | File | LOC | API Calls |
|------|------|-----|----------|
| alerts | web/static/js/pages/alerts.js | 51 | /api/alerts, /api/alerts/acknowledge |
| attribution | web/static/js/components/attribution.js | 39 | /api/dashboard/pnl-attribution |
| backtest | web/static/js/pages/backtest.js | 209 | /api/backtest/run, /api/backtest/status, /api/report/latest |
| benchmark | web/static/js/components/benchmark.js | 40 | /api/dashboard/benchmark-comparison |
| circuit-breaker | web/static/js/components/circuit-breaker.js | 251 | /api/dashboard/circuit-breaker, /api/dashboard/circuit-breaker/reset |
| datachannels | web/static/js/pages/datachannels.js | 267 | /api/channels/ingest, /api/dashboard/api-keys/update, /api/dashboard/channels/..., +1 more |
| event-source | web/static/js/services/event-source.js | 152 | /api/events/stream |
| evolution | web/static/js/pages/evolution.js | 466 | /api/dashboard/agent-observatory, /api/dashboard/experiment-inbox, /api/dashboard/regime-history |
| evolution-panel | web/static/js/pages/evolution_panel.js | 683 | /api/dashboard/agent-observatory, /api/dashboard/experiment-inbox, /api/dashboard/regime-history |
| experiments | web/static/js/pages/experiments.js | 260 | /api/control/active-overrides, /api/control/approve-recommendation, /api/control/audit-log, +9 more |
| industry | web/static/js/pages/industry.js | 757 | /api/dashboard/industry-classification, /api/dashboard/industry-detail, /api/dashboard/industry-overview, +3 more |
| main | web/static/js/main.js | 537 | /api/alerts, /api/dashboard/agent-observatory, /api/dashboard/capital-phase, +27 more |
| metrics | web/static/js/pages/metrics.js | 75 | /api/dashboard/capital-phase, /api/dashboard/metrics, /api/metrics/storage |
| performance-report | web/static/js/components/performance-report.js | 193 | /api/dashboard/performance-report |
| pipeline | web/static/js/pages/pipeline.js | 673 | /api/dashboard/recommendation-pipeline |
| portfolio | web/static/js/pages/portfolio.js | 202 | /api/dashboard/live-status, /api/dashboard/portfolio-state, /api/dashboard/tax-snapshot, +1 more |
| reasoning-trace | web/static/js/components/reasoning-trace.js | 153 | /api/dashboard/reasoning-trace, /api/dashboard/sessions |
| risk-gate-panel | web/static/js/components/risk-gate-panel.js | 47 | /api/dashboard/risk |
| risk-panel | web/static/js/components/risk-panel.js | 42 | /api/dashboard/correlation-matrix, /api/dashboard/portfolio-state |
| scheduler | web/static/js/pages/scheduler.js | 136 | /api/scheduler/status, /api/scheduler/toggle |
| sim-health | web/static/js/components/sim-health.js | 262 | /api/traces/sim-latest |
| task-executor | web/static/js/components/task-executor.js | 29 | /api/channels/ingest |

## API → Frontend Matrix

| API Route | Handler | Frontend Page(s) | Status |
|-----------|---------|-----------------|--------|
| GET /api/backtest/status | *ast.CallExpr | backtest | ✅ matched |
| POST /api/backtest/run | *ast.CallExpr | backtest | ✅ matched |
| GET /api/control/active-overrides | *ast.CallExpr | experiments | ✅ matched |
| GET /api/control/audit-log | *ast.CallExpr | experiments | ✅ matched |
| POST /api/control/approve-recommendation | *ast.CallExpr | experiments | ✅ matched |
| POST /api/control/pause-agent | *ast.CallExpr | experiments | ✅ matched |
| POST /api/control/reject-recommendation | *ast.CallExpr | experiments | ✅ matched |
| POST /api/control/resume-agent | *ast.CallExpr | experiments | ✅ matched |
| POST /api/control/sector-ban | *ast.CallExpr | experiments | ✅ matched |
| /api/dashboard/api-keys/update | anonymous | datachannels | ✅ matched |
| /api/dashboard/channels/ | anonymous | datachannels | ✅ matched |
| /api/dashboard/data-channels | anonymous | datachannels, main | ✅ matched |
| GET /api/dashboard/agent-observatory | *ast.CallExpr | evolution, evolution-panel, main | ✅ matched |
| GET /api/dashboard/benchmark-comparison | *ast.CallExpr | benchmark | ✅ matched |
| GET /api/dashboard/capital-phase | *ast.CallExpr | main, metrics | ✅ matched |
| GET /api/dashboard/circuit-breaker | *ast.CallExpr | circuit-breaker | ✅ matched |
| GET /api/dashboard/correlation-matrix | *ast.CallExpr | risk-panel | ✅ matched |
| GET /api/dashboard/experiment-inbox | *ast.CallExpr | evolution, evolution-panel, main | ✅ matched |
| GET /api/dashboard/industry-classification | *ast.CallExpr | industry | ✅ matched |
| GET /api/dashboard/industry-detail | *ast.CallExpr | industry | ✅ matched |
| GET /api/dashboard/industry-overview | *ast.CallExpr | industry | ✅ matched |
| GET /api/dashboard/industry-seasonality | *ast.CallExpr | industry | ✅ matched |
| GET /api/dashboard/industry-seasonality-calendar | *ast.CallExpr | industry | ✅ matched |
| GET /api/dashboard/macro-radar | *ast.CallExpr | main | ✅ matched |
| GET /api/dashboard/metrics | *ast.CallExpr | metrics | ✅ matched |
| GET /api/dashboard/metrics/trend | *ast.CallExpr | metrics | ✅ matched |
| GET /api/dashboard/phase3-status | *ast.CallExpr | main | ✅ matched |
| GET /api/dashboard/portfolio-state | *ast.CallExpr | portfolio, risk-panel | ✅ matched |
| GET /api/dashboard/reasoning-trace | *ast.CallExpr | reasoning-trace | ✅ matched |
| GET /api/dashboard/recommendation-pipeline | *ast.CallExpr | main, pipeline | ✅ matched |
| GET /api/dashboard/regime-history | *ast.CallExpr | evolution, evolution-panel, main | ✅ matched |
| GET /api/dashboard/retail-sentiment | *ast.CallExpr | main | ✅ matched |
| GET /api/dashboard/sessions | *ast.CallExpr | main, reasoning-trace | ✅ matched |
| GET /api/dashboard/system-health | *ast.CallExpr | main | ✅ matched |
| GET /api/dashboard/tax-snapshot | *ast.CallExpr | main, portfolio | ✅ matched |
| GET /api/dashboard/trade-history | *ast.CallExpr | portfolio | ✅ matched |
| GET /api/dashboard/universe-overlap | *ast.CallExpr | main | ✅ matched |
| POST /api/dashboard/circuit-breaker/reset | *ast.CallExpr | circuit-breaker | ✅ matched |
| POST /api/dashboard/industry-shock-simulation | *ast.CallExpr | industry | ✅ matched |
| GET /api/experiment/diff | *ast.CallExpr | experiments | ✅ matched |
| GET /api/experiment/history | *ast.CallExpr | experiments | ✅ matched |
| POST /api/experiment/judge | *ast.CallExpr | experiments | ✅ matched |
| POST /api/experiment/promote | *ast.CallExpr | experiments | ✅ matched |
| POST /api/experiment/revert | *ast.CallExpr | experiments | ✅ matched |
| /api/dashboard/risk-calibration | a.handleRiskCalibration | main | ✅ matched |
| GET /api/dashboard/live-status | *ast.CallExpr | main, portfolio | ✅ matched |
| GET /api/dashboard/pnl-attribution | *ast.CallExpr | attribution | ✅ matched |
| GET /api/dashboard/risk | *ast.CallExpr | risk-gate-panel | ✅ matched |
| GET /api/dashboard/risk-exposure | *ast.CallExpr | main | ✅ matched |
| GET /api/narrative/chains | *ast.CallExpr | main | ✅ matched |
| GET /api/narrative/events | *ast.CallExpr | main | ✅ matched |
| GET /api/narrative/models | *ast.CallExpr | main | ✅ matched |
| GET /api/narrative/seasonal | *ast.CallExpr | main | ✅ matched |
| GET /api/narrative/templates | *ast.CallExpr | main | ✅ matched |
| /api/alerts | a.handleListAlerts | alerts, main | ✅ matched |
| /api/alerts/unacknowledged | a.handleUnacknowledged | alerts, main | ✅ matched |
| /api/events/stream | sseHandler.ServeHTTP | event-source | ✅ matched |
| /api/traces/sim-latest | a.handleSimLatest | sim-health | ✅ matched |
| GET /api/macro/snapshot/latest | *ast.CallExpr | main | ✅ matched |
| GET /api/metrics/storage | *ast.CallExpr | metrics | ✅ matched |
| GET /api/parameters | *ast.CallExpr | main | ✅ matched |
| GET /api/parameters/audit-log | *ast.CallExpr | main | ✅ matched |
| GET /api/parameters/categories | *ast.CallExpr | main | ✅ matched |
| GET /api/parameters/snapshots | *ast.CallExpr | main | ✅ matched |
| GET /api/report/latest | *ast.CallExpr | backtest | ✅ matched |
| GET /api/synergy/darwinian-status | *ast.CallExpr | main | ✅ matched |
| GET /api/synergy/darwinian-trend | *ast.CallExpr | main | ✅ matched |
| GET /api/taiwan/stress-index | *ast.CallExpr | main | ✅ matched |
| POST /api/alerts/acknowledge | *ast.CallExpr | alerts, main | ✅ matched |
| POST /api/channels/ingest | *ast.CallExpr | datachannels, task-executor | ✅ matched |
| POST /api/parameters | *ast.CallExpr | main | ✅ matched |
| POST /api/parameters/infer-garch | *ast.CallExpr | main | ✅ matched |
| POST /api/parameters/reload | *ast.CallExpr | main | ✅ matched |
| POST /api/parameters/rollback | *ast.CallExpr | main | ✅ matched |
| POST /api/parameters/sweep | *ast.CallExpr | main | ✅ matched |
| GET /api/dashboard/performance-report | *ast.CallExpr | performance-report | ✅ matched |
| GET /api/dashboard/performance-report/export | *ast.CallExpr | performance-report | ✅ matched |
| /api/health/data-integrity | *ast.CallExpr | main | ✅ matched |
| GET /api/scheduler/status | *ast.CallExpr | scheduler | ✅ matched |
| POST /api/scheduler/toggle | *ast.CallExpr | scheduler | ✅ matched |

## Orphan APIs (no frontend consumer)

| Route | Handler | Group |
|-------|---------|-------|
| GET /api/backtest/signals | *ast.CallExpr | backtest |
| GET /api/backtest/snapshots | *ast.CallExpr | backtest |
| GET /api/agents/health | *ast.CallExpr | control |
| POST /api/control/set-model-weight | *ast.CallExpr | control |
| /api/dashboard/data-pipeline | anonymous | dashboard |
| /api/dashboard/drawdown | a.handleDrawdown | dashboard |
| GET /api/dashboard/baseline-info | *ast.CallExpr | dashboard |
| GET /api/dashboard/clamping-events | *ast.CallExpr | dashboard |
| GET /api/dashboard/conviction-clamping-events | *ast.CallExpr | dashboard |
| GET /api/dashboard/daily-summary | *ast.CallExpr | dashboard |
| GET /api/dashboard/data-quality | *ast.CallExpr | dashboard |
| GET /api/dashboard/forecast-vs-reality | *ast.CallExpr | dashboard |
| GET /api/dashboard/industry-cycle | *ast.CallExpr | dashboard |
| GET /api/dashboard/industry-graph | *ast.CallExpr | dashboard |
| GET /api/dashboard/industry-linkage | *ast.CallExpr | dashboard |
| GET /api/dashboard/industry-risk | *ast.CallExpr | dashboard |
| GET /api/narrative/stress-index/current | *ast.CallExpr | narrative |
| GET /api/narrative/stress-index/history | *ast.CallExpr | narrative |
| GET /api/narrative/stress-index/thresholds | *ast.CallExpr | narrative |
| / | *ast.CallExpr | other |
| /admin/reload-config | *ast.CallExpr | other |
| /admin/trigger-simulation | *ast.CallExpr | other |
| /api/admin/calibrate-thresholds | *ast.CallExpr | other |
| /metrics | *ast.CallExpr | other |
| /static/ | *ast.CallExpr | other |
| GET /api/docs | *ast.CallExpr | other |
| GET /api/docs/swagger.json | *ast.CallExpr | other |
| GET /api/macro/capital-flow/latest | *ast.CallExpr | other |
| GET /api/macro/snapshot/history | *ast.CallExpr | other |
| GET /api/report/list | *ast.CallExpr | other |
| POST /api/macro/ingest | *ast.CallExpr | other |
| GET /api/tasks | *ast.CallExpr | system |
| GET /api/tasks/{id} | *ast.CallExpr | system |
| GET /api/tasks/{id}/events | *ast.CallExpr | system |
| GET /health | *ast.CallExpr | system |
| POST /api/tasks | *ast.CallExpr | system |
| POST /api/tasks/{id}/cancel | *ast.CallExpr | system |
| POST /api/tasks/{id}/confirm | *ast.CallExpr | system |
| POST /api/tasks/{id}/retry | *ast.CallExpr | system |

## Broken Links (frontend calls non-existent API)

(none found)

