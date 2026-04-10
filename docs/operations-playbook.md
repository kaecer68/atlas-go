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

For mutation runs, prefer explicit mode selection:

- isolated validation mode: `--no-fallback --no-auto-pivot`
- guarded throughput mode: default auto-pivot with `--min-sample-for-rank <n>`

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

When running `today-start`, sample size also affects mutation-type ranking:

- mutation types with insufficient historical sample count are excluded from weighted ranking
- raise `--min-sample-for-rank` when you prefer conservative switching
- lower it only for exploratory search, and treat outcomes as low-confidence

### Understand guard outcomes

`today-start` can skip or switch before execution:

- `Primary mutation marked futile...`: recent runs for that mutation type are all non-improving in the same window
- `Primary cycle skipped due to futility guard...`: skip path (usually with `--no-auto-pivot`)
- `[pivot] Switching primary mutation type...`: auto-pivot picked an alternative using weighted ranking

Interpret these as control signals, not errors.

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

If mutation flow behaves unexpectedly, also inspect:

6. futility guard status in `scripts/openclaw/today-start.sh`
7. `--min-sample-for-rank` value and candidate sample counts (`n` in pivot logs)

## Baseline Promotion

After an experiment is accepted:

1. Promote the accepted result into the baseline policy store.
2. Confirm the baseline policy version and promotion history changed.
3. Re-run replay or backtest commands so the next cycle uses the promoted baseline.

This keeps runtime execution, replay compare, and future mutations aligned to the same formal baseline.

## Human Approval Workflow

Use the human-in-the-loop wrapper as the default decision entrypoint for promote/reject/revert.

### Decision Entry

```bash
# approve and promote
./scripts/openclaw/human-approval.sh --approve --experiment <exp-id> --reason "Passes replay and guard gates"

# reject (audit-only)
./scripts/openclaw/human-approval.sh --reject --experiment <exp-id> --reason "Insufficient improvement evidence"

# revert baseline
./scripts/openclaw/human-approval.sh --revert --reason "Rollback after post-promotion alert"
```

### Audit Artifact Check

Each decision writes one event file under `data/state/approvals/`.
Validate that the event contains required fields:

- `decision_id`
- `timestamp`
- `actor`
- `action`
- `reason`
- `dry_run`

### Event Replay (Dry-Run First)

Use approval event replay to verify the decision can be reconstructed from audit artifacts:

```bash
# replay one stored approval/reject/revert event without state mutation
./scripts/openclaw/replay-approval-event.sh --event data/state/approvals/<decision-file>.json --dry-run
```

### One-Command Verification

Run the dedicated checker when changing decision scripts or event schema:

```bash
./scripts/openclaw/verify-human-approval-eventx.sh
```

This verifies:

- event JSON schema fields are present and correctly typed
- event file persistence matches emitted decision payload
- replay wrapper can reconstruct and execute a dry-run decision from stored event

### CI Gate Requirement

Governance and operations verifiers are enforced in CI as dedicated jobs in `.github/workflows/ci.yml`:

- workflow: `ci`
- job: `governance`
- job: `operations`

For branch protection, require both status checks `ci / governance` and `ci / operations` so promote/reject/revert logic, replay determinism, M5 scenario verification, and M8 operations drills cannot regress silently.

### Branch Protection Setup (GitHub)

Preferred path (automation + guided approval):

```bash
# default: dry-run, show current config, options, and risk notes
./scripts/openclaw/setup-branch-protection.sh

# apply after reviewing prompts and confirmation phrase
./scripts/openclaw/setup-branch-protection.sh --apply
```

The setup script includes anti-misconfiguration checks:

- always starts in dry-run mode
- shows current protection config before proposing changes
- explains option-level trade-offs and risk consequences
- requires explicit final confirmation before apply
- creates a pre-apply snapshot under `data/state/branch-protection-snapshots/`

Optional snapshot location override:

```bash
./scripts/openclaw/setup-branch-protection.sh --apply --backup-dir data/state/custom-branch-protection-backups
```

Restore from a previous snapshot:

```bash
# preview restore payload and risk notes (dry-run)
./scripts/openclaw/setup-branch-protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json

# apply restore (requires explicit confirmation phrase)
./scripts/openclaw/setup-branch-protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json --apply
```

Restore mode anti-misconfiguration checks:

- snapshot file must exist and include `owner/repo/branch`
- snapshot target must match current repository and branch
- snapshot must contain a valid `protection` object
- restore mode still requires explicit human confirmation before apply

Recommended repository setting path:

1. GitHub repository -> Settings -> Branches
2. Add or edit branch protection rule for `main`
3. Enable `Require status checks to pass before merging`
4. Select required checks:
   - `ci / governance`
   - `ci / operations`
5. Optional but recommended:
   - Enable `Require branches to be up to date before merging`
   - Enable `Require conversation resolution before merging`
6. Save rule and verify by opening a test PR

Quick verification checklist after saving:

- PR Checks tab shows both `ci / governance` and `ci / operations`
- Merge button stays blocked until both checks pass
- Failed operations or governance jobs block merge as expected

The CI governance job runs strict mode by default:

```bash
./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity
```

Use this strict mode after scenario design is calibrated for your replay window.

## Operations Gate (M8)

Use the operations gate verifier for staging-safe production-readiness checks:

```bash
./scripts/openclaw/verify-operations-gate.sh
```

What it checks:

- runbook command coverage for rollback and replay workflow
- Prometheus config sanity for atlas metrics scraping
- dry-run rollback drill via human approval event + replay
- human approval event schema/replay contract

Optional deep mode:

```bash
./scripts/openclaw/verify-operations-gate.sh --with-governance
```

Use `--with-governance` when you want to chain M8 checks with strict governance verification in one run.
