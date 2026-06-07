# ODM Data Provider

Fetch monthly revenue data for Taiwan ODM manufacturers (2317 Foxconn, 2382 Quanta, 6669 Wiwynn) via FinMind. Compute YoY growth rates and expose ODM revenue points to the industry channel model.

## Data Sources
- Primary: FinMind TaiwanStockMonthRevenue dataset
- Symbols: 2317 (Foxconn/Hon Hai), 2382 (Quanta), 6669 (Wiwynn)

## Key Outputs
- `ODMRevenuePoint`: Symbol, Revenue (NTD thousands), YoYPct, Timestamp
- `FetchODMRevenue(ctx, symbol)`: Single ODM revenue with YoY computation
- `FetchAllODMRevenue(ctx)`: All three ODMs, partial failures logged via Warn

## Operational Notes
1. Rate limit: 600 requests/hour (FinMind shared client)
2. YoY requires prior year same-month revenue; zero prior → YoY = 0
3. Missing data (no FinMind record) → propagated as error with context
4. Do not extrapolate or fabricate revenue data; return error on missing
5. Cache FinMind client via GetSharedFinMindClient singleton — never create new HTTP clients
