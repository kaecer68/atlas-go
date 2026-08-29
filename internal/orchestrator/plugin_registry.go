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

// PositionEvaluator evaluates a held position and produces a rotation recommendation
// (SELL or REDUCE) when the position should be trimmed or exited.
type PositionEvaluator interface {
	Supports(agent domain.AgentSpec) bool
	EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool)
}

// PortfolioRotator evaluates held positions through registered PositionEvaluators
// and generates SELL/REDUCE recommendations to rotate out of underperforming holdings.
type PortfolioRotator struct {
	evaluators []PositionEvaluator
}

func NewPortfolioRotator(evaluators ...PositionEvaluator) *PortfolioRotator {
	return &PortfolioRotator{evaluators: evaluators}
}

// Rotate evaluates all positions through all registered evaluators and returns
// sell/reduce recommendations. Only one recommendation per position (the first
// evaluator to fire) is returned.
// Rotate evaluates held positions against a single agent and returns
// SELL/REDUCE recommendations. Used for per-agent position evaluation.
func (r *PortfolioRotator) Rotate(positions []domain.Position, quotes map[string]domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) []domain.Recommendation {
	if r == nil || len(r.evaluators) == 0 {
		return nil
	}
	var recs []domain.Recommendation
	seen := make(map[string]bool)
	for _, pos := range positions {
		if seen[pos.Symbol] {
			continue
		}
		quote, ok := quotes[pos.Symbol]
		if !ok || !quote.IsTradable {
			continue
		}
		for _, eval := range r.evaluators {
			if !eval.Supports(agent) {
				continue
			}
			if rec, ok := eval.EvaluatePosition(pos, quote, agent, prompt, regime, fq); ok {
				recs = append(recs, rec)
				seen[pos.Symbol] = true
				break
			}
		}
	}
	return recs
}

