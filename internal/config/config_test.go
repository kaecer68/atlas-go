package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// 清除可能影響預設值的環境變數
	envKeys := []string{
		"ATLAS_MARKET_DATA_PROVIDER", "ATLAS_PRIMARY_MARKET", "ATLAS_REPLAY_MODE",
		"ATLAS_AGENT_REGISTRY_PATH", "ATLAS_BASELINE_POLICY_PATH", "ATLAS_LEDGER_DIR",
		"ATLAS_REPLAY_DATA_PATH", "ATLAS_REPLAY_SESSION_DATE",
		"FUGLE_API_KEY", "ATLAS_FUGLE_API_KEY", "ATLAS_YAHOO_ENABLED",
		"ATLAS_BROKER_MODE", "ATLAS_BROKER_MAX_RETRIES",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
	}

	// 執行測試時隔離 .env 檔案
	t.Chdir(t.TempDir())

	cfg := Load()

	checks := map[string]string{
		"MarketDataProvider": cfg.MarketDataProvider,
		"PrimaryMarket":      cfg.PrimaryMarket,
		"ReplayMode":         cfg.ReplayMode,
	}
	want := map[string]string{
		"MarketDataProvider": "twse",
		"PrimaryMarket":      "TW",
		"ReplayMode":         "daily",
	}
	for field, got := range checks {
		if got != want[field] {
			t.Errorf("%s: got %q, want %q", field, got, want[field])
		}
	}

	if cfg.YahooEnabled {
		t.Error("YahooEnabled should default to false")
	}
	if cfg.FugleAPIKey != "" {
		t.Errorf("FugleAPIKey should default to empty, got %q", cfg.FugleAPIKey)
	}
	if cfg.BrokerMode != "dry-run" {
		t.Errorf("BrokerMode should default to dry-run, got %q", cfg.BrokerMode)
	}
	if cfg.BrokerMaxRetries != 1 {
		t.Errorf("BrokerMaxRetries should default to 1, got %d", cfg.BrokerMaxRetries)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_MARKET_DATA_PROVIDER", "fugle")
	t.Setenv("ATLAS_PRIMARY_MARKET", "US")
	t.Setenv("ATLAS_REPLAY_MODE", "tick")
	t.Setenv("ATLAS_YAHOO_ENABLED", "true")
	t.Setenv("ATLAS_BROKER_MODE", "dry-run")
	t.Setenv("ATLAS_BROKER_MAX_RETRIES", "3")

	cfg := Load()

	if cfg.MarketDataProvider != "fugle" {
		t.Errorf("MarketDataProvider = %q, want fugle", cfg.MarketDataProvider)
	}
	if cfg.PrimaryMarket != "US" {
		t.Errorf("PrimaryMarket = %q, want US", cfg.PrimaryMarket)
	}
	if cfg.ReplayMode != "tick" {
		t.Errorf("ReplayMode = %q, want tick", cfg.ReplayMode)
	}
	if !cfg.YahooEnabled {
		t.Error("YahooEnabled should be true")
	}
	if cfg.BrokerMode != "dry-run" {
		t.Errorf("BrokerMode = %q, want dry-run", cfg.BrokerMode)
	}
	if cfg.BrokerMaxRetries != 3 {
		t.Errorf("BrokerMaxRetries = %d, want 3", cfg.BrokerMaxRetries)
	}
}

func TestLoad_BrokerMaxRetriesInvalidFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_BROKER_MAX_RETRIES", "invalid")

	cfg := Load()
	if cfg.BrokerMaxRetries != 1 {
		t.Errorf("BrokerMaxRetries = %d, want 1 when invalid", cfg.BrokerMaxRetries)
	}
}

func TestLoad_FugleAPIKeyPriority(t *testing.T) {
	t.Chdir(t.TempDir())

	t.Run("FUGLE_API_KEY takes priority", func(t *testing.T) {
		t.Setenv("FUGLE_API_KEY", "primary-key")
		t.Setenv("ATLAS_FUGLE_API_KEY", "secondary-key")
		cfg := Load()
		if cfg.FugleAPIKey != "primary-key" {
			t.Errorf("FugleAPIKey = %q, want primary-key", cfg.FugleAPIKey)
		}
	})

	t.Run("falls back to ATLAS_FUGLE_API_KEY", func(t *testing.T) {
		t.Setenv("FUGLE_API_KEY", "")
		t.Setenv("ATLAS_FUGLE_API_KEY", "fallback-key")
		cfg := Load()
		if cfg.FugleAPIKey != "fallback-key" {
			t.Errorf("FugleAPIKey = %q, want fallback-key", cfg.FugleAPIKey)
		}
	})
}

func TestLoad_EnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// 清除對應環境變數，確保由 .env 檔填入
	t.Setenv("ATLAS_MARKET_DATA_PROVIDER", "")
	t.Setenv("ATLAS_REPLAY_MODE", "")

	envContent := "ATLAS_MARKET_DATA_PROVIDER=from-env-file\nATLAS_REPLAY_MODE=weekly\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := Load()

	if cfg.MarketDataProvider != "from-env-file" {
		t.Errorf("MarketDataProvider = %q, want from-env-file", cfg.MarketDataProvider)
	}
	if cfg.ReplayMode != "weekly" {
		t.Errorf("ReplayMode = %q, want weekly", cfg.ReplayMode)
	}
}

func TestLoad_EnvFileSkipsComments(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	t.Setenv("ATLAS_PRIMARY_MARKET", "")

	// 確保帶 # 的行不被當作設定值解析
	envContent := "# This is a comment\nATLAS_PRIMARY_MARKET=JP\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := Load()
	if cfg.PrimaryMarket != "JP" {
		t.Errorf("PrimaryMarket = %q, want JP", cfg.PrimaryMarket)
	}
}

func TestLoad_EnvFileDoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	t.Setenv("ATLAS_REPLAY_MODE", "process-env-value")

	envContent := "ATLAS_REPLAY_MODE=env-file-value\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := Load()
	// 已存在的 process env 不應被 .env 覆蓋
	if cfg.ReplayMode != "process-env-value" {
		t.Errorf("ReplayMode = %q, want process-env-value (process env should win)", cfg.ReplayMode)
	}
}
