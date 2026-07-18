package sectorallocation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// ---- snapshots & receipts ------------------------------------------------

// SectorAllocationSnapshot is the canonical view-model for a single
// simulation-closing sector allocation snapshot as defined in
// docs/specs/sector-allocation-simulation-closure-spec.md §4.4.
type SectorAllocationSnapshot struct {
	AsOfTradingDate   string                        `json:"as_of_trading_date"`
	EffectiveFrom     string                        `json:"effective_from"`
	Target            map[industry.SectorID]float64 `json:"target"`
	Current           map[industry.SectorID]float64 `json:"current"`
	Delta             map[industry.SectorID]float64 `json:"delta"`
	ModelVersion      string                        `json:"model_version"`
	CalibrationStatus string                        `json:"calibration_status"`
	WeightSource      string                        `json:"weight_source"`
	FallbackReason    string                        `json:"fallback_reason,omitempty"`
	Applied           bool                          `json:"applied"`
	// MutationReceipt is the store-returned receipt after a successful Store().
	MutationReceipt *MutationReceipt `json:"mutation_receipt,omitempty"`
}

// MutationReceipt proves that a snapshot was successfully persisted.
// It is returned by ClosureStore.Store and must be attached to the
// Applied=true response from ApplySectorRotation.
type MutationReceipt struct {
	ReceiptID string    `json:"receipt_id"`
	StoredAt  time.Time `json:"stored_at"`
	SHA256    string    `json:"sha256"`
}

// ConsumptionReceipt proves that a snapshot was consumed by the next
// trading session's budget allocator. It is stored alongside the
// consumed snapshot as an audit trail.
type ConsumptionReceipt struct {
	FromReceiptID string    `json:"from_receipt_id"`
	ConsumedAt    time.Time `json:"consumed_at"`
	SessionID     string    `json:"session_id,omitempty"`
}

// ---- ClosureStore ---------------------------------------------------------

// ClosureStore persists sector allocation snapshots between
// simulation sessions. The store is the single source of truth for
// next-session policy — no snapshot is considered "applied" until
// Store returns a MutationReceipt, and no allocation is produced
// until a ConsumptionReceipt is recorded.
type ClosureStore interface {
	// Store persists a snapshot and returns a receipt. If the snapshot
	// is empty, degraded, or missing required fields, Store returns an
	// error and no receipt (Applied must be false).
	Store(snap SectorAllocationSnapshot) (*MutationReceipt, error)

	// Latest returns the most recently stored (unconsumed) snapshot,
	// or nil if none exists.
	Latest() (*SectorAllocationSnapshot, error)

	// Consume marks a snapshot as consumed by recording a
	// ConsumptionReceipt. After Consume, Latest() should return the
	// next unconsumed snapshot (or nil).
	Consume(receiptID string, sessionID string) (*ConsumptionReceipt, error)

	// Delete removes a snapshot by receipt ID (rollback path).
	Delete(receiptID string) error
}

// ---- FileClosureStore -----------------------------------------------------

// FileClosureStore is the production implementation of ClosureStore.
// It stores snapshots as newline-delimited JSON in a single file.
type FileClosureStore struct {
	mu  sync.Mutex
	dir string
}

const closurePolicyFileName = "sector_closure_policy.jsonl"

// NewFileClosureStore creates a FileClosureStore rooted at dir.
func NewFileClosureStore(dir string) *FileClosureStore {
	return &FileClosureStore{dir: dir}
}

func (s *FileClosureStore) filePath() string {
	return filepath.Join(s.dir, closurePolicyFileName)
}

// Store appends a snapshot as a JSON line. It computes the SHA256 hash
// of the serialized snapshot body (excluding empty optional fields) and
// returns a MutationReceipt.
func (s *FileClosureStore) Store(snap SectorAllocationSnapshot) (*MutationReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateSnapshot(snap); err != nil {
		return nil, err
	}

	snap.Applied = true
	body, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(body))

	receipt := &MutationReceipt{
		ReceiptID: hash[:16],
		StoredAt:  time.Now(),
		SHA256:    hash,
	}
	snap.MutationReceipt = receipt
	snap.Applied = true

	line, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot line: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for closure store: %w", err)
	}

	f, err := os.OpenFile(s.filePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open closure store: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return nil, fmt.Errorf("write closure store: %w", err)
	}

	return receipt, nil
}

// Latest reads the most recent unconsumed snapshot from the file.
// Returns nil if the file does not exist or contains no unconsumed snapshots.
func (s *FileClosureStore) Latest() (*SectorAllocationSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.latestUnlocked()
}

func (s *FileClosureStore) latestUnlocked() (*SectorAllocationSnapshot, error) {
	rows, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	// Walk backwards to find the most recent unconsumed snapshot.
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].consumed {
			snap := rows[i].snap
			return &snap, nil
		}
	}
	return nil, nil
}

