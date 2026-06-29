package startup

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func occupyAddr(t *testing.T, addr string) func() {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:"+portStr)
	if err != nil {
		t.Fatalf("occupy %s: %v", addr, err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

func TestPreflight_AllFree(t *testing.T) {
	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: freeAddr(t)},
	}
	if err := Preflight(claims); err != nil {
		t.Fatalf("Preflight(all free) = %v, want nil", err)
	}
}

func TestPreflight_OneForeign(t *testing.T) {
	foreignAddr := freeAddr(t)
	cleanup := occupyAddr(t, foreignAddr)
	defer cleanup()

	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: foreignAddr},
	}
	err := Preflight(claims)
	if err == nil {
		t.Fatal("Preflight(foreign) = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fubon-proxy") {
		t.Errorf("error should mention component, got: %q", msg)
	}
	if !strings.Contains(msg, "kill") {
		t.Errorf("error should suggest kill, got: %q", msg)
	}
	if !strings.Contains(msg, foreignAddr) {
		t.Errorf("error should mention address, got: %q", msg)
	}
}

func TestPreflight_MixFreeAndForeign_ReturnsFirstForeign(t *testing.T) {
	foreignAddr := freeAddr(t)
	cleanup := occupyAddr(t, foreignAddr)
	defer cleanup()

	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: foreignAddr},
		{Component: "extra", Addr: freeAddr(t)},
	}
	err := Preflight(claims)
	if err == nil {
		t.Fatal("Preflight(mix) = nil, want error")
	}
	if !strings.Contains(err.Error(), "fubon-proxy") {
		t.Errorf("expected first foreign error, got: %q", err.Error())
	}
}

func TestPreflight_ProbeError_Continues(t *testing.T) {
	claims := []PortClaim{
		{Component: "bad-addr", Addr: "not-an-address"},
		{Component: "atlas-http", Addr: freeAddr(t)},
	}
	if err := Preflight(claims); err != nil {
		t.Fatalf("Preflight(probe error + free) = %v, want nil", err)
	}
}
