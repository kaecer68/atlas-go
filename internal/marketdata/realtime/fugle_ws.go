package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kaecer68/atlas-go/internal/domain"
)

const (
	defaultWSURL       = "wss://api.fugle.tw/marketdata/v1.0/stock/streaming"
	initialBackoff     = 1 * time.Second
	maxBackoff         = 60 * time.Second
	backoffMultiplier  = 2.0
	writeWait          = 10 * time.Second
	pongWait           = 60 * time.Second
	pingPeriod         = (pongWait * 9) / 10
	subscribeQueueSize = 256
)

type fugleWSAuth struct {
	Event string `json:"event"`
	Data  struct {
		APIKey string `json:"apikey"`
	} `json:"data"`
}

type fugleWSSubscribe struct {
	Event string `json:"event"`
	Data  struct {
		Channel string `json:"channel"`
		Symbol  string `json:"symbol"`
	} `json:"data"`
}

type fugleWSUnsubscribe struct {
	Event string `json:"event"`
	Data  struct {
		Channel string `json:"channel"`
		Symbol  string `json:"symbol"`
	} `json:"data"`
}

type fugleWSTradeData struct {
	Symbol  string  `json:"symbol"`
	Price   float64 `json:"price"`
	Volume  int64   `json:"volume"`
	Open    float64 `json:"open"`
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Close   float64 `json:"close"`
	IsTrial bool    `json:"isTrial"`
	IsOdd   bool    `json:"isOdd"`
}

type fugleWSMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type FugleWSConfig struct {
	APIKey     string
	WSURL      string
	EnablePing bool
}

func DefaultFugleWSConfig(apiKey string) FugleWSConfig {
	return FugleWSConfig{
		APIKey:     apiKey,
		WSURL:      defaultWSURL,
		EnablePing: true,
	}
}

type FugleWebSocketProvider struct {
	config    FugleWSConfig
	conn      *websocket.Conn
	connMu    sync.Mutex
	state     ProviderState
	stateMu   sync.RWMutex
	subs      map[string]struct{}
	subsMu    sync.RWMutex
	callbacks []QuoteCallback
	cbMu      sync.RWMutex

	reconnectCount int
	lastErr        error

	backoff    time.Duration
	cancelCtx  context.Context
	cancelFunc context.CancelFunc
	running    bool
	runningMu  sync.Mutex
	subCh      chan fugleWSSubscribe
	unsubCh    chan fugleWSUnsubscribe
}

func NewFugleWebSocketProvider(config FugleWSConfig) *FugleWebSocketProvider {
	return &FugleWebSocketProvider{
		config:  config,
		state:   ProviderStateDisconnected,
		subs:    make(map[string]struct{}),
		backoff: initialBackoff,
		subCh:   make(chan fugleWSSubscribe, subscribeQueueSize),
		unsubCh: make(chan fugleWSUnsubscribe, subscribeQueueSize),
	}
}

func (p *FugleWebSocketProvider) Connect(ctx context.Context) error {
	p.runningMu.Lock()
	if p.running {
		p.runningMu.Unlock()
		return nil
	}
	p.running = true
	p.runningMu.Unlock()

	p.setState(ProviderStateConnecting)

	connCtx, cancel := context.WithCancel(ctx)
	p.connMu.Lock()
	p.cancelCtx = connCtx
	p.cancelFunc = cancel
	p.connMu.Unlock()

	if err := p.dial(); err != nil {
		p.setState(ProviderStateFailed)
		p.lastErr = err
		return fmt.Errorf("fugle-ws connect: %w", err)
	}

	p.setState(ProviderStateConnected)
	p.backoff = initialBackoff

	go p.readLoop()
	if p.config.EnablePing {
		go p.pingLoop()
	}
	go p.subscriptionLoop()

	return nil
}

func (p *FugleWebSocketProvider) Disconnect(ctx context.Context) error {
	p.runningMu.Lock()
	p.running = false
	p.runningMu.Unlock()

	p.connMu.Lock()
	if p.cancelFunc != nil {
		p.cancelFunc()
	}
	if p.conn != nil {
		p.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(writeWait),
		)
		p.conn.Close()
		p.conn = nil
	}
	p.connMu.Unlock()

	p.setState(ProviderStateDisconnected)
	return nil
}

func (p *FugleWebSocketProvider) Subscribe(symbols []string) error {
	if !p.IsConnected() {
		return fmt.Errorf("fugle-ws: not connected")
	}
	for _, sym := range symbols {
		p.subsMu.Lock()
		p.subs[sym] = struct{}{}
		p.subsMu.Unlock()

		msg := fugleWSSubscribe{
			Event: "subscribe",
		}
		msg.Data.Channel = "trades"
		msg.Data.Symbol = sym
		p.subCh <- msg
	}
	return nil
}

func (p *FugleWebSocketProvider) Unsubscribe(symbols []string) error {
	if !p.IsConnected() {
		return fmt.Errorf("fugle-ws: not connected")
	}
	for _, sym := range symbols {
		p.subsMu.Lock()
		delete(p.subs, sym)
		p.subsMu.Unlock()

		msg := fugleWSUnsubscribe{
			Event: "unsubscribe",
		}
		msg.Data.Channel = "trades"
		msg.Data.Symbol = sym
		p.unsubCh <- msg
	}
	return nil
}

