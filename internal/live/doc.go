// Package live provides live trading orchestration, broker execution,
// order management, circuit breaking, and state persistence.
//
// Known structural debt (P3 — future refactoring):
//
// This package currently mixes two roles:
//  1. Infrastructure — circuit_breaker.go, store.go, nonce_store.go,
//     eventbus.go, http_adapter.go
//  2. Business logic — broker.go, execution.go, order_manager.go,
//     agent_runner.go, orchestrator.go, scheduler.go
//
// Target: split into sub-packages:
//
//	live/broker/  — broker execution, order management (business)
//	live/store/   — Redis nonce store, state persistence (infrastructure)
//	live/http/    — HTTP broker adapter (infrastructure)
//
// Blocked by: high internal coupling through orchestrator.go central
// coordinator. Requires first extracting interfaces for Broker,
// livestore.StateStore, and EventBus before splitting.
package live
