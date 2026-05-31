package marketdata

// CalendarProviderData represents a calendar event from an external data provider.
// This is a marketdata-package type; industry package converts these into its own
// CalendarEvent structs. This avoids circular imports.
//
// Maturity: evolving
type CalendarProviderData struct {
	Date        string  `json:"date"`        // ISO date "2006-01-02"
	EventType   string  `json:"event_type"`  // "ex_dividend", "shareholder_meeting"
	Name        string  `json:"name"`        // Human-readable name
	Symbol      string  `json:"symbol"`      // Stock symbol (e.g., "2330"), empty for market-wide
	Direction   string  `json:"direction"`   // "bullish", "bearish", "mixed", "neutral"
	Weight      float64 `json:"weight"`      // 0.0–1.0 significance weight
	Description string  `json:"description"` // Additional context
	Source      string  `json:"source"`      // "twse", "finmind", etc.
}
