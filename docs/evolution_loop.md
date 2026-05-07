# Evolution Loop

## Goal

Turn the current scaffold into a controlled self-improving research system.

## Required Closed Loop

1. run agents on replay or live paper market data
2. emit structured recommendations
3. simulate fills through the execution engine
4. score outcomes by agent, skill, and portfolio
5. identify weak areas
6. mutate one prompt or rule at a time
7. replay on unseen periods
8. keep or revert based on objective metrics

## Mutation Unit

The smallest allowed unit of change should be one of:

- one prompt revision for one agent
- one scoring rule revision
- one portfolio constraint revision

Never mutate several unrelated parts at once.

## Acceptance Metrics

Default metrics:

- portfolio return
- Sharpe-like risk-adjusted score
- max drawdown
- hit rate
- turnover
- exposure discipline

Acceptance should scale with maturity:

- exploratory experiments can pass with smaller improvements
- window-validated experiments should require clearer replay deltas
- regime-aware experiments should require the strongest evidence before promotion

## Guardrails

- no live trading mutation path
- no hidden risk parameter changes
- no prompt acceptance without experiment metadata
- no training on the evaluation window
