// Package server — canary response classification, shared by the runtime
// canary (tools_canary_test.go, built with -tags=canary) and its offline
// unit tests below.
//
// This file deliberately has NO build tag so that:
//   - a normal `go test ./cmd/atlas-mcp/server/` compiles and runs the unit
//     tests (they are part of `make ci-full`), and
//   - the canary build (`-tags=canary`) reuses the exact same decision logic
//     instead of duplicating it.
package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// warmupGraceDefault is how long after the canary starts a freshly-started
// service is still allowed to be warming up. The container reports healthy
// (ci-wait-healthy) while background RunWarmup is still running; the first
// cold /api/recommendations call joins the in-flight macro FetchSnapshot
// (25+ upstream providers) and can exceed the 8s global middleware ceiling
// → 503 {"degraded":true}. Override with CANARY_WARMUP_GRACE (seconds).
const warmupGraceDefault = 120 * time.Second

// tolerateEnvFailures lists tools whose non-2xx responses are
// environment-dependent rather than code regressions, with the exact
// app-level body markers that downgrade the result to WARN:
//   - stock_*: live TWSE upstream unreachable/timeout from a US-hosted
//     GitHub runner → 503 {"degraded":true,...} / context deadline exceeded
//   - macro_*: fresh container where the startup ingest has not produced a
//     snapshot yet (US runner, slow upstreams) → 404 "no macro snapshot
//     available" / 500 "macro data health"
//
// NOTE: the warmup-window 503 class (capital_flow_summary on fresh PG,
// get_recommendations during RunWarmup, and any future endpoint) is NOT
// listed here anymore — it is covered by the unified warmup-grace window in
// classifyCanaryResponse, so new endpoints never need a per-tool marker.
//
// Anything not matching these markers still FAILs.
var tolerateEnvFailures = map[string][]string{
	"stock_get_quote":               {`"degraded":true`, "context deadline exceeded", "insufficient historical quote data"},
	"stock_get_chips":               {`"degraded":true`, "context deadline exceeded", "insufficient historical quote data"},
	"stock_get_technical":           {`"degraded":true`, "context deadline exceeded", "insufficient historical quote data"},
	"macro_get_snapshot_latest":     {"no macro snapshot available"},
	"macro_get_capital_flow_latest": {"no macro snapshot available"},
	"macro_get_ingest_status":       {"macro data health"},
}

