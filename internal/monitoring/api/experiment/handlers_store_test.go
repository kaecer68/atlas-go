package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Q2 batch 2 unit A：admin judge 的 store seam（hermetic）。
//
// 兩個方向都要釘住：
//   - 注入 ⇒ 用注入的 store（讀取端收斂到部署宣告的後端）；
//   - nil ⇒ 顯式 JSONL 後端，行為與遷移前的 ledger.NewStore(h.LedgerDir) 逐字相同。

// stubExperimentStore 只實作 ExperimentStore 的三個方法；其餘由內嵌介面滿足。
type stubExperimentStore struct {
	ledger.ExperimentStore
	results []string
}

func (s *stubExperimentStore) RecordPromptExperimentResult(id string, r domain.PromptExperimentResult) error {
	s.results = append(s.results, id)
	return nil
}

func (s *stubExperimentStore) UpdatePromptExperimentResult(id string, r domain.PromptExperimentResult) error {
	s.results = append(s.results, "update:"+id)
	return nil
}

func TestHandlersExperimentStore_InjectedStoreWins(t *testing.T) {
	injected := &stubExperimentStore{}
	h := &Handlers{LedgerDir: t.TempDir(), Store: injected}

	got, err := h.experimentStore()
	if err != nil {
		t.Fatalf("experimentStore: %v", err)
	}
	if got != ledger.ExperimentStore(injected) {
		t.Fatalf("experimentStore returned %T, want the injected store", got)
	}
	if err := got.RecordPromptExperimentResult("exp-injected", domain.PromptExperimentResult{}); err != nil {
		t.Fatalf("record through injected store: %v", err)
	}
	if len(injected.results) != 1 || injected.results[0] != "exp-injected" {
		t.Fatalf("injected store did not receive the write: %v", injected.results)
	}
}

// TestHandlersExperimentStore_NilFallsBackToExplicitJSONL is the parity half:
// with no injection the seam must resolve to the same JSONL directory the raw
// constructor used, so struct-literal construction (tests, side binaries)
// keeps its previous behavior.
func TestHandlersExperimentStore_NilFallsBackToExplicitJSONL(t *testing.T) {
	dir := t.TempDir()
	h := &Handlers{LedgerDir: dir}

	store, err := h.experimentStore()
	if err != nil {
		t.Fatalf("experimentStore: %v", err)
	}
	result := domain.PromptExperimentResult{
		Experiment:      domain.ExperimentRecord{ID: "exp-fallback"},
		CandidatePrompt: "v1",
	}
	if err := store.RecordPromptExperimentResult("exp-fallback", result); err != nil {
		t.Fatalf("RecordPromptExperimentResult: %v", err)
	}

	// Same file the pre-seam ledger.NewStore(dir) wrote: <LedgerDir>/experiments/<id>.json.
	path := filepath.Join(dir, "experiments", "exp-fallback.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("result file missing at %s: %v", path, err)
	}
	var got domain.PromptExperimentResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if got.Experiment.ID != "exp-fallback" {
		t.Fatalf("round-tripped id = %q, want exp-fallback", got.Experiment.ID)
	}
}

// TestHandlersExperimentStore_BackendIsNamedNotDefaulted pins the contract the
// ledger seam guard enforces: the fallback must name its backend ("jsonl")
// instead of relying on an unset value's default, so a deployment that forgot
// to configure a backend cannot look like a deliberate JSONL choice.
func TestHandlersExperimentStore_BackendIsNamedNotDefaulted(t *testing.T) {
	dir := t.TempDir()
	h := &Handlers{LedgerDir: dir}

	store, err := h.experimentStore()
	if err != nil {
		t.Fatalf("experimentStore: %v", err)
	}
	jsonlStore, err := ledger.NewFullStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("NewFullStore: %v", err)
	}
	if got, want := typeName(store), typeName(jsonlStore); got != want {
		t.Fatalf("fallback store type = %s, want %s (the explicit JSONL store)", got, want)
	}
}

func typeName(v any) string {
	return fmt.Sprintf("%T", v)
}
