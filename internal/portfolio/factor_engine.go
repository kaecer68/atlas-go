package portfolio

// factor_engine.go is intentionally minimal: all FactorEngine methods,
// types, and helpers have been decomposed into 11 concern-separated
// files (factor_engine_types.go, factor_engine_constructors.go,
// factor_engine_helpers.go, factor_engine_{momentum,value,quality,
// institutional,liquidity,aggregate,etf,precious_metals}.go). See
// internal/portfolio/AGENTS.md §2 for the full file map.
//
// This stub remains so the factor_engine family of files is anchored
// at factor_engine.go for grep navigation and so the Layer 3 snapshot
// test (api_snapshot_test.go) keeps an factor_engine.go entry.
