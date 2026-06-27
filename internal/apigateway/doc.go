// Package apigateway provides the unified data gateway for atlas-go.
//
// All external data fetches (TWSE, FinMind, Fugle, Yahoo, Frankfurter, etc.)
// MUST go through gateway.Fetch(channelID). Direct use of net/http or
// http.Client is forbidden by CONSTITUTION.md and blocked at PR review.
//
// Core components:
//
//	Gateway                — Single entry point: cache + rate-limit + circuit-breaker
//	ChannelRegistry        — Channel ID enumeration and lookup
//	RateLimitManager       — Per-channel rate limiting (some channels use rate.Inf)
//	CircuitBreakerManager  — Per-channel FSM (closed/open/half-open)
//	BackgroundTaskManager  — Scheduled tasks with jitter + overlap protection
//	ChannelHealthStore     — Per-channel fetch health records (Wave 12 Phase 2 canonical)
//
// FetchResult wraps Data + Meta + Stale/Fallback/LastError. When the circuit
// breaker is open and a stale cache hit is served, Fallback is true and LastError
// carries the original error — consumers must treat this as last-known-good
// rather than fresh data.
//
// Adding a new channel requires two changes: rate-limit registration in limits.go
// plus the channel ID enum in gateway.go's channelIDs().
//
// Maturity: stable
package apigateway
