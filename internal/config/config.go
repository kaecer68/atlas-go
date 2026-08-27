package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type Config struct {
	WorkDir                    string
	GoMemberJwksURL            string // GO_MEMBER_JWKS_URL — go-member /.well-known/jwks.json for RS256 auth (C-02)
	GoMemberAPIBaseURL         string // GO_MEMBER_API_BASE_URL — go-member API base for login/register thin proxy (M4b); empty = legacy HS256 login
	MaturityFirstStart         string // ATLAS_MATURITY_FIRST_START — RFC3339/date-only seed for fresh maturity tracker (survives data-dir loss); empty = now
	DatabaseURL                string
	MigrationsPath             string
	MarketDataProvider         string
	PrimaryMarket              string
	ReplayMode                 string
	AgentRegistryPath          string
	AgentRegistryExtraPaths    []string // merged with AgentRegistryPath via LoadRegistryMulti
	BaselinePolicyPath         string
	ParametersConfigPath       string
	LedgerDir                  string
	StoreBackend               string // "jsonl" (default), "sqlite", or "postgres" — ATLAS_STORE_BACKEND
	SQLitePath                 string // path to SQLite database file — ATLAS_SQLITE_PATH
	ReplayDataPath             string
	ReplaySessionDate          string
	FubonAPIKey                string
	FugleAPIKey                string
	FinMindAPIKey              string
	YahooEnabled               bool
	BrokerMode                 string
	BrokerMaxRetries           int
	BrokerAdapter              string
	BrokerAPIBaseURL           string
	BrokerAPIKey               string
	BrokerAPISecret            string
	BrokerHTTPTimeoutS         int
	BrokerHTTPAttempts         int
	BrokerHTTPRetryStatusCodes []int
	BrokerMaxClockSkewS        int
	BrokerNonceTTLS            int
	BrokerNonceStore           string
	BrokerNonceStorePath       string
	BrokerNonceRedisURL        string
	BrokerNonceRedisKeyPrefix  string
	BrokerSigner               string
	BrokerKeyID                string
	TWSEAPIURL                 string
	TWSEAPIKey                 string
	TWSEAPISecret              string
	TWSEAccountID              string
	FubonDMAPersonalID         string
	FubonDMAAPIKey             string
	FubonDMAScriptPath         string
	FubonDMAPythonPath         string

	// LLM capability wiring flags (opt-in, all default false).
	// Each flag gates a package-level var hook that wires an LLM capability
	// handler into a product module (narrative / prism / risk) without
	// creating an import cycle. Set to "true" via env var or .env file.
	LLMRationaleTranslationEnabled bool // LLM_RATIONALE_TRANSLATION_ENABLED — W1: rationale_corpus.go translator hook
	LLMPrismScenarioEnabled        bool // LLM_PRISM_SCENARIO_ENABLED — W2: prism_executor.go scenario explainer hook
	LLMNarrativeExplainEnabled     bool // LLM_NARRATIVE_EXPLAIN_ENABLED — W3: explain_hooks.go regime+sentiment hooks
	LLMRiskForensicsEnabled        bool // LLM_RISK_FORENSICS_ENABLED — W4: forensics_hook.go performance forensics hook
	LLMConfidenceCommentaryEnabled bool // LLM_CONFIDENCE_COMMENTARY_ENABLED — W5: confidence_hook.go confidence commentary hook
	LLMSectorAgentsEnabled         bool // LLM_SECTOR_AGENTS_ENABLED — W6: sector_agent_llm.go wired plugin hook (Issue #719, Wave 11 L2.1)
	SectorPredictionEnabled        bool // SECTOR_PREDICTION_ENABLED — gates eventdriven.SetMacroProvider() sector direction predictions (Wave 11+ C07)
	EventPredictionEnabled         bool // ATLAS_EVENT_PREDICTION_ENABLED — gates event-driven prediction consumption in orchestrator (F04)
	AllowLiveBroker                bool // ATLAS_ALLOW_LIVE_BROKER — cmd/atlas/main.go:211 live broker double-gate env
	AllowHTTPBroker                bool // ATLAS_ALLOW_HTTP_BROKER — cmd/atlas/main.go:214 HTTP broker adapter double-gate env
	AllowRealSigner                bool // ATLAS_ALLOW_REAL_SIGNER — cmd/atlas/main.go:217 real signer double-gate env
	CharterMode                    bool // ATLAS_CHARTER_MODE — Phase C2: charter-driven period→strategy/cash wiring (default false)

	Stage3TasksEnabled  bool // STAGE3_TASKS_ENABLED — gates registerStage3Tasks (default true)
	Stage3AlertsEnabled bool // STAGE3_ALERTS_ENABLED — gates registerStage3AlertTasks (default true)
}

