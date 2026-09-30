package main

// calibration_tasks_test.go — FU-20260930-06 (auto_calibrate go-run sweep).
//
// The task used to spawn `go run ./cmd/calibrate-parameters`, which can never
// work in the container: the alpine image ships no Go toolchain, so every
// 7-day tick failed with `exec: "go": executable file not found in $PATH`.
//
// These tests pin the replacement contract:
//   1. binary present  ⇒ registered, and the spawned command is the SIBLING
//      binary (never `go run`);
//   2. binary absent   ⇒ not registered, with one explicit skip line naming the
//      path it looked at;
//   3. the arguments always carry --writeback=overlay and never "ssot" (the run
//      must not rewrite the SSOT inside a container where configs/ is not
//      mounted);
//   4. the working directory handed to the binary is the configured work dir
//      (the tool resolves its own workdir from os.Getwd());
//   5. no other spawner was introduced: `go`/`go run` cannot reappear in this
//      file, and the only exec.Command construction is the seam.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
)

// execCall records one calibration-binary invocation.
type execCall struct {
	dir  string
	bin  string
	args []string
}

// withAutoCalibrateSeams points the executable resolver at a fake directory and
// captures what the task would spawn. Returns the recorded calls.
func withAutoCalibrateSeams(t *testing.T, dir string, siblingExists bool) *[]execCall {
	t.Helper()
	calls := &[]execCall{}

	prevExe := executablePathFn
	prevExec := autoCalibrateExecFn
	executablePathFn = func() (string, error) {
		return filepath.Join(dir, "atlas-go"), nil
	}
	if siblingExists {
		if err := os.WriteFile(filepath.Join(dir, "calibrate-parameters"), []byte("#!/bin/sh\n"), 0o600); err != nil {
			t.Fatalf("write fake binary: %v", err)
		}
	}
	autoCalibrateExecFn = func(_ context.Context, callDir, bin string, args ...string) ([]byte, error) {
		*calls = append(*calls, execCall{dir: callDir, bin: bin, args: args})
		return []byte("Wrote 0 changed value(s) to /app/data/state/parameters.calibrated.json (configs/parameters.json untouched)\n"), nil
	}
	t.Cleanup(func() {
		executablePathFn = prevExe
		autoCalibrateExecFn = prevExec
	})
	return calls
}

func newCalibrationDeps(t *testing.T, workDir string) calibrationDeps {
	t.Helper()
	gw, err := apigateway.NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	return calibrationDeps{
		TaskMgr: apigateway.NewBackgroundTaskManager(gw),
		Cfg:     config.Config{WorkDir: workDir},
	}
}

// TestRegisterAutoCalibrate_BinaryPresentRegistersSiblingBinary pins ① and ⑤.
func TestRegisterAutoCalibrate_BinaryPresentRegistersSiblingBinary(t *testing.T) {
	dir := t.TempDir()
	calls := withAutoCalibrateSeams(t, dir, true)
	deps := newCalibrationDeps(t, "/tmp/atlas-workdir")

	buf := captureLog(t)
	deps.registerAutoCalibrate()

	task, found := deps.TaskMgr.Get("auto_calibrate")
	if !found {
		t.Fatalf("auto_calibrate was not registered although the sibling binary exists\n--- log ---\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "registered auto_calibrate background task") {
		t.Errorf("registration was not reported\n--- log ---\n%s", buf.String())
	}
	if task.Interval != 7*24*time.Hour {
		t.Errorf("interval = %v, want 7d", task.Interval)
	}

	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task returned %v, want nil (a failed run is logged, not propagated)", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("spawn count = %d, want 1", len(*calls))
	}
	call := (*calls)[0]
	wantBin := filepath.Join(dir, "calibrate-parameters")
	if call.bin != wantBin {
		t.Errorf("spawned %q, want the sibling binary %q — `go run` must never come back", call.bin, wantBin)
	}
	if strings.Contains(call.bin, "go") && !strings.Contains(call.bin, "calibrate-parameters") {
		t.Errorf("spawned a Go toolchain rather than the shipped binary: %q", call.bin)
	}
}

