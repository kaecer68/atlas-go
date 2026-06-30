package server

import (
	"errors"
	"testing"
)

func TestTokenAuth_DevMode(t *testing.T) {
	a := NewTokenAuth("")
	if err := a.Check(""); err != nil {
		t.Fatalf("dev mode should accept empty: %v", err)
	}
	if err := a.Check("anything"); err != nil {
		t.Fatalf("dev mode should accept anything: %v", err)
	}
}

func TestTokenAuth_Required(t *testing.T) {
	a := NewTokenAuth("topsecret")
	if err := a.Check("topsecret"); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if err := a.Check(""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("empty token should fail with ErrUnauthorized: %v", err)
	}
	if err := a.Check("wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong token should fail with ErrUnauthorized: %v", err)
	}
	if err := a.Check("TOPSECRET"); err != nil {
		t.Fatalf("token check is case-insensitive: %v", err)
	}
}

func TestTokenAuth_Status(t *testing.T) {
	if got := NewTokenAuth("").Status(); got == "" {
		t.Fatal("expected non-empty status")
	}
	if got := NewTokenAuth("k").Status(); got == "" {
		t.Fatal("expected non-empty status")
	}
}

func TestTokenAuth_Wrap(t *testing.T) {
	a := NewTokenAuth("k")
	called := false
	handler := func(token string) error {
		called = true
		if token != "k" {
			t.Fatalf("handler got unexpected token %q", token)
		}
		return nil
	}
	wrapped := a.Wrap(handler)
	if err := wrapped("k"); err != nil {
		t.Fatalf("good token: %v", err)
	}
	if !called {
		t.Fatal("handler not called on good token")
	}
	if err := wrapped("bad"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token should be unauthorized: %v", err)
	}
}
