// Package portprobe provides stateless helpers for probing TCP ports and
// managing processes that occupy them. It is intentionally free of mutable
// package state and sync primitives; callers own any lifecycle coordination.
//
// Maturity: stable
package portprobe
