// Package taskexec provides async task execution: Manager, Cancel, and Subscribe.
//
// Pitfall: non-blocking tasks must surface errors via the Subscribe callback.
// Silent error dropping is not allowed (would break observability of failures).
//
// Maturity: utility
package taskexec
