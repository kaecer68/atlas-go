package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/logging"
)

type Config struct {
	WorkDir                    string
	DatabaseURL                string
	MigrationsPath             string
	MarketDataProvider         string
	PrimaryMarket              string
	ReplayMode                 string
	AgentRegistryPath          string
	BaselinePolicyPath         string
	ParametersConfigPath       string
	LedgerDir                  string
	StoreBackend               string // "jsonl" (default) or "sqlite" — ATLAS_STORE_BACKEND
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
}

func Load() Config {
	// 加载 .env 文件 — 优先使用 ATLAS_ENV_FILE 指定的路径，
	// 然后依次尝试 .env、~/.config/atlas-go/.env
	loadEnvFile(resolveEnvFilePath())

	return Config{
		WorkDir:                    envOr("ATLAS_WORK_DIR", "."),
		DatabaseURL:                envOr("DATABASE_URL", ""),
		MigrationsPath:             envOr("ATLAS_MIGRATIONS_PATH", "sql/migrations"),
		MarketDataProvider:         envOr("ATLAS_MARKET_DATA_PROVIDER", "twse"),
		PrimaryMarket:              envOr("ATLAS_PRIMARY_MARKET", "TW"),
		ReplayMode:                 envOr("ATLAS_REPLAY_MODE", "daily"),
		AgentRegistryPath:          envOr("ATLAS_AGENT_REGISTRY_PATH", "configs/agents.json"),
		BaselinePolicyPath:         envOr("ATLAS_BASELINE_POLICY_PATH", "data/state/baseline_policy.json"),
		ParametersConfigPath:       envOr("ATLAS_PARAMETERS_CONFIG_PATH", "configs/parameters.json"),
		LedgerDir:                  envOr("ATLAS_LEDGER_DIR", "data/state"),
		StoreBackend:               envOr("ATLAS_STORE_BACKEND", "jsonl"),
		SQLitePath:                 envOr("ATLAS_SQLITE_PATH", "data/state/atlas.db"),
		ReplayDataPath:             envOr("ATLAS_REPLAY_DATA_PATH", "samples/replay/twse_stock_day_all_sample.csv"),
		ReplaySessionDate:          envOr("ATLAS_REPLAY_SESSION_DATE", ""),
		FubonAPIKey:                envOrKeychain("FUBON_API_KEY", ""),
		FugleAPIKey:                envOrKeychain("FUGLE_API_KEY", ""),
		FinMindAPIKey:              envOrKeychain("FINMIND_API_KEY", ""),
		YahooEnabled:               os.Getenv("ATLAS_YAHOO_ENABLED") == "true",
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
	}
}

func Normalize(cfg Config) Config {
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
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
func envOrKeychain(key, fallback string) string {
	return envOr(key, fallback)
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
	return envOrKeychain(key, "")
}

// resolveEnvFilePath 返回要加载的 .env 文件路径。
// 优先级：1) ATLAS_ENV_FILE 环境变量 2) 当前目录 .env 3) ~/.config/atlas-go/.env
func resolveEnvFilePath() string {
	if p := os.Getenv("ATLAS_ENV_FILE"); p != "" {
		return p
	}
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".config", "atlas-go", ".env")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ".env" // fallback: 让 loadEnvFile 静默跳过
}

// loadEnvFile 从 .env 文件加载环境变量
func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return // .env 文件不存在时静默跳过
	}
	defer file.Close()

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
			os.Setenv(key, value)
		}
	}
}
