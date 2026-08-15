package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

func TestRun_Phase0And1(t *testing.T) {
	var buf bytes.Buffer
	orig := logging.Default()
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logging.SetLogger(slog.New(handler))
	defer logging.SetLogger(orig)

	reg, err := run()
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}

	if reg == nil {
		t.Fatal("registry is nil")
	}

	// A1
	if got := reg.Count(); got != 12 {
		t.Errorf("A1 FAIL: expected registry.Count() == 12, got %d", got)
	} else {
		t.Logf("A1 PASS: registry.Count() == 12")
	}

	// A2
	allValid := true
	for _, f := range reg.All() {
		if vErr := f.Validate(); vErr != nil {
			t.Errorf("A2 FAIL: frame %q invalid: %v", f.ID, vErr)
			allValid = false
		}
	}
	if allValid {
		t.Logf("A2 PASS: all frames pass Validate()")
	}

	// A3
	logOutput := buf.String()
	if !strings.Contains(logOutput, "strategy_techniques_loaded") || !strings.Contains(logOutput, "count=12") {
		t.Errorf("A3 FAIL: expected log line strategy_techniques_loaded count=12, got:\n%s", logOutput)
	} else {
		t.Logf("A3 PASS: log line strategy_techniques_loaded count=12 present")
	}
}

func TestRun_Phase2_SystemConstruction(t *testing.T) {
	// Hermetic: JSONL backend in temp dirs regardless of machine-global
	// ATLAS_STORE_BACKEND (~/.config/atlas-go/.env may default to postgres).
	origBackend := os.Getenv("ATLAS_STORE_BACKEND")
	os.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	defer os.Setenv("ATLAS_STORE_BACKEND", origBackend)

	var buf bytes.Buffer
	orig := logging.Default()
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logging.SetLogger(slog.New(handler))
	defer logging.SetLogger(orig)

	reg, err := run()
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if reg == nil {
		t.Fatal("registry is nil")
	}

	tempDir, err := os.MkdirTemp("", "phase2-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)
	savePath := filepath.Join(tempDir, "save.json")

	sys, err := runPhase2(reg, savePath)
	if err != nil {
		t.Fatalf("runPhase2 error: %v", err)
	}
	if sys == nil {
		t.Fatal("system is nil after runPhase2")
	}

	sysVal := reflect.ValueOf(sys).Elem()
	hostField := sysVal.FieldByName("host")
	if !hostField.IsValid() {
		t.Fatal("A4 FAIL: System.host field not found")
	}
	hostPtr := unsafe.Pointer(hostField.UnsafeAddr())
	hostVal := reflect.NewAt(hostField.Type(), hostPtr).Elem()
	if hostVal.IsNil() {
		t.Fatal("A4 FAIL: System.host is nil after WithStrategyTechniques")
	}
	pluginsField := hostVal.Elem().FieldByName("plugins")
	if !pluginsField.IsValid() {
		t.Fatal("A4 FAIL: PluginHost.plugins field not found")
	}
	pluginsPtr := unsafe.Pointer(pluginsField.UnsafeAddr())
	pluginsVal := reflect.NewAt(pluginsField.Type(), pluginsPtr).Elem()
	n := pluginsVal.Len()
	if n == 0 {
		t.Errorf("A4 FAIL: PluginHost.plugins is empty (expected >=1 registered plugin)")
	} else {
		t.Logf("A4 PASS: PluginHost.plugins has %d entry(ies) registered", n)
	}
}

func TestRun_Phase3_PostSimulation(t *testing.T) {
	// Hermetic: JSONL backend in temp dirs regardless of machine-global
	// ATLAS_STORE_BACKEND (~/.config/atlas-go/.env may default to postgres).
	origBackend := os.Getenv("ATLAS_STORE_BACKEND")
	os.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	defer os.Setenv("ATLAS_STORE_BACKEND", origBackend)

	var buf bytes.Buffer
	orig := logging.Default()
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logging.SetLogger(slog.New(handler))
	defer logging.SetLogger(orig)

	reg, err := run()
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "phase3-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	savePath := filepath.Join(tempDir, "save.json")
	sys, err := runPhase2(reg, savePath)
	if err != nil {
		t.Fatalf("runPhase2 error: %v", err)
	}

	hostField := reflect.ValueOf(sys).Elem().FieldByName("host")
	if !hostField.IsValid() {
		t.Fatal("System.host field not found")
	}
	hostPtr := unsafe.Pointer(hostField.UnsafeAddr())
	pluginHost := *(**orchestrator.PluginHost)(hostPtr)
	if pluginHost == nil {
		t.Fatal("pluginHost is nil")
	}

	var quotes []domain.Quote
	regime := domain.Regime("")
	for i := 1; i <= 3; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Minute)
		pluginHost.PostSimulation(quotes, regime, ts)
	}

	logOutput := buf.String()

	if !strings.Contains(logOutput, "strategy_techniques_plugin") || !strings.Contains(logOutput, "PostSimulation") {
		t.Errorf("A5 FAIL: expected log to contain 'strategy_techniques_plugin' component and 'PostSimulation' event, got:\n%s", logOutput)
	} else {
		t.Logf("A5 PASS: strategy_techniques_plugin component with PostSimulation event present")
	}

	if !strings.Contains(logOutput, "active_strategies=12") {
		t.Errorf("A6 FAIL: expected log line 'active_strategies=12', got:\n%s", logOutput)
	} else {
		t.Logf("A6 PASS: active_strategies=12 in log")
	}

	hasE1 := strings.Contains(logOutput, "evt_buf=0")
	if hasE1 {
		t.Logf("A7 PASS: evt_buf field logged (buffer not populated by PostSimulation; growth requires narrative events)")
	} else {
		t.Errorf("A7 FAIL: expected log line evt_buf=0, got:\n%s", logOutput)
	}

	t.Logf("A8 PASS: no panic during 3 PostSimulation calls")
}

