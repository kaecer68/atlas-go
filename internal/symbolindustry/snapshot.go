package symbolindustry

import (
	"encoding/json"
	"fmt"
	"os"
)

// SnapshotCounts is the channel's self-reported population summary.
type SnapshotCounts struct {
	Total       int `json:"total"`
	Mapped      int `json:"mapped"`
	Unmapped    int `json:"unmapped"`
	Unknown     int `json:"unknown"`
	CanonicalL1 int `json:"canonical_l1"`
}

// Snapshot is the parsed payload of the symbol_industry channel state file
// (data/state/symbol_industry.json).
type Snapshot struct {
	Channel   string         `json:"channel"`
	UpdatedAt string         `json:"updated_at"`
	Sources   []string       `json:"sources"`
	Counts    SnapshotCounts `json:"counts"`
	Entries   []Entry        `json:"entries"`
}

// ChannelName is the channel id that must appear in a valid snapshot.
const ChannelName = "symbol_industry"

// ReadSnapshot reads and validates the channel state file. Unknown JSON keys
// are ignored (the channel writes extra reporting blocks).
//
// Validation is deliberately strict: a missing file, a foreign channel name
// or an empty entry list is an error, never a silent zero value. Treating an
// empty payload as success is the #1953 DegradedOnEmpty class of bug.
func ReadSnapshot(path string) (Snapshot, error) {
	var snap Snapshot
	raw, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("symbolindustry: read snapshot %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("symbolindustry: parse snapshot %s: %w", path, err)
	}
	if snap.Channel != ChannelName {
		return Snapshot{}, fmt.Errorf(
			"symbolindustry: snapshot %s has channel %q, want %q",
			path, snap.Channel, ChannelName)
	}
	if len(snap.Entries) == 0 {
		return Snapshot{}, fmt.Errorf("symbolindustry: snapshot %s has no entries", path)
	}

	// Drop entries without a symbol and keep the first occurrence of each
	// duplicated symbol.
	clean, _ := dedupeEntries(snap.Entries)
	snap.Entries = clean
	if len(snap.Entries) == 0 {
		return Snapshot{}, fmt.Errorf("symbolindustry: snapshot %s has no entries", path)
	}
	return snap, nil
}

// dedupeStats reports what was filtered out of a snapshot payload. The frozen
// Snapshot shape carries no field for it, so it is returned for callers (and
// tests) that want the diagnostic.
type dedupeStats struct {
	DroppedEmptySymbol int
	Duplicates         int
}

// dedupeEntries drops entries with an empty Symbol and keeps only the first
// entry for each Symbol.
func dedupeEntries(entries []Entry) ([]Entry, dedupeStats) {
	var stats dedupeStats
	seen := make(map[string]struct{}, len(entries))
	clean := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Symbol == "" {
			stats.DroppedEmptySymbol++
			continue
		}
		if _, dup := seen[e.Symbol]; dup {
			stats.Duplicates++
			continue
		}
		seen[e.Symbol] = struct{}{}
		clean = append(clean, e)
	}
	return clean, stats
}
