// Package reflexivity implements Soros' Theory of Reflexivity for market feedback loops
// Models the two-way connection between market participants' bias and market reality
package reflexivity

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ReflexivityEngine tracks market biases and their reflexive effects
type ReflexivityEngine struct {
	feedbackLoops []FeedbackLoop
	biases        map[string]*MarketBias
	realities     map[string]*MarketReality
	mu            sync.RWMutex
}

// FeedbackLoop represents a reflexive feedback cycle
type FeedbackLoop struct {
	ID        string
	Name      string
	Bias      *MarketBias
	Reality   *MarketReality
	Direction LoopDirection
	Strength  float64 // 0.0 to 1.0
	Status    LoopStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LoopDirection indicates if loop is positive or negative feedback
type LoopDirection int

const (
	PositiveFeedback LoopDirection = iota // Self-reinforcing (bubble/crash)
	NegativeFeedback                      // Self-correcting (mean reversion)
)

// LoopStatus tracks the lifecycle of a feedback loop
type LoopStatus int

const (
	LoopEmerging LoopStatus = iota
	LoopActive
	LoopMaturing
	LoopExhausting
	LoopCompleted
)

// MarketBias represents participants' cognitive bias toward a market condition
type MarketBias struct {
	ID         string
	Type       BiasType
	Target     string   // Symbol or market
	Magnitude  float64  // -1.0 to 1.0 (bearish to bullish)
	Confidence float64  // 0.0 to 1.0
	Source     []string // Agent IDs contributing to this bias
	Timestamp  time.Time
}

// BiasType categorizes different cognitive biases
type BiasType int

const (
	TrendFollowing BiasType = iota
	Contrarian
	Anchoring
	Recency
	Confirmation
	Herding
	Overconfidence
	FearGreed
)

// MarketReality represents the actual market conditions
type MarketReality struct {
	ID         string
	Target     string
	Price      float64
	Trend      float64 // Price velocity
	Volatility float64
	Volume     float64
	Liquidity  float64
	Timestamp  time.Time
}

// ReflexivityObservation captures a moment in the reflexive process
type ReflexivityObservation struct {
	Timestamp     time.Time
	BiasMagnitude float64
	RealityTrend  float64
	LoopStrength  float64
	Prediction    string
}

// NewReflexivityEngine creates a new reflexivity engine
func NewReflexivityEngine() *ReflexivityEngine {
	return &ReflexivityEngine{
		feedbackLoops: make([]FeedbackLoop, 0),
		biases:        make(map[string]*MarketBias),
		realities:     make(map[string]*MarketReality),
	}
}

// RegisterBias records a market bias from agent recommendations
func (re *ReflexivityEngine) RegisterBias(bias *MarketBias) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	// Validate bias before registration
	if err := re.validateBias(bias); err != nil {
		return fmt.Errorf("invalid bias: %w", err)
	}

	// Generate unique key for this bias
	key := fmt.Sprintf("%s_%d_%s", bias.Target, bias.Type, bias.ID)

	// Check for existing bias and merge if appropriate
	if existing, exists := re.biases[key]; exists {
		// Merge with existing bias (weighted average)
		mergedBias := re.mergeBiases(existing, bias)
		re.biases[key] = mergedBias
	} else {
		// Add new bias
		re.biases[key] = bias
	}

	// Check if this creates or affects a feedback loop
	re.evaluateFeedbackLoops(bias.Target)

	return nil
}

// validateBias ensures bias meets minimum requirements
func (re *ReflexivityEngine) validateBias(bias *MarketBias) error {
	if bias == nil {
		return fmt.Errorf("bias is nil")
	}

	if bias.Target == "" {
		return fmt.Errorf("bias target cannot be empty")
	}

	if len(bias.Source) == 0 {
		return fmt.Errorf("bias must have at least one source")
	}

	if bias.Confidence < 0 || bias.Confidence > 1 {
		return fmt.Errorf("bias confidence must be between 0 and 1")
	}

	if bias.Magnitude < -1 || bias.Magnitude > 1 {
		return fmt.Errorf("bias magnitude must be between -1 and 1")
	}

	if bias.Timestamp.IsZero() {
		return fmt.Errorf("bias timestamp cannot be zero")
	}

	return nil
}

