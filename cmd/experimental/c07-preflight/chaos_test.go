package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestChaosPreflight verifies the preflight exits non-zero when simulated
// failures occur. This is a subprocess-based chaos test: it builds the
// preflight binary, starts a configurable test server, and runs the
// preflight against it.
//
// Test cases:
//   - /health returns 500 → expect exit 1
//   - /api/macro/snapshot/latest missing fields → expect exit 1
//   - /api/events/prediction missing sector_predictions → expect exit 1
//   - All endpoints OK → expect exit 0
func TestChaosPreflight(t *testing.T) {
	// Build the preflight binary.
	binPath := buildPreflight(t)
	defer os.Remove(binPath)

	tests := []struct {
		name           string
		healthStatus   int
		macroResponse  map[string]any
		eventsResponse map[string]any
		wantExitCode   int
	}{
		{
			name:         "health_500_fails",
			healthStatus: 500,
			macroResponse: map[string]any{
				"dxy":                  map[string]any{"symbol": "DXY", "value": 104.5},
				"foreign_investor_net": map[string]any{"symbol": "FIN", "value": -1200000000},
				"nvda":                 map[string]any{"symbol": "NVDA", "value": 875.0},
				"tsm_adr":              map[string]any{"symbol": "TSM", "value": 175.0},
			},
			eventsResponse: map[string]any{
				"sector_predictions": []any{},
			},
			wantExitCode: 1,
		},
		{
			name:         "macro_missing_fields_fails",
			healthStatus: 200,
			macroResponse: map[string]any{
				"dxy": map[string]any{"symbol": "DXY", "value": 104.5},
				// missing foreign_investor_net, nvda, tsm_adr
			},
			eventsResponse: map[string]any{
				"sector_predictions": []any{},
			},
			wantExitCode: 1,
		},
		{
			name:         "events_missing_sector_predictions_fails",
			healthStatus: 200,
			macroResponse: map[string]any{
				"dxy":                  map[string]any{"symbol": "DXY", "value": 104.5},
				"foreign_investor_net": map[string]any{"symbol": "FIN", "value": -1200000000},
				"nvda":                 map[string]any{"symbol": "NVDA", "value": 875.0},
				"tsm_adr":              map[string]any{"symbol": "TSM", "value": 175.0},
			},
			eventsResponse: map[string]any{
				// missing sector_predictions
			},
			wantExitCode: 1,
		},
		{
			name:         "all_ok_passes",
			healthStatus: 200,
			macroResponse: map[string]any{
				"dxy":                  map[string]any{"symbol": "DXY", "value": 104.5},
				"foreign_investor_net": map[string]any{"symbol": "FIN", "value": -1200000000},
				"nvda":                 map[string]any{"symbol": "NVDA", "value": 875.0},
				"tsm_adr":              map[string]any{"symbol": "TSM", "value": 175.0},
			},
			eventsResponse: map[string]any{
				"sector_predictions": []any{},
			},
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health":
					w.WriteHeader(tt.healthStatus)
					if tt.healthStatus == 200 {
						_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
					}
				case "/api/macro/snapshot/latest":
					w.WriteHeader(200)
					_ = json.NewEncoder(w).Encode(tt.macroResponse)
				case "/api/events/prediction":
					w.WriteHeader(200)
					_ = json.NewEncoder(w).Encode(tt.eventsResponse)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()

			// Create temporary obs log and parameters.json for file-based checks.
			tmpDir := t.TempDir()
			obsLogPath := filepath.Join(tmpDir, ".omo", "evidence", "sector-prediction-observation-log.md")
			if err := os.MkdirAll(filepath.Dir(obsLogPath), 0o755); err != nil {
				t.Fatalf("mkdir obs log dir: %v", err)
			}
			if err := os.WriteFile(obsLogPath, []byte("## Records\n"), 0o644); err != nil {
				t.Fatalf("write obs log: %v", err)
			}

			paramsPath := filepath.Join(tmpDir, "parameters.json")
			paramsContent := `{"orchestrator":{"use_llm_sector_agents":{"value":false,"source":"experimental"}}}`
			if err := os.WriteFile(paramsPath, []byte(paramsContent), 0o644); err != nil {
				t.Fatalf("write parameters.json: %v", err)
			}

			// Run preflight as subprocess.
			cmd := exec.Command(binPath, server.URL)
			cmd.Dir = tmpDir // so relative paths (configs/, docs/) resolve
			cmd.Env = append(os.Environ(), "HOME="+tmpDir)

			// Capture output for debugging.
			output, err := cmd.CombinedOutput()
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					t.Fatalf("run preflight: %v\noutput: %s", err, output)
				}
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("exit code = %d, want %d\noutput: %s", exitCode, tt.wantExitCode, output)
			}
		})
	}
}

// buildPreflight builds the preflight binary and returns its path.
func buildPreflight(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "c07-preflight")

	cmd := exec.Command("go", "build", "-o", binPath, "github.com/kaecer68/atlas-go/cmd/experimental/c07-preflight")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build preflight: %v\noutput: %s", err, output)
	}

	return binPath
}

// TestValidateLocalhostURLChaos tests the SSRF guard with edge cases.
func TestValidateLocalhostURLChaos(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"localhost_ok", "http://localhost:18080", false},
		{"127_ok", "http://127.0.0.1:18080", false},
		{"ipv6_ok", "http://[::1]:18080", false},
		{"https_localhost_ok", "https://localhost:18080", false},
		{"external_host_fails", "https://atlas.example.com", true},
		{"prod_domain_fails", "https://prod.atlas.io", true},
		{"private_ip_fails", "http://10.0.0.5:18080", true},
		{"file_scheme_fails", "file://localhost/etc/passwd", true},
		{"ftp_scheme_fails", "ftp://localhost", true},
		{"gopher_scheme_fails", "gopher://localhost:9001", true},
		{"empty_fails", "", true},
		{"no_scheme_fails", "localhost:18080", true},
		{"no_host_fails", "http://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLocalhostURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLocalhostURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
