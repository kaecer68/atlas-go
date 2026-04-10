package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
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
