package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func SeedRegistry() domain.AgentRegistry {
	return domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{
			{
				ID:               "taiwan-macro-01",
				Name:             "Taiwan Macro",
				Layer:            domain.LayerContext,
				Skill:            "taiwan_macro",
				PromptFile:       "prompts/agents/taiwan_macro.md",
				Enabled:          true,
				PrimaryMetrics:   []string{"regime_accuracy", "drawdown_avoidance"},
				RequiredSkills:   []string{"taiwan_macro", "replay_operator"},
				ForbiddenActions: []string{"single_stock_orders", "position_sizing_override"},
				OperatingNotes:   []string{"Only output regime and macro risk context."},
			},
			{
				ID:               "foreign-flow-01",
				Name:             "Foreign Flow",
				Layer:            domain.LayerContext,
				Skill:            "foreign_flow",
				PromptFile:       "prompts/agents/foreign_flow.md",
				Enabled:          true,
				PrimaryMetrics:   []string{"regime_accuracy", "flow_alignment"},
				RequiredSkills:   []string{"foreign_flow", "replay_operator"},
				ForbiddenActions: []string{"single_factor_final_decision"},
				OperatingNotes:   []string{"Treat flow as a modifier, not a full thesis."},
			},
			{
				ID:               "semi-desk-01",
				Name:             "Semiconductor Desk",
				Layer:            domain.LayerSector,
				Skill:            "semiconductor_desk",
				PromptFile:       "prompts/agents/semiconductor_desk.md",
				Enabled:          true,
				Universe:         []string{"2330.TW", "2303.TW", "2454.TW", "3034.TW"},
				PrimaryMetrics:   []string{"alpha_hit_rate", "risk_adjusted_return"},
				RequiredSkills:   []string{"semiconductor_desk", "earnings_quality", "technical_breakout"},
				ForbiddenActions: []string{"illiquid_name_selection", "macro_override"},
				OperatingNotes:   []string{"Differentiate foundry, packaging, and design roles."},
			},
			{
				ID:               "ai-desk-01",
				Name:             "AI Supply Chain Desk",
				Layer:            domain.LayerSector,
				Skill:            "ai_supply_chain_desk",
				PromptFile:       "prompts/agents/ai_supply_chain_desk.md",
				Enabled:          true,
				Universe:         []string{"2382.TW", "6669.TW", "3017.TW", "3037.TW"},
				PrimaryMetrics:   []string{"alpha_hit_rate", "turnover_efficiency"},
				RequiredSkills:   []string{"ai_supply_chain_desk", "growth_momentum"},
				ForbiddenActions: []string{"narrative_only_selection"},
				OperatingNotes:   []string{"Separate durable order flow from hype-driven moves."},
			},
			{
				ID:               "etf-rotation-01",
				Name:             "ETF Rotation Desk",
				Layer:            domain.LayerSector,
				Skill:            "etf_rotation_desk",
				PromptFile:       "prompts/agents/etf_rotation_desk.md",
				Enabled:          true,
				Universe:         []string{"0050.TW", "0056.TW", "00878.TW"},
				PrimaryMetrics:   []string{"drawdown_control", "defensive_capture"},
				RequiredSkills:   []string{"etf_rotation_desk", "value_yield"},
				ForbiddenActions: []string{"single_name_substitution_bias"},
				OperatingNotes:   []string{"Use ETF rotation when dispersion or risk is elevated."},
			},
			{
				ID:               "financials-desk-01",
				Name:             "Financials Desk",
				Layer:            domain.LayerSector,
				Skill:            "financials_desk",
				PromptFile:       "prompts/agents/financials_desk.md",
				Enabled:          true,
				Universe:         []string{"2881.TW", "2882.TW", "2891.TW"},
				PrimaryMetrics:   []string{"downside_capture", "carry_efficiency"},
				RequiredSkills:   []string{"financials_desk", "value_yield"},
				ForbiddenActions: []string{"cyclical_misclassification", "illiquid_preference"},
				OperatingNotes:   []string{"Favor balance-sheet resilience and dividend support over excitement."},
			},
			{
				ID:               "shipping-desk-01",
				Name:             "Shipping Desk",
				Layer:            domain.LayerSector,
				Skill:            "shipping_desk",
				PromptFile:       "prompts/agents/shipping_desk.md",
				Enabled:          true,
				Universe:         []string{"2603.TW", "2609.TW", "2615.TW"},
				PrimaryMetrics:   []string{"cycle_timing", "volatility_capture"},
				RequiredSkills:   []string{"shipping_desk", "technical_breakout"},
				ForbiddenActions: []string{"cycle_blindness", "headline_only_selection"},
				OperatingNotes:   []string{"Treat freight beta as tactical exposure, not permanent core risk."},
			},
			{
				ID:               "leo-satellite-desk-01",
				Name:             "LEO Satellite Desk",
				Layer:            domain.LayerSector,
				Skill:            "leo_satellite_desk",
				PromptFile:       "prompts/agents/leo_satellite_desk.md",
				Enabled:          true,
				Universe:         []string{"3491.TW", "2313.TW", "6285.TW", "7717.TW", "3105.TW", "2367.TW", "3022.TW", "3138.TW"},
				PrimaryMetrics:   []string{"alpha_hit_rate", "deployment_timing"},
				RequiredSkills:   []string{"leo_satellite_desk", "growth_momentum"},
				ForbiddenActions: []string{"launch_hype_chasing", "ignoring_flight_heritage"},
				OperatingNotes:   []string{"Distinguish between launch-driven hype and durable deployment order flow."},
			},
			{
				ID:               "growth-momentum-01",
				Name:             "Growth Momentum",
				Layer:            domain.LayerStyle,
				Skill:            "growth_momentum",
				PromptFile:       "prompts/agents/growth_momentum.md",
				Enabled:          true,
				Universe:         []string{"2317.TW", "2382.TW", "2454.TW", "3034.TW", "3037.TW", "6669.TW"},
				PrimaryMetrics:   []string{"alpha_hit_rate", "momentum_followthrough"},
				RequiredSkills:   []string{"growth_momentum", "technical_breakout"},
				ForbiddenActions: []string{"illiquid_breakout_chasing", "earnings_blindness"},
				OperatingNotes:   []string{"Require confirmation and liquidity before upgrading conviction."},
			},
			{
				ID:               "value-yield-01",
				Name:             "Value Yield",
				Layer:            domain.LayerStyle,
				Skill:            "value_yield",
				PromptFile:       "prompts/agents/value_yield.md",
				Enabled:          true,
				Universe:         []string{"2881.TW", "2882.TW", "2886.TW", "2891.TW", "0056.TW", "00878.TW"},
				PrimaryMetrics:   []string{"drawdown_control", "carry_quality"},
				RequiredSkills:   []string{"value_yield", "financials_desk"},
				ForbiddenActions: []string{"yield_trap_selection", "narrative_chasing"},
				OperatingNotes:   []string{"Demand cash-flow support before treating yield as defensive."},
			},
			{
				ID:               "earnings-quality-01",
				Name:             "Earnings Quality",
				Layer:            domain.LayerStyle,
				Skill:            "earnings_quality",
				PromptFile:       "prompts/agents/earnings_quality.md",
				Enabled:          true,
				Universe:         []string{"2330.TW", "2308.TW", "3008.TW", "1301.TW", "1303.TW", "1326.TW"},
				PrimaryMetrics:   []string{"estimate_quality", "post_earnings_followthrough"},
				RequiredSkills:   []string{"earnings_quality", "semiconductor_desk"},
				ForbiddenActions: []string{"revenue_only_logic", "guidance_ignorance"},
				OperatingNotes:   []string{"Prefer repeatable earnings quality over one-off spikes."},
			},
			{
				ID:               "technical-breakout-01",
				Name:             "Technical Breakout",
				Layer:            domain.LayerStyle,
				Skill:            "technical_breakout",
				PromptFile:       "prompts/agents/technical_breakout.md",
				Enabled:          true,
				Universe:         []string{"2330.TW", "2317.TW", "2382.TW", "2881.TW", "2603.TW", "2609.TW", "0050.TW"},
				PrimaryMetrics:   []string{"breakout_followthrough", "false_breakout_avoidance"},
				RequiredSkills:   []string{"technical_breakout", "growth_momentum"},
				ForbiddenActions: []string{"low_volume_breakout_chasing", "range_blindness"},
				OperatingNotes:   []string{"Require structure, volume, and close strength before breakout upgrades."},
			},
			{
				ID:               "cro-01",
				Name:             "CRO",
				Layer:            domain.LayerControl,
				Skill:            "cro_risk",
				PromptFile:       "prompts/agents/cro_risk.md",
				Enabled:          true,
				PrimaryMetrics:   []string{"drawdown_control", "concentration_violations"},
				RequiredSkills:   []string{"cro_risk", "system_guardrail"},
				ForbiddenActions: []string{"alpha_generation", "risk_limit_override"},
				OperatingNotes:   []string{"Attack ideas; do not originate them."},
			},
			{
				ID:               "cio-01",
				Name:             "CIO",
				Layer:            domain.LayerControl,
				Skill:            "cio_portfolio",
				PromptFile:       "prompts/agents/cio_seed.md",
				Enabled:          true,
				PrimaryMetrics:   []string{"portfolio_return", "sharpe_like"},
				RequiredSkills:   []string{"cio_portfolio", "research_auditor"},
				ForbiddenActions: []string{"cro_bypass", "engine_bypass"},
				OperatingNotes:   []string{"Synthesize only within engine and CRO constraints."},
			},
		},
	}
}

