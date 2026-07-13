# Dashboard Frontend Sync for Stock Screening Layer — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the Dashboard frontend fully aware of the stock screening layer by persisting per-session screening rejects and displaying them in the Agent Observatory and Pipeline pages.

**Architecture:** Add a `ScreenResult` type to the screener so it can report *why* a symbol failed. Thread a `sessionID` and reject accumulator through `collectRecommendations` so rejects are captured at session time and persisted to `screened_symbols.jsonl` via the ledger. The dashboard API reads this JSONL on demand. The frontend renders criteria badges in the Agent Observatory and a toggleable screened-out table in the Pipeline page.

**Tech Stack:** Go 1.25, vanilla HTML/JS/CSS (no framework), JSONL append-only storage.

---

## Pre-Flight Checks

Run these before starting:

```bash
go test ./internal/screener/... ./internal/orchestrator/... ./internal/ledger/... ./internal/monitoring/...
test -z "$(gofmt -l .)"
```

Expected: all tests pass, no unformatted files.

---

### Task 1: Add `ScreeningReject` domain type

**Files:**
- Modify: `internal/domain/screening.go`
- Test: `internal/domain/screening_test.go` (create)

**Step 1: Write the failing test**

Create `internal/domain/screening_test.go`:

```go
package domain

import "testing"

func TestScreeningRejectStruct(t *testing.T) {
	reject := ScreeningReject{
		SessionID:      "session-20260101-daily",
		Symbol:         "2330.TW",
		AgentID:        "semi-desk-01",
		Skill:          "semiconductor_desk",
		Criterion:      "volume_intraday_min",
		CriterionLabel: "Volume ≥ 1,000,000",
		Threshold:      "1000000",
		ActualValue:    "850000",
	}
	if reject.Symbol != "2330.TW" {
		t.Fatal("unexpected symbol")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/ -run TestScreeningRejectStruct -v
```

Expected: FAIL — `ScreeningReject` undefined.

**Step 3: Add `ScreeningReject` to `internal/domain/screening.go`**

Append to `internal/domain/screening.go` (after `HasFilters`):

```go
// ScreeningReject records a single symbol-agent screening failure for audit.
type ScreeningReject struct {
	SessionID      string    `json:"session_id"`
	Symbol         string    `json:"symbol"`
	AgentID        string    `json:"agent_id"`
	Skill          string    `json:"skill"`
	Criterion      string    `json:"criterion"`
	CriterionLabel string    `json:"criterion_label"`
	Threshold      string    `json:"threshold"`
	ActualValue    string    `json:"actual_value"`
	RecordedAt     time.Time `json:"recorded_at"`
}
```

Also add `import "time"` if not present.

**Step 4: Run test to verify it passes**

```bash
go test ./internal/domain/ -run TestScreeningRejectStruct -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/screening.go internal/domain/screening_test.go
git commit -m "feat(domain): add ScreeningReject type for audit trail"
```

---

### Task 2: Add `ScreenResult` and `ScreenDetailed` to screener

**Files:**
- Modify: `internal/screener/screener.go`
- Test: `internal/screener/engine_test.go`

**Step 1: Write the failing test**

Add to `internal/screener/engine_test.go`:

```go
func TestScreenDetailedVolumeFail(t *testing.T) {
	fe := portfolio.NewFactorEngine(nil)
	fp := portfolio.NewFundamentalProvider(nil)
	e := NewEngine(fe, fp)
	minVol := int64(1000000)
	criteria := domain.ScreeningCriteria{
		VolumeIntraday: &domain.MinFilter{Min: &minVol},
	}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true},
	}
	res, err := e.ScreenDetailed(context.Background(), "2330.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion == "" {
		t.Fatal("expected criterion to be set")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/screener/ -run TestScreenDetailedVolumeFail -v
```

Expected: FAIL — `ScreenDetailed` undefined.

**Step 3: Implement `ScreenResult` and `ScreenDetailed`**

In `internal/screener/screener.go`:

1. Add the type after the `Engine` struct:

```go
// ScreenResult carries the outcome of a single screening evaluation.
type ScreenResult struct {
	Passed    bool
	Reason    string
	Criterion string
	Label     string
	Threshold string
	Actual    string
}
```

