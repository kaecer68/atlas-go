package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestResolveSeasonalReplayPath verifies the seasonal_calibration
// registration guard (#1757): the replay dataset path resolves only when
// finmind_2020_2024.jsonl exists under workDir/data/replay. An empty result
// means the task must be skipped — calibrate-seasonal hard-refuses -update
// without real replay data, so registering would guarantee a task_failed
// every 7d tick.
func TestResolveSeasonalReplayPath(t *testing.T) {
	t.Run("missing dataset returns empty", func(t *testing.T) {
		dir := t.TempDir()
		if got := resolveSeasonalReplayPath(dir); got != "" {
			t.Errorf("resolveSeasonalReplayPath(%q) = %q, want empty", dir, got)
		}
	})

	t.Run("empty replay dir returns empty", func(t *testing.T) {
		dir := t.TempDir()
		replayDir := filepath.Join(dir, "data", "replay")
		if err := os.MkdirAll(replayDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := resolveSeasonalReplayPath(dir); got != "" {
			t.Errorf("resolveSeasonalReplayPath(%q) = %q, want empty", dir, got)
		}
	})

	t.Run("existing dataset returns absolute path", func(t *testing.T) {
		dir := t.TempDir()
		replayDir := filepath.Join(dir, "data", "replay")
		if err := os.MkdirAll(replayDir, 0o755); err != nil {
			t.Fatal(err)
		}
		dataset := filepath.Join(replayDir, "finmind_2020_2024.jsonl")
		if err := os.WriteFile(dataset, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := resolveSeasonalReplayPath(dir)
		if got == "" {
			t.Fatalf("resolveSeasonalReplayPath(%q) = empty, want %q", dir, dataset)
		}
		if want, err := filepath.Abs(got); err == nil && want != got {
			t.Errorf("path not absolute-normalized: %q", got)
		}
		if filepath.Base(got) != "finmind_2020_2024.jsonl" {
			t.Errorf("unexpected dataset filename: %q", got)
		}
	})
}

// --- 2026-09-23 fubon registration hardening --------------------------------

// captureLog redirects the standard logger for one test and returns the buffer.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})
	return buf
}

// TestRegisterBackgroundTask_LogsSuccessOnlyWhenRegisterSucceeds pins the
// 2026-09-21 failure shape: the old `_ = taskMgr.Register(...)` discarded the
// error while still printing "registered …", so a dropped channel health task
// looked identical to a healthy one in the operator log.
func TestRegisterBackgroundTask_LogsSuccessOnlyWhenRegisterSucceeds(t *testing.T) {
	gw, err := apigateway.NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	// Case 1: ChannelID not registered in the gateway → Register rejects the
	// task; the error must be reported and the success line suppressed.
	mgrMissing := apigateway.NewBackgroundTaskManager(gw)
	buf := captureLog(t)
	ok := registerBackgroundTask(mgrMissing, &apigateway.ScheduledTask{
		Name:      "channel_health_missing",
		ChannelID: "not_a_registered_channel",
		Interval:  time.Hour,
		Enabled:   true,
		Task:      func(context.Context) error { return nil },
	}, "registered channel_health_missing background task (1h interval)")
	out := buf.String()
	if ok {
		t.Error("registerBackgroundTask = true for a task whose channel is not registered, want false")
	}
	if _, found := mgrMissing.Get("channel_health_missing"); found {
		t.Error("rejected task must not be present in the manager registry after a failed Register")
	}
	if !strings.Contains(out, "FAILED to register background task channel_health_missing") {
		t.Errorf("failure was not logged honestly; log output = %q", out)
	}
	if strings.Contains(out, "registered channel_health_missing background task") {
		t.Errorf("success line must NOT be printed when Register failed; log output = %q", out)
	}

	// Case 2: the channel exists → the task is registered and the success line
	// is printed exactly where it was before this change.
	gwOK, err := apigateway.NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	if err := apigateway.RegisterChannelAdapters(gwOK, t.TempDir(), config.Config{}, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters: %v", err)
	}
	mgrOK := apigateway.NewBackgroundTaskManager(gwOK)
	buf2 := captureLog(t)
	ok = registerBackgroundTask(mgrOK, &apigateway.ScheduledTask{
		Name:      "channel_health_twse_replay",
		ChannelID: "twse_replay",
		Interval:  time.Hour,
		Enabled:   true,
		Task:      func(context.Context) error { return nil },
	}, "registered channel_health_twse_replay background task (1h interval)")
	out2 := buf2.String()
	if !ok {
		t.Error("registerBackgroundTask = false for a task whose channel is registered, want true")
	}
	if _, found := mgrOK.Get("channel_health_twse_replay"); !found {
		t.Error("accepted task must be present in the manager registry")
	}
	if !strings.Contains(out2, "registered channel_health_twse_replay background task (1h interval)") {
		t.Errorf("success line missing on the success path; log output = %q", out2)
	}
}

// TestRegisterDataSyncAndHealthTasks_FubonHealthTaskDeferredWithoutAdapter
// covers the degraded startup: no fubon adapter in the gateway, so
// channel_health_fubon cannot be registered. The failure must be reported and
// the self-healing watch must be registered instead of nothing.
func TestRegisterDataSyncAndHealthTasks_FubonHealthTaskDeferredWithoutAdapter(t *testing.T) {
	gw, err := apigateway.NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	mgr := apigateway.NewBackgroundTaskManager(gw)
	buf := captureLog(t)

	cfg := config.Config{WorkDir: t.TempDir(), FubonAPIKey: "test-fubon-key"}
	registerDataSyncAndHealthTasks(mgr, cfg, gw, nil, nil, nil, nil)

	if _, found := mgr.Get("channel_health_fubon"); found {
		t.Error("channel_health_fubon must not be registered while the fubon channel is missing")
	}
	if _, found := mgr.Get("fubon_adapter_watch"); !found {
		t.Fatal("fubon_adapter_watch must be registered so the missed registration can self-heal")
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED to register background task channel_health_fubon") {
		t.Errorf("rejected channel_health_fubon registration was not reported; log output = %q", out)
	}
	if strings.Contains(out, "registered channel_health_fubon background task") {
		t.Errorf("channel_health_fubon success line printed although the task was dropped; log output = %q", out)
	}
	if !strings.Contains(out, "registered fubon_adapter_watch background task") {
		t.Errorf("fubon_adapter_watch success line missing; log output = %q", out)
	}
}

// TestRegisterDataSyncAndHealthTasks_FubonHealthTaskRegisteredWithAdapter is
// the normal-production shape: a reachable proxy (here a real TCP listener on
// the configured fubon-proxy port) registers the adapter, and both the health
// probe and the watch task are wired.
func TestRegisterDataSyncAndHealthTasks_FubonHealthTaskRegisteredWithAdapter(t *testing.T) {
	t.Setenv("ATLAS_FUBON_API_KEY", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	prevPort := fubonproxy.GetFubonProxyPort()
	fubonproxy.SetFubonProxyPort(port)
	t.Cleanup(func() {
		fubonproxy.SetProxyHost("fubon-proxy")
		fubonproxy.SetFubonProxyPort(prevPort)
		marketdata.ResetSharedFubonClient()
	})

	cfg := config.Config{WorkDir: t.TempDir(), FubonAPIKey: "test-fubon-key"}
	gw, err := apigateway.NewGateway(cfg.WorkDir, nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	if err := apigateway.RegisterChannelAdapters(gw, cfg.WorkDir, cfg, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters: %v", err)
	}
	if !gw.HasChannel("fubon") {
		t.Fatalf("precondition failed: fubon adapter not registered although a listener answers on port %d", port)
	}

	mgr := apigateway.NewBackgroundTaskManager(gw)
	buf := captureLog(t)
	registerDataSyncAndHealthTasks(mgr, cfg, gw, nil, nil, nil, nil)

	healthTask, found := mgr.Get("channel_health_fubon")
	if !found {
		t.Fatalf("channel_health_fubon not registered although the fubon adapter is present; log = %q", buf.String())
	}
	if healthTask.ChannelID != "fubon" {
		t.Errorf("channel_health_fubon ChannelID = %q, want fubon", healthTask.ChannelID)
	}
	if _, found := mgr.Get("fubon_adapter_watch"); !found {
		t.Error("fubon_adapter_watch must be registered while a fubon key is configured")
	}
	if !strings.Contains(buf.String(), "registered channel_health_fubon background task (1h interval)") {
		t.Errorf("success line missing on the healthy path; log = %q", buf.String())
	}
}
