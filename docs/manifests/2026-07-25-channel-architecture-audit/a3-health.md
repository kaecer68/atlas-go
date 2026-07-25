# A3 Health Monitoring Audit Summary

## Duplicate / Conflicting Health Endpoints

| Endpoint | What it returns | Conflict |
|----------|-----------------|----------|
| `GET /health` | `status=ok` + `atlas_http`/`fubon_proxy` port occupancy | Canonical process liveness |
| `GET /api/health` | Hardcoded `{"status":"ok"}` | Looks like the same endpoint but performs no probes |
| `GET /api/health/aggregate` | Four-tier aggregate (port, channel_health file mtime, LLM key, auth) | Partial overlap with `/api/dashboard/system-health` but different channel list |
| `GET /api/dashboard/system-health` | Hand-coded list of ~20 channels + health-store override | Channel list may diverge from canonical 37 IDs |
| `GET /api/dashboard/channel-health` | Reads `data/state/channel_health.json`; returns only id/status/updated_at | Drops `last_error`, latency, rate-limit from `ChannelHealthRecord` |

**Verdict**: There are at least **two duplicate health endpoints** (`/health` vs `/api/health`) and **two overlapping channel-health views** (`/api/health/aggregate` vs `/api/dashboard/system-health` vs `/api/dashboard/channel-health`) that are not guaranteed to agree.

## Real Per-Fetch Health Store

The authoritative per-channel fetch outcome lives in `internal/apigateway/channel_health.go`:

- Records: `status`, `last_fetch`, `data`, `error`, `success`, `rate_limit`, `latency`, `records`, `symbols`, `errors`
- Storage: JSON file, optional DB, fetch log
- Any `Gateway.Fetch` writes a record

## Wave9 Synthesizer Gap

`internal/monitoring/wave9_runtime.go` emits `EventChannelIndividualHealth` only when `status != ok/inactive` **and** `LastError != ""`. Therefore, a channel that becomes `warn`/`error` without a textual `LastError` will **not** trigger a Wave9 event.

## Prometheus / Grafana Gap

- Prometheus only scrapes `up{job="atlas-go"}` for process health.
- The only per-channel Prometheus rule is `monitoring/rules/wave9_channel_individual_health.yml` using `atlas_channel_health_errors_total{channel}`.
- Grafana System Health dashboard does **not** display per-channel health/error/latency panels.

## Unmonitored Channels

Stub channels (`tdcc_equity_dispersion`, `twse_sbl`) and file-backed channels (`sector_data`, `government_flow`) have no real fetch health signal.