// RotatePortfolio performs portfolio-level rotation: scores all held positions,
// compares against BUY candidates, and generates SELL signals for the weakest
// holding(s) when the portfolio is at capacity and BUY candidates exist.
func (r *PortfolioRotator) RotatePortfolio(
	positions []domain.Position,
	buyRecs []domain.Recommendation,
	quotes map[string]domain.Quote,
	registry domain.AgentRegistry,
	plugins *PluginRegistry,
	overrides map[string]string,
	regime domain.Regime,
	fq FactorQuery,
) []domain.Recommendation {
	if r == nil || len(r.evaluators) == 0 || len(positions) == 0 || len(buyRecs) == 0 {
		return nil
	}

	// Score each held position using the best-matching evaluator.
	// Apply a concentration penalty: positions exceeding the max limit (15%)
	// get their conviction reduced, making them more likely to be rotated out.
	type positionScore struct {
		pos        domain.Position
		conviction int
	}
	var positionScores []positionScore

	// Compute total portfolio value for concentration calculations
	totalValue := 0.0
	for _, pos := range positions {
		totalValue += pos.MarketValue
	}

	const maxPositionPct = 0.15
	const concentrationPenalty = 30 // conviction reduction per 1% over limit

	for _, pos := range positions {
		quote, ok := quotes[pos.Symbol]
		if !ok || !quote.IsTradable {
			continue
		}
		bestConviction := 0
		for _, agent := range registry.Agents {
			if !agent.Enabled || agent.Layer == domain.LayerControl || agent.Layer == domain.LayerContext {
				continue
			}
			prompt := plugins.ResolvePrompt(agent, overrides)
			for _, eval := range r.evaluators {
				if !eval.Supports(agent) {
					continue
				}
				if rec, ok := eval.EvaluatePosition(pos, quote, agent, prompt, regime, fq); ok {
					if rec.Conviction > bestConviction {
						bestConviction = rec.Conviction
					}
					break
				}
			}
		}
		// Fallback: neutral score for positions without evaluator coverage
		if bestConviction == 0 {
			bestConviction = 50
		}

		// Concentration penalty: positions over 15% get scored lower
		if totalValue > 0 {
			pct := pos.MarketValue / totalValue
			if pct > maxPositionPct {
				overPct := (pct - maxPositionPct) * 100
				penalty := min(int(overPct*concentrationPenalty), bestConviction-10)
				bestConviction -= penalty
			}
		}

		positionScores = append(positionScores, positionScore{
			pos: pos, conviction: bestConviction,
		})
	}

	if len(positionScores) == 0 {
		return nil
	}

	// Find weakest held position
	weakest := positionScores[0]
	for _, ps := range positionScores[1:] {
		if ps.conviction < weakest.conviction {
			weakest = ps
		}
	}

	// Find strongest BUY candidate not already held
	heldSymbols := make(map[string]bool)
	for _, pos := range positions {
		heldSymbols[pos.Symbol] = true
	}
	var bestBuy *domain.Recommendation
	for i := range buyRecs {
		if heldSymbols[buyRecs[i].Symbol] {
			continue
		}
		if bestBuy == nil || buyRecs[i].Conviction > bestBuy.Conviction {
			bestBuy = &buyRecs[i]
		}
	}

	if bestBuy == nil {
		return nil
	}

	// Generate SELL for weakest holding to free up capacity for the best BUY
	return []domain.Recommendation{{
		Agent:      bestBuy.Agent,
		Skill:      bestBuy.Skill,
		Layer:      bestBuy.Layer,
		Symbol:     weakest.pos.Symbol,
		Side:       domain.SideSell,
		Conviction: 100,
		Reason:     fmt.Sprintf("rotation: replace %s (score=%d) with %s (conviction=%d)", weakest.pos.Symbol, weakest.conviction, bestBuy.Symbol, bestBuy.Conviction),
	}}
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
	mlScorer           *MLScorer
	promptResolver     PromptResolver    // plugin boundary — injected via WithPromptResolver; nil = fallback to os.ReadFile
	rotator            *PortfolioRotator // position rotation evaluator
	heldPositions      []domain.Position // positions carried over from prior session; drives SELL/REDUCE in collectRecommendations
	recOverrides       map[string]string // human-in-the-loop approve/reject per "agentID:symbol"; nil = machine-first default
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

// WithMLScorer injects an MLScorer for ML-based factor scoring.
// When set and trained, CalculateFactorScoresWithBreakdown will use it
// to produce an ML-adjusted total score.
func (r *PluginRegistry) WithMLScorer(s *MLScorer) *PluginRegistry {
	r.mlScorer = s
	return r
}

// WithPromptResolver injects a PromptResolver for loading prompts from
// external directories. When nil (default), ResolvePrompt falls back to
// os.ReadFile on agent.PromptFile. DO NOT REMOVE — plugin boundary setter.
func (r *PluginRegistry) WithPromptResolver(pr PromptResolver) *PluginRegistry {
	r.promptResolver = pr
	return r
}

// RegisterPositionEvaluators registers executor instances as PositionEvaluator
// for use by PortfolioRotator during recommendation collection.
func (r *PluginRegistry) RegisterPositionEvaluators(evaluators ...PositionEvaluator) *PluginRegistry {
	if r.rotator == nil {
		r.rotator = NewPortfolioRotator(evaluators...)
	} else {
		r.rotator.evaluators = append(r.rotator.evaluators, evaluators...)
	}
	return r
}

// WithHeldPositions attaches the portfolio's currently-held positions so that
// collectRecommendations can run PortfolioRotator.Rotate and emit SELL/REDUCE
// recs for positions whose factor signals have decayed. When heldPositions is
// nil/empty (or the rotator has no evaluators), the rotation path is a no-op
// and behavior is identical to pre-fix.
func (r *PluginRegistry) WithHeldPositions(positions []domain.Position) *PluginRegistry {
	r.heldPositions = positions
	return r
}

// WithRecOverrides attaches human-in-the-loop override decisions (approve/reject
// per recommendation) so that collectRecommendations can force-pass or skip
// specific recs before they enter the guard layer. The map key is "agentID:symbol"
// and the value is "approved" or "rejected".
// When recOverrides is nil/empty, no overrides apply — machine-first default.
func (r *PluginRegistry) WithRecOverrides(overrides map[string]string) *PluginRegistry {
	r.recOverrides = overrides
	return r
}

// Rotator returns the PortfolioRotator, or nil if no evaluators are registered.
func (r *PluginRegistry) Rotator() *PortfolioRotator {
	return r.rotator
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
	breakdown, scores := r.factorEngine.CalculateAllScoresWithBreakdown(symbol, quotes, agentRecs, agentWeights, defaultWeights)

	// When ML scorer is attached and trained, override the total score
	// with an ML-predicted value while preserving heuristic per-factor breakdowns.
	if r.mlScorer != nil && r.mlScorer.IsTrained() && scores != nil {
		quote, ok := quotes[symbol]
		if ok {
			if mlTotal, err := r.mlScorer.Score(quote, scores); err == nil {
				scores["total"] = mlTotal
				if breakdown != nil {
					breakdown.Total.Score = mlTotal
				}
			}
		}
	}

	return breakdown, scores
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

func (r *PluginRegistry) ApplyControl(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation {
	for _, exec := range r.controlExecutors {
		if exec.Supports(agent) {
			return exec.Apply(agent, recs, policy, regime)
		}
	}
	return recs
}
