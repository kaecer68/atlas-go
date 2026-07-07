package strategy_ranker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

func TestHandleRank(t *testing.T) {
	data := []byte(`[
		{
			"id": "momentum-001",
			"name": "Momentum L2",
			"layer": "L2",
			"summary": "test",
			"conditions": [{"field": "ForeignInvestorNet", "operator": "gt", "value": 0}],
			"direction": "up",
			"risk": "medium",
			"source": "backtest",
			"status": "active",
			"attribution_mode": "rule_based",
			"hit_rate": 0.75,
			"total_tests": 100,
			"total_hits": 75
		},
		{
			"id": "value-001",
			"name": "Value L3",
			"layer": "L3",
			"summary": "test",
			"conditions": [{"field": "PE", "operator": "lt", "value": 15}],
			"direction": "up",
			"risk": "low",
			"source": "backtest",
			"status": "active",
			"attribution_mode": "rule_based",
			"hit_rate": 0.60,
			"total_tests": 80,
			"total_hits": 48
		},
		{
			"id": "expired-001",
			"name": "Expired Rule",
			"layer": "L1",
			"summary": "test",
			"conditions": [{"field": "DXY", "operator": "gt", "value": 100}],
			"direction": "down",
			"risk": "high",
			"source": "manual",
			"status": "expired",
			"attribution_mode": "rule_based",
			"hit_rate": 0.40,
			"total_tests": 50,
			"total_hits": 20
		}
	]`)
	reg, err := strategy_techniques.LoadFromBytes(data)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, reg)
	req := httptest.NewRequest(http.MethodGet, "/api/strategy-ranker/rank", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"rank":1`) {
		t.Fatalf("expected ranked output: %s", body)
	}
	if !contains(body, `"tier":"premium"`) {
		t.Fatalf("expected premium tier: %s", body)
	}
}

func TestHandleRankNilRegistry(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/strategy-ranker/rank", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
