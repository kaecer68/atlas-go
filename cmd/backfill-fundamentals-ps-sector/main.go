package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	path := flag.String("file", "data/fundamentals.json", "fundamentals.json path")
	dryRun := flag.Bool("dry-run", false, "print summary without writing")
	flag.Parse()

	data, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	var m map[string]map[string]float64
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	// Inject PS=0 and Sector="" via separate maps; we need string values for Sector
	// Re-decode as map[string]map[string]any to preserve flexible types.
	var raw map[string]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "reparse: %v\n", err)
		os.Exit(1)
	}
	addedPS, addedSector := 0, 0
	for sym, entry := range raw {
		if entry == nil {
			entry = map[string]any{}
			raw[sym] = entry
		}
		if _, ok := entry["PS"]; !ok {
			entry["PS"] = 0.0
			addedPS++
		}
		if _, ok := entry["Sector"]; !ok {
			entry["Sector"] = ""
			addedSector++
		}
	}

	fmt.Printf("found %d symbols, added PS=%d Sector=%d (dry=%v)\n", len(raw), addedPS, addedSector, *dryRun)
	if *dryRun {
		return
	}

	// Stable output: sort keys
	type kvp struct {
		key string
		val map[string]any
	}
	out := make([]kvp, 0, len(raw))
	for k, v := range raw {
		out = append(out, kvp{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })

	out2, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(*path, out2, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	_ = filepath.Join // touch import
	fmt.Printf("wrote %s\n", *path)
}
