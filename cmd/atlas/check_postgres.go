package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/config"
)

const postgresContainerName = "atlas-postgres"

// ensurePostgres attempts to make PostgreSQL available before the application
// initializes its database connection. It returns an empty string on success
// (or when no action was needed), or a diagnostic message describing which
// recovery steps were attempted and why they failed.
func ensurePostgres() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return "" // no database needed
	}
	if os.Getenv("ATLAS_SKIP_DOCKER") != "" {
		logPostgres("ATLAS_SKIP_DOCKER set — skipping automated PostgreSQL startup")
		return ""
	}

	logPostgres("connecting...")

	// Fast path: TCP reachable + credentials valid
	if tryAuthPostgres(dsn, 3*time.Second) {
		logPostgres("connected")
		return ""
	}

	var diags []string
	needsDocker := false
	if tryConnectPostgres(dsn, 3*time.Second) {
		logPostgres("TCP reachable but authentication failed — attempting password repair...")
		diags = append(diags, "auth failed; attempted password repair")
		needsDocker = true
	} else {
		logPostgres("not reachable — checking Docker...")
		diags = append(diags, "not reachable")
		needsDocker = true
	}

	if !needsDocker {
		return ""
	}

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		diags = append(diags, "docker CLI not found")
		logPostgres("docker CLI not found (install Docker Desktop or set DATABASE_URL= to skip)")
		printPostgresHints()
		return strings.Join(diags, "; ")
	}

	if !isDockerDaemonRunning(dockerPath) {
		diags = append(diags, "Docker daemon not running; attempted auto-start")
		logPostgres("Docker daemon not running — attempting to start...")
		if err := startDockerDaemon(); err != nil {
			diags = append(diags, fmt.Sprintf("Docker daemon start failed: %s", err.Error()))
			logPostgres("could not start Docker daemon: " + err.Error())
			printPostgresHints()
			return strings.Join(diags, "; ")
		}
		logPostgres("waiting for Docker daemon...")
		if !waitForDockerDaemon(dockerPath, 60*time.Second) {
			diags = append(diags, "Docker daemon start timeout (60s)")
			logPostgres("Docker daemon did not start within 60s")
			printPostgresHints()
			return strings.Join(diags, "; ")
		}
		diags = append(diags, "Docker daemon started")
		logPostgres("Docker daemon ready")
	}

	containerRunning := isPostgresContainerRunning(dockerPath)
	if !containerRunning {
		diags = append(diags, "postgres container not running; attempted compose up")
		logPostgres("starting postgres service...")
		if err := startPostgresService(); err != nil {
			diags = append(diags, fmt.Sprintf("compose up failed: %s", err.Error()))
			logPostgres("could not start postgres: " + err.Error())
			printPostgresHints()
			return strings.Join(diags, "; ")
		}
	}

	logPostgres("waiting for postgres to become healthy...")
	if !waitForPostgres(dsn, 45*time.Second) {
		diags = append(diags, "postgres healthy wait timeout (45s)")
		logPostgres("postgres did not become ready within 45s")
		printPostgresHints()
		return strings.Join(diags, "; ")
	}

	if tryAuthPostgres(dsn, 5*time.Second) {
		logPostgres("ready")
		return ""
	}

	user, pass := parsePostgresCredentials(dsn)
	if user != "" && pass != "" {
		diags = append(diags, "auth failed; attempted password repair via docker exec")
		logPostgres("authentication failed — attempting password repair via docker exec...")
		if fixPostgresPassword(postgresContainerName, user, pass) {
			logPostgres("password repaired, retrying connection (up to 3 attempts with backoff)...")
			// PostgreSQL SCRAM/MD5 auth cache settles ~2s after ALTER ROLE;
			// single probe misses this. Linear backoff retries cover it.
			if tryAuthPostgresWithRetry(dsn, 3, 5*time.Second) {
				logPostgres("ready")
				return ""
			}
		}
		diags = append(diags, "password repair failed or did not resolve auth")
	}

	logPostgres("postgres did not become ready")
	printPostgresHints()
	return strings.Join(diags, "; ")
}

// tryAuthPostgres attempts a real PostgreSQL connection to verify credentials.
// Uses the provided DSN with a context deadline.
func tryAuthPostgres(dsn string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return false
	}
	_ = conn.Close(ctx)
	return true
}

