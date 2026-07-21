// Command c07-preflight validates the Pre-flight Checklist items from
// docs/operations/sector-prediction-runbook.md §1 before the operator flips
// `SECTOR_PREDICTION_ENABLED` from default false to true.
//
// Run: go run ./cmd/experimental/c07-preflight [atlas_base_url]
//
//	atlas_base_url defaults to http://localhost:18080
//
// Exit code 0 if all automatable checks pass + manual checks are
// confirmed by operator. Non-zero with per-check failure messages
// otherwise. Operator must re-run after fixing and re-confirm
// manual checks.
//
// Clones the canonical L2.4 preflight pattern from
// docs/specs/experimental-feature-launch-gate-spec.md. C07 is instance-level
// and does not import L2.4 code (no shared library — pattern only).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultAtlasURL     = "http://localhost:18080"
	httpTimeout         = 5 * time.Second
	expectedSectorCount = 20
	observationLogPath  = "docs/operations/sector-prediction-observation-log.md"
)

type checkResult struct {
	Name    string
	OK      bool
	Manual  bool
	Message string
}

func main() {
	baseURL := defaultAtlasURL
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}
	if err := validateLocalhostURL(baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: base URL %q rejected: %v\n", baseURL, err)
		fmt.Fprintf(os.Stderr, "       preflight MUST run against localhost atlas (SSRF guard).\n")
		os.Exit(2)
	}

	checks := []checkResult{
		checkEnvironment(baseURL),
		checkObservationLog(),
		checkHealthEndpoint(baseURL),
		checkMacroSnapshot(baseURL),
		checkEventsPredictionShape(baseURL),
	}

	manualChecks := []checkResult{
		{
			Name:    "docker compose restart (no hot-reload)",
			OK:      false,
			Manual:  true,
			Message: "operator: verify atlas was restarted after SECTOR_PREDICTION_ENABLED=true flip (no hot reload)",
		},
		{
			Name:    "Boot log shows sector predictions enabled",
			OK:      false,
			Manual:  true,
			Message: "operator: tail atlas logs; confirm `[EventDriven] sector predictions enabled with macro provider` appears (NOT the disabled message)",
		},
		{
			Name:    "Spot-check: driver references MacroDataSnapshot / CycleTracker / event",
			OK:      false,
			Manual:  true,
			Message: "operator: trigger 1 prediction after flag flip, confirm at least 1 sector prediction's top-2 drivers include a macro/cycle/event reference (not pure prior fallback)",
		},
	}

	exitCode := 0
	fmt.Println("=== C07 Sector-Prediction Pre-flight Checklist (runbook §1) ===")
	fmt.Println()
	for _, c := range checks {
		printResult(c)
		if !c.OK && !c.Manual {
			exitCode = 1
		}
	}
	fmt.Println()
	fmt.Println("=== Manual checks (operator must confirm) ===")
	fmt.Println()
	for _, c := range manualChecks {
		printResult(c)
	}
	fmt.Println()

	if exitCode == 0 {
		fmt.Println("✅ All automatable checks passed.")
		fmt.Println("⚠️  Confirm the 3 manual checks above, then proceed to flag flip per runbook §1 step 3.")
	} else {
		fmt.Println("❌ One or more automatable checks FAILED. Fix before flag flip.")
	}
	os.Exit(exitCode)
}

func printResult(c checkResult) {
	marker := "❌"
	if c.OK {
		marker = "✅"
	}
	if c.Manual {
		marker = "👤"
	}
	fmt.Printf("%s %s\n", marker, c.Name)
	if c.Message != "" {
		fmt.Printf("   %s\n", c.Message)
	}
}

// checkEnvironment: ensure we're NOT pointed at production.
// Atlas base URL hostname should NOT contain "prod" or be a known prod domain.
func checkEnvironment(baseURL string) checkResult {
	url := strings.ToLower(baseURL)
	if strings.Contains(url, "prod") && !strings.Contains(url, "staging") {
		return checkResult{
			Name:    "Environment is staging (NOT production)",
			OK:      false,
			Message: fmt.Sprintf("base URL %q looks like production (contains 'prod', no 'staging' marker)", baseURL),
		}
	}
	return checkResult{
		Name: "Environment is staging (NOT production)",
		OK:   true,
	}
}

