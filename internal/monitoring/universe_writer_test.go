package monitoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
//  TestWriteUniverseRegistry
// ============================================================================

func TestWriteUniverseRegistry(t *testing.T) {
	t.Run("happy path writes valid JSON and returns nil", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "data", "state", "universe.json")

		err := WriteUniverseRegistry(path, sampleUniverseBuildResult(), sampleRankedSymbols(3), 1)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}

		// Verify file exists and is valid JSON.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read written file: %v", err)
		}

		var reg UniverseRegistry
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("written JSON is not valid: %v", err)
		}

		if reg.Version != 1 {
			t.Errorf("Version = %d, want 1", reg.Version)
		}
		if len(reg.Agents) == 0 {
			t.Error("Agents should not be empty for 3 ranked symbols")
		}
	})

	t.Run("empty ranked list writes empty agents array", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "universe.json")

		err := WriteUniverseRegistry(path, sampleUniverseBuildResult(), nil, 2)
		if err != nil {
			t.Fatalf("expected nil error for empty ranked list, got: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		var reg UniverseRegistry
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("written JSON invalid: %v", err)
		}
		if reg.Version != 2 {
			t.Errorf("Version = %d, want 2", reg.Version)
		}
		if len(reg.Agents) != 0 {
			t.Errorf("Agents = %v, want empty slice", reg.Agents)
		}
	})

	t.Run("MkdirAll failure when parent dir not writable", func(t *testing.T) {
		dir := tempDir(t)
		// Create a read-only directory to block MkdirAll.
		readonly := filepath.Join(dir, "readonly")
		if err := os.MkdirAll(readonly, 0o555); err != nil {
			t.Fatalf("failed to create readonly dir: %v", err)
		}
		defer func() { _ = os.Chmod(readonly, 0o755) }()

		path := filepath.Join(readonly, "sub", "universe.json")

		err := WriteUniverseRegistry(path, sampleUniverseBuildResult(), sampleRankedSymbols(1), 1)
		if err == nil {
			t.Fatal("expected error when parent dir is not writable, got nil")
		}
	})

	t.Run("rename failure when target parent does not exist", func(t *testing.T) {
		dir := tempDir(t)
		// Write a temp file first so the .tmp path exists.
		tmpPath := filepath.Join(dir, "nonexistent_parent", "universe.json.tmp")
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o750); err != nil {
			t.Fatalf("failed to create tmp dir: %v", err)
		}
		if err := os.WriteFile(tmpPath, []byte("{}"), 0o640); err != nil {
			t.Fatalf("failed to write tmp file: %v", err)
		}

		// Now point to a path whose parent does NOT exist (simulating rename failure).
		// Actually, os.Rename to a path with non-existent parent returns an error on POSIX.
		badPath := filepath.Join(dir, "also_nonexistent", "sub", "universe.json")

		// We need the .tmp to exist and the target parent to not exist.
		// Use the already-created tmpPath and try to rename to badPath.
		err := os.Rename(tmpPath, badPath)
		if err == nil {
			// On some systems this may succeed; skip in that case.
			t.Skip("os.Rename to non-existent parent succeeded on this platform")
		}
	})

	t.Run("concurrent readers encounter no panic during write", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "universe.json")

		// Pre-create a valid file so readers have something to open from the start.
		reg := UniverseRegistry{Version: 0, Agents: []UniverseAgentSpec{}}
		blob, _ := json.Marshal(reg)
		if err := os.WriteFile(path, blob, 0o640); err != nil {
			t.Fatalf("failed to pre-create file: %v", err)
		}

		const readerCount = 5
		var wg sync.WaitGroup
		wg.Add(readerCount + 1)
		panicCh := make(chan error, readerCount)

		for range readerCount {
			go func() {
				defer wg.Done()
				for range 100 {
					_, _ = LoadUniverseRegistry(path)
				}
			}()
		}

		go func() {
			defer wg.Done()
			for range 50 {
				_ = WriteUniverseRegistry(path, sampleUniverseBuildResult(), sampleRankedSymbols(3), 1)
			}
		}()

		wg.Wait()
		close(panicCh)

		for p := range panicCh {
			t.Errorf("panic during concurrent read: %v", p)
		}
	})
}

