// Package dailyreport generates automated daily market reports.
//
// Reports are triggered at 14:30 (market close + 30 min) and include:
//   - Global capital flow overview (bond, USD, JPY, VIX)
//   - Taiwan capital flow decomposition (7 forces + resonance)
//   - Event calendar (tomorrow's events)
//   - Strategy signals (recommended strategy + entry conditions)
//   - Risk warnings (stress index, drawdown alert)
//
// Output formats: JSON (API/MCP), Markdown (web display), optional HTML email.
//
// Maturity: evolving
package dailyreport
