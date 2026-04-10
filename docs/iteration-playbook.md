# Iteration Playbook

## Purpose

This document defines how to improve `atlas-go` intelligently instead of making random changes.

## Golden Rule

Change one meaningful thing at a time, then evaluate it on a replay window.

## Iteration Workflow

1. Identify the weakest agent from session or window scorecards.
2. Read its prompt, skill definition, and recent outcomes.
3. Extract a repeated failure pattern.
4. Write a hypothesis.
5. Produce one small prompt or rule mutation.
6. Replay on a defined comparison window.
7. Compare baseline and candidate.
8. Keep or revert.

Recommended execution mode for clean comparison:

- use isolated runs (`--no-fallback --no-auto-pivot`) when validating a single mutation hypothesis
- use auto-pivot mode when the goal is throughput rather than strict A/B causality

Before step 5, read the machine-readable policy:

- required skills
- forbidden actions
- mutation type
- acceptance gates

## What Counts as a Good Mutation

- adds a missing evidence requirement
- tightens a weak entry condition
- reduces false positives in a specific regime
- clarifies what to avoid
- improves consistency without changing the role entirely

## What Counts as a Bad Mutation

- rewriting the whole prompt at once
- changing role identity and evaluation criteria together
- mixing market assumptions with execution rules
- hiding a risk-policy change inside a prompt
- accepting a change because the explanation sounds smart

## Intelligence-Building Techniques

### 1. Repeated-loss clustering

Group failures by:

- sector
- regime
- style
- liquidity profile
- ETF versus single-name behavior

Mutate only when a pattern appears more than once.

### 2. Prompt tightening

Prefer:

- "require confirmation"
- "exclude low-liquidity names"
- "downgrade conviction when signal conflicts with regime"

Avoid:

- vague tone changes
- broad personality rewrites

### 3. Boundary strengthening

If an agent keeps drifting outside its role, strengthen the role boundary rather than teaching it more trivia.

### 4. Rule promotion

If the same lesson keeps recurring, move it upward:

- from experiment
- to skill map
- to docs
- to config
- to engine code

The strongest lessons should become system rules, not folklore.

### 5. Regression awareness

Never optimize one metric blindly. Every candidate should be checked for:

- worse drawdown
- more concentration
- higher fragility
- lower generalization

### 6. Policy-constrained mutation

Every mutation should preserve:

- required skill coverage
- forbidden action boundaries
- acceptance gates

If a proposed change violates policy, revise it before replay.

Mutation type affects acceptance strictness with current judge thresholds:

- `prompt_tightening`: minimum improvement `0.0005`
- `risk_rule_change`: minimum improvement `0.001`
- `portfolio_constraint_revision`: minimum improvement `0.001`

Mutation type should also change the candidate artifact shape:

- prompt tightening should emit a prompt candidate
- risk rule changes should emit a rule proposal artifact
- portfolio constraint revisions should emit a governance/constraint proposal artifact

Judge logic should read those artifact shapes differently:

- prompt candidates should be checked for signal-tightening language
- risk rule proposals should be checked for structured rule fields
- portfolio revisions should be checked for structured governance fields

### 7. Guard-aware iteration

`today-start` applies guard logic before and during mutation execution:

- futility guard: same `agent + window + mutation_type` with 3 recent non-improving runs (`candidate <= baseline`) is treated as futile
- minimum sample for ranking: mutation types with `n < --min-sample-for-rank` do not enter weighted ranking
- weighted ranking for auto-pivot: `weighted = avg_delta * min(1, n/5)`, where `avg_delta = avg(candidate - baseline)` over recent runs

Use these guards as planning signals:

- if all candidates are futile, skip primary and move to new window/agent
- if sample count is low, collect more windows before trusting ranking

## Suggested Evaluation Questions

- Did the mutation improve the intended metric?
- Did it reduce a specific repeated failure mode?
- Did it make behavior more stable across sessions?
- Did it degrade another critical dimension?
- Is this lesson prompt-specific or should it become a system rule?

## Maturity Ladder

### Level 1: Exploratory

- one replay session
- useful for debugging
- not enough for acceptance

### Level 2: Window Validated

- multi-session replay
- enough for tentative acceptance

### Level 3: Regime-Aware

- validated across multiple market conditions
- suitable for promotion into baseline prompt set

### Level 4: Systemized

- lesson moved into docs, registry, guardrails, or code
- no longer relies on memory alone
