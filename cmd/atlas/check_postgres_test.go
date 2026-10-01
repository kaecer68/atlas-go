package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakePostgres is a minimal PostgreSQL server that answers the startup packet
// with a single ErrorResponse. It exists so the classification tests exercise
// the real pgx/pgconn error chain (ConnectError -> perDialConnectError -> PgError)
// instead of a hand-built stand-in.
type fakePostgres struct {
	addr string
	stop func()
}

func startFakePostgres(t *testing.T, code, message string) *fakePostgres {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			handleFakePostgresConn(conn, code, message)
		}
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = ln.Close()
		})
		wg.Wait()
	}
	t.Cleanup(stop)

	return &fakePostgres{addr: ln.Addr().String(), stop: stop}
}

// handleFakePostgresConn reads the startup packet (length-prefixed) and replies
// with an ErrorResponse carrying the requested SQLSTATE.
func handleFakePostgresConn(conn net.Conn, code, message string) {
	defer conn.Close()

	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return
	}
	n := int(binary.BigEndian.Uint32(hdr[:]))
	if n < 4 || n > 1<<16 {
		return
	}
	if _, err := io.ReadFull(conn, make([]byte, n-4)); err != nil {
		return
	}

	payload := &bytes.Buffer{}
	for _, field := range []struct {
		kind byte
		text string
	}{
		{'S', "FATAL"},
		{'V', "FATAL"},
		{'C', code},
		{'M', message},
	} {
		payload.WriteByte(field.kind)
		payload.WriteString(field.text)
		payload.WriteByte(0)
	}
	payload.WriteByte(0) // terminator

	out := &bytes.Buffer{}
	out.WriteByte('E')
	_ = binary.Write(out, binary.BigEndian, int32(payload.Len()+4))
	out.Write(payload.Bytes())
	_, _ = conn.Write(out.Bytes())

	// Give the client a moment to read the response before the socket closes.
	time.Sleep(100 * time.Millisecond)
}

func (f *fakePostgres) dsn() string {
	// sslmode=disable keeps the handshake to "startup packet -> ErrorResponse".
	return "postgres://probe:probe@" + f.addr + "/atlas_probe?sslmode=disable"
}

// TestIsPostgresAuthFailure covers the synthetic classification table.
func TestIsPostgresAuthFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"invalid_password", &pgconn.PgError{Code: "28P01"}, true},
		{"invalid_authorization_specification", &pgconn.PgError{Code: "28000"}, true},
		{"cannot_connect_now_while_starting_up", &pgconn.PgError{Code: "57P03"}, false},
		{"unknown_database", &pgconn.PgError{Code: "3D000"}, false},
		{"too_many_connections", &pgconn.PgError{Code: "53300"}, false},
		{"wrapped_invalid_password", fmt.Errorf("server error: %w", &pgconn.PgError{Code: "28P01"}), true},
		{"joined_with_auth_failure", errors.Join(errors.New("dial tcp: connection refused"), &pgconn.PgError{Code: "28P01"}), true},
		{"connection_refused", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, false},
		{"context_deadline", context.DeadlineExceeded, false},
		{"plain_error", errors.New("no such host"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPostgresAuthFailure(tc.err); got != tc.want {
				t.Fatalf("isPostgresAuthFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestTryAuthPostgres_ClassifiesServerRejection is the end-to-end half of the
// classification: a real (fake) server rejects the startup packet, and the error
// pgx actually returns must be classified correctly. This is what failed before:
// the probe collapsed every error into `false`, so a still-starting postmaster
// (57P03) was reported as an authentication failure.
func TestTryAuthPostgres_ClassifiesServerRejection(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		wantAuth bool
	}{
		{"bad_password_is_auth_failure", "28P01", true},
		{"invalid_authorization_specification_is_auth_failure", "28000", true},
		{"starting_up_is_not_an_auth_failure", "57P03", false},
		{"unknown_database_is_not_an_auth_failure", "3D000", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := startFakePostgres(t, tc.code, "fake server rejection")
			defer srv.stop()

			err := tryAuthPostgres(srv.dsn(), 5*time.Second)
			if err == nil {
				t.Fatal("tryAuthPostgres returned nil for a server that rejected the connection")
			}
			if got := isPostgresAuthFailure(err); got != tc.wantAuth {
				t.Fatalf("isPostgresAuthFailure(%v) = %v, want %v", err, got, tc.wantAuth)
			}
			// The classification must be able to see the SQLSTATE, not just a string.
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("error chain does not expose *pgconn.PgError: %v", err)
			}
			if pgErr.Code != tc.code {
				t.Fatalf("SQLSTATE = %q, want %q", pgErr.Code, tc.code)
			}
		})
	}
}

// TestTryAuthPostgres_UnreachableIsNotAuthFailure pins the second half of the
// contract: a closed port is a connection-layer problem, never a credential one.
func TestTryAuthPostgres_UnreachableIsNotAuthFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing is listening now

	dsn := "postgres://probe:probe@" + addr + "/atlas_probe?sslmode=disable"
	probeErr := tryAuthPostgres(dsn, 2*time.Second)
	if probeErr == nil {
		t.Fatal("tryAuthPostgres returned nil against a closed port")
	}
	if isPostgresAuthFailure(probeErr) {
		t.Fatalf("a closed port must not be classified as an auth failure: %v", probeErr)
	}
}

