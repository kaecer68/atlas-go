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
	// IS returns have slight positive variance so IsSharpe is well-defined (not 0/NaN from constant inputs).
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	outcomes := make([]domain.RecommendationOutcome, 25)
	oosReturns := []float64{-0.02, -0.03, 0.01, -0.04, -0.01} // variance so ComputeSharpe works
	for i := range 25 {
		r := 0.01 + 0.0001*float64(i)
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

func TestRecordAndLoadSessionTrades(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	sessionID := "session-trades"

	trades := []domain.TradeRecord{
		{TradeID: "t1", SessionID: sessionID, Symbol: "2330.TW", Side: domain.SideBuy, Quantity: 1000, Price: 500.0, Amount: 500000, Timestamp: time.Now()},
		{TradeID: "t2", SessionID: sessionID, Symbol: "2317.TW", Side: domain.SideSell, Quantity: 2000, Price: 100.0, Amount: 200000, Timestamp: time.Now()},
	}
	if err := store.RecordSessionTrades(sessionID, trades); err != nil {
		t.Fatalf("RecordSessionTrades: %v", err)
	}

	loaded, err := store.LoadSessionTrades(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionTrades: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(loaded))
	}
	if loaded[0].TradeID != "t1" {
		t.Errorf("first trade ID = %q, want t1", loaded[0].TradeID)
	}
	if loaded[1].TradeID != "t2" {
		t.Errorf("second trade ID = %q, want t2", loaded[1].TradeID)
	}
}

func TestRecordSessionTrades_EmptySlice_Noop(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	if err := store.RecordSessionTrades("session-empty", nil); err != nil {
		t.Fatalf("RecordSessionTrades with nil: %v", err)
	}
	if err := store.RecordSessionTrades("session-empty", []domain.TradeRecord{}); err != nil {
		t.Fatalf("RecordSessionTrades with empty: %v", err)
	}
	// No file should have been created
	tradesPath := filepath.Join(dir, "sessions", "session-empty", "trades.jsonl")
	if _, err := os.Stat(tradesPath); !os.IsNotExist(err) {
		t.Errorf("expected no trades file for empty slice, but found one at %s", tradesPath)
	}
}

func TestLoadSessionTrades_NotExist(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	loaded, err := store.LoadSessionTrades("nonexistent")
	if err != nil {
		t.Fatalf("LoadSessionTrades: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for nonexistent session, got %v", loaded)
	}
}

func TestLoadAllSessionTrades(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	trades1 := []domain.TradeRecord{
		{TradeID: "t1", SessionID: "s1", Symbol: "2330.TW", Side: domain.SideBuy, Quantity: 100, Price: 500, Amount: 50000, Timestamp: time.Now().Add(-2 * time.Hour)},
	}
	trades2 := []domain.TradeRecord{
		{TradeID: "t2", SessionID: "s2", Symbol: "2317.TW", Side: domain.SideSell, Quantity: 200, Price: 100, Amount: 20000, Timestamp: time.Now()},
	}
	if err := store.RecordSessionTrades("s1", trades1); err != nil {
		t.Fatalf("RecordSessionTrades s1: %v", err)
	}
	if err := store.RecordSessionTrades("s2", trades2); err != nil {
		t.Fatalf("RecordSessionTrades s2: %v", err)
	}

	all, err := store.LoadAllSessionTrades()
	if err != nil {
		t.Fatalf("LoadAllSessionTrades: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total trades, got %d", len(all))
	}
	// Should be sorted newest-first
	if all[0].TradeID != "t2" {
		t.Errorf("newest trade ID = %q, want t2", all[0].TradeID)
	}
}

func TestRecordWindowSummary(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	summary := domain.BacktestWindowSummary{
		WindowID: "window-1", SessionCount: 10, OutcomeCount: 100,
		GeneratedAt: time.Now(),
	}
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	path := filepath.Join(dir, "windows", "window-1.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("window summary file should exist: %v", err)
	}
}

func TestRecordAndLoadExperiments(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	rec := domain.ExperimentRecord{ID: "exp-1", Status: domain.ExperimentPlanned}
	if err := store.RecordExperiment(rec); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}

	loaded, err := store.LoadExperiments()
	if err != nil {
		t.Fatalf("LoadExperiments: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 experiment, got %d", len(loaded))
	}
	if loaded[0].ID != "exp-1" {
		t.Errorf("experiment ID = %q, want exp-1", loaded[0].ID)
	}
}

