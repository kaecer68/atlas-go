package bootstrap

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
)

type BrokerOverrides struct {
	Mode                string
	Adapter             string
	Signer              string
	KeyID               string
	RetryStatusCodes    string
	MaxRetries          int
	MaxClockSkewSec     int
	NonceTTLSec         int
	NonceStore          string
	NonceStorePath      string
	NonceRedisURL       string
	NonceRedisKeyPrefix string
	AllowLiveBroker     bool
	AllowHTTPBroker     bool
	AllowRealSigner     bool
}

func ApplyBrokerConfig(cfg *config.Config, o BrokerOverrides) error {
	applyBrokerOverrides(cfg, o)
	normalizeBrokerStrings(cfg)
	if err := validateBrokerEnums(cfg); err != nil {
		return fmt.Errorf("validate broker enums: %w", err)
	}
	if err := validateBrokerLiveMode(cfg, o.AllowLiveBroker, o.AllowHTTPBroker, o.AllowRealSigner); err != nil {
		return fmt.Errorf("validate broker live mode: %w", err)
	}
	if err := validateBrokerRetryConfig(cfg); err != nil {
		return fmt.Errorf("validate broker retry config: %w", err)
	}
	if err := validateBrokerNonceConfig(cfg); err != nil {
		return fmt.Errorf("validate broker nonce config: %w", err)
	}
	return nil
}

func applyBrokerOverrides(cfg *config.Config, o BrokerOverrides) {
	if o.Mode != "" {
		cfg.BrokerMode = o.Mode
	}
	if o.Adapter != "" {
		cfg.BrokerAdapter = o.Adapter
	}
	if o.Signer != "" {
		cfg.BrokerSigner = o.Signer
	}
	if o.KeyID != "" {
		cfg.BrokerKeyID = o.KeyID
	}
	if o.RetryStatusCodes != "" {
		cfg.BrokerHTTPRetryStatusCodes = parseStatusCodeCSV(o.RetryStatusCodes, cfg.BrokerHTTPRetryStatusCodes)
	}
	if o.MaxRetries >= 0 {
		cfg.BrokerMaxRetries = o.MaxRetries
	}
	if o.MaxClockSkewSec >= 0 {
		cfg.BrokerMaxClockSkewS = o.MaxClockSkewSec
	}
	if o.NonceTTLSec >= 0 {
		cfg.BrokerNonceTTLS = o.NonceTTLSec
	}
	if o.NonceStore != "" {
		cfg.BrokerNonceStore = o.NonceStore
	}
	if o.NonceStorePath != "" {
		cfg.BrokerNonceStorePath = o.NonceStorePath
	}
	if o.NonceRedisURL != "" {
		cfg.BrokerNonceRedisURL = o.NonceRedisURL
	}
	if o.NonceRedisKeyPrefix != "" {
		cfg.BrokerNonceRedisKeyPrefix = o.NonceRedisKeyPrefix
	}
}

func normalizeBrokerStrings(cfg *config.Config) {
	cfg.BrokerMode = strings.TrimSpace(strings.ToLower(cfg.BrokerMode))
	if cfg.BrokerMode == "" {
		cfg.BrokerMode = "dry-run"
	}
	cfg.BrokerAdapter = strings.TrimSpace(strings.ToLower(cfg.BrokerAdapter))
	if cfg.BrokerAdapter == "" {
		cfg.BrokerAdapter = "guarded"
	}
	cfg.BrokerSigner = strings.TrimSpace(strings.ToLower(cfg.BrokerSigner))
	if cfg.BrokerSigner == "" {
		cfg.BrokerSigner = "placeholder"
	}
	cfg.BrokerKeyID = strings.TrimSpace(cfg.BrokerKeyID)
}

func validateBrokerEnums(cfg *config.Config) error {
	if cfg.BrokerAdapter != "guarded" && cfg.BrokerAdapter != "mock" && cfg.BrokerAdapter != "http" {
		return fmt.Errorf("unsupported broker adapter %q (allowed: guarded, mock, http)", cfg.BrokerAdapter)
	}
	if cfg.BrokerSigner != "placeholder" && cfg.BrokerSigner != "hmac-sha256" {
		return fmt.Errorf("unsupported broker signer %q (allowed: placeholder, hmac-sha256)", cfg.BrokerSigner)
	}
	return nil
}