// TestPostgresPasswordRepair_ReportsOnlyWhatRan pins the diagnostic contract that
// used to be wrong: the "attempted password repair" fragment is produced only
// after the repair command has actually run (the old code appended it first, so
// the diagnostic claimed a step that had not happened).
func TestPostgresPasswordRepair_ReportsOnlyWhatRan(t *testing.T) {
	const (
		container = "atlas-postgres"
		user      = "atlas"
		pass      = "secret"
		attempted = "auth failed; attempted password repair via docker exec"
	)

	t.Run("successful_repair", func(t *testing.T) {
		var calls int
		var gotArgs [3]string
		diags, repaired := attemptPasswordRepair(container, user, pass, func(c, u, p string) bool {
			calls++
			gotArgs = [3]string{c, u, p}
			return true
		})

		if calls != 1 {
			t.Fatalf("fix called %d times, want 1", calls)
		}
		if gotArgs != [3]string{container, user, pass} {
			t.Fatalf("fix args = %v, want [%s %s %s]", gotArgs, container, user, pass)
		}
		if !repaired {
			t.Fatal("repaired = false, want true")
		}
		want := []string{attempted}
		if len(diags) != len(want) || diags[0] != want[0] {
			t.Fatalf("diags = %v, want %v", diags, want)
		}
	})

	t.Run("failed_repair_command", func(t *testing.T) {
		var calls int
		diags, repaired := attemptPasswordRepair(container, user, pass, func(string, string, string) bool {
			calls++
			return false
		})

		if calls != 1 {
			t.Fatalf("fix called %d times, want 1", calls)
		}
		if repaired {
			t.Fatal("repaired = true, want false")
		}
		want := []string{attempted, "password repair command failed"}
		if len(diags) != len(want) {
			t.Fatalf("diags = %v, want %v", diags, want)
		}
		for i := range want {
			if diags[i] != want[i] {
				t.Fatalf("diags[%d] = %q, want %q", i, diags[i], want[i])
			}
		}
	})
}

// TestParsePostgresCredentials_ProbeDSN guards the probe helpers the tests above
// rely on, so a parse regression cannot silently mis-route the repair path.
func TestParsePostgresCredentials_ProbeDSN(t *testing.T) {
	user, pass := parsePostgresCredentials("postgres://probe:probe@127.0.0.1:5432/atlas_probe?sslmode=disable")
	if user != "probe" || pass != "probe" {
		t.Fatalf("parsePostgresCredentials = (%q, %q), want (probe, probe)", user, pass)
	}
	host, port := parsePostgresHostPort("postgres://probe:probe@127.0.0.1:5432/atlas_probe?sslmode=disable")
	if host != "127.0.0.1" || port != "5432" {
		t.Fatalf("parsePostgresHostPort = (%q, %q), want (127.0.0.1, 5432)", host, port)
	}
}