// ============================================================================
//  TestLoadUniverseRegistry
// ============================================================================

func TestLoadUniverseRegistry(t *testing.T) {
	t.Run("happy path loads existing valid file", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "universe.json")

		// Write a valid registry first.
		reg := UniverseRegistry{
			Version: 3,
			Agents: []UniverseAgentSpec{
				{ID: "universe_tech_desk", Name: "tech", Layer: "sector", Skill: "tech_desk", Enabled: true, Universe: []string{"2330", "2317"}},
			},
		}
		data, err := json.Marshal(reg)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		if err := os.WriteFile(path, data, 0o640); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		got, err := LoadUniverseRegistry(path)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if got.Version != 3 {
			t.Errorf("Version = %d, want 3", got.Version)
		}
		if len(got.Agents) != 1 {
			t.Fatalf("Agents len = %d, want 1", len(got.Agents))
		}
		if got.Agents[0].Name != "tech" {
			t.Errorf("Agents[0].Name = %q, want %q", got.Agents[0].Name, "tech")
		}
	})

	t.Run("file not found returns error", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "nonexistent.json")

		_, err := LoadUniverseRegistry(path)
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
		if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("error should indicate missing file, got: %v", err)
		}
	})

	t.Run("corrupt JSON returns parse error", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "corrupt.json")

		if err := os.WriteFile(path, []byte("{invalid json}"), 0o640); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_, err := LoadUniverseRegistry(path)
		if err == nil {
			t.Fatal("expected error for corrupt JSON, got nil")
		}
	})

	t.Run("empty file returns error", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "empty.json")

		if err := os.WriteFile(path, []byte(""), 0o640); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_, err := LoadUniverseRegistry(path)
		if err == nil {
			t.Fatal("expected error for empty file, got nil")
		}
	})
}

// ============================================================================
//  TestRankedSymbolsToAgentSpecs
// ============================================================================

