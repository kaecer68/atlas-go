package tax

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// =====================================================================
// readPositionsFromFile
// =====================================================================

func TestReadPositionsFromFile_NonExistent(t *testing.T) {
	pos := readPositionsFromFile("/nonexistent/path/positions.json")
	if pos != nil {
		t.Errorf("expected nil for nonexistent file, got %v", pos)
	}
}

func TestReadPositionsFromFile_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pos := readPositionsFromFile(path)
	if pos != nil {
		t.Errorf("expected nil for empty array, got %v", pos)
	}
}

func TestReadPositionsFromFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pos := readPositionsFromFile(path)
	if pos != nil {
		t.Errorf("expected nil for invalid JSON, got %v", pos)
	}
}

func TestReadPositionsFromFile_ValidPositions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.json")
	positions := []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500.0, CurrentPrice: 550.0},
	}
	data, _ := json.Marshal(positions)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pos := readPositionsFromFile(path)
	if pos == nil {
		t.Fatal("expected non-nil positions")
	}
	if len(pos) != 1 {
		t.Errorf("expected 1 position, got %d", len(pos))
	}
	if pos[0].Symbol != "2330.TW" {
		t.Errorf("expected symbol 2330.TW, got %s", pos[0].Symbol)
	}
}

func TestReadPositionsFromFile_TooShortData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tooshort.json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pos := readPositionsFromFile(path)
	if pos != nil {
		t.Errorf("expected nil for empty array, got %v", pos)
	}
}

// =====================================================================
// loadPositions – no positions available
// =====================================================================

func TestLoadPositions_LedgerDirDoesNotExist(t *testing.T) {
	h := NewHandlers("/nonexistent/ledger/dir", nil)
	pos := h.loadPositions()
	if pos != nil {
		t.Errorf("expected nil when ledger dir does not exist, got %v", pos)
	}
}

func TestLoadPositions_LivePositionsFile(t *testing.T) {
	dir := t.TempDir()
	liveDir := filepath.Join(dir, "live", "state")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	positions := []domain.Position{
		{Symbol: "2317.TW", Quantity: 500, AverageCost: 100.0, CurrentPrice: 110.0},
	}
	data, _ := json.Marshal(positions)
	if err := os.WriteFile(filepath.Join(liveDir, "positions_current.json"), data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := NewHandlers(dir, nil)
	pos := h.loadPositions()
	if pos == nil {
		t.Fatal("expected non-nil positions from live file")
	}
	if len(pos) != 1 || pos[0].Symbol != "2317.TW" {
		t.Errorf("unexpected positions: %+v", pos)
	}
}

func TestLoadPositions_LivePositionsJSONLFile(t *testing.T) {
	dir := t.TempDir()
	liveDir := filepath.Join(dir, "live", "state")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	positions := []domain.Position{
		{Symbol: "2498.TW", Quantity: 200, AverageCost: 300.0, CurrentPrice: 320.0},
	}
	data, _ := json.Marshal(positions)
	if err := os.WriteFile(filepath.Join(liveDir, "positions_current.jsonl"), data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := NewHandlers(dir, nil)
	pos := h.loadPositions()
	if pos == nil {
		t.Fatal("expected non-nil positions from jsonl file")
	}
	if len(pos) != 1 || pos[0].Symbol != "2498.TW" {
		t.Errorf("unexpected positions: %+v", pos)
	}
}

func TestLoadPositions_SessionsDir_NoSessions(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := NewHandlers(dir, nil)
	pos := h.loadPositions()
	if pos != nil {
		t.Errorf("expected nil when no sessions, got %v", pos)
	}
}

func TestLoadPositions_SessionsDir_ZeroPositionCount(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions", "session-20250601")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	summary := `{"position_count":0}`
	if err := os.WriteFile(filepath.Join(sessionsDir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := NewHandlers(dir, nil)
	pos := h.loadPositions()
	if pos != nil {
		t.Errorf("expected nil when position_count=0, got %v", pos)
	}
}

func TestLoadPositions_SessionsDir_MissingSummaryFile(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions", "session-20250601")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := NewHandlers(dir, nil)
	pos := h.loadPositions()
	if pos != nil {
		t.Errorf("expected nil when summary.json missing, got %v", pos)
	}
}

// =====================================================================
// HandleTaxSnapshot – additional coverage
// =====================================================================

// Empty ledger dir → returns 200 with empty snapshots.
func TestHandleTaxSnapshot_EmptyLedgerDir(t *testing.T) {
	dir := t.TempDir()
	h := NewHandlers(dir, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/tax-snapshot", nil)
	status, body := h.HandleTaxSnapshot(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp := body.(map[string]any)
	snapshots := resp["snapshots"].([]domain.TaxSnapshot)
	if len(snapshots) != 0 {
		t.Errorf("expected empty snapshots, got %d", len(snapshots))
	}
	if resp["is_simulated"] != true {
		t.Errorf("expected is_simulated=true, got %v", resp["is_simulated"])
	}
}

// Live positions file takes precedence over sessions.
func TestHandleTaxSnapshot_LiveFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	liveDir := filepath.Join(dir, "live", "state")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	positions := []domain.Position{
		{Symbol: "2603.TW", Quantity: 100, AverageCost: 30.0, CurrentPrice: 35.0},
	}
	data, _ := json.Marshal(positions)
	if err := os.WriteFile(filepath.Join(liveDir, "positions_current.json"), data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Also create a sessions dir that would be picked up if live path failed.
	sessionsDir := filepath.Join(dir, "sessions", "session-20250601")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := NewHandlers(dir, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/tax-snapshot", nil)
	status, body := h.HandleTaxSnapshot(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp := body.(map[string]any)
	if resp["is_simulated"] != false {
		t.Errorf("expected is_simulated=false when live positions exist, got %v", resp["is_simulated"])
	}
}
