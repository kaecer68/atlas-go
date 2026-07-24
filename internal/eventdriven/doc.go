// Package eventdriven provides event-driven capital flow prediction.
//
// It maps Taiwan market events (ETF rebalances, revenue announcements,
// MSCI adjustments, etc.) to predicted capital flow directions over a
// 1-5 day horizon, weighted by current capital quality scores from
// the internal/capitalflow module.
//
// Core investment logic:
//
//	"事件驅動資金流決定節奏"
//
// This module extends the existing EventCalendar (internal/industry/)
// with a prediction layer that answers: "Given upcoming events, where
// will smart money move in the next 1-5 days?"
//
// API endpoints:
//   - GET /api/events/prediction — 5-day capital flow prediction
//   - GET /api/events/calendar — upcoming event list (wraps EventCalendar)
//
//
// Package independence: This package is NOT related to eventbus or eventquality
// despite the shared \"event\" prefix. eventbus is a pub/sub infrastructure layer;
// eventquality validates event data quality for industry.EventCalendar.
// eventdriven consumes industry.EventCalendar + capitalflow, produces FlowPrediction
// consumed by recommender.
// Maturity: evolving
package eventdriven