// tryAuthPostgresWithRetry is the post-password-repair variant. After a
// successful ALTER ROLE the server may need a few seconds to refresh its
// auth cache; a single probe is unreliable. We retry with linear backoff
// (1s, 2s, 3s, ...) up to `attempts` times, each with the same per-call
// context deadline.
func tryAuthPostgresWithRetry(dsn string, attempts int, perCallTimeout time.Duration) bool {
	for i := 1; i <= attempts; i++ {
		time.Sleep(time.Duration(i) * time.Second)
		if tryAuthPostgres(dsn, perCallTimeout) {
			return true
		}
		logPostgres(fmt.Sprintf("retry %d/%d: auth still failing", i, attempts))
	}
	return false
}

// fixPostgresPassword resets the postgres user's password inside the Docker container
// using a Unix-socket connection (trust auth via docker exec).
// This repairs credential mismatches from configuration drift or container rebuilds.
func fixPostgresPassword(containerName, user, password string) bool {
	sql := fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s';", user, password)
	cmd := exec.Command("docker", "exec", containerName, "psql", "-U", user, "-c", sql) //nolint:gosec // dev diagnostic tool, not prod
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}

// isPostgresContainerRunning checks if the postgres container exists and is running.
func isPostgresContainerRunning(dockerPath string) bool {
	cmd := exec.Command(dockerPath, "ps", "--filter", fmt.Sprintf("name=%s", postgresContainerName), //nolint:gosec // dockerPath is a resolved binary path, postgresContainerName is a constant
		"--filter", "status=running", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == postgresContainerName
}

// tryConnectPostgres attempts a TCP dial to the PostgreSQL host extracted from dsn.
func tryConnectPostgres(dsn string, timeout time.Duration) bool {
	host, port := parsePostgresHostPort(dsn)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second) //nolint:gosec // dev diagnostic tool, host comes from DSN
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// parsePostgresCredentials extracts the user and password from a postgres:// DSN.
// Returns ("", "") on parse failure.
func parsePostgresCredentials(dsn string) (user, password string) {
	if !strings.Contains(dsn, "://") {
		return "", ""
	}
	after := strings.SplitN(dsn, "://", 2)[1]
	if !strings.Contains(after, "@") {
		return "", ""
	}
	userInfo, _, _ := strings.Cut(after, "@")
	if strings.Contains(userInfo, ":") {
		parts := strings.SplitN(userInfo, ":", 2)
		return parts[0], parts[1]
	}
	return userInfo, ""
}

// parsePostgresHostPort extracts host and port from a postgres:// DSN.
// Returns "localhost", "5432" on any parse failure.
func parsePostgresHostPort(dsn string) (host, port string) {
	host = "localhost"
	port = "5432"
	if !strings.Contains(dsn, "@") {
		return
	}
	after := strings.SplitN(dsn, "@", 2)[1]
	// Remove query string and database name
	before, _, _ := strings.Cut(after, "/")
	if strings.Contains(before, ":") {
		hp := strings.SplitN(before, ":", 2)
		host = hp[0]
		port = strings.SplitN(hp[1], "?", 2)[0]
	} else {
		host = strings.SplitN(before, "?", 2)[0]
	}
	return
}

func isDockerDaemonRunning(dockerPath string) bool {
	return exec.Command(dockerPath, "info").Run() == nil //nolint:gosec // dockerPath is a resolved binary path, not user input
}

func startDockerDaemon() error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Docker").Start()
	case "linux":
		return exec.Command("systemctl", "start", "docker").Run()
	default:
		return fmt.Errorf("unsupported OS: %s (please start Docker manually)", runtime.GOOS)
	}
}

func waitForDockerDaemon(dockerPath string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isDockerDaemonRunning(dockerPath) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func startPostgresService() error {
	cmd := exec.Command("docker", "compose", "up", "-d", "postgres")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Explicitly pass DB_PASSWORD so docker-compose resolves ${DB_PASSWORD} in compose
	// files even when .env is absent from the project root. The value comes from
	// loadEnvFile() reading ~/.config/atlas-go/.env.
	cmd.Env = append(
		os.Environ(),
		"DB_PASSWORD="+config.GetSecret("DB_PASSWORD"),
	)
	return cmd.Run()
}

func waitForPostgres(dsn string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tryConnectPostgres(dsn, 2*time.Second) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func logPostgres(msg string) {
	log.Printf("[PostgreSQL] %s", msg)
}

func printPostgresHints() {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "── postgreSQL startup hints ──────────────────────────")
	fmt.Fprintln(os.Stderr, "  1. Start Docker Desktop → open -a Docker")
	fmt.Fprintln(os.Stderr, "  2. Then: docker compose up -d postgres")
	fmt.Fprintln(os.Stderr, "  3. Or set DATABASE_URL= to skip PostgreSQL entirely")
	fmt.Fprintln(os.Stderr, "  4. Or set ATLAS_SKIP_DOCKER=1 to suppress this check")
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────────────")
	fmt.Fprintln(os.Stderr, "")
}