// mergeBiases combines two biases using weighted average based on confidence
func (re *ReflexivityEngine) mergeBiases(existing, new *MarketBias) *MarketBias {
	totalConfidence := existing.Confidence + new.Confidence

	// Weighted average of magnitude and confidence
	mergedMagnitude := (existing.Magnitude*existing.Confidence + new.Magnitude*new.Confidence) / totalConfidence
	mergedConfidence := math.Min(totalConfidence/2, 1.0) // Cap at 1.0

	// Combine sources
	mergedSources := make([]string, 0, len(existing.Source)+len(new.Source))
	mergedSources = append(mergedSources, existing.Source...)
	mergedSources = append(mergedSources, new.Source...)

	// Use the most recent timestamp
	mergedTimestamp := existing.Timestamp
	if new.Timestamp.After(existing.Timestamp) {
		mergedTimestamp = new.Timestamp
	}

	return &MarketBias{
		ID:         existing.ID,   // Keep original ID
		Type:       existing.Type, // Keep original type
		Target:     existing.Target,
		Magnitude:  mergedMagnitude,
		Confidence: mergedConfidence,
		Source:     mergedSources,
		Timestamp:  mergedTimestamp,
	}
}

// UpdateReality updates market reality conditions
func (re *ReflexivityEngine) UpdateReality(reality *MarketReality) {
	re.mu.Lock()
	defer re.mu.Unlock()

	re.realities[reality.Target] = reality

	// Evaluate feedback effects
	re.evaluateFeedbackLoops(reality.Target)
}

// evaluateFeedbackLoops checks for and updates feedback loops
func (re *ReflexivityEngine) evaluateFeedbackLoops(target string) {
	// Get current bias and reality for target
	bias := re.getDominantBias(target)
	reality := re.realities[target]

	if bias == nil || reality == nil {
		return
	}

	// Detect feedback loop
	loop := re.detectLoop(target, bias, reality)
	if loop != nil {
		// Update or create loop
		re.updateOrCreateLoop(loop)
	}
}

