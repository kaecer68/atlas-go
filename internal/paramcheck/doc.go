// Package paramcheck validates ParameterMetadata sections inside a JSON
// config tree. It is used by cmd/validate-parameters as a pre-commit gate.
//
// paramcheck walks a JSON config recursively and verifies that every tunable
// parameter (e.g. inside configs/parameters.json) has the required
// metadata fields per the ParametersConfig contract:
//
//   - "rationale" (string, non-empty): why this value was chosen
//   - "source" (string, non-empty): authoritative origin of the value
//   - "todo" (string, optional): known issue or pending change description
//
// Validation results are emitted as a structured report (pass/fail per field)
// suitable for CI integration. The validator returns an error on the first
// invalid parameter to fail fast in pre-commit hooks.
//
// This package is pure (no I/O); cmd/validate-parameters handles file reading
// and report writing.
//
// Maturity: utility
package paramcheck
