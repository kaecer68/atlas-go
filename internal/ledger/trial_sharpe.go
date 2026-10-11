package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TrialSharpe 是單一歷史 trial 的 Sharpe 摘要：DSR gate 的 K（試驗次數）
// 與 V̂（trial Sharpe 截面變異數）的唯一來源。Sharpe 取自每次試驗持久化的
// PromptExperimentResult.EvalMetrics（eval.SharpeRatio 年化口徑）；沒有
// EvalMetrics 的舊 blob 跳過（永不回填推測）。
type TrialSharpe struct {
	ExperimentID  string  `json:"experiment_id"`
	TargetAgentID string  `json:"target_agent_id"`
	MutationType  string  `json:"mutation_type"`
	Sharpe        float64 `json:"sharpe"`
}

// TrialSharpeLoader 是可選的歷史 trial Sharpe 讀取（DSR gate 用）。
// *SQLiteStore 與 *PostgresLedgerStore 滿足它；不實作它的 store
// （如測試 mock）使 judge 走 fail-open 路徑——向後相容。
type TrialSharpeLoader interface {
	LoadTrialSharpes() ([]TrialSharpe, error)
}

// trialSharpeFromResult 解析單一 result blob；無 EvalMetrics 回
// ok=false（跳過）。
func trialSharpeFromResult(data string) (TrialSharpe, bool) {
	var result domain.PromptExperimentResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return TrialSharpe{}, false
	}
	if result.EvalMetrics == nil {
		return TrialSharpe{}, false
	}
	return TrialSharpe{
		ExperimentID:  result.Experiment.ID,
		TargetAgentID: result.Experiment.TargetAgentID,
		MutationType:  result.Experiment.MutationType,
		Sharpe:        result.EvalMetrics.Sharpe,
	}, true
}

// LoadTrialSharpes 讀回所有含 EvalMetrics 的歷史 trial Sharpe（newest first）。
func (s *SQLiteStore) LoadTrialSharpes() ([]TrialSharpe, error) {
	rows, err := s.db.Query(`SELECT data_json FROM prompt_experiment_results ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query trial sharpes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []TrialSharpe
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan trial sharpe: %w", err)
		}
		if ts, ok := trialSharpeFromResult(data); ok {
			out = append(out, ts)
		}
	}
	return out, rows.Err()
}
