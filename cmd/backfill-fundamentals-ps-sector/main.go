package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

	// Decode as map[string]map[string]any to preserve flexible types
	// (PS is float64, Sector is string). The output must stay a
	// `map[string]FundamentalData` because FundamentalProvider.LoadFromJSON
	// unmarshals into a map — an array shape would break HasData() and
	// return 503 on every /api/stock/fundamentals request.
	var raw map[string]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
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

	// Emit the map directly. json.MarshalIndent sorts keys
	// alphabetically, so the produced file is deterministic.
	out, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(*path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *path)
}
