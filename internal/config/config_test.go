package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	env := newMapEnvSource()
	cfg := loadWithSource(env)

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
	if cfg.BrokerAdapter != "guarded" {
		t.Errorf("BrokerAdapter should default to guarded, got %q", cfg.BrokerAdapter)
	}
	if cfg.BrokerHTTPTimeoutS != 5 {
		t.Errorf("BrokerHTTPTimeoutS should default to 5, got %d", cfg.BrokerHTTPTimeoutS)
	}
	if cfg.BrokerHTTPAttempts != 2 {
		t.Errorf("BrokerHTTPAttempts should default to 2, got %d", cfg.BrokerHTTPAttempts)
	}
	if len(cfg.BrokerHTTPRetryStatusCodes) == 0 {
		t.Error("BrokerHTTPRetryStatusCodes should not be empty by default")
	}
	if cfg.BrokerMaxClockSkewS != 300 {
		t.Errorf("BrokerMaxClockSkewS should default to 300, got %d", cfg.BrokerMaxClockSkewS)
	}
	if cfg.BrokerNonceTTLS != 300 {
		t.Errorf("BrokerNonceTTLS should default to 300, got %d", cfg.BrokerNonceTTLS)
	}
	if cfg.BrokerNonceStore != "memory" {
		t.Errorf("BrokerNonceStore should default to memory, got %q", cfg.BrokerNonceStore)
	}
	if cfg.BrokerNonceStorePath != "" {
		t.Errorf("BrokerNonceStorePath should default to empty, got %q", cfg.BrokerNonceStorePath)
	}
	if cfg.BrokerNonceRedisURL != "" {
		t.Errorf("BrokerNonceRedisURL should default to empty, got %q", cfg.BrokerNonceRedisURL)
	}
	if cfg.BrokerNonceRedisKeyPrefix != "atlas:nonce:" {
		t.Errorf("BrokerNonceRedisKeyPrefix should default to atlas:nonce:, got %q", cfg.BrokerNonceRedisKeyPrefix)
	}
	if cfg.BrokerSigner != "placeholder" {
		t.Errorf("BrokerSigner should default to placeholder, got %q", cfg.BrokerSigner)
	}
	if cfg.BrokerKeyID != "" {
		t.Errorf("BrokerKeyID should default to empty, got %q", cfg.BrokerKeyID)
	}
}