func Load() Config {
	// Tests must be hermetic: never load .env or ~/.config/atlas-go/.env
	// (M7). A developer's local user env file (e.g. ATLAS_STORE_BACKEND=
	// postgres, API keys) must not leak into `go test` process state and
	// flip tests onto a postgres backend or expose secrets in logs.
	if !testing.Testing() {
		// 加载 .env 文件 — 优先使用 ATLAS_ENV_FILE 指定的路径，
		// 然后依次尝试 .env、~/.config/atlas-go/.env
		loadEnvFile(resolveEnvFilePath())
		loadUserEnvFile()
	}

	cfg := Config{
		WorkDir:                    envOr("ATLAS_WORK_DIR", "."),
		GoMemberJwksURL:            envOr("GO_MEMBER_JWKS_URL", ""),
		GoMemberAPIBaseURL:         envOr("GO_MEMBER_API_BASE_URL", ""),
		MaturityFirstStart:         envOr("ATLAS_MATURITY_FIRST_START", ""),
		DatabaseURL:                envOr("DATABASE_URL", ""),
		MigrationsPath:             envOr("ATLAS_MIGRATIONS_PATH", "sql/migrations"),
		MarketDataProvider:         envOr("ATLAS_MARKET_DATA_PROVIDER", "twse"),
		PrimaryMarket:              envOr("ATLAS_PRIMARY_MARKET", "TW"),
		ReplayMode:                 envOr("ATLAS_REPLAY_MODE", "daily"),
		AgentRegistryPath:          envOr("ATLAS_AGENT_REGISTRY_PATH", constants.AgentsConfigPath),
		AgentRegistryExtraPaths:    parseExtraPaths(envOr("ATLAS_AGENT_REGISTRY_EXTRA_PATHS", "")),
		BaselinePolicyPath:         envOr("ATLAS_BASELINE_POLICY_PATH", constants.StateBaselinePolicy+".json"),
		ParametersConfigPath:       envOr("ATLAS_PARAMETERS_CONFIG_PATH", "configs/parameters.json"),
		LedgerDir:                  envOr("ATLAS_LEDGER_DIR", "data/state"),
		StoreBackend:               envOr("ATLAS_STORE_BACKEND", "jsonl"),
		SQLitePath:                 envOr("ATLAS_SQLITE_PATH", "data/state/atlas.db"),
		ReplayDataPath:             envOr("ATLAS_REPLAY_DATA_PATH", "samples/replay/twse_stock_day_all_sample.csv"),
		ReplaySessionDate:          envOr("ATLAS_REPLAY_SESSION_DATE", ""),
		FubonAPIKey:                envOrKeychain("FUBON_API_KEY"),
		FugleAPIKey:                envOrKeychain("FUGLE_API_KEY"),
		FinMindAPIKey:              envOrKeychain("FINMIND_API_KEY"),
		YahooEnabled:               envOrBool("ATLAS_YAHOO_ENABLED", true),
		BrokerMode:                 envOr("ATLAS_BROKER_MODE", "dry-run"),
		BrokerMaxRetries:           envOrInt("ATLAS_BROKER_MAX_RETRIES", 1),
		BrokerAdapter:              envOr("ATLAS_BROKER_ADAPTER", "guarded"),
		BrokerAPIBaseURL:           envOr("ATLAS_BROKER_API_BASE_URL", ""),
		BrokerAPIKey:               envOr("ATLAS_BROKER_API_KEY", ""),
		BrokerAPISecret:            envOr("ATLAS_BROKER_API_SECRET", ""),
		BrokerHTTPTimeoutS:         envOrInt("ATLAS_BROKER_HTTP_TIMEOUT_SEC", 5),
		BrokerHTTPAttempts:         envOrInt("ATLAS_BROKER_HTTP_ATTEMPTS", 2),
		BrokerHTTPRetryStatusCodes: envOrIntCSV("ATLAS_BROKER_HTTP_RETRY_STATUS_CODES", []int{408, 425, 429, 500, 502, 503, 504}),
		BrokerMaxClockSkewS:        envOrInt("ATLAS_BROKER_MAX_CLOCK_SKEW_SEC", 300),
		BrokerNonceTTLS:            envOrInt("ATLAS_BROKER_NONCE_TTL_SEC", 300),
		BrokerNonceStore:           envOr("ATLAS_BROKER_NONCE_STORE", "memory"),
		BrokerNonceStorePath:       envOr("ATLAS_BROKER_NONCE_STORE_PATH", ""),
		BrokerNonceRedisURL:        envOr("ATLAS_BROKER_NONCE_REDIS_URL", ""),
		BrokerNonceRedisKeyPrefix:  envOr("ATLAS_BROKER_NONCE_REDIS_KEY_PREFIX", "atlas:nonce:"),
		BrokerSigner:               envOr("ATLAS_BROKER_SIGNER", "placeholder"),
		BrokerKeyID:                envOr("ATLAS_BROKER_KEY_ID", ""),
		TWSEAPIURL:                 envOr("ATLAS_TWSE_API_URL", ""),
		TWSEAPIKey:                 envOr("ATLAS_TWSE_API_KEY", ""),
		TWSEAPISecret:              envOr("ATLAS_TWSE_API_SECRET", ""),
		TWSEAccountID:              envOr("ATLAS_TWSE_ACCOUNT_ID", ""),
		FubonDMAPersonalID:         envOr("FUBON_DMA_PERSONAL_ID", ""),
		FubonDMAAPIKey:             envOr("FUBON_DMA_API_KEY", ""),
		FubonDMAScriptPath:         envOr("FUBON_DMA_SCRIPT_PATH", "cmd/fubon-dma/wrapper.py"),
		FubonDMAPythonPath:         envOr("FUBON_DMA_PYTHON_PATH", "python3"),

		// LLM capability wiring (all opt-in, default false)
		LLMRationaleTranslationEnabled: envOrBool("LLM_RATIONALE_TRANSLATION_ENABLED", false),
		LLMPrismScenarioEnabled:        envOrBool("LLM_PRISM_SCENARIO_ENABLED", false),
		LLMNarrativeExplainEnabled:     envOrBool("LLM_NARRATIVE_EXPLAIN_ENABLED", false),
		LLMRiskForensicsEnabled:        envOrBool("LLM_RISK_FORENSICS_ENABLED", false),
		LLMConfidenceCommentaryEnabled: envOrBool("LLM_CONFIDENCE_COMMENTARY_ENABLED", false),
		LLMSectorAgentsEnabled:         envOrBool("LLM_SECTOR_AGENTS_ENABLED", false),
		SectorPredictionEnabled:        envOrBool("SECTOR_PREDICTION_ENABLED", false),
		EventPredictionEnabled:         envOrBool("ATLAS_EVENT_PREDICTION_ENABLED", false),
		AllowLiveBroker:                envOrBool("ATLAS_ALLOW_LIVE_BROKER", false),
		AllowHTTPBroker:                envOrBool("ATLAS_ALLOW_HTTP_BROKER", false),
		AllowRealSigner:                envOrBool("ATLAS_ALLOW_REAL_SIGNER", false),
		CharterMode:                    envOrBool("ATLAS_CHARTER_MODE", false),
		Stage3TasksEnabled:             envOrBool("STAGE3_TASKS_ENABLED", true),
		Stage3AlertsEnabled:            envOrBool("STAGE3_ALERTS_ENABLED", true),
	}
	validateMaturityFirstStart(cfg.MaturityFirstStart)
	return cfg
}