2. Add `ScreenDetailed` method. Convert the existing `Screen` body into `ScreenDetailed`, returning `ScreenResult` at every `return false` and `return true`. Then make `Screen` a thin wrapper:

```go
func (e *Engine) Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error) {
	res, err := e.ScreenDetailed(ctx, symbol, criteria, quotes)
	return res.Passed, err
}

func (e *Engine) ScreenDetailed(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (ScreenResult, error) {
	pass := func() ScreenResult {
		_ = ctx
		return ScreenResult{Passed: true}
	}
	fail := func(criterion, label, threshold, actual string) ScreenResult {
		return ScreenResult{
			Passed:    false,
			Reason:    fmt.Sprintf("%s: %s (threshold %s, actual %s)", criterion, label, threshold, actual),
			Criterion: criterion,
			Label:     label,
			Threshold: threshold,
			Actual:    actual,
		}
	}

	if !criteria.HasFilters() {
		return pass(), nil
	}

	quote, hasQuote := quotes[symbol]

	if criteria.VolumeIntraday != nil && criteria.VolumeIntraday.Min != nil {
		if !hasQuote {
			return fail("volume_intraday_min", "Volume intraday", fmt.Sprintf("%d", *criteria.VolumeIntraday.Min), "missing quote"), nil
		}
		minVol := *criteria.VolumeIntraday.Min
		if quote.Volume < minVol {
			return fail("volume_intraday_min", "Volume intraday", fmt.Sprintf("%d", minVol), fmt.Sprintf("%d", quote.Volume)), nil
		}
	}

	if e.fundamentals != nil && e.fundamentals.HasData() {
		data := e.fundamentals.Get(symbol)

		if criteria.PE != nil {
			if data.PE > 0 {
				if criteria.PE.Min != nil && data.PE < *criteria.PE.Min {
					return fail("pe_min", "P/E", fmt.Sprintf("%.2f", *criteria.PE.Min), fmt.Sprintf("%.2f", data.PE)), nil
				}
				if criteria.PE.Max != nil && data.PE > *criteria.PE.Max {
					return fail("pe_max", "P/E", fmt.Sprintf("%.2f", *criteria.PE.Max), fmt.Sprintf("%.2f", data.PE)), nil
				}
			} else {
				return fail("pe_missing", "P/E", "required", "missing data"), nil
			}
		}

		if criteria.PB != nil {
			if data.PB > 0 {
				if criteria.PB.Min != nil && data.PB < *criteria.PB.Min {
					return fail("pb_min", "P/B", fmt.Sprintf("%.2f", *criteria.PB.Min), fmt.Sprintf("%.2f", data.PB)), nil
				}
				if criteria.PB.Max != nil && data.PB > *criteria.PB.Max {
					return fail("pb_max", "P/B", fmt.Sprintf("%.2f", *criteria.PB.Max), fmt.Sprintf("%.2f", data.PB)), nil
				}
			} else {
				return fail("pb_missing", "P/B", "required", "missing data"), nil
			}
		}

		if criteria.DividendYield != nil {
			if data.DividendYield > 0 {
				if criteria.DividendYield.Min != nil && data.DividendYield < *criteria.DividendYield.Min {
					return fail("dividend_yield_min", "Dividend yield", fmt.Sprintf("%.2f", *criteria.DividendYield.Min), fmt.Sprintf("%.2f", data.DividendYield)), nil
				}
				if criteria.DividendYield.Max != nil && data.DividendYield > *criteria.DividendYield.Max {
					return fail("dividend_yield_max", "Dividend yield", fmt.Sprintf("%.2f", *criteria.DividendYield.Max), fmt.Sprintf("%.2f", data.DividendYield)), nil
				}
			}
		}
	}

	if e.factorEngine != nil {
		if criteria.Momentum20Day != nil {
			momentum := e.factorEngine.CalculateMomentumScore(symbol, quotes)
			if criteria.Momentum20Day.Min != nil && momentum < *criteria.Momentum20Day.Min {
				return fail("momentum_20d_min", "20-day momentum", fmt.Sprintf("%.2f", *criteria.Momentum20Day.Min), fmt.Sprintf("%.2f", momentum)), nil
			}
			if criteria.Momentum20Day.Max != nil && momentum > *criteria.Momentum20Day.Max {
				return fail("momentum_20d_max", "20-day momentum", fmt.Sprintf("%.2f", *criteria.Momentum20Day.Max), fmt.Sprintf("%.2f", momentum)), nil
			}
		}

		if criteria.MinTotalFactorScore != nil {
			defaultWeights := map[portfolio.FactorType]float64{
				portfolio.FactorMomentum: 0.30,
				portfolio.FactorValue:    0.25,
				portfolio.FactorQuality:  0.25,
				portfolio.FactorAgent:    0.20,
			}
			scores := e.factorEngine.CalculateAllScores(symbol, quotes, nil, nil, defaultWeights)
			total, ok := scores["total"]
			if !ok || total < *criteria.MinTotalFactorScore {
				actual := "missing"
				if ok {
					actual = fmt.Sprintf("%.3f", total)
				}
				return fail("min_total_factor_score", "Total factor score", fmt.Sprintf("%.3f", *criteria.MinTotalFactorScore), actual), nil
			}
		}
	}

	return pass(), nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/screener/... -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/screener/screener.go internal/screener/engine_test.go
git commit -m "feat(screener): add ScreenDetailed with reject reason metadata"
```

