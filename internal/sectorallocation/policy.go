package sectorallocation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

	// Applied is the OUTWARD application status (spec §8.3). It is true only
	// when consumption evidence exists for this snapshot, i.e. Consumption is
	// non-nil. Persisting a snapshot is NOT application: "一份只供展示、未被
	// allocation/order-sizing path 消費的 snapshot 不算 applied". The store
	// therefore never hard-writes this field; see DecorateApplicationStatus.
	Applied bool `json:"applied"`
	// Consumption is the evidence that a downstream consumer read this
	// snapshot before producing orders. nil = no consumption evidence.
	Consumption *ConsumptionReceipt `json:"consumption,omitempty"`
	// FallbackReason is the machine-readable reason the snapshot is NOT in
	// effect, using the spec §9 vocabulary (allocator_unavailable,
	// pending_consumption, no_simulation_session, ...). Empty when Applied.
	FallbackReason string `json:"fallback_reason,omitempty"`
	// TargetNote records target-COMPUTATION degradation ("no weight engine",
	// "projection failed: ..."). It is provenance for how Target was derived,
	// not application status, and therefore never drives Applied.
	TargetNote string `json:"target_note,omitempty"`

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
// next-session policy.
//
// Evidence ladder (spec §8.3, enforced since issue #1944 Batch 1):
//
//	Store()          -> MutationReceipt  = persisted, NOT applied
//	Consume()        -> ConsumptionReceipt = consumed by an allocator
//	                                            => the only evidence that
//	                                            allows Applied=true
//
// A persisted snapshot with no consumer is therefore reported as
// applied=false with FallbackReason=allocator_unavailable.
type ClosureStore interface {
	// Store persists a snapshot and returns a receipt. If the snapshot
	// is empty, degraded, or missing required fields, Store returns an
	// error and no receipt (Applied must be false).
	Store(snap SectorAllocationSnapshot) (*MutationReceipt, error)

	// Latest returns the most recently stored snapshot that is still eligible
	// as next-session policy — i.e. neither consumed nor deleted — or nil if
	// none exists. The returned snapshot carries derived status
	// (DecorateApplicationStatus), never the raw stored `applied` value.
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

// closureStoreDirSegments is the work-dir-relative location of the closure
// policy store: <work_dir>/data/sector/allocation. It lives under the
// persistent data/ tree (not data/state) so the file survives container
// restarts together with the rest of the persistent mount.
var closureStoreDirSegments = []string{"data", "sector", "allocation"}

// ClosureStoreDirRel returns the work-dir-relative directory of the closure
// policy store.
//
// It is the single source of truth for that location (issue #1944 N-A4). Every
// writer and reader MUST resolve its directory through this helper or through
// ResolveClosureStoreDir/ResolveClosureStorePath, so external tooling cannot end
// up probing a path nothing writes. The regression this replaced: the closure
// preflight CLI validated <work_dir>/data/state/sector_closure_policy.jsonl,
// while the production store was created at <work_dir>/data/sector/allocation.
func ClosureStoreDirRel() string {
	return filepath.Join(closureStoreDirSegments...)
}

// ResolveClosureStoreDir returns the closure-store directory for a work dir.
// Construct the store with
// NewFileClosureStore(ResolveClosureStoreDir(workDir)).
func ResolveClosureStoreDir(workDir string) string {
	return filepath.Join(workDir, ClosureStoreDirRel())
}

// ResolveClosureStorePath returns the full path of the closure policy file for a
// work dir — exactly the file FileClosureStore reads from and writes to.
func ResolveClosureStorePath(workDir string) string {
	return filepath.Join(ResolveClosureStoreDir(workDir), closurePolicyFileName)
}

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

	// NOTE (issue #1944 Batch 1): persisting is a mutation of the store, not
	// evidence that the policy changed any allocation (spec §8.3). The stored
	// record therefore always carries applied=false; the outward value is
	// derived from consumption evidence at read time
	// (DecorateApplicationStatus). Normalising here keeps the raw JSONL from
	// overclaiming even when a caller hands in Applied=true.
	snap.Applied = false
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
	// Walk backwards to find the most recent unconsumed, non-deleted snapshot.
	for _, row := range slices.Backward(rows) {
		if !row.consumedForAllocator() {
			return DecorateApplicationStatus(row.snap), nil
		}
	}
	return nil, nil
}

