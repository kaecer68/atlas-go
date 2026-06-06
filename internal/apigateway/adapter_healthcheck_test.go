package apigateway

import (
	"context"
	"testing"
)

// HealthCheck tests for channels that previously returned fake "ok" responses
// have been removed. Real HealthChecks require actual network connectivity and
// cannot be meaningfully unit-tested with zero-value structs.
// Integration tests in the CI pipeline verify HealthCheck behavior against
// real API endpoints.

func TestJANUSRegimeChannelAdapter_HealthCheck_NilEngine(t *testing.T) {
	a := &JANUSRegimeChannelAdapter{}
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if status.Status != "inactive" {
		t.Errorf("Status = %q, want inactive", status.Status)
	}
	if status.CheckType != "computed" {
		t.Errorf("CheckType = %q, want computed", status.CheckType)
	}
}

func TestJANUSRegimeChannelAdapter_Fetch_NilEngine(t *testing.T) {
	a := &JANUSRegimeChannelAdapter{}
	_, err := a.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch with nil engine should return error")
	}
}
