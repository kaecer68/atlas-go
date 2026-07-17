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
// fubon-proxy and atlas are both Docker containers on the same bridge network.
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

	// Primary check: "host.docker.internal" (extra_hosts entry) must appear in docker-compose.yml.
	if !strings.Contains(content, "host.docker.internal") {
		t.Fatalf("docker-compose.yml is missing 'host.docker.internal'")
	}

	// Secondary check: it must be within the atlas service section
	// (not accidentally in another service). Find "atlas:" line and
	// the next top-level service line, then verify host.docker.internal extra_hosts entry
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
		t.Errorf(
			"'host.docker.internal' not found within atlas service section\n\n" +
				"原因：atlas 容器需要 host.docker.internal extra_hosts 條目作為 host gateway fallback。" +
				"（PR #941 後 fubon-proxy 主要走 Docker DNS fubon-proxy。）",
		)
	}
}
