package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Q2 batch 2 unit A — experiments 家族 store 的 hermetic 測試（不需 PostgreSQL）。
//
// 覆蓋四件事：
//  1. 後端分派（jsonl ⇒ 單一 store；postgres 無注入池 ⇒ 明確錯誤，不降級）；
//  2. result 半的鏡射（後端 ＋ 檔案）；後端寫入失敗時**不得**留下檔案；
//  3. LoadTrialSharpes 的轉發 —— 少了它 DSR gate 會靜默停用（負向對照在下方）；
//  4. LoadExperimentRecords 的順序／折疊／錯誤語意（不可靜默回退檔案）。

// recordingFamilyStore 只實作被測的幾個方法；其餘 FullStore 方法由內嵌介面
// 滿足（呼叫到會 panic，正好證明測試沒有意外走到未覆蓋的路徑）。
type recordingFamilyStore struct {
	FullStore
	results     []string
	updates     []string
	records     []domain.ExperimentRecord
	trials      []TrialSharpe
	resultErr   error
	loadErr     error
	recordCalls int
}

func (s *recordingFamilyStore) RecordPromptExperimentResult(id string, r domain.PromptExperimentResult) error {
	if s.resultErr != nil {
		return s.resultErr
	}
	s.results = append(s.results, id)
	return nil
}

func (s *recordingFamilyStore) UpdatePromptExperimentResult(id string, r domain.PromptExperimentResult) error {
	if s.resultErr != nil {
		return s.resultErr
	}
	s.updates = append(s.updates, id)
	return nil
}

func (s *recordingFamilyStore) RecordExperiment(rec domain.ExperimentRecord) error {
	s.recordCalls++
	s.records = append(s.records, rec)
	return nil
}

func (s *recordingFamilyStore) LoadExperiments() ([]domain.ExperimentRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.records, nil
}

func (s *recordingFamilyStore) LoadTrialSharpes() ([]TrialSharpe, error) {
	return s.trials, nil
}

func familyTestResult(id string) domain.PromptExperimentResult {
	return domain.PromptExperimentResult{
		Experiment:      domain.ExperimentRecord{ID: id},
		CandidatePrompt: "v1",
	}
}

