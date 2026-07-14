package industry

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventquality"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type stubProvider struct {
	mu     sync.Mutex
	events []marketdata.CalendarProviderData
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) FetchEvents(_ context.Context, _ int) ([]marketdata.CalendarProviderData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]marketdata.CalendarProviderData, len(s.events))
	copy(out, s.events)
	return out, nil
}

func TestEventCalendar_AcceptsAllWhenValidatorNil(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)
	if len(tec.events) == 0 {
		t.Fatal("default rules should generate events without validator")
	}
}

func TestEventCalendar_RefreshEventsFilteredByValidator(t *testing.T) {
	var buf bytes.Buffer
	tec := NewEventCalendar()
	validator := eventquality.NewValidator(eventquality.DateRange{
		PastBound:   -30 * 24 * time.Hour,
		FutureBound: 90 * 24 * time.Hour,
	}, 0)
	validator.SetClock(func() time.Time {
		return time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	})
	log := eventquality.NewQualityLog(&buf)
	tec.WithValidator(validator).WithQualityLog(log)

	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)
	t.Logf("RefreshEvents produced %d events, %d quality log entries", len(tec.events), strings.Count(strings.TrimSpace(buf.String()), "\n")+1)
	if len(tec.events) == 0 {
		t.Fatalf("expected events within 30d past / 90d future to pass validation, quality log: %s", buf.String())
	}
}

func TestEventCalendar_UpdateFromProviderFilteredByValidator(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	validator := eventquality.NewValidator(eventquality.DateRange{
		PastBound:   -30 * 24 * time.Hour,
		FutureBound: 90 * 24 * time.Hour,
	}, 0)
	validator.SetClock(func() time.Time { return now })

	provider := &stubProvider{
		events: []marketdata.CalendarProviderData{
			{Date: "2026-07-15", EventType: "earnings", Name: "Q2 earnings", Direction: "bullish", Weight: 0.8},
			{Date: "2026-12-25", EventType: "dividend", Name: "year-end", Direction: "neutral", Weight: 0.3},
		},
	}

	tec := NewEventCalendar()
	tec.WithValidator(validator)
	tec.generatedAt = now

	tec.UpdateFromProvider(context.Background(), provider)

	// 2026-07-15 is within 90d future (accept), 2026-12-25 is > 90d future (reject)
	if got, want := len(tec.events), 1; got != want {
		t.Errorf("expected %d accepted event, got %d (validator should reject 2026-12-25 as > 90d future)", want, got)
	}
	if len(tec.events) > 0 && !strings.Contains(tec.events[0].Name, "Q2") {
		t.Errorf("expected accepted event to be Q2 earnings, got %s", tec.events[0].Name)
	}
}

func TestEventCalendar_QualityLogRecordsRejection(t *testing.T) {
	var buf bytes.Buffer
	log := eventquality.NewQualityLog(&buf)

	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	validator := eventquality.NewValidator(eventquality.DateRange{
		PastBound:   -30 * 24 * time.Hour,
		FutureBound: 90 * 24 * time.Hour,
	}, 0)
	validator.SetClock(func() time.Time { return now })

	provider := &stubProvider{
		events: []marketdata.CalendarProviderData{
			{Date: "2026-12-25", EventType: "dividend", Name: "year-end", Direction: "neutral", Weight: 0.3},
		},
	}

	tec := NewEventCalendar()
	tec.WithValidator(validator).WithQualityLog(log)
	tec.generatedAt = now
	tec.UpdateFromProvider(context.Background(), provider)

	if len(tec.events) != 0 {
		t.Errorf("rejected event should not be in events, got %d", len(tec.events))
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 quality log line, got %d:\n%s", len(lines), buf.String())
	}
	var got eventquality.ValidationResult
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("quality log not valid JSON: %v", err)
	}
	if got.Accepted {
		t.Errorf("expected rejected, got accepted")
	}
	if got.Rule != "date_range" {
		t.Errorf("expected rule=date_range, got %s", got.Rule)
	}
}
