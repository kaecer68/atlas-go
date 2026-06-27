// Package scheduler provides background task scheduling for periodic
// maintenance operations including ML model retraining.
//
// All schedules MUST be registered via BackgroundTaskManager. Do NOT
// launch time.Ticker directly. Model storage format is shared with
// internal/ml — coordinate serialization changes.
//
// Maturity: evolving
package scheduler
