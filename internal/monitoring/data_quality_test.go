package monitoring

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDataQualityChecker_RunAll(t *testing.T) {
	tmp := t.TempDir()
	setupMinimalDataDirs(t, tmp)

	dq := NewDataQualityChecker(tmp, filepath.Join(tmp, "data", "ledger"))
	report := dq.RunAll(context.Background())

	if len(report.Checks) == 0 {
		t.Fatal("expected checks")
	}
	if report.Overall != StatusOK {
		t.Logf("overall status: %s", report.Overall)
	}
}

func TestDataQualityChecker_RunAll_ContextCancelled(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report := dq.RunAll(ctx)
	if len(report.Checks) != 0 {
		t.Errorf("expected 0 checks on cancelled context, got %d", len(report.Checks))
	}
}

func TestDataQualityChecker_checkAlertsFile(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkAlertsFile(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("missing alerts file: expected warning, got %s", res.Status)
	}

	alertsDir := filepath.Join(tmp, "data", "state", "alerts")
	if err := os.MkdirAll(alertsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alertsDir, "alerts.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkAlertsFile(context.Background())
	if res.Status != StatusOK {
		t.Errorf("recent alerts file: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkLedgerFiles(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, filepath.Join(tmp, "data", "ledger"))

	res := dq.checkLedgerFiles(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("missing ledger dir: expected warning, got %s", res.Status)
	}

	ledgerDir := filepath.Join(tmp, "data", "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledgerDir, "test.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkLedgerFiles(context.Background())
	if res.Status != StatusOK {
		t.Errorf("ledger file present: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkSessionFiles(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkSessionFiles(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("missing session dir: expected warning, got %s", res.Status)
	}

	sessionDir := filepath.Join(tmp, "data", "state", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "s1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkSessionFiles(context.Background())
	if res.Status != StatusOK {
		t.Errorf("session file present: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkExperimentFiles(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkExperimentFiles(context.Background())
	if res.Status != StatusSkipped {
		t.Errorf("missing experiment dir: expected skipped, got %s", res.Status)
	}

	experimentDir := filepath.Join(tmp, "data", "state", "experiments")
	if err := os.MkdirAll(experimentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(experimentDir, "e1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkExperimentFiles(context.Background())
	if res.Status != StatusOK {
		t.Errorf("experiment file present: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkConfigFiles(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkConfigFiles(context.Background())
	if res.Status != StatusCritical {
		t.Errorf("missing configs: expected critical, got %s", res.Status)
	}

	configsDir := filepath.Join(tmp, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configsDir, "agents.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configsDir, "parameters.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkConfigFiles(context.Background())
	if res.Status != StatusOK {
		t.Errorf("configs present: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkPromptFiles(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkPromptFiles(context.Background())
	if res.Status != StatusSkipped {
		t.Errorf("missing agents.json: expected skipped, got %s", res.Status)
	}

	configsDir := filepath.Join(tmp, "configs")
	promptsDir := filepath.Join(tmp, "prompts", "agents")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	agentsJSON := `{"agents":[{"name":"scout","enabled":true}]}`
	if err := os.WriteFile(filepath.Join(configsDir, "agents.json"), []byte(agentsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkPromptFiles(context.Background())
	if res.Status != StatusCritical {
		t.Errorf("missing prompt file: expected critical, got %s", res.Status)
	}

	if err := os.WriteFile(filepath.Join(promptsDir, "scout.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkPromptFiles(context.Background())
	if res.Status != StatusOK {
		t.Errorf("prompt file present: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkDataDirectorySize(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)

	res := dq.checkDataDirectorySize(context.Background())
	if res.Status != StatusOK {
		t.Errorf("empty data dir: expected ok, got %s", res.Status)
	}

	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "big.bin"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	res = dq.checkDataDirectorySize(context.Background())
	if res.Status != StatusOK {
		t.Errorf("small data dir: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkFilePermissions(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, filepath.Join(tmp, "data", "ledger"))

	res := dq.checkFilePermissions(context.Background())
	if res.Status != StatusCritical {
		t.Errorf("missing dirs: expected critical, got %s", res.Status)
	}

	stateDir := filepath.Join(tmp, "data", "state")
	ledgerDir := filepath.Join(tmp, "data", "ledger")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	res = dq.checkFilePermissions(context.Background())
	if res.Status != StatusOK {
		t.Errorf("writable dirs: expected ok, got %s", res.Status)
	}
}

func TestDataQualityChecker_determineOverallStatus(t *testing.T) {
	dq := NewDataQualityChecker(".", ".")

	tests := []struct {
		name   string
		checks []DataQualityCheck
		want   CheckStatus
	}{
		{"ok", []DataQualityCheck{{Status: StatusOK}}, StatusOK},
		{"warning", []DataQualityCheck{{Status: StatusWarning}}, StatusWarning},
		{"critical", []DataQualityCheck{{Status: StatusWarning}, {Status: StatusCritical}}, StatusCritical},
		{"empty", []DataQualityCheck{}, StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dq.determineOverallStatus(tt.checks)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseEnabledAgents(t *testing.T) {
	data := []byte(`{"agents":[{"name":"a1","enabled":true},{"name":"a2","enabled":false}]}`)
	got := parseEnabledAgents(data)
	if len(got) != 1 || got[0].name != "a1" {
		t.Errorf("got %v, want [{a1 }]", got)
	}

	if got := parseEnabledAgents([]byte("invalid")); got != nil {
		t.Errorf("invalid json: expected nil, got %v", got)
	}
}

func TestDataQualityChecker_checkAlertsFile_Empty(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)
	alertsDir := filepath.Join(tmp, "data", "state", "alerts")
	if err := os.MkdirAll(alertsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alertsDir, "alerts.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	res := dq.checkAlertsFile(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("empty alerts file: expected warning, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkAlertsFile_Stale(t *testing.T) {
	tmp := t.TempDir()
	dq := NewDataQualityChecker(tmp, tmp)
	alertsDir := filepath.Join(tmp, "data", "state", "alerts")
	if err := os.MkdirAll(alertsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(alertsDir, "alerts.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	res := dq.checkAlertsFile(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("stale alerts file: expected warning, got %s", res.Status)
	}
}

func TestDataQualityChecker_checkLedgerFiles_Oversized(t *testing.T) {
	tmp := t.TempDir()
	ledgerDir := filepath.Join(tmp, "data", "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 520 MB sparse file (Truncate) — matches the recalibrated 500 MB
	// threshold without allocating real disk space.
	f, err := os.Create(filepath.Join(ledgerDir, "big.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(520 * 1024 * 1024); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dq := NewDataQualityChecker(tmp, ledgerDir)
	res := dq.checkLedgerFiles(context.Background())
	if res.Status != StatusWarning {
		t.Errorf("oversized ledger: expected warning, got %s", res.Status)
	}
}

func setupMinimalDataDirs(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(root, "data", "state", "alerts"),
		filepath.Join(root, "data", "ledger"),
		filepath.Join(root, "data", "state", "sessions"),
		filepath.Join(root, "data", "state", "experiments"),
		filepath.Join(root, "configs"),
		filepath.Join(root, "prompts", "agents"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "agents.json"), []byte(`{"agents":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "parameters.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}
