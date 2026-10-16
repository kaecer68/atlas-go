package ledger

// experiments 家族的 store 選擇（Q2 第二批 unit A，2026-10-16）。
//
// # 家族有兩半，寫入端必須分開對待
//
//	① record/status 半：domain.ExperimentRecord → experiments 表（PG/SQLite）／
//	   <LedgerDir>/experiments.jsonl。這半承載生命週期
//	   （planned → running → accepted/rejected/expired）。
//	② result 半：domain.PromptExperimentResult → prompt_experiment_results 表／
//	   <LedgerDir>/experiments/<experiment_id>.json。
//
// # 為什麼 record 半原本進不了 PG
//
// 家族的 record 寫者有兩個來源：
//   - replay pipeline（internal/orchestrator/system.go）透過 backend-aware 的
//     ledger.NewOutcomeStore 寫 → 生產是 PG（只寫進 planned 的候選列）。
//   - 實驗執行／判定 pipeline（本檔的呼叫端：internal/experiment/auto.go、
//     internal/taskexec、cmd/run-experiment、cmd/judge-experiment）直建
//     ledger.NewStore(LedgerDir) → 永遠是 JSONL。
//
// 於是生產的 PG experiments 表只拿到「半條生命週期」：894 列全部 planned、
// accepted <> 0 = 0、prompt_experiment_results = 0（FU-20261016-04 實測）。
// 讀取端一旦切到 PG 就會看到假的「接受率 0%」——不是舊快照，而是半份資料。
// 本檔把 record 半收斂到部署宣告的後端，讓生命週期完整落地。
//
// # 為什麼 result 半仍然要寫檔案（本單元的顯式例外）
//
// <LedgerDir>/experiments/<id>.json 不是「JSONL 後端的副產品」，而是整個家族
// **以檔案路徑為介面的內部契約**，至少這些讀者直接吃它：
//
//	experiment.Judge.Evaluate(resultPath)（judge.go 的 loadExperimentResult）
//	experiment.FindLatestExperiment / LoadPendingExperiments（auto.go / find.go）
//	AutoJudgePromoter.TryPromoteFromPath(expPath)、baseline.Manager.PromoteResult(expPath)
//	monitoring/api/experiment 的 HandleInbox（掃目錄）與 HandleDiff（讀單檔）
//
// 這些都不是 store 建構站點（守門掃不到，屬另一家族），本單元不動它們。
// 若 result 只寫 PG，生產的 judge/inbox/diff/promoter 會全部讀不到東西 ——
// 那是比現況更糟的迴歸。因此 result 半以「部署後端 ＋ JSONL 檔案」雙寫：
// PG 側是 prompt_experiment_results（DSR gate 的 LoadTrialSharpes 讀它，
// 見 internal/experiment/dsr.go），檔案側維持上述讀者的契約。
//
// jsonl 後端時兩者是同一個 store（同一個檔案），故只有一次寫入，行為與遷移前
// 逐字相同。
//
// # 仍然走檔案的部分（本單元留下的缺口，已登記 FU）
//
//   - 上述 result 檔案的**讀者**（judge 的 resultPath、inbox、diff、promoter）。
//   - window summary：judge 以檔案路徑讀回（judge.go windowSummaryPath／
//     loadWindowSummary），其寫者是 auto.go 的「judge 前即時補算」，依 T2
//     （PR #2238）的裁決維持顯式 JSONL。要 PG 化必須同時改 judge 的 window 讀取。
//   - evolution_health 的 lastProposalActivity（讀 experiments.jsonl）。

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// NewExperimentFamilyStore builds the store the experiments family's write paths use.
//
// The returned store is backend-aware: it is resolved from cfg.StoreBackend
// exactly like every other store factory (jsonl / sqlite / postgres), so a
// deployment that declares postgres gets the family's records and result blobs
// in PostgreSQL instead of in a JSONL directory no PG reader looks at.
//
// It never falls back to JSONL. A postgres deployment without an injected pool
// (ledger.SetPostgresPool) returns an error: silently writing the lifecycle to
// JSONL is the exact failure mode the ledger seam guard exists for.
//
// The result half is additionally mirrored to <LedgerDir>/experiments/<id>.json
// for every non-jsonl backend — see the package comment above for why that file
// is still a contract, and which readers still depend on it.
func NewExperimentFamilyStore(cfg config.Config) (FullStore, error) {
	backend, err := ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, fmt.Errorf("experiment store: %w", err)
	}
	// Reuse the normalized backend so the factory does not resolve (and warn)
	// a second time.
	cfg.StoreBackend = backend

	primary, err := NewFullStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("experiment store: %w", err)
	}
	if backend == "jsonl" {
		// The JSONL store already writes both halves into LedgerDir.
		return primary, nil
	}

	files, err := NewFullStore(config.Config{
		StoreBackend: "jsonl",
		LedgerDir:    cfg.LedgerDir,
	})
	if err != nil {
		return nil, fmt.Errorf("experiment result mirror: %w", err)
	}
	return resultMirror{FullStore: primary, files: files}, nil
}

// resultMirror routes the family's record half to the deployment's backend and
// keeps the judge-facing result files on disk.
//
// The embedded FullStore answers every method that is not overridden
// (records, statuses, sessions, ...). Only the two result-blob writes are
// mirrored, and LoadTrialSharpes is re-exposed (see below).
type resultMirror struct {
	FullStore
	files FullStore
}

// RecordPromptExperimentResult writes the result to the deployment's backend
// first and then to the JSONL directory the path-based readers share. The
// backend write is the one whose error is returned: it carries the DSR gate's
// trial history.
func (s resultMirror) RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	if err := s.FullStore.RecordPromptExperimentResult(experimentID, result); err != nil {
		return err
	}
	return s.files.RecordPromptExperimentResult(experimentID, result)
}

// UpdatePromptExperimentResult mirrors RecordPromptExperimentResult for the
// judge's write-back (the judged result must be visible to the inbox/diff
// readers, which read the file).
func (s resultMirror) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	if err := s.FullStore.UpdatePromptExperimentResult(experimentID, result); err != nil {
		return err
	}
	return s.files.UpdatePromptExperimentResult(experimentID, result)
}

// LoadTrialSharpes forwards to the wrapped store so the wrapper keeps
// satisfying TrialSharpeLoader.
//
// This is not optional plumbing: the DSR gate looks the loader up with a type
// assertion on the judge's store
// (internal/experiment/dsr.go: `j.store.(TrialSharpeLoader)`), and
// FullStore does not declare that method. A wrapper that only embedded
// FullStore would therefore fail the assertion, judge.deflatedSharpeGate would
// return "skip" for its whole life, and the anti-luck gate would silently stop
// running again — the exact 2026-10-12 wiring hole documented on
// ledger.Store.LoadTrialSharpes.
func (s resultMirror) LoadTrialSharpes() ([]TrialSharpe, error) {
	loader, ok := s.FullStore.(TrialSharpeLoader)
	if !ok {
		return nil, fmt.Errorf("experiment store: %T does not implement TrialSharpeLoader", s.FullStore)
	}
	return loader.LoadTrialSharpes()
}
