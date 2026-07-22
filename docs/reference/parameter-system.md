# Parameter System Documentation

## Overview

The Atlas parameter system manages **228 tunable parameters across 23 modules** in `configs/parameters.json` (the canonical source of truth). Of these, **215 are currently runnable by `parameter-health-check`** — the remaining 13 belong to the 7 v0.0.0.31 experimental modules whose parameter monitoring is pending P1 follow-up (see "v0.0.0.31 模組數更新" callout below). Each parameter includes provenance tracking, calibration metadata, and runtime update capabilities.

> **v0.0.0.31 模組數更新**：從 16 個 S/E-level modules 擴增至 23 個，新增 7 個 experimental 模組（`strategy_validator`、`capitalflow`、`eventdriven`、`strategy_ranker`、`subscription`、`recommender`、`dailyreport`）。新模組的 parameters 暫未進入 health check 流程（P1 殘留），calibration 應使用 `--module=all` 對既有 16 個 S/E module 跑。

## Quick Reference

| Command | Purpose |
|---------|---------|
| `go run ./cmd/parameter-health-check` | Audit parameter health and citations |
| `go run ./cmd/calibrate-parameters --module=garch --data=returns.json` | Calibrate GARCH parameters |
| `go run ./cmd/calibrate-parameters --module=var --data=returns.json` | Calibrate VaR-based parameters |
| `go run ./cmd/calibrate-parameters --module=all --data=returns.json` | Calibrate all supported modules |
| `go run ./cmd/calibrate-thresholds` | Calibrate industry cycle thresholds |

## Parameter Structure

Each parameter in `configs/parameters.json` follows this schema:

```json
{
  "value": <typed-value>,
  "rationale": "human-readable explanation",
  "source": "heuristic|empirical|literature|inferred|calibrated",
  "todo": "optional improvement note",
  "calibration_method": "optional: MLE_grid_search|VaR_inference|...",
  "citation": {
    "source_type": "heuristic|empirical|literature|inferred|calibrated",
    "source_reference": "specific source or reasoning",
    "evidence_quality": "high|medium|low",
    "update_policy": "auto|manual|frozen",
    "validation_method": "how to verify correctness",
    "dependencies": ["other_parameter_names"],
    "last_validated": "2026-05-04T00:00:00Z"
  }
}
```

## Source Types

| Type | Description | Update Policy | Example |
|------|-------------|---------------|---------|
| **literature** | Peer-reviewed academic source | frozen | GARCH parameters (Bollerslev 1986) |
| **empirical** | Derived from historical data | auto | TWSE volatility distribution |
| **heuristic** | Domain expert judgment | manual | Darwinian weight thresholds |
| **inferred** | Automated inference/calibration | manual | IEEE 754 guard thresholds |
| **calibrated** | Backtest optimization result | manual | Performance bonus percentage |

## Module Coverage

| Module | Parameters | Calibrated | Literature | Empirical | Heuristic |
|--------|-----------|------------|------------|-----------|-----------|
| darwinian | 22 | 0 | 1 | 4 | 17 |
| factor | 16 | 0 | 2 | 3 | 11 |
| optimizer | 9 | 0 | 2 | 0 | 7 |
| sizing | 20 | 2 | 5 | 1 | 12 |
| health | 11 | 0 | 2 | 0 | 9 |
| garch | 14 | 3 | 6 | 0 | 5 |
| experiment | 10 | 0 | 2 | 0 | 8 |
| baseline | 9 | 0 | 2 | 2 | 5 |
| orchestrator | 19 | 0 | 0 | 0 | 19 |
| risk | 15 | 0 | 2 | 0 | 13 |
| realtime | 8 | 0 | 0 | 0 | 8 |
| janus | 14 | 0 | 0 | 0 | 14 |
| narrative | 22 | 0 | 0 | 0 | 22 |
| marketdata | 12 | 0 | 0 | 1 | 11 |
| industry | 20 | 1 | 0 | 1 | 18 |
| strategy | 4 | 0 | 0 | 0 | 4 |

