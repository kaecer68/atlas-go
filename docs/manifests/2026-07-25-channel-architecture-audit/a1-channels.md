# A1 Channel Audit Summary

## Canonical Count

- **40 channel IDs** are defined in `internal/apigateway/gateway.go` (`channelIDs`) (canonical list for circuit breaker / health scan / summary API).
- Actual runtime registration is conditional on API keys / feature flags in `internal/apigateway/register_adapters.go:17-315`.

## Registered Channel Categories

| Category | Channels | Condition |
|----------|----------|-----------|
| TWSE official | `twse_replay`, `twse_capital_flow`, `twse_margin`, `twse_sector_index`, `twse_oddlot`, `twse_etf`, `twse_insider`, `market_volume` | always registered |
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
| File-backed / computed | `sector_data`, `government_flow`, `government_broker`, `janus_regime` | file-backed or computed |

## Rogue Channels (Constitution Article 1 Violations)

**RESOLVED 2026-07-25.** The four FinMind backfill CLIs and `backfill-replay` that previously created raw `http.Client` instances were removed in PR #1338. `scripts/ci/check_constitution.sh` now blocks any new direct `os.Getenv("XXX_API_KEY")` + raw HTTP client violations in CI.

## Constitution Baseline

`internal/apigateway/CONSTITUTION.md` was revised to **v1.3** on 2026-07-25 and states **40 channels**. The `a1-channels.json` / `a2-tasks.json` artifacts in this manifest are the source of truth for the canonical channel list and task registry. `scripts/ci/check_channel_index.py` (wired to CI) verifies that the JSON channel count and IDs stay synchronized with `internal/apigateway/gateway.go`. If a developer adds a runtime channel registration without updating the canonical list or this manifest, CI will fail.