// validateMaturityFirstStart warns when ATLAS_MATURITY_FIRST_START is set but
// not parseable as RFC3339 or date-only, so a bad seed never silently resets
// the maturity clock to "now" on a fresh deployment.
func validateMaturityFirstStart(raw string) {
	seed := strings.TrimSpace(raw)
	if seed == "" {
		return
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if _, err := time.Parse(layout, seed); err == nil {
			return
		}
	}
	logging.Warn("config", "maturity_first_start_invalid",
		logging.FStr("value", seed),
		logging.FStr("expected", "RFC3339 (e.g. 2026-06-01T05:10:28Z) or date-only (2026-06-01)"))
}

func Normalize(cfg Config) Config {
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = constants.ReplayCSVPath
	}
	// Resolve relative SQLitePath against WorkDir so tests and subcommands
	// invoked from non-root directories still find the database.
	if cfg.SQLitePath != "" && !filepath.IsAbs(cfg.SQLitePath) && cfg.WorkDir != "" {
		cfg.SQLitePath = filepath.Join(cfg.WorkDir, cfg.SQLitePath)
	}
	return cfg
}

// GetReplayDataPath returns the current replay data path.
// Priority: 1) ATLAS_REPLAY_DATA_PATH env var, 2) VERSION file, 3) Normalize() default.
func GetReplayDataPath(workDir string) string {
	cfg := Load()
	cfg = Normalize(cfg)

	if v := os.Getenv("ATLAS_REPLAY_DATA_PATH"); v != "" {
		return v
	}

	versionFile := filepath.Join(workDir, "data", "replay", "VERSION")
	if data, err := os.ReadFile(versionFile); err == nil {
		if name := strings.TrimSpace(string(data)); name != "" {
			return filepath.Join(workDir, "data", "replay", name)
		}
	}

	return filepath.Join(workDir, cfg.ReplayDataPath)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// envOrKeychain reads from environment variable first,
// falling back to the system keychain. Currently delegates to envOr
// since keychain integration is not yet implemented.
func envOrKeychain(key string) string {
	return envOr(key, "")
}

// SafeKey returns a redacted version of an API key suitable for logging,
// preventing accidental key exposure in log output. Returns "(not set)"
// for empty keys. For keys ≤ 8 chars, returns "****" to avoid leaking
// the entire value. Otherwise returns "XXXX****YYY" (first 4 + last 3).
//
// Usage: logging.Info("config", "key_loaded", logging.FStr("key", config.SafeKey(cfg.FubonAPIKey)))
func SafeKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-3:]
}

func envOrInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		logging.Warn("config", "parse_int_failed", logging.FStr("key", key), logging.FStr("value", value), logging.FInt("fallback", fallback), logging.Err(err))
		return fallback
	}
	return n
}

func envOrBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

func envOrIntCSV(key string, fallback []int) []int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]int(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		parsed = append(parsed, n)
	}
	if len(parsed) == 0 {
		return append([]int(nil), fallback...)
	}
	return parsed
}

// GetSecret retrieves a secret from the environment or macOS Keychain.
// Use this for secrets not covered by the Config struct fields.
func GetSecret(key string) string {
	return envOrKeychain(key)
}

// resolveEnvFilePath finds the .env file to load.
// Priority: 1) ATLAS_ENV_FILE env var 2) local .env
// Note: ~/.config/atlas-go/.env is handled separately by loadUserEnvFile()
func resolveEnvFilePath() string {
	if p := os.Getenv("ATLAS_ENV_FILE"); p != "" {
		return p
	}
	if info, err := os.Stat(".env"); err == nil && info.Mode().IsRegular() {
		return ".env"
	}
	return ".env" // fallback: let loadEnvFile skip silently
}

// loadUserEnvFile loads ~/.config/atlas-go/.env with LookupEnv semantics
// so that t.Setenv always takes priority over user config files.
func loadUserEnvFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "atlas-go", ".env")
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return
	}
	loadWithLookupEnv(path)
}

// loadWithLookupEnv loads a .env file but never overrides an already-set env var.
// It uses os.LookupEnv so that t.Setenv always wins.
func loadWithLookupEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, ok := os.LookupEnv(key); !ok {
			if err := os.Setenv(key, value); err != nil {
				logging.Warn("config", "setenv_failed",
					logging.FStr("key", key),
					logging.Err(err))
			}
		}
	}
}

// loadEnvFile 从 .env 文件加载环境变量
func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return // .env 文件不存在时静默跳过
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Strip matching quotes (single or double) commonly used in .env files
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// 如果环境变量未设置，则使用 .env 中的值
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				logging.Warn("config", "setenv_failed",
					logging.FStr("key", key),
					logging.Err(err))
			}
		}
	}
}

func parseExtraPaths(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var paths []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
