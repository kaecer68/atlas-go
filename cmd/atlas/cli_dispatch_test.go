package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// ── E21: the CLI must reject positional arguments it cannot dispatch ──
//
// Root cause this locks down: `isPrismWorkerCmd` was the ONLY positional
// check in run(); every other positional argument was silently dropped and
// run() fell through to the default one-shot simulation. The seven CLI calls
// in .github/workflows/daily-maintenance.yml (`weights adjust --apply`,
// `prism status`, `reflexivity report`, ...) therefore "succeeded" (exit 0)
// for months while producing nothing.

func TestClassifyPositionalArgs(t *testing.T) {
	const usage = "USAGE-SENTINEL"

	cases := []struct {
		name      string
		args      []string
		want      positionalCommand
		wantUsage bool // error must carry the usage text
	}{
		{"no positional args keeps the flag-driven default", []string{}, commandDefault, false},
		{"prism worker is dispatched", []string{"prism", "worker"}, commandPrismWorker, false},
		{"prism worker with extra arg still dispatches", []string{"prism", "worker", "extra"}, commandPrismWorker, false},
		{"-h prints usage", []string{"-h"}, commandHelp, false},
		{"-help prints usage", []string{"-help"}, commandHelp, false},
		{"--help prints usage", []string{"--help"}, commandHelp, false},
		{"--help after a subcommand does not start a daemon", []string{"prism", "worker", "--help"}, commandHelp, false},
		{"bare prism is rejected", []string{"prism"}, commandDefault, true},
		{"prism status (old CI call) is rejected", []string{"prism", "status"}, commandDefault, true},
		{"weights adjust --apply (old CI call) is rejected", []string{"weights", "adjust", "--apply"}, commandDefault, true},
		{"weights report (old CI call) is rejected", []string{"weights", "report"}, commandDefault, true},
		{"reflexivity report (old CI call) is rejected", []string{"reflexivity", "report"}, commandDefault, true},
		{"reflexivity alert (old CI call) is rejected", []string{"reflexivity", "alert"}, commandDefault, true},
		{"worker without prism is rejected", []string{"worker"}, commandDefault, true},
		{"an unrelated word is rejected", []string{"swarm", "run"}, commandDefault, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classifyPositionalArgs(tc.args, usage)
			if got != tc.want {
				t.Errorf("classifyPositionalArgs(%v) command = %v, want %v", tc.args, got, tc.want)
			}
			if !tc.wantUsage {
				if err != nil {
					t.Fatalf("classifyPositionalArgs(%v): unexpected error: %v", tc.args, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("classifyPositionalArgs(%v): expected a usage error, got nil", tc.args)
			}
			var usageErr *usageError
			if !errors.As(err, &usageErr) {
				t.Fatalf("classifyPositionalArgs(%v) error type = %T, want *usageError", tc.args, err)
			}
			if !strings.Contains(err.Error(), usage) {
				t.Errorf("error %q must embed the usage text", err.Error())
			}
			if !strings.Contains(err.Error(), "prism worker") {
				t.Errorf("error %q must name the only supported positional subcommand", err.Error())
			}
		})
	}
}

// run() must reject an unknown positional argument BEFORE any bootstrap work
// (config load, compose root, fubon-proxy port injection). A guard placed after
// deps.loadConfig() would still let a typo'd invocation mutate state and burn
// seconds of setup before failing.
func TestRunRejectsUnknownPositionalArgsBeforeBootstrap(t *testing.T) {
	loadConfigCalled := false
	deps := appDeps{
		loadConfig: func() config.Config {
			loadConfigCalled = true
			return config.Config{}
		},
	}

	err := run([]string{"weights", "adjust", "--apply"}, deps)
	if err == nil {
		t.Fatal("run(weights adjust --apply) = nil error; an unknown subcommand must not run the default simulation")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("run(...) error type = %T (%v), want *usageError", err, err)
	}
	if loadConfigCalled {
		t.Error("deps.loadConfig was called: the unknown-subcommand guard must fire before bootstrap")
	}
	for _, want := range []string{"weights adjust --apply", "prism worker", "Usage:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("usage error %q must contain %q", err.Error(), want)
		}
	}
}

// A bare "prism" (no "worker") is the trap the daily-maintenance workflow hit
// with `prism status`/`prism balance`/`prism report`; it must not be treated as
// a dispatchable command either.
func TestRunRejectsBarePrismSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"prism"},
		{"prism", "status"},
		{"prism", "balance"},
		{"prism", "report"},
	} {
		var usageErr *usageError
		err := run(args, appDeps{loadConfig: func() config.Config { return config.Config{} }})
		if !errors.As(err, &usageErr) {
			t.Errorf("run(%v) error = %v, want *usageError", args, err)
		}
	}
}

// -h/-help must print usage and exit 0 (nil error) without bootstrapping.
func TestRunHelpFlagIsNotAnError(t *testing.T) {
	stdoutPath := filepath.Join(t.TempDir(), "stdout.txt")
	f, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("create stdout redirect: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = f
	defer func() {
		os.Stdout = origStdout
		_ = f.Close()
	}()

	loadConfigCalled := false
	deps := appDeps{
		loadConfig: func() config.Config {
			loadConfigCalled = true
			return config.Config{}
		},
	}
	if runErr := run([]string{"-h"}, deps); runErr != nil {
		t.Fatalf("run(-h) = %v, want nil (help is not an error)", runErr)
	}
	if loadConfigCalled {
		t.Error("run(-h) must not bootstrap: deps.loadConfig was called")
	}
	os.Stdout = origStdout
	if err := f.Close(); err != nil {
		t.Fatalf("close stdout redirect: %v", err)
	}
	printed, readErr := os.ReadFile(stdoutPath)
	if readErr != nil {
		t.Fatalf("read redirected stdout: %v", readErr)
	}
	if !strings.Contains(string(printed), "Usage:") {
		t.Errorf("-h output must contain the usage header, got %q", string(printed))
	}
	if !strings.Contains(string(printed), "prism worker") {
		t.Errorf("-h output must document the prism worker subcommand, got %q", string(printed))
	}
}
