# AI Agent Architecture Documentation

## Overview

Atlas-Go implements a sophisticated multi-layer AI agent system with 15 specialized agents working in coordination. This document provides a comprehensive mapping between the agent registry, skill definitions, and system architecture.

## Agent Layer Architecture

### Layer 1: Context Agents (Market Environment Analysis)
| Agent ID | Skill | Focus | Universe | Primary Metrics |
|----------|-------|-------|-----------|-----------------|
| `taiwan-macro-01` | `taiwan_macro` | Taiwan macro conditions | None | regime_accuracy, drawdown_avoidance |
| `foreign-flow-01` | `foreign_flow` | Foreign institutional flows | None | regime_accuracy, flow_alignment |

**Responsibilities**:
- Determine market regime and risk budget
- Provide macro context for lower layers
- Avoid single-stock recommendations

### Layer 2: Sector Agents (Industry Expertise)
| Agent ID | Skill | Focus | Universe | Primary Metrics |
|----------|-------|-------|-----------|-----------------|
| `semi-desk-01` | `semiconductor_desk` | Foundry, packaging, equipment | 2330.TW, 2303.TW, 2454.TW, 3034.TW | alpha_hit_rate, risk_adjusted_return |
| `ai-desk-01` | `ai_supply_chain_desk` | ODM, server, PCB, thermal | 2382.TW, 6669.TW, 3017.TW, 3037.TW | alpha_hit_rate, turnover_efficiency |
| `etf-rotation-01` | `etf_rotation_desk` | Broad ETF allocation | 0050.TW, 0056.TW, 00878.TW | drawdown_control, defensive_capture |
| `financials-desk-01` | `financials_desk` | Banks, insurers, brokers | 2881.TW, 2882.TW, 2891.TW | downside_capture, carry_efficiency |
| `shipping-desk-01` | `shipping_desk` | Container and transport | 2603.TW, 2609.TW, 2615.TW | cycle_timing, volatility_capture |

**Responsibilities**:
- Generate sector-specific recommendations
- Apply industry expertise and knowledge
- Respect sector-specific constraints and risks

### Layer 3: Style Agents (Strategy Implementation)
| Agent ID | Skill | Focus | Universe | Primary Metrics |
|----------|-------|-------|-----------|-----------------|
| `growth-momentum-01` | `growth_momentum` | Trend persistence | None | alpha_hit_rate, momentum_followthrough |
| `value-yield-01` | `value_yield` | Dividend durability | None | drawdown_control, carry_quality |
| `earnings-quality-01` | `earnings_quality` | Accounting quality | None | estimate_quality, post_earnings_followthrough |
| `technical-breakout-01` | `technical_breakout` | Entry timing | None | breakout_followthrough, false_breakout_avoidance |

**Responsibilities**:
- Apply style-specific filters to sector recommendations
- Ensure style consistency across recommendations
- Provide style-specific risk adjustments

### Layer 4: Control Agents (Risk Management)
| Agent ID | Skill | Focus | Universe | Primary Metrics |
|----------|-------|-------|-----------|-----------------|
| `cro-01` | `cro_risk` | Portfolio risk control | None | drawdown_control, concentration_violations |
| `cio-01` | `cio_portfolio` | Portfolio synthesis | None | portfolio_return, sharpe_like |

**Responsibilities**:
- Attack portfolio proposals before execution
- Ensure risk limits are respected
- Synthesize final portfolio decisions

### Layer 5: Superinvestor Agents (Master Strategies)
| Agent ID | Skill | Focus | Universe | Primary Metrics |
|----------|-------|-------|-----------|-----------------|
| `super-dru-01` | `druckenmiller_macro` | Macro asymmetric trades | None | macro_asymmetric_hits, momentum_capture |
| `super-asc-01` | `aschenbrenner_ai_compute` | AI capex beneficiaries | None | ai_capex_alignment, compute_cycle_timing |
| `super-bak-01` | `baker_deep_tech` | IP moat analysis | None | ip_quality_score, rdn_efficiency |
| `super-ack-01` | `ackman_quality_compound` | Quality compounders | None | roic_consistency, fcf_conversion |

**Responsibilities**:
- Provide high-conviction, concentrated recommendations
- Apply master investor philosophies
- Override lower layers when edge is clear

## Phase 3 Advanced Systems

### Agent Spawning System
- **Purpose**: Automatically detect capability gaps and spawn new agents
- **Components**: GapDetector, AgentFactory, SpawningManager
- **Process**: Gap detection -> Agent creation -> Training -> Validation -> Acceptance

