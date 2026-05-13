package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDataPipelineService_GetPipelineStatus(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "data", "state")
	os.MkdirAll(stateDir, 0o755)

	// Create channel_health.json with some records
	healthData := `{
		"channels": {
			"twse_replay": {"status": "ok", "last_fetch_at": "2026-05-12T10:00:00+08:00", "last_success_at": "2026-05-12T10:00:00+08:00"},
			"us_yahoo": {"status": "ok", "last_fetch_at": "2026-05-12T10:00:00+08:00", "last_success_at": "2026-05-12T10:00:00+08:00"}
		}
	}`
	os.WriteFile(filepath.Join(stateDir, "channel_health.json"), []byte(healthData), 0o644)

	svc := NewDataPipelineService(tmpDir, filepath.Join(tmpDir, "ledger"))
	statuses, err := svc.GetPipelineStatus()
	if err != nil {
		t.Fatalf("GetPipelineStatus: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("expected non-empty statuses")
	}

	// Find twse_replay
	var replayFound bool
	for _, s := range statuses {
		if s.SourceID == "twse_replay" {
			replayFound = true
			if s.Status != "ok" {
				t.Errorf("twse_replay status = %q, want ok", s.Status)
			}
			if s.Producer != "daily-replay-sync" {
				t.Errorf("twse_replay producer = %q, want daily-replay-sync", s.Producer)
			}
			if s.LastProduced == "" {
				t.Error("twse_replay LastProduced should not be empty")
			}
		}
	}
	if !replayFound {
		t.Error("twse_replay not found in statuses")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
		{24 * time.Hour, "1d"},
		{3*24*time.Hour + 5*time.Hour, "3d5h"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
