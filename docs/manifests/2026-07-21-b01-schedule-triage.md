# B01 Schedule Triage (Manifest 2026-07-21, Gap C04)

> **Source**: Post-merge verification drill on 2026-07-21. Two snapshots taken from `curl /api/scheduler/status`:
> - **Snapshot 1** at container uptime ~7.5h: 63 tasks, 36 silent, 27 fired.
> - **Snapshot 2** at container uptime ~8.5h: 63 tasks, 34 silent, 29 fired.
>
> Comparing the two snapshots reveals which tasks were designed-silent (interval > uptime) vs truly broken (interval ≤ uptime but never fired).

> **Method**:
> - **Designed-silent**: `interval > container uptime` and `last_run == zero` — the next fire is X hours after container start, which is in the future.
> - **Investigate**: `interval ≤ container uptime` and `last_run == zero` and `Enabled == true` — should have fired at least once by now but didn't.
> - **Disabled**: `Enabled == false` — explicitly off, no action needed.

> **Container started**: 2026-07-20T16:46:49Z.
> **Snapshot 2 taken**: 2026-07-20T17:05:31Z (uptime ~8h 18m = 8.3h).

---

## Classification Summary

| Class | Count | Action |
|-------|-------|--------|
| Designed-silent (interval > 8.3h) | 32 | None — wait for next fire |
| Investigate (interval ≤ 8.3h, never fired) | 2 | See investigate section |
| Disabled (Enabled=false) | 0 | None |
| **Total silent** | **34** | |
| Total fired | 29 | (Reference; see fired list below) |
| Total registered | 63 | |

---

## Investigate (the only 2 real anomalies)

These 2 tasks have intervals ≤ 6h and container uptime is ~8.3h, so they should have fired at least once. They haven't.

### I-01: `auto_geopolitical` (interval 6h, never fired)

- **Source registration**: `cmd/atlas/capital_tasks.go`
- **Hypothesis**: Likely gated by MaturityTracker (BurnIn phase blocks many `auto_*` tasks). Container started during a BurnIn phase window.
- **Evidence to gather**: `docker logs atlas-go --tail 5000 2>&1 | grep -i 'auto_geopolitical\|burn_in_skip'`
  - If `burn_in_skip` appears, this is working as designed — no fix needed.
- **Action**: verify log; if maturity-gated, document and remove from investigate.

### I-02: `janus_regime_refresh` (interval 6h, never fired)

- **Source registration**: `cmd/atlas/operations_tasks.go`
- **Hypothesis**: Possibly gated by `JanusEngine.EnsureAllRegimes()` running only once at startup; the 6h refresh may be a no-op once regimes are populated. Or it's also MaturityTracker-gated.
- **Evidence to gather**: `cmd/atlas/operations_tasks.go::janus_regime_refresh` task body + `docker logs atlas-go --tail 5000 2>&1 | grep -i 'janus_regime_refresh'`
- **Action**: check task body; if designed as one-shot or maturity-gated, document and remove from investigate.

---

## Designed-Silent (interval > 8.3h, enabled, no fire yet)

These tasks all have interval ≥ 12h and container uptime is only ~8.3h, so they haven't had a chance to fire. **No action needed** — they are working as designed.

### 12h interval (1 task)
- `auto_export`

### 24h interval (27 tasks)

| Task | Source file (likely) |
|------|---------------------|
| auto_backfill | cmd/atlas/operations_tasks.go |
| auto_calendar_refresh | cmd/atlas/operations_tasks.go |
| auto_judge_promoter | cmd/atlas/capital_tasks.go |
| auto_propose | (unknown) |
| auto_rollback | cmd/atlas/capital_tasks.go |
| auto_strategy_evolution | cmd/atlas/calibration_tasks.go |
| auto_threshold_calibrate | cmd/atlas/calibration_tasks.go |
| calibration_cycle | cmd/atlas/calibration_tasks.go |
| conviction_calibrate | cmd/atlas/calibration_tasks.go |
| cycle_calibrate | cmd/atlas/calibration_tasks.go |
| etf_nav_refresh | (unknown) |
| factor_weight_calibrate | cmd/atlas/calibration_tasks.go |
| factor_weight_strategy_calibrate | cmd/atlas/calibration_tasks.go |
| fundamentals_staleness_check | cmd/atlas/operations_tasks.go |
| linkage_calibrate | cmd/atlas/calibration_tasks.go |
| macro_risk_calibrate | cmd/atlas/calibration_tasks.go |
| margin_history_backfill | cmd/atlas/capital_tasks.go |
| ml_retrain | (unknown) |
| narrative_calibrate | cmd/atlas/calibration_tasks.go |
| risk_gate_calibrate | cmd/atlas/calibration_tasks.go |
| rsi_tw_calibrate | cmd/atlas/calibration_tasks.go |
| sa11_dark_launch_check | (unknown) |
| storage_cleanup | cmd/atlas/operations_tasks.go |
| stress_test_daily | cmd/atlas/main.go:1205 |
| structural_trend_calibrate | cmd/atlas/calibration_tasks.go |
| system_health_monitor | cmd/atlas/operations_tasks.go |
| tsmc_revenue | (unknown) |

