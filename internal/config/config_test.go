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
		"ATLAS_MATURITY_FIRST_START",
		"ATLAS_BROKER_MODE", "ATLAS_BROKER_MAX_RETRIES", "ATLAS_BROKER_ADAPTER",
		"ATLAS_BROKER_API_BASE_URL", "ATLAS_BROKER_API_KEY", "ATLAS_BROKER_API_SECRET",
		"ATLAS_BROKER_HTTP_TIMEOUT_SEC", "ATLAS_BROKER_HTTP_ATTEMPTS", "ATLAS_BROKER_HTTP_RETRY_STATUS_CODES", "ATLAS_BROKER_MAX_CLOCK_SKEW_SEC", "ATLAS_BROKER_NONCE_TTL_SEC", "ATLAS_BROKER_NONCE_STORE", "ATLAS_BROKER_NONCE_STORE_PATH", "ATLAS_BROKER_NONCE_REDIS_URL", "ATLAS_BROKER_NONCE_REDIS_KEY_PREFIX", "ATLAS_BROKER_SIGNER", "ATLAS_BROKER_KEY_ID",
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

	if !cfg.YahooEnabled {
		t.Error("YahooEnabled should default to true (flipped 2026-06; PR #484 safeguard now exposed the silent-zero failure mode this default caused)")
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
	if cfg.MaturityFirstStart != "" {
		t.Errorf("MaturityFirstStart should default to empty, got %q", cfg.MaturityFirstStart)
	}
}

func TestLoad_BrokerAdapterDefaultFallbackWhenEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_BROKER_ADAPTER", "")

	cfg := Load()
	if cfg.BrokerAdapter != "guarded" {
		t.Errorf("BrokerAdapter = %q, want guarded when env empty", cfg.BrokerAdapter)
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
	t.Setenv("ATLAS_BROKER_ADAPTER", "mock")
	t.Setenv("ATLAS_BROKER_API_BASE_URL", "https://broker.example")
	t.Setenv("ATLAS_BROKER_API_KEY", "key-1")
	t.Setenv("ATLAS_BROKER_API_SECRET", "sec-1")
	t.Setenv("ATLAS_BROKER_HTTP_TIMEOUT_SEC", "9")
	t.Setenv("ATLAS_BROKER_HTTP_ATTEMPTS", "4")
	t.Setenv("ATLAS_BROKER_HTTP_RETRY_STATUS_CODES", "408,429,503")
	t.Setenv("ATLAS_BROKER_MAX_CLOCK_SKEW_SEC", "120")
	t.Setenv("ATLAS_BROKER_NONCE_TTL_SEC", "180")
	t.Setenv("ATLAS_BROKER_NONCE_STORE", "file")
	t.Setenv("ATLAS_BROKER_NONCE_STORE_PATH", "data/state/nonces.json")
	t.Setenv("ATLAS_BROKER_NONCE_REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("ATLAS_BROKER_NONCE_REDIS_KEY_PREFIX", "atlas:test:")
	t.Setenv("ATLAS_BROKER_SIGNER", "hmac-sha256")
	t.Setenv("ATLAS_BROKER_KEY_ID", "kid-01")

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

func TestLoad_MaturityFirstStartEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_MATURITY_FIRST_START", "2026-06-01T05:10:28Z")
	cfg := Load()
	if cfg.MaturityFirstStart != "2026-06-01T05:10:28Z" {
		t.Errorf("MaturityFirstStart = %q, want 2026-06-01T05:10:28Z", cfg.MaturityFirstStart)
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

func TestLoad_YahooEnabled_ExplicitFalseOptOut(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_YAHOO_ENABLED", "false")

	cfg := Load()
	if cfg.YahooEnabled {
		t.Error("YahooEnabled should be false when ATLAS_YAHOO_ENABLED=false (explicit opt-out)")
	}
}

func TestLoad_YahooEnabled_InvalidValueFallsBackToDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_YAHOO_ENABLED", "garbage")

	cfg := Load()
	if !cfg.YahooEnabled {
		t.Error("YahooEnabled should fall back to default (true) when env value is invalid")
	}
}

func TestLoad_YahooEnabled_AcceptsCommonTruthyTokens(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, val := range []string{"true", "TRUE", " 1 ", "yes", "on"} {
		t.Run("val="+val, func(t *testing.T) {
			t.Setenv("ATLAS_YAHOO_ENABLED", val)
			cfg := Load()
			if !cfg.YahooEnabled {
				t.Errorf("YahooEnabled should be true for env=%q, got false", val)
			}
		})
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

	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("FUGLE_API_KEY", "")
		cfg := Load()
		if cfg.FugleAPIKey != "" {
			t.Errorf("FugleAPIKey = %q, want empty (falls back to Keychain at runtime)", cfg.FugleAPIKey)
		}
	})
}

func TestLoadEnvFile_LoadsValues(t *testing.T) {
	dir := t.TempDir()

	// 清除對應環境變數，確保由 .env 檔填入
	t.Setenv("ATLAS_MARKET_DATA_PROVIDER", "")
	t.Setenv("ATLAS_REPLAY_MODE", "")

	envContent := "ATLAS_MARKET_DATA_PROVIDER=from-env-file\nATLAS_REPLAY_MODE=weekly\n"
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	loadEnvFile(path)

	if got := os.Getenv("ATLAS_MARKET_DATA_PROVIDER"); got != "from-env-file" {
		t.Errorf("ATLAS_MARKET_DATA_PROVIDER = %q, want from-env-file", got)
	}
	if got := os.Getenv("ATLAS_REPLAY_MODE"); got != "weekly" {
		t.Errorf("ATLAS_REPLAY_MODE = %q, want weekly", got)
	}
}

func TestLoadEnvFile_SkipsComments(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("ATLAS_PRIMARY_MARKET", "")

	// 確保帶 # 的行不被當作設定值解析
	envContent := "# This is a comment\nATLAS_PRIMARY_MARKET=JP\n"
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	loadEnvFile(path)
	if got := os.Getenv("ATLAS_PRIMARY_MARKET"); got != "JP" {
		t.Errorf("ATLAS_PRIMARY_MARKET = %q, want JP", got)
	}
}

// TestLoad_SkipsEnvFilesUnderTest is the M7 regression guard: Load() must
// never read .env or ~/.config/atlas-go/.env while running under `go test`,
// so a developer's local config cannot flip tests onto a postgres backend or
// leak secrets into test logs.
func TestLoad_SkipsEnvFilesUnderTest(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	t.Setenv("ATLAS_PRIMARY_MARKET", "")

	envContent := "ATLAS_PRIMARY_MARKET=JP\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := Load()
	if cfg.PrimaryMarket == "JP" {
		t.Errorf("Load() read .env under test; config loading must be hermetic (M7)")
	}
}

// TestLoad_SkipsUserEnvFileUnderTest is the M7 regression guard for the user
// config file specifically: a developer's ~/.config/atlas-go/.env (e.g.
// ATLAS_STORE_BACKEND=postgres) must not leak into `go test` process state.
func TestLoad_SkipsUserEnvFileUnderTest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("ATLAS_STORE_BACKEND", "")

	userEnv := filepath.Join(home, ".config", "atlas-go", ".env")
	if err := os.MkdirAll(filepath.Dir(userEnv), 0o755); err != nil {
		t.Fatalf("mkdir user config dir: %v", err)
	}
	if err := os.WriteFile(userEnv, []byte("ATLAS_STORE_BACKEND=postgres\n"), 0o644); err != nil {
		t.Fatalf("write user .env: %v", err)
	}

	cfg := Load()
	if cfg.StoreBackend == "postgres" {
		t.Errorf("Load() read ~/.config/atlas-go/.env under test; config loading must be hermetic (M7)")
	}
}