// matchesAnyMarker reports whether body contains any of the given
// substrings (empty marker list → false, so unmapped tools never WARN).
func matchesAnyMarker(body string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// classifyCanaryResponse decides one canary probe's status string
// ("ok" / "WARN: …" / "FAIL: …") from the raw probe result.
//
// Tolerated (downgraded to WARN) only:
//   - env markers (tolerateEnvFailures) — always, they are environmental
//   - HTTP 503 or connection-refused while now < graceUntil — the unified
//     warmup-grace window; a fresh service may still be warming up right
//     after "healthy"
//
// Anything else (including 500/502/504, and 503 AFTER the grace window) FAILs,
// so a real regression is still detected — the grace window only postpones
// the verdict for the first N seconds.
func classifyCanaryResponse(code int, body, tool, connErr string, now, graceUntil time.Time) string {
	if connErr != "" {
		if now.Before(graceUntil) {
			return "WARN: HTTP error (warmup-grace: connection refused, retry after grace): " + truncate(connErr, 90)
		}
		return "FAIL: HTTP error: " + truncate(connErr, 120)
	}
	switch {
	case code >= 200 && code < 300:
		return "ok"
	case code >= 300 && code < 400:
		return fmt.Sprintf("WARN: HTTP %d", code)
	case code == 401 || code == 403:
		return fmt.Sprintf("WARN: HTTP %d (auth — set ATLAS_API_KEY)", code)
	default:
		if markers := tolerateEnvFailures[tool]; matchesAnyMarker(body, markers) {
			return fmt.Sprintf("WARN: HTTP %d (env-dependent: %s)", code, truncate(body, 90))
		}
		if code == 503 && now.Before(graceUntil) {
			return fmt.Sprintf("WARN: HTTP %d (warmup-grace: 503 tolerated inside grace window, service may still be warming up)", code)
		}
		return fmt.Sprintf("FAIL: HTTP %d", code)
	}
}

// warmupGraceDuration returns the canary warmup-grace window from the
// CANARY_WARMUP_GRACE env var (seconds), defaulting to warmupGraceDefault.
func warmupGraceDuration() time.Duration {
	if v := os.Getenv("CANARY_WARMUP_GRACE"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return warmupGraceDefault
}

func TestClassifyCanaryResponse(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	graceUntil := base.Add(warmupGraceDefault)
	afterGrace := base.Add(warmupGraceDefault + time.Second)

	cases := []struct {
		name            string
		code            int
		body, tool, err string
		now, graceUntil time.Time
		wantPrefix      string // "ok" | "WARN" | "FAIL"
		wantSubstr      string // optional extra assertion
	}{
		{"2xx ok", 200, "", "regime_get_history", "", base, graceUntil, "ok", ""},
		{"3xx warn", 302, "", "regime_get_history", "", base, graceUntil, "WARN", ""},
		{"401 auth warn", 401, "", "regime_get_history", "", base, graceUntil, "WARN", "auth"},
		// get_recommendations: marker removed → covered by grace window
		{"503 within grace (recommendations)", 503, `{"degraded":true,"error":"upstream timeout"}`, "get_recommendations", "", base, graceUntil, "WARN", "warmup-grace"},
		{"503 after grace FAILs (recommendations)", 503, `{"degraded":true,"error":"upstream timeout"}`, "get_recommendations", "", afterGrace, graceUntil, "FAIL", ""},
		// capital_flow_summary: fresh-PG 503 marker removed → grace covers it
		{"503 within grace (capital_flow_summary)", 503, `{"error":"failed to fetch market data: ..."}`, "capital_flow_summary", "", base, graceUntil, "WARN", "warmup-grace"},
		{"503 after grace FAILs (capital_flow_summary)", 503, `{"error":"failed to fetch market data: ..."}`, "capital_flow_summary", "", afterGrace, graceUntil, "FAIL", ""},
		// env markers still apply inside AND outside grace
		{"503 env marker within grace (stock)", 503, `{"degraded":true}`, "stock_get_quote", "", base, graceUntil, "WARN", "env-dependent"},
		{"503 env marker after grace (stock)", 503, `{"degraded":true}`, "stock_get_quote", "", afterGrace, graceUntil, "WARN", "env-dependent"},
		{"500 env marker (macro ingest)", 500, "macro data health", "macro_get_ingest_status", "", base, graceUntil, "WARN", "env-dependent"},
		// only 503 is grace-tolerated, other 5xx still FAIL inside grace
		{"500 within grace FAILs", 500, "boom", "regime_get_history", "", base, graceUntil, "FAIL", ""},
		{"504 within grace FAILs", 504, "gateway timeout", "get_recommendations", "", base, graceUntil, "FAIL", ""},
		{"503 no marker within grace", 503, "", "regime_get_history", "", base, graceUntil, "WARN", "warmup-grace"},
		{"503 no marker after grace", 503, "", "regime_get_history", "", afterGrace, graceUntil, "FAIL", ""},
		// connection refused: tolerated only inside grace
		{"conn refused within grace", 0, "", "regime_get_history", "connection refused", base, graceUntil, "WARN", "warmup-grace"},
		{"conn refused after grace", 0, "", "regime_get_history", "connection refused", afterGrace, graceUntil, "FAIL", "connection refused"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCanaryResponse(tc.code, tc.body, tc.tool, tc.err, tc.now, tc.graceUntil)
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Fatalf("classifyCanaryResponse(%d,%q,%q,err=%q,now=%v,grace=%v) = %q; want prefix %q",
					tc.code, tc.body, tc.tool, tc.err, tc.now, tc.graceUntil, got, tc.wantPrefix)
			}
			if tc.wantSubstr != "" && !strings.Contains(got, tc.wantSubstr) {
				t.Fatalf("classifyCanaryResponse(...) = %q; want substring %q", got, tc.wantSubstr)
			}
		})
	}
}

func TestWarmupGraceDuration(t *testing.T) {
	t.Setenv("CANARY_WARMUP_GRACE", "45")
	if got := warmupGraceDuration(); got != 45*time.Second {
		t.Fatalf("CANARY_WARMUP_GRACE=45 → got %v, want 45s", got)
	}
	t.Setenv("CANARY_WARMUP_GRACE", "abc")
	if got := warmupGraceDuration(); got != warmupGraceDefault {
		t.Fatalf("invalid CANARY_WARMUP_GRACE → got %v, want default %v", got, warmupGraceDefault)
	}
	t.Setenv("CANARY_WARMUP_GRACE", "")
	if got := warmupGraceDuration(); got != warmupGraceDefault {
		t.Fatalf("empty CANARY_WARMUP_GRACE → got %v, want default %v", got, warmupGraceDefault)
	}
}
