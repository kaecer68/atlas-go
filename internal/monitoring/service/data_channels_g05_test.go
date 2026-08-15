package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"gopkg.in/yaml.v3"
)

// TestGetAllChannelStatuses_IncludesRegisteredFallback verifies the admin
// page lists channels from the registry that don't have a static builder
// (manifest #G05 — the admin page previously missed ~18 registered channels).
func TestGetAllChannelStatuses_IncludesRegisteredFallback(t *testing.T) {
	dir := t.TempDir()
	_ = yaml.Node{} // keep yaml import alive for future tightening of the assertion
	svc := NewDataChannelService(dir, nil, nil, nil, nil, nil, "", "", "", "")
	svc.RegisteredChannelIDs = []string{
		"taifex_institutional", // new in #E01 — no static builder
		"government_flow",      // new in #E04 — no static builder
		"twse_replay",          // static builder covers; must NOT be duplicated
		"us_spx",               // covered by buildUSMacroChannels
	}
	channels, err := svc.GetAllChannelStatuses(context.TODO())
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	seen := map[string]int{}
	for _, c := range channels {
		seen[c.ChannelID]++
	}
	for _, id := range []string{"taifex_institutional", "government_flow"} {
		if seen[id] == 0 {
			t.Errorf("registered channel %q missing from admin page", id)
		}
	}
	// No duplicate for channels already produced by static builders.
	if seen["twse_replay"] != 1 {
		t.Errorf("twse_replay duplicated: count=%d", seen["twse_replay"])
	}
	if seen["us_spx"] != 1 {
		t.Errorf("us_spx duplicated: count=%d", seen["us_spx"])
	}
	// Fallback entry has the registry marker so operators can tell it apart
	// from hand-built entries.
	for _, c := range channels {
		if c.ChannelID == "taifex_institutional" && !strings.Contains(c.Platform, "registered") {
			t.Errorf("fallback platform=%q, expected marker", c.Platform)
		}
	}
}

// TestGetAllChannelStatuses_FallbackResolvesHealthFromStore verifies the
// fallback no longer hardcodes "inactive": a registered channel with a live
// fetcher must surface its real health-store status instead of looking disabled.
func TestGetAllChannelStatuses_FallbackResolvesHealthFromStore(t *testing.T) {
	dir := t.TempDir()
	store := apigateway.NewChannelHealthStoreWithPool(dir, nil)
	if err := store.Record("taifex_institutional", "ok", ""); err != nil {
		t.Fatalf("record health: %v", err)
	}
	if err := store.Record("government_flow", "error", "connection refused"); err != nil {
		t.Fatalf("record health: %v", err)
	}

	svc := &DataChannelService{
		WorkDir:              dir,
		healthStore:          store,
		RegisteredChannelIDs: []string{"taifex_institutional", "government_flow"},
	}
	channels, err := svc.GetAllChannelStatuses(context.TODO())
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	byID := make(map[string]DataChannel, len(channels))
	for _, c := range channels {
		byID[c.ChannelID] = c
	}

	if got := byID["taifex_institutional"].Status; got != "ok" {
		t.Errorf("taifex_institutional status = %q, want ok (from health store)", got)
	}
	if got := byID["taifex_institutional"].StatusText; got != "正常" {
		t.Errorf("taifex_institutional status_text = %q, want 正常", got)
	}
	if got := byID["government_flow"].Status; got != "error" {
		t.Errorf("government_flow status = %q, want error (from health store)", got)
	}
	if got := byID["government_flow"].LastError; !strings.Contains(got, "connection refused") {
		t.Errorf("government_flow last_error = %q, want to contain connection refused", got)
	}
}