// TestLoad_StockpickerExpectDB_EnvOr pins the M12 guard knob: default is
// empty (sqlite path needs no guard); set via ATLAS_STOCKPICKER_EXPECT_DB
// for the postgres path (prod sets "atlas").
func TestLoad_StockpickerExpectDB_EnvOr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ATLAS_STOCKPICKER_EXPECT_DB", "")
	if cfg := Load(); cfg.StockpickerExpectDB != "" {
		t.Errorf("StockpickerExpectDB = %q, want default empty", cfg.StockpickerExpectDB)
	}
	t.Setenv("ATLAS_STOCKPICKER_EXPECT_DB", "atlas")
	if cfg := Load(); cfg.StockpickerExpectDB != "atlas" {
		t.Errorf("StockpickerExpectDB = %q, want atlas (from env)", cfg.StockpickerExpectDB)
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
	if cfg.ReplayMode != "process-env-value" {
		t.Errorf("ReplayMode = %q, want process-env-value (process env should win)", cfg.ReplayMode)
	}
}

func TestGetReplayDataPath_Default(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_REPLAY_DATA_PATH", "")

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

	t.Setenv("ATLAS_REPLAY_DATA_PATH", "")
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

	t.Setenv("ATLAS_REPLAY_DATA_PATH", "/custom/path/replay.csv")
	got := GetReplayDataPath(tmpDir)
	want := "/custom/path/replay.csv"
	if got != want {
		t.Errorf("GetReplayDataPath = %q, want %q", got, want)
	}
}

func TestSafeKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"empty", "", "(not set)"},
		{"short", "ab", "****"},
		{"exactly_eight", "12345678", "****"},
		{"long_sk_key", "sk-abc123xyz789", "sk-a****789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeKey(tt.key)
			if got != tt.want {
				t.Errorf("SafeKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoad_Stage3FlagsDefaultTrue(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("STAGE3_TASKS_ENABLED", "")
	t.Setenv("STAGE3_ALERTS_ENABLED", "")

	cfg := Load()
	if !cfg.Stage3TasksEnabled {
		t.Errorf("STAGE3_TASKS_ENABLED default should be true (opt-out), got false")
	}
	if !cfg.Stage3AlertsEnabled {
		t.Errorf("STAGE3_ALERTS_ENABLED default should be true (opt-out), got false")
	}
}

func TestLoad_Stage3FlagsEnvOptOut(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("STAGE3_TASKS_ENABLED", "false")
	t.Setenv("STAGE3_ALERTS_ENABLED", "false")

	cfg := Load()
	if cfg.Stage3TasksEnabled {
		t.Errorf("STAGE3_TASKS_ENABLED=false did not take effect")
	}
	if cfg.Stage3AlertsEnabled {
		t.Errorf("STAGE3_ALERTS_ENABLED=false did not take effect")
	}
}
