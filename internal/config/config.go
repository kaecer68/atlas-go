package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	MarketDataProvider string
	PrimaryMarket      string
	ReplayMode         string
	AgentRegistryPath  string
	BaselinePolicyPath string
	LedgerDir          string
	ReplayDataPath     string
	ReplaySessionDate  string
	FugleAPIKey        string
	YahooEnabled       bool
}

func Load() Config {
	// 加载 .env 文件
	loadEnvFile(".env")

	return Config{
		MarketDataProvider: envOr("ATLAS_MARKET_DATA_PROVIDER", "twse"),
		PrimaryMarket:      envOr("ATLAS_PRIMARY_MARKET", "TW"),
		ReplayMode:         envOr("ATLAS_REPLAY_MODE", "daily"),
		AgentRegistryPath:  envOr("ATLAS_AGENT_REGISTRY_PATH", "configs/agents.json"),
		BaselinePolicyPath: envOr("ATLAS_BASELINE_POLICY_PATH", "data/state/baseline_policy.json"),
		LedgerDir:          envOr("ATLAS_LEDGER_DIR", "data/state"),
		ReplayDataPath:     envOr("ATLAS_REPLAY_DATA_PATH", "samples/replay/twse_stock_day_all_sample.csv"),
		ReplaySessionDate:  envOr("ATLAS_REPLAY_SESSION_DATE", "2026-03-26"),
		// 优先使用 FUGLE_API_KEY，其次 ATLAS_FUGLE_API_KEY
		FugleAPIKey:  envOrPriority("FUGLE_API_KEY", "ATLAS_FUGLE_API_KEY"),
		YahooEnabled: os.Getenv("ATLAS_YAHOO_ENABLED") == "true",
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrPriority(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
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

		// 如果环境变量未设置，则使用 .env 中的值
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