### PRISM Training System
- **Purpose**: Multi-regime training for different market conditions
- **Regimes**: trending_up, trending_down, range_bound, high_volatility, low_volatility, rotation, earnings
- **Process**: Queue management -> Regime-specific training -> Performance optimization

### Reflexivity Engine
- **Purpose**: Detect and analyze market feedback loops
- **Bias Types**: TrendFollowing, Contrarian, Anchoring, Confirmation, Herding
- **Process**: Bias detection -> Loop analysis -> Recommendation adjustment

### MiroFish Swarm
- **Purpose**: Parallel strategy simulation and consensus building
- **Components**: 100 MiroFish agents, ConsensusEngine, AnomalyDetector
- **Process**: Parallel simulation -> Consensus aggregation -> Anomaly detection

## Agent Coordination Flow

### Daily Operation Sequence
```
1. Market Data Input
   |
2. Layer 1: Context Analysis (taiwan-macro-01, foreign-flow-01)
   |
3. Layer 2: Sector Analysis (semi-desk-01, ai-desk-01, etc.)
   |
4. Layer 3: Style Filtering (growth-momentum-01, value-yield-01, etc.)
   |
5. Layer 4: Risk Control (cro-01, cio-01)
   |
6. Layer 5: Superinvestor Override (if applicable)
   |
7. Integrated System Decision
```

### Weight Adjustment Process
```
1. Performance Tracking (DarwinianWeightManager)
   |
2. Rolling Metrics Calculation (20-day windows)
   |
3. Performance Ranking (top 33% boost, bottom 33% reduction)
   |
4. Volatility Penalty Application
   |
5. Weight Range Enforcement (0.3 - 2.5)
```

## Configuration Mapping

### Agent Registry to Skills Mapping
Each agent in `configs/agents.json` maps to a specific skill in `docs/skills-map.md`:

```json
{
  "id": "semi-desk-01",
  "skill": "semiconductor_desk",
  "layer": "sector",
  "requiredSkills": ["semiconductor_desk", "earnings_quality", "technical_breakout"]
}
```

### Skill Dependencies
- **Sector agents** depend on **Context agents** for market regime
- **Style agents** depend on **Sector agents** for recommendations
- **Control agents** depend on **all agents** for risk assessment
- **Superinvestor agents** can override all lower layers

## Performance Metrics

### Agent-Specific Metrics
Each agent type should be evaluated with layer-appropriate metrics:

- **Context agents**: regime_accuracy, drawdown_avoidance
- **Sector agents**: alpha_hit_rate, risk_adjusted_return
- **Style agents**: style_consistency, risk_adjusted_style_return
- **Control agents**: risk_limit_compliance, portfolio_stability
- **Superinvestor agents**: high_conviction_return, asymmetric_capture

### System-Level Metrics
System-level values are runtime outputs, not fixed constants in documentation.
Use replay and experiment artifacts as source of truth:

- `data/state/experiments/*.json`
- `data/state/sessions/*/summary.json`
- window backtest outputs from `cmd/backtest-window`

## Evolution and Learning

### Weak Agent Selection
- Identify weak agents from replay-window evidence
- Use observation count and regime context before mutation
- Avoid changing multiple weak agents in one iteration

### Mutation Strategies
Current mutation classes are:

- `prompt_tightening`
- `risk_rule_change`
- `portfolio_constraint_revision`

Apply one mutation class per cycle and validate against baseline via judge gates.

### Continuous Improvement
- Track outcomes continuously in ledger artifacts
- Use futility and sample-size guards to avoid low-signal retries
- Promote only after gate-satisfying replay evidence

## Integration with Advanced Systems

### Spawning Integration
- New agents automatically inherit layer-appropriate skills
- Gap detection focuses on missing sector or style coverage
- Training uses PRISM multi-regime approach

### PRISM Integration
- Each agent trained on specific regime scenarios
- Performance tracked by regime type
- Weights adjusted based on regime-specific performance

### Reflexivity Integration
- Bias detection applied to all agent recommendations
- Feedback loops identified across agent layers
- Recommendations adjusted based on reflexivity analysis

### Swarm Integration
- Agent strategies tested in parallel simulations
- Consensus used to validate agent recommendations
- Anomaly detection identifies outlier agent behavior

## Conclusion

Atlas-Go uses a layered, policy-bounded multi-agent architecture.

- `docs/skills-map.md` defines current skills, mutation profiles, and guard behavior.
- This document explains execution structure and component boundaries.
- Runtime artifacts remain the final truth for measured performance.
