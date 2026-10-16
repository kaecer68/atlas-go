package experiment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Q2 batch 2 unit A：experiments 家族的寫者統一（hermetic）。
//
// 這一組測試釘住三件事：
//  1. 注入 store ⇒ pipeline（expiry / pending 選擇）真的走它；
//  2. nil ⇒ 顯式 JSONL 後端，與遷移前逐字相同（parity：同一 fixture 兩條路徑
//     產生同樣的過期集合與同樣的檔案內容）；
//  3. 排序：store 端回傳的是「新到舊」（SQLite／PG 的 ORDER BY id DESC），
//     選 candidate 的語意是「最舊的 planned」⇒ 順序錯了會選到最新的。

// memoryFamilyStore 是只存在記憶體裡的 family store；未覆寫的方法由內嵌介面
// 滿足（呼叫到會 panic ⇒ 測試沒有意外走到未覆蓋的路徑）。
type memoryFamilyStore struct {
	ledger.FullStore
	records []domain.ExperimentRecord
	written []domain.ExperimentRecord
	loadErr error
}

func (s *memoryFamilyStore) LoadExperiments() ([]domain.ExperimentRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.records, nil
}

func (s *memoryFamilyStore) RecordExperiment(rec domain.ExperimentRecord) error {
	s.written = append(s.written, rec)
	return nil
}

func expTestDir(t *testing.T, records ...domain.ExperimentRecord) string {
	t.Helper()
	dir := t.TempDir()
	var buf strings.Builder
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal fixture record: %v", err)
		}
		buf.Write(data)
		buf.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

func stalePlannedFixture() []domain.ExperimentRecord {
	old := time.Now().Add(-60 * 24 * time.Hour).Unix()
	fresh := time.Now().Unix()
	return []domain.ExperimentRecord{
		{ID: fmt.Sprintf("exp-old-%d", old), TargetAgentID: "a", Status: domain.ExperimentPlanned},
		{ID: fmt.Sprintf("exp-fresh-%d", fresh), TargetAgentID: "b", Status: domain.ExperimentPlanned},
		{ID: "exp-done-1700000003", TargetAgentID: "c", Status: domain.ExperimentPlanned},
		{ID: "exp-done-1700000003", TargetAgentID: "c", Status: domain.ExperimentAccepted},
	}
}

