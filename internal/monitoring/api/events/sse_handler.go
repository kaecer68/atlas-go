package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// SSEClient represents a single SSE connection.
type SSEClient struct {
	id     string
	types  []eventbus.EventType
	events chan eventbus.BusEvent
}

// SSEHandler streams EventBus events over Server-Sent Events.
type SSEHandler struct {
	eventBus    *eventbus.ChannelEventBus
	clients     map[string]*SSEClient
	clientCount int64
	mutex       sync.RWMutex
	maxClients  int
}

// BufferedNarrativeEvent holds a published narrative event for catchup.
type BufferedNarrativeEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedNarrativeEvents = 50

var (
	narrativeBuffer    []BufferedNarrativeEvent
	lastNarrativeMutex sync.RWMutex
)

const defaultMaxSSEClients = 20

// BufferNarrativeEvent stores a narrative event for catchup by new SSE clients.
func BufferNarrativeEvent(event eventbus.BusEvent) {
	lastNarrativeMutex.Lock()
	defer lastNarrativeMutex.Unlock()
	narrativeBuffer = append(narrativeBuffer, BufferedNarrativeEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(narrativeBuffer) > maxBufferedNarrativeEvents {
		narrativeBuffer = narrativeBuffer[len(narrativeBuffer)-maxBufferedNarrativeEvents:]
	}
}

// NewSSEHandler creates a new SSE handler.
func NewSSEHandler(eventBus *eventbus.ChannelEventBus) *SSEHandler {
	return &SSEHandler{
		eventBus:   eventBus,
		clients:    make(map[string]*SSEClient),
		maxClients: defaultMaxSSEClients,
	}
}

// ServeHTTP implements the http.Handler interface for SSE.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Enforce max concurrent connections
	count := atomic.LoadInt64(&h.clientCount)
	if count >= int64(h.maxClients) {
		fmt.Fprintf(w, "event: error\ndata: {\"message\":\"too many connections\"}\n\n")
		flusher.Flush()
		return
	}

	clientID := fmt.Sprintf("sse-%d", time.Now().UnixNano())
	filterTypes := h.parseFilterTypes(r)

	client := &SSEClient{
		id:     clientID,
		types:  filterTypes,
		events: make(chan eventbus.BusEvent, 128),
	}

	h.mutex.Lock()
	h.clients[clientID] = client
	h.mutex.Unlock()
	atomic.AddInt64(&h.clientCount, 1)

	// Send initial connected event.
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", `{"client_id":"`+clientID+`"}`)
	flusher.Flush()

	// Send any buffered narrative events for catchup.
	lastNarrativeMutex.RLock()
	buffered := narrativeBuffer
	lastNarrativeMutex.RUnlock()
	for _, b := range buffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	// Subscribe to EventBus and forward events to this client.
	sub := h.eventBus.SubscribeAll(func(ctx context.Context, event eventbus.BusEvent) error {
		if event.Type == eventbus.EventNarrative {
			BufferNarrativeEvent(event)
		}
		if !h.matchesFilter(client, event.Type) {
			return nil
		}
		select {
		case client.events <- event:
		default:
			// Drop oldest event if channel is full.
			select {
			case <-client.events:
			default:
			}
			select {
			case client.events <- event:
			default:
			}
		}
		return nil
	})

	// Stream events until client disconnects.
	ctx := r.Context()
	for {
		select {
		case event := <-client.events:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		case <-ctx.Done():
			sub.Cancel()
			h.mutex.Lock()
			delete(h.clients, clientID)
			h.mutex.Unlock()
			atomic.AddInt64(&h.clientCount, -1)
			close(client.events)
			return
		}
	}
}

// parseFilterTypes extracts event type filters from query parameter.
func (h *SSEHandler) parseFilterTypes(r *http.Request) []eventbus.EventType {
	param := r.URL.Query().Get("type")
	if param == "" {
		return nil
	}

	parts := strings.Split(param, ",")
	types := make([]eventbus.EventType, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			types = append(types, eventbus.EventType(p))
		}
	}
	return types
}

// matchesFilter checks if the event type matches the client's filter.
func (h *SSEHandler) matchesFilter(client *SSEClient, eventType eventbus.EventType) bool {
	if len(client.types) == 0 {
		return true
	}
	for _, t := range client.types {
		if t == eventType {
			return true
		}
	}
	return false
}
