package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	apischeduler "github.com/kaecer68/atlas-go/internal/monitoring/api/scheduler"
	"github.com/kaecer68/atlas-go/internal/storage"
)

// TestStorageCleanupIntegration verifies the full lifecycle of the storage_cleanup
// background task: registration → API status query → toggle → manual execution →
// file cleanup verification.
func TestStorageCleanupIntegration(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")

	// Create LifecycleManager pointing at temp directory.
	mgr := storage.NewLifecycleManager(tmpDir)

	// Seed test files: old files (should be deleted) and new files (should be kept).
	oldTime := time.Now().AddDate(0, 0, -100)
	newTime := time.Now().AddDate(0, 0, -1)

	// macro/ — pattern: 20*.json, exclude: latest.json
	makeTestFile(t, tmpDir, "macro/2024-01-01.json", oldTime) // old → delete
	makeTestFile(t, tmpDir, "macro/2026-06-01.json", newTime) // new → keep
	makeTestFile(t, tmpDir, "macro/latest.json", oldTime)     // excluded → keep

	// margin/ — pattern: *_margin.json
	makeTestFile(t, tmpDir, "margin/old_margin.json", oldTime) // old → delete
	makeTestFile(t, tmpDir, "margin/new_margin.json", newTime) // new → keep

	// export/ — pattern: *_export.json
	makeTestFile(t, tmpDir, "export/old_export.json", oldTime) // old → delete
	makeTestFile(t, tmpDir, "export/new_export.json", newTime) // new → keep

	// capital_flow/ — pattern: *.json
	makeTestFile(t, tmpDir, "capital_flow/old.json", oldTime) // old → delete
	makeTestFile(t, tmpDir, "capital_flow/new.json", newTime) // new → keep

	// tsmc_revenue/ — pattern: *_revenue.json
	makeTestFile(t, tmpDir, "tsmc_revenue/old_revenue.json", oldTime) // old → delete
	makeTestFile(t, tmpDir, "tsmc_revenue/new_revenue.json", newTime) // new → keep

	// Create BackgroundTaskManager and register storage_cleanup.
	taskMgr := apigateway.NewBackgroundTaskManager(nil)
	taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "storage_cleanup",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			_, err := mgr.Run(ctx, false)
			return err
		},
	})

	// Create scheduler API handlers.
	svc := apischeduler.NewSchedulerService(taskMgr)
	handlers := apischeduler.NewHandlers(svc)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	// Step 1: Query status via API.
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status query: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var statuses []apigateway.TaskStatus
	if err := json.Unmarshal(w.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("unmarshal statuses: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 task status, got %d", len(statuses))
	}
	if statuses[0].Name != "storage_cleanup" {
		t.Fatalf("expected task name 'storage_cleanup', got %s", statuses[0].Name)
	}
	if !statuses[0].Enabled {
		t.Fatal("expected storage_cleanup to be enabled initially")
	}

	// Step 2: Toggle disable via API.
	toggleBody, _ := json.Marshal(map[string]any{"name": "storage_cleanup", "enabled": false})
	req = httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewReader(toggleBody))
	req.Header.Set("X-API-Key", "test-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("toggle disable: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	task, ok := taskMgr.Get("storage_cleanup")
	if !ok {
		t.Fatal("storage_cleanup task not found after toggle")
	}
	if task.IsEnabled() {
		t.Fatal("expected storage_cleanup to be disabled after toggle")
	}

	// Step 3: Toggle re-enable via API.
	toggleBody, _ = json.Marshal(map[string]any{"name": "storage_cleanup", "enabled": true})
	req = httptest.NewRequest(http.MethodPost, "/api/scheduler/toggle", bytes.NewReader(toggleBody))
	req.Header.Set("X-API-Key", "test-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("toggle enable: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !task.IsEnabled() {
		t.Fatal("expected storage_cleanup to be re-enabled")
	}

	// Step 4: Manually trigger the task (simulate what BackgroundTaskManager does).
	report, err := mgr.Run(ctx, false)
	if err != nil {
		t.Fatalf("cleanup run failed: %v", err)
	}

	// Verify report: 5 old files deleted, 5 new files + 1 excluded = 6 kept.
	// Two additional trace policies report empty because no traces dir is seeded.
	if report.TotalDeleted != 5 {
		t.Fatalf("expected 5 deleted, got %d", report.TotalDeleted)
	}
	if report.TotalKept != 6 {
		t.Fatalf("expected 6 kept, got %d", report.TotalKept)
	}
	if len(report.Policies) != 7 {
		t.Fatalf("expected 7 policy reports, got %d", len(report.Policies))
	}

	// Step 5: Verify actual filesystem state.
	assertFileExists(t, tmpDir, "macro/2026-06-01.json", true)
	assertFileExists(t, tmpDir, "macro/latest.json", true)
	assertFileExists(t, tmpDir, "macro/2024-01-01.json", false)

	assertFileExists(t, tmpDir, "margin/new_margin.json", true)
	assertFileExists(t, tmpDir, "margin/old_margin.json", false)

	assertFileExists(t, tmpDir, "export/new_export.json", true)
	assertFileExists(t, tmpDir, "export/old_export.json", false)

	assertFileExists(t, tmpDir, "capital_flow/new.json", true)
	assertFileExists(t, tmpDir, "capital_flow/old.json", false)

	assertFileExists(t, tmpDir, "tsmc_revenue/new_revenue.json", true)
	assertFileExists(t, tmpDir, "tsmc_revenue/old_revenue.json", false)
}

func makeTestFile(t *testing.T, base, rel string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func assertFileExists(t *testing.T, base, rel string, want bool) {
	t.Helper()
	path := filepath.Join(base, rel)
	_, err := os.Stat(path)
	exists := !os.IsNotExist(err)
	if exists != want {
		t.Fatalf("file %s: expected exists=%v, got %v", rel, want, exists)
	}
}