func validateBrokerLiveMode(cfg *config.Config, allowLiveBroker bool, allowHTTPBroker bool, allowRealSigner bool) error {
	switch cfg.BrokerMode {
	case "dry-run", "paper":
		return nil
	case "live":
		if !allowLiveBroker {
			return fmt.Errorf("broker mode %q is disabled by default; pass -allow-live-broker to enable", cfg.BrokerMode)
		}
		if cfg.BrokerAdapter == "http" && !allowHTTPBroker {
			return fmt.Errorf("broker adapter %q is disabled by default in live mode; pass -allow-http-broker to enable", cfg.BrokerAdapter)
		}
		if cfg.BrokerAdapter == "http" && cfg.BrokerSigner != "placeholder" && !allowRealSigner {
			return fmt.Errorf("broker signer %q is disabled by default for http adapter; pass -allow-real-signer to enable", cfg.BrokerSigner)
		}
		if cfg.BrokerAdapter == "http" && cfg.BrokerSigner != "placeholder" && cfg.BrokerKeyID == "" {
			return fmt.Errorf("broker key id is required when using signer %q with http adapter", cfg.BrokerSigner)
		}
		return nil
	default:
		return fmt.Errorf("unsupported broker mode %q (allowed: dry-run, paper, live)", cfg.BrokerMode)
	}
}

func validateBrokerRetryConfig(cfg *config.Config) error {
	if cfg.BrokerMaxRetries < 0 {
		return fmt.Errorf("broker max retries must be >= 0, got %d", cfg.BrokerMaxRetries)
	}
	if len(cfg.BrokerHTTPRetryStatusCodes) == 0 {
		cfg.BrokerHTTPRetryStatusCodes = []int{408, 425, 429, 500, 502, 503, 504}
	}
	for _, code := range cfg.BrokerHTTPRetryStatusCodes {
		if code < 400 || code > 599 {
			return fmt.Errorf("broker retry status code must be 4xx/5xx, got %d", code)
		}
	}
	if cfg.BrokerMaxClockSkewS < 0 {
		return fmt.Errorf("broker max clock skew must be >= 0, got %d", cfg.BrokerMaxClockSkewS)
	}
	return nil
}

func validateBrokerNonceConfig(cfg *config.Config) error {
	if cfg.BrokerNonceTTLS == 0 {
		cfg.BrokerNonceTTLS = 300
	}
	if cfg.BrokerNonceTTLS < 0 {
		return fmt.Errorf("broker nonce ttl must be >= 0, got %d", cfg.BrokerNonceTTLS)
	}
	cfg.BrokerNonceStore = strings.TrimSpace(strings.ToLower(cfg.BrokerNonceStore))
	if cfg.BrokerNonceStore == "" {
		cfg.BrokerNonceStore = "memory"
	}
	if cfg.BrokerNonceStore != "memory" && cfg.BrokerNonceStore != "file" && cfg.BrokerNonceStore != "redis" {
		return fmt.Errorf("unsupported broker nonce store %q (allowed: memory, file, redis)", cfg.BrokerNonceStore)
	}
	cfg.BrokerNonceStorePath = strings.TrimSpace(cfg.BrokerNonceStorePath)
	defaultedNonceStorePath := false
	if cfg.BrokerNonceStore == "file" && cfg.BrokerNonceStorePath == "" {
		ledgerDir := strings.TrimSpace(cfg.LedgerDir)
		if ledgerDir == "" {
			ledgerDir = "data/state"
		}
		cfg.BrokerNonceStorePath = filepath.Join(ledgerDir, "broker-nonce-replay.json")
		defaultedNonceStorePath = true
	}
	if cfg.BrokerNonceStore == "file" && !defaultedNonceStorePath && !filepath.IsAbs(cfg.BrokerNonceStorePath) {
		ledgerDir := strings.TrimSpace(cfg.LedgerDir)
		if ledgerDir == "" {
			ledgerDir = "data/state"
		}
		cfg.BrokerNonceStorePath = filepath.Join(ledgerDir, cfg.BrokerNonceStorePath)
	}
	cfg.BrokerNonceRedisURL = strings.TrimSpace(cfg.BrokerNonceRedisURL)
	cfg.BrokerNonceRedisKeyPrefix = strings.TrimSpace(cfg.BrokerNonceRedisKeyPrefix)
	if cfg.BrokerNonceRedisKeyPrefix == "" {
		cfg.BrokerNonceRedisKeyPrefix = "atlas:nonce:"
	}
	if cfg.BrokerNonceStore == "redis" && cfg.BrokerNonceRedisURL == "" {
		return fmt.Errorf("broker nonce redis url is required when broker nonce store is redis")
	}
	return nil
}

func parseStatusCodeCSV(raw string, fallback []int) []int {
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
