package main

import (
	"os"
	"testing"
)

func TestSplitComma(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{"2330,2454,2317", []string{"2330", "2454", "2317"}},
		{" a , b ,c", []string{" a ", " b ", "c"}},
	}
	for _, c := range cases {
		got := splitComma(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitComma(%q): want %v, got %v", c.in, c.want, got)
		}
	}
}

func TestTrimSpace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"  ", ""},
		{"a", "a"},
		{"  a  ", "a"},
		{"\t a \t", "a"},
	}
	for _, c := range cases {
		if got := trimSpace(c.in); got != c.want {
			t.Errorf("trimSpace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimSuffixTW(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2330", "2330"},
		{"2330.TW", "2330"},
		{"AAPL.US", "AAPL.US"}, // only .TW suffix is stripped
		{"", ""},
		{"TW", "TW"},
	}
	for _, c := range cases {
		if got := trimSuffixTW(c.in); got != c.want {
			t.Errorf("trimSuffixTW(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitSymbols(t *testing.T) {
	if got := splitSymbols(""); got != nil {
		t.Errorf("splitSymbols(\"\") = %v, want nil", got)
	}
	if got := splitSymbols("  "); got != nil {
		t.Errorf("splitSymbols(\"  \") = %v, want nil (empty after trim)", got)
	}
	got := splitSymbols("  2330 , 2454,  2317  ")
	want := []string{"2330", "2454", "2317"}
	if len(got) != len(want) {
		t.Fatalf("splitSymbols: want %v, got %v", want, got)
	}
	for i, s := range got {
		if s != want[i] {
			t.Errorf("splitSymbols[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestLoadSymbolsFromFundamentalsNotFound(t *testing.T) {
	if got := loadSymbolsFromFundamentals("/nonexistent/path/fundamentals.json"); got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}

func TestLoadSymbolsFromFundamentalsStripsTW(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/fundamentals.json"
	contents := `{"2330.TW": {"PE": 25.5}, "1101.TW": {"PE": 0}}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadSymbolsFromFundamentals(path)
	want := []string{"2330", "1101"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing symbol %q in result", w)
		}
	}
}

func TestFlagDefaults(t *testing.T) {
	if *startDate != "" {
		t.Errorf("startDate default = %q, want \"\"", *startDate)
	}
	if *endDate != "" {
		t.Errorf("endDate default = %q, want \"\"", *endDate)
	}
	if *symbols != "" {
		t.Errorf("symbols default = %q, want \"\"", *symbols)
	}
	if *workDir != "." {
		t.Errorf("workDir default = %q, want \".\"", *workDir)
	}
}
