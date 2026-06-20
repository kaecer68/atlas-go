package apigateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDockerCompose_HasHostDockerInternalAlias (P1-S2) verifies that
// docker-compose.yml includes the extra_hosts entry:
//
//	extra_hosts:
//	  - "host.docker.internal:host-gateway"
//
// on the atlas service. This enables the atlas container to reach the
// fubon-proxy running on the macOS host via host.docker.internal:8081.
// The host-gateway special value is supported on Docker Desktop 4.13+
// (macOS/Windows) and resolves to the host's gateway IP.
func TestDockerCompose_HasHostDockerInternalAlias(t *testing.T) {
	// Find the repo root (where docker-compose.yml lives).
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find repo root (go.mod)")
		}
		dir = parent
	}

	composePath := filepath.Join(dir, "docker-compose.yml")
	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}

	content := string(data)

	// Primary check: "host.docker.internal" must appear in the file.
	if !strings.Contains(content, "host.docker.internal") {
		t.Fatalf("docker-compose.yml is missing 'host.docker.internal'")
	}

	// Secondary check: it must be within the atlas service section
	// (not accidentally in another service). Find "atlas:" line and
	// the next top-level service line, then verify host.docker.internal
	// appears between them.
	lines := strings.Split(content, "\n")
	inAtlas := false
	foundInAtlas := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Top-level service key (2-space indent, no deeper indent)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			// This is a service name at the top level (e.g., "  atlas:", "  postgres:")
			if trimmed == "atlas:" {
				inAtlas = true
			} else {
				inAtlas = false
			}
		}

		if inAtlas && strings.Contains(line, "host.docker.internal") {
			foundInAtlas = true
			t.Logf("host.docker.internal found in atlas service: %s", trimmed)
		}
	}

	if !foundInAtlas {
		t.Errorf("'host.docker.internal' not found within atlas service section\n\n"+
			"原因：atlas 容器需要透過 host.docker.internal 連線至 macOS host 上的 fubon-proxy。"+
			"Docker Desktop macOS/Windows 4.13+ 支援 host-gateway 特殊值。",
		)
	}
}
