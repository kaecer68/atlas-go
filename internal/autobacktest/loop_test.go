package autobacktest

import (
	"context"
	"testing"
	"time"
)

func TestStartDailyLoop_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	StartDailyLoop(ctx, nil)
}

func TestRunScheduledBacktest_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RunScheduledBacktest(ctx, nil)
	if err == nil {
		t.Error("RunScheduledBacktest with cancelled ctx should return error")
	}
}

func TestNext13_30Weekday(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "before 13:30 same day",
			input:    time.Date(2026, 4, 1, 12, 0, 0, 0, loc),
			expected: time.Date(2026, 4, 1, 13, 30, 0, 0, loc),
		},
		{
			name:     "at 13:30 same day",
			input:    time.Date(2026, 4, 1, 13, 30, 0, 0, loc),
			expected: time.Date(2026, 4, 2, 13, 30, 0, 0, loc),
		},
		{
			name:     "after 13:30 next day",
			input:    time.Date(2026, 4, 1, 14, 0, 0, 0, loc),
			expected: time.Date(2026, 4, 2, 13, 30, 0, 0, loc),
		},
		{
			name:     "friday after 13:30 skips weekend",
			input:    time.Date(2026, 4, 3, 14, 0, 0, 0, loc),
			expected: time.Date(2026, 4, 6, 13, 30, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := next13_30(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("next13_30(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNext13_30Weekend(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")

	saturday := time.Date(2026, 4, 4, 10, 0, 0, 0, loc)
	sunday := time.Date(2026, 4, 5, 10, 0, 0, 0, loc)

	if got := next13_30(saturday); got.Weekday() != time.Monday {
		t.Errorf("expected Monday for Saturday input, got %v", got.Weekday())
	}
	if got := next13_30(sunday); got.Weekday() != time.Monday {
		t.Errorf("expected Monday for Sunday input, got %v", got.Weekday())
	}
}