func TestLoad_BrokerAdapterDefaultFallbackWhenEmpty(t *testing.T) {
	env := newMapEnvSource()
	env.Setenv("ATLAS_BROKER_ADAPTER", "")

	cfg := loadWithSource(env)
	if cfg.BrokerAdapter != "guarded" {
		t.Errorf("BrokerAdapter = %q, want guarded when env empty", cfg.BrokerAdapter)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	env := newMapEnvSource()
	env.Setenv("ATLAS_MARKET_DATA_PROVIDER", "fugle")
	env.Setenv("ATLAS_PRIMARY_MARKET", "US")
	env.Setenv("ATLAS_REPLAY_MODE", "tick")
	env.Setenv("ATLAS_YAHOO_ENABLED", "true")
	env.Setenv("ATLAS_BROKER_MODE", "dry-run")
	env.Setenv("ATLAS_BROKER_MAX_RETRIES", "3")
	env.Setenv("ATLAS_BROKER_ADAPTER", "mock")
	env.Setenv("ATLAS_BROKER_API_BASE_URL", "https://broker.example")
	env.Setenv("ATLAS_BROKER_API_KEY", "key-1")
	env.Setenv("ATLAS_BROKER_API_SECRET", "sec-1")
	env.Setenv("ATLAS_BROKER_HTTP_TIMEOUT_SEC", "9")
	env.Setenv("ATLAS_BROKER_HTTP_ATTEMPTS", "4")
	env.Setenv("ATLAS_BROKER_HTTP_RETRY_STATUS_CODES", "408,429,503")
	env.Setenv("ATLAS_BROKER_MAX_CLOCK_SKEW_SEC", "120")
	env.Setenv("ATLAS_BROKER_NONCE_TTL_SEC", "180")
	env.Setenv("ATLAS_BROKER_NONCE_STORE", "file")
	env.Setenv("ATLAS_BROKER_NONCE_STORE_PATH", "data/state/nonces.json")
	env.Setenv("ATLAS_BROKER_NONCE_REDIS_URL", "redis://localhost:6379/0")
	env.Setenv("ATLAS_BROKER_NONCE_REDIS_KEY_PREFIX", "atlas:test:")
	env.Setenv("ATLAS_BROKER_SIGNER", "hmac-sha256")
	env.Setenv("ATLAS_BROKER_KEY_ID", "kid-01")

	cfg := loadWithSource(env)

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
	if cfg.BrokerAdapter != "mock" {
		t.Errorf("BrokerAdapter = %q, want mock", cfg.BrokerAdapter)
	}
	if cfg.BrokerAPIBaseURL != "https://broker.example" {
		t.Errorf("BrokerAPIBaseURL = %q, want https://broker.example", cfg.BrokerAPIBaseURL)
	}
	if cfg.BrokerAPIKey != "key-1" {
		t.Errorf("BrokerAPIKey = %q, want key-1", cfg.BrokerAPIKey)
	}
	if cfg.BrokerAPISecret != "sec-1" {
		t.Errorf("BrokerAPISecret = %q, want sec-1", cfg.BrokerAPISecret)
	}
	if cfg.BrokerHTTPTimeoutS != 9 {
		t.Errorf("BrokerHTTPTimeoutS = %d, want 9", cfg.BrokerHTTPTimeoutS)
	}
	if cfg.BrokerHTTPAttempts != 4 {
		t.Errorf("BrokerHTTPAttempts = %d, want 4", cfg.BrokerHTTPAttempts)
	}
	if len(cfg.BrokerHTTPRetryStatusCodes) != 3 || cfg.BrokerHTTPRetryStatusCodes[0] != 408 || cfg.BrokerHTTPRetryStatusCodes[1] != 429 || cfg.BrokerHTTPRetryStatusCodes[2] != 503 {
		t.Errorf("BrokerHTTPRetryStatusCodes = %v, want [408 429 503]", cfg.BrokerHTTPRetryStatusCodes)
	}
	if cfg.BrokerMaxClockSkewS != 120 {
		t.Errorf("BrokerMaxClockSkewS = %d, want 120", cfg.BrokerMaxClockSkewS)
	}
	if cfg.BrokerNonceTTLS != 180 {
		t.Errorf("BrokerNonceTTLS = %d, want 180", cfg.BrokerNonceTTLS)
	}
	if cfg.BrokerNonceStore != "file" {
		t.Errorf("BrokerNonceStore = %q, want file", cfg.BrokerNonceStore)
	}
	if cfg.BrokerNonceStorePath != "data/state/nonces.json" {
		t.Errorf("BrokerNonceStorePath = %q, want data/state/nonces.json", cfg.BrokerNonceStorePath)
	}
	if cfg.BrokerNonceRedisURL != "redis://localhost:6379/0" {
		t.Errorf("BrokerNonceRedisURL = %q, want redis://localhost:6379/0", cfg.BrokerNonceRedisURL)
	}
	if cfg.BrokerNonceRedisKeyPrefix != "atlas:test:" {
		t.Errorf("BrokerNonceRedisKeyPrefix = %q, want atlas:test:", cfg.BrokerNonceRedisKeyPrefix)
	}
	if cfg.BrokerSigner != "hmac-sha256" {
		t.Errorf("BrokerSigner = %q, want hmac-sha256", cfg.BrokerSigner)
	}
	if cfg.BrokerKeyID != "kid-01" {
		t.Errorf("BrokerKeyID = %q, want kid-01", cfg.BrokerKeyID)
	}
}

func TestLoad_BrokerMaxRetriesInvalidFallback(t *testing.T) {
	env := newMapEnvSource()
	env.Setenv("ATLAS_BROKER_MAX_RETRIES", "invalid")

	cfg := loadWithSource(env)
	if cfg.BrokerMaxRetries != 1 {
		t.Errorf("BrokerMaxRetries = %d, want 1 when invalid", cfg.BrokerMaxRetries)
	}
}

func TestLoad_FugleAPIKeyPriority(t *testing.T) {
	t.Run("FUGLE_API_KEY takes priority", func(t *testing.T) {
		env := newMapEnvSource()
		env.Setenv("FUGLE_API_KEY", "primary-key")
		env.Setenv("ATLAS_FUGLE_API_KEY", "secondary-key")
		cfg := loadWithSource(env)
		if cfg.FugleAPIKey != "primary-key" {
			t.Errorf("FugleAPIKey = %q, want primary-key", cfg.FugleAPIKey)
		}
	})

	t.Run("returns empty when not set", func(t *testing.T) {
		env := newMapEnvSource()
		env.Setenv("FUGLE_API_KEY", "")
		cfg := loadWithSource(env)
		if cfg.FugleAPIKey != "" {
			t.Errorf("FugleAPIKey = %q, want empty (falls back to Keychain at runtime)", cfg.FugleAPIKey)
		}
	})
}

func TestLoad_EnvFile(t *testing.T) {
	dir := t.TempDir()

	for _, k := range []string{"ATLAS_MARKET_DATA_PROVIDER", "ATLAS_REPLAY_MODE", "ATLAS_ENV_FILE"} {
		os.Unsetenv(k)
	}

	envContent := "ATLAS_MARKET_DATA_PROVIDER=from-env-file\nATLAS_REPLAY_MODE=weekly\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

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

	os.Unsetenv("ATLAS_PRIMARY_MARKET")
	os.Unsetenv("ATLAS_ENV_FILE")

	envContent := "# This is a comment\nATLAS_PRIMARY_MARKET=JP\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg := Load()
	if cfg.PrimaryMarket != "JP" {
		t.Errorf("PrimaryMarket = %q, want JP", cfg.PrimaryMarket)
	}
}

func TestLoad_EnvFileDoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()

	envContent := "ATLAS_REPLAY_MODE=env-file-value\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.Setenv("ATLAS_REPLAY_MODE", "process-env-value")
	os.Unsetenv("ATLAS_ENV_FILE")
	defer os.Unsetenv("ATLAS_REPLAY_MODE")

	cfg := Load()
	if cfg.ReplayMode != "process-env-value" {
		t.Errorf("ReplayMode = %q, want process-env-value (process env should win)", cfg.ReplayMode)
	}
}