// TestNewExperimentFamilyStore_JSONLWritesBothHalvesIntoOneStore pins the
// degenerate case: on the JSONL backend the result mirror must NOT be layered
// on top (both halves already live in LedgerDir), so the returned store is the
// JSONL store itself and a result write produces exactly one file.
func TestNewExperimentFamilyStore_JSONLWritesBothHalvesIntoOneStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewExperimentFamilyStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewExperimentFamilyStore: %v", err)
	}
	if _, ok := store.(*Store); !ok {
		t.Fatalf("jsonl backend returned %T, want *Store (no redundant mirror)", store)
	}

	if err := store.RecordExperiment(domain.ExperimentRecord{ID: "exp-jsonl-1", Status: domain.ExperimentPlanned}); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}
	if err := store.RecordPromptExperimentResult("exp-jsonl-1", familyTestResult("exp-jsonl-1")); err != nil {
		t.Fatalf("RecordPromptExperimentResult: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "experiments.jsonl")); err != nil {
		t.Fatalf("experiments.jsonl missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "experiments", "exp-jsonl-1.json")); err != nil {
		t.Fatalf("experiments/exp-jsonl-1.json missing: %v", err)
	}
}

// TestNewExperimentFamilyStore_EmptyBackendBehavesLikeJSONL is the parity test
// for the pre-migration behavior: an unset StoreBackend used to reach
// ledger.NewStore(LedgerDir) directly, so it must still resolve to the same
// JSONL directory (the factory's documented default, warned about at runtime).
func TestNewExperimentFamilyStore_EmptyBackendBehavesLikeJSONL(t *testing.T) {
	dir := t.TempDir()
	store, err := NewExperimentFamilyStore(config.Config{LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewExperimentFamilyStore: %v", err)
	}
	if err := store.RecordExperiment(domain.ExperimentRecord{ID: "exp-default-1"}); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("experiments.jsonl missing: %v", err)
	}
	if !strings.Contains(string(data), "exp-default-1") {
		t.Fatalf("record not in the JSONL ledger: %s", data)
	}
}

// TestNewExperimentFamilyStore_PostgresWithoutPoolFailsLoudly is the guard
// against the silent-degradation failure mode: a postgres deployment that never
// injected a pool must NOT be handed a JSONL store.
func TestNewExperimentFamilyStore_PostgresWithoutPoolFailsLoudly(t *testing.T) {
	saved := postgresPool
	SetPostgresPool(nil)
	t.Cleanup(func() { SetPostgresPool(saved) })

	store, err := NewExperimentFamilyStore(config.Config{StoreBackend: "postgres", LedgerDir: t.TempDir()})
	if err == nil {
		t.Fatalf("postgres without a pool must fail, got store %T", store)
	}
	if !strings.Contains(err.Error(), "SetPostgresPool") {
		t.Fatalf("error must name the missing injection, got: %v", err)
	}
}

// TestNewExperimentFamilyStore_UnknownBackendFailsLoudly pins that a typo in
// ATLAS_STORE_BACKEND is not silently treated as jsonl.
func TestNewExperimentFamilyStore_UnknownBackendFailsLoudly(t *testing.T) {
	if _, err := NewExperimentFamilyStore(config.Config{StoreBackend: "postgre", LedgerDir: t.TempDir()}); err == nil {
		t.Fatal("unknown backend must fail")
	}
}

// TestResultMirror_WritesPrimaryThenFile pins the two-write contract and its
// ordering: the backend write is authoritative (its error is returned), and the
// file is only updated after it succeeded.
func TestResultMirror_WritesPrimaryThenFile(t *testing.T) {
	dir := t.TempDir()
	files, err := NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	primary := &recordingFamilyStore{}
	store := resultMirror{FullStore: primary, files: files}

	if err := store.RecordPromptExperimentResult("exp-mirror-1", familyTestResult("exp-mirror-1")); err != nil {
		t.Fatalf("RecordPromptExperimentResult: %v", err)
	}
	if err := store.UpdatePromptExperimentResult("exp-mirror-1", familyTestResult("exp-mirror-1")); err != nil {
		t.Fatalf("UpdatePromptExperimentResult: %v", err)
	}

	if len(primary.results) != 1 || len(primary.updates) != 1 {
		t.Fatalf("primary writes: results=%v updates=%v, want one each", primary.results, primary.updates)
	}
	if _, err := os.Stat(filepath.Join(dir, "experiments", "exp-mirror-1.json")); err != nil {
		t.Fatalf("mirrored result file missing: %v", err)
	}
}

// TestResultMirror_PrimaryFailureLeavesNoFile is the negative half: if the
// authoritative backend write fails, the mirror must not run — a file that
// exists while the backend has nothing is exactly the divergence the unit
// removes.
func TestResultMirror_PrimaryFailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	files, err := NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	primary := &recordingFamilyStore{resultErr: fmt.Errorf("pg down")}
	store := resultMirror{FullStore: primary, files: files}

	if err := store.RecordPromptExperimentResult("exp-mirror-2", familyTestResult("exp-mirror-2")); err == nil {
		t.Fatal("the primary error must surface")
	}
	if _, err := os.Stat(filepath.Join(dir, "experiments", "exp-mirror-2.json")); !os.IsNotExist(err) {
		t.Fatalf("no file may be written when the primary write failed (stat err = %v)", err)
	}
}

// TestResultMirror_RecordsGoToThePrimaryOnly pins that the record/status half
// is NOT mirrored: the backend is its single source.
func TestResultMirror_RecordsGoToThePrimaryOnly(t *testing.T) {
	dir := t.TempDir()
	files, err := NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	primary := &recordingFamilyStore{}
	store := resultMirror{FullStore: primary, files: files}

	if err := store.RecordExperiment(domain.ExperimentRecord{ID: "exp-mirror-3"}); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}
	if primary.recordCalls != 1 {
		t.Fatalf("primary RecordExperiment calls = %d, want 1", primary.recordCalls)
	}
	if _, err := os.Stat(filepath.Join(dir, "experiments.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("the record half must not be mirrored to the JSONL file (stat err = %v)", err)
	}
}

// TestResultMirror_KeepsTrialSharpeLoader is the DSR-gate pin.
//
// The gate looks the loader up with a type assertion on the judge's store
// (internal/experiment/dsr.go). A wrapper that merely embedded FullStore would
// fail that assertion and silently disable the anti-luck gate for its whole
// life — the 2026-10-12 wiring hole documented on Store.LoadTrialSharpes.
func TestResultMirror_KeepsTrialSharpeLoader(t *testing.T) {
	primary := &recordingFamilyStore{trials: []TrialSharpe{{ExperimentID: "exp-dsr", Sharpe: 1.5}}}
	store := resultMirror{FullStore: primary}

	loader, ok := any(store).(TrialSharpeLoader)
	if !ok {
		t.Fatal("the family store must still satisfy ledger.TrialSharpeLoader")
	}
	trials, err := loader.LoadTrialSharpes()
	if err != nil {
		t.Fatalf("LoadTrialSharpes: %v", err)
	}
	if len(trials) != 1 || trials[0].Sharpe != 1.5 {
		t.Fatalf("trials = %+v, want the primary's", trials)
	}

	// Negative control: embedding alone loses the capability. If this ever
	// starts satisfying the interface, the forwarding method above is dead code
	// and the test above no longer proves anything.
	bare := struct{ FullStore }{FullStore: primary}
	if _, ok := any(bare).(TrialSharpeLoader); ok {
		t.Fatal("a wrapper without the explicit forwarding method must NOT satisfy TrialSharpeLoader")
	}
}

