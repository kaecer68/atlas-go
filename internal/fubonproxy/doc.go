// Package fubonproxy manages the Fubon MarketData Proxy lifecycle.
//
// Maturity: evolving
//
// The manager auto-starts the Python FastAPI service on atlas boot,
// monitors its health, restarts on failure, and gracefully shuts it
// down on atlas exit.
package fubonproxy
