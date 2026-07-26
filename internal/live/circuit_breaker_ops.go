// Package live — CircuitBreaker interface (C10 refactoring).
//
// Extracted from the concrete CircuitBreaker type to decouple Orchestrator
// and Scheduler from the file-persisted implementation. The concrete type
// (circuit_breaker.go) implicitly implements this interface.
//
// This is step 1 of the doc.go refactoring plan: extract interfaces
// before splitting into sub-packages.
package live

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// CircuitBreakerOps defines the minimal circuit breaker contract used by
// Orchestrator and Scheduler. The concrete CircuitBreaker in circuit_breaker.go
// implements this implicitly.
type CircuitBreakerOps interface {
	State() CircuitState
	ResetDayState(startingPortfolioValue float64)
	RecordStopLoss()
	Evaluate(portfolio livestore.PortfolioState, positions map[string]domain.Position, events []livestore.Event)
	CanPlaceOrder(side domain.Side) bool
}
