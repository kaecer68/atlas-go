// Package clients provides shared HTTP infrastructure for LLM provider
// clients. It includes a BaseClient with retry, rate-limiting, circuit
// breaking, and metrics, so each concrete provider implementation
// (DeepSeek, MiniMax, Kimi standalone) can reuse the same HTTP layer
// without duplicating cross-cutting concerns.
//
// Maturity: experimental — the API surface may change as additional
// provider implementations exercise the BaseClient.
package clients
