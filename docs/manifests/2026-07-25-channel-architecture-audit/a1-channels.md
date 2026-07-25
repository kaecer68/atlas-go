# A1 Channel Audit Summary

## Canonical Count

- **37 channel IDs** are defined in `internal/apigateway/gateway.go:168-210` (canonical list for circuit breaker / health scan / summary API).
- Actual runtime registration is conditional on API keys / feature flags in `internal/apigateway/register_adapters.go:17-315`.

## Registered Channel Categories

| Category | Channels | Condition |
|----------|----------|-----------|
| TWSE official | `twse_replay`, `twse_capital_flow`, `twse_margin`, `twse_sector_index`, `twse_oddlot`, `twse_etf` | always registered |
| TAIFEX official | `taifex_daily`, `taifex_institutional` | always registered |
| Yahoo Finance | `us_yahoo`, `sox_index`, `dram_spot_price`, `us_spx`, `us_ndx`, `us_dji`, `taiex_index`, `tw_vol`, `us_nvda`, `us_aapl`, `us_msft`, `tsm_adr` | `YahooEnabled` |
| FinMind | `finmind` | `FINMIND_API_KEY` |
| Fugle | `fugle` | `FUGLE_API_KEY` |
| Fubon | `fubon` | proxy reachable |
| FX / Customs | `frankfurter_fx`, `exchange_rate`, `export_statistics` | registered |
| Geopolitical | `geopolitical`, `geopolitical_taiwan` | registered |
| TEJ | `tej` | registered |
| TSMC | `tsmc_revenue` | registered |
| BD|Day trading | `bdi`, `day_trading` | registered |
| Stubs | `tdcc_equity_dispersion`, `twse_sbl` | stub / no real fetch |
| File-backed / computed | `sector_data`, `government_flow`, `janus_regime` | file-backed or computed |

## Rogue Channels (Constitution Article 1 Violations)

| Tool | Env Var | Duplicate Of | Lines |
|------|---------|--------------|-------|
| `cmd/backfill-financial-statements` | `FINMIND_API_KEY` | `finmind` | 43, 55, 66 |
| `cmd/backfill-institutional-investors` | `FINMIND_API_KEY` | `finmind` | 51, 70, 80 |
| `cmd/backfill-month-revenue` | `FINMIND_API_KEY` | `finmind` | 55, 67, 77 |
| `cmd/backfill-taifex-oi` | `FINMIND_API_KEY` | `taifex_institutional` / `finmind` | 59, 79, 126, 129 |

All four create raw `&http.Client{}`, form FinMind URLs, and bypass `apigateway.Fetch` / `ProviderRegistry`.

## Broader Bypass Candidate

- `cmd/backfill-replay/main.go:126,242-250` uses a raw `http.Client` without an API key in the same file. If it fetches remote replay data, it is also a rogue fetcher.

## Constitution Gap

`internal/apigateway/CONSTITUTION.md:14-18` claims **16 channels** and **9 tasks**, but the codebase now has **37 canonical channel IDs** and ~**60 task registrations** (see `a2-tasks.json` and `a5-violations.json`). The document is materially outdated.