## Runtime Parameter Updates

The `InferenceEngine` in `internal/config/inference.go` supports runtime parameter updates:

```go
ie := config.NewInferenceEngine(cfg)

// Set parameter
err := ie.SetParameter("garch_alpha", 0.15)

// Get parameter
value, ok := ie.GetParameter("garch_alpha")

// List all parameters
params := ie.ListParameters() // returns []string with 140+ names

// Calibrate GARCH
err := ie.CalibrateGARCH(historicalReturns)

// Calibrate VaR
err := ie.CalibrateVaR(historicalReturns)
```

### Supported Parameter Names

Parameters use dot notation for nested values:
- Simple: `garch_alpha`, `sizing_kelly_fraction`
- Map sub-keys: `factor_institutional_sentiment_weights_foreign`

## Calibration Methods

### GARCH Calibration
- **Method**: MLE grid search
- **Updates**: `garch_omega`, `garch_alpha`, `garch_beta`
- **Requirements**: 100+ historical returns
- **Validation**: Stationarity constraint (alpha + beta < 1)

### VaR Calibration（#1265 canonical metric source）
- **Method**: Historical simulation（與 `risk.CalculateVaR()` 相同公式，不同 estimand）
- **Updates**: `sizing_target_volatility`, `sizing_max_drawdown_limit`
- **Requirements**: 30+ historical returns（校準用閘；生產監控用 252）
- **Validation**: ES > VaR, negative values for losses
- **Canonical split**（#1265）：
  - **生產監控 VaR**：`risk.CalculateVaR(returns, 0.95)` — 252 obs gate，用於 `/api/risk/metrics`、`risk_get_metrics`
  - **校準 VaR**：`config.InferenceEngine.EstimateVaR()` — 30 obs gate，用於參數自動校準
  - **回測訊號 VaR**：`risk.CalculateVaRPercentile()` — 無 gate，由 `SignalEngine` 自備 20-sample guard

### Industry Threshold Calibration
- **Method**: Percentile-based from revenue data
- **Updates**: `industry.cycle_thresholds.*`
- **Requirements**: `data/replay/month_revenue.jsonl`
- **Validation**: Sample size >= 10 per industry

## Health Metrics

Run `go run ./cmd/parameter-health-check` to generate:

```json
{
  "total_parameters": 215,
  "parameters_with_citation": 215,
  "parameters_with_todo": 90,
  "parameters_calibrated": 5,
  "high_evidence_count": 32,
  "medium_evidence_count": 96,
  "low_evidence_count": 87,
  "issues": [...],
  "recommendations": [...]
}
```

## Best Practices

1. **Always cite new parameters** - Include `citation` block with evidence quality and validation method
2. **Use empirical data when possible** - Prefer `source: empirical` over `heuristic`
3. **Add calibration_method for auto-updated params** - Mark parameters that CalibrateGARCH/VaR update
4. **Document TODOs** - Leave actionable TODOs for parameters needing further validation
5. **Validate before deploying** - Run `parameter-health-check` before committing parameter changes

## Troubleshooting

### Parameter not found in InferenceEngine
- Check exact name using `ie.ListParameters()`
- Map parameters use underscore notation: `factor_institutional_sentiment_weights_foreign`

### Calibration fails with insufficient data
- GARCH: Requires 100+ returns
- VaR: Requires 30+ returns
- Industry: Requires 10+ observations per industry

### Citation missing
- Run `python3 scripts/add_citations.py` to auto-generate citation blocks
- Review generated citations for accuracy

## Migration Guide

### From hardcoded values
1. Move value to `configs/parameters.json`
2. Add `rationale`, `source`, and `citation`
3. Use `config.DefaultParametersConfig()` to load
4. Access via `cfg.Module.ParamName.Value`

### Adding new module
1. Add struct to `internal/config/parameters.go`
2. Add to `ParametersConfig` struct
3. Add to `configs/parameters.json`
4. Add SetParameter cases in `inference.go`
5. Run `scripts/add_citations.py`
