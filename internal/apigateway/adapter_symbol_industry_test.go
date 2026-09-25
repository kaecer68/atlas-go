package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ─── symbol_industry channel (issue #1943) ──────────────────────────────────

// newSymbolIndustryTestAdapter wires the adapter against two stub upstreams so
// the whole channel (fetch -> map -> persist -> contract evaluation) can be
// exercised offline.
func newSymbolIndustryTestAdapter(t *testing.T) *SymbolIndustryChannelAdapter {
	t.Helper()

	// The provider rate-limits through the shared TWSE / TPEx token buckets;
	// tests must not queue behind the production policy.
	restoreTWSE := marketdata.SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { marketdata.SetTWSESharedLimiterForTest(restoreTWSE) })
	restoreTPEx := marketdata.SetTPExSharedLimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { marketdata.SetTPExSharedLimiterForTest(restoreTPEx) })

	twse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"出表日期":"1150923","公司代號":"2330","公司名稱":"台灣積體電路製造股份有限公司","產業別":"24"},
			{"出表日期":"1150923","公司代號":"1101","公司名稱":"臺灣水泥股份有限公司","產業別":"01"},
			{"出表日期":"1150923","公司代號":"9103","公司名稱":"美德醫療-DR","產業別":"91"}
		]`))
	}))
	t.Cleanup(twse.Close)

	tpex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"1150924","SecuritiesCompanyCode":"8069","CompanyName":"元太科技工業股份有限公司","SecuritiesIndustryCode":"26"},
			{"Date":"1150924","SecuritiesCompanyCode":"1240","CompanyName":"茂生農經股份有限公司","SecuritiesIndustryCode":"33"}
		]`))
	}))
	t.Cleanup(tpex.Close)

	provider := marketdata.NewSymbolIndustryProvider(t.TempDir())
	provider.SetBaseURLs(twse.URL, tpex.URL)
	provider.SetHTTPClient(twse.Client())

	adapter := NewSymbolIndustryChannelAdapter(t.TempDir())
	adapter.SetProvider(provider)
	return adapter
}

func TestSymbolIndustryChannelAdapter_FetchReportsPopulation(t *testing.T) {
	adapter := newSymbolIndustryTestAdapter(t)

	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Stale {
		t.Error("a successful fetch must not be marked stale")
	}
	if res.Meta.ChannelID != "symbol_industry" {
		t.Errorf("channel id = %q", res.Meta.ChannelID)
	}

	var payload struct {
		Channel  string         `json:"channel"`
		Sources  []string       `json:"sources"`
		Counts   map[string]int `json:"counts"`
		L1Counts map[string]int `json:"l1_counts"`
		File     string         `json:"state_file"`
	}
	if err := json.Unmarshal(res.Data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Channel != "symbol_industry" {
		t.Errorf("payload channel = %q", payload.Channel)
	}
	if payload.Counts["total"] != 5 {
		t.Errorf("counts.total = %d, want 5", payload.Counts["total"])
	}
	if payload.Counts["mapped"] != 3 || payload.Counts["unmapped"] != 1 || payload.Counts["unknown"] != 1 {
		t.Errorf("counts = %v, want mapped=3 unmapped=1 unknown=1", payload.Counts)
	}
	if payload.Counts["canonical_l1"] != 3 {
		t.Errorf("canonical_l1 = %d, want 3 (cement, optoelectronics, semiconductor)", payload.Counts["canonical_l1"])
	}
	if len(payload.L1Counts) != 3 {
		t.Errorf("l1_counts = %v", payload.L1Counts)
	}
	if payload.File == "" {
		t.Error("payload must point at the persisted state file")
	}
	if _, err := filepath.Abs(payload.File); err != nil {
		t.Errorf("state_file is not an absolute path: %q", payload.File)
	}

	// The gateway payload must NOT carry the ~2000 rows: that is the state
	// file's job, and the cache must not grow by ~600 KB per fetch.
	if len(res.Data) > 4096 {
		t.Errorf("gateway payload is %d bytes; it must stay a summary", len(res.Data))
	}
}

func TestSymbolIndustryChannelAdapter_DataStateAndHealth(t *testing.T) {
	adapter := newSymbolIndustryTestAdapter(t)
	ctx := context.Background()

	// Before any fetch: nothing on disk, and the contract must not see "ok".
	ds, err := adapter.DataState(ctx)
	if err != nil {
		t.Fatalf("DataState: %v", err)
	}
	if ds.Present || ds.NonZero {
		t.Fatalf("empty state must be reported as absent, got %+v", ds)
	}
	health, err := adapter.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if health.Status != "warn" {
		t.Errorf("health with no snapshot = %q, want warn", health.Status)
	}
	if health.CheckType != "readiness" {
		t.Errorf("check type = %q, want readiness", health.CheckType)
	}

	if _, err := adapter.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	ds, err = adapter.DataState(ctx)
	if err != nil {
		t.Fatalf("DataState after fetch: %v", err)
	}
	if !ds.Present || !ds.NonZero {
		t.Fatalf("populated state must be present and non-zero, got %+v", ds)
	}
	if ds.RecordedAt.IsZero() {
		t.Error("RecordedAt must carry the snapshot timestamp")
	}

	health, err = adapter.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck after fetch: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("health after fetch = %q, want ok", health.Status)
	}
}

// TestSymbolIndustryContract_DegradedOnEmpty pins the #1953 rule for this new
// channel: an empty or missing snapshot degrades the channel instead of
// reporting a successful "ok".
func TestSymbolIndustryContract_DegradedOnEmpty(t *testing.T) {
	contract := ChannelContracts().Contract("symbol_industry")
	if contract.HealthSource != HealthSourceFileState {
		t.Errorf("HealthSource = %q, want %q", contract.HealthSource, HealthSourceFileState)
	}
	if contract.SuccessCriteria != SuccessCriteriaValueNonzero {
		t.Errorf("SuccessCriteria = %q, want %q", contract.SuccessCriteria, SuccessCriteriaValueNonzero)
	}
	if !contract.DegradedOnEmpty {
		t.Error("DegradedOnEmpty must be true: an empty population is a degraded channel")
	}
	if len(contract.SourcePriority) == 0 || contract.SourcePriority[0] != "TWSE" {
		t.Errorf("SourcePriority = %v, want first-party TWSE/TPEx", contract.SourcePriority)
	}

	// Contract evaluation: an "ok" adapter result is downgraded while the data
	// is missing, and passes through once real data exists.
	adapter := newSymbolIndustryTestAdapter(t)
	ctx := context.Background()
	base := HealthStatus{Status: "ok", UpdatedAt: time.Now().Format(time.RFC3339)}

	got := EvaluateContractHealth(ctx, contract, adapter, base)
	if got.Status != "degraded" {
		t.Fatalf("empty data must degrade an ok result, got %q", got.Status)
	}

	if _, err := adapter.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := EvaluateContractHealth(ctx, contract, adapter, base); got.Status != "ok" {
		t.Fatalf("populated data must keep ok, got %q (%s)", got.Status, got.LastError)
	}
}

func TestSymbolIndustryChannelAdapter_Metadata(t *testing.T) {
	adapter := NewSymbolIndustryChannelAdapter(t.TempDir())
	meta := adapter.Metadata()
	if meta.ChannelID != "symbol_industry" {
		t.Errorf("ChannelID = %q", meta.ChannelID)
	}
	if !meta.HasLimiter {
		t.Error("HasLimiter must be true")
	}
	if adapter.RateLimit() == nil {
		t.Error("RateLimit must not be nil")
	}
}
