package domain

import "testing"

func TestScreeningRejectStruct(t *testing.T) {
	reject := ScreeningReject{
		SessionID:      "session-20260101-daily",
		Symbol:         "2330.TW",
		AgentID:        "semi-desk-01",
		Skill:          "semiconductor_desk",
		Criterion:      "volume_intraday_min",
		CriterionLabel: "Volume ≥ 1,000,000",
		Threshold:      "1000000",
		ActualValue:    "850000",
	}
	if reject.Symbol != "2330.TW" {
		t.Fatal("unexpected symbol")
	}
}
