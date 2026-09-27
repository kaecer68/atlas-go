package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// captureStderr redirects os.Stderr into a temp file for one test and returns a
// reader for what was written.
//
// Why not log.SetOutput: run() calls logging.Init(), which rebuilds the slog
// handler on os.Stderr and (Go's slog) re-installs the standard log bridge, so
// any writer installed before run() is discarded. Swapping os.Stderr before
// run() survives that re-init, which is the supported precedent in this repo
// (cmd/atlas-mcp-setup/coverage_test.go does the same).
//
// Reads are safe while background goroutines keep logging: *os.File is safe for
// concurrent use, and nothing asserts on bytes written after the assertion.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "atlas-stderr-*.log")
	if err != nil {
		t.Fatalf("create stderr capture file: %v", err)
	}
	prev := os.Stderr
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = prev
		// Deliberately not closed: runLiveTrading-style background goroutines
		// may still hold the handler that writes here.
	})
	return func() string {
		raw, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatalf("read stderr capture: %v", err)
		}
		return string(raw)
	}
}

// apiModeDeps builds the minimal dependency set the `-api` block needs, plus
// counters for the two seams that would fire twice if the `-live` path also
// ran (its own dashboard API and its own HTTP server).
func apiModeDeps(t *testing.T, shutdown chan struct{}) (appDeps, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var dashboardCalls, listenCalls atomic.Int32
	ledgerDir := t.TempDir()

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			// Only runLiveTrading reaches this seam when dataFetcher is set:
			// the `-api` block builds its dashboard through
			// monitoring.NewDashboardAPIWithGateway directly.
			dashboardCalls.Add(1)
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			listenCalls.Add(1)
			return nil
		},
		shutdown: shutdown,
	}
	return deps, &dashboardCalls, &listenCalls
}

// TestAPIModeIgnoresLiveFlagByDesign pins the dispatch contract for the
// production command line. docker-compose used to pass ["-api","-live",...],
// and `-api` returns from its own block before the `-live` check, so `-live`
// was silently dropped. The product boundary says the live-broker path is
// research-only (docs/reference/product-positioning.md §3), so the fix is to
// keep the precedence and make it explicit rather than to activate `-live`.
func TestAPIModeIgnoresLiveFlagByDesign(t *testing.T) {
	captured := captureStderr(t)
	shutdown := make(chan struct{})
	deps, dashboardCalls, listenCalls := apiModeDeps(t, shutdown)

	go func() {
		time.Sleep(150 * time.Millisecond)
		close(shutdown)
	}()

	addr := fmt.Sprintf(":%d", freePort(t))
	if err := run([]string{"-api", "-live", "-addr", addr}, deps); err != nil {
		t.Fatalf("run(-api -live) returned error: %v", err)
	}

	logged := captured()
	if !strings.Contains(logged, "live_flag_ignored_in_api_mode") {
		t.Errorf("combined -api -live must log the live_flag_ignored_in_api_mode event so the precedence is not mistaken for a dispatch bug; stderr was:\n%s", tail(logged, 40))
	}
	if !strings.Contains(logged, "live flag ignored in api mode by design") {
		t.Errorf("the ignored-flag log must carry the human-readable explanation; stderr was:\n%s", tail(logged, 40))
	}
	if strings.Contains(logged, "starting live trading orchestrator") {
		t.Errorf("-live must not start the research-only orchestrator in api mode; stderr was:\n%s", tail(logged, 40))
	}
	if n := listenCalls.Load(); n != 1 {
		t.Errorf("deps.listenAndServe called %d times, want exactly 1 (a single dashboard API server)", n)
	}
	if n := dashboardCalls.Load(); n != 0 {
		t.Errorf("runLiveTrading's own dashboard API was built %d times, want 0", n)
	}
}

// TestAPIModeAloneDoesNotMentionLiveFlag keeps the new message scoped to the
// combined invocation: a plain `-api` run must not gain new noise.
func TestAPIModeAloneDoesNotMentionLiveFlag(t *testing.T) {
	captured := captureStderr(t)
	shutdown := make(chan struct{})
	deps, _, listenCalls := apiModeDeps(t, shutdown)

	go func() {
		time.Sleep(150 * time.Millisecond)
		close(shutdown)
	}()

	addr := fmt.Sprintf(":%d", freePort(t))
	if err := run([]string{"-api", "-addr", addr}, deps); err != nil {
		t.Fatalf("run(-api) returned error: %v", err)
	}

	if logged := captured(); strings.Contains(logged, "live_flag_ignored_in_api_mode") {
		t.Errorf("-api alone must not log the -live precedence message; stderr was:\n%s", tail(logged, 40))
	}
	if n := listenCalls.Load(); n != 1 {
		t.Errorf("deps.listenAndServe called %d times, want exactly 1", n)
	}
}

// TestLiveModeAloneStillStartsOrchestrator is the control for the combined
// case: `-live` without `-api` must still reach runLiveTrading (the flag keeps
// working for researchers) and must not print the api-precedence message.
func TestLiveModeAloneStillStartsOrchestrator(t *testing.T) {
	captured := captureStderr(t)
	var dashboardAPICalled atomic.Bool

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			// runLiveTrading builds its own dashboard API: reaching this seam
			// proves the `-live` branch executed (the `-api` block uses
			// monitoring.NewDashboardAPIWithGateway instead when a fetcher is
			// injected).
			dashboardAPICalled.Store(true)
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error { return nil },
	}

	done := make(chan error, 1)
	go func() { done <- run([]string{"-live"}, deps) }()

	if !liveModeWaitFor(liveModeStartupDeadline, dashboardAPICalled.Load) {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("run(-live) returned error before starting the orchestrator: %v", err)
			}
		default:
		}
		t.Fatalf("-live alone must still enter runLiveTrading; stderr was:\n%s", tail(captured(), 40))
	}
	if logged := captured(); strings.Contains(logged, "live_flag_ignored_in_api_mode") {
		t.Errorf("-live alone must not log the api-precedence message; stderr was:\n%s", tail(logged, 40))
	}
}

// TestProductionComposeDoesNotRequestLiveMode guards the deployment artifact:
// the production container command must stay api-only, because the live-broker
// path is research-only (docs/reference/product-positioning.md §3) and the flag
// is ignored when combined with -api anyway.
func TestProductionComposeDoesNotRequestLiveMode(t *testing.T) {
	raw, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	// Services are decoded loosely: this compose file mixes the list form of
	// `command:` with the string form used by other services.
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}
	svc, ok := doc.Services["atlas"]
	if !ok {
		t.Fatal("docker-compose.yml has no `atlas` service")
	}
	rawCommand, ok := svc["command"]
	if !ok {
		t.Fatal("`atlas` service declares no command")
	}
	var args []string
	switch v := rawCommand.(type) {
	case []any:
		for _, a := range v {
			args = append(args, fmt.Sprint(a))
		}
	case string:
		args = strings.Fields(v)
	default:
		t.Fatalf("unexpected `atlas` command shape %T", rawCommand)
	}
	if !containsArg(args, "-api") {
		t.Errorf("`atlas` service command %q must keep serving the dashboard API (-api)", args)
	}
	if containsArg(args, "-live") {
		t.Errorf("`atlas` service command %q must not pass -live: the live-broker path is research-only and the flag is ignored in -api mode (docs/reference/product-positioning.md §3)", args)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// tail returns the last n lines of s, to keep failure output readable when the
// captured stderr holds thousands of startup lines.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
