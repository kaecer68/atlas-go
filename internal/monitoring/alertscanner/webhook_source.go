package alertscanner

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// WebhookFetcher is the subset of alerting.AlertWebhookHandler that
// WebhookSource needs. It exists so unit tests can supply a stub
// without depending on the full HTTP handler.
type WebhookFetcher interface {
	Recent(n int) []alerting.AlertmanagerAlert
	Len() int
}

// WebhookSource adapts a Prometheus Alertmanager webhook receiver
// (alerting.AlertWebhookHandler) to the AlertSource interface.
// It converts Alertmanager firing/resolved alerts into domain.AlertRecord
// entries and surfaces them through the unified scanner pipeline.
type WebhookSource struct {
	fetcher WebhookFetcher
}

// NewWebhookSource creates an AlertSource backed by an Alertmanager
// webhook receiver. fetcher is typically an *alerting.AlertWebhookHandler.
func NewWebhookSource(fetcher WebhookFetcher) *WebhookSource {
	return &WebhookSource{fetcher: fetcher}
}

func (w *WebhookSource) Name() string { return "webhook" }

func (w *WebhookSource) ListActive(ctx context.Context) ([]domain.AlertRecord, error) {
	if w.fetcher == nil {
		return nil, nil
	}
	alerts := w.fetcher.Recent(0) // 0 = all retained
	records := make([]domain.AlertRecord, 0, len(alerts))
	for _, a := range alerts {
		// Only include firing alerts (not resolved). Resolved alerts
		// are informational and should not block workflows.
		if a.Status != "firing" {
			continue
		}
		records = append(records, convertAlertmanager(a))
	}
	return records, nil
}

// convertAlertmanager maps an Alertmanager webhook alert to a domain.AlertRecord.
func convertAlertmanager(a alerting.AlertmanagerAlert) domain.AlertRecord {
	severity := a.Labels["severity"]
	if severity == "" {
		severity = "warning"
	}
	alertname := a.Labels["alertname"]
	msg := a.Annotations["summary"]
	if msg == "" {
		msg = a.Annotations["description"]
	}
	if msg == "" {
		msg = alertname
	}

	id := fmt.Sprintf("am-%s-%d", alertname, a.StartsAt.Unix())
	now := time.Now()
	return domain.AlertRecord{
		ID:        id,
		Timestamp: a.StartsAt,
		Rule:      alertname,
		Severity:  severity,
		Message:   msg,
		Status:    domain.AlertStatus(a.Status),
		DedupKey:  a.Labels["alertname"],
		FirstSeen: &a.StartsAt,
		LastSeen:  &now,
	}
}
