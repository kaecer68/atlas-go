# Phase 4: Near-Real-Time Paper Trading Architecture

## Overview

This document outlines the architecture for integrating Fugle real-time market data and transitioning from replay-based simulation to near-real-time paper trading.

## Goals

1. **Real-time Market Data**: Integrate Fugle API for live snapshots and intraday data
2. **Live State Store**: Maintain current portfolio, positions, and market state
3. **Event-Driven Orchestration**: React to market events in real-time
4. **Monitoring & Alerting**: Track system health and trading performance

## Architecture Components

```
┌─────────────────────────────────────────────────────────────────┐
│                     Fugle Data Source                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ Snapshot API │  │  WebSocket  │  │   Historical API       │  │
│  │  (REST)      │  │ (Streaming) │  │   (Daily/Intraday)    │  │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘  │
└─────────┼────────────────┼─────────────────────┼────────────────┘
          │                │                     │
          ▼                ▼                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Data Ingestion Layer                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ Polling     │  │ WebSocket   │  │   Rate Limiter          │  │
│  │ Adapter     │  │  Adapter    │  │   (Fugle: 50 req/min)   │  │
│  └──────┬──────┘  └──────┬──────┘  └─────────────────────────┘  │
└─────────┼────────────────┼─────────────────────────────────────┘
          │                │
          ▼                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Event Bus (Pub/Sub)                           │
│  Topics:                                                         │
│  - market.snapshot.{symbol}    - market.tick.{symbol}            │
│  - market.regime.change        - portfolio.position.update         │
│  - agent.recommendation        - order.placed / order.filled     │
└─────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Live State Store                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ Portfolio   │  │  Positions  │  │   Market Regime          │  │
│  │ State       │  │   State     │  │   State                  │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│                                                                  │
│  Persistence: JSONL files in data/live/                          │
│  - portfolio_state.jsonl         - positions_{date}.jsonl      │
│  - regime_state.jsonl            - events_{date}.jsonl         │
└─────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────┐
│               Event-Driven Orchestrator                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ Market Open │  │ Intraday    │  │   Market Close           │  │
│  │ Handler     │  │ Handler     │  │   Handler                │  │
│  │ (09:00)     │  │ (Periodic)  │  │   (13:30)                │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│                                                                  │
│  Triggers:                                                       │
│  - Pre-market: Load positions, check gaps                         │
│  - On-tick: Update prices, check stop-losses                      │
│  - Periodic: Run agent evaluation, generate recommendations       │
│  - On-signal: Execute orders (paper trading)                      │
└─────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Paper Trading                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ Order       │  │ Position    │  │   PnL Calculation        │  │
│  │ Simulator   │  │  Tracker    │  │   (Real-time)            │  │
│  │ (No exec)   │  │             │  │                          │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│                                                                  │
│  Note: Fugle paper trading available for funded accounts         │
│  Fallback: Internal order simulation with slippage model         │
└─────────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Market Data Ingestion

**Fugle API Integration:**
- **REST API**: Snapshot quotes (polling every 5-30 seconds)
- **WebSocket**: Real-time ticks (for high-frequency monitoring)
- **Rate Limiting**: 50 requests/minute for free tier

```go
// FugleProvider implements real-time data fetching
type FugleProvider struct {
    apiKey      string
    httpClient  *http.Client
    wsClient    *websocket.Conn
    rateLimiter *rate.Limiter
}

func (p *FugleProvider) GetQuotes(ctx context.Context, symbols []string) ([]domain.Quote, error)
func (p *FugleProvider) SubscribeTicks(ctx context.Context, symbols []string) (<-chan domain.Tick, error)
```

### 2. Event Bus

**Event Types:**

```go
type EventType string

const (
    EventMarketSnapshot   EventType = "market.snapshot"
    EventMarketTick       EventType = "market.tick"
    EventRegimeChange     EventType = "market.regime.change"
    EventPositionUpdate   EventType = "portfolio.position.update"
    EventRecommendation   EventType = "agent.recommendation"
    EventOrderPlaced      EventType = "order.placed"
    EventOrderFilled      EventType = "order.filled"
    EventMarketOpen       EventType = "market.open"
    EventMarketClose      EventType = "market.close"
)

type Event struct {
    ID        string
    Type      EventType
    Timestamp time.Time
    Payload   interface{}
}
```

### 3. Live State Store

**Storage Structure:**

```
data/live/
├── state/
│   ├── portfolio_state.jsonl      # Current portfolio (cash, exposure)
│   ├── positions_current.jsonl    # Open positions
│   └── regime_state.jsonl         # Current market regime
├── events/
│   └── events_2026-04-05.jsonl    # Daily event log
├── quotes/
│   └── quotes_2026-04-05.jsonl    # Intraday quote snapshots
└── orders/
    └── orders_2026-04-05.jsonl    # Paper trading orders
