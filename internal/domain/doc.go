// Package domain provides canonical domain types: string enums for regime, recommendation, and position.
//
// Convention: types in this package fall into two categories:
//
//   - Contract types (must remain open-source): interfaces and structs that define
//     the system's public vocabulary — AgentSpec, Recommendation, Quote, Regime,
//     AgentLayer, etc. Every package depends on these; changing them is a breaking change.
//
//   - Implementation types (may be extracted to region-specific packages): concrete
//     configurations and defaults with locale-specific values — tax configs, regime
//     calibration constants, Taiwan-specific defaults. These may be moved to
//     domain/tw/ or a separate package in the future.
//
// When adding new types, ask: "Does every consumer of atlas-go need this type?"
// If yes → contract type. If no → implementation type; consider a sub-package.
//
// Maturity: stable
package domain