// Consume marks a snapshot as consumed by appending a consumption record
// as a JSON line. The consumption record references the original receipt.
func (s *FileClosureStore) Consume(receiptID string, sessionID string) (*ConsumptionReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cr := &ConsumptionReceipt{
		FromReceiptID: receiptID,
		ConsumedAt:    time.Now(),
		SessionID:     sessionID,
	}

	line, err := json.Marshal(struct {
		Type               string `json:"_type"`
		ConsumptionReceipt *ConsumptionReceipt
	}{
		Type:               "consumption",
		ConsumptionReceipt: cr,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal consumption: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for closure store: %w", err)
	}

	f, err := os.OpenFile(s.filePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open closure store: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return nil, fmt.Errorf("write consumption: %w", err)
	}

	return cr, nil
}

// Delete removes a snapshot by receipt. For the JSONL file, deletion is
// a soft operation: it appends a tombstone record. Rollback is supported
// by Consume → Store new snapshot in the opposite direction.
func (s *FileClosureStore) Delete(receiptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	line, _ := json.Marshal(struct {
		Type      string `json:"_type"`
		ReceiptID string `json:"receipt_id"`
		DeletedAt string `json:"deleted_at"`
	}{
		Type:      "tombstone",
		ReceiptID: receiptID,
		DeletedAt: time.Now().Format(time.RFC3339),
	})
	line = append(line, '\n')

	f, err := os.OpenFile(s.filePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open closure store for delete: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(line)
	return err
}

// ---- SnapshotReader -------------------------------------------------------

// SnapshotReader is the read-only interface that SA09 handlers consume.
// SA08 provides the production implementation (FileClosureStore).
type SnapshotReader interface {
	LatestSnapshot() *SectorAllocationSnapshot
}

// LatestSnapshot implements SnapshotReader for FileClosureStore.
// Returns nil when no snapshot is available (SA09 handlers degrade gracefully).
func (s *FileClosureStore) LatestSnapshot() *SectorAllocationSnapshot {
	snap, err := s.Latest()
	if err != nil || snap == nil {
		return nil
	}
	return snap
}

// ---- internal helpers ----------------------------------------------------

type storedRow struct {
	snap     SectorAllocationSnapshot
	consumed bool
}

func (s *FileClosureStore) readAllLocked() ([]storedRow, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := splitLines(data)

	// Pass 1: collect all consumption receipts and tombstones.
	consumed := map[string]bool{}
	tombstoned := map[string]bool{}

	for _, raw := range lines {
		if len(raw) == 0 {
			continue
		}
		var typeCheck struct {
			Type string `json:"_type"`
		}
		if err := json.Unmarshal(raw, &typeCheck); err != nil {
			continue
		}
		switch typeCheck.Type {
		case "consumption":
			var cr struct {
				ConsumptionReceipt *ConsumptionReceipt `json:"ConsumptionReceipt"`
			}
			if err := json.Unmarshal(raw, &cr); err == nil && cr.ConsumptionReceipt != nil {
				consumed[cr.ConsumptionReceipt.FromReceiptID] = true
			}
		case "tombstone":
			var ts struct {
				ReceiptID string `json:"receipt_id"`
			}
			if err := json.Unmarshal(raw, &ts); err == nil && ts.ReceiptID != "" {
				tombstoned[ts.ReceiptID] = true
			}
		}
	}

	// Pass 2: collect snapshot rows, checking consumed/tombstoned status.
	var rows []storedRow
	for _, raw := range lines {
		if len(raw) == 0 {
			continue
		}
		// Skip metadata lines (consumption, tombstone) — already processed.
		var typeCheck struct {
			Type string `json:"_type"`
		}
		if err := json.Unmarshal(raw, &typeCheck); err == nil && typeCheck.Type != "" {
			continue
		}
		var snap SectorAllocationSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}
		if snap.MutationReceipt == nil {
			continue
		}
		isConsumed := consumed[snap.MutationReceipt.ReceiptID] || tombstoned[snap.MutationReceipt.ReceiptID]
		rows = append(rows, storedRow{
			snap:     snap,
			consumed: isConsumed,
		})
	}
	return rows, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func validateSnapshot(snap SectorAllocationSnapshot) error {
	if snap.AsOfTradingDate == "" {
		return fmt.Errorf("missing as_of_trading_date")
	}
	if snap.EffectiveFrom == "" {
		return fmt.Errorf("missing effective_from")
	}
	if len(snap.Target) == 0 {
		return fmt.Errorf("empty target allocations")
	}
	if snap.ModelVersion == "" {
		return fmt.Errorf("missing model_version")
	}
	return nil
}