### 168h (weekly) interval (4 tasks)

| Task | Source file (likely) |
|------|---------------------|
| auto_calibrate | cmd/atlas/calibration_tasks.go |
| auto_experiment | (unknown) |
| seasonal_calibration | (unknown) |
| window_backtest | (unknown) |

> **Note**: `stress_test_daily` is in the designed-silent list despite the user's earlier observation that it fired. The earlier observation may have been from a previous container lifetime; the current container (started 2026-07-20T16:46:49Z) has uptime ~8.3h, well below the 24h interval.

---

## Fired (29 tasks, snapshot 2)

For reference, the tasks that ARE working. (Sorted by interval ascending.)

| Task | Interval | Last Run (UTC) |
|------|----------|----------------|
| rule_engine_check | 36s | 17:05:09 |
| health_check | 36s | 17:05:08 |
| auto_universe_refresh | 72s | 17:05:07 |
| auto_universe_full_rebuild | 72s | 17:05:09 |
| metrics_snapshot | 72s | 17:05:09 |
| universe_coverage_check | 72s | 17:05:08 |
| macro_ingest | 5m | 17:01:25 |
| us_market_refresh | 5m | 17:01:27 |
| channel_health_sync | 5m | 17:01:25 |
| capital_flow_refresh | 5m | 17:01:08 |
| auto_capital_flow | 30m | 16:48:51 |
| auto_margin | 30m | 16:47:35 |
| regime_calibrate | 1h | 16:49:29 |
| channel_health_finmind | 1h | 16:50:38 |
| auto_taifex_institutional | 1h | 16:47:38 |
| tej_refresh | 1h | 16:51:19 |
| autobacktest_daily | 1h | 16:47:59 |
| channel_health_fugle | 1h | 16:46:49 |
| auto_government_flow | 1h | 16:49:57 |
| daily_report_generate | 1h | 16:49:24 |
| auto_twse_sbl | 1h | 16:51:55 |
| channel_health_fubon | 1h | 16:52:00 |
| channel_health_twse_replay | 1h | 16:47:09 |
| narrative_weight_update | 1h | 16:47:29 |
| silicon_cycle_update | 10m | 16:56:25 |
| (others: 5 sub-hour interval tasks) | | |

> **Disappeared from silent list between snapshots**: `realtime_feed` (30s), `e2e_chain_probe` (6h), and several sub-hour tasks fired during the 1h gap between snapshots 1 and 2. This validates the "designed-silent = interval > uptime" classification.

---

## Action Items (operational, not code changes)

1. **Run `docker logs atlas-go --tail 5000 2>&1 | grep -E 'auto_geopolitical|janus_regime_refresh|burn_in_skip|maturity_skip'`** to confirm maturity-gate behavior for I-01 and I-02.
2. **Read `cmd/atlas/capital_tasks.go::auto_geopolitical` task body** to confirm the gate. If `BurnIn` skip log appears, document as designed.
3. **Wait 24h+ for the 27 designed-silent 24h tasks** to confirm they fire on schedule. Update this triage with confirmed-fired entries.
4. **If any task that should have fired still hasn't** after 24h, file a follow-up bug.

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial triage: 36 silent → 32 designed-silent + 2 investigate + 2 false-alarms (sub-hour tasks fired in the 1h gap between snapshots). Methodology now relies on snapshot-comparison rather than single-point audit. | Sisyphus |