---

### Task 3: Add `ScreenDetailed` to `PluginRegistry`

**Files:**
- Modify: `internal/orchestrator/plugin_registry.go`
- Test: `internal/orchestrator/plugin_registry_test.go` (or existing registry_test.go)

**Step 1: Write the failing test**

In `internal/orchestrator/registry_test.go` (or create `plugin_registry_test.go`):

```go
func TestPluginRegistryScreenDetailed(t *testing.T) {
	r := NewPluginRegistry()
	minVol := int64(1000000)
	agent := domain.AgentSpec{
		ID: "test-agent",
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: &minVol},
		},
	}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true}}
	res, err := r.ScreenDetailed(context.Background(), agent, "2330.TW", quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion == "" {
		t.Fatal("expected criterion")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrator/ -run TestPluginRegistryScreenDetailed -v
```

Expected: FAIL — `ScreenDetailed` undefined.

**Step 3: Implement `ScreenDetailed` on `PluginRegistry`**

Add to `internal/orchestrator/plugin_registry.go`:

```go
func (r *PluginRegistry) ScreenDetailed(ctx context.Context, agent domain.AgentSpec, symbol string, quotes map[string]domain.Quote) (screener.ScreenResult, error) {
	if r.screener == nil || !agent.ScreeningCriteria.HasFilters() {
		return screener.ScreenResult{Passed: true}, nil
	}
	return r.screener.ScreenDetailed(ctx, symbol, agent.ScreeningCriteria, quotes)
}
```

**Step 4: Run test**

```bash
go test ./internal/orchestrator/ -run TestPluginRegistryScreenDetailed -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/orchestrator/plugin_registry.go internal/orchestrator/registry_test.go
git commit -m "feat(orchestrator): expose ScreenDetailed on PluginRegistry"
```

---

### Task 4: Ledger read/write for screening rejects

**Files:**
- Modify: `internal/ledger/ledger.go`
- Test: `internal/ledger/ledger_test.go`

**Step 1: Write the failing test**

Add to `internal/ledger/ledger_test.go`:

```go
func TestRecordAndLoadSessionScreeningRejects(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	rejects := []domain.ScreeningReject{
		{SessionID: "s1", Symbol: "2330.TW", AgentID: "a1", Criterion: "pe_max"},
		{SessionID: "s1", Symbol: "2317.TW", AgentID: "a2", Criterion: "volume_min"},
	}
	if err := s.RecordSessionScreeningRejects("s1", rejects); err != nil {
		t.Fatalf("record failed: %v", err)
	}
	loaded, err := s.LoadSessionScreeningRejects("s1")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rejects, got %d", len(loaded))
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ledger/ -run TestRecordAndLoadSessionScreeningRejects -v
```

Expected: FAIL — methods undefined.

**Step 3: Implement methods**

