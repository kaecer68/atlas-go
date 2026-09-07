package marketdata

import "testing"

// --- issue #1776 hot reload ---

func TestUpdateSharedFugleAPIKey(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()

	c := GetSharedFugleClient("initial-key-aaaa")
	if got := c.currentAPIKey(); got != "initial-key-aaaa" {
		t.Fatalf("initial = %q", got)
	}
	UpdateSharedFugleAPIKey("rotated-key-bbbb")
	if got := c.currentAPIKey(); got != "rotated-key-bbbb" {
		t.Fatalf("after update = %q, want rotated-key-bbbb", got)
	}
	// Same singleton: the shared pointer must be mutated, not replaced.
	if GetSharedFugleClient("ignored") != c {
		t.Fatal("shared client identity changed")
	}
}

func TestUpdateSharedFugleAPIKey_NoClientNoop(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()
	UpdateSharedFugleAPIKey("key-before-client") // must not panic
	if sharedFugleClient != nil {
		t.Fatal("no-op update must not create the shared client")
	}
	c := GetSharedFugleClient("real-key")
	if got := c.currentAPIKey(); got != "real-key" {
		t.Fatalf("client created with key %q, want real-key", got)
	}
}

func TestUpdateSharedFinMindAPIKey(t *testing.T) {
	ResetSharedFinMindClient()
	defer ResetSharedFinMindClient()

	c := GetSharedFinMindClient("initial-key-aaaa", t.TempDir())
	if got := c.currentAPIKey(); got != "initial-key-aaaa" {
		t.Fatalf("initial = %q", got)
	}
	UpdateSharedFinMindAPIKey("rotated-key-bbbb")
	if got := c.currentAPIKey(); got != "rotated-key-bbbb" {
		t.Fatalf("after update = %q, want rotated-key-bbbb", got)
	}
}
