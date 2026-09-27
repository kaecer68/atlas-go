package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// ---------------------------------------------------------------------------
// Issue #1897 — production /annotate must run on the LLM Router
//
// These tests exercise the REAL production wiring helper
// (setupStrategiesAnnotator) against the REAL strategies HTTP handler, the REAL
// default routing table and a spy provider that stands in for the MiniMax node
// of the failure_attribution chain. They also wire a REAL legacy
// llm_annotator.KimiClient in front of a counting HTTP server, so "the annotate
// path never calls the provider client directly" is an observed fact
// (request count 0) rather than a claim about the code.
// ---------------------------------------------------------------------------

// annotateSeedsJSON is a minimal strategy registry: one active frame whose
// /annotate call can succeed.
const annotateSeedsJSON = `[
  {"id":"alpha","name":"alpha","layer":"L1","summary":"us rate down",
   "direction":"up","risk":"medium","source":"backtest","status":"active",
   "attribution_mode":"rule_based","attribution":["seed_attribution"],
   "conditions":[{"field":"DXY.ChangePct","operator":"lt","value":-0.3,"string_value":"","timeframe":"1D","source":"us_yahoo"}]}
]`

// spyProvider is an llm.ProviderImpl that records every request it receives and
// answers with a fixed annotation. provider is the router identity it registers
// under, so tests can place it at any chain position.
type spyProvider struct {
	provider llm.Provider
	output   string

	mu   sync.Mutex
	reqs []llm.Request
}

func (s *spyProvider) Supports(capability llm.Capability) bool {
	return capability == llm.CapabilityFailureAttribution
}

func (s *spyProvider) Call(_ context.Context, req llm.Request) (llm.Response, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.mu.Unlock()
	return llm.Response{Output: s.output, Provider: s.provider}, nil
}

func (s *spyProvider) Health() llm.HealthStatus {
	return llm.HealthStatus{Provider: s.provider, Healthy: true}
}

func (s *spyProvider) calls() []llm.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Request, len(s.reqs))
	copy(out, s.reqs)
	return out
}

// annotateTestEnv is the assembled production-shaped harness.
type annotateTestEnv struct {
	dashboard *monitoring.DashboardAPI
	mux       *http.ServeMux
	routerCfg llm.RouterConfig
	spy       *spyProvider
	legacy    *llm_annotator.KimiClient
	// legacyHits counts HTTP requests that reached the legacy provider client's
	// endpoint. It must stay 0: any other value means the /annotate path called
	// the provider directly instead of going through the Router.
	legacyHits *int32
}

