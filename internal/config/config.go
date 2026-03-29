package config

import "os"

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
	return Config{
		MarketDataProvider: envOr("ATLAS_MARKET_DATA_PROVIDER", "twse"),
		PrimaryMarket:      envOr("ATLAS_PRIMARY_MARKET", "TW"),
		ReplayMode:         envOr("ATLAS_REPLAY_MODE", "daily"),
		AgentRegistryPath:  envOr("ATLAS_AGENT_REGISTRY_PATH", "configs/agents.json"),
		BaselinePolicyPath: envOr("ATLAS_BASELINE_POLICY_PATH", "data/state/baseline_policy.json"),
		LedgerDir:          envOr("ATLAS_LEDGER_DIR", "data/state"),
		ReplayDataPath:     envOr("ATLAS_REPLAY_DATA_PATH", "samples/replay/twse_stock_day_all_sample.csv"),
		ReplaySessionDate:  envOr("ATLAS_REPLAY_SESSION_DATE", "2026-03-26"),
		FugleAPIKey:        os.Getenv("ATLAS_FUGLE_API_KEY"),
		YahooEnabled:       os.Getenv("ATLAS_YAHOO_ENABLED") == "true",
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
