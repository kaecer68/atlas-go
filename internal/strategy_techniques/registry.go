// Package strategy_techniques — registry.go
//
// Registry loads StrategyFrame seeds from a JSON file and provides
// lookup helpers. The JSON schema is the single source of truth for
// the 12 production seed rules (9 L1-L5 originals + 3 L4 extensions)
// format, plus 3 L5 geopolitical seeds). New seeds can be added by
// editing data/seeds/strategy_techniques.json without recompiling.
//
// Production path: data/seeds/strategy_techniques.json
// Test path:      internal/strategy_techniques/testdata/seed.json
package strategy_techniques

import (
	"encoding/json"
	"fmt"
	"os"
)

// Registry holds the loaded StrategyFrame seeds.
type Registry struct {
	Frames []StrategyFrame `json:"frames"`
}

// LoadFromFile reads a JSON file from disk and returns a validated Registry.
//
// The file must contain a JSON array of StrategyFrame objects. All frames
// are validated via StrategyFrame.Validate(); the first invalid frame
// causes LoadFromFile to return an error.
func LoadFromFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("strategy_techniques: read %q: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses a JSON byte slice and returns a validated Registry.
func LoadFromBytes(data []byte) (*Registry, error) {
	var reg Registry
	if err := json.Unmarshal(data, &reg.Frames); err != nil {
		return nil, fmt.Errorf("strategy_techniques: unmarshal: %w", err)
	}
	for i := range reg.Frames {
		if err := reg.Frames[i].Validate(); err != nil {
			return nil, fmt.Errorf("strategy_techniques: frame[%d] %q invalid: %w", i, reg.Frames[i].ID, err)
		}
	}
	return &reg, nil
}

// Count returns the number of frames in the registry.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.Frames)
}

// All returns a copy of the frames slice (callers cannot mutate registry).
func (r *Registry) All() []StrategyFrame {
	if r == nil {
		return nil
	}
	out := make([]StrategyFrame, len(r.Frames))
	copy(out, r.Frames)
	return out
}

// FindByID returns the frame with the given id, or an error if not found.
func (r *Registry) FindByID(id string) (*StrategyFrame, error) {
	if r == nil {
		return nil, fmt.Errorf("strategy_techniques: nil registry")
	}
	for i := range r.Frames {
		if r.Frames[i].ID == id {
			return &r.Frames[i], nil
		}
	}
	return nil, fmt.Errorf("strategy_techniques: frame %q not found", id)
}

// FindByLayer returns all frames whose Layer matches the given layer.
func (r *Registry) FindByLayer(layer Layer) []StrategyFrame {
	if r == nil {
		return nil
	}
	out := make([]StrategyFrame, 0)
	for _, f := range r.Frames {
		if f.Layer == layer {
			out = append(out, f)
		}
	}
	return out
}

// PeriodRegimeTags maps the seven-period market cycle (domain.MarketPeriod
// values) to the regime vocabulary used by StrategyFrame.Regimes
// (BULL / NEUTRAL / BEAR / HIGH_VOL). PR-3d: the decision-chain display
// filters technique frames by the current period through this mapping —
// display only; recommendation ordering and conviction are untouched.
//
//	turnaround_up → BULL, bull → BULL
//	plateau, consolidation → NEUTRAL
//	turnaround_down → BEAR, downturn → BEAR
//	black_swan → HIGH_VOL (seeds only annotate defensive frames with HIGH_VOL)
var PeriodRegimeTags = map[string]string{
	"turnaround_up":   "BULL",
	"bull":            "BULL",
	"plateau":         "NEUTRAL",
	"consolidation":   "NEUTRAL",
	"turnaround_down": "BEAR",
	"downturn":        "BEAR",
	"black_swan":      "HIGH_VOL",
}

// FramesForPeriod returns frames annotated for the given seven-period
// market classification ("bull", "black_swan", …). Frames with an empty
// Regimes list always pass (backward compatible with unannotated seeds).
// Unknown periods return all frames (fail-open display, never filtering
// by guesswork).
func (r *Registry) FramesForPeriod(period string) []StrategyFrame {
	if r == nil {
		return nil
	}
	tag, ok := PeriodRegimeTags[period]
	if !ok {
		return r.All()
	}
	out := make([]StrategyFrame, 0, len(r.Frames))
	for _, f := range r.Frames {
		if len(f.Regimes) == 0 {
			out = append(out, f)
			continue
		}
		for _, g := range f.Regimes {
			if g == tag {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Layers returns the unique Layers present in the registry, sorted by definition order.
func (r *Registry) Layers() []Layer {
	if r == nil {
		return nil
	}
	seen := make(map[Layer]bool)
	out := make([]Layer, 0)
	for _, f := range r.Frames {
		if !seen[f.Layer] {
			seen[f.Layer] = true
			out = append(out, f.Layer)
		}
	}
	return out
}
