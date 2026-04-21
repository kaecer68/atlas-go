package risk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ApprovalStatus string

const (
	StatusPending   ApprovalStatus = "pending"
	StatusApproved  ApprovalStatus = "approved"
	StatusRejected  ApprovalStatus = "rejected"
	StatusCancelled ApprovalStatus = "cancelled"
)

type ApprovalRequest struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Status      ApprovalStatus `json:"status"`
	RequestedBy string         `json:"requested_by"`
	Reason      string         `json:"reason"`
	ApprovedBy  string         `json:"approved_by,omitempty"`
	Note        string         `json:"note,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ApprovalWorkflow struct {
	dir string
}

func NewApprovalWorkflow(dir string) (*ApprovalWorkflow, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create approval directory: %w", err)
	}
	return &ApprovalWorkflow{dir: dir}, nil
}

func (w *ApprovalWorkflow) RequestApproval(reqType, requestedBy, reason string) (*ApprovalRequest, error) {
	if reqType == "" {
		return nil, fmt.Errorf("request type is required")
	}
	if requestedBy == "" {
		return nil, fmt.Errorf("requested_by is required")
	}

	now := time.Now()
	req := &ApprovalRequest{
		ID:          fmt.Sprintf("approval-%s-%d", reqType, now.UnixNano()),
		Type:        reqType,
		Status:      StatusPending,
		RequestedBy: requestedBy,
		Reason:      reason,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := w.appendRequest(req); err != nil {
		return nil, fmt.Errorf("persist approval request: %w", err)
	}

	return req, nil
}

func (w *ApprovalWorkflow) Approve(requestID, approvedBy, note string) error {
	reqs, err := w.loadAll()
	if err != nil {
		return fmt.Errorf("load requests: %w", err)
	}

	found := false
	for i, r := range reqs {
		if r.ID == requestID {
			if r.Status != StatusPending {
				return fmt.Errorf("request %q is already %q", requestID, r.Status)
			}
			reqs[i].Status = StatusApproved
			reqs[i].ApprovedBy = approvedBy
			reqs[i].Note = note
			reqs[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("request %q not found", requestID)
	}

	return w.rewriteAll(reqs)
}

func (w *ApprovalWorkflow) Reject(requestID, rejectedBy, note string) error {
	reqs, err := w.loadAll()
	if err != nil {
		return fmt.Errorf("load requests: %w", err)
	}

	found := false
	for i, r := range reqs {
		if r.ID == requestID {
			if r.Status != StatusPending {
				return fmt.Errorf("request %q is already %q", requestID, r.Status)
			}
			reqs[i].Status = StatusRejected
			reqs[i].ApprovedBy = rejectedBy
			reqs[i].Note = note
			reqs[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("request %q not found", requestID)
	}

	return w.rewriteAll(reqs)
}

func (w *ApprovalWorkflow) LoadAll() ([]ApprovalRequest, error) {
	return w.loadAll()
}

func (w *ApprovalWorkflow) GetRequest(requestID string) (*ApprovalRequest, error) {
	reqs, err := w.loadAll()
	if err != nil {
		return nil, fmt.Errorf("load requests: %w", err)
	}

	for _, r := range reqs {
		if r.ID == requestID {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("request %q not found", requestID)
}

func (w *ApprovalWorkflow) PendingRequests() ([]ApprovalRequest, error) {
	reqs, err := w.loadAll()
	if err != nil {
		return nil, fmt.Errorf("load requests: %w", err)
	}

	var pending []ApprovalRequest
	for _, r := range reqs {
		if r.Status == StatusPending {
			pending = append(pending, r)
		}
	}
	return pending, nil
}

func (w *ApprovalWorkflow) filePath() string {
	return filepath.Join(w.dir, "approvals.jsonl")
}

func (w *ApprovalWorkflow) appendRequest(req *ApprovalRequest) error {
	f, err := os.OpenFile(w.filePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(req)
}

func (w *ApprovalWorkflow) loadAll() ([]ApprovalRequest, error) {
	f, err := os.Open(w.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var reqs []ApprovalRequest
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r ApprovalRequest
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("parse approval request: %w", err)
		}
		reqs = append(reqs, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read approval file: %w", err)
	}

	return reqs, nil
}

func (w *ApprovalWorkflow) rewriteAll(reqs []ApprovalRequest) error {
	f, err := os.OpenFile(w.filePath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, r := range reqs {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}
