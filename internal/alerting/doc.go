// Package alerting receives Alertmanager webhooks for the atlas-go
// observability stack. AlertWebhookHandler decodes the standard
// Alertmanager payload, retains each alert in an in-memory ring buffer
// (cap 1000) for downstream consumers, and acknowledges with 200.
//
// Recent(n) exposes the snapshot for SSE streams or recent-alerts
// endpoints. The handler is safe for concurrent writes and rejects
// non-POST (405) and malformed JSON (400) per HTTP convention.
//
// Maturity: experimental
package alerting
