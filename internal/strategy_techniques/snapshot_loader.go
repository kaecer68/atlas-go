package strategy_techniques

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// datedSnapshotPattern matches YYYY-MM-DD.json (same regex as
// monitoring/service/macro.go to keep snapshot loading consistent).
var datedSnapshotRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.json$`)

// loadSnapshotsFromDir reads dated MacroDataSnapshot JSON files from a
// directory, sorted by date ascending. Non-date files and parse errors
// are silently skipped.
func loadSnapshotsFromDir(dir string) ([]marketdata.MacroDataSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type dated struct {
		date string
		snap marketdata.MacroDataSnapshot
	}
	var datedSnaps []dated

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !datedSnapshotRe.MatchString(e.Name()) {
			continue
		}
		date := e.Name()[:10] // strip .json

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		datedSnaps = append(datedSnaps, dated{date: date, snap: snap})
	}

	sort.Slice(datedSnaps, func(i, j int) bool {
		return datedSnaps[i].date < datedSnaps[j].date
	})

	result := make([]marketdata.MacroDataSnapshot, len(datedSnaps))
	for i, d := range datedSnaps {
		result[i] = d.snap
	}
	return result, nil
}