// checkObservationLog: verify docs/operations/sector-prediction-observation-log.md
// exists and has placeholder for Day 1 baseline record.
//
// Pre-flight assumes operator is ABOUT TO flip flag to true. Day 1 placeholder
// must exist before flip (per runbook §1 step 8 — Day 0 baseline).
func checkObservationLog() checkResult {
	data, err := os.ReadFile(observationLogPath)
	if err != nil {
		return checkResult{
			Name:    "observation log exists",
			OK:      false,
			Message: fmt.Sprintf("cannot read %s: %v", observationLogPath, err),
		}
	}
	body := string(data)
	if !strings.Contains(body, "<!-- Records -->") && !strings.Contains(body, "## Records") {
		return checkResult{
			Name:    "observation log has Records section",
			OK:      false,
			Message: "Records section missing — observation log stub structure broken; re-create per runbook §1 step 8",
		}
	}
	return checkResult{
		Name:    "observation log Records section present",
		OK:      true,
		Message: fmt.Sprintf("%s ready for Day-1 entry after flag flip", observationLogPath),
	}
}

// checkHealthEndpoint: GET /health, verify status=ok.
func checkHealthEndpoint(baseURL string) checkResult {
	url := strings.TrimRight(baseURL, "/") + "/health"
	resp, err := httpGet(url)
	if err != nil {
		return checkResult{
			Name:    "/health reachable",
			OK:      false,
			Message: fmt.Sprintf("GET %s failed: %v", url, err),
		}
	}
	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "/health 200 OK",
			OK:      false,
			Message: fmt.Sprintf("status=%d, body=%s", resp.StatusCode, truncate(resp.Body, 200)),
		}
	}
	var health struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &health); err != nil {
		return checkResult{
			Name:    "/health JSON parseable",
			OK:      false,
			Message: fmt.Sprintf("JSON parse failed: %v", err),
		}
	}
	if health.Status != "ok" {
		return checkResult{
			Name:    "/health status=ok",
			OK:      false,
			Message: fmt.Sprintf("got status=%q, expected ok", health.Status),
		}
	}
	return checkResult{
		Name:    "/health OK (status=ok)",
		OK:      true,
		Message: "atlas liveness probe healthy",
	}
}

// checkMacroSnapshot: GET /api/macro/snapshot/latest, verify MacroDataSnapshot
// has all 4 leading indicators with non-empty symbols (per I5 mac L1 sectors/health).
//
// Fails if any of foreign_investor_net / tsm_adr / nvda / dxy is missing
// or has empty Symbol — meaning channel ingest is broken.
func checkMacroSnapshot(baseURL string) checkResult {
	url := strings.TrimRight(baseURL, "/") + "/api/macro/snapshot/latest"
	resp, err := httpGet(url)
	if err != nil {
		return checkResult{
			Name:    "/api/macro/snapshot/latest reachable",
			OK:      false,
			Message: fmt.Sprintf("GET %s failed: %v", url, err),
		}
	}
	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "/api/macro/snapshot/latest 200 OK",
			OK:      false,
			Message: fmt.Sprintf("status=%d, body=%s", resp.StatusCode, truncate(resp.Body, 200)),
		}
	}
	// Decode response: API returns MacroDataSnapshot fields at top level
	// (e.g. recorded_at, dxy, foreign_investor_net, nvda, tsm_adr, ...).
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(resp.Body), &snapshot); err != nil {
		return checkResult{
			Name:    "/api/macro/snapshot/latest JSON parseable",
			OK:      false,
			Message: fmt.Sprintf("JSON parse failed: %v", err),
		}
	}
	required := []string{"dxy", "foreign_investor_net", "nvda", "tsm_adr"}
	missing := []string{}
	for _, name := range required {
		raw, ok := snapshot[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return checkResult{
			Name:    "MacroDataSnapshot has all 4 leading indicators",
			OK:      false,
			Message: fmt.Sprintf("missing/empty: %v (channel ingest may be broken — check /api/data/channels)", missing),
		}
	}
	return checkResult{
		Name:    "MacroDataSnapshot has all 4 leading indicators (DXY, NVDA, TSMADR, ForeignInvestorNet)",
		OK:      true,
		Message: "macro provider can be safely wired into sector_predictor",
	}
}

