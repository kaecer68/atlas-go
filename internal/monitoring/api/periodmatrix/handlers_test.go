package periodmatrix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// seededHandler builds a handler over an in-memory SQLite store with session
// outcomes: agent-a has 30 bull wins, agent-b has 20 plateau wins.
func seededHandler(t *testing.T) *Handlers {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteOutcomeStore(db)
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	var outcomes []domain.RecommendationOutcome
	for i := 0; i < 30; i++ {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID: "agent-a", Symbol: "2330", Side: domain.SideBuy, Conviction: 80,
			Window: "2026-04-01", RecordedAt: day, PassedGuards: true,
			ForwardReturn: 0.02, Hit: true,
			MarketPeriod: "bull", MarketPeriodSource: "live",
		})
	}
	for i := 0; i < 20; i++ {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID: "agent-b", Symbol: "2317", Side: domain.SideBuy, Conviction: 70,
			Window: "2026-04-01", RecordedAt: day, PassedGuards: true,
			ForwardReturn: 0.01, Hit: true,
			MarketPeriod: "plateau", MarketPeriodSource: "live",
		})
	}
	session := domain.ReplaySession{ID: "session-20260401-daily", SessionDate: day}
	if err := store.RecordSessionOutcomes(session, outcomes); err != nil {
		t.Fatalf("record: %v", err)
	}
	return NewHandlers(service.NewPeriodMatrixServiceWithStore(store))
}

func TestHandlePeriodMatrix_Shape(t *testing.T) {
	h := seededHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/strategy/period-matrix", nil)
	status, data := h.HandlePeriodMatrix(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	periods, ok := m["periods"].([]any)
	if !ok || len(periods) != 7 {
		t.Fatalf("periods = %v, want 7 entries", m["periods"])
	}
	if m["min_samples"].(float64) != 30 {
		t.Errorf("min_samples = %v, want 30", m["min_samples"])
	}
	cells, ok := m["cells"].([]any)
	if !ok {
		t.Fatalf("cells missing or not an array: %v", m["cells"])
	}
	if len(cells) != 14 { // 2 agents × 7 periods
		t.Fatalf("cells len = %d, want 14", len(cells))
	}
	first := cells[0].(map[string]any)
	for _, key := range []string{"agent_id", "market_period", "sample_count", "win_rate", "sharpe", "status"} {
		if _, ok := first[key]; !ok {
			t.Errorf("cell missing key %q: %v", key, first)
		}
	}
}

func TestHandlePeriodMatrix_RouteAnd503(t *testing.T) {
	mux := http.NewServeMux()
	seededHandler(t).RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/strategy/period-matrix", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("route status = %d, want 200", rr.Code)
	}

	// Unwired service → 503 with a clear message.
	mux2 := http.NewServeMux()
	NewHandlers(nil).RegisterRoutes(mux2)
	rr2 := httptest.NewRecorder()
	mux2.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/strategy/period-matrix", nil))
	if rr2.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired status = %d, want 503", rr2.Code)
	}
}
