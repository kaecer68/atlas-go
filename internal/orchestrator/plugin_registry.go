package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/screener"
)

type AgentExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool)
}

type RegimeExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Score(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int
}

// ── PLUGIN BOUNDARY: DO NOT REMOVE ──────────────────────────────────
//
// PromptResolver decouples prompt loading from PluginRegistry. It appears
// unused in the open-source core (the ResolvePrompt method falls back to
// os.ReadFile when promptResolver is nil), but proprietary modules inject
// a PromptResolver via WithPromptResolver to load prompts from an external
// directory (e.g., atlas-strategies-tw/prompts/) without modifying core.
//
// FileSystemPromptResolver is the reference implementation.
// promptResolver field on PluginRegistry is the injection point.
// WithPromptResolver is the setter.
//
// When refactoring: keep the interface, the implementation, the field,
// and the setter. They are NOT dead code — they are the prompt-loading
// boundary between open-source engine and private strategy IP.
// ─────────────────────────────────────────────────────────────────────

// PromptResolver resolves prompt content for an agent. This abstraction
// allows prompts to be loaded from the filesystem, embedded in a binary,
// or fetched from an external service — without changing PluginRegistry.
type PromptResolver interface {
	Resolve(agent domain.AgentSpec) (string, error)
}

// FileSystemPromptResolver loads prompt files from a configurable base directory.
// The base directory typically points to a separate repo containing proprietary
// strategy prompts (e.g., atlas-strategies-tw/prompts/agents/).
type FileSystemPromptResolver struct {
	baseDir string
}

func NewFileSystemPromptResolver(baseDir string) *FileSystemPromptResolver {
	return &FileSystemPromptResolver{baseDir: baseDir}
}

func (r *FileSystemPromptResolver) Resolve(agent domain.AgentSpec) (string, error) {
	path := agent.PromptFile
	if !filepath.IsAbs(path) && r.baseDir != "" {
		path = filepath.Join(r.baseDir, agent.PromptFile)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("resolve prompt %s: %w", agent.PromptFile, err)
	}
	return strings.ToLower(string(bytes)), nil
}

type PluginRegistry struct {
	regimeExecutors    []RegimeExecutor
	agentExecutors     []AgentExecutor
	controlExecutors   []ControlExecutor
	screener           screener.Screener
	factorEngine       *portfolio.FactorEngine
	healthManager      *portfolio.AgentHealthManager
	cycleModulator     *IndustryCycleModulator
	narrativeModulator *NarrativeConvictionModulator
	promptResolver     PromptResolver // plugin boundary — injected via WithPromptResolver; nil = fallback to os.ReadFile
}

func NewPluginRegistry(loaders ...ExecutorLoader) *PluginRegistry {
	loader := ExecutorLoader(StaticLoader{})
	if len(loaders) > 0 {
		loader = loaders[0]
	}
	regime, _ := loader.LoadRegimeExecutors()
	agent, _ := loader.LoadAgentExecutors()
	control, _ := loader.LoadControlExecutors()
	return &PluginRegistry{
		regimeExecutors:  regime,
		agentExecutors:   agent,
		controlExecutors: control,
	}
}

func (r *PluginRegistry) WithScreener(s screener.Screener) *PluginRegistry {
	r.screener = s
	return r
}

func (r *PluginRegistry) WithFactorEngine(fe *portfolio.FactorEngine) *PluginRegistry {
	r.factorEngine = fe
	return r
}

func (r *PluginRegistry) WithAgentHealthManager(m *portfolio.AgentHealthManager) *PluginRegistry {
	r.healthManager = m
	return r
}

func (r *PluginRegistry) WithCycleModulator(m *IndustryCycleModulator) *PluginRegistry {
	r.cycleModulator = m
	return r
}

func (r *PluginRegistry) WithNarrativeModulator(m *NarrativeConvictionModulator) *PluginRegistry {
	r.narrativeModulator = m
	return r
}

// WithPromptResolver injects a PromptResolver for loading prompts from
// external directories. When nil (default), ResolvePrompt falls back to
// os.ReadFile on agent.PromptFile. DO NOT REMOVE — plugin boundary setter.
func (r *PluginRegistry) WithPromptResolver(pr PromptResolver) *PluginRegistry {
	r.promptResolver = pr
	return r
}