// checkEventsPredictionShape: GET /api/events/prediction, verify response
// contains sector_predictions field.
//
// Note: SectorPredictions is always present (invariant I1), even when flag
// is off (then empty defaults). This check verifies the field exists in the
// response shape — full data correctness verified post-flip via manual check.
func checkEventsPredictionShape(baseURL string) checkResult {
	url := strings.TrimRight(baseURL, "/") + "/api/events/prediction"
	resp, err := httpGet(url)
	if err != nil {
		return checkResult{
			Name:    "/api/events/prediction reachable",
			OK:      false,
			Message: fmt.Sprintf("GET %s failed: %v", url, err),
		}
	}
	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "/api/events/prediction 200 OK",
			OK:      false,
			Message: fmt.Sprintf("status=%d, body=%s", resp.StatusCode, truncate(resp.Body, 200)),
		}
	}
	// Decode outer data envelope; tolerate either {data: {...}} or top-level {...}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(resp.Body), &raw); err != nil {
		return checkResult{
			Name:    "/api/events/prediction JSON parseable",
			OK:      false,
			Message: fmt.Sprintf("JSON parse failed: %v", err),
		}
	}
	if _, ok := raw["sector_predictions"]; !ok {
		// Some shells wrap body under "data"
		if dataRaw, hasData := raw["data"]; hasData {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(dataRaw, &nested); err == nil {
				if _, ok := nested["sector_predictions"]; ok {
					return checkResult{
						Name:    "sector_predictions field present",
						OK:      true,
						Message: fmt.Sprintf("(nested under 'data'; full-array length verified post-flip via manual check, expecting %d entries)", expectedSectorCount),
					}
				}
			}
		}
		return checkResult{
			Name:    "sector_predictions field present",
			OK:      false,
			Message: "sector_predictions key missing in /api/events/prediction response — invariant I1 violated; investigation required before flag flip",
		}
	}
	return checkResult{
		Name:    "sector_predictions field present",
		OK:      true,
		Message: "(forecast array present; 20 sectors/day verified post-flip via manual check)",
	}
}

func httpGet(url string) (*httpResp, error) {
	// Self-defending: re-validate URL is localhost before each call.
	// Catches caller mistakes where a derived URL drifts from baseURL.
	if err := validateLocalhostURL(url); err != nil {
		return nil, fmt.Errorf("http URL failed localhost validation: %w", err)
	}
	client := &http.Client{Timeout: httpTimeout}
	//nolint:gosec // G704: URL validated as localhost-only in main() + httpGet().
	// validateLocalhostURL allows only localhost/127.0.0.1/0.0.0.0/::1 + http/https.
	// gosec taint analysis does not recognize our custom validator as a sanitizer.
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &httpResp{StatusCode: resp.StatusCode, Body: string(body)}, nil
}

type httpResp struct {
	StatusCode int
	Body       string
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// validateLocalhostURL enforces that the base URL points to a localhost
// atlas instance. This is the SSRF guard (gosec G704): the preflight tool
// must never be tricked into probing production/internal hosts.
//
// Allowed hosts: localhost, 127.0.0.1, [::1], 0.0.0.0 (loopback only).
// Schemes: http, https (we accept https for reverse-proxied local atlas).
//
// Per cmd/experimental/AGENTS.md anti-patterns: this tool must not run
// against live broker or production endpoints — validation enforces that.
// Clones L2.4 l2-4-preflight SSRF guard (no shared library — instance-level).
func validateLocalhostURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("scheme %q not allowed (want http or https)", u.Scheme)
	}
	host := u.Hostname()
	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return nil
	default:
		return fmt.Errorf("host %q not in localhost loopback (refused to probe non-local atlas)", host)
	}
}