func newAnnotateTestEnv(t *testing.T) *annotateTestEnv {
	t.Helper()
	t.Setenv("ATLAS_API_KEY", "test-key")

	hits := new(int32)
	legacySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(hits, 1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"LEGACY-DIRECT-CALL"}}],` +
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(legacySrv.Close)

	legacy, err := llm_annotator.NewKimiClient(llm_annotator.Config{
		APIKey:  "legacy-key",
		BaseURL: legacySrv.URL,
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}

	reg, err := strategy_techniques.LoadFromBytes([]byte(annotateSeedsJSON))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}

	//lint:ignore SA1019 test stub: no gateway fetcher needed for route wiring
	dashboard := monitoring.NewDashboardAPI(t.TempDir(), t.TempDir(), nil)
	dashboard.SetStrategiesHandlers(apistrategies.NewHandlers(reg, nil))

	mux := http.NewServeMux()
	dashboard.RegisterStrategiesRoutes(mux)
	dashboard.RegisterCrossMarketRoutes(mux)

	// The production routing table: ResolveRouterConfig falls back to the
	// built-in table when configs/llm_router.yaml is not reachable from the test
	// working directory, and both tables are kept equal by
	// TestDefaultRoutingTable_MatchesYAML.
	routerCfg, _ := llm.ResolveRouterConfig()

	return &annotateTestEnv{
		dashboard:  dashboard,
		mux:        mux,
		routerCfg:  routerCfg,
		spy:        &spyProvider{provider: llm.ProviderMiniMax, output: "外資賣超導致策略未觸發"},
		legacy:     legacy,
		legacyHits: hits,
	}
}

func (e *annotateTestEnv) postAnnotate(t *testing.T, id string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/strategies/"+id+"/annotate",
		bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key")
	rr := httptest.NewRecorder()
	e.mux.ServeHTTP(rr, req)

	var body map[string]any
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
	}
	return rr.Code, body
}

func (e *annotateTestEnv) getCost(t *testing.T) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/llm_annotator/cost", nil)
	rr := httptest.NewRecorder()
	e.mux.ServeHTTP(rr, req)

	var body map[string]any
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
	}
	return rr.Code, body
}

// TestSetupStrategiesAnnotator_RoutesAnnotateThroughRouter is the end-to-end
// proof for issue #1897: with production wiring applied, POST /annotate lands
// on the Router's failure_attribution chain, carries the shared prompt and
// request options, and never touches the legacy provider client.
func TestSetupStrategiesAnnotator_RoutesAnnotateThroughRouter(t *testing.T) {
	env := newAnnotateTestEnv(t)

	router := llm.NewDefaultRouterFromConfig(env.routerCfg, env.spy)

	wiring := setupStrategiesAnnotator(annotateWiringDeps{
		Dashboard: env.dashboard,
		Router:    router,
		Config:    env.routerCfg,
		Legacy:    env.legacy,
	})
	if !wiring.Enabled {
		t.Fatalf("annotate wiring not enabled: reason=%q (want the Router path)", wiring.Reason)
	}
	if wiring.Backend == "kimi" || !strings.Contains(wiring.Backend, "router") {
		t.Fatalf("annotate backend = %q, want the router chain (never the legacy client)", wiring.Backend)
	}

	code, body := env.postAnnotate(t, "alpha")
	if code != http.StatusOK {
		t.Fatalf("POST /annotate status = %d, want 200; body=%v", code, body)
	}
	if got := body["annotation"]; got != env.spy.output {
		t.Errorf("annotation = %v, want %q (the router provider's output)", got, env.spy.output)
	}
	if got := body["backend"]; got != wiring.Backend {
		t.Errorf("backend = %v, want %q", got, wiring.Backend)
	}
	// Response shape contract: the success body is exactly these three keys.
	for _, key := range []string{"id", "annotation", "backend"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response is missing key %q: %v", key, body)
		}
	}
	if len(body) != 3 {
		t.Errorf("response has %d keys (%v), want exactly 3 (id/annotation/backend)", len(body), body)
	}

	// The Router chain received the call exactly once.
	calls := env.spy.calls()
	if len(calls) != 1 {
		t.Fatalf("router provider calls = %d, want 1", len(calls))
	}
	req := calls[0]
	if req.Capability != llm.CapabilityFailureAttribution {
		t.Errorf("capability = %q, want %q", req.Capability, llm.CapabilityFailureAttribution)
	}
	if req.DataClass != llm.DataClassNonRegulated {
		t.Errorf("data class = %v, want %v", req.DataClass, llm.DataClassNonRegulated)
	}
	payload, ok := req.Payload.([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte (chat messages JSON)", req.Payload)
	}
	// One prompt for both entry points (#1887/#1891): the HTTP path must send
	// the same system prompt the Router capability handler sends.
	if !strings.Contains(string(payload), llm_annotator.FailureAttributionSystemPrompt) {
		t.Errorf("payload does not carry the shared failure-attribution prompt: %s", payload)
	}
	var messages struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(payload, &messages); err != nil {
		t.Fatalf("payload is not a messages object: %v (%s)", err, payload)
	}
	if len(messages.Messages) != 2 || messages.Messages[0].Role != "system" || messages.Messages[1].Role != "user" {
		t.Fatalf("payload messages = %+v, want [system, user] (Router handler contract)", messages.Messages)
	}
	if !strings.Contains(messages.Messages[1].Content, "alpha") {
		t.Errorf("user message does not describe the requested frame: %s", messages.Messages[1].Content)
	}
	// Same request options as capabilities.NewFailureAttributionHandler.
	if req.Options.MaxTokens != 2048 {
		t.Errorf("max_tokens = %d, want 2048 (ADR-012 reasoning floor, shared with the handler)", req.Options.MaxTokens)
	}
	if req.Options.Temperature != llm_annotator.FailureAttributionTemperature {
		t.Errorf("temperature = %v, want %v (shared sampling)", req.Options.Temperature, llm_annotator.FailureAttributionTemperature)
	}

	// Negative proof (behavioural): the legacy provider client was never called.
	if got := atomic.LoadInt32(env.legacyHits); got != 0 {
		t.Errorf("legacy KimiClient received %d HTTP request(s), want 0: /annotate bypassed the Router", got)
	}
	if n := len(env.legacy.RecentAnnotations(0)); n != 0 {
		t.Errorf("legacy KimiClient recorded %d annotation(s), want 0", n)
	}
}

// TestSetupStrategiesAnnotator_NoChainProviderKeeps503 pins the "not
// configured" contract: when no failure_attribution chain member is
// registered, /annotate must keep answering 503 (never 502, which would claim
// the LLM was tried and failed).
func TestSetupStrategiesAnnotator_NoChainProviderKeeps503(t *testing.T) {
	env := newAnnotateTestEnv(t)

	// Router with an empty provider set, exactly like a deployment without any
	// LLM key.
	router := llm.NewDefaultRouterFromConfig(env.routerCfg)

	wiring := setupStrategiesAnnotator(annotateWiringDeps{
		Dashboard: env.dashboard,
		Router:    router,
		Config:    env.routerCfg,
		Legacy:    env.legacy,
	})
	if wiring.Enabled {
		t.Fatalf("annotate wiring enabled without any chain provider: %+v", wiring)
	}
	if wiring.Reason != "no_failure_attribution_provider" {
		t.Errorf("reason = %q, want no_failure_attribution_provider", wiring.Reason)
	}

	code, body := env.postAnnotate(t, "alpha")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("POST /annotate status = %d, want 503; body=%v", code, body)
	}
	if _, ok := body["fallback"]; !ok {
		t.Errorf("503 body lost the rule-based fallback: %v", body)
	}
	if got := atomic.LoadInt32(env.legacyHits); got != 0 {
		t.Errorf("legacy KimiClient received %d HTTP request(s), want 0", got)
	}
}

// TestSetupStrategiesAnnotator_CostEndpointStaysNumeric covers the other half of
// issue #1897's acceptance: removing the concrete-type dependency must not
// break GET /api/llm_annotator/cost.
func TestSetupStrategiesAnnotator_CostEndpointStaysNumeric(t *testing.T) {
	env := newAnnotateTestEnv(t)
	router := llm.NewDefaultRouterFromConfig(env.routerCfg, env.spy)

	setupStrategiesAnnotator(annotateWiringDeps{
		Dashboard: env.dashboard,
		Router:    router,
		Config:    env.routerCfg,
		Legacy:    env.legacy,
	})

	code, body := env.getCost(t)
	if code != http.StatusOK {
		t.Fatalf("GET /api/llm_annotator/cost status = %d, want 200; body=%v", code, body)
	}
	for _, key := range []string{"provider", "total_tokens", "total_requests", "total_cost", "generated_at"} {
		if _, ok := body[key]; !ok {
			t.Errorf("cost report is missing key %q: %v", key, body)
		}
	}
}

// TestSetupStrategiesAnnotator_NoLegacyClientStillRoutes documents that the
// legacy client is optional: with no key at all the Router path must still be
// wired as soon as a chain provider exists.
func TestSetupStrategiesAnnotator_NoLegacyClientStillRoutes(t *testing.T) {
	env := newAnnotateTestEnv(t)
	router := llm.NewDefaultRouterFromConfig(env.routerCfg, env.spy)

	wiring := setupStrategiesAnnotator(annotateWiringDeps{
		Dashboard: env.dashboard,
		Router:    router,
		Config:    env.routerCfg,
		Legacy:    nil,
	})
	if !wiring.Enabled {
		t.Fatalf("annotate wiring not enabled without a legacy client: %+v", wiring)
	}

	code, body := env.postAnnotate(t, "alpha")
	if code != http.StatusOK {
		t.Fatalf("POST /annotate status = %d, want 200; body=%v", code, body)
	}
	// Cost endpoint has no source wired → 503, unchanged behaviour.
	costCode, _ := env.getCost(t)
	if costCode != http.StatusServiceUnavailable {
		t.Errorf("GET /api/llm_annotator/cost status = %d, want 503 (no usage source)", costCode)
	}
}

// TestFailureAttributionRouterReady checks the readiness predicate directly,
// including the off-chain registration that must NOT make /annotate look
// configured (the legacy AnnotatorAdapter registers as ProviderKimi, which no
// routing chain contains).
func TestFailureAttributionRouterReady(t *testing.T) {
	cfg, _ := llm.ResolveRouterConfig()

	tests := []struct {
		name     string
		router   llm.Router
		expected bool
	}{
		{
			name:     "chain primary registered",
			router:   llm.NewDefaultRouterFromConfig(cfg, &spyProvider{provider: llm.ProviderMiniMax}),
			expected: true,
		},
		{
			name:     "only chain backup registered",
			router:   llm.NewDefaultRouterFromConfig(cfg, &spyProvider{provider: llm.ProviderDeepSeek}),
			expected: true,
		},
		{
			name:     "no provider registered",
			router:   llm.NewDefaultRouterFromConfig(cfg),
			expected: false,
		},
		{
			name:     "only off-chain provider registered",
			router:   llm.NewDefaultRouterFromConfig(cfg, &spyProvider{provider: llm.ProviderKimi}),
			expected: false,
		},
		{
			name:     "nil router",
			router:   nil,
			expected: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureAttributionRouterReady(tc.router, cfg); got != tc.expected {
				t.Errorf("failureAttributionRouterReady() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestSetupStrategiesAnnotator_EmitsFailureAttributionSpan asserts the
// observability contract from issue #1897's acceptance list: the request
// produces an `llm.failure_attribution` span whose
// `llm.attempted_providers` attribute lists the chain member that served it.
// The span is emitted by DefaultRouter.Call, so this is independent evidence
// (beyond the response body) that /annotate really traversed the Router.
func TestSetupStrategiesAnnotator_EmitsFailureAttributionSpan(t *testing.T) {
	env := newAnnotateTestEnv(t)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = tp.Shutdown(context.Background())
	})

	router := llm.NewDefaultRouterFromConfig(env.routerCfg, env.spy)
	wiring := setupStrategiesAnnotator(annotateWiringDeps{
		Dashboard: env.dashboard,
		Router:    router,
		Config:    env.routerCfg,
		Legacy:    env.legacy,
	})
	if !wiring.Enabled {
		t.Fatalf("annotate wiring not enabled: %+v", wiring)
	}

	if code, body := env.postAnnotate(t, "alpha"); code != http.StatusOK {
		t.Fatalf("POST /annotate status = %d, want 200; body=%v", code, body)
	}

	var spanNames []string
	found := false
	for _, span := range exporter.GetSpans() {
		spanNames = append(spanNames, span.Name)
		if span.Name != "llm."+string(llm.CapabilityFailureAttribution) {
			continue
		}
		found = true
		attrs := make(map[string]string, len(span.Attributes))
		for _, kv := range span.Attributes {
			attrs[string(kv.Key)] = kv.Value.String()
		}
		// attribute.Value.String renders a string slice as a JSON array.
		if got := attrs["llm.attempted_providers"]; got != `["minimax"]` {
			t.Errorf("llm.attempted_providers = %q, want %q (the MiniMax chain member served it)", got, `["minimax"]`)
		}
	}
	if !found {
		t.Fatalf("no llm.failure_attribution span emitted; spans=%v", spanNames)
	}
}
