package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Notifier interface {
	Notify(alert domain.AlertRecord) error
	Name() string
	IsConfigured() bool
}

type WebhookNotifier struct {
	URL     string
	Headers map[string]string
	client  *http.Client
}

func NewWebhookNotifier(url string, headers map[string]string) *WebhookNotifier {
	return &WebhookNotifier{
		URL:     url,
		Headers: headers,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *WebhookNotifier) Name() string {
	return "webhook"
}

func (n *WebhookNotifier) IsConfigured() bool {
	return n.URL != ""
}

func (n *WebhookNotifier) Notify(alert domain.AlertRecord) error {
	if !n.IsConfigured() {
		return fmt.Errorf("webhook not configured")
	}

	payload := map[string]any{
		"rule":      alert.Rule,
		"severity":  alert.Severity,
		"message":   alert.Message,
		"timestamp": alert.Timestamp,
		"value":     alert.Value,
		"threshold": alert.Threshold,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, n.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

type TelegramNotifier struct {
	BotToken string
	ChatID   string
	baseURL  string
	client   *http.Client
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		BotToken: botToken,
		ChatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *TelegramNotifier) Name() string {
	return "telegram"
}

func (n *TelegramNotifier) IsConfigured() bool {
	return n.BotToken != "" && n.ChatID != ""
}

func (n *TelegramNotifier) Notify(alert domain.AlertRecord) error {
	if !n.IsConfigured() {
		return fmt.Errorf("telegram not configured")
	}

	message := fmt.Sprintf("🚨 *%s* \n\n"+
		"*Rule:* %s\n"+
		"*Message:* %s\n"+
		"*Time:* %s",
		alert.Severity,
		alert.Rule,
		alert.Message,
		alert.Timestamp.Format("2006-01-02 15:04:05"))

	url := n.baseURL
	if url == "" {
		url = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.BotToken)
	}
	payload := map[string]any{
		"chat_id":    n.ChatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	resp, err := n.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("send telegram request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	return nil
}

type MultiNotifier struct {
	notifiers []Notifier
}

func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

func (m *MultiNotifier) AddNotifier(n Notifier) {
	m.notifiers = append(m.notifiers, n)
}

func (m *MultiNotifier) Notify(alert domain.AlertRecord) []error {
	var errs []error
	for _, n := range m.notifiers {
		if !n.IsConfigured() {
			continue
		}
		if err := n.Notify(alert); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n.Name(), err))
		}
	}
	return errs
}

func (m *MultiNotifier) NotifierNames() []string {
	var names []string
	for _, n := range m.notifiers {
		if n.IsConfigured() {
			names = append(names, n.Name())
		}
	}
	return names
}