Add to `internal/ledger/ledger.go` (near existing session methods):

```go
func (s *Store) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	if err := os.MkdirAll(s.sessionDir(sessionID), 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.sessionDir(sessionID), "screened_symbols.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rejects {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	path := filepath.Join(s.sessionDir(sessionID), "screened_symbols.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	rejects := make([]domain.ScreeningReject, 0)
	for scanner.Scan() {
		var rec domain.ScreeningReject
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode screening reject: %w", err)
		}
		rejects = append(rejects, rec)
	}
	return rejects, scanner.Err()
}
```

**Step 4: Run test**

```bash
go test ./internal/ledger/ -run TestRecordAndLoadSessionScreeningRejects -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/ledger/ledger.go internal/ledger/ledger_test.go
git commit -m "feat(ledger): add per-session screening reject persistence"
```

---

### Task 5: Thread `sessionID` and reject accumulator through `collectRecommendations`

**Files:**
- Modify: `internal/orchestrator/executors.go`
- Modify: `internal/orchestrator/executors_test.go`

**Step 1: Update `collectRecommendations` signature and body**

In `internal/orchestrator/executors.go`, change:

```go
func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime) []domain.Recommendation {
```

to:

```go
func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime, sessionID string) ([]domain.Recommendation, []domain.ScreeningReject) {
```

Update the body:

```go
func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime, sessionID string) ([]domain.Recommendation, []domain.ScreeningReject) {
	recs := make([]domain.Recommendation, 0)
	rejects := make([]domain.ScreeningReject, 0)
	now := time.Now().UTC()
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		prompt := plugins.ResolvePrompt(agent, overrides)
		symbols := agent.Universe
		if len(symbols) == 0 {
			symbols = slices.Collect(symbolIterator(DefaultSymbols()))
		}

		for _, symbol := range symbols {
			quote, ok := quotes[symbol]
			if !ok || !quote.IsTradable {
				continue
			}
			screenRes, err := plugins.ScreenDetailed(context.Background(), agent, symbol, quotes)
			if err != nil || !screenRes.Passed {
				if !screenRes.Passed {
					rejects = append(rejects, domain.ScreeningReject{
						SessionID:      sessionID,
						Symbol:         symbol,
						AgentID:        agent.ID,
						Skill:          agent.Skill,
						Criterion:      screenRes.Criterion,
						CriterionLabel: screenRes.Label,
						Threshold:      screenRes.Threshold,
						ActualValue:    screenRes.Actual,
						RecordedAt:     now,
					})
				}
				continue
			}
			rec, ok := plugins.Recommendation(agent, quote, prompt, regime)
			if !ok {
				continue
			}
			recs = append(recs, rec)
		}
	}
	return recs, rejects
}
```

Add `import "time"` to `internal/orchestrator/executors.go` if missing.

**Step 2: Update all callers**

Find/replace every call to `collectRecommendations(...)` to add `sessionID` and handle the new second return value. Call sites are in the same file:

- `executeRegistryResearchDetailedWithPolicyAndGuards`
- `executeRegistryResearchWithDarwinianWeights`

For functions that don't have a `sessionID` available, pass `""` and ignore rejects for now. We will wire the real sessionID in Task 6.

**Step 3: Update tests**

In `internal/orchestrator/executors_test.go`, every call to `collectRecommendations` must be updated. Since tests don't have a session context, pass `""` and ignore the second return:

```go
baseline, _ := collectRecommendations(registry, quoteBySymbol, plugins, map[string]string{}, domain.RegimeNeutral, "")
```

**Step 4: Run tests**

```bash
go test ./internal/orchestrator/... -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/orchestrator/executors.go internal/orchestrator/executors_test.go
git commit -m "feat(orchestrator): capture ScreeningRejects in collectRecommendations"
```

---

### Task 6: Wire session recording into the backtest/simulation loop

**Files:**
- Search and modify: files that call `RecordSessionOutcomes` and `collectRecommendations`

**Goal:** Ensure the real `sessionID` reaches `collectRecommendations` and rejects are persisted.

**Step 1: Identify the backtest/simulation entry point**

Search for `RecordSessionOutcomes`:

