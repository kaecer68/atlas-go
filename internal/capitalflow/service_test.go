package capitalflow

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type stubProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return s.snap, s.err
}

func TestService_LatestDaily_AssemblesReport(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	svc := NewService(provider, 0)
	report, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	if report.Date.Unix() != recordedAt {
		t.Errorf("Date = %v, want %v", report.Date.Unix(), recordedAt)
	}
	if len(report.Forces) == 0 {
		t.Errorf("Forces empty after Extract")
	}
	if report.Resonance.Direction == "" {
		t.Errorf("Resonance.Direction empty after ComputeResonance")
	}
}

func TestService_LatestDaily_ProviderError(t *testing.T) {
	provider := &stubProvider{err: context.DeadlineExceeded}
	svc := NewService(provider, 0)
	if _, err := svc.LatestDaily(context.Background()); err == nil {
		t.Errorf("expected error from provider")
	}
}