// TestLoadExperimentRecords_StorePath is the read-side parity/replacement test:
// the store's stream (here in newest-first order, the shape SQLite and
// PostgreSQL return) must be folded into the same oldest-first order the file
// reader produced.
func TestLoadExperimentRecords_StorePath(t *testing.T) {
	// newest first, exactly as ORDER BY id DESC returns it (one row per
	// experiment_id — the column is UNIQUE on both relational backends).
	store := &recordingFamilyStore{records: []domain.ExperimentRecord{
		{ID: "exp-c-1700000003", Status: domain.ExperimentPlanned},
		{ID: "exp-b-1700000002", Status: domain.ExperimentAccepted},
		{ID: "exp-a-1700000001", Status: domain.ExperimentPlanned},
	}}

	records, err := LoadExperimentRecords(store, t.TempDir())
	if err != nil {
		t.Fatalf("LoadExperimentRecords: %v", err)
	}
	want := []string{"exp-a-1700000001", "exp-b-1700000002", "exp-c-1700000003"}
	if len(records) != len(want) {
		t.Fatalf("got %d records, want %d", len(records), len(want))
	}
	for i, id := range want {
		if records[i].ID != id {
			t.Fatalf("record[%d] = %s, want %s (oldest-first is backend-independent)", i, records[i].ID, id)
		}
	}
	// Selection semantics used by the drain: the oldest planned candidate.
	if oldest := firstPlannedID(records); oldest != "exp-a-1700000001" {
		t.Fatalf("oldest planned = %s, want exp-a-1700000001", oldest)
	}
}

// firstPlannedID mirrors what loadOldestPendingExperiment selects: the first
// record of an unresolved ("planned") id in the canonical stream order.
func firstPlannedID(records []domain.ExperimentRecord) string {
	latest := LatestExperimentStatusByID(records)
	for _, rec := range records {
		if rec.Status == domain.ExperimentPlanned && latest[rec.ID] == domain.ExperimentPlanned {
			return rec.ID
		}
	}
	return ""
}

// TestLoadExperimentRecords_FilePathParity proves the store path and the file
// path answer the same question on the same data.
func TestLoadExperimentRecords_FilePathParity(t *testing.T) {
	dir := t.TempDir()
	lines := []domain.ExperimentRecord{
		{ID: "exp-2-1700000002", TargetAgentID: "b", Status: domain.ExperimentPlanned},
		{ID: "exp-1-1700000001", TargetAgentID: "a", Status: domain.ExperimentPlanned},
		{ID: "exp-1-1700000001", TargetAgentID: "a", Status: domain.ExperimentAccepted},
	}
	var buf strings.Builder
	for _, rec := range lines {
		data, _ := json.Marshal(rec)
		buf.Write(data)
		buf.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	fromFile, err := LoadExperimentRecords(nil, dir)
	if err != nil {
		t.Fatalf("file path: %v", err)
	}
	fileStore, err := NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	fromStore, err := LoadExperimentRecords(fileStore, dir)
	if err != nil {
		t.Fatalf("store path: %v", err)
	}

	if len(fromFile) != len(fromStore) {
		t.Fatalf("length mismatch: file=%d store=%d", len(fromFile), len(fromStore))
	}
	for i := range fromFile {
		gotFile, _ := json.Marshal(fromFile[i])
		gotStore, _ := json.Marshal(fromStore[i])
		if string(gotFile) != string(gotStore) {
			t.Fatalf("record[%d] differs: file=%s store=%s", i, gotFile, gotStore)
		}
	}
	if CountUnresolvedPlannedIn(fromFile) != CountUnresolvedPlanned(dir) {
		t.Fatalf("CountUnresolvedPlannedIn = %d, CountUnresolvedPlanned = %d",
			CountUnresolvedPlannedIn(fromFile), CountUnresolvedPlanned(dir))
	}
}

// TestLoadExperimentRecords_StoreErrorIsNotAnsweredFromFile is the negative
// proof for the silent-staleness rule: when the store is the caller's source,
// a store failure must be returned rather than papered over with the frozen
// JSONL file.
func TestLoadExperimentRecords_StoreErrorIsNotAnsweredFromFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "experiments.jsonl"),
		[]byte(`{"id":"exp-stale-1700000001","status":"planned"}`+"\n"), 0o644,
	); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	store := &recordingFamilyStore{loadErr: fmt.Errorf("pg unreachable")}
	records, err := LoadExperimentRecords(store, dir)
	if err == nil {
		t.Fatalf("store failure must surface, got %d records from the stale file", len(records))
	}
	if records != nil {
		t.Fatalf("no fallback records may be returned, got %+v", records)
	}
}
