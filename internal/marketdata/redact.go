package marketdata

import "github.com/kaecer68/atlas-go/internal/redact"

// ─── Upstream response text must never reach a sink raw ─────────────────────
//
// Every provider in this package copies an upstream error body into a log line
// and/or an error string — that text is the only way an operator can tell
// "token rejected" from "quota exhausted" from "IP banned". But the same
// upstreams echo credential material back in that body: FinMind returns
// `{"msg":"Token is illegal.","status":400,"token_tail":"...abc"}`, and in
// production (2026-09-26 02:10Z) it returned `token_tail` on the 402 quota
// response as well. This repository is public and log excerpts are pasted into
// issues and PRs, so a fragment of a live key must not travel with them.
//
// The rules themselves live in internal/redact since FU-20261016-15: this
// package was the first (and for a while the only) place that needed them, but
// internal/live, internal/llm, internal/llm_annotator and cmd/* have the same
// sink shape. Copies drift and a drifted list is a silent leak, so there is
// exactly one implementation. This wrapper stays so the 20+ call sites in this
// package did not have to churn, and so the package-local corpus test keeps
// guarding this entry point.

// redactUpstreamText removes credential material from a raw upstream body
// before it reaches a log line, an error string, or the on-disk quota state.
func redactUpstreamText(body string, secrets ...string) string {
	return redact.Text(body, secrets...)
}
