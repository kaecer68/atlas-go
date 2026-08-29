package eventbus

import (
	"slices"
	"sync"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// SSEClient represents a single SSE client connection.
type SSEClient struct {
	ID     string
	Types  []EventType
	Events chan BusEvent
}

// SSEBridge bridges EventBus events to SSE clients.
type SSEBridge struct {
	clients map[string]*SSEClient
	mutex   sync.RWMutex
}

// NewSSEBridge creates a new SSE bridge.
func NewSSEBridge() *SSEBridge {
	return &SSEBridge{
		clients: make(map[string]*SSEClient),
	}
}

// AddClient adds a new SSE client.
func (b *SSEBridge) AddClient(id string, types []EventType) *SSEClient {
	client := &SSEClient{
		ID:     id,
		Types:  types,
		Events: make(chan BusEvent, 128),
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.clients[id] = client

	return client
}

// RemoveClient removes an SSE client.
func (b *SSEBridge) RemoveClient(id string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if client, ok := b.clients[id]; ok {
		close(client.Events)
		delete(b.clients, id)
	}
}

// Broadcast sends an event to all matching clients.
func (b *SSEBridge) Broadcast(event BusEvent) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, client := range b.clients {
		if !b.matchesFilter(client, event.Type) {
			continue
		}

		select {
		case client.Events <- event:
		default:
			// Channel full, drop oldest event.
			select {
			case <-client.Events:
			default:
			}
			select {
			case client.Events <- event:
			default:
			}
			logging.Info("ssebridge", "dropped_oldest_event", "client_id", client.ID, "reason", "channel full")
		}
	}
}

// matchesFilter checks if event type matches client's filter.
func (b *SSEBridge) matchesFilter(client *SSEClient, eventType EventType) bool {
	if len(client.Types) == 0 {
		return true
	}
	return slices.Contains(client.Types, eventType)
}

// ClientCount returns the number of connected clients.
func (b *SSEBridge) ClientCount() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return len(b.clients)
}