// WireScreenerTraceWriter attaches a trace writer to the underlying screener
// engine. No-op if the screener is nil or not a *screener.Engine.
func (r *PluginRegistry) WireScreenerTraceWriter(tw *SimTraceWriter) {
	if r.screener == nil {
		return
	}
	if eng, ok := r.screener.(*screener.Engine); ok {
		eng.WithTraceWriter(tw)
	}
}

func (r *PluginRegistry) IsAgentHealthy(agentID string) bool {
	if r.healthManager == nil {
		return true
	}
	return r.healthManager.IsAgentHealthy(agentID)
}

func (r *PluginRegistry) CalculateFactorScores(symbol string, quotes map[string]domain.Quote, agentRecs []domain.Recommendation, agentWeights map[string]float64) map[portfolio.FactorType]float64 {
	if r.factorEngine == nil {
		return nil
	}
	defaultWeights := map[portfolio.FactorType]float64{
		portfolio.FactorMomentum: 0.30,
		portfolio.FactorValue:    0.25,
		portfolio.FactorQuality:  0.25,
		portfolio.FactorAgent:    0.20,
	}
	return r.factorEngine.CalculateAllScores(symbol, quotes, agentRecs, agentWeights, defaultWeights)
}

func (r *PluginRegistry) CalculateFactorScoresWithBreakdown(symbol string, quotes map[string]domain.Quote, agentRecs []domain.Recommendation, agentWeights map[string]float64) (*domain.FactorScoreBreakdown, map[portfolio.FactorType]float64) {
	if r.factorEngine == nil {
		return nil, nil
	}
	defaultWeights := map[portfolio.FactorType]float64{
		portfolio.FactorMomentum: 0.30,
		portfolio.FactorValue:    0.25,
		portfolio.FactorQuality:  0.25,
		portfolio.FactorAgent:    0.20,
	}
	return r.factorEngine.CalculateAllScoresWithBreakdown(symbol, quotes, agentRecs, agentWeights, defaultWeights)
}

func (r *PluginRegistry) Screen(ctx context.Context, agent domain.AgentSpec, symbol string, quotes map[string]domain.Quote) (bool, error) {
	if r.screener == nil || !agent.ScreeningCriteria.HasFilters() {
		return true, nil
	}
	return r.screener.Screen(ctx, symbol, agent.ScreeningCriteria, quotes)
}

func (r *PluginRegistry) ScreenDetailed(ctx context.Context, agent domain.AgentSpec, symbol string, quotes map[string]domain.Quote) (screener.ScreenResult, error) {
	if r.screener == nil || !agent.ScreeningCriteria.HasFilters() {
		return screener.ScreenResult{Passed: true}, nil
	}
	return r.screener.ScreenDetailed(ctx, symbol, agent.ScreeningCriteria, quotes)
}

func (r *PluginRegistry) ResolvePrompt(agent domain.AgentSpec, overrides map[string]string) string {
	if override, ok := overrides[agent.ID]; ok && override != "" {
		return override
	}
	if override, ok := overrides[agent.Skill]; ok && override != "" {
		return override
	}
	// Plugin boundary: use injected PromptResolver if available (enables
	// proprietary prompt repos); otherwise fall back to direct filesystem read.
	if r.promptResolver != nil {
		if resolved, err := r.promptResolver.Resolve(agent); err == nil {
			return resolved
		}
	}
	// Fallback for open-source core when no PromptResolver is injected.
	bytes, err := os.ReadFile(agent.PromptFile)
	if err != nil {
		return ""
	}
	return strings.ToLower(string(bytes))
}

func (r *PluginRegistry) RegimeScore(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int {
	for _, exec := range r.regimeExecutors {
		if exec.Supports(agent) {
			return exec.Score(agent, quotes, prompt)
		}
	}
	return 0
}

func (r *PluginRegistry) Recommendation(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq ...FactorQuery) (domain.Recommendation, bool) {
	var resolved FactorQuery = &FactorSnapshot{} // empty snapshot: all GetScore calls return (0, false)
	if len(fq) > 0 && fq[0] != nil {
		resolved = fq[0]
	}
	for _, exec := range r.agentExecutors {
		if exec.Supports(agent) {
			return exec.Recommend(agent, quote, prompt, regime, resolved)
		}
	}
	return domain.Recommendation{}, false
}

func (r *PluginRegistry) ApplyControl(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	for _, exec := range r.controlExecutors {
		if exec.Supports(agent) {
			return exec.Apply(agent, recs, policy)
		}
	}
	return recs
}
