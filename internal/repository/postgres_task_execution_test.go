package repository

import (
	"testing"
)

func TestMustJSON(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, `42`},
		{"map", map[string]int{"a": 1}, `{"a":1}`},
		{"nil", nil, `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(mustJSON(tt.v))
			if got != tt.want {
				t.Errorf("mustJSON(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}