func ValidateRegistry(reg domain.AgentRegistry, configDir string) error {
	if reg.Version <= 0 {
		return fmt.Errorf("registry version must be positive")
	}

	seen := make(map[string]struct{}, len(reg.Agents))
	for _, agent := range reg.Agents {
		if agent.ID == "" {
			return fmt.Errorf("agent id must not be empty")
		}
		if agent.Skill == "" {
			return fmt.Errorf("agent %s skill must not be empty", agent.ID)
		}
		if len(agent.RequiredSkills) == 0 {
			return fmt.Errorf("agent %s required skills must not be empty", agent.ID)
		}
		if _, ok := seen[agent.ID]; ok {
			return fmt.Errorf("duplicate agent id: %s", agent.ID)
		}
		seen[agent.ID] = struct{}{}

		if configDir != "" && agent.PromptFile != "" {
			promptPath := agent.PromptFile
			if !filepath.IsAbs(promptPath) {
				promptPath = filepath.Join(configDir, promptPath)
			}
			if _, err := os.Stat(promptPath); err != nil {
				return fmt.Errorf("agent %s prompt file not found: %s", agent.ID, promptPath)
			}
		}
	}

	return nil
}

// LoadRegistry loads agent registry from a single JSON file.
// If path is empty, falls back to SeedRegistry().
func LoadRegistry(path string) (domain.AgentRegistry, error) {
	if path == "" {
		reg := SeedRegistry()
		return reg, ValidateRegistry(reg, "")
	}
	return LoadRegistryMulti(path)
}