// withEnv sets an env var for the duration of a test, restoring the prior
// value (or absence) via t.Cleanup so parallel/sub-test isolation is safe.
// Pass val=="" to ensure the var is unset for the test.
func withEnv(t *testing.T, key, val string) {
	t.Helper()
	orig, hadOrig := os.LookupEnv(key)
	if val == "" {
		_ = os.Unsetenv(key)
	} else {
		if err := os.Setenv(key, val); err != nil {
			t.Fatalf("os.Setenv(%q): %v", key, err)
		}
	}
	t.Cleanup(func() {
		if hadOrig {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// firstFrameID returns the first frame's ID from the registry.
// Test-only seam; the production main() must not depend on it.
func firstFrameID(t *testing.T, reg *strategy_techniques.Registry) string {
	t.Helper()
	all := reg.All()
	if len(all) == 0 {
		t.Fatal("registry is empty (expected 12 frames)")
	}
	return all[0].ID
}

// startAnnotateServer hosts a /api/strategies/{id}/annotate handler on an
// httptest.Server, optionally wiring the supplied annotator. Server is
// closed via t.Cleanup.
func startAnnotateServer(t *testing.T, reg *strategy_techniques.Registry, annotator llm_annotator.Annotator) *httptest.Server {
	t.Helper()
	h := strategies.NewHandlers(reg, nil)
	if annotator != nil {
		h.SetAnnotator(annotator)
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestRun_Phase4_And_5 covers A9 (no API key → dummy mode, no error),
// A10 (with API key + endpoint → /annotate returns 200), and Phase 5
// (no temp dirs leak across runs).
func TestRun_Phase4_And_5(t *testing.T) {
	reg, err := run()
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}
	if reg == nil {
		t.Fatal("registry is nil")
	}
	frameID := firstFrameID(t, reg)

	t.Run("A9_no_api_key_dummy_mode", func(t *testing.T) {
		// Given: LLM_ANNOTATOR_API_KEY is unset, no endpoint URL configured.
		withEnv(t, "LLM_ANNOTATOR_API_KEY", "")
		withEnv(t, annotateEndpointEnvVar, "")

		// When: runPhase4And5 is called.
		// Then: it returns nil — the CLI does not exit non-zero when the
		// LLM key is absent. This is the dummy-mode contract.
		if err := runPhase4And5(reg, frameID); err != nil {
			t.Errorf("A9 FAIL: expected nil in dummy mode, got: %v", err)
			return
		}
		t.Logf("A9 PASS: no LLM_ANNOTATOR_API_KEY -> dummy mode, no error")
	})

	t.Run("A10_with_api_key_200", func(t *testing.T) {
		// Given: a real /annotate endpoint (httptest) with a wired mock
		// annotator, and LLM_ANNOTATOR_API_KEY set in the env.
		withEnv(t, "LLM_ANNOTATOR_API_KEY", "test-key-xxx")
		// Given: ATLAS_API_KEY unset — AuthMiddleware captures env at mux-
		// handle time and would gate /annotate behind X-API-Key otherwise.
		withEnv(t, "ATLAS_API_KEY", "")
		withEnv(t, "ATLAS_ADMIN_KEY", "")
		withEnv(t, "ATLAS_ENV", "")

		mock := llm_annotator.NewMock("ok: simulated LLM annotation")
		srv := startAnnotateServer(t, reg, mock)

		// When: runPhase4And5WithEndpoint is called with the test URL.
		// Then: it returns nil — the endpoint responded 200 to /annotate.
		if err := runPhase4And5WithEndpoint(reg, frameID, srv.URL); err != nil {
			t.Errorf("A10 FAIL: %v", err)
			return
		}
		t.Logf("A10 PASS: with LLM_ANNOTATOR_API_KEY -> /annotate returned 200")
	})

	t.Run("phase5_temp_dir_cleanup", func(t *testing.T) {
		// Given: a baseline count of staging-drill-phase45-* temp dirs.
		pattern := filepath.Join(os.TempDir(), "staging-drill-phase45-*")
		beforeMatches, _ := filepath.Glob(pattern)
		before := len(beforeMatches)

		// When: runPhase4And5WithEndpoint runs to completion.
		withEnv(t, "LLM_ANNOTATOR_API_KEY", "test-key-cleanup")
		withEnv(t, "ATLAS_API_KEY", "")
		withEnv(t, "ATLAS_ADMIN_KEY", "")
		withEnv(t, "ATLAS_ENV", "")
		mock := llm_annotator.NewMock("cleanup probe")
		srv := startAnnotateServer(t, reg, mock)
		if err := runPhase4And5WithEndpoint(reg, frameID, srv.URL); err != nil {
			t.Fatalf("phase5 setup call failed: %v", err)
		}
		// Give the OS a moment to release handles before the next glob.
		time.Sleep(50 * time.Millisecond)

		// Then: no new temp dirs leaked past the function return.
		afterMatches, _ := filepath.Glob(pattern)
		after := len(afterMatches)
		if after != before {
			t.Errorf("Phase 5 FAIL: temp dir leak: before=%d after=%d (leaked: %v)", before, after, afterAfter(beforeMatches, afterMatches))
			return
		}
		t.Logf("Phase 5 PASS: no temp dirs leaked (before=%d after=%d)", before, after)
	})
}

// afterAfter returns the slice of paths present in `after` but not in `before`.
// Used only for failure diagnostics; not asserted on.
func afterAfter(before, after []string) []string {
	set := make(map[string]struct{}, len(before))
	for _, p := range before {
		set[p] = struct{}{}
	}
	var leaked []string
	for _, p := range after {
		if _, ok := set[p]; !ok {
			leaked = append(leaked, p)
		}
	}
	return leaked
}
