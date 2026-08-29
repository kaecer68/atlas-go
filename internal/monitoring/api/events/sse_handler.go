package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
	clientCount atomic.Int64
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

// resetRiskGateBuffer clears the risk-gate buffer. Test-only helper.
func resetRiskGateBuffer() {
	lastRiskGateMutex.Lock()
	defer lastRiskGateMutex.Unlock()
	riskGateBuffer = nil
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

// resetBacktestCompletedBuffer clears the backtest-completed buffer. Test-only helper.
func resetBacktestCompletedBuffer() {
	lastBacktestCompletedMutex.Lock()
	defer lastBacktestCompletedMutex.Unlock()
	backtestCompletedBuffer = nil
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

type BufferedTradeSlippageEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedTradeSlippageEvents = 50

var (
	tradeSlippageBuffer    []BufferedTradeSlippageEvent
	lastTradeSlippageMutex sync.RWMutex
)

func BufferTradeSlippageEvent(event eventbus.BusEvent) {
	lastTradeSlippageMutex.Lock()
	defer lastTradeSlippageMutex.Unlock()
	tradeSlippageBuffer = append(tradeSlippageBuffer, BufferedTradeSlippageEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(tradeSlippageBuffer) > maxBufferedTradeSlippageEvents {
		tradeSlippageBuffer = tradeSlippageBuffer[len(tradeSlippageBuffer)-maxBufferedTradeSlippageEvents:]
	}
}

func GetBufferedTradeSlippageEvents() []BufferedTradeSlippageEvent {
	lastTradeSlippageMutex.RLock()
	defer lastTradeSlippageMutex.RUnlock()
	out := make([]BufferedTradeSlippageEvent, len(tradeSlippageBuffer))
	copy(out, tradeSlippageBuffer)
	return out
}

func resetTradeSlippageBuffer() {
	lastTradeSlippageMutex.Lock()
	defer lastTradeSlippageMutex.Unlock()
	tradeSlippageBuffer = nil
}

// Wave 9 YELLOW observability event buffers. Forward-compat design per
// docs/refactor-611-contract.md: slots reserved before publishers exist so
// the 5 events become operational without modifying #611 target files.

// BufferedChannelIndividualHealthEvent holds a published channel individual health event for SSE catchup.
type BufferedChannelIndividualHealthEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedChannelIndividualHealthEvents = 50

var (
	channelIndividualHealthBuffer    []BufferedChannelIndividualHealthEvent
	lastChannelIndividualHealthMutex sync.RWMutex
)

// BufferedRegimeChangeConfirmedEvent holds a published regime-change-confirmed event for SSE catchup.
type BufferedRegimeChangeConfirmedEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedRegimeChangeConfirmedEvents = 50

var (
	regimeChangeConfirmedBuffer    []BufferedRegimeChangeConfirmedEvent
	lastRegimeChangeConfirmedMutex sync.RWMutex
)

// BufferedFactorWeightRegressionEvent holds a published factor weight regression event for SSE catchup.
type BufferedFactorWeightRegressionEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedFactorWeightRegressionEvents = 50

var (
	factorWeightRegressionBuffer    []BufferedFactorWeightRegressionEvent
	lastFactorWeightRegressionMutex sync.RWMutex
)

// BufferedDriftDetectedEvent holds a published portfolio drift detected event for SSE catchup.
type BufferedDriftDetectedEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedDriftDetectedEvents = 50

var (
	driftDetectedBuffer    []BufferedDriftDetectedEvent
	lastDriftDetectedMutex sync.RWMutex
)

// BufferedIngestionLagSpikeEvent holds a published ingestion lag spike event for SSE catchup.
type BufferedIngestionLagSpikeEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedIngestionLagSpikeEvents = 50

var (
	ingestionLagSpikeBuffer    []BufferedIngestionLagSpikeEvent
	lastIngestionLagSpikeMutex sync.RWMutex
)

// BufferedAgentHealthChangeEvent holds a published agent-health-change event for SSE catchup.
type BufferedAgentHealthChangeEvent struct {
	Event      eventbus.BusEvent
	ReceivedAt time.Time
}

const maxBufferedAgentHealthChangeEvents = 50

var (
	agentHealthChangeBuffer    []BufferedAgentHealthChangeEvent
	lastAgentHealthChangeMutex sync.RWMutex
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

// GetBufferedNarrativeEvents returns a snapshot of buffered narrative events for SSE catchup.
func GetBufferedNarrativeEvents() []BufferedNarrativeEvent {
	lastNarrativeMutex.RLock()
	defer lastNarrativeMutex.RUnlock()
	out := make([]BufferedNarrativeEvent, len(narrativeBuffer))
	copy(out, narrativeBuffer)
	return out
}

// resetNarrativeBuffer clears the narrative buffer. Test-only helper.
func resetNarrativeBuffer() {
	lastNarrativeMutex.Lock()
	defer lastNarrativeMutex.Unlock()
	narrativeBuffer = nil
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

// BufferChannelIndividualHealthEvent stores a channel individual health event for catchup by new SSE clients.// BufferChannelIndividualHealthEvent stores a channel individual health event for catchup by new SSE clients.
func BufferChannelIndividualHealthEvent(event eventbus.BusEvent) {
	lastChannelIndividualHealthMutex.Lock()
	defer lastChannelIndividualHealthMutex.Unlock()
	channelIndividualHealthBuffer = append(channelIndividualHealthBuffer, BufferedChannelIndividualHealthEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(channelIndividualHealthBuffer) > maxBufferedChannelIndividualHealthEvents {
		channelIndividualHealthBuffer = channelIndividualHealthBuffer[len(channelIndividualHealthBuffer)-maxBufferedChannelIndividualHealthEvents:]
	}
}

// GetBufferedChannelIndividualHealthEvents returns a snapshot of the latest channel individual health events for SSE catchup.
func GetBufferedChannelIndividualHealthEvents() []BufferedChannelIndividualHealthEvent {
	lastChannelIndividualHealthMutex.RLock()
	defer lastChannelIndividualHealthMutex.RUnlock()
	result := make([]BufferedChannelIndividualHealthEvent, len(channelIndividualHealthBuffer))
	copy(result, channelIndividualHealthBuffer)
	return result
}

func resetChannelIndividualHealthBuffer() {
	lastChannelIndividualHealthMutex.Lock()
	defer lastChannelIndividualHealthMutex.Unlock()
	channelIndividualHealthBuffer = nil
}

// BufferRegimeChangeConfirmedEvent stores a regime-change-confirmed event for catchup by new SSE clients.
func BufferRegimeChangeConfirmedEvent(event eventbus.BusEvent) {
	lastRegimeChangeConfirmedMutex.Lock()
	defer lastRegimeChangeConfirmedMutex.Unlock()
	regimeChangeConfirmedBuffer = append(regimeChangeConfirmedBuffer, BufferedRegimeChangeConfirmedEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(regimeChangeConfirmedBuffer) > maxBufferedRegimeChangeConfirmedEvents {
		regimeChangeConfirmedBuffer = regimeChangeConfirmedBuffer[len(regimeChangeConfirmedBuffer)-maxBufferedRegimeChangeConfirmedEvents:]
	}
}

// GetBufferedRegimeChangeConfirmedEvents returns a snapshot of the latest regime-change-confirmed events for SSE catchup.
func GetBufferedRegimeChangeConfirmedEvents() []BufferedRegimeChangeConfirmedEvent {
	lastRegimeChangeConfirmedMutex.RLock()
	defer lastRegimeChangeConfirmedMutex.RUnlock()
	result := make([]BufferedRegimeChangeConfirmedEvent, len(regimeChangeConfirmedBuffer))
	copy(result, regimeChangeConfirmedBuffer)
	return result
}

func resetRegimeChangeConfirmedBuffer() {
	lastRegimeChangeConfirmedMutex.Lock()
	defer lastRegimeChangeConfirmedMutex.Unlock()
	regimeChangeConfirmedBuffer = nil
}

// BufferFactorWeightRegressionEvent stores a factor-weight-regression event for catchup by new SSE clients.
func BufferFactorWeightRegressionEvent(event eventbus.BusEvent) {
	lastFactorWeightRegressionMutex.Lock()
	defer lastFactorWeightRegressionMutex.Unlock()
	factorWeightRegressionBuffer = append(factorWeightRegressionBuffer, BufferedFactorWeightRegressionEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(factorWeightRegressionBuffer) > maxBufferedFactorWeightRegressionEvents {
		factorWeightRegressionBuffer = factorWeightRegressionBuffer[len(factorWeightRegressionBuffer)-maxBufferedFactorWeightRegressionEvents:]
	}
}

// GetBufferedFactorWeightRegressionEvents returns a snapshot of the latest factor-weight-regression events for SSE catchup.
func GetBufferedFactorWeightRegressionEvents() []BufferedFactorWeightRegressionEvent {
	lastFactorWeightRegressionMutex.RLock()
	defer lastFactorWeightRegressionMutex.RUnlock()
	result := make([]BufferedFactorWeightRegressionEvent, len(factorWeightRegressionBuffer))
	copy(result, factorWeightRegressionBuffer)
	return result
}

func resetFactorWeightRegressionBuffer() {
	lastFactorWeightRegressionMutex.Lock()
	defer lastFactorWeightRegressionMutex.Unlock()
	factorWeightRegressionBuffer = nil
}

// BufferDriftDetectedEvent stores a portfolio-drift-detected event for catchup by new SSE clients.
func BufferDriftDetectedEvent(event eventbus.BusEvent) {
	lastDriftDetectedMutex.Lock()
	defer lastDriftDetectedMutex.Unlock()
	driftDetectedBuffer = append(driftDetectedBuffer, BufferedDriftDetectedEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(driftDetectedBuffer) > maxBufferedDriftDetectedEvents {
		driftDetectedBuffer = driftDetectedBuffer[len(driftDetectedBuffer)-maxBufferedDriftDetectedEvents:]
	}
}

// GetBufferedDriftDetectedEvents returns a snapshot of the latest portfolio-drift-detected events for SSE catchup.
func GetBufferedDriftDetectedEvents() []BufferedDriftDetectedEvent {
	lastDriftDetectedMutex.RLock()
	defer lastDriftDetectedMutex.RUnlock()
	result := make([]BufferedDriftDetectedEvent, len(driftDetectedBuffer))
	copy(result, driftDetectedBuffer)
	return result
}

func resetDriftDetectedBuffer() {
	lastDriftDetectedMutex.Lock()
	defer lastDriftDetectedMutex.Unlock()
	driftDetectedBuffer = nil
}

// BufferIngestionLagSpikeEvent stores an ingestion-lag-spike event for catchup by new SSE clients.
func BufferIngestionLagSpikeEvent(event eventbus.BusEvent) {
	lastIngestionLagSpikeMutex.Lock()
	defer lastIngestionLagSpikeMutex.Unlock()
	ingestionLagSpikeBuffer = append(ingestionLagSpikeBuffer, BufferedIngestionLagSpikeEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(ingestionLagSpikeBuffer) > maxBufferedIngestionLagSpikeEvents {
		ingestionLagSpikeBuffer = ingestionLagSpikeBuffer[len(ingestionLagSpikeBuffer)-maxBufferedIngestionLagSpikeEvents:]
	}
}

// GetBufferedIngestionLagSpikeEvents returns a snapshot of the latest ingestion-lag-spike events for SSE catchup.
func GetBufferedIngestionLagSpikeEvents() []BufferedIngestionLagSpikeEvent {
	lastIngestionLagSpikeMutex.RLock()
	defer lastIngestionLagSpikeMutex.RUnlock()
	result := make([]BufferedIngestionLagSpikeEvent, len(ingestionLagSpikeBuffer))
	copy(result, ingestionLagSpikeBuffer)
	return result
}

func resetIngestionLagSpikeBuffer() {
	lastIngestionLagSpikeMutex.Lock()
	defer lastIngestionLagSpikeMutex.Unlock()
	ingestionLagSpikeBuffer = nil
}

// BufferAgentHealthChangeEvent stores an agent-health-change event for catchup by new SSE clients.
func BufferAgentHealthChangeEvent(event eventbus.BusEvent) {
	lastAgentHealthChangeMutex.Lock()
	defer lastAgentHealthChangeMutex.Unlock()
	agentHealthChangeBuffer = append(agentHealthChangeBuffer, BufferedAgentHealthChangeEvent{
		Event:      event,
		ReceivedAt: time.Now(),
	})
	if len(agentHealthChangeBuffer) > maxBufferedAgentHealthChangeEvents {
		agentHealthChangeBuffer = agentHealthChangeBuffer[len(agentHealthChangeBuffer)-maxBufferedAgentHealthChangeEvents:]
	}
}

// GetBufferedAgentHealthChangeEvents returns a snapshot of buffered agent-health-change events for SSE catchup.
func GetBufferedAgentHealthChangeEvents() []BufferedAgentHealthChangeEvent {
	lastAgentHealthChangeMutex.RLock()
	defer lastAgentHealthChangeMutex.RUnlock()
	out := make([]BufferedAgentHealthChangeEvent, len(agentHealthChangeBuffer))
	copy(out, agentHealthChangeBuffer)
	return out
}

// resetAgentHealthChangeBuffer clears the agent-health-change buffer. Test-only helper.
func resetAgentHealthChangeBuffer() {
	lastAgentHealthChangeMutex.Lock()
	defer lastAgentHealthChangeMutex.Unlock()
	agentHealthChangeBuffer = nil
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
	count := h.clientCount.Load()
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
	h.clientCount.Add(1)

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

	lastTradeSlippageMutex.RLock()
	tradeSlippageBuffered := tradeSlippageBuffer
	lastTradeSlippageMutex.RUnlock()
	for _, b := range tradeSlippageBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastChannelIndividualHealthMutex.RLock()
	channelIndividualHealthBuffered := channelIndividualHealthBuffer
	lastChannelIndividualHealthMutex.RUnlock()
	for _, b := range channelIndividualHealthBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastRegimeChangeConfirmedMutex.RLock()
	regimeChangeConfirmedBuffered := regimeChangeConfirmedBuffer
	lastRegimeChangeConfirmedMutex.RUnlock()
	for _, b := range regimeChangeConfirmedBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastFactorWeightRegressionMutex.RLock()
	factorWeightRegressionBuffered := factorWeightRegressionBuffer
	lastFactorWeightRegressionMutex.RUnlock()
	for _, b := range factorWeightRegressionBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastDriftDetectedMutex.RLock()
	driftDetectedBuffered := driftDetectedBuffer
	lastDriftDetectedMutex.RUnlock()
	for _, b := range driftDetectedBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastIngestionLagSpikeMutex.RLock()
	ingestionLagSpikeBuffered := ingestionLagSpikeBuffer
	lastIngestionLagSpikeMutex.RUnlock()
	for _, b := range ingestionLagSpikeBuffered {
		data, err := json.Marshal(b.Event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", b.Event.Type, data)
		flusher.Flush()
	}

	lastAgentHealthChangeMutex.RLock()
	agentHealthChangeBuffered := agentHealthChangeBuffer
	lastAgentHealthChangeMutex.RUnlock()
	for _, b := range agentHealthChangeBuffered {
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
			h.clientCount.Add(-1)
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
	return slices.Contains(client.types, eventType)
}