func (p *FugleWebSocketProvider) OnQuote(callback QuoteCallback) {
	p.cbMu.Lock()
	p.callbacks = append(p.callbacks, callback)
	p.cbMu.Unlock()
}

func (p *FugleWebSocketProvider) IsConnected() bool {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.state == ProviderStateConnected
}

func (p *FugleWebSocketProvider) Name() string {
	return "fugle-ws"
}

func (p *FugleWebSocketProvider) Status() ProviderStatus {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	p.subsMu.RLock()
	defer p.subsMu.RUnlock()

	lastErrStr := ""
	if p.lastErr != nil {
		lastErrStr = p.lastErr.Error()
	}
	return ProviderStatus{
		Name:            p.Name(),
		State:           p.state,
		SubscribedCount: len(p.subs),
		ReconnectCount:  p.reconnectCount,
		LastError:       lastErrStr,
	}
}

func (p *FugleWebSocketProvider) dial() error {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(p.config.WSURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", p.config.WSURL, err)
	}

	auth := fugleWSAuth{Event: "auth"}
	auth.Data.APIKey = p.config.APIKey
	if err := conn.WriteJSON(auth); err != nil {
		conn.Close()
		return fmt.Errorf("auth write: %w", err)
	}

	p.conn = conn
	return nil
}

func (p *FugleWebSocketProvider) readLoop() {
	for {
		p.connMu.Lock()
		conn := p.conn
		ctx := p.cancelCtx
		p.connMu.Unlock()

		if conn == nil || ctx == nil {
			return
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			p.handleReadError(err)
			continue
		}

		var msg fugleWSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		p.handleMessage(msg)
	}
}

func (p *FugleWebSocketProvider) handleMessage(msg fugleWSMessage) {
	switch msg.Event {
	case "trade":
		var trade fugleWSTradeData
		if err := json.Unmarshal(msg.Data, &trade); err != nil {
			return
		}
		quote := p.tradeToQuote(trade)
		p.emitQuote(quote)
	case "snapshot":
		var trade fugleWSTradeData
		if err := json.Unmarshal(msg.Data, &trade); err != nil {
			return
		}
		quote := p.tradeToQuote(trade)
		p.emitQuote(quote)
	}
}

func (p *FugleWebSocketProvider) tradeToQuote(trade fugleWSTradeData) domain.Quote {
	return domain.Quote{
		Symbol:     trade.Symbol,
		Last:       trade.Price,
		Open:       trade.Open,
		High:       trade.High,
		Low:        trade.Low,
		Volume:     trade.Volume,
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: !trade.IsTrial,
		Source:     "fugle-ws",
	}
}

func (p *FugleWebSocketProvider) emitQuote(quote domain.Quote) {
	p.cbMu.RLock()
	callbacks := p.callbacks
	p.cbMu.RUnlock()

	for _, cb := range callbacks {
		cb(quote)
	}
}

func (p *FugleWebSocketProvider) handleReadError(err error) {
	p.runningMu.Lock()
	isRunning := p.running
	p.runningMu.Unlock()

	if !isRunning {
		return
	}

	p.lastErr = err
	p.setState(ProviderStateReconnecting)
	p.reconnectCount++

	p.reconnectWithBackoff()
}

func (p *FugleWebSocketProvider) reconnectWithBackoff() {
	for {
		p.connMu.Lock()
		ctx := p.cancelCtx
		p.connMu.Unlock()

		select {
		case <-ctx.Done():
			p.setState(ProviderStateDisconnected)
			return
		default:
		}

		time.Sleep(p.backoff)

		if err := p.dial(); err != nil {
			p.lastErr = err
			p.backoff = min(time.Duration(float64(p.backoff)*backoffMultiplier), maxBackoff)
			continue
		}

		p.setState(ProviderStateConnected)
		p.backoff = initialBackoff

		p.resubscribe()
		return
	}
}

func (p *FugleWebSocketProvider) resubscribe() {
	p.subsMu.RLock()
	symbols := make([]string, 0, len(p.subs))
	for sym := range p.subs {
		symbols = append(symbols, sym)
	}
	p.subsMu.RUnlock()

	for _, sym := range symbols {
		msg := fugleWSSubscribe{
			Event: "subscribe",
		}
		msg.Data.Channel = "trades"
		msg.Data.Symbol = sym
		p.subCh <- msg
	}
}

func (p *FugleWebSocketProvider) pingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.connMu.Lock()
			conn := p.conn
			p.connMu.Unlock()

			if conn == nil {
				continue
			}

			if err := conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			); err != nil {
				return
			}
		case <-p.cancelCtx.Done():
			return
		}
	}
}

func (p *FugleWebSocketProvider) subscriptionLoop() {
	for {
		select {
		case msg := <-p.subCh:
			p.connMu.Lock()
			conn := p.conn
			p.connMu.Unlock()

			if conn != nil {
				conn.WriteJSON(msg)
			}
		case msg := <-p.unsubCh:
			p.connMu.Lock()
			conn := p.conn
			p.connMu.Unlock()

			if conn != nil {
				conn.WriteJSON(msg)
			}
		case <-p.cancelCtx.Done():
			return
		}
	}
}

func (p *FugleWebSocketProvider) setState(state ProviderState) {
	p.stateMu.Lock()
	p.state = state
	p.stateMu.Unlock()
}
