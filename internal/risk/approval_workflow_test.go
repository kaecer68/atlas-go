package risk

import (
	"path/filepath"
	"testing"
)

func setupWorkflow(t *testing.T) *ApprovalWorkflow {
	t.Helper()
	dir := t.TempDir()
	w, err := NewApprovalWorkflow(dir)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	return w
}

func TestNewApprovalWorkflow_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub", "approvals")
	w, err := NewApprovalWorkflow(subDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.dir != subDir {
		t.Errorf("expected dir %q, got %q", subDir, w.dir)
	}
}

func TestRequestApproval_Success(t *testing.T) {
	w := setupWorkflow(t)

	req, err := w.RequestApproval("phase_advance", "system", "moving from simulation to paper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ID == "" {
		t.Error("expected non-empty ID")
	}
	if req.Type != "phase_advance" {
		t.Errorf("expected type %q, got %q", "phase_advance", req.Type)
	}
	if req.Status != StatusPending {
		t.Errorf("expected status %q, got %q", StatusPending, req.Status)
	}
	if req.RequestedBy != "system" {
		t.Errorf("expected requested_by %q, got %q", "system", req.RequestedBy)
	}
}

func TestRequestApproval_MissingType(t *testing.T) {
	w := setupWorkflow(t)

	_, err := w.RequestApproval("", "system", "reason")
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestRequestApproval_MissingRequestedBy(t *testing.T) {
	w := setupWorkflow(t)

	_, err := w.RequestApproval("phase_advance", "", "reason")
	if err == nil {
		t.Fatal("expected error for missing requested_by")
	}
}

func TestApprove_Success(t *testing.T) {
	w := setupWorkflow(t)

	req, _ := w.RequestApproval("phase_advance", "system", "test")

	err := w.Approve(req.ID, "admin", "approved for testing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := w.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("failed to get request: %v", err)
	}
	if updated.Status != StatusApproved {
		t.Errorf("expected status %q, got %q", StatusApproved, updated.Status)
	}
	if updated.ApprovedBy != "admin" {
		t.Errorf("expected approved_by %q, got %q", "admin", updated.ApprovedBy)
	}
	if updated.Note != "approved for testing" {
		t.Errorf("expected note %q, got %q", "approved for testing", updated.Note)
	}
}

func TestApprove_NotFound(t *testing.T) {
	w := setupWorkflow(t)

	err := w.Approve("nonexistent", "admin", "note")
	if err == nil {
		t.Fatal("expected error for non-existent request")
	}
}

func TestApprove_AlreadyApproved(t *testing.T) {
	w := setupWorkflow(t)

	req, _ := w.RequestApproval("phase_advance", "system", "test")
	_ = w.Approve(req.ID, "admin", "first approval")

	err := w.Approve(req.ID, "admin2", "second approval")
	if err == nil {
		t.Fatal("expected error for already approved request")
	}
}

func TestReject_Success(t *testing.T) {
	w := setupWorkflow(t)

	req, _ := w.RequestApproval("phase_advance", "system", "test")

	err := w.Reject(req.ID, "admin", "insufficient data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := w.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("failed to get request: %v", err)
	}
	if updated.Status != StatusRejected {
		t.Errorf("expected status %q, got %q", StatusRejected, updated.Status)
	}
}

func TestReject_AlreadyRejected(t *testing.T) {
	w := setupWorkflow(t)

	req, _ := w.RequestApproval("phase_advance", "system", "test")
	_ = w.Reject(req.ID, "admin", "first rejection")

	err := w.Reject(req.ID, "admin2", "second rejection")
	if err == nil {
		t.Fatal("expected error for already rejected request")
	}
}

func TestLoadAll_Empty(t *testing.T) {
	w := setupWorkflow(t)

	reqs, err := w.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0 requests, got %d", len(reqs))
	}
}

func TestLoadAll_Persistence(t *testing.T) {
	w := setupWorkflow(t)

	_, _ = w.RequestApproval("type1", "user1", "reason1")
	_, _ = w.RequestApproval("type2", "user2", "reason2")

	reqs, err := w.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].Type != "type1" {
		t.Errorf("expected first type %q, got %q", "type1", reqs[0].Type)
	}
	if reqs[1].Type != "type2" {
		t.Errorf("expected second type %q, got %q", "type2", reqs[1].Type)
	}
}

func TestGetRequest_NotFound(t *testing.T) {
	w := setupWorkflow(t)

	_, err := w.GetRequest("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent request")
	}
}

func TestPendingRequests(t *testing.T) {
	w := setupWorkflow(t)

	req1, _ := w.RequestApproval("type1", "user1", "reason1")
	req2, _ := w.RequestApproval("type2", "user2", "reason2")
	_ = w.Approve(req1.ID, "admin", "approved")

	pending, err := w.PendingRequests()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}
	if pending[0].ID != req2.ID {
		t.Errorf("expected pending %q, got %q", req2.ID, pending[0].ID)
	}
}

func TestApprovalWorkflow_StatusTracking(t *testing.T) {
	w := setupWorkflow(t)

	req, _ := w.RequestApproval("phase_advance", "system", "test")

	if req.Status != StatusPending {
		t.Errorf("initial status should be pending, got %q", req.Status)
	}

	_ = w.Approve(req.ID, "admin", "ok")
	updated, _ := w.GetRequest(req.ID)
	if updated.Status != StatusApproved {
		t.Errorf("status should be approved, got %q", updated.Status)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt")
	}
}

func TestApprovalWorkflow_FilePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	w1, _ := NewApprovalWorkflow(dir)
	req, _ := w1.RequestApproval("test_type", "user1", "test reason")

	w2, _ := NewApprovalWorkflow(dir)
	found, err := w2.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("failed to find request in new instance: %v", err)
	}
	if found.Type != "test_type" {
		t.Errorf("expected type %q, got %q", "test_type", found.Type)
	}
}
