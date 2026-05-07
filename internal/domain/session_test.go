package domain

import (
	"encoding/json"
	"testing"
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
