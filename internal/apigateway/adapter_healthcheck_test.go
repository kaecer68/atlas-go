package apigateway

import (
	"context"
	"testing"
)

func TestExchangeRateChannelAdapter_HealthCheck(t *testing.T) {
	a := &ExchangeRateChannelAdapter{}
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.CheckType != "liveness" {
		t.Errorf("CheckType = %q, want liveness", status.CheckType)
	}
}

func TestSOXIndexChannelAdapter_HealthCheck(t *testing.T) {
	a := &SOXIndexChannelAdapter{}
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestSectorDataChannelAdapter_HealthCheck(t *testing.T) {
	a := &SectorDataChannelAdapter{}
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.CheckType != "readiness" {
		t.Errorf("CheckType = %q, want readiness", status.CheckType)
	}
}

func TestDayTradingChannelAdapter_HealthCheck(t *testing.T) {
	a := &DayTradingChannelAdapter{}
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

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
