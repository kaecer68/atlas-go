package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/kaecer68/atlas-go/internal/logging"
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
