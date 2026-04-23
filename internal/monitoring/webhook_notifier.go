package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type WebhookNotifier struct {
	url     string
	headers map[string]string
	client  *http.Client
}

func NewWebhookNotifier(cfg domain.AlertChannelConfig) *WebhookNotifier {
	return &WebhookNotifier{
		url:     cfg.WebhookURL,
		headers: cfg.WebhookHeaders,
		client:  &http.Client{},
	}
}

func (n *WebhookNotifier) Name() string { return "webhook" }

func (n *WebhookNotifier) IsConfigured() bool {
	return n.url != ""
}

func (n *WebhookNotifier) Notify(alert domain.AlertRecord) error {
	if !n.IsConfigured() {
		return fmt.Errorf("webhook notifier not configured")
	}

	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook error: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
