package recommender

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// TestE2E_RecommendationsEndpoint exercises the full HTTP server flow
// against /api/recommendations with all Sprint 2 integrations wired
// (narrative + capitalflow + events + strategy). Uses mocks for the
// services — production T13 (real services) and T14 (shadow rollout)
// live in follow-up PRs.
func TestE2E_RecommendationsEndpoint(t *testing.T) {
	dir, err := os.MkdirTemp("", "rec-e2e")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := subscription.NewStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_, _ = store.Register("premium@test.com", "pass")

	narrative := &mockNarrative{stress: 22.5, regime: "RISK_OFF"}
	capflow := &mockCapitalFlow{summary: "外資連三買超 800 億"}
	evts := &mockEventPredictor{direction: "MSCI 調整"}
	strategy := &mockComparisonEngine{score: 0.85}

	handler := NewHandlerWithServices(*store, nil, narrative, capflow, evts, strategy, nil).WithDevMode(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/recommendations", func(w http.ResponseWriter, r *http.Request) {
		code, data := handler.HandleRecommendations(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if data != nil {
			_ = json.NewEncoder(w).Encode(data)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name       string
		email      string
		wantStatus int
		wantTier   string
		wantRegime string
		wantSignal string
	}{
		{
			name:       "premium user gets full wiring",
			email:      "premium@test.com",
			wantStatus: http.StatusOK,
			wantTier:   "premium",
			wantRegime: "RISK_OFF",
			wantSignal: "Score=0.85 — 排名第1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/recommendations", nil)
			req.Header.Set("X-User-Email", tc.email)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("HTTP call: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			var rec TierRecommendation
			if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if rec.Tier != tc.wantTier {
				t.Errorf("tier = %q, want %q", rec.Tier, tc.wantTier)
			}
			if rec.Market.Regime != tc.wantRegime {
				t.Errorf("regime = %q, want %q (from narrative mock)", rec.Market.Regime, tc.wantRegime)
			}
			if rec.Market.StressIndex != 22.5 {
				t.Errorf("stress = %f, want 22.5 (from narrative mock)", rec.Market.StressIndex)
			}
			if rec.Market.CapitalFlow != "外資連三買超 800 億" {
				t.Errorf("capital_flow = %q, want from mock", rec.Market.CapitalFlow)
			}
			if len(rec.Market.EventsToday) != 1 {
				t.Errorf("events_today len = %d, want 1 (from PredictToday mock)", len(rec.Market.EventsToday))
			}
			if rec.Strategies == nil {
				t.Errorf("Strategies nil for premium tier")
			} else if rec.Strategies.EntrySignal != tc.wantSignal {
				t.Errorf("entry_signal = %q, want %q (from ComparisonEngine mock)", rec.Strategies.EntrySignal, tc.wantSignal)
			}
		})
	}
}
