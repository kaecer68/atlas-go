package maps

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const mapsDir = ".omo/maps"

// MapsDir returns the absolute path to the maps output directory.
func MapsDir() string {
	return mapsDir
}

// WriteMap writes content to a markdown file in the maps directory.
// It ensures the directory exists.
func WriteMap(filename, content string) error {
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		return fmt.Errorf("create maps dir: %w", err)
	}
	path := filepath.Join(mapsDir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}

// Header returns a standard map header with metadata.
func Header(title string) string {
	now := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	return fmt.Sprintf(`# %s
> Generated: %s

`, title, now)
}

// LoadPrevious attempts to read the previous version of a map file.
// Returns empty string if not found.
func LoadPrevious(filename string) string {
	path := filepath.Join(mapsDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// MarkdownTable formats a slice of rows into a markdown table.
func MarkdownTable(header []string, rows [][]string) string {
	if len(rows) == 0 {
		return "_(no data)_\n"
	}

	var b []byte
	// Header
	b = append(b, "|"...)
	for _, h := range header {
		b = append(b, " "...)
		b = append(b, h...)
		b = append(b, " |"...)
	}
	b = append(b, '\n')

	// Separator
	b = append(b, "|"...)
	for range header {
		b = append(b, " --- |"...)
	}
	b = append(b, '\n')

	// Rows
	for _, row := range rows {
		b = append(b, "|"...)
		for _, cell := range row {
			b = append(b, " "...)
			b = append(b, cell...)
			b = append(b, " |"...)
		}
		b = append(b, '\n')
	}
	return string(b)
}

// Pct formats a float as a percentage string.
func Pct(v float64) string {
	return fmt.Sprintf("%.0f%%", v*100)
}

// BoolMark returns ✅ for true, ❌ for false.
func BoolMark(v bool) string {
	if v {
		return "✅"
	}
	return "❌"
}
