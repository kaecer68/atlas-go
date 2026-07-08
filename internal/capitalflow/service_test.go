package capitalflow

import (
	"context"
	"errors"
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

func TestService_Summary_DerivesFromLatestDaily(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	svc := NewService(provider, 0)
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Date.Unix() != recordedAt {
		t.Errorf("Date = %v, want %v", summary.Date.Unix(), recordedAt)
	}
	if summary.QualityLabel == "" {
		t.Errorf("QualityLabel empty after GenerateSummaryReport")
	}
	if summary.DominantForce == "" {
		t.Errorf("DominantForce empty after GenerateSummaryReport")
	}
	if summary.ResonanceDir == "" {
		t.Errorf("ResonanceDir empty after GenerateSummaryReport")
	}
}

func TestService_Summary_PropagatesProviderError(t *testing.T) {
	provider := &stubProvider{err: context.DeadlineExceeded}
	svc := NewService(provider, 0)
	_, err := svc.Summary(context.Background())
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Summary error does not wrap context.DeadlineExceeded: %v", err)
	}
}

func TestService_Summary_SharesForcesWithDailyReport(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 200},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -100},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 50},
	}}
	svc := NewService(provider, 0)
	daily, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !daily.Date.Equal(summary.Date) {
		t.Errorf("Date mismatch: daily=%v summary=%v", daily.Date, summary.Date)
	}
	if daily.Resonance.Direction != summary.ResonanceDir {
		t.Errorf("ResonanceDir mismatch: daily=%q summary=%q",
			daily.Resonance.Direction, summary.ResonanceDir)
	}
	if daily.QualityScore != summary.QualityScore {
		t.Errorf("QualityScore mismatch: daily=%v summary=%v",
			daily.QualityScore, summary.QualityScore)
	}
}
