package main

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than limit unchanged", "hello", 10, "hello"},
		{"exact limit unchanged", "hello", 5, "hello"},
		{"longer adds ellipsis", "hello world", 8, "hello..."},
		{"just over limit", "abcd", 3, "..."},
		{"empty string", "", 5, ""},
		{"long experiment id", "exec-growth-momentum-01-1774800459", 18, "exec-growth-mom..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncate result length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}
