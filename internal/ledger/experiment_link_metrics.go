package ledger

import "sync/atomic"

// sessionExperimentLinkErrorTotal counts failed session-scoped experiment
// writes (RecordSessionExperiment) across every outcome store.
//
// FU-20261016-03 (2026-10-16): production had 894/894 experiments rows with
// session_id = NULL because (a) the session-scoped INSERT had no ON CONFLICT
// while the global RecordExperiment for the same experiment_id already wrote
// the row, and (b) the resulting UNIQUE violation was dropped at the call site.
// The upsert fixes (a); this counter makes (b) impossible to hide, because the
// write failure is now observable even for callers that only log.
//
// It mirrors the repository.DualWriteFallbackTotal / ScorecardSlimRepoFallbackTotal
// pattern: a package-level atomic with an exported accessor, so monitoring and
// alerting can read it without the ledger package depending on the metrics
// wiring. A non-zero delta between two reads means session links are being lost.
var sessionExperimentLinkErrorTotal atomic.Int64

// SessionExperimentLinkErrorTotal returns the number of failed
// session-scoped experiment writes since process start. Exposed for
// monitoring/alerting consumption.
func SessionExperimentLinkErrorTotal() int64 {
	return sessionExperimentLinkErrorTotal.Load()
}
