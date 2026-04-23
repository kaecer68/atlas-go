package monitoring

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type EmailNotifier struct {
	smtpHost string
	smtpPort int
	from     string
	to       []string
	password string
}

func NewEmailNotifier(cfg domain.AlertChannelConfig) *EmailNotifier {
	return &EmailNotifier{
		smtpHost: cfg.EmailSMTPHost,
		smtpPort: cfg.EmailSMTPPort,
		from:     cfg.EmailFrom,
		to:       cfg.EmailTo,
		password: cfg.EmailPassword,
	}
}

func (n *EmailNotifier) Name() string { return "email" }

func (n *EmailNotifier) IsConfigured() bool {
	return n.smtpHost != "" && n.smtpPort > 0 && n.from != "" && len(n.to) > 0
}

func (n *EmailNotifier) Notify(alert domain.AlertRecord) error {
	if !n.IsConfigured() {
		return fmt.Errorf("email notifier not configured")
	}

	msg := buildEmailMessage(n.from, alert)
	addr := fmt.Sprintf("%s:%d", n.smtpHost, n.smtpPort)

	auth := smtp.PlainAuth("", n.from, n.password, n.smtpHost)
	return smtp.SendMail(addr, auth, n.from, n.to, []byte(msg))
}

func buildEmailMessage(from string, alert domain.AlertRecord) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join([]string{from}, ", ") + "\r\n")
	b.WriteString("Subject: [Atlas Alert] " + alert.Severity + " - " + alert.Rule + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(fmt.Sprintf("Rule: %s\n", alert.Rule))
	b.WriteString(fmt.Sprintf("Severity: %s\n", alert.Severity))
	b.WriteString(fmt.Sprintf("Message: %s\n", alert.Message))
	b.WriteString(fmt.Sprintf("Value: %.2f\n", alert.Value))
	b.WriteString(fmt.Sprintf("Threshold: %.2f\n", alert.Threshold))
	b.WriteString(fmt.Sprintf("Time: %s\n", alert.Timestamp.Format("2006-01-02 15:04:05")))
	return b.String()
}
