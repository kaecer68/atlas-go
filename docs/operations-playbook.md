# Operations Playbook

## Purpose

This document explains how to operate `atlas-go` correctly day to day.

## Operating Modes

### 1. Single Session Replay

Use when:

- validating one replay date
- checking agent output shape
- verifying ledger output
- testing one importer or one prompt adjustment

Core command:

```bash
go run ./cmd/atlas
```

### 2. Replay Import

Use when:

- normalizing TWSE or TPEX open-data files
- preparing replay-ready datasets
- moving from raw CSV into internal JSONL

Core command:

```bash
go run ./cmd/import-replay -source samples/replay/twse_stock_day_all_sample.csv -target data/replay/tw_open_data.jsonl
```

### 3. Window Backtest

Use when:

- evaluating agent behavior over a period
- choosing the weakest agent
- generating mutation candidates

Core command:

```bash
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

## Standard Operating Procedure

1. Verify the agent registry path and replay data path.
2. Confirm the replay date or backtest window.
3. Run import if the source data is still raw.
4. Run a replay session or window backtest.
5. Inspect:
   - session summary
   - outcomes
   - experiments
   - weakest-agent output
6. Decide whether the result is exploratory or ready for mutation design.

## Operator Techniques

### Start small

Use one replay date first. Confirm the ledger and summary artifacts before running wider windows.

### Keep raw and normalized data separate

- raw files: source dumps
- normalized JSONL: replay-ready internal format

This keeps importer bugs from polluting analysis.

### Treat session artifacts as evidence

The files in `data/state/sessions/<session-id>/` are not logs for decoration. They are the evidence trail for why a future prompt mutation was justified.

### Respect sample-size limits

If only one or two sessions exist, use the result for orientation, not for strong model claims.

### Read the weakest-agent result with context

The weakest agent is a candidate for investigation, not automatic proof that the prompt is bad. Check:

- number of observations
- regime context
- concentration of failures
- data completeness
- required skills and forbidden actions from the registry policy

## Artifacts Checklist

For a healthy run, expect:

- `recommendation_outcomes.jsonl`
- `experiments.jsonl`
- `summary.json`
- window summary when a backtest window is run

## Failure Handling

If a run looks wrong, inspect in this order:

1. replay source path
2. session date and forward-return availability
3. registry load path
4. outcome file contents
5. weakest-agent selection logic

## Baseline Promotion

After an experiment is accepted:

1. Promote the accepted result into the baseline policy store.
2. Confirm the baseline policy version and promotion history changed.
3. Re-run replay or backtest commands so the next cycle uses the promoted baseline.

This keeps runtime execution, replay compare, and future mutations aligned to the same formal baseline.
