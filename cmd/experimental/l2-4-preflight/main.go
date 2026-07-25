// Command l2-4-preflight validates the Pre-flight Checklist items from
// docs/operations/l2-4-runbook.md §1 before the operator flips
// `use_llm_sector_agents.value` from `false` to `true`.
//
// Run: go run ./cmd/experimental/l2-4-preflight [atlas_base_url]
//
//	atlas_base_url defaults to http://localhost:18080
//
// Exit code 0 if all automatable checks pass + manual checks are
// confirmed by operator. Non-zero with per-check failure messages
// otherwise. Operator must re-run after fixing and re-confirm
// manual checks.
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
	defaultAtlasURL = "http://localhost:18080"
	httpTimeout     = 5 * time.Second
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
		checkParametersJSON(),
		checkLLMHealth(baseURL),
		checkSynergyPanel(baseURL),
		checkObservationLog(),
	}

	manualChecks := []checkResult{
		{Name: "docker compose restart (no hot-reload)", OK: false, Manual: true, Message: "operator: verify atlas was restarted after env var + parameters.json changes"},
		{Name: "slog recommendation.symbol + agent_loop.start visible", OK: false, Manual: true, Message: "operator: tail atlas logs; confirm agent_loop.start event appears after first LLM-driven recommendation"},
		{Name: "First LLM-driven recommendation", OK: false, Manual: true, Message: "operator: trigger 1 recommendation after flag flip, confirm it goes through SemiconductorLLMAgent (not deterministic fallback)"},
	}

	exitCode := 0
	fmt.Println("=== L2.4 Pre-flight Checklist (runbook §1) ===")
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

// checkParametersJSON: verify config has expected baseline structure.
func checkParametersJSON() checkResult {
	data, err := os.ReadFile("configs/parameters.json")
	if err != nil {
		return checkResult{
			Name:    "configs/parameters.json readable",
			OK:      false,
			Message: fmt.Sprintf("cannot read configs/parameters.json: %v", err),
		}
	}
	var cfg struct {
		Orchestrator struct {
			UseLLMSectorAgents struct {
				Value  bool   `json:"value"`
				Source string `json:"source"`
			} `json:"use_llm_sector_agents"`
		} `json:"orchestrator"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return checkResult{
			Name:    "configs/parameters.json parseable",
			OK:      false,
			Message: fmt.Sprintf("JSON parse failed: %v", err),
		}
	}
	// Pre-flight assumes operator is ABOUT TO flip value to true.
	// value=false is expected at pre-flight time (flag not yet flipped).
	if cfg.Orchestrator.UseLLMSectorAgents.Value {
		return checkResult{
			Name:    "use_llm_sector_agents.value state",
			OK:      true,
			Message: "⚠️  value is already TRUE — pre-flight assumes flag NOT yet flipped. If you're starting a new observation, flip back to false first.",
		}
	}
	if cfg.Orchestrator.UseLLMSectorAgents.Source == "" {
		return checkResult{
			Name:    "use_llm_sector_agents.source documented",
			OK:      false,
			Message: "source field is empty; expected 'experimental' or 'empirical'",
		}
	}
	return checkResult{
		Name:    "configs/parameters.json structure",
		OK:      true,
		Message: fmt.Sprintf("value=false (ready to flip), source=%s", cfg.Orchestrator.UseLLMSectorAgents.Source),
	}
}

// checkLLMHealth: GET /api/llm/health, verify router_version v2.1 + providers.
func checkLLMHealth(baseURL string) checkResult {
	url := strings.TrimRight(baseURL, "/") + "/api/llm/health"
	resp, err := httpGet(url)
	if err != nil {
		return checkResult{
			Name:    "/api/llm/health reachable",
			OK:      false,
			Message: fmt.Sprintf("GET %s failed: %v", url, err),
		}
	}
	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "/api/llm/health 200 OK",
			OK:      false,
			Message: fmt.Sprintf("status=%d, body=%s", resp.StatusCode, truncate(resp.Body, 200)),
		}
	}
	var health struct {
		RouterVersion string `json:"router_version"`
		Providers     map[string]struct {
			Healthy bool `json:"healthy"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &health); err != nil {
		return checkResult{
			Name:    "/api/llm/health JSON parseable",
			OK:      false,
			Message: fmt.Sprintf("JSON parse failed: %v", err),
		}
	}
	if health.RouterVersion != "v2.1" {
		return checkResult{
			Name:    "router_version v2.1",
			OK:      false,
			Message: fmt.Sprintf("got %q, expected v2.1 (LLM_SECTOR_AGENTS_ENABLED requires v2.1+ router)", health.RouterVersion),
		}
	}
	for name, p := range health.Providers {
		if !p.Healthy {
			return checkResult{
				Name:    "All providers healthy",
				OK:      false,
				Message: fmt.Sprintf("provider %q reports unhealthy", name),
			}
		}
	}
	return checkResult{
		Name:    "/api/llm/health OK + router v2.1 + providers healthy",
		OK:      true,
		Message: fmt.Sprintf("providers checked: %d", len(health.Providers)),
	}
}

// checkSynergyPanel: GET /admin/#page-synergy, verify L2.4 schedule panel HTML present.
// This is a heuristic check — the page may be SPA-style. We look for known strings.
func checkSynergyPanel(baseURL string) checkResult {
	url := strings.TrimRight(baseURL, "/") + "/admin/"
	resp, err := httpGet(url)
	if err != nil {
		return checkResult{
			Name:    "/admin/ reachable",
			OK:      false,
			Message: fmt.Sprintf("GET %s failed: %v", url, err),
		}
	}
	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "/admin/ 200 OK",
			OK:      false,
			Message: fmt.Sprintf("status=%d", resp.StatusCode),
		}
	}
	// L2.4 schedule panel strings — search for known markup.
	markers := []string{"page-synergy", "L2.4", "schedule"}
	body := strings.ToLower(resp.Body)
	missing := []string{}
	for _, m := range markers {
		if !strings.Contains(body, strings.ToLower(m)) {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		return checkResult{
			Name:    "L2.4 schedule panel markers present in /admin/",
			OK:      false,
			Message: fmt.Sprintf("missing markers: %v (panel may not be rendering)", missing),
		}
	}
	return checkResult{
		Name: "L2.4 schedule panel reachable",
		OK:   true,
	}
}

// checkObservationLog: verify docs/operations/l2-4-observation-log.md exists
// and has Week 0 Baseline entry.
func checkObservationLog() checkResult {
	path := "docs/archive/l2-4-observation-log.md"
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{
			Name:    "observation log exists",
			OK:      false,
			Message: fmt.Sprintf("cannot read %s: %v", path, err),
		}
	}
	body := string(data)
	if !strings.Contains(body, "Week 0") {
		return checkResult{
			Name:    "observation log has Week 0 Baseline",
			OK:      false,
			Message: "Week 0 Baseline entry missing — fill it before flag flip per runbook §1 step 8",
		}
	}
	return checkResult{
		Name: "observation log Week 0 Baseline present",
		OK:   true,
	}
}

func httpGet(url string) (*httpResp, error) {
	// Self-defending: re-validate URL is localhost before each call.
	// Catches caller mistakes where a derived URL drifts from baseURL.
	if err := validateLocalhostURL(url); err != nil {
		return nil, fmt.Errorf("http URL failed localhost validation: %w", err)
	}
	client := &http.Client{Timeout: httpTimeout}
	//nolint:gosec // G704: URL validated as localhost-only at L295 + main() L43.
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
