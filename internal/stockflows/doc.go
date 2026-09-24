// Package stockflows maintains the per-symbol T86 institutional-flow store
// (data/state/stock_flows/<symbol>.json) that backs the stockpicker flow
// condition panel and the stockpicker win-rate flow gate.
//
// store.go owns the on-disk store: the canonical stockpicker.FlowFile shape,
// the idempotent per-date merge (atomic rewrite), post-run verification and
// the incremental window source NewestStoredDate. backfill.go is the run
// engine (one TWSE T86 request per weekday, whole-market rows merged per
// symbol) plus the anti-fake-success gates (per-day min-rows, aggregated
// "no trading day produced data").
//
// Two callers share this package so the store can no longer be refreshed by
// an unregistered tool only (issue #1945): the manual CLI
// cmd/backfill-stockpicker-flows (initial backfill / gap repair) and the
// scheduler task stockpicker_flows_update (daily post-close incremental
// refresh).
//
// Maturity: evolving
package stockflows
