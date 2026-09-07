// Package channelsecrets implements persistent, encrypted storage and hot
// reload for data-channel API keys (issue #1776 Phase 1).
//
// The old admin "API key" panel was an os.Setenv placebo (#1777 removed it):
// keys are injected at startup via config and nothing re-reads the process
// environment. This package provides the replacement, with all three pieces
// the issue requires:
//
//  1. Persistence: keys are AES-256-GCM encrypted and stored (Postgres in
//     production, job-local SQLite as dev fallback) — they survive restarts.
//  2. Hot reload: Manager.Set persists, then applies the key to the live
//     shared clients via registered appliers (no restart needed).
//  3. Precedence: at startup the caller merges DB overrides over .env —
//     DB wins so an admin-set key is not silently reverted by a redeploy.
//
// The master encryption key comes from ATLAS_SECRET_STORE_KEY (base64, 32
// bytes for AES-256) and is NEVER persisted to the database. Without it,
// writes fail closed (we refuse to store plaintext keys) and encrypted rows
// cannot be decrypted (they are skipped, never logged).
package channelsecrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// MasterKeyEnv is the env var holding the base64-encoded 32-byte
	// AES-256 master key. Losing it makes stored keys undecryptable —
	// back it up alongside the iMac .env (issue #1776).
	MasterKeyEnv = "ATLAS_SECRET_STORE_KEY"
	// masterKeyLen is the required decoded key length (AES-256).
	masterKeyLen = 32
)

// ErrNoMasterKey is returned when ATLAS_SECRET_STORE_KEY is unset. Callers
// must treat it as fail-closed for writes: storing a plaintext key is worse
// than not storing one.
var ErrNoMasterKey = errors.New("channelsecrets: ATLAS_SECRET_STORE_KEY not set")

// LoadMasterKey reads and decodes the base64-encoded AES-256 master key
// from MasterKeyEnv.
func LoadMasterKey() ([]byte, error) {
	raw := os.Getenv(MasterKeyEnv)
	if raw == "" {
		return nil, ErrNoMasterKey
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("channelsecrets: decode %s: %w", MasterKeyEnv, err)
	}
	if len(key) != masterKeyLen {
		return nil, fmt.Errorf("channelsecrets: %s must decode to %d bytes, got %d", MasterKeyEnv, masterKeyLen, len(key))
	}
	return key, nil
}

// Encrypt seals plaintext with AES-256-GCM under the given master key,
// returning nonce || ciphertext. A fresh random nonce is used per call.
func Encrypt(masterKey, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(masterKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("channelsecrets: nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a nonce || ciphertext blob produced by Encrypt.
func Decrypt(masterKey, blob []byte) ([]byte, error) {
	gcm, err := newGCM(masterKey)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("channelsecrets: blob too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("channelsecrets: decrypt (wrong master key or corrupted row?): %w", err)
	}
	return pt, nil
}

func newGCM(masterKey []byte) (cipher.AEAD, error) {
	if len(masterKey) != masterKeyLen {
		return nil, fmt.Errorf("channelsecrets: master key must be %d bytes, got %d", masterKeyLen, len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
