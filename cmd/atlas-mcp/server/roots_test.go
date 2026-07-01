package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type recordingPublisher struct {
	mu     sync.Mutex
	alerts []alerting.Alert
}

func (r *recordingPublisher) Publish(_ context.Context, a alerting.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alerts = append(r.alerts, a)
	return nil
}

func (r *recordingPublisher) snapshot() []alerting.Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]alerting.Alert, len(r.alerts))
	copy(out, r.alerts)
	return out
}

func makeRootDir(t *testing.T) (rootPath, rootURI string) {
	t.Helper()
	root := t.TempDir()
	return root, "file://" + root
}

func TestMCPRootsList_CallsListRoots(t *testing.T) {
	rootPath, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}

	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "note.txt"), []byte("ok"), 0o600))
	client.AddRoots(&mcp.Root{URI: rootURI})

	_, out, err := s.handleMCPRootsList(context.Background(), &mcp.CallToolRequest{Session: ss}, struct{}{})
	require.NoError(t, err)
	require.Contains(t, out.Roots, rootURI)
}

func TestMCPRootsListChangedHandler_RefreshesCache(t *testing.T) {
	_, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}

	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	err := s.handleRootsListChanged(context.Background(), &mcp.RootsListChangedRequest{Session: ss})
	require.NoError(t, err)
	require.Equal(t, []string{rootURI}, s.cachedRoots())
}

func TestMCPRootsReadFile_RejectsPathOutsideRoots(t *testing.T) {
	rootPath, rootURI := makeRootDir(t)
	outside := filepath.Join(rootPath, "..", "outside-secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	maliciousURI := rootURI + "/../outside-secret.txt"
	_, _, err := s.handleMCPRootsReadFile(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPRootsReadFileInput{Path: maliciousURI})
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside")
}

func TestMCPRootsReadFile_RejectsWriteFlagPath(t *testing.T) {
	rootPath, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "clean.txt"), []byte("ok"), 0o600))

	_, _, err := s.handleMCPRootsReadFile(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPRootsReadFileInput{Path: rootURI + "/clean.txt?write=1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write")
}

func TestMCPRootsReadFile_EmitsAuditEntry(t *testing.T) {
	rootPath, rootURI := makeRootDir(t)
	content := []byte("audit-me")
	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "audit.txt"), content, 0o600))

	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	ctx := ContextWithTenantID(context.Background(), "tenant-roots")
	_, _, err := s.handleMCPRootsReadFile(ctx, &mcp.CallToolRequest{Session: ss}, MCPRootsReadFileInput{Path: rootURI + "/audit.txt"})
	require.NoError(t, err)

	entries, rerr := ReadAuditEntries(s.audit.path, 0, time.Now())
	require.NoError(t, rerr)
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "mcp_roots_read_file", entry.Tool)
	require.Equal(t, "tenant-roots", entry.TenantID)
	require.NotNil(t, entry.Extra)
	require.Equal(t, rootURI+"/audit.txt", entry.Extra["path"])
	require.Equal(t, float64(len(content)), entry.Extra["size_bytes"])
	require.NotEmpty(t, entry.Extra["ts"])
	require.Equal(t, "tenant-roots", entry.Extra["tenant_id"])
}

func TestMCPRootsReadFile_FileNotFound_Error(t *testing.T) {
	_, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	_, _, err := s.handleMCPRootsReadFile(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPRootsReadFileInput{Path: rootURI + "/missing.txt"})
	require.Error(t, err)
}

func TestMCPRootsList_ClientLacksRootsCapability_ReturnsUnsupported(t *testing.T) {
	// Explicitly disable the roots capability; the SDK default advertises roots.
	clientOpts := &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	res, _, err := s.handleMCPRootsList(context.Background(), &mcp.CallToolRequest{Session: ss}, struct{}{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	require.Equal(t, "unsupported", text.Text)
}

func TestMCPRootsReadFile_SizeCapRejected(t *testing.T) {
	rootPath, rootURI := makeRootDir(t)
	big := strings.Repeat("x", defaultRootsReadSizeCap+1)
	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "big.txt"), []byte(big), 0o600))

	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{Roots: RootsConfig{ReadSizeCap: defaultRootsReadSizeCap}})
	defer done()

	client.AddRoots(&mcp.Root{URI: rootURI})

	_, _, err := s.handleMCPRootsReadFile(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPRootsReadFileInput{Path: rootURI + "/big.txt"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "size")
}

func TestMCPRootsListChanged_PublisherReceivesInfoAlert_OnSuccess(t *testing.T) {
	_, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	pub := &recordingPublisher{}
	s.alerter = pub

	client.AddRoots(&mcp.Root{URI: rootURI})

	require.NoError(t, s.handleRootsListChanged(context.Background(), &mcp.RootsListChangedRequest{Session: ss}))

	alerts := pub.snapshot()
	require.Len(t, alerts, 1, "exactly one alert must be emitted on successful refresh")
	got := alerts[0]
	require.Equal(t, alerting.EventSecurityRootsChanged, got.Type)
	require.Equal(t, alerting.SeverityInfo, got.Severity)
	require.Equal(t, "atlas-mcp", got.Source)
	require.Contains(t, got.Labels["alertname"], "security_roots_changed")
	require.Equal(t, rootURI, got.Annotations["root_uri"])
	require.Equal(t, "1", got.Annotations["root_count"])
}

func TestMCPRootsListChanged_PublisherReceivesWarningAlert_OnListFailure(t *testing.T) {
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, _, _, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	pub := &recordingPublisher{}
	s.alerter = pub

	s.publishRootsChangedAlert(context.Background(), errors.New("list roots: synthetic"))

	alerts := pub.snapshot()
	require.Len(t, alerts, 1, "exactly one alert must be emitted on refresh failure")
	got := alerts[0]
	require.Equal(t, alerting.EventSecurityRootsChanged, got.Type)
	require.Equal(t, alerting.SeverityWarning, got.Severity)
	require.NotEmpty(t, got.Message)
	require.Contains(t, got.Annotations["error"], "list roots")
}

func TestMCPRootsListChanged_NoPublisher_DoesNotPanic(t *testing.T) {
	_, rootURI := makeRootDir(t)
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{}},
	}
	s, client, ss, done := newTestServerWithClient(t, clientOpts, Config{})
	defer done()

	s.alerter = nil
	client.AddRoots(&mcp.Root{URI: rootURI})

	require.NoError(t, s.handleRootsListChanged(context.Background(), &mcp.RootsListChangedRequest{Session: ss}))
}
