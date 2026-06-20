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
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
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

// BufferedPromotionRecordedEvent holds a published promotion-recorded event for SSE catchup.
type BufferedPromotionRecordedEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedPromotionEvents = 50

var (
	promotionBuffer    []BufferedPromotionRecordedEvent
	lastPromotionMutex sync.RWMutex
)

// BufferedHealthAlertEvent holds a published health-alert event for SSE catchup.
type BufferedHealthAlertEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedHealthAlertEvents = 50

var (
	healthAlertBuffer    []BufferedHealthAlertEvent
	lastHealthAlertMutex sync.RWMutex
)

// BufferedRiskGateEvent holds a published risk-gate event for SSE catchup.
type BufferedRiskGateEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedRiskGateEvents = 50

var (
	riskGateBuffer    []BufferedRiskGateEvent
	lastRiskGateMutex sync.RWMutex
)

// BufferedIndustryCalendarEvent holds a published industry calendar event for SSE catchup.
type BufferedIndustryCalendarEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

// BufferedBacktestCompletedEvent holds a published backtest-completed event for SSE catchup.
type BufferedBacktestCompletedEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedIndustryCalendarEvents = 50

var (
	industryCalendarBuffer    []BufferedIndustryCalendarEvent
	lastIndustryCalendarMutex sync.RWMutex
)

