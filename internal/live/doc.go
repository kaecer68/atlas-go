// Package live provides live trading orchestration, broker execution,
// order management, circuit breaking, and state persistence.
//
// Architectural relationship to internal/orchestrator:
//
//   - internal/orchestrator.System is the batch research engine: it runs the
//     full simulation pipeline (screening → recommendation → guard filters) via
//     RunDailySimulation and produces SimulationResult.
//   - live.Orchestrator is the event-driven execution engine: it schedules
//     market-open / intraday-cycle / market-close events and executes orders
//     through a Broker.
//   - The two engines are intentionally separate because they have different
//     latency, state, and failure models. Simulation can take minutes; live
//     cycles must finish in seconds. Simulation tracks full history for learning;
//     live tracks current positions for execution.
//   - The bridge is orchestrator.AdapterProducer, which implements
//     orchestrator.LiveExecutionInputProvider by calling the same
//     ExecuteWithContext used by batch simulations and emitting a
//     domain.ExecutionInput for live.AgentRunner to execute. This ensures live
//     trading consumes the same screened recommendations as the research engine.
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
//
// Maturity: stable
package live
