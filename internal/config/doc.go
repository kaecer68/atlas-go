// Package config provides the dual configuration system for atlas-go:
// deployment settings via Config (environment variables) and tunable
// algorithm parameters via ParametersConfig (JSON file with authoritative
// source tracking).
//
// Two systems, never mix:
//
//	Config            — config.Load()                    Deployment: paths, API keys, feature flags
//	                   Example: ATLAS_WORK_DIR, ATLAS_LEDGER_DIR
//	ParametersConfig  — config.LoadParametersConfig(path) Tunable: WeightMax, MomentumLookbackDays
//	                   Each field is wrapped in ParameterMetadata[T] with Rationale + Source
//
// Deployment settings MUST NOT live in ParametersConfig and vice versa.
//
// Common entry points:
//
//	config.Load()                  — Load Config; call once in main()
//	config.LoadParametersConfig(p) — Returns *ParametersConfig; call Validate()
//	config.GetParametersConfig()   — Singleton accessor via sync.Once
//	config.GetReplayDataPath(wd)   — Resolve replay path (env → VERSION file → default)
//
// Cautions:
//   - loadEnvFile() strips KEY="value" and KEY='value' quotes silently;
//     KEY=va"lue" is left untouched. Do not rely on quoting as escaping
//   - Setting ATLAS_ENV_FILE completely replaces .env loading (document this)
//   - envOrKeychain() currently delegates to envOr() only — Keychain
//     integration is TODO; treat environment secrets as not-secret
//   - LoadParametersConfig() silently falls back to defaults
//     (parameters_defaults.go) on invalid JSON, before Validate() runs
//   - Hardcoded magic numbers in business logic are forbidden; add new fields
//     to ParametersConfig with ParameterMetadata[T]. See docs/PARAMETER_SYSTEM.md
//   - os.Getenv calls in config.go are grandfathered; new code MUST NOT use them
//
// Maturity: stable
package config
