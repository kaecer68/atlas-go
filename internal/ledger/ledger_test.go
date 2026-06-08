package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestBuildScorecards(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "a", Skill: "alpha", Window: "1d", ForwardReturn: -0.02, Hit: false, RecordedAt: time.Now()},
		{AgentID: "a", Skill: "alpha", Window: "5d", ForwardReturn: 0.01, Hit: true, RecordedAt: time.Now()},
		{AgentID: "b", Skill: "beta", Window: "1d", ForwardReturn: 0.03, Hit: true, RecordedAt: time.Now()},
	}

	scorecards := BuildScorecards(outcomes)
	if len(scorecards) != 2 {
		t.Fatalf("expected 2 scorecards, got %d", len(scorecards))
	}
	if scorecards[0].AgentID != "a" {
		t.Fatalf("expected weakest scorecard first")
	}
	if scorecards[0].WindowCount != 2 {
		t.Fatalf("expected alpha window count 2, got %d", scorecards[0].WindowCount)
	}
}

func TestRecordSessionSummaryPersistsTraceIDs(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir)
	session := domain.ReplaySession{ID: "session-test"}

	summary := domain.SessionSummary{
		SessionID:  session.ID,
		Regime:     domain.RegimeNeutral,
		ProposalID: "proposal-1",
		CommitID:   "commit-1",
		ApprovalID: "approval-1",
		BrokerRuntime: domain.BrokerRuntimeAudit{
			Mode:             "live",
			Adapter:          "http",
			Signer:           "hmac-sha256",
			SignerVersion:    "v1",
			KeyID:            "kid-1",
			MaxRetries:       2,
			HTTPTimeoutSec:   5,
			HTTPAttempts:     3,
			RetryStatusCodes: []int{408, 429, 503},
			MaxClockSkewSec:  120,
			NonceTTLSec:      180,
			NonceStore:       "file",
			NonceStorePath:   "data/state/broker-nonce-replay.json",
			NonceRedisPrefix: "atlas:nonce:",
		},
		RecordedAt: time.Now(),
	}

	if err := store.RecordSessionSummary(session, summary); err != nil {
		t.Fatalf("record session summary: %v", err)
	}

	path := filepath.Join(baseDir, "sessions", session.ID, "summary.json")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}

	var got domain.SessionSummary
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if got.ProposalID != summary.ProposalID {
		t.Fatalf("proposal_id mismatch: got %q want %q", got.ProposalID, summary.ProposalID)
	}
	if got.CommitID != summary.CommitID {
		t.Fatalf("commit_id mismatch: got %q want %q", got.CommitID, summary.CommitID)
	}
	if got.ApprovalID != summary.ApprovalID {
		t.Fatalf("approval_id mismatch: got %q want %q", got.ApprovalID, summary.ApprovalID)
	}
	if got.BrokerRuntime.Mode != summary.BrokerRuntime.Mode {
		t.Fatalf("broker runtime mode mismatch: got %q want %q", got.BrokerRuntime.Mode, summary.BrokerRuntime.Mode)
	}
	if got.BrokerRuntime.Adapter != summary.BrokerRuntime.Adapter {
		t.Fatalf("broker runtime adapter mismatch: got %q want %q", got.BrokerRuntime.Adapter, summary.BrokerRuntime.Adapter)
	}
	if got.BrokerRuntime.MaxClockSkewSec != summary.BrokerRuntime.MaxClockSkewSec {
		t.Fatalf("broker runtime max clock skew mismatch: got %d want %d", got.BrokerRuntime.MaxClockSkewSec, summary.BrokerRuntime.MaxClockSkewSec)
	}
	if got.BrokerRuntime.NonceTTLSec != summary.BrokerRuntime.NonceTTLSec {
		t.Fatalf("broker runtime nonce ttl mismatch: got %d want %d", got.BrokerRuntime.NonceTTLSec, summary.BrokerRuntime.NonceTTLSec)
	}
	if got.BrokerRuntime.NonceStore != summary.BrokerRuntime.NonceStore {
		t.Fatalf("broker runtime nonce store mismatch: got %q want %q", got.BrokerRuntime.NonceStore, summary.BrokerRuntime.NonceStore)
	}
	if got.BrokerRuntime.NonceStorePath != summary.BrokerRuntime.NonceStorePath {
		t.Fatalf("broker runtime nonce store path mismatch: got %q want %q", got.BrokerRuntime.NonceStorePath, summary.BrokerRuntime.NonceStorePath)
	}
	if got.BrokerRuntime.SignerVersion != summary.BrokerRuntime.SignerVersion {
		t.Fatalf("broker runtime signer version mismatch: got %q want %q", got.BrokerRuntime.SignerVersion, summary.BrokerRuntime.SignerVersion)
	}
	if got.BrokerRuntime.NonceRedisPrefix != summary.BrokerRuntime.NonceRedisPrefix {
		t.Fatalf("broker runtime nonce redis prefix mismatch: got %q want %q", got.BrokerRuntime.NonceRedisPrefix, summary.BrokerRuntime.NonceRedisPrefix)
	}
}

