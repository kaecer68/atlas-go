package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
