package monitoring

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestTelegramNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  domain.AlertChannelConfig
		want bool
	}{
		{"fully configured", domain.AlertChannelConfig{TelegramBotToken: "tok", TelegramChatID: "123"}, true},
		{"missing token", domain.AlertChannelConfig{TelegramChatID: "123"}, false},
		{"missing chat ID", domain.AlertChannelConfig{TelegramBotToken: "tok"}, false},
		{"empty", domain.AlertChannelConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewTelegramNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTelegramNotifier_Name(t *testing.T) {
	n := NewTelegramNotifier(domain.AlertChannelConfig{})
	if n.Name() != "telegram" {
		t.Errorf("Name() = %q, want telegram", n.Name())
	}
}

func TestTelegramNotifier_Notify_NotConfigured(t *testing.T) {
	n := NewTelegramNotifier(domain.AlertChannelConfig{})
	err := n.Notify(domain.AlertRecord{ID: "1"})
	if err == nil {
		t.Fatal("expected error when not configured, got nil")
	}
}

func TestTelegramNotifier_Notify_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	n := &TelegramNotifier{
		botToken: "test-token",
		chatID:   "123",
		baseURL:  server.URL,
		client:   &http.Client{},
	}

	alert := domain.AlertRecord{
		ID:        "alert-1",
		Rule:      "test_rule",
		Severity:  "WARNING",
		Message:   "test",
		Value:     42.0,
		Threshold: 50.0,
	}

	err := n.Notify(alert)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestEmailNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  domain.AlertChannelConfig
		want bool
	}{
		{
			"fully configured",
			domain.AlertChannelConfig{EmailSMTPHost: "smtp.example.com", EmailSMTPPort: 587, EmailFrom: "a@b.com", EmailTo: []string{"x@y.com"}},
			true,
		},
		{"missing host", domain.AlertChannelConfig{EmailSMTPPort: 587, EmailFrom: "a@b.com", EmailTo: []string{"x@y.com"}}, false},
		{"missing port", domain.AlertChannelConfig{EmailSMTPHost: "smtp.example.com", EmailFrom: "a@b.com", EmailTo: []string{"x@y.com"}}, false},
		{"missing from", domain.AlertChannelConfig{EmailSMTPHost: "smtp.example.com", EmailSMTPPort: 587, EmailTo: []string{"x@y.com"}}, false},
		{"missing to", domain.AlertChannelConfig{EmailSMTPHost: "smtp.example.com", EmailSMTPPort: 587, EmailFrom: "a@b.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewEmailNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailNotifier_Name(t *testing.T) {
	n := NewEmailNotifier(domain.AlertChannelConfig{})
	if n.Name() != "email" {
		t.Errorf("Name() = %q, want email", n.Name())
	}
}

func TestEmailNotifier_Notify_NotConfigured(t *testing.T) {
	n := NewEmailNotifier(domain.AlertChannelConfig{})
	err := n.Notify(domain.AlertRecord{ID: "1"})
	if err == nil {
		t.Fatal("expected error when not configured, got nil")
	}
}

func TestWebhookNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  domain.AlertChannelConfig
		want bool
	}{
		{"configured", domain.AlertChannelConfig{WebhookURL: "https://example.com/hook"}, true},
		{"empty", domain.AlertChannelConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewWebhookNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebhookNotifier_Name(t *testing.T) {
	n := NewWebhookNotifier(domain.AlertChannelConfig{})
	if n.Name() != "webhook" {
		t.Errorf("Name() = %q, want webhook", n.Name())
	}
}

func TestWebhookNotifier_Notify_NotConfigured(t *testing.T) {
	n := NewWebhookNotifier(domain.AlertChannelConfig{})
	err := n.Notify(domain.AlertRecord{ID: "1"})
	if err == nil {
		t.Fatal("expected error when not configured, got nil")
	}
}

func TestWebhookNotifier_Notify_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(domain.AlertChannelConfig{WebhookURL: server.URL})
	alert := domain.AlertRecord{ID: "alert-1", Rule: "test", Severity: "INFO", Message: "hello"}

	if err := n.Notify(alert); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestWebhookNotifier_Notify_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "secret" {
			t.Errorf("X-API-Key = %q, want secret", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(domain.AlertChannelConfig{
		WebhookURL:     server.URL,
		WebhookHeaders: map[string]string{"X-API-Key": "secret"},
	})
	if err := n.Notify(domain.AlertRecord{ID: "1"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestWebhookNotifier_Notify_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	n := NewWebhookNotifier(domain.AlertChannelConfig{WebhookURL: server.URL})
	err := n.Notify(domain.AlertRecord{ID: "1"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}