func TestLoadExperiments_NotExist(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	loaded, err := store.LoadExperiments()
	if err != nil {
		t.Fatalf("LoadExperiments: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for missing experiments, got %v", loaded)
	}
}

func TestLoadOutcomesFromSessions(t *testing.T) {
	dir := t.TempDir()
	// LoadOutcomesFromSessions reads session dirs from baseDir/sessions/
	sessionsDir := filepath.Join(dir, "sessions")
	sessionDir := filepath.Join(sessionsDir, "session-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "a1", Skill: "s1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "1d", ForwardReturn: 0.01, Hit: true, RecordedAt: time.Now(), Conviction: 80},
		{AgentID: "a2", Skill: "s2", Symbol: "2317.TW", Side: domain.SideSell, Window: "1d", ForwardReturn: -0.01, Hit: false, RecordedAt: time.Now(), Conviction: 60},
	}
	if err := writeOutcomesToFile(path, outcomes); err != nil {
		t.Fatalf("write outcomes: %v", err)
	}

	store := NewStore(dir).(*Store)
	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(loaded))
	}
}

func TestLoadOutcomesFromSessions_NoSessions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	store := NewStore(dir).(*Store)
	// Empty sessions dir returns nil, nil (not an error)
	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for no sessions, got %v", loaded)
	}
}

func TestLoadOutcomesFromSessions_SkipsNonSessions(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	// Create a non-session directory that should be skipped
	otherDir := filepath.Join(sessionsDir, "not-a-session")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := NewStore(dir).(*Store)
	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for no session dirs, got %v", loaded)
	}
}

func TestRecordAndLoadSessionScreeningRejects_Detailed(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	sessionID := "session-rejects"

	rejects := []domain.ScreeningReject{
		{Symbol: "2498.TW", AgentID: "a1", Criterion: "volume"},
		{Symbol: "3008.TW", AgentID: "a2", Criterion: "pe"},
	}
	if err := store.RecordSessionScreeningRejects(sessionID, rejects); err != nil {
		t.Fatalf("RecordSessionScreeningRejects: %v", err)
	}

	loaded, err := store.LoadSessionScreeningRejects(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rejects, got %d", len(loaded))
	}
	if loaded[0].Symbol != "2498.TW" {
		t.Errorf("first reject symbol = %q, want 2498.TW", loaded[0].Symbol)
	}
}

func TestLoadSessionScreeningRejects_NotExist(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	loaded, err := store.LoadSessionScreeningRejects("nonexistent")
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for nonexistent session, got %v", loaded)
	}
}

func TestRecordSpawnRecord(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	rec := SpawnRecord{
		AgentID:    "agent-1",
		GapID:      "gap-1",
		GapPattern: "momentum",
		CreatedAt:  time.Now(),
	}
	if err := store.RecordSpawnRecord(rec); err != nil {
		t.Fatalf("RecordSpawnRecord: %v", err)
	}
	loaded, err := store.LoadSpawnRecords()
	if err != nil {
		t.Fatalf("LoadSpawnRecords: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 spawn record, got %d", len(loaded))
	}
	if loaded[0].AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", loaded[0].AgentID)
	}
}

func TestRecordHumanIntervention_PersistsFullRecord(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	intervention := domain.HumanIntervention{
		ID: "hi-1", Type: "override", Reason: "manual correction",
		Operator: "admin", RecordedAt: time.Now(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		t.Fatalf("RecordHumanIntervention: %v", err)
	}
	loaded, err := store.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 intervention, got %d", len(loaded))
	}
}

func TestConcurrentRecordOutcomes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	var wg sync.WaitGroup
	n := 10
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			outcome := domain.RecommendationOutcome{
				AgentID: fmt.Sprintf("agent-%d", idx),
				Skill:   "s", Symbol: "2330.TW", Window: "1d",
				ForwardReturn: float64(idx) * 0.01, Hit: idx%2 == 0,
				RecordedAt: time.Now(),
			}
			if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
				t.Errorf("concurrent RecordOutcomes idx=%d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes after concurrent writes: %v", err)
	}
	if len(loaded) != n {
		t.Errorf("expected %d outcomes after concurrent writes, got %d", n, len(loaded))
	}
}