// newestUnlocked returns the most recently stored snapshot that was not rolled
// back, together with its consumption evidence. The outward reader
// (LatestSnapshot / dashboard) needs the newest plan *and* whether it was
// consumed, so — unlike Latest() — consumed rows are included; tombstoned
// (deleted) rows are not.
func (s *FileClosureStore) newestUnlocked() (*SectorAllocationSnapshot, error) {
	rows, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	for _, row := range slices.Backward(rows) {
		if row.deleted {
			continue
		}
		snap := row.snap
		snap.Consumption = row.consumption
		return &snap, nil
	}
	return nil, nil
}

// ConsumptionFor returns the consumption receipt recorded for receiptID, or
// nil when nothing consumed that receipt (or the store cannot answer).
// It implements ConsumptionReader.
func (s *FileClosureStore) ConsumptionFor(receiptID string) *ConsumptionReceipt {
	if receiptID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readAllLocked()
	if err != nil {
		return nil
	}
	for _, row := range rows {
		if row.snap.MutationReceipt != nil && row.snap.MutationReceipt.ReceiptID == receiptID {
			return row.consumption
		}
	}
	return nil
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
//
// The returned snapshot is the NEWEST stored one (consumed or not) and its
// Applied/FallbackReason fields are derived from consumption evidence rather
// than from the stored JSON (issue #1944 Batch 1).
func (s *FileClosureStore) LatestSnapshot() *SectorAllocationSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.newestUnlocked()
	if err != nil || snap == nil {
		return nil
	}
	return DecorateApplicationStatus(*snap)
}

// ---- application status (consumption evidence) ----------------------------

// Machine-readable application reasons. Vocabulary follows
// docs/specs/sector-allocation-simulation-closure-spec.md §9 (subset relevant
// to the allocation plan surface).
const (
	// FallbackNoSimulationSession: no snapshot has been stored yet.
	FallbackNoSimulationSession = "no_simulation_session"
	// FallbackSnapshotUnavailable: the snapshot reader itself is unavailable.
	FallbackSnapshotUnavailable = "snapshot_unavailable"
	// FallbackAllocatorUnavailable: a snapshot is persisted but no allocator /
	// order-sizing consumer is wired, so per spec §8.3 it is NOT applied.
	FallbackAllocatorUnavailable = "allocator_unavailable"
	// FallbackPendingConsumption: a consumer is wired but has not recorded a
	// ConsumptionReceipt for this snapshot yet.
	FallbackPendingConsumption = "pending_consumption"
)

// ConsumptionReader is implemented by stores that can report whether a
// specific receipt was consumed.
type ConsumptionReader interface {
	ConsumptionFor(receiptID string) *ConsumptionReceipt
}

var (
	policyConsumerMu sync.RWMutex
	policyConsumers  []string
)

// RegisterPolicyConsumer announces a downstream consumer that reads a stored
// sector allocation policy before producing orders (the
// portfolio.SectorBudgetAllocator path, or any successor). Until at least one
// consumer is registered, snapshots are reported as
// applied=false / fallback_reason=allocator_unavailable instead of claiming to
// be in effect (spec §8.3; issue #1944 Batch 1).
//
// Wiring code must call this where the consumer is constructed, so the
// outward status stays evidence-driven rather than hard-coded.
func RegisterPolicyConsumer(label string) {
	if label == "" {
		return
	}
	policyConsumerMu.Lock()
	defer policyConsumerMu.Unlock()
	for _, c := range policyConsumers {
		if c == label {
			return
		}
	}
	policyConsumers = append(policyConsumers, label)
}

// ResetPolicyConsumers clears the registry. Wiring owners call it on
// teardown/rollback (and tests use it between cases): registration is a
// process-global fact, so a removed consumer must be able to restore the
// honest default (applied=false + allocator_unavailable) instead of leaving a
// stale "a consumer exists" claim behind.
func ResetPolicyConsumers() {
	policyConsumerMu.Lock()
	defer policyConsumerMu.Unlock()
	policyConsumers = nil
}

// RegisteredPolicyConsumers returns the labels registered so far.
func RegisteredPolicyConsumers() []string {
	policyConsumerMu.RLock()
	defer policyConsumerMu.RUnlock()
	out := make([]string, len(policyConsumers))
	copy(out, policyConsumers)
	return out
}

