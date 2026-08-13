package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	concurrentWorkers         = 50
	concurrentWritesPerWorker = 20
	concurrentHandlerRounds   = 50
)

// TestAuditWriter_ConcurrentWrites spins up N goroutines that all share one
// AuditWriter and asserts every write is durable (no dropped or interleaved
// entries) and the file ends with exactly N×M well-formed JSON lines.
//
// Expected to PASS without code changes — sync.Mutex in AuditWriter.Write is
// the contract under test. The race detector (-race) is the real proof.
func TestAuditWriter_ConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent-audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("audit new: %v", err)
	}
	defer w.Close()

	var wg sync.WaitGroup
	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < concurrentWritesPerWorker; j++ {
				entry := AuditEntry{
					Tool:       fmt.Sprintf("worker_%d", id),
					ArgKeys:    []string{fmt.Sprintf("j=%d", j)},
					Status:     "ok",
					DurationMS: int64(j),
				}
				if wErr := w.Write(entry); wErr != nil {
					t.Errorf("write worker=%d j=%d: %v", id, j, wErr)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("audit log empty")
	}
	trimmed := strings.TrimRight(string(raw), "\n")
	lines := strings.Split(trimmed, "\n")
	want := concurrentWorkers * concurrentWritesPerWorker
	if len(lines) != want {
		t.Fatalf("line count: got %d want %d", len(lines), want)
	}
	for i, line := range lines {
		var e AuditEntry
		if jErr := json.Unmarshal([]byte(line), &e); jErr != nil {
			t.Fatalf("line %d not valid JSON: %v", i, jErr)
		}
		if !strings.HasPrefix(e.Tool, "worker_") {
			t.Fatalf("line %d unexpected tool: %q", i, e.Tool)
		}
	}
}

// TestHTTPClient_ConcurrentRequests drives N parallel GETs through one
// HttpClient (which wraps Go's stdlib *http.Client, documented safe for
// concurrent use). Asserts no errors and that the upstream saw exactly N
// requests.
func TestHTTPClient_ConcurrentRequests(t *testing.T) {
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond) // mild contention to surface races
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	cli := NewHTTPClient(Config{AtlasBaseURL: ts.URL})

	const n = concurrentHandlerRounds
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out []map[string]any
			if err := cli.Get(context.Background(), "/x", nil, &out); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Fatalf("client get: %v", e)
		}
	}
	if got := hits.Load(); got != n {
		t.Fatalf("upstream hit count: got %d want %d", got, n)
	}
}

// TestHandlers_ConcurrentInvocations fires N parallel regime_get_history calls
// through the same *server and asserts every invocation succeeds. Audit log
// is shared, so this also exercises AuditWriter under concurrency.
func TestHandlers_ConcurrentInvocations(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{"sessions":[]}`))

	const n = concurrentHandlerRounds
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: intPtr(id + 1)})
			if err != nil {
				t.Errorf("goroutine %d: %v", id, err)
			}
		}(i)
	}
	wg.Wait()
}

// TestWithAudit_ConcurrentMix exercises every tool handler concurrently so
// audit writes and HTTP calls from different goroutines don't interleave.
// This is the closest in-process reproduction of a busy MCP session.
func TestWithAudit_ConcurrentMix(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	const n = concurrentHandlerRounds
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			switch id % 5 {
			case 0:
				_, _, _ = s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: intPtr(7)})
			case 1:
				rec.SetResponseBody([]byte(`{"strategies":[]}`))
				_, _, _ = s.handleStrategyListActive(context.Background(), nil, struct{}{})
			case 2:
				rec.SetResponseBody([]byte(`{}`))
				_, _, _ = s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{ExperimentID: "e"})
			case 3:
				_, _, _ = s.handleAlertListUnacknowledged(context.Background(), nil, alertListInput{})
			case 4:
				rec.SetResponseBody([]byte(`{"status":"ok"}`))
				_, _, _ = s.handleSystemGetHealth(context.Background(), nil, struct{}{})
			}
		}(i)
	}
	wg.Wait()
}