// TestExpireStalePlannedWithStore_WritesThroughTheInjectedStore pins the writer
// half: the expiry transition must be appended to the store the pipeline reads,
// not to a JSONL file the store-backed reader never consults.
func TestExpireStalePlannedWithStore_WritesThroughTheInjectedStore(t *testing.T) {
	// Newest first, the order the relational backends return.
	records := stalePlannedFixture()
	store := &memoryFamilyStore{records: []domain.ExperimentRecord{
		records[2], records[3], records[1], records[0],
	}}

	n, err := ExpireStalePlannedWithStore(store, t.TempDir(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStalePlannedWithStore: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d, want 1", n)
	}
	if len(store.written) != 1 {
		t.Fatalf("store received %d writes, want 1", len(store.written))
	}
	got := store.written[0]
	if got.Status != domain.ExperimentExpired || !strings.HasPrefix(got.ID, "exp-old-") {
		t.Fatalf("expired record = %+v, want the 60-day-old planned record marked expired", got)
	}
}

// TestExpireStalePlannedWithStore_ParityWithFilePath is the parity test: on the
// same fixture the store path and the file path must expire the SAME records.
func TestExpireStalePlannedWithStore_ParityWithFilePath(t *testing.T) {
	records := stalePlannedFixture()

	// File path: the pre-migration call (nil store) over experiments.jsonl.
	fileDir := expTestDir(t, records...)
	nFile, err := ExpireStalePlanned(fileDir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStalePlanned(file): %v", err)
	}

	// Store path: the same data behind a JSONL-backed store.
	storeDir := expTestDir(t, records...)
	jsonlStore, err := ledger.NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: storeDir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	nStore, err := ExpireStalePlannedWithStore(jsonlStore, storeDir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStalePlannedWithStore: %v", err)
	}

	if nFile != nStore {
		t.Fatalf("expired counts differ: file=%d store=%d", nFile, nStore)
	}
	fileBytes, err := os.ReadFile(filepath.Join(fileDir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read file ledger: %v", err)
	}
	storeBytes, err := os.ReadFile(filepath.Join(storeDir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read store ledger: %v", err)
	}
	if string(fileBytes) != string(storeBytes) {
		t.Fatalf("ledgers diverge:\nfile:  %s\nstore: %s", fileBytes, storeBytes)
	}
}

// TestExpireStalePlannedWithStore_StoreReadErrorIsReturned is the negative
// proof against silently answering from the frozen file when the store (the
// authoritative source on a postgres deployment) is unreachable.
func TestExpireStalePlannedWithStore_StoreReadErrorIsReturned(t *testing.T) {
	dir := expTestDir(t, stalePlannedFixture()...)
	store := &memoryFamilyStore{loadErr: errors.New("pg down")}

	n, err := ExpireStalePlannedWithStore(store, dir, 30*24*time.Hour)
	if err == nil {
		t.Fatalf("store read failure must surface, got n=%d", n)
	}
	if len(store.written) != 0 {
		t.Fatalf("nothing may be written after a failed read: %+v", store.written)
	}
}

// TestLoadOldestPendingExperiment_StorePathPicksTheOldestPlanned is the ordering
// regression: the relational stores return newest-first, so a naive "first
// planned match" would pick the newest candidate and starve the oldest pending
// experiment forever.
func TestLoadOldestPendingExperiment_StorePathPicksTheOldestPlanned(t *testing.T) {
	records := []domain.ExperimentRecord{
		{ID: "exp-newest-1700000003", TargetAgentID: "c", Status: domain.ExperimentPlanned},
		{ID: "exp-middle-1700000002", TargetAgentID: "b", Status: domain.ExperimentPlanned},
		{ID: "exp-oldest-1700000001", TargetAgentID: "a", Status: domain.ExperimentPlanned},
	}
	store := &memoryFamilyStore{records: records}

	pending, err := loadOldestPendingExperiment(store, t.TempDir())
	if err != nil {
		t.Fatalf("loadOldestPendingExperiment: %v", err)
	}
	if pending == nil || pending.ID != "exp-oldest-1700000001" {
		t.Fatalf("pending = %+v, want exp-oldest-1700000001", pending)
	}

	// Control: the fixture really does contain the newer records, so the
	// assertion above is not satisfied by "the only one there".
	if len(store.records) != 3 {
		t.Fatalf("fixture lost records: %d", len(store.records))
	}
}

// TestLoadOldestPendingExperiment_FileParity checks the store path and the
// pre-migration file path agree on the same data.
func TestLoadOldestPendingExperiment_FileParity(t *testing.T) {
	records := []domain.ExperimentRecord{
		{ID: "exp-a-1700000001", TargetAgentID: "a", Status: domain.ExperimentPlanned},
		{ID: "exp-a-1700000001", TargetAgentID: "a", Status: domain.ExperimentRejected},
		{ID: "exp-b-1700000002", TargetAgentID: "b", Status: domain.ExperimentPlanned},
	}
	dir := expTestDir(t, records...)

	fromFile, err := loadOldestPendingExperiment(nil, dir)
	if err != nil {
		t.Fatalf("file path: %v", err)
	}
	jsonlStore, err := ledger.NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	fromStore, err := loadOldestPendingExperiment(jsonlStore, dir)
	if err != nil {
		t.Fatalf("store path: %v", err)
	}

	if fromFile == nil || fromStore == nil {
		t.Fatalf("one path found nothing: file=%+v store=%+v", fromFile, fromStore)
	}
	if *fromFile != *fromStore {
		t.Fatalf("paths disagree: file=%+v store=%+v", *fromFile, *fromStore)
	}
	if fromFile.ID != "exp-b-1700000002" {
		t.Fatalf("pending = %s, want exp-b-1700000002", fromFile.ID)
	}
}

// TestResolveAutoExperimentStore_InjectedAndDefault pins the seam itself:
// injected ⇒ the exact instance; nil ⇒ JSONL behavior over Config.LedgerDir.
func TestResolveAutoExperimentStore_InjectedAndDefault(t *testing.T) {
	injected := &memoryFamilyStore{}
	got, err := resolveAutoExperimentStore(AutoExperimentConfig{Store: injected})
	if err != nil {
		t.Fatalf("resolveAutoExperimentStore(injected): %v", err)
	}
	if ledger.FullStore(got) != ledger.FullStore(injected) {
		t.Fatalf("injected store was not used: %T", got)
	}

	dir := t.TempDir()
	got, err = resolveAutoExperimentStore(AutoExperimentConfig{Config: config.Config{LedgerDir: dir}})
	if err != nil {
		t.Fatalf("resolveAutoExperimentStore(default): %v", err)
	}
	if err := got.RecordExperiment(domain.ExperimentRecord{ID: "exp-default-store"}); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("default store did not write the JSONL ledger: %v", err)
	}
	if !strings.Contains(string(data), "exp-default-store") {
		t.Fatalf("default store wrote elsewhere: %s", data)
	}
}
