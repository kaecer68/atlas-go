package main

import "testing"

func TestParseTriStateFlag(t *testing.T) {
	cases := []struct {
		in       string
		want     bool
		explicit bool
		wantErr  bool
	}{
		// Empty sentinel — must NOT be treated as override.
		{"", false, false, false},

		// Truthy variants.
		{"true", true, true, false},
		{"TRUE", true, true, false},
		{"True", true, true, false},
		{"1", true, true, false},
		{"yes", true, true, false},
		{"YES", true, true, false},

		// Falsy variants.
		{"false", false, true, false},
		{"False", false, true, false},
		{"0", false, true, false},
		{"no", false, true, false},

		// Whitespace tolerated.
		{"  true  ", true, true, false},
		{"\tfalse\n", false, true, false},

		// Invalid → error (fails fast).
		{"invalid", false, false, true},
		{"2", false, false, true},
		{"truthy", false, false, true},
		{"-1", false, false, true},
	}
	for _, c := range cases {
		got, exp, err := parseTriStateFlag(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseTriStateFlag(%q): expected error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTriStateFlag(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseTriStateFlag(%q) value = %v, want %v", c.in, got, c.want)
		}
		if exp != c.explicit {
			t.Errorf("parseTriStateFlag(%q) explicit = %v, want %v", c.in, exp, c.explicit)
		}
	}
}