func TestRecordAndLoadSessionScreeningRejects(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	rejects := []domain.ScreeningReject{
		{SessionID: "s1", Symbol: "2330.TW", AgentID: "a1", Criterion: "pe_max"},
		{SessionID: "s1", Symbol: "2317.TW", AgentID: "a2", Criterion: "volume_min"},
	}
	if err := s.RecordSessionScreeningRejects("s1", rejects); err != nil {
		t.Fatalf("record failed: %v", err)
	}
	loaded, err := s.LoadSessionScreeningRejects("s1")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rejects, got %d", len(loaded))
	}
}

func TestStore_ConcurrentWrites(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir)

	const goroutines = 8
	const perGoroutine = 50
	const total = goroutines * perGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			batch := make([]domain.RecommendationOutcome, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				batch[i] = domain.RecommendationOutcome{
					AgentID:       fmt.Sprintf("agent-%d", g),
					Skill:         "alpha",
					Window:        "1d",
					Symbol:        fmt.Sprintf("%04d.TW", g*perGoroutine+i),
					Side:          "BUY",
					ForwardReturn: float64(i) * 0.001,
					Hit:           i%2 == 0,
					RecordedAt:    time.Now().UTC(),
				}
			}
			if err := store.RecordOutcomes(batch); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", g, err)
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent RecordOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if len(loaded) != total {
		t.Fatalf("expected %d outcomes, got %d (data lost under concurrent writes)", total, len(loaded))
	}

	seen := make(map[string]bool, total)
	for _, oc := range loaded {
		key := oc.Symbol
		if seen[key] {
			t.Errorf("duplicate outcome: %s", key)
		}
		seen[key] = true
	}
	if len(seen) != total {
		t.Errorf("expected %d unique outcomes, got %d", total, len(seen))
	}
}

func TestStore_ConcurrentMixedReadWrite(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir)

	const goroutines = 6
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			summary := domain.SessionSummary{
				SessionID:    fmt.Sprintf("session-%d", g),
				Regime:       domain.RegimeNeutral,
				OutcomeCount: 1,
			}
			if err := store.RecordSessionSummary(domain.ReplaySession{ID: summary.SessionID}, summary); err != nil {
				errCh <- fmt.Errorf("write goroutine %d: %w", g, err)
				return
			}
			if _, _, err := store.LoadAllSessionScorecards(); err != nil {
				errCh <- fmt.Errorf("read goroutine %d: %w", g, err)
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent mixed read/write failed: %v", err)
	}
}

// ==== Audit remediation tests for AI Observatory ====