```bash
grep -r "RecordSessionOutcomes" --include="*.go" .
```

Likely candidates: `internal/backtest/runner.go`, `internal/sim/engine.go`, `cmd/atlas/main.go`, or `internal/orchestrator/system.go`.

**Step 2: Thread sessionID into the executor path**

If the session runner calls `ExecuteRegistryResearchDetailedWithPolicyAndGuards`, we need a variant that accepts `sessionID` and returns rejects. Options:

- Add a new exported wrapper: `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndSession`
- Or modify the existing `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins` to also take `sessionID`.

Minimal change: modify `executeRegistryResearchDetailedWithPolicyAndGuards` signature to accept `sessionID string` and return rejects. Then update its exported wrappers.

Example change in `executors.go`:

```go
func ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome, []domain.ScreeningReject) {
	return executeRegistryResearchDetailedWithPolicyAndGuards(registry, quotes, overrides, policy, NewPluginRegistry(), "")
}
```

And the internal function:

```go
func executeRegistryResearchDetailedWithPolicyAndGuards(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy, plugins *PluginRegistry, sessionID string) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome, []domain.ScreeningReject) {
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw, rejects := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime, sessionID)
	final, guardOutcomes := applyControlLayerWithOutcomes(registry, plugins, raw, policy)
	return regime, raw, final, guardOutcomes, rejects
}
```

Update **all** exported wrappers and their callers accordingly.

**Step 3: Persist rejects after simulation**

In the file that calls `RecordSessionOutcomes` (e.g., `internal/backtest/runner.go` or `internal/sim/engine.go`), add:

```go
if err := ledgerStore.RecordSessionScreeningRejects(session.ID, rejects); err != nil {
    log.Printf("[%s] warn: failed to record screening rejects: %v", session.ID, err)
}
```

**Step 4: Run tests**

```bash
go test ./internal/orchestrator/... ./internal/sim/... ./internal/backtest/... -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/orchestrator/executors.go internal/orchestrator/executors_test.go <other changed files>
git commit -m "feat(orchestrator): thread sessionID and persist screening rejects"
```

---

### Task 7: Dashboard API — expose screened rejects in pipeline

**Files:**
- Modify: `internal/monitoring/dashboard_api.go`
- Test: `internal/monitoring/dashboard_api_test.go`

**Step 1: Add `ScreenedItems` to pipeline response**

In `internal/monitoring/dashboard_api.go`, update `RecommendationPipelineResponse`:

```go
type RecommendationPipelineResponse struct {
	SessionID      string                   `json:"session_id"`
	Regime         domain.Regime            `json:"regime"`
	Items          []PipelineItem           `json:"items"`
	GuardOutcomes  []domain.GuardOutcome    `json:"guard_outcomes"`
	ScreenedItems  []domain.ScreeningReject `json:"screened_items"`
	RecordedAt     time.Time                `json:"recorded_at"`
}
```

**Step 2: Load rejects in `handleRecommendationPipeline`**

Inside `handleRecommendationPipeline`, after loading `outcomesPath`, add:

```go
store := ledger.NewStore(a.ledgerDir)
screened, _ := store.LoadSessionScreeningRejects(summary.SessionID)
```

Then include `ScreenedItems: screened` in the final `writeJSON` response.

**Step 3: Write test**

In `internal/monitoring/dashboard_api_test.go`, write a test that:
1. Creates a temp ledger dir
2. Writes a `summary.json` under `sessions/test-session/`
3. Writes `screened_symbols.jsonl` with 1 reject
4. Calls `handleRecommendationPipeline` via `httptest`
5. Asserts `screened_items` has length 1

**Step 4: Run tests**

```bash
go test ./internal/monitoring/... -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/monitoring/dashboard_api.go internal/monitoring/dashboard_api_test.go
git commit -m "feat(dashboard): expose ScreeningRejects in recommendation pipeline API"
```

---

### Task 8: Dashboard API — expose `screening_criteria` in universe overlap

**Files:**
- Modify: `internal/monitoring/dashboard_api.go`

**Step 1: Enrich `AgentUniverseView`**

