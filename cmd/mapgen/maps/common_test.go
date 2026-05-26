package maps

import (
	"os"
	"strings"
	"testing"
)

func TestMapsDir(t *testing.T) {
	dir := MapsDir()
	if dir != ".omo/maps" {
		t.Errorf("expected .omo/maps, got %s", dir)
	}
}

func TestMarkdownTable(t *testing.T) {
	header := []string{"Module", "Score"}
	rows := [][]string{
		{"foo", "90%"},
		{"bar", "80%"},
	}
	got := MarkdownTable(header, rows)

	if !strings.HasPrefix(got, "|") {
		t.Errorf("expected table to start with '|', got %q", got[:1])
	}
	if !strings.Contains(got, "foo") {
		t.Errorf("expected table to contain 'foo', got:\n%s", got)
	}
	if !strings.Contains(got, "bar") {
		t.Errorf("expected table to contain 'bar', got:\n%s", got)
	}
}

func TestMarkdownTable_empty(t *testing.T) {
	got := MarkdownTable([]string{"H"}, [][]string{})
	if got != "_(no data)_\n" {
		t.Errorf("expected empty placeholder, got %q", got)
	}
}

func TestMarkdownTable_format(t *testing.T) {
	header := []string{"A", "B"}
	rows := [][]string{{"1", "2"}}
	got := MarkdownTable(header, rows)

	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + sep + row), got %d", len(lines))
	}

	// Header line
	if !strings.Contains(lines[0], "| A | B |") {
		t.Errorf("header mismatch: %s", lines[0])
	}

	// Separator line
	if !strings.Contains(lines[1], "---") {
		t.Errorf("separator missing '---': %s", lines[1])
	}

	// Row line
	if !strings.Contains(lines[2], "| 1 | 2 |") {
		t.Errorf("row mismatch: %s", lines[2])
	}
}

func TestPct(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.5, "50%"},
		{0.1234, "12%"},
		{1.0, "100%"},
		{0.0, "0%"},
		{0.999, "100%"},
	}
	for _, tc := range tests {
		got := Pct(tc.input)
		if got != tc.want {
			t.Errorf("Pct(%f) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestBoolMark(t *testing.T) {
	if BoolMark(true) != "✅" {
		t.Errorf("expected checkmark for true")
	}
	if BoolMark(false) != "❌" {
		t.Errorf("expected cross for false")
	}
}

func TestHeader(t *testing.T) {
	h := Header("Test Map")
	if !strings.HasPrefix(h, "# Test Map\n") {
		t.Errorf("header missing title: %s", h)
	}
	if !strings.Contains(h, "Generated:") {
		t.Errorf("header missing 'Generated:' timestamp: %s", h)
	}
}

func TestHeader_emptyTitle(t *testing.T) {
	h := Header("")
	if !strings.Contains(h, "# ") {
		t.Errorf("header should contain title even if empty")
	}
}

func TestWriteMap(t *testing.T) {
	// Write a temp map file and verify it persists
	err := WriteMap("_test_write.md", "# Test Content\n")
	if err != nil {
		t.Fatalf("WriteMap failed: %v", err)
	}

	// Read back
	data, err := os.ReadFile(".omo/maps/_test_write.md")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "# Test Content\n" {
		t.Errorf("content mismatch: %q", string(data))
	}

	// Cleanup
	os.Remove(".omo/maps/_test_write.md")
}

func TestWriteMap_overwrite(t *testing.T) {
	// Overwriting an existing file should work
	err := WriteMap("_test_overwrite.md", "original")
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	err = WriteMap("_test_overwrite.md", "updated")
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	data, _ := os.ReadFile(".omo/maps/_test_overwrite.md")
	if string(data) != "updated" {
		t.Errorf("expected 'updated', got %q", data)
	}
	os.Remove(".omo/maps/_test_overwrite.md")
}

func TestLoadPrevious_notfound(t *testing.T) {
	got := LoadPrevious("_nonexistent_file.md")
	if got != "" {
		t.Errorf("expected empty for nonexistent, got %q", got)
	}
}

func TestLoadPrevious_found(t *testing.T) {
	// Write a file then load it
	_ = WriteMap("_test_load.md", "prev content")
	got := LoadPrevious("_test_load.md")
	if got != "prev content" {
		t.Errorf("expected 'prev content', got %q", got)
	}
	os.Remove(".omo/maps/_test_load.md")
}

func BenchmarkMarkdownTable(b *testing.B) {
	header := []string{"A", "B", "C"}
	rows := [][]string{
		{"1", "2", "3"},
		{"4", "5", "6"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarkdownTable(header, rows)
	}
}

// Verify types implement expected interfaces
func TestTypes_compile(t *testing.T) {
	_ = ModuleInfo{Name: "test"}
	_ = RouteInfo{Pattern: "/api/test"}
	_ = CompletenessReport{Module: "test"}
	_ = FrontendPage{Name: "test"}
	_ = FEBEMapping{BackendRoute: "/api/test"}
	_ = MapMeta{Title: "test"}
}
