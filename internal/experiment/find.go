package experiment

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// FindLatestExperiment discovers the most recent experiment JSON file
// by sorting filenames by embedded Unix timestamp suffix.
// Files named "test-experiment.json" are excluded.
func FindLatestExperiment(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" && name != "test-experiment.json" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Slice(files, func(i, j int) bool {
		return extractTimestamp(files[i]) > extractTimestamp(files[j])
	})
	return filepath.Join(dir, files[0])
}

// extractTimestamp parses the last hyphen-separated segment of a filename
// as a Unix timestamp. Returns 0 if parsing fails.
func extractTimestamp(filename string) int64 {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	parts := strings.Split(base, "-")
	if len(parts) > 0 {
		if ts, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil {
			return ts
		}
	}
	return 0
}