// TestRegisterAutoCalibrate_BinaryAbsentSkipsWithReason pins ②.
func TestRegisterAutoCalibrate_BinaryAbsentSkipsWithReason(t *testing.T) {
	dir := t.TempDir()
	withAutoCalibrateSeams(t, dir, false)
	deps := newCalibrationDeps(t, "/tmp/atlas-workdir")

	buf := captureLog(t)
	deps.registerAutoCalibrate()

	if _, found := deps.TaskMgr.Get("auto_calibrate"); found {
		t.Error("auto_calibrate must NOT be registered when the binary is missing")
	}
	logs := buf.String()
	if !strings.Contains(logs, "auto_calibrate skipped") {
		t.Errorf("a missing binary must produce one explicit skip line\n--- log ---\n%s", logs)
	}
	if !strings.Contains(logs, filepath.Join(dir, "calibrate-parameters")) {
		t.Errorf("the skip line must name the path it looked at\n--- log ---\n%s", logs)
	}
	if strings.Contains(logs, "registered auto_calibrate") {
		t.Errorf("the success line must not appear when registration was skipped\n--- log ---\n%s", logs)
	}
}

// TestRegisterAutoCalibrate_ArgumentsUseOverlayWriteback pins ③.
func TestRegisterAutoCalibrate_ArgumentsUseOverlayWriteback(t *testing.T) {
	dir := t.TempDir()
	calls := withAutoCalibrateSeams(t, dir, true)
	deps := newCalibrationDeps(t, "/tmp/atlas-workdir")
	deps.registerAutoCalibrate()
	task, found := deps.TaskMgr.Get("auto_calibrate")
	if !found {
		t.Fatal("auto_calibrate not registered")
	}
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
	args := strings.Join((*calls)[0].args, " ")
	if !strings.Contains(args, "--writeback=overlay") {
		t.Errorf("args = %q, want --writeback=overlay (configs/ is not mounted in the container)", args)
	}
	if strings.Contains(args, "ssot") {
		t.Errorf("args = %q must never request the SSOT writeback from inside the container", args)
	}
	if !strings.Contains(args, "--module=darwinian") {
		t.Errorf("args = %q, want --module=darwinian", args)
	}
}

// TestRegisterAutoCalibrate_RunsInConfiguredWorkDir pins ④.
func TestRegisterAutoCalibrate_RunsInConfiguredWorkDir(t *testing.T) {
	dir := t.TempDir()
	calls := withAutoCalibrateSeams(t, dir, true)
	const workDir = "/tmp/atlas-explicit-workdir"
	deps := newCalibrationDeps(t, workDir)
	deps.registerAutoCalibrate()
	task, _ := deps.TaskMgr.Get("auto_calibrate")
	if task == nil {
		t.Fatal("auto_calibrate not registered")
	}
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}
	if got := (*calls)[0].dir; got != workDir {
		t.Errorf("working directory = %q, want %q — the tool derives its work dir from os.Getwd()", got, workDir)
	}
}

// TestCalibrationTasksSourceHasNoGoRunSpawner pins ⑤ by PATTERN, not by counting
// call sites: the regression this ticket fixes was a `go run` spawn, so the
// property worth defending is "no package code spawns the Go toolchain", not
// "this file contains N exec calls" (which any legitimate future addition would
// break).
//
// The scan covers every non-test .go file in this package and ignores comment
// lines, because the file's comments deliberately record the removed `go run`
// history and must stay free to do so.
func TestCalibrationTasksSourceHasNoGoRunSpawner(t *testing.T) {
	goExec := regexp.MustCompile(`exec\.Command(?:Context)?\(\s*(?:ctx[^,]*,\s*)?"go"`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if goExec.MatchString(line) {
				t.Errorf("%s:%d spawns the Go toolchain (%q) — the container ships no Go toolchain; use a shipped binary (FU-20260930-06)", name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("scan found no package sources — the guard would pass vacuously")
	}

	// The guarded task must actually go through the seam, so "no direct spawn"
	// cannot be satisfied by simply deleting the call.
	raw, err := os.ReadFile("calibration_tasks.go")
	if err != nil {
		t.Fatalf("read calibration_tasks.go: %v", err)
	}
	if !strings.Contains(string(raw), "autoCalibrateExecFn(ctx") {
		t.Error("calibration_tasks.go no longer routes auto_calibrate through autoCalibrateExecFn")
	}
}