func TestSharpeRatio(t *testing.T) {
	tests := []struct {
		name     string
		returns  []float64
		expected float64
	}{
		{"empty", []float64{}, 0},
		{"single element", []float64{0.01}, 0},
		{"all same", []float64{0.01, 0.01, 0.01}, 0},
		{"two elements positive", []float64{0.01, 0.02}, 2.121},
		{"two elements negative", []float64{-0.01, -0.02}, -2.121},
		{"known values", []float64{0.01, -0.02, 0.015, -0.005, 0.02}, 0.245},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portfolio.ComputeSharpe(tt.returns, portfolio.SharpeConfig{
				Frequency:  portfolio.FrequencyPerOutcome,
				MinSamples: 2,
			})
			if diff := math.Abs(got - tt.expected); diff > 0.001 {
				t.Errorf("sharpe via ComputeSharpe = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMaxDrawdown(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"empty", []float64{}, 0},
		{"all positive", []float64{0.01, 0.02, 0.01}, 0},
		{"single large drop", []float64{-0.5}, 0.5},
		{"known sequence", []float64{0.1, -0.05, 0.03, -0.08, 0.02}, 0.0998},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxDrawdown(tt.values)
			if diff := math.Abs(got - tt.expected); diff > 0.001 {
				t.Errorf("maxDrawdown() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRatio(t *testing.T) {
	if ratio(0, 0) != 0 {
		t.Errorf("ratio(0,0) = %v, want 0", ratio(0, 0))
	}
	if ratio(3, 5) != 0.6 {
		t.Errorf("ratio(3,5) = %v, want 0.6", ratio(3, 5))
	}
}

func TestMean(t *testing.T) {
	if mean([]float64{}) != 0 {
		t.Errorf("mean(empty) = %v, want 0", mean([]float64{}))
	}
	if mean([]float64{1, 2, 3}) != 2 {
		t.Errorf("mean([1,2,3]) = %v, want 2", mean([]float64{1, 2, 3}))
	}
}

func TestBuildScorecardsNewFields(t *testing.T) {
	// Create outcomes with enough windows for statistical significance
	outcomes := []domain.RecommendationOutcome{}
	for i := 0; i < 25; i++ {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:       "sig-agent",
			Skill:         "alpha",
			Window:        fmt.Sprintf("w%d", i),
			ForwardReturn: 0.01,
			Hit:           true,
			RecordedAt:    time.Now(),
		})
	}
	// Add some negative returns for variance
	outcomes = append(outcomes, domain.RecommendationOutcome{
		AgentID:       "sig-agent",
		Skill:         "alpha",
		Window:        "w25",
		ForwardReturn: -0.02,
		Hit:           false,
		RecordedAt:    time.Now(),
	})

	scorecards := BuildScorecards(outcomes)
	if len(scorecards) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(scorecards))
	}
	sc := scorecards[0]
	if !sc.StatisticallySignificant {
		t.Errorf("expected StatisticallySignificant=true for 26 windows, got false")
	}
	if sc.TStat <= 0 {
		t.Errorf("expected positive TStat, got %v", sc.TStat)
	}
	if sc.ConfidenceLow >= sc.ConfidenceHigh {
		t.Errorf("expected ConfidenceLow < ConfidenceHigh, got %v >= %v", sc.ConfidenceLow, sc.ConfidenceHigh)
	}
	if sc.HitRateTStat <= 0 {
		t.Errorf("expected positive HitRateTStat, got %v", sc.HitRateTStat)
	}

	// Test non-significant case
	outcomesSmall := []domain.RecommendationOutcome{
		{AgentID: "small", Skill: "beta", Window: "w1", ForwardReturn: 0.01, Hit: true, RecordedAt: time.Now()},
	}
	scSmall := BuildScorecards(outcomesSmall)[0]
	if scSmall.StatisticallySignificant {
		t.Errorf("expected StatisticallySignificant=false for 1 window, got true")
	}
	if scSmall.TStat != 0 {
		t.Errorf("expected TStat=0 for n<2, got %v", scSmall.TStat)
	}
}

func TestBuildScorecards_OOS_InsufficientTestSamples(t *testing.T) {
	// 12 outcomes → 80/20 split → 9 train / 3 test → test < 5 → warning
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	outcomes := make([]domain.RecommendationOutcome, 12)
	for i := range 12 {
		outcomes[i] = domain.RecommendationOutcome{
			AgentID:       "oos-agent",
			Skill:         "alpha",
			ForwardReturn: 0.01,
			Hit:           true,
			RecordedAt:    now.Add(time.Duration(i) * time.Hour),
		}
	}
	sc := BuildScorecards(outcomes)
	if len(sc) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(sc))
	}
	s := sc[0]
	if s.OosSampleWarning == "" {
		t.Fatal("expected oos_sample_warning for insufficient test samples (< 5)")
	}
	if !strings.Contains(s.OosSampleWarning, "insufficient_test_samples") {
		t.Errorf("warning should mention insufficient_test_samples, got: %s", s.OosSampleWarning)
	}
	if s.OosSharpe != 0 {
		t.Errorf("expected OosSharpe=0 when test < 5, got %v", s.OosSharpe)
	}
	if s.IsSharpe != 0 {
		t.Errorf("expected IsSharpe=0 when train < 10, got %v", s.IsSharpe)
	}
}

