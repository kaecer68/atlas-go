package channelsecrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"sync"
	"testing"
)

func testMasterKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	master := testMasterKey(t)
	pt := []byte("super-secret-finmind-token-1234")
	blob, err := Encrypt(master, pt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(master, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
	// Fresh nonce per call: same plaintext → different blobs.
	blob2, _ := Encrypt(master, pt)
	if bytes.Equal(blob, blob2) {
		t.Fatal("nonce reuse: identical ciphertexts for identical plaintext")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	blob, err := Encrypt(testMasterKey(t), []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(testMasterKey(t), blob); err == nil {
		t.Fatal("decrypt with wrong master key must fail")
	}
}

func TestDecryptTamperedBlobFails(t *testing.T) {
	master := testMasterKey(t)
	blob, _ := Encrypt(master, []byte("secret"))
	blob[len(blob)-1] ^= 0xFF
	if _, err := Decrypt(master, blob); err == nil {
		t.Fatal("tampered blob must fail GCM auth")
	}
}

func TestLoadMasterKey(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		t.Setenv(MasterKeyEnv, "")
		if _, err := LoadMasterKey(); !errors.Is(err, ErrNoMasterKey) {
			t.Fatalf("err = %v, want ErrNoMasterKey", err)
		}
	})
	t.Run("invalid base64", func(t *testing.T) {
		t.Setenv(MasterKeyEnv, "!!!not-base64!!!")
		if _, err := LoadMasterKey(); err == nil || errors.Is(err, ErrNoMasterKey) {
			t.Fatalf("err = %v, want decode error", err)
		}
	})
	t.Run("wrong length", func(t *testing.T) {
		t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString([]byte("short")))
		if _, err := LoadMasterKey(); err == nil {
			t.Fatal("short key must be rejected")
		}
	})
	t.Run("valid", func(t *testing.T) {
		k := testMasterKey(t)
		t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(k))
		got, err := LoadMasterKey()
		if err != nil || !bytes.Equal(got, k) {
			t.Fatalf("got %v err %v", got, err)
		}
	})
}

func TestManagerSetLoadApply(t *testing.T) {
	dir := t.TempDir()
	master := testMasterKey(t)
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(master))

	store, err := NewStore(context.Background(), "sqlite", nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	var applied []string
	var mu sync.Mutex
	mgr, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	mgr.RegisterApplier("finmind", func(k string) {
		mu.Lock()
		defer mu.Unlock()
		applied = append(applied, k)
	})

	st, err := mgr.Set(context.Background(), "finmind", "  new-finmind-token-9876  ", "ops@test")
	if err != nil {
		t.Fatal(err)
	}
	if st.MaskedKey != "****9876" {
		t.Errorf("masked = %q, want ****9876", st.MaskedKey)
	}
	if st.Source != "db" || st.UpdatedBy != "ops@test" {
		t.Errorf("status = %+v", st)
	}
	mu.Lock()
	if len(applied) != 1 || applied[0] != "new-finmind-token-9876" {
		t.Errorf("applier got %v", applied)
	}
	mu.Unlock()

	// Overrides roundtrip (trimmed plaintext).
	ov := mgr.LoadOverrides(context.Background())
	if ov["finmind"] != "new-finmind-token-9876" {
		t.Fatalf("override = %q", ov["finmind"])
	}
}

func TestManagerSetValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(testMasterKey(t)))
	store, _ := NewStore(context.Background(), "sqlite", nil, dir)
	mgr, _ := NewManager(store)
	ctx := context.Background()

	if _, err := mgr.Set(ctx, "tej", "1234567890", "x"); !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("tej: err = %v, want ErrUnknownProvider", err)
	}
	if _, err := mgr.Set(ctx, "finmind", "short", "x"); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("short key: err = %v, want ErrInvalidKey", err)
	}
}

func TestManagerNoMasterKeyFailsClosed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(MasterKeyEnv, "")
	store, _ := NewStore(context.Background(), "sqlite", nil, dir)
	mgr, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.EncryptionEnabled() {
		t.Fatal("encryption must be disabled without master key")
	}
	if _, err := mgr.Set(context.Background(), "finmind", "1234567890", "x"); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("err = %v, want ErrNoMasterKey (fail-closed)", err)
	}
	if ov := mgr.LoadOverrides(context.Background()); ov != nil {
		t.Fatalf("overrides = %v, want nil", ov)
	}
}

// TestStorePersistenceAcrossRestart: the acceptance criterion — a key set
// via the manager must survive creating a brand-new store over the same
// DB (simulated restart).
func TestStorePersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	master := testMasterKey(t)
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(master))
	ctx := context.Background()

	store1, err := NewStore(ctx, "sqlite", nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	mgr1, _ := NewManager(store1)
	if _, err := mgr1.Set(ctx, "fugle", "fugle-key-before-restart", "ops"); err != nil {
		t.Fatal(err)
	}

	store2, err := NewStore(ctx, "sqlite", nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	mgr2, _ := NewManager(store2)
	ov := mgr2.LoadOverrides(ctx)
	if ov["fugle"] != "fugle-key-before-restart" {
		t.Fatalf("after restart: %q", ov["fugle"])
	}
}

func TestStatusSources(t *testing.T) {
	dir := t.TempDir()
	master := testMasterKey(t)
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(master))
	t.Setenv("FINMIND_API_KEY", "env-sourced-finmind-0001")
	ctx := context.Background()

	store, _ := NewStore(ctx, "sqlite", nil, dir)
	mgr, _ := NewManager(store)
	if _, err := mgr.Set(ctx, "fugle", "db-sourced-fugle-9999", "ops"); err != nil {
		t.Fatal(err)
	}
	keys, err := mgr.Status(ctx, map[string]string{"finmind": os.Getenv("FINMIND_API_KEY")})
	if err != nil {
		t.Fatal(err)
	}
	byProvider := map[string]KeyStatus{}
	for _, k := range keys {
		byProvider[k.Provider] = k
	}
	if byProvider["finmind"].Source != "env" || byProvider["finmind"].MaskedKey != "****0001" {
		t.Errorf("finmind = %+v", byProvider["finmind"])
	}
	if byProvider["fugle"].Source != "db" || byProvider["fugle"].MaskedKey != "****9999" {
		t.Errorf("fugle = %+v", byProvider["fugle"])
	}
	if _, ok := byProvider["tej"]; ok {
		t.Error("tej must not be listed (not in whitelist)")
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"abc":      "****",
		"abcd":     "****",
		"abcde":    "****bcde",
		"longkey9": "****key9",
	}
	for in, want := range cases {
		if got := MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUndecryptableRowSkipped(t *testing.T) {
	dir := t.TempDir()
	master1 := testMasterKey(t)
	ctx := context.Background()

	// Store under master1.
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(master1))
	store1, _ := NewStore(ctx, "sqlite", nil, dir)
	mgr1, _ := NewManager(store1)
	if _, err := mgr1.Set(ctx, "finmind", "key-under-old-master", "ops"); err != nil {
		t.Fatal(err)
	}

	// Reopen under a different master: row must be skipped, not crash.
	master2 := testMasterKey(t)
	t.Setenv(MasterKeyEnv, base64.StdEncoding.EncodeToString(master2))
	store2, _ := NewStore(ctx, "sqlite", nil, dir)
	mgr2, _ := NewManager(store2)
	ov := mgr2.LoadOverrides(ctx)
	if len(ov) != 0 {
		t.Fatalf("undecryptable row must be skipped, got %v", ov)
	}
}