```go
type AgentUniverseView struct {
	AgentID           string                   `json:"agent_id"`
	Name              string                   `json:"name"`
	Layer             string                   `json:"layer"`
	Universe          []string                 `json:"universe"`
	ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
}
```

**Step 2: Populate in `handleUniverseOverlap`**

In the loop that builds `AgentUniverseView`, add:

```go
agents = append(agents, AgentUniverseView{
	AgentID:           agent.ID,
	Name:              agent.Name,
	Layer:             string(agent.Layer),
	Universe:          universe,
	ScreeningCriteria: agent.ScreeningCriteria,
})
```

**Step 3: Run tests**

```bash
go test ./internal/monitoring/... -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/monitoring/dashboard_api.go
git commit -m "feat(dashboard): expose screening_criteria in universe overlap API"
```

---

### Task 9: Frontend — Workflow banner and Agent Observatory badges

**Files:**
- Modify: `web/static/index.html`

**Step 1: Insert screening step in workflow banner**

Find the `.workflow` element (inside `#page-experiments` or any shared workflow). Add a step for screening if it appears in the Pipeline page context. If the Pipeline page doesn't have its own workflow banner, add one near the top of `#page-pipeline`:

```html
<div class="workflow" style="margin-bottom:12px">
  <div class="step">AI 推薦</div><span class="arrow">→</span>
  <div class="step" id="workflowScreening">篩選層</div><span class="arrow">→</span>
  <div class="step">控制層</div><span class="arrow">→</span>
  <div class="step">模擬投組</div>
</div>
```

**Step 2: Add criteria badges to Agent Observatory**

In `renderAgentObservatory`, after building the table, add a new section below it that lists criteria badges per agent. Since `agentObservatory` data does not include criteria, fetch from the existing `/api/dashboard/universe-overlap` endpoint or rely on the same data source.

Better: modify the `loadAll()` function to also fetch `/api/dashboard/universe-overlap` and pass it to `renderAgentObservatory`.

In `renderAgentObservatory(data, overlapData)`:

```javascript
let criteriaHtml = '';
if (overlapData && overlapData.agents) {
  criteriaHtml = '<div style="margin-top:14px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">篩選條件</div>' +
    overlapData.agents.map(a => {
      const sc = a.screening_criteria || {};
      const badges = [];
      if (sc.pe && sc.pe.max != null) badges.push(`P/E≤${sc.pe.max}`);
      if (sc.pb && sc.pb.max != null) badges.push(`P/B≤${sc.pb.max}`);
      if (sc.dividend_yield && sc.dividend_yield.min != null) badges.push(`股息≥${sc.dividend_yield.min}%`);
      if (sc.volume_intraday && sc.volume_intraday.min != null) badges.push(`Vol≥${(sc.volume_intraday.min/10000).toFixed(0)}萬`);
      if (sc.momentum_20d && sc.momentum_20d.min != null) badges.push(`動能≥${sc.momentum_20d.min}`);
      if (sc.min_total_factor_score != null) badges.push(`因子≥${sc.min_total_factor_score}`);
      if (!badges.length) return '';
      return `<div style="margin:6px 0;font-size:12px"><strong>${agentName(a.agent_id)}</strong> <span style="color:var(--muted)">${badges.map(b => `<span class="badge info" style="cursor:help" title="${b}">${b}</span>`).join(' ')}</span></div>`;
    }).join('') + '</div>';
}
```

Append `criteriaHtml` to the observatory container innerHTML.

**Step 3: Verify in browser**

Run `go run ./cmd/atlas`, open `http://localhost:8080`, navigate to **AI 觀測台**, confirm badges appear for agents with `screening_criteria`.

**Step 4: Commit**

```bash
git add web/static/index.html
git commit -m "feat(dashboard): add screening layer banner and agent criteria badges"
```

---

### Task 10: Frontend — Pipeline toggle and screened-out table

**Files:**
- Modify: `web/static/index.html`

**Step 1: Add toggle and section HTML**

In the `renderPipeline` function, add a second checkbox after the existing `pipelineShowAll`:

