package twse

import (
	"testing"
	"time"
)

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"normal number", "123.45", 123.45},
		{"with commas", "1,234.56", 1234.56},
		{"with whitespace", "  789.01  ", 789.01},
		{"empty string", "", 0},
		{"double dash", "--", 0},
		{"single dash", "-", 0},
		{"zero", "0", 0},
		{"negative number", "-5.5", -5.5},
		{"comma and whitespace", " 3,456.78 ", 3456.78},
		{"large number", "9999999.99", 9999999.99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFloat(tt.input)
			if got != tt.want {
				t.Errorf("ParseFloat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"normal number", "12345", 12345},
		{"with commas", "1,234,567", 1234567},
		{"with whitespace", "  78901  ", 78901},
		{"empty string", "", 0},
		{"double dash", "--", 0},
		{"single dash", "-", 0},
		{"zero", "0", 0},
		{"negative number", "-5000", -5000},
		{"comma and whitespace", " 3,456 ", 3456},
		{"large number", "9999999999", 9999999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInt64(tt.input)
			if got != tt.want {
				t.Errorf("ParseInt64(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{"valid date", "2026-06-14", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), false},
		{"valid date leap year", "2024-02-29", time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), false},
		{"valid date new year", "2026-01-01", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{"empty string", "", time.Time{}, true},
		{"invalid format", "2026/06/14", time.Time{}, true},
		{"invalid month", "2026-13-01", time.Time{}, true},
		{"partial date", "2026-06", time.Time{}, true},
		{"with whitespace", " 2026-06-14", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDate(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTradingDates(t *testing.T) {
	monday := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	tuesday := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	thursday := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)

	t.Run("single weekday", func(t *testing.T) {
		got := TradingDates(wednesday, wednesday)
		if len(got) != 1 || !got[0].Equal(wednesday) {
			t.Errorf("got %v, want [%v]", got, wednesday)
		}
	})

	t.Run("full week excluding weekends", func(t *testing.T) {
		got := TradingDates(monday, sunday)
		want := []time.Time{monday, tuesday, wednesday, thursday, friday}
		if len(got) != len(want) {
			t.Fatalf("got %d dates, want %d", len(got), len(want))
		}
		for i, d := range got {
			if !d.Equal(want[i]) {
				t.Errorf("got[%d] = %v, want %v", i, d, want[i])
			}
		}
	})

	t.Run("weekend only returns empty", func(t *testing.T) {
		got := TradingDates(saturday, sunday)
		if len(got) != 0 {
			t.Errorf("got %d dates, want 0", len(got))
		}
	})

	t.Run("start after end returns empty", func(t *testing.T) {
		got := TradingDates(sunday, monday)
		if len(got) != 0 {
			t.Errorf("got %d dates, want 0", len(got))
		}
	})

	t.Run("friday only", func(t *testing.T) {
		got := TradingDates(friday, friday)
		if len(got) != 1 || !got[0].Equal(friday) {
			t.Errorf("got %v, want [%v]", got, friday)
		}
	})

	t.Run("mon to fri returns 5 dates", func(t *testing.T) {
		got := TradingDates(monday, friday)
		if len(got) != 5 {
			t.Errorf("got %d dates, want 5", len(got))
		}
	})
}
