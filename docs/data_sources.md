# Data Sources

## Recommendation

For `2026-03-29`, the recommended source hierarchy is:

1. TWSE and TPEX official datasets for baseline market structure and historical replay
2. Fugle for near-real-time quotes and websocket streaming
3. Yahoo data only as a fallback or cross-check source

## Why Not Yahoo as Primary

- no stable public developer-first official trading API
- ecosystem often relies on unofficial scraping
- field stability and uptime are harder to guarantee
- weak fit for repeatable simulation and audit requirements

## Provider Roles

### TWSE

Use for:

- daily market reports
- listed stock daily data
- market breadth
- institutional and ownership related datasets where available

### TPEX

Use for:

- OTC stock daily data
- supplementary market structure for Taiwan equities

### Fugle

Use for:

- near-real-time quote snapshots
- websocket streaming
- intraday simulation expansion

### Yahoo

Use for:

- sanity checks
- enrichment of public-facing quote views
- backup display data only

## MVP Provider Strategy

### Phase 1

- build replay engine on daily data
- implement TWSE and TPEX adapters first

### Phase 2

- add Fugle snapshot support
- support delayed or near-real-time paper-trading views

### Phase 3

- add Fugle websocket event loop
- run event-driven intraday simulations