```javascript
const showScreenedCheckbox = `
  <div style="margin-bottom:8px;font-size:12px">
    <label style="cursor:pointer;user-select:none">
      <input type="checkbox" id="pipelineShowScreened" ${showScreened ? 'checked' : ''} onchange="togglePipelineShowScreened(this)">
      <span style="margin-left:4px">顯示被篩選層排除的標的</span>
    </label>
  </div>
`;
```

Add a container for screened items:

```javascript
const screenedSection = (screenedItems && screenedItems.length && showScreened) ? `
  <div style="margin-top:12px">
    <h3 style="font-size:13px;color:var(--muted);margin-bottom:8px">被篩選層排除的標的（${screenedItems.length} 檔）</h3>
    <div style="max-height:260px;overflow:auto">
    <table style="font-size:12px">
      <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th>未通過條件</th><th>門檻</th><th>實際值</th></tr></thead>
      <tbody>
        ${screenedItems.map(s => `<tr style="border-left:3px solid #666;color:var(--muted)">
          <td>${s.symbol}</td>
          <td>${stockName(s.symbol) || '-'}</td>
          <td>${agentName(s.agent_id)}</td>
          <td>${s.criterion_label || s.criterion || '-'}</td>
          <td>${s.threshold || '-'}</td>
          <td>${s.actual_value || '-'}</td>
        </tr>`).join('')}
      </tbody>
    </table>
    </div>
  </div>
` : '';
```

Append `screenedSection` to the pipeline HTML output.

**Step 2: Add `togglePipelineShowScreened` handler**

```javascript
async function togglePipelineShowScreened(checkbox) {
  const sessionSelect = document.getElementById('pipelineSessionSelect');
  const sessionId = sessionSelect ? sessionSelect.value : '';
  const showAllCheckbox = document.getElementById('pipelineShowAll');
  const showAll = showAllCheckbox ? showAllCheckbox.checked : false;
  let url = '/api/dashboard/recommendation-pipeline';
  const params = [];
  if (sessionId) params.push('session_id=' + encodeURIComponent(sessionId));
  if (showAll) params.push('show_all=true');
  if (params.length) url += '?' + params.join('&');
  const data = await getJSON(url);
  renderPipeline(data, showAll, sessionId, checkbox.checked);
}
```

Update `renderPipeline(data, showAll, sessionId, showScreened)` signature to accept the fourth argument (default `false`).

**Step 3: Update workflow banner with count**

When data loads, if `screened_items` exists, update the workflow step badge:

```javascript
const screeningBadge = (data.screened_items && data.screened_items.length) ? `（${data.screened_items.length} 檔被篩除）` : '';
// Inject into the #workflowScreening element text if it exists
document.getElementById('workflowScreening').textContent = '篩選層' + screeningBadge;
```

**Step 4: Verify in browser**

Toggle `顯示被篩選層排除的標的`, confirm the muted table appears with correct columns.

**Step 5: Commit**

```bash
git add web/static/index.html
git commit -m "feat(dashboard): pipeline screened-out toggle and table"
```

---

### Task 11: Final verification and CI alignment

**Step 1: Format check**

```bash
test -z "$(gofmt -l .)"
```

Expected: no output.

**Step 2: Build**

```bash
go build ./...
```

Expected: success.

**Step 3: Run all tests**

```bash
go test ./...
```

Expected: PASS across all packages.

**Step 4: Run quality checks**

```bash
go vet ./...
staticcheck ./...
```

Expected: no errors.

**Step 5: Spot-check coverage**

```bash
go test -coverprofile=coverage.out ./internal/screener/ ./internal/orchestrator/ ./internal/ledger/ ./internal/monitoring/
go tool cover -func=coverage.out | grep total
```

Expected: total ≥ 40%.

**Step 6: Commit any final fixes**

```bash
git commit -m "chore: ci alignment and final fixes for screening dashboard sync"
```

---

## Execution Choice

**Plan complete and saved to `docs/plans/2026-04-16-dashboard-screening-sync.md`（歷史路徑；依現行 `docs/documentation-standard.md` 規劃文件應置於 `.omo/plans/`）。**

Two execution options:

1. **Subagent-Driven (this session)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Parallel Session (separate)** — Open a new session with `superpowers:executing-plans`, batch execution with checkpoints.

Which approach would you like?
