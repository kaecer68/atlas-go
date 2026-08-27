package ledger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

func TestResolveStoreBackend(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		want      string
		wantErr   bool
	}{
		{name: "explicit_jsonl", flagValue: "jsonl", want: "jsonl"},
		{name: "empty_defaults_to_jsonl", flagValue: "", want: "jsonl"},
		{name: "sqlite", flagValue: "sqlite", want: "sqlite"},
		{name: "postgres", flagValue: "postgres", want: "postgres"},
		{name: "typo_postgre", flagValue: "postgre", wantErr: true},
		{name: "typo_sqllite", flagValue: "sqllite", wantErr: true},
		{name: "case_sensitive", flagValue: "Postgres", wantErr: true},
		{name: "garbage", flagValue: "unknown_backend", wantErr: true},
		{name: "whitespace", flagValue: " ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveStoreBackend(tt.flagValue)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveStoreBackend(%q) expected error, got %q", tt.flagValue, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveStoreBackend(%q) unexpected error: %v", tt.flagValue, err)
			}
			if got != tt.want {
				t.Fatalf("ResolveStoreBackend(%q) = %q, want %q", tt.flagValue, got, tt.want)
			}
		})
	}
}

func TestResolveStoreBackendEmptyLogsWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { logging.SetLogger(prev) })

	got, err := ResolveStoreBackend("")
	if err != nil {
		t.Fatalf("ResolveStoreBackend(empty) unexpected error: %v", err)
	}
	if got != "jsonl" {
		t.Fatalf("ResolveStoreBackend(empty) = %q, want jsonl", got)
	}
	if !strings.Contains(buf.String(), "store_backend_empty_default_jsonl") {
		t.Fatalf("expected empty-backend warn event, got log: %q", buf.String())
	}
}

// TestFactoriesUnknownBackendFails covers M4①: every New*Store factory must
// fail loudly on an unrecognized backend string instead of silently falling
// back to the JSONL ledger. "postgre" is the canonical typo from the audit.
func TestFactoriesUnknownBackendFails(t *testing.T) {
	cfg := config.Config{LedgerDir: t.TempDir(), StoreBackend: "postgre"}
	cases := []struct {
		name string
		call func() error
	}{
		{"NewFullStore", func() error { _, err := NewFullStore(cfg); return err }},
		{"NewOutcomeStore", func() error { _, err := NewOutcomeStore(cfg); return err }},
		{"NewReportOutcomeStore", func() error { _, err := NewReportOutcomeStore(cfg); return err }},
		{"NewSessionStore", func() error { _, err := NewSessionStore(cfg); return err }},
		{"NewQuoteStore", func() error { _, err := NewQuoteStore(cfg); return err }},
		{"NewDetectorScanStore", func() error { _, err := NewDetectorScanStore(cfg); return err }},
		{"NewHistoricalStore", func() error { _, err := NewHistoricalStore(cfg); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s(postgre typo) should fail, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "unknown store backend") {
				t.Fatalf("%s error should mention unknown store backend, got: %v", tc.name, err)
			}
		})
	}
}

func TestNewOutcomeStoreEmptyBackendFallsBackToJSONL(t *testing.T) {
	cfg := config.Config{
		LedgerDir:    t.TempDir(),
		StoreBackend: "", // legacy unset behavior → JSONL
	}
	store, err := NewOutcomeStore(cfg)
	if err != nil {
		t.Fatalf("NewOutcomeStore(empty) should fall back to JSONL, got error: %v", err)
	}
	if _, ok := store.(*Store); !ok {
		t.Fatalf("expected JSONL *Store, got %T", store)
	}
}

func TestNewFullStoreEmptyBackendFallsBackToJSONL(t *testing.T) {
	cfg := config.Config{
		LedgerDir:    t.TempDir(),
		StoreBackend: "",
	}
	store, err := NewFullStore(cfg)
	if err != nil {
		t.Fatalf("NewFullStore(empty) should fall back to JSONL, got error: %v", err)
	}
	if _, ok := store.(*Store); !ok {
		t.Fatalf("expected JSONL *Store, got %T", store)
	}
}