// PolicyConsumerWired reports whether any policy consumer announced itself.
func PolicyConsumerWired() bool {
	return len(RegisteredPolicyConsumers()) > 0
}

// ApplicationStatusFor derives (applied, reason) from consumption evidence.
// applied is true ONLY when a consumption receipt exists (spec §8.3).
func ApplicationStatusFor(consumption *ConsumptionReceipt) (bool, string) {
	if consumption != nil {
		return true, ""
	}
	if !PolicyConsumerWired() {
		return false, FallbackAllocatorUnavailable
	}
	return false, FallbackPendingConsumption
}

// ConsumptionFor returns the consumption receipt recorded for receiptID by the
// given store, or nil when the store cannot answer or nothing consumed it.
func ConsumptionFor(store ClosureStore, receiptID string) *ConsumptionReceipt {
	reader, ok := store.(ConsumptionReader)
	if !ok || receiptID == "" {
		return nil
	}
	return reader.ConsumptionFor(receiptID)
}

// DecorateApplicationStatus returns a COPY of snap with Applied and
// FallbackReason derived from consumption evidence (snap.Consumption), and with
// legacy target notes migrated (see migrateLegacyTargetNote).
//
// It is pure: the input is never mutated, so it is safe to apply both in the
// store reader and in the HTTP handler, and safe when a SnapshotReader hands
// out a shared/cached pointer (no cross-request mutation).
//
// Pre-condition: none. Post-condition: result.Applied == (Consumption != nil).
func DecorateApplicationStatus(snap SectorAllocationSnapshot) *SectorAllocationSnapshot {
	out := snap
	migrateLegacyTargetNote(&out)
	applied, reason := ApplicationStatusFor(out.Consumption)
	out.Applied = applied
	if applied {
		out.FallbackReason = ""
	} else {
		out.FallbackReason = reason
	}
	return &out
}

// legacyStatusReasons are the values FallbackReason may legitimately hold after
// decoration; anything else is a pre-#1944 target-computation note that used to
// be stored in this field.
var legacyStatusReasons = map[string]bool{
	"":                           true,
	FallbackAllocatorUnavailable: true,
	FallbackPendingConsumption:   true,
	FallbackNoSimulationSession:  true,
	FallbackSnapshotUnavailable:  true,
}

// migrateLegacyTargetNote moves a pre-#1944 target-computation note out of
// FallbackReason (where Store used to put "no weight engine" /
// "projection failed: ...") into TargetNote, so existing JSONL rows keep their
// provenance now that FallbackReason is status-only.
func migrateLegacyTargetNote(snap *SectorAllocationSnapshot) {
	if snap == nil || snap.TargetNote != "" || legacyStatusReasons[snap.FallbackReason] {
		return
	}
	snap.TargetNote = snap.FallbackReason
	snap.FallbackReason = ""
}

// ---- internal helpers ----------------------------------------------------

type storedRow struct {
	snap SectorAllocationSnapshot
	// consumption is the ConsumptionReceipt that marked this row consumed
	// (nil when nothing consumed it). It is the evidence Applied is derived
	// from — see DecorateApplicationStatus.
	consumption *ConsumptionReceipt
	// deleted marks a tombstoned row (Delete = rollback path). A deleted
	// snapshot must never be served again: not by Latest() (the allocator
	// would resurrect a rolled-back policy) and not by LatestSnapshot()
	// (the dashboard would show it as the current plan).
	deleted bool
}

// consumedForAllocator reports whether this row is no longer eligible as
// "next-session policy" for Latest(): it was consumed already, or it was
// rolled back.
func (r storedRow) consumedForAllocator() bool { return r.consumption != nil || r.deleted }

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
	consumed := map[string]*ConsumptionReceipt{}
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
				consumed[cr.ConsumptionReceipt.FromReceiptID] = cr.ConsumptionReceipt
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
		row := storedRow{snap: snap}
		row.deleted = tombstoned[snap.MutationReceipt.ReceiptID]
		// A deleted (rolled-back) snapshot is not consumption evidence, even
		// when a consumption record exists for it.
		if c := consumed[snap.MutationReceipt.ReceiptID]; c != nil && !row.deleted {
			row.consumption = c
		}
		rows = append(rows, row)
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