// detectLoop identifies if a feedback loop exists
func (re *ReflexivityEngine) detectLoop(target string, bias *MarketBias, reality *MarketReality) *FeedbackLoop {
	// Check correlation between bias and reality trend
	correlation := bias.Magnitude * reality.Trend

	// If bias and reality are aligned, it's positive feedback
	// If opposed, it's negative feedback
	var direction LoopDirection
	var strength float64

	if correlation > 0 {
		direction = PositiveFeedback
		strength = math.Abs(correlation)
	} else {
		direction = NegativeFeedback
		strength = math.Abs(correlation) * 0.5 // Negative feedback is typically weaker
	}

	// Only create loop if strength is significant
	if strength < 0.3 {
		return nil
	}

	return &FeedbackLoop{
		ID:        generateLoopID(target, bias.Type),
		Name:      fmt.Sprintf("%s_%s_loop", target, biasTypeString(bias.Type)),
		Bias:      bias,
		Reality:   reality,
		Direction: direction,
		Strength:  strength,
		Status:    LoopEmerging,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// updateOrCreateLoop adds or updates a feedback loop
func (re *ReflexivityEngine) updateOrCreateLoop(loop *FeedbackLoop) {
	found := false
	for i, existing := range re.feedbackLoops {
		if existing.ID == loop.ID {
			re.feedbackLoops[i] = *loop
			found = true
			break
		}
	}
	if !found {
		re.feedbackLoops = append(re.feedbackLoops, *loop)
	}
}

// getDominantBias returns the strongest bias for a target
func (re *ReflexivityEngine) getDominantBias(target string) *MarketBias {
	var dominant *MarketBias
	maxMagnitude := 0.0

	for _, bias := range re.biases {
		if bias.Target == target {
			magnitude := math.Abs(bias.Magnitude) * bias.Confidence
			if magnitude > maxMagnitude {
				maxMagnitude = magnitude
				dominant = bias
			}
		}
	}

	return dominant
}

// GetActiveLoops returns all currently active feedback loops
func (re *ReflexivityEngine) GetActiveLoops() []FeedbackLoop {
	re.mu.RLock()
	defer re.mu.RUnlock()

	active := make([]FeedbackLoop, 0)
	for _, loop := range re.feedbackLoops {
		if loop.Status == LoopActive || loop.Status == LoopEmerging || loop.Status == LoopMaturing {
			active = append(active, loop)
		}
	}
	return active
}

// GetLoopsByTarget returns feedback loops for a specific target
func (re *ReflexivityEngine) GetLoopsByTarget(target string) []FeedbackLoop {
	re.mu.RLock()
	defer re.mu.RUnlock()

	loops := make([]FeedbackLoop, 0)
	for _, loop := range re.feedbackLoops {
		if loop.Bias != nil && loop.Bias.Target == target {
			loops = append(loops, loop)
		}
	}
	return loops
}

// PredictLoopOutcome forecasts where a feedback loop is likely to lead
func (re *ReflexivityEngine) PredictLoopOutcome(loopID string) (string, float64) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	for _, loop := range re.feedbackLoops {
		if loop.ID == loopID {
			return re.calculateOutcome(loop)
		}
	}

	return "No prediction available", 0.0
}

// calculateOutcome determines likely outcome of a feedback loop
func (re *ReflexivityEngine) calculateOutcome(loop FeedbackLoop) (string, float64) {
	confidence := loop.Strength * loop.Bias.Confidence

	switch loop.Direction {
	case PositiveFeedback:
		if loop.Bias.Magnitude > 0 {
			return "Potential bubble formation - monitor for exhaustion", confidence
		}
		return "Potential crash/capitulation - watch for reversal", confidence
	case NegativeFeedback:
		return "Mean reversion likely - expect stabilization", confidence * 0.7
	default:
		return "Uncertain outcome", 0.0
	}
}

// UpdateLoopStatus changes the status of a feedback loop
func (re *ReflexivityEngine) UpdateLoopStatus(loopID string, status LoopStatus) {
	re.mu.Lock()
	defer re.mu.Unlock()

	for i := range re.feedbackLoops {
		if re.feedbackLoops[i].ID == loopID {
			re.feedbackLoops[i].Status = status
			re.feedbackLoops[i].UpdatedAt = time.Now()
			break
		}
	}
}

// GetReflexivityReport generates comprehensive analysis
func (re *ReflexivityEngine) GetReflexivityReport() *ReflexivityReport {
	re.mu.RLock()
	defer re.mu.RUnlock()

	report := &ReflexivityReport{
		GeneratedAt:    time.Now(),
		TotalLoops:     len(re.feedbackLoops),
		ActiveLoops:    0,
		PositiveLoops:  0,
		NegativeLoops:  0,
		BiasCount:      len(re.biases),
		TargetsCovered: make(map[string]bool),
		LoopDetails:    make([]LoopDetail, 0),
	}

	for _, loop := range re.feedbackLoops {
		if loop.Status == LoopActive || loop.Status == LoopEmerging {
			report.ActiveLoops++
		}
		if loop.Direction == PositiveFeedback {
			report.PositiveLoops++
		} else {
			report.NegativeLoops++
		}
		if loop.Bias != nil {
			report.TargetsCovered[loop.Bias.Target] = true
		}

		outcome, conf := re.calculateOutcome(loop)
		report.LoopDetails = append(report.LoopDetails, LoopDetail{
			LoopID:     loop.ID,
			Target:     loop.Bias.Target,
			Direction:  loop.Direction,
			Strength:   loop.Strength,
			Status:     loop.Status,
			Prediction: outcome,
			Confidence: conf,
		})
	}

	return report
}

// ReflexivityReport provides comprehensive analysis
type ReflexivityReport struct {
	GeneratedAt    time.Time
	TotalLoops     int
	ActiveLoops    int
	PositiveLoops  int
	NegativeLoops  int
	BiasCount      int
	TargetsCovered map[string]bool
	LoopDetails    []LoopDetail
}

// LoopDetail provides details on individual loops
type LoopDetail struct {
	LoopID     string
	Target     string
	Direction  LoopDirection
	Strength   float64
	Status     LoopStatus
	Prediction string
	Confidence float64
}

// Utility functions

func generateLoopID(target string, biasType BiasType) string {
	return fmt.Sprintf("loop_%s_%s_%d", target, biasTypeString(biasType), time.Now().Unix())
}

func biasTypeString(t BiasType) string {
	names := []string{
		"trend", "contrarian", "anchoring", "recency",
		"confirmation", "herding", "overconfidence", "fear_greed",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return "unknown"
}

// ProcessRecommendations analyzes batch of recommendations for reflexive patterns
func (re *ReflexivityEngine) ProcessRecommendations(recs []domain.Recommendation) {
	// Aggregate bias from recommendations
	biasMap := make(map[string][]float64)

	for _, rec := range recs {
		// Convert conviction and side to bias magnitude
		magnitude := float64(rec.Conviction) / 100.0
		if rec.Side == domain.SideSell {
			magnitude = -magnitude
		}

		biasMap[rec.Symbol] = append(biasMap[rec.Symbol], magnitude)
	}

	// Register aggregated biases
	for symbol, magnitudes := range biasMap {
		avgMagnitude := average(magnitudes)
		bias := &MarketBias{
			ID:         fmt.Sprintf("bias_%s_%d", symbol, time.Now().Unix()),
			Type:       Herding, // Aggregate bias treated as herding
			Target:     symbol,
			Magnitude:  avgMagnitude,
			Confidence: math.Min(float64(len(magnitudes))/5.0, 1.0),
			Source:     extractAgentIDs(recs, symbol),
			Timestamp:  time.Now(),
		}
		_ = re.RegisterBias(bias)
	}
}

func extractAgentIDs(recs []domain.Recommendation, symbol string) []string {
	agents := make([]string, 0)
	seen := make(map[string]bool)
	for _, rec := range recs {
		if rec.Symbol == symbol && !seen[rec.Agent] {
			agents = append(agents, rec.Agent)
			seen[rec.Agent] = true
		}
	}
	return agents
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// ApplyReflexivityAdjustment adjusts recommendations based on feedback loops
func (re *ReflexivityEngine) ApplyReflexivityAdjustment(recs []domain.Recommendation) []domain.Recommendation {
	adjusted := make([]domain.Recommendation, len(recs))
	copy(adjusted, recs)

	for i, rec := range adjusted {
		// Check for active loops on this symbol
		loops := re.GetLoopsByTarget(rec.Symbol)
		if len(loops) == 0 {
			continue
		}

		// Find strongest positive feedback loop
		var strongestLoop *FeedbackLoop
		maxStrength := 0.0
		for _, loop := range loops {
			if loop.Direction == PositiveFeedback && loop.Strength > maxStrength {
				maxStrength = loop.Strength
				strongestLoop = &loop
			}
		}

		if strongestLoop == nil {
			continue
		}

		// Adjust based on loop direction and rec side
		adjustment := 0.0
		if rec.Side == domain.SideBuy && strongestLoop.Bias.Magnitude > 0 {
			// Positive feedback on buy - could be bubble, reduce conviction
			adjustment = -strongestLoop.Strength * 20
		} else if rec.Side == domain.SideSell && strongestLoop.Bias.Magnitude < 0 {
			// Positive feedback on sell - could be crash, increase conviction
			adjustment = strongestLoop.Strength * 10
		}

		adjusted[i].Conviction = int(math.Max(1, math.Min(100, float64(rec.Conviction)+adjustment)))
		adjusted[i].Reason = fmt.Sprintf("%s [Reflex: %.0f%%]", rec.Reason, strongestLoop.Strength*100)
	}

	return adjusted
}
