package narrative

import "time"

// NarrativeEvent represents a detected macro event that may trigger causal chains.
type NarrativeEvent struct {
	ID               string             `json:"id"`
	Theme            string             `json:"theme"`      // e.g., "US_rates_up"
	Region           string             `json:"region"`     // e.g., "US", "Global", "Asia"
	Sentiment        float64            `json:"sentiment"`  // -1.0 (very negative) to +1.0
	Confidence       float64            `json:"confidence"` // 0.0 to 1.0
	ConfidenceSource string             `json:"confidence_source"`
	HitRate          float64            `json:"hit_rate"`
	CapitalFlow      string             `json:"capital_flow"` // e.g., "flight_to_USD", "risk_off"
	TimeWindow       string             `json:"time_window"`  // "immediate", "1_week", "1_month"
	Timestamp        time.Time          `json:"timestamp"`
	SourceData       map[string]float64 `json:"source_data,omitempty"`
	Duration         time.Duration      `json:"duration"`
	ExpiresAt        time.Time          `json:"expires_at"`
	Severity         string             `json:"severity"`
	Status           string             `json:"status"`

	TaxonomyL1 TaxonomyL1 `json:"taxonomy_l1"`
	TaxonomyL2 TaxonomyL2 `json:"taxonomy_l2"`

	// Explanation is an optional LLM-generated regime explanation for this
	// event, populated by RegimeExplainer when LLM_NARRATIVE_EXPLAIN_ENABLED
	// is true. Uses var indirection to avoid narrative→llm import cycle.
	Explanation string `json:"explanation,omitempty"`

	// SentimentExplanation is an optional LLM-generated sentiment analysis
	// for this event, populated by SentimentExplainer when
	// LLM_NARRATIVE_EXPLAIN_ENABLED is true.
	SentimentExplanation string `json:"sentiment_explanation,omitempty"`
}

func (e *NarrativeEvent) NormalizeTaxonomy() {
	if e.TaxonomyL1 == "" {
		e.TaxonomyL1 = TaxonomyL1Uncategorized
	}
	if e.TaxonomyL2 == "" {
		e.TaxonomyL2 = TaxonomyL2Uncategorized
	}
}

// CausalStep represents one step in a causal transmission chain.
type CausalStep struct {
	Description string   `json:"description"`
	Affected    []string `json:"affected"` // sectors, asset classes, or symbols
	Impact      float64  `json:"impact"`   // -1.0 to +1.0
}

// CausalTemplate is a reusable "if A then B" macro narrative template.
type CausalTemplate struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	TriggerTheme      string       `json:"trigger_theme"`
	RequiredRegion    string       `json:"required_region,omitempty"`
	Steps             []CausalStep `json:"steps"`
	HistoricalHitRate float64      `json:"historical_hit_rate"`
	SourceReferences  []string     `json:"source_references"`
	Rationale         string       `json:"rationale"`
}

// CausalChain is an instantiated causal chain from a specific event.
type CausalChain struct {
	EventID         string       `json:"event_id"`
	TemplateID      string       `json:"template_id"`
	TriggerTheme    string       `json:"trigger_theme"`    // the event theme that triggered this chain
	AffectedSectors []string     `json:"affected_sectors"` // aggregated sectors from all steps
	FavoredSectors  []string     `json:"favored_sectors"`  // sectors with net positive impact
	AvoidedSectors  []string     `json:"avoided_sectors"`  // sectors with net negative impact
	Steps           []CausalStep `json:"steps"`
	Score           float64      `json:"score"`       // combined confidence * historical hit rate
	DetectedAt      time.Time    `json:"detected_at"` // when the trigger event was detected
}

// InvestmentModel represents a narrative-driven investment hypothesis.
type InvestmentModel struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Rationale        string   `json:"rationale"`
	ActiveThemes     []string `json:"active_themes"`
	FavoredSectors   []string `json:"favored_sectors"`
	AvoidedSectors   []string `json:"avoided_sectors"`
	RecentPrediction float64  `json:"recent_prediction"`
	RecentError      float64  `json:"recent_error"` // lower is better
	HitRate          float64  `json:"hit_rate"`     // 1 - RecentError, clamped to [0, 1]
	Weight           float64  `json:"weight"`
}
