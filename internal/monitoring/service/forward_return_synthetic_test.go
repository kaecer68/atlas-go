package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// TestLoadSessionPipelineData_ForwardReturnSyntheticGate 驗證：
//   - IsSynthetic=true 且 ForwardReturn=0 → 觸發 dataset 回填（placeholder 補齊）
//   - IsSynthetic=false 且 ForwardReturn=0 → 視為真實 0% 漲跌，保留原值
//
// 修復前 `if fr == 0` 無法區分上述兩種情境；修復後以 IsSynthetic 作為精準門檻。
func TestLoadSessionPipelineData_ForwardReturnSyntheticGate(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 0, 0, 0, time.UTC)
	nextDate := time.Date(2026, time.April, 23, 4, 0, 0, 0, time.UTC)
	sessionID := "session-20260422-daily"

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "synthetic-agent",
			Skill:         "value_yield",
			Layer:         domain.LayerStyle,
			Symbol:        "2330.TW",
			Side:          domain.SideBuy,
			Conviction:    70,
			Window:        "2026-04-22",
			ForwardReturn: 0,
			Price:         100,
			PassedGuards:  true,
			RecordedAt:    recordedAt,
			IsSynthetic:   true,
		},
		{
			AgentID:       "real-agent",
			Skill:         "value_yield",
			Layer:         domain.LayerStyle,
			Symbol:        "2330.TW",
			Side:          domain.SideBuy,
			Conviction:    71,
			Window:        "2026-04-22",
			ForwardReturn: 0,
			Price:         100,
			PassedGuards:  true,
			RecordedAt:    recordedAt,
			IsSynthetic:   false,
		},
	}

	writeTestSessionOutcomes(
		t, baseDir, sessionID,
		domain.SessionSummary{
			SessionID:    sessionID,
			Regime:       domain.RegimeRiskOn,
			RecordedAt:   recordedAt,
			OutcomeCount: len(outcomes),
		},
		outcomes,
	)

	// 建構 in-memory dataset：2330.TW 在 04-22 收 100、04-23 收 110，
	// ForwardReturn(symbol, recordedAt, 1) 應回傳 (110-100)/100 = 0.10。
	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{
			recordedAt.Format("2006-01-02"): {
				"2330.TW": {Date: recordedAt, Symbol: "2330.TW", Close: 100},
			},
			nextDate.Format("2006-01-02"): {
				"2330.TW": {Date: nextDate, Symbol: "2330.TW", Close: 110},
			},
		},
		Dates: []time.Time{recordedAt, nextDate},
	}

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	sd := svc.loadSessionPipelineData(sessionID, filepath.Join(baseDir, "sessions"), true, ds)
	if sd == nil {
		t.Fatal("expected sessionData, got nil")
	}
	if len(sd.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sd.Items))
	}

	byAgent := make(map[string]PipelineItemData, len(sd.Items))
	for _, item := range sd.Items {
		byAgent[item.AgentID] = item
	}

	synth, ok := byAgent["synthetic-agent"]
	if !ok {
		t.Fatal("synthetic-agent not found in pipeline items")
	}
	const wantRecalc = 0.10
	if diff := synth.ForwardReturn - wantRecalc; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("synthetic forward_return: got %v, want %v (recalculation expected)", synth.ForwardReturn, wantRecalc)
	}

	real, ok := byAgent["real-agent"]
	if !ok {
		t.Fatal("real-agent not found in pipeline items")
	}
	if real.ForwardReturn != 0 {
		t.Errorf("real forward_return: got %v, want 0 (genuine zero must be preserved)", real.ForwardReturn)
	}
}
