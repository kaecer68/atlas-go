package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSessionSummaryUnmarshalSnakeCase(t *testing.T) {
	canonical := []byte(`{
		"session_id": "session-20260407-daily",
		"regime": "RISK_ON",
		"order_count": 0,
		"position_count": 5,
		"ending_cash": 313967.48,
		"portfolio_value": 1046223.78,
		"outcome_count": 26,
		"broker_runtime": {
			"mode": "dry-run",
			"adapter": "guarded",
			"signer": "placeholder",
			"signer_version": "v1",
			"key_id": "",
			"max_retries": 1,
			"http_timeout_sec": 5,
			"http_attempts": 2,
			"retry_status_codes": [408, 429, 503],
			"max_clock_skew_sec": 300,
			"nonce_ttl_sec": 300,
			"nonce_store": "memory",
			"nonce_store_path": "",
			"nonce_redis_prefix": "atlas:nonce:"
		},
		"guard_outcomes": [
			{
				"guard_id": "cro-01",
				"guard_skill": "cro_risk",
				"severity": "hard",
				"passed": true,
				"reason": "ok",
				"input_count": 26,
				"output_count": 24
			}
		],
		"recorded_at": "2026-04-22T04:02:30.434394+08:00"
	}`)

	var got SessionSummary
	if err := json.Unmarshal(canonical, &got); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	if got.SessionID != "session-20260407-daily" {
		t.Fatalf("session_id: got %q", got.SessionID)
	}
	if got.Regime != RegimeRiskOn {
		t.Fatalf("regime: got %q", got.Regime)
	}
	if got.BrokerRuntime.Adapter != "guarded" {
		t.Fatalf("broker_runtime.adapter: got %q", got.BrokerRuntime.Adapter)
	}
	if len(got.GuardOutcomes) != 1 {
		t.Fatalf("guard_outcomes len: got %d", len(got.GuardOutcomes))
	}
	if got.GuardOutcomes[0].GuardID != "cro-01" {
		t.Fatalf("guard_outcomes[0].guard_id: got %q", got.GuardOutcomes[0].GuardID)
	}
	if got.GuardOutcomes[0].GuardSkill != "cro_risk" {
		t.Fatalf("guard_outcomes[0].guard_skill: got %q", got.GuardOutcomes[0].GuardSkill)
	}
}

func TestSessionSummaryUnmarshalLegacyPascalCase(t *testing.T) {
	legacy := []byte(`{
		"SessionID": "session-20260326-daily",
		"Regime": "NEUTRAL",
		"OrderCount": 0,
		"PositionCount": 0,
		"EndingCash": 0,
		"PortfolioValue": 0,
		"OutcomeCount": 0,
		"BrokerRuntime": {
			"Mode": "dry-run",
			"Adapter": "guarded",
			"Signer": "placeholder",
			"SignerVersion": "v1",
			"KeyID": "",
			"MaxRetries": 1,
			"HTTPTimeoutSec": 5,
			"HTTPAttempts": 2,
			"RetryStatusCodes": [408, 429, 503],
			"MaxClockSkewSec": 300,
			"NonceTTLSec": 300,
			"NonceStore": "memory",
			"NonceStorePath": "",
			"NonceRedisPrefix": "atlas:nonce:"
		},
		"GuardOutcomes": [
			{
				"GuardID": "control-bypass",
				"GuardSkill": "control_bypass",
				"Severity": "soft",
				"Passed": true,
				"Reason": "ok",
				"InputCount": 0,
				"OutputCount": 0
			}
		],
		"RecordedAt": "2026-05-06T15:51:39.840309175Z"
	}`)

	var got SessionSummary
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if got.SessionID != "session-20260326-daily" {
		t.Fatalf("legacy SessionID: got %q", got.SessionID)
	}
	if got.OutcomeCount != 0 {
		t.Fatalf("legacy OutcomeCount: got %d", got.OutcomeCount)
	}
	if got.BrokerRuntime.Adapter != "guarded" {
		t.Fatalf("legacy BrokerRuntime.Adapter: got %q", got.BrokerRuntime.Adapter)
	}
	if len(got.GuardOutcomes) != 1 {
		t.Fatalf("legacy GuardOutcomes len: got %d", len(got.GuardOutcomes))
	}
	if got.GuardOutcomes[0].GuardID != "control-bypass" {
		t.Fatalf("legacy GuardOutcomes[0].GuardID: got %q", got.GuardOutcomes[0].GuardID)
	}

	reencoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal canonical: %v", err)
	}
	text := string(reencoded)
	for _, want := range []string{"\"session_id\"", "\"outcome_count\"", "\"recorded_at\"", "\"guard_outcomes\"", "\"guard_id\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected canonical output to contain %s, got:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"\"SessionID\"", "\"OutcomeCount\"", "\"RecordedAt\"", "\"GuardOutcomes\"", "\"GuardID\""} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected canonical output NOT to contain %s, got:\n%s", forbidden, text)
		}
	}
}

func TestSessionSummaryMarshalRoundTripStaysCanonical(t *testing.T) {
	summary := SessionSummary{
		SessionID:      "session-20260407-daily",
		Regime:         RegimeRiskOn,
		OrderCount:     3,
		PositionCount:  2,
		EndingCash:     313967.48,
		PortfolioValue: 1046223.78,
		OutcomeCount:   26,
		BrokerRuntime: BrokerRuntimeAudit{
			Mode:             "dry-run",
			Adapter:          "guarded",
			Signer:           "placeholder",
			SignerVersion:    "v1",
			MaxRetries:       1,
			HTTPTimeoutSec:   5,
			HTTPAttempts:     2,
			RetryStatusCodes: []int{408, 429, 503},
			MaxClockSkewSec:  300,
			NonceTTLSec:      300,
			NonceStore:       "memory",
			NonceRedisPrefix: "atlas:nonce:",
		},
		GuardOutcomes: []GuardOutcome{{
			GuardID:     "cro-01",
			GuardSkill:  "cro_risk",
			Severity:    "hard",
			Passed:      true,
			Reason:      "ok",
			InputCount:  26,
			OutputCount: 24,
		}},
		RecordedAt: time.Date(2026, 4, 22, 4, 2, 30, 434394000, time.FixedZone("UTC+8", 8*60*60)),
	}

	first, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}

	var roundTripped SessionSummary
	if err := json.Unmarshal(first, &roundTripped); err != nil {
		t.Fatalf("unmarshal round trip: %v", err)
	}

	second, err := json.MarshalIndent(roundTripped, "", "  ")
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("expected canonical round trip to be stable\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if strings.Contains(string(second), `"SessionID"`) {
		t.Fatalf("expected canonical round trip not to contain PascalCase SessionID, got:\n%s", second)
	}
}
