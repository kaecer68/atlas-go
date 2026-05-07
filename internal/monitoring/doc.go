// Package monitoring provides system observability: dashboard HTTP API,
// metrics collection, alerting, health checks, and monitoring rules.
//
// Package structure:
//
//	monitoring/         — top-level: dashboard API router, metrics, alerting,
//	                      health checks, monitoring rules, prometheus
//	monitoring/api/     — 20 domain-specific HTTP handler sub-packages
//	monitoring/service/ — data aggregation, pipeline loading, report logic
//
// Known structural debt (P3 — future refactoring):
//
// The top-level files (dashboard_api.go, metrics.go, alert_api.go, etc.)
// should move into either service/ or an alert/ sub-package. Blocked by:
//  1. dashboard_api.go is the central routing hub (27 imports) — moving it
//     requires extracting router interface first
//  2. api/* sub-packages import monitoring package for shared types
//
// Target:
//
//	monitoring/api/     — HTTP handlers + routing (infrastructure, already split)
//	monitoring/service/ — data aggregation (business, already split)
//	monitoring/alert/   — alerting (alert_api, alert_store, notifier, email_notifier)
//	monitoring/metrics/ — metrics, health, prometheus (metrics.go, metrics_store.go, etc.)
//
// Interim: keep top-level files where they are. Focus on api/ and service/
// boundaries which are already clean.
package monitoring