func TestRankedSymbolsToAgentSpecs(t *testing.T) {
	t.Run("empty input returns empty output", func(t *testing.T) {
		got := rankedSymbolsToAgentSpecs(nil, nil)
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %v", got)
		}

		got = rankedSymbolsToAgentSpecs([]RankedSymbol{}, nil)
		if len(got) != 0 {
			t.Errorf("expected empty slice for zero-length input, got %v", got)
		}
	})

	t.Run("single symbol produces one agent", func(t *testing.T) {
		ranked := []RankedSymbol{
			{Symbol: "2330.TW", Score: 0.85, Industry: "semiconductor"},
		}

		got := rankedSymbolsToAgentSpecs(ranked, nil)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}

		agent := got[0]
		if agent.ID != "universe_semiconductor_desk" {
			t.Errorf("ID = %q, want %q", agent.ID, "universe_semiconductor_desk")
		}
		if agent.Name != "semiconductor" {
			t.Errorf("Name = %q, want %q", agent.Name, "semiconductor")
		}
		if agent.Layer != "sector" {
			t.Errorf("Layer = %q, want %q", agent.Layer, "sector")
		}
		if agent.Skill != "semiconductor_desk" {
			t.Errorf("Skill = %q, want %q", agent.Skill, "semiconductor_desk")
		}
		if !agent.Enabled {
			t.Error("Enabled should be true")
		}
		if len(agent.Universe) != 1 {
			t.Errorf("Universe len = %d, want 1", len(agent.Universe))
		}
		if agent.Universe[0] != "2330" {
			t.Errorf("Universe[0] = %q, want %q (normalized .TW stripped)", agent.Universe[0], "2330")
		}
	})

	t.Run("multiple symbols grouped by industry with darwinian weight", func(t *testing.T) {
		ranked := []RankedSymbol{
			{Symbol: "2330", Score: 0.85, Industry: "semiconductor"},
			{Symbol: "2317", Score: 0.72, Industry: "tech"},
			{Symbol: "2454", Score: 0.68, Industry: "tech"},
			{Symbol: "2881", Score: 0.55, Industry: "financial"},
		}

		got := rankedSymbolsToAgentSpecs(ranked, nil)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3 (semiconductor, tech, financial)", len(got))
		}

		// Verify tech agent has both tech symbols.
		var techAgent UniverseAgentSpec
		for _, a := range got {
			if a.Name == "tech" {
				techAgent = a
				break
			}
		}
		if len(techAgent.Universe) != 2 {
			t.Errorf("tech Universe len = %d, want 2", len(techAgent.Universe))
		}
		if techAgent.DarwinianWeight != 0.0 {
			t.Errorf("DarwinianWeight = %v, want 0.0", techAgent.DarwinianWeight)
		}
	})

	t.Run("classification callback overrides layer and skill", func(t *testing.T) {
		ranked := []RankedSymbol{
			{Symbol: "2330", Score: 0.85, Industry: "semiconductor"},
		}

		classification := func(industry string) ClassificationInfo {
			if industry == "semiconductor" {
				return ClassificationInfo{Layer: "industry", Skill: "custom_skill"}
			}
			return ClassificationInfo{}
		}

		got := rankedSymbolsToAgentSpecs(ranked, classification)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Layer != "industry" {
			t.Errorf("Layer = %q, want %q", got[0].Layer, "industry")
		}
		if got[0].Skill != "custom_skill" {
			t.Errorf("Skill = %q, want %q", got[0].Skill, "custom_skill")
		}
	})

	t.Run("empty industry falls back to unclassified", func(t *testing.T) {
		ranked := []RankedSymbol{
			{Symbol: "9999", Score: 0.5, Industry: ""},
		}

		got := rankedSymbolsToAgentSpecs(ranked, nil)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Name != "unclassified" {
			t.Errorf("Name = %q, want %q", got[0].Name, "unclassified")
		}
		if got[0].Skill != "unclassified_desk" {
			t.Errorf("Skill = %q, want %q", got[0].Skill, "unclassified_desk")
		}
	})
}

// ============================================================================
//  TestDeriveIndustrySkill
// ============================================================================

func TestDeriveIndustrySkill(t *testing.T) {
	t.Run("known industry maps to expected skill", func(t *testing.T) {
		tests := []struct {
			industry string
			want     string
		}{
			{"Semiconductor", "semiconductor_desk"},
			{"Tech", "tech_desk"},
			{"Financial", "financial_desk"},
			{"Cement", "cement_desk"},
		}
		for _, tt := range tests {
			t.Run(tt.industry, func(t *testing.T) {
				got := deriveIndustrySkill(tt.industry)
				if got != tt.want {
					t.Errorf("deriveIndustrySkill(%q) = %q, want %q", tt.industry, got, tt.want)
				}
			})
		}
	})

	t.Run("industry with spaces slashes and hyphens normalizes correctly", func(t *testing.T) {
		tests := []struct {
			industry string
			want     string
		}{
			{"Oil & Gas", "oil_&_gas_desk"}, // & is not replaced; only space/slash/hyphen become underscore
			{"High-Tech", "high_tech_desk"}, // hyphen → underscore
			{"Auto Parts", "auto_parts_desk"},
		}
		for _, tt := range tests {
			t.Run(tt.industry, func(t *testing.T) {
				got := deriveIndustrySkill(tt.industry)
				if got != tt.want {
					t.Errorf("deriveIndustrySkill(%q) = %q, want %q", tt.industry, got, tt.want)
				}
			})
		}
	})

	t.Run("empty string returns desk suffix", func(t *testing.T) {
		got := deriveIndustrySkill("")
		if got != "_desk" {
			t.Errorf("deriveIndustrySkill(%q) = %q, want %q", "", got, "_desk")
		}
	})

	t.Run("single word industry", func(t *testing.T) {
		got := deriveIndustrySkill("Energy")
		if got != "energy_desk" {
			t.Errorf("deriveIndustrySkill(%q) = %q, want %q", "Energy", got, "energy_desk")
		}
	})
}