// LoadRegistryMulti merges agent registries from multiple JSON sources.
// Sources are loaded in order; duplicate agent IDs skip with a warning
// (first-write-wins). This enables proprietary modules to layer their own
// agent definitions on top of the open-source core registry.
func LoadRegistryMulti(paths ...string) (domain.AgentRegistry, error) {
	if len(paths) == 0 {
		reg := SeedRegistry()
		return reg, ValidateRegistry(reg, "")
	}

	merged := domain.AgentRegistry{Version: 1}
	seen := make(map[string]struct{}, 64)

	for _, path := range paths {
		reg, err := loadRegistryFile(path)
		if err != nil {
			return domain.AgentRegistry{}, fmt.Errorf("load %s: %w", path, err)
		}
		for _, agent := range reg.Agents {
			if _, exists := seen[agent.ID]; exists {
				fmt.Fprintf(os.Stderr, "[registry] agent %q from %s skipped (duplicate ID — first source wins)\n", agent.ID, path)
				continue
			}
			seen[agent.ID] = struct{}{}
			merged.Agents = append(merged.Agents, agent)
		}
		if reg.Version > merged.Version {
			merged.Version = reg.Version
		}
	}

	configDir := filepath.Dir(paths[0])
	return merged, ValidateRegistry(merged, configDir)
}

func loadRegistryFile(path string) (domain.AgentRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.AgentRegistry{}, err
	}
	var reg domain.AgentRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return domain.AgentRegistry{}, err
	}
	return reg, nil
}
