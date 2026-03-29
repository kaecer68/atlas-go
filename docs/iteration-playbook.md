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

Mutation type also affects acceptance strictness:

- prompt tightening is the default, least risky mutation class
- risk rule changes should face higher evidence thresholds
- portfolio constraint revisions should face the highest thresholds because they affect the whole system

Mutation type should also change the candidate artifact shape:

- prompt tightening should emit a prompt candidate
- risk rule changes should emit a rule proposal artifact
- portfolio constraint revisions should emit a governance/constraint proposal artifact

Judge logic should read those artifact shapes differently:

- prompt candidates should be checked for signal-tightening language
- risk rule proposals should be checked for structured rule fields
- portfolio revisions should be checked for structured governance fields

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
