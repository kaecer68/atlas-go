package industry

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

func TestSectorSymbolsJSON(t *testing.T) {
	data, err := os.ReadFile("../../configs/sector_symbols.json")
	if err != nil {
		t.Fatalf("failed to read sector_symbols.json: %v", err)
	}

	var sectors map[string][]string
	if err := json.Unmarshal(data, &sectors); err != nil {
		t.Fatalf("failed to parse sector_symbols.json: %v", err)
	}

	required := []string{"foundry", "server_assembly", "cooling", "robotics", "energy"}
	for _, id := range required {
		symbols, ok := sectors[id]
		if !ok {
			t.Errorf("missing sector entry: %s", id)
			continue
		}
		if len(symbols) == 0 {
			t.Errorf("sector %s has empty symbol list (expected >= 1)", id)
		}
	}

	graphIndustries := []string{
		"semiconductor", "foundry", "ai_supply_chain", "server_assembly",
		"cooling", "robotics", "shipping", "financials", "energy",
	}
	for _, id := range graphIndustries {
		if _, ok := sectors[id]; !ok {
			t.Errorf("supply-chain-graph industry %s has no sector_symbols entry", id)
		}
	}
}

func TestSectorSymbolsNonEmpty(t *testing.T) {
	data, err := os.ReadFile("../../configs/sector_symbols.json")
	if err != nil {
		t.Fatalf("failed to read sector_symbols.json: %v", err)
	}

	var sectors map[string][]string
	if err := json.Unmarshal(data, &sectors); err != nil {
		t.Fatalf("failed to parse sector_symbols.json: %v", err)
	}

	knownEmpty := map[string]bool{"tourism": true}

	for id, symbols := range sectors {
		if knownEmpty[id] {
			continue
		}
		if len(symbols) == 0 {
			t.Errorf("sector %s has no symbols (expected >= 1)", id)
		}
	}
}

func TestSectorSymbolFormat(t *testing.T) {
	data, err := os.ReadFile("../../configs/sector_symbols.json")
	if err != nil {
		t.Fatalf("failed to read sector_symbols.json: %v", err)
	}

	var sectors map[string][]string
	if err := json.Unmarshal(data, &sectors); err != nil {
		t.Fatalf("failed to parse sector_symbols.json: %v", err)
	}

	formatRe := regexp.MustCompile(`^\d{4,5}\.TW$`)

	for id, symbols := range sectors {
		for _, sym := range symbols {
			if !formatRe.MatchString(sym) {
				t.Errorf("sector %s: symbol %q does not match ^\\d{4}\\.TW$", id, sym)
			}
		}
	}
}

func TestSupplyChainGraphCoverage(t *testing.T) {
	data, err := os.ReadFile("../../configs/sector_symbols.json")
	if err != nil {
		t.Fatalf("failed to read sector_symbols.json: %v", err)
	}

	var sectors map[string][]string
	if err := json.Unmarshal(data, &sectors); err != nil {
		t.Fatalf("failed to parse sector_symbols.json: %v", err)
	}

	_, _, err = LoadSupplyChainGraph("../../configs/supply_chain_graph.json")
	if err != nil {
		t.Fatalf("failed to load supply chain graph: %v", err)
	}

	graphIndustries := []string{
		"semiconductor", "foundry", "ai_supply_chain", "server_assembly",
		"cooling", "robotics", "shipping", "financials", "energy",
	}
	for _, id := range graphIndustries {
		if _, ok := sectors[id]; !ok {
			t.Errorf("graph industry %s has no sector_symbols entry", id)
		}
	}
}
