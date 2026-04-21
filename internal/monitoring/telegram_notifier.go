package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type TelegramNotifier struct {
	botToken string
	chatID   string
	baseURL  string
	client   *http.Client
}

func NewTelegramNotifier(cfg domain.AlertChannelConfig) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: cfg.TelegramBotToken,
		chatID:   cfg.TelegramChatID,
		baseURL:  "https://api.telegram.org",
		client:   &http.Client{},
	}
}

func (n *TelegramNotifier) Name() string { return "telegram" }

func (n *TelegramNotifier) IsConfigured() bool {
	return n.botToken != "" && n.chatID != ""
}

func (n *TelegramNotifier) Notify(alert domain.AlertRecord) error {
	if !n.IsConfigured() {
		return fmt.Errorf("telegram notifier not configured")
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", n.baseURL, n.botToken)

	body := map[string]string{
		"chat_id": n.chatID,
		"text":    formatAlertText(alert),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func formatAlertText(alert domain.AlertRecord) string {
	return fmt.Sprintf(
		"🔔 *Alert*\n\n*Rule:* %s\n*Severity:* %s\n*Message:* %s\n*Value:* %.2f\n*Threshold:* %.2f\n*Time:* %s",
		alert.Rule,
		alert.Severity,
		alert.Message,
		alert.Value,
		alert.Threshold,
		alert.Timestamp.Format("2006-01-02 15:04:05"),
	)
}