// BufferIndustryCalendarEvent stores an industry calendar event for catchup by new SSE clients.
func BufferIndustryCalendarEvent(event eventbus.BusEvent) {
	lastIndustryCalendarMutex.Lock()
	defer lastIndustryCalendarMutex.Unlock()
	industryCalendarBuffer = append(industryCalendarBuffer, BufferedIndustryCalendarEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(industryCalendarBuffer) > maxBufferedIndustryCalendarEvents {
		industryCalendarBuffer = industryCalendarBuffer[len(industryCalendarBuffer)-maxBufferedIndustryCalendarEvents:]
	}
}

// GetBufferedIndustryCalendarEvents returns a snapshot of the latest industry calendar events for SSE catchup.
func GetBufferedIndustryCalendarEvents() []BufferedIndustryCalendarEvent {
	lastIndustryCalendarMutex.RLock()
	defer lastIndustryCalendarMutex.RUnlock()
	result := make([]BufferedIndustryCalendarEvent, len(industryCalendarBuffer))
	copy(result, industryCalendarBuffer)
	return result
}

// resetIndustryCalendarBuffer clears the industry calendar buffer. Test-only helper.
func resetIndustryCalendarBuffer() {
	lastIndustryCalendarMutex.Lock()
	defer lastIndustryCalendarMutex.Unlock()
	industryCalendarBuffer = nil
}

const maxBufferedBacktestCompletedEvents = 50

var (
	backtestCompletedBuffer    []BufferedBacktestCompletedEvent
	lastBacktestCompletedMutex sync.RWMutex
)

// BufferRiskGateEvent stores a risk-gate event for catchup by new SSE clients.
func BufferRiskGateEvent(event eventbus.BusEvent) {
	lastRiskGateMutex.Lock()
	defer lastRiskGateMutex.Unlock()
	riskGateBuffer = append(riskGateBuffer, BufferedRiskGateEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(riskGateBuffer) > maxBufferedRiskGateEvents {
		riskGateBuffer = riskGateBuffer[len(riskGateBuffer)-maxBufferedRiskGateEvents:]
	}
}

// GetBufferedRiskGateEvents returns a snapshot of the latest risk-gate events for SSE catchup.
func GetBufferedRiskGateEvents() []BufferedRiskGateEvent {
	lastRiskGateMutex.RLock()
	defer lastRiskGateMutex.RUnlock()
	result := make([]BufferedRiskGateEvent, len(riskGateBuffer))
	copy(result, riskGateBuffer)
	return result
}

// BufferBacktestCompletedEvent stores a backtest-completed event for catchup by new SSE clients.
func BufferBacktestCompletedEvent(event eventbus.BusEvent) {
	lastBacktestCompletedMutex.Lock()
	defer lastBacktestCompletedMutex.Unlock()
	backtestCompletedBuffer = append(backtestCompletedBuffer, BufferedBacktestCompletedEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(backtestCompletedBuffer) > maxBufferedBacktestCompletedEvents {
		backtestCompletedBuffer = backtestCompletedBuffer[len(backtestCompletedBuffer)-maxBufferedBacktestCompletedEvents:]
	}
}

// GetBufferedBacktestCompletedEvents returns a snapshot of buffered backtest-completed events.
func GetBufferedBacktestCompletedEvents() []BufferedBacktestCompletedEvent {
	lastBacktestCompletedMutex.RLock()
	defer lastBacktestCompletedMutex.RUnlock()
	out := make([]BufferedBacktestCompletedEvent, len(backtestCompletedBuffer))
	copy(out, backtestCompletedBuffer)
	return out
}

// BufferedCalibrationCompletedEvent holds a published calibration-completed event for SSE catchup.
type BufferedCalibrationCompletedEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedCalibrationCompletedEvents = 50

var (
	calibrationCompletedBuffer    []BufferedCalibrationCompletedEvent
	lastCalibrationCompletedMutex sync.RWMutex
)

// BufferCalibrationCompletedEvent stores a calibration-completed event for catchup by new SSE clients.
func BufferCalibrationCompletedEvent(event eventbus.BusEvent) {
	lastCalibrationCompletedMutex.Lock()
	defer lastCalibrationCompletedMutex.Unlock()
	calibrationCompletedBuffer = append(calibrationCompletedBuffer, BufferedCalibrationCompletedEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(calibrationCompletedBuffer) > maxBufferedCalibrationCompletedEvents {
		calibrationCompletedBuffer = calibrationCompletedBuffer[len(calibrationCompletedBuffer)-maxBufferedCalibrationCompletedEvents:]
	}
}

// GetBufferedCalibrationCompletedEvents returns a snapshot of buffered calibration-completed events.
func GetBufferedCalibrationCompletedEvents() []BufferedCalibrationCompletedEvent {
	lastCalibrationCompletedMutex.RLock()
	defer lastCalibrationCompletedMutex.RUnlock()
	out := make([]BufferedCalibrationCompletedEvent, len(calibrationCompletedBuffer))
	copy(out, calibrationCompletedBuffer)
	return out
}

func resetCalibrationCompletedBuffer() {
	lastCalibrationCompletedMutex.Lock()
	defer lastCalibrationCompletedMutex.Unlock()
	calibrationCompletedBuffer = nil
}

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

// BufferPromotionRecordedEvent stores a promotion-recorded event for SSE catchup.
func BufferPromotionRecordedEvent(event eventbus.BusEvent) {
	lastPromotionMutex.Lock()
	defer lastPromotionMutex.Unlock()
	promotionBuffer = append(promotionBuffer, BufferedPromotionRecordedEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(promotionBuffer) > maxBufferedPromotionEvents {
		promotionBuffer = promotionBuffer[len(promotionBuffer)-maxBufferedPromotionEvents:]
	}
}

// GetBufferedPromotionEvents returns a snapshot of buffered promotion-recorded events.
func GetBufferedPromotionEvents() []BufferedPromotionRecordedEvent {
	lastPromotionMutex.RLock()
	defer lastPromotionMutex.RUnlock()
	out := make([]BufferedPromotionRecordedEvent, len(promotionBuffer))
	copy(out, promotionBuffer)
	return out
}

// resetPromotionBuffer clears the promotion buffer. Test-only helper.
func resetPromotionBuffer() {
	lastPromotionMutex.Lock()
	defer lastPromotionMutex.Unlock()
	promotionBuffer = nil
}

// BufferHealthAlertEvent stores a health-alert event for catchup by new SSE clients.
func BufferHealthAlertEvent(event eventbus.BusEvent) {
	lastHealthAlertMutex.Lock()
	defer lastHealthAlertMutex.Unlock()
	healthAlertBuffer = append(healthAlertBuffer, BufferedHealthAlertEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(healthAlertBuffer) > maxBufferedHealthAlertEvents {
		healthAlertBuffer = healthAlertBuffer[len(healthAlertBuffer)-maxBufferedHealthAlertEvents:]
	}
}

// GetBufferedHealthAlerts returns a snapshot of buffered health-alert events.
func GetBufferedHealthAlerts() []BufferedHealthAlertEvent {
	lastHealthAlertMutex.RLock()
	defer lastHealthAlertMutex.RUnlock()
	out := make([]BufferedHealthAlertEvent, len(healthAlertBuffer))
	copy(out, healthAlertBuffer)
	return out
}

// resetHealthAlertBuffer clears the health-alert buffer. Test-only helper.
func resetHealthAlertBuffer() {
	lastHealthAlertMutex.Lock()
	defer lastHealthAlertMutex.Unlock()
	healthAlertBuffer = nil
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
		shared.WriteJSONErrorEx(w, http.StatusInternalServerError, "streaming_not_supported", "streaming not supported")
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

	// Send system status catchup so new clients know the current state immediately.
	statusEvt := eventbus.BusEvent{
		ID:          "status-" + clientID,
		Type:        eventbus.EventSystemStart,
		Timestamp:   time.Now(),
		Description: "儀表板已連線 · 系統運行中 · 排程模擬將於每個交易日 13:30 自動執行",
		Severity:    "info",
	}
	eventbus.EnrichEvent(&statusEvt)
	data, _ := json.Marshal(statusEvt)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", statusEvt.Type, data)
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

	lastPromotionMutex.RLock()
	promotionBuffered := promotionBuffer
	lastPromotionMutex.RUnlock()
	for _, b := range promotionBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastHealthAlertMutex.RLock()
	healthAlertBuffered := healthAlertBuffer
	lastHealthAlertMutex.RUnlock()
	for _, b := range healthAlertBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastRiskGateMutex.RLock()
	riskGateBuffered := riskGateBuffer
	lastRiskGateMutex.RUnlock()
	for _, b := range riskGateBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastIndustryCalendarMutex.RLock()
	industryCalendarBuffered := industryCalendarBuffer
	lastIndustryCalendarMutex.RUnlock()
	for _, b := range industryCalendarBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastBacktestCompletedMutex.RLock()
	backtestCompletedBuffered := backtestCompletedBuffer
	lastBacktestCompletedMutex.RUnlock()
	for _, b := range backtestCompletedBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastCalibrationCompletedMutex.RLock()
	calibrationCompletedBuffered := calibrationCompletedBuffer
	lastCalibrationCompletedMutex.RUnlock()
	for _, b := range calibrationCompletedBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	// Subscribe to EventBus and forward events to this client.
	sub := h.eventBus.SubscribeAll(func(ctx context.Context, event eventbus.BusEvent) error {
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
			eventbus.EnrichEvent(&event)
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
