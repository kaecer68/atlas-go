package monitoring

import "github.com/kaecer68/atlas-go/internal/domain"

// Notifier defines the interface for alert dispatch channels.
type Notifier interface {
	Notify(alert domain.AlertRecord) error
	Name() string
	IsConfigured() bool
}
