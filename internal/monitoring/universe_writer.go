// universe_writer.go — persists SmartUniverseBuilder output to
// data/state/universe.json in agents.json-compatible format with atomic write
// and version control.

package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Types ─────────────────────────────────────────────────────────────────

// ClassificationInfo carries per-industry classification overrides that
// callers may supply to customize the mapping from RankedSymbol.Industry to
// UniverseAgentSpec.Layer and UniverseAgentSpec.Skill. Zero-value fields
// signal "use the default derivation".
type ClassificationInfo struct {
	Layer string
	Skill string
}

// UniverseRegistry mirrors domain.AgentRegistry in shape but uses local types
// with explicit snake_case JSON tags so the output is directly compatible with
// the agents.json reader. Written to data/state/universe.json.
type UniverseRegistry struct {
	Version int                 `json:"version"`
	Agents  []UniverseAgentSpec `json:"agents"`
}

// UniverseAgentSpec describes one industry-scoped universe agent. Unlike
// domain.AgentSpec, every field carries an explicit snake_case JSON tag —
// this is CRITICAL because domain.AgentSpec has NO JSON tags on most fields,
// which would cause silent unmarshal failures in the agents.json reader.
type UniverseAgentSpec struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Layer           string   `json:"layer"`
	Skill           string   `json:"skill"`
	Enabled         bool     `json:"enabled"`
	Universe        []string `json:"universe"`
	DarwinianWeight float64  `json:"darwinian_weight,omitempty"`
}

// ── Conversion helper ─────────────────────────────────────────────────────

// rankedSymbolsToAgentSpecs converts ranked symbols into one
// UniverseAgentSpec per industry. Symbols are grouped by the Industry field
// of RankedSymbol; each group becomes one agent whose Universe lists the
// symbols in discovery order.
//
// The classification callback, when non-nil, supplies per-industry Layer and
// Skill overrides. Zero-value ClassificationInfo fields fall back to defaults:
// layer → "sector", skill → deriveIndustrySkill(industry).
func rankedSymbolsToAgentSpecs(ranked []RankedSymbol, classification func(string) ClassificationInfo) []UniverseAgentSpec {
	type industryGroup struct {
		symbols []string
	}
	groups := make(map[string]*industryGroup)
	var order []string // preserve discovery order for stable output

	for _, r := range ranked {
		industry := r.Industry
		if industry == "" {
			industry = "unclassified"
		}
		if _, ok := groups[industry]; !ok {
			groups[industry] = &industryGroup{}
			order = append(order, industry)
		}
		sym := normalizeSymbol(r.Symbol)
		groups[industry].symbols = append(groups[industry].symbols, sym)
	}

	specs := make([]UniverseAgentSpec, 0, len(order))
	for _, industry := range order {
		grp := groups[industry]

		info := ClassificationInfo{}
		if classification != nil {
			info = classification(industry)
		}

		layer := info.Layer
		if layer == "" {
			layer = "sector"
		}

		skill := info.Skill
		if skill == "" {
			skill = deriveIndustrySkill(industry)
		}

		name := industry
		id := "universe_" + skill

		specs = append(specs, UniverseAgentSpec{
			ID:              id,
			Name:            name,
			Layer:           layer,
			Skill:           skill,
			Enabled:         true,
			Universe:        grp.symbols,
			DarwinianWeight: 0.0,
		})
	}
	return specs
}

// deriveIndustrySkill converts an industry name into a skill identifier.
// Example: "Semiconductor" → "semiconductor_desk".
func deriveIndustrySkill(industry string) string {
	s := strings.ToLower(industry)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s + "_desk"
}

// ── Persistence ───────────────────────────────────────────────────────────

// WriteUniverseRegistry persists the SmartUniverseBuilder output as an
// agents.json-compatible universe registry with atomic write semantics.
//
// The result parameter is accepted for audit/logging but is not embedded in
// the JSON output (the output mirrors agents.json's Version + Agents shape
// exclusively).
//
// Atomic sequence (follows config.ParametersConfig.SaveWithRollback):
//  1. Marshal UniverseRegistry to indented JSON.
//  2. Write to path.tmp, fsync the temp file.
//  3. If path exists, rename it to path.bak.
//  4. Rename path.tmp → path.
//  5. On failure, restore from path.bak if it exists.
//  6. On success, remove path.bak.
func WriteUniverseRegistry(path string, result *UniverseBuildResult, ranked []RankedSymbol, version int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create universe registry parent dir: %w", err)
	}

	specs := rankedSymbolsToAgentSpecs(ranked, nil)
	registry := UniverseRegistry{
		Version: version,
		Agents:  specs,
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal universe registry: %w", err)
	}

	tmpPath := path + ".tmp"
	bakPath := path + ".bak"

	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		return fmt.Errorf("write temp universe registry: %w", err)
	}

	// Fsync the temp file to durable storage.
	f, err := os.OpenFile(tmpPath, os.O_RDONLY, 0)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("open temp file for sync: %w", err)
	}
	_ = f.Sync()
	_ = f.Close()

	// Backup existing file before replacing it.
	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Rename(path, bakPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("backup existing universe registry: %w", err)
		}
	}

	// Promote temp file to final path.
	if err := os.Rename(tmpPath, path); err != nil {
		if _, bakErr := os.Stat(bakPath); bakErr == nil {
			_ = os.Rename(bakPath, path)
		}
		return fmt.Errorf("promote temp universe registry: %w", err)
	}

	_ = os.Remove(bakPath)
	return nil
}

// LoadUniverseRegistry reads and unmarshals a universe.json file previously
// written by WriteUniverseRegistry. Returns an error if the file does not
// exist or is malformed.
func LoadUniverseRegistry(path string) (*UniverseRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read universe registry %q: %w", path, err)
	}
	var reg UniverseRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("unmarshal universe registry %q: %w", path, err)
	}
	return &reg, nil
}
