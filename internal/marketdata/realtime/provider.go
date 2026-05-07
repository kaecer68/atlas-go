// Package realtime 定義即時行情串流介面，供 WebSocket/SSE 等實作遵循
package realtime

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteCallback 是即時報價回呼函式型別
type QuoteCallback func(quote domain.Quote)

// RealtimeProvider 即時行情供應者介面
type RealtimeProvider interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Subscribe(symbols []string) error
	Unsubscribe(symbols []string) error
	OnQuote(callback QuoteCallback)
	IsConnected() bool
	Name() string
}

type ProviderState string

const (
	ProviderStateDisconnected ProviderState = "disconnected"
	ProviderStateConnecting   ProviderState = "connecting"
	ProviderStateConnected    ProviderState = "connected"
	ProviderStateReconnecting ProviderState = "reconnecting"
	ProviderStateFailed       ProviderState = "failed"
)

type ProviderStatus struct {
	Name            string        `json:"name"`
	State           ProviderState `json:"state"`
	SubscribedCount int           `json:"subscribed_count"`
	ReconnectCount  int           `json:"reconnect_count"`
	LastError       string        `json:"last_error,omitempty"`
}