func TestBuildScorecards_OOS_NormalCase(t *testing.T) {
	// 25 outcomes across 25 windows → 20 train / 5 test → IS & OOS computed
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	outcomes := make([]domain.RecommendationOutcome, 25)
	oosReturns := []float64{-0.02, -0.03, 0.01, -0.04, -0.01} // variance so ComputeSharpe works
	for i := range 25 {
		r := 0.01
		if i >= 20 {
			r = oosReturns[i-20]
		}
		outcomes[i] = domain.RecommendationOutcome{
			AgentID:       "oos-agent-2",
			Skill:         "beta",
			Window:        fmt.Sprintf("w%d", i),
			ForwardReturn: r,
			Hit:           r > 0,
			RecordedAt:    now.Add(time.Duration(i) * time.Hour),
		}
	}
	sc := BuildScorecards(outcomes)
	if len(sc) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(sc))
	}
	s := sc[0]
	if s.OosSampleWarning != "" {
		t.Errorf("expected no sample warning for 20/5 split, got: %s", s.OosSampleWarning)
	}
	if s.IsSharpe == 0 {
		t.Errorf("expected non-zero IsSharpe for 20 train samples")
	}
	if s.OosSharpe == 0 {
		t.Errorf("expected non-zero OosSharpe for 5 test samples")
	}
	if !s.OverfitWarning {
		t.Errorf("expected overfit_warning for IS positive + OOS negative, got false")
	}
	if s.IsOosRatio <= 0 {
		t.Errorf("expected positive IsOosRatio, got %v", s.IsOosRatio)
	}
	if s.RollingSharpeTrend == 0 {
		t.Errorf("expected non-zero RollingSharpeTrend for 25 windows with increasing returns")
	}
}

func TestBuildScorecards_OOS_ChronologicalOrder(t *testing.T) {
	// 25 outcomes (20 train / 5 test); earliest 20 positive, latest 5 negative
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	outcomes := make([]domain.RecommendationOutcome, 25)
	for i := range 20 {
		outcomes[i] = domain.RecommendationOutcome{
			AgentID:       "chrono-agent",
			Skill:         "gamma",
			ForwardReturn: 0.01,
			Hit:           true,
			RecordedAt:    now.Add(time.Duration(i) * time.Hour),
		}
	}
	oosReturns := []float64{-0.10, -0.12, 0.02, -0.08, -0.09}
	for i := 20; i < 25; i++ {
		outcomes[i] = domain.RecommendationOutcome{
			AgentID:       "chrono-agent",
			Skill:         "gamma",
			ForwardReturn: oosReturns[i-20],
			Hit:           false,
			RecordedAt:    now.Add(time.Duration(100+i) * time.Hour),
		}
	}
	sc := BuildScorecards(outcomes)
	if len(sc) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(sc))
	}
	s := sc[0]
	if s.OosSharpe >= 0 {
		t.Errorf("expected negative OosSharpe for negative OOS returns, got %v", s.OosSharpe)
	}
	if s.OverfitWarning != true {
		t.Errorf("expected overfit_warning when OOS negative and IS positive")
	}
}