```

**State Types:**

```go
type LivePortfolioState struct {
    Cash              float64
    TotalExposure     float64
    AvailableCash     float64
    DayPnL            float64
    UnrealizedPnL     float64
    LastUpdated       time.Time
}

type LivePosition struct {
    Symbol            string
    Quantity          int
    AverageCost       float64
    CurrentPrice      float64
    MarketValue       float64
    UnrealizedPnL     float64
    DayOpenPnL        float64
    EntryTime         time.Time
    StopLossPrice     float64
    TakeProfitPrice   float64
}

type LiveRegimeState struct {
    CurrentRegime     domain.Regime
    Confidence        float64
    LastChangedAt     time.Time
    DeterminedBy      string  // Agent or manual
}
```

### 4. Orchestration Handlers

**Market Open Handler (09:00 TST):**
1. Load overnight positions
2. Fetch pre-market snapshots
3. Check for significant gaps
4. Run Context layer agents for regime assessment
5. Generate daily trading plan

**Intraday Handler (Periodic, e.g., every 5 min):**
1. Update position prices with latest quotes
2. Check stop-loss and take-profit triggers
3. Run Style/Sector agents for new signals
4. Apply CRO risk filters
5. Generate recommendations
6. Simulate order execution (paper trading)

**Market Close Handler (13:30 TST):**
1. Mark all positions to market
2. Calculate daily PnL
3. Update ledger
4. Run post-trade analysis
5. Archive day's data

## Implementation Plan

### Phase 4A: Fugle Integration (Week 1)

- [ ] Implement Fugle REST API client
- [ ] Add rate limiting and error handling
- [ ] Create WebSocket adapter for ticks
- [ ] Add configuration for Fugle API key
- [ ] Implement quote caching to reduce API calls

### Phase 4B: Live State Store (Week 2)

- [ ] Create LiveStateStore interface and JSONL implementation
- [ ] Implement PortfolioState manager
- [ ] Implement PositionTracker with price updates
- [ ] Implement RegimeState manager
- [ ] Add state persistence and recovery

### Phase 4C: Event Bus (Week 2-3)

- [ ] Create EventBus interface (channel-based)
- [ ] Implement event publishing
- [ ] Implement event subscription with filters
- [ ] Add event persistence for audit trail

### Phase 4D: Orchestration (Week 3-4)

- [ ] Implement MarketOpen handler
- [ ] Implement Intraday handler (configurable interval)
- [ ] Implement MarketClose handler
- [ ] Create paper trading order simulator
- [ ] Add stop-loss and take-profit monitoring

### Phase 4E: Monitoring (Week 4)

- [ ] Add health check endpoints
- [ ] Create status dashboard (CLI or web)
- [ ] Implement alerting for critical events
- [ ] Add performance metrics collection

## Configuration

```yaml
# configs/live.yaml
live_trading:
  enabled: true
  mode: paper  # paper | production
  
  market_data:
    provider: fugle
    poll_interval: 30s
    symbols:
      - "2330.TW"
      - "2317.TW"
      - "0050.TW"
    # Fugle API credentials (via env var)
    api_key: ${FUGLE_API_KEY}
    
  orchestration:
    market_open_time: "09:00"
    market_close_time: "13:30"
    intraday_interval: 5m
    pre_market_check: true
    
  risk_management:
    max_daily_loss_pct: 2.0
    max_position_loss_pct: 5.0
    stop_loss_enabled: true
    take_profit_enabled: true
    
  state_store:
    base_path: "data/live"
    persistence_interval: 30s
```

## Risk Considerations

1. **API Rate Limits**: Implement backoff and caching
2. **Data Quality**: Validate quotes for stale data
3. **State Consistency**: Ensure atomic state updates
4. **Error Recovery**: Handle API failures gracefully
5. **Paper vs Production**: Clear separation with safety checks

## Success Metrics

- Data latency < 30 seconds
- 99.9% uptime during market hours
- Zero state corruption events
- Accurate PnL tracking vs theoretical

## Migration Path

1. **Current**: Replay-based backtesting (Phase 3) ✅
2. **Next**: Live data with replay-style execution (Phase 4A-B)
3. **Then**: Full intraday paper trading (Phase 4C-E)
4. **Future**: Production trading (Phase 5)