func TestGetReplayDataPath_Default(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("ATLAS_REPLAY_DATA_PATH", "")
	defer os.Unsetenv("ATLAS_REPLAY_DATA_PATH")

	got := GetReplayDataPath(tmpDir)
	want := filepath.Join(tmpDir, "data", "replay", "tw_extended_90days.csv")
	if got != want {
		t.Errorf("GetReplayDataPath = %q, want %q", got, want)
	}
}

func TestGetReplayDataPath_VERSIONFile(t *testing.T) {
	tmpDir := t.TempDir()
	replayDir := filepath.Join(tmpDir, "data", "replay")
	os.MkdirAll(replayDir, 0o755)

	if err := os.WriteFile(filepath.Join(replayDir, "VERSION"), []byte("merged.csv\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	os.Setenv("ATLAS_REPLAY_DATA_PATH", "")
	defer os.Unsetenv("ATLAS_REPLAY_DATA_PATH")

	got := GetReplayDataPath(tmpDir)
	want := filepath.Join(tmpDir, "data", "replay", "merged.csv")
	if got != want {
		t.Errorf("GetReplayDataPath = %q, want %q", got, want)
	}
}

func TestGetReplayDataPath_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	replayDir := filepath.Join(tmpDir, "data", "replay")
	os.MkdirAll(replayDir, 0o755)

	if err := os.WriteFile(filepath.Join(replayDir, "VERSION"), []byte("merged.csv\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	os.Setenv("ATLAS_REPLAY_DATA_PATH", "/custom/path/replay.csv")
	defer os.Unsetenv("ATLAS_REPLAY_DATA_PATH")

	got := GetReplayDataPath(tmpDir)
	want := "/custom/path/replay.csv"
	if got != want {
		t.Errorf("GetReplayDataPath = %q, want %q", got, want)
	}
}
