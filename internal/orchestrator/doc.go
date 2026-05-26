// Package orchestrator provides the central coordination layer for the Atlas-Go
// investment research system. It routes agents through layered executors
// (Context → Sector/Style/Superinvestor → Control), manages the plugin registry,
// and coordinates data flow between market data, simulation, and ledger.
//
// Key components:
//
//	SystemCore          — Main orchestration engine, session lifecycle
//	PluginHost          — Plugin lifecycle management (attach / before-sim / after-sim)
//	Registry            — Agent registry loading from configs/agents.json
//	Executor interfaces — RegimeExecutor, AgentExecutor, ControlExecutor
//
// Architecture:
//
//	Market Data → Orchestrator (context → screener → sector/style → control)
//	              → Simulator → Ledger
//
// Each executor implements a small, focused interface: Supports() check + one
// operation method. The registry iterates executors in order; first match wins
// for Regime & Agent, all matches run sequentially for Control.
//
// Maturity: stable
package orchestrator
