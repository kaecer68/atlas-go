// StockpickerWinrateExecutor is the read-only executor for the
// stockpicker-winrate-01 style agent (skill "stockpicker_winrate",
// configs/agents.json). PR 2d registered the agent with enabled:false
// because no builtin executor matched its skill — an enabled agent with no
// matching executor is silently skipped by collectRecommendations and
// leaves an empty Darwinian weight row. This executor makes the agent
// actually produce recommendations.
//
// Read-only contract: the executor NEVER recomputes win rates. It reads
// the persisted stockpicker ledger (stock_win_rate, written by the
// stockpicker backfill / daily update) and the per-symbol T86 flow files
// (data/state/stock_flows/<symbol>.json, written by
// backfill-stockpicker-flows), then applies the documented gates from
// prompts/agents/stockpicker_winrate.md:
//
//  1. calibration gate: only calibration_status == eligible is
//     recommendation-worthy (calibrating/degraded are observation-only);
//  2. win-rate gate: observations >= 30, win_rate >= 0.55,
//     wilson_lower >= 0.45 (defaults, injectable);
//  3. flow gateway: the latest per-symbol foreign net flow must pass
//     stockpicker.FlowGateway.Check (internal/stockpicker/validator.go).
//     Missing flow data fails CLOSED — a candidate without foreign
//     backing is never recommended ("不誤殺也不亂推").
//
// Injection (all fields nil → production defaults, so the zero value
// `StockpickerWinrateExecutor{}` is the wiring-free default):
//
//   - WinRateStore: a WinRateStoreReader; nil opens the read-only ledger
//     at DBPath (default ATLAS_MCP_STOCKPICKER_DB, else
//     data/state/atlas.db under WorkDir) with OpenDB (default
//     stocktools.OpenWinRateDB, mode=ro).
//   - FlowSource: supplies the latest per-symbol foreign net flow
//     (FlowPoint.ForeignNet units, 千股); nil reads the flow file.
//   - Gateway: stockpicker.FlowGateway; nil → NewDefaultFlowGateway()
//     (configs/parameters.json → stockpicker.flow_gateway thresholds).
//   - Source/Window/ConditionID + MinObservations/MinWinRate/MinWilsonLower
//     override the documented defaults.
//
// Any DB/flow error fails silently — (domain.Recommendation{}, false) with
// a debug log — matching the contract "DB/flow 任何錯誤 → (zero, false)
// 靜默失敗（log）".
//
// File: internal/orchestrator/stockpicker_winrate_executor.go
// PR: 2d-executor (StockpickerWinrateExecutor 本體 + re-enable agent)
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/stocktools"
)

// StockpickerWinrateSkill is the domain.Skill value matched by
// StockpickerWinrateExecutor.Supports. Must stay in lockstep with
// configs/agents.json → stockpicker-winrate-01.skill.
const StockpickerWinrateSkill = "stockpicker_winrate"

// Default (source, window, condition) keys for the win-rate read and the
// flow-gateway check. The source is the outcome-source prefix written by
// the stockpicker backfill plus the foreign-3d-net-buy condition id; the
// window matches the MCP tool's default rolling window.
const (
	stockpickerWinrateDefaultSource      = "stockpicker-foreign-3d-net-buy"
	stockpickerWinrateDefaultWindow      = "120d"
	stockpickerWinrateDefaultConditionID = string(stockpicker.ConditionForeign3DNetBuy)
)

// Default win-rate eligibility thresholds — aligned with
// prompts/agents/stockpicker_winrate.md (min_samples = 30; prefer the
// Wilson lower bound over the raw win rate; sample size matters).
const (
	stockpickerWinrateMinObservations = 30
	stockpickerWinrateMinWinRate      = 0.55
	stockpickerWinrateMinWilsonLower  = 0.45
)

// Conviction mapping constants (deterministic, evidence-based).
const (
	stockpickerWinrateConvictionBase  = 55 // passed every gate
	stockpickerWinrateConvictionCap   = 80 // max reachable: 55 base + 10 + 10 + 5 (not a dead ceiling)
	stockpickerWinrateStrongEdge      = 0.60
	stockpickerWinrateStrongLower     = 0.50
	stockpickerWinrateConvictionStep1 = 10
	stockpickerWinrateConvictionStep2 = 5
)

// WinRateStoreReader is the minimal read-only win-rate access the executor
// needs. *stockpicker.WinRateStore satisfies it.
type WinRateStoreReader interface {
	LoadWinRate(ctx context.Context, symbol, source, window string) (stockpicker.StockWinRateSummary, bool, error)
}

// FlowSource supplies the latest per-symbol foreign net flow for the flow
// gateway. The symbol argument is the bare TWSE code (exchange suffix
// stripped); the value is FlowPoint.ForeignNet units (千股).
type FlowSource interface {
	LatestForeignNet(symbol string) (float64, bool)
}

// CapitalFlowReportProvider supplies the market-wide capitalflow DailyReport
// for the flow gateway's institutional/retail market-regime layers (市場層).
// *capitalflow.Service satisfies it. When nil, or when LatestDaily errors,
// the executor falls back to the documented foreign-only scope (nil report →
// the market layers fail-open skip) — issue #1737.
type CapitalFlowReportProvider interface {
	LatestDaily(ctx context.Context) (capitalflow.DailyReport, error)
}

// fileFlowSource is the production FlowSource: it reads
// data/state/stock_flows/<symbol>.json and returns the newest FlowPoint.
// Flow files store points in ascending date order (the backfill sorts by
// date), so the last element is the latest reading.
type fileFlowSource struct {
	flowsDir string
}

// LatestForeignNet implements FlowSource. Missing file, empty flow list,
// or parse failure all report "no data" (the caller fails closed).
func (s fileFlowSource) LatestForeignNet(symbol string) (float64, bool) {
	data, err := os.ReadFile(filepath.Join(s.flowsDir, stockpickerSymbol(symbol)+".json"))
	if err != nil {
		return 0, false
	}
	var f flowFile
	if err := json.Unmarshal(data, &f); err != nil || len(f.Flows) == 0 {
		return 0, false
	}
	return f.Flows[len(f.Flows)-1].ForeignNet, true
}

// flowFile is the on-disk shape of data/state/stock_flows/<symbol>.json
// (JSON tags aligned with stockpicker.FlowPoint / real_panel.go).
type flowFile struct {
	Symbol string                  `json:"symbol"`
	Flows  []stockpicker.FlowPoint `json:"flows"`
}

// StockpickerWinrateExecutor implements AgentExecutor for the
// stockpicker_winrate skill. It is read-only by construction: the ledger
// handle is opened with mode=ro (stocktools.OpenWinRateDB) and flows are
// plain file reads. The zero value is the fully-default production wiring.
type StockpickerWinrateExecutor struct {
	// WinRateStore reads persisted win-rate summaries. nil → the executor
	// opens the read-only ledger (DBPath or the default) via OpenDB and
	// wraps it in stockpicker.WinRateStore.
	WinRateStore WinRateStoreReader
	// OpenDB opens the read-only stockpicker ledger at path. nil →
	// stocktools.OpenWinRateDB (mode=ro).
	OpenDB func(path string) (*sql.DB, error)
	// DBPath overrides the ledger path. Empty → ATLAS_MCP_STOCKPICKER_DB,
	// else data/state/atlas.db under WorkDir.
	DBPath string
	// FlowSource supplies the latest per-symbol foreign net flow. nil →
	// fileFlowSource reading data/state/stock_flows/ under FlowDir/WorkDir.
	FlowSource FlowSource
	// FlowDir overrides the flows directory (default
	// data/state/stock_flows under WorkDir).
	FlowDir string
	// WorkDir is the base directory for relative data paths. Empty →
	// ATLAS_WORK_DIR, else ".".
	WorkDir string
	// Gateway is the capital-flow gate. nil → NewDefaultFlowGateway()
	// (configs/parameters.json → stockpicker.flow_gateway).
	Gateway *stockpicker.FlowGateway
	// CapitalFlow supplies the market-wide DailyReport for the gateway's
	// market-regime layers (institutional/retail). nil → foreign-only scope
	// (the gateway evaluates only the per-symbol foreign layer; market
	// layers fail-open skip). Issue #1737: production wiring injects
	// *capitalflow.Service through System.WithCapitalFlowService.
	CapitalFlow CapitalFlowReportProvider
	// Source/Window/ConditionID override the default win-rate key and the
	// flow-gateway condition id. Empty → defaults.
	//
	// WARNING: Source MUST point at a buy-semantics condition. Pointing it
	// at an avoid-semantics condition (stockpicker.IsAvoidCondition, e.g.
	// price-volume-top-divergence 頂背離) would invert the win_rate >= 0.55
	// gate into "recommend BUY on stocks whose top-divergence signal FAILED"
	// — the executor refuses such sources outright (fail-closed, k3 review
	// 2026-09-07 F3).
	Source      string
	Window      string
	ConditionID string
	// MinObservations/MinWinRate/MinWilsonLower override the eligibility
	// gates. Zero → documented defaults (30 / 0.55 / 0.45).
	MinObservations int
	MinWinRate      float64
	MinWilsonLower  float64
}

// Supports satisfies AgentExecutor: true for the stockpicker_winrate
// skill. Enabled-ness is the collectRecommendations loop's concern — an
// executor matches by skill alone so the registry guard
// (TestRegistryExecutorsCovered) can resolve enabled agents to executors.
func (e StockpickerWinrateExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == StockpickerWinrateSkill
}

// Recommend satisfies AgentExecutor. Read-only flow:
//
//  1. load the persisted (symbol, source, window) win-rate summary;
//  2. calibration gate — eligible only;
//  3. win-rate gate — observations / win_rate / wilson_lower thresholds;
//  4. flow gateway — latest per-symbol foreign net flow plus the
//     market-wide capitalflow DailyReport must pass
//     FlowGateway.CheckFromReport (missing per-symbol flow data → fail
//     closed; missing market report → foreign-only fallback);
//  5. emit a BUY recommendation carrying the win-rate evidence.
//
// Returns (zero, false) on any gate failure or DB/flow error (logged at
// debug level — silent failure, not an error return).
func (e StockpickerWinrateExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !e.Supports(agent) {
		return domain.Recommendation{}, false
	}

	symbol := stockpickerSymbol(quote.Symbol) // ledger + flow files key on the bare TWSE code
	ctx := context.Background()

	// Stage 0: semantic-direction guard (k3 review F3). Avoid-semantics
	// conditions (頂背離: low forward win rate = signal working) must never
	// drive a BUY gate whose thresholds assume buy semantics. Fail closed.
	src := e.source()
	condID := strings.TrimPrefix(src, "stockpicker-")
	if stockpicker.IsAvoidCondition(condID) {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "direction_guard",
			"source", src, "reason", "avoid-semantics condition cannot drive BUY gate")
		return domain.Recommendation{}, false
	}

	// Stage 1: persisted win-rate evidence (read-only).
	store, closer, err := e.winRateStore()
	defer closer()
	if err != nil {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "winrate_db", "err", err)
		return domain.Recommendation{}, false
	}
	summary, found, err := store.LoadWinRate(ctx, symbol, src, e.window())
	if err != nil || !found {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "winrate_load", "found", found, "err", err)
		return domain.Recommendation{}, false
	}

	// Stage 2: calibration + win-rate gates.
	minObs, minWR, minWL := e.thresholds()
	if summary.CalibrationStatus != stockpicker.CalibrationEligible {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "calibration",
			"status", string(summary.CalibrationStatus))
		return domain.Recommendation{}, false
	}
	if summary.Observations < minObs {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "winrate_observations",
			"observations", summary.Observations, "min", minObs)
		return domain.Recommendation{}, false
	}
	if summary.WinRate < minWR {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "winrate",
			"win_rate", summary.WinRate, "min", minWR)
		return domain.Recommendation{}, false
	}
	if summary.WilsonLower < minWL {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "wilson_lower",
			"wilson_lower", summary.WilsonLower, "min", minWL)
		return domain.Recommendation{}, false
	}

	// Stage 3: capital-flow gateway — fail closed when the per-symbol
	// foreign flow is missing (never fabricate backing).
	net, ok := e.flowSource().LatestForeignNet(symbol)
	if !ok {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "flow_missing")
		return domain.Recommendation{}, false
	}
	// Issue #1737: enforce the full two-level gate (個股層 + 市場層) by
	// reading the market-wide capitalflow DailyReport and delegating to
	// FlowGateway.CheckFromReport. The report supplies the
	// institutional/retail ForceScore dimensions that the plain Check call
	// could not see when forces=nil (PR 2d-executor, k3 review A).
	//
	// Fallback (documented, fail-open): when no CapitalFlow provider is
	// injected or LatestDaily fails, latestReport returns nil and
	// CheckFromReport degrades to the original foreign-only scope — the
	// market-regime layers fail-open skip exactly as before. The per-symbol
	// foreign layer still decides the verdict, so this is a deliberate,
	// documented foreign-only fallback, not an oversight.
	verdict := e.gateway().CheckFromReport(quote.Symbol, e.conditionID(), map[string]stockpicker.FlowPoint{
		quote.Symbol: {ForeignNet: net},
	}, e.latestReport(ctx))
	if !verdict.Pass {
		logging.Debug("stockpicker_winrate", "skip",
			logging.Symbol(quote.Symbol), "stage", "flow_gate", "reason", verdict.Reason)
		return domain.Recommendation{}, false
	}

	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Layer:      agent.Layer,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: convictionForWinRate(summary, minObs),
		Reason:     winRateReason(summary, verdict),
	}, true
}

// winRateStore returns the win-rate reader plus a closer for the handle it
// owns. An injected store returns a no-op closer; the default path opens
// the read-only ledger per call and releases it after the read (the
// executor stays stateless and value-type safe — the registry holds
// executor values, so instance caching would not survive interface copies).
func (e StockpickerWinrateExecutor) winRateStore() (WinRateStoreReader, func(), error) {
	if e.WinRateStore != nil {
		return e.WinRateStore, func() {}, nil
	}
	opener := e.OpenDB
	if opener == nil {
		opener = stocktools.OpenWinRateDB
	}
	db, err := opener(e.dbPath())
	if err != nil {
		return nil, func() {}, err
	}
	return stockpicker.NewWinRateStore(db), func() { _ = db.Close() }, nil
}

// flowSource returns the injected FlowSource or the default file reader.
func (e StockpickerWinrateExecutor) flowSource() FlowSource {
	if e.FlowSource != nil {
		return e.FlowSource
	}
	return fileFlowSource{flowsDir: e.flowsDir()}
}

// gateway returns the injected FlowGateway or the config-backed default.
func (e StockpickerWinrateExecutor) gateway() *stockpicker.FlowGateway {
	if e.Gateway != nil {
		return e.Gateway
	}
	return stockpicker.NewDefaultFlowGateway()
}

// latestReport returns the market-wide capitalflow DailyReport for the flow
// gateway's market-regime layers, or nil when the provider is absent or
// LatestDaily fails. A nil return keeps the documented foreign-only fallback
// (issue #1737): the market layers fail-open skip and only the per-symbol
// foreign layer is enforced.
func (e StockpickerWinrateExecutor) latestReport(ctx context.Context) *capitalflow.DailyReport {
	if e.CapitalFlow == nil {
		return nil
	}
	report, err := e.CapitalFlow.LatestDaily(ctx)
	if err != nil {
		logging.Debug("stockpicker_winrate", "market_report_unavailable",
			"err", err, "fallback", "foreign_only")
		return nil
	}
	return &report
}

// thresholds returns the eligibility gates (injected overrides or the
// documented defaults).
func (e StockpickerWinrateExecutor) thresholds() (minObs int, minWR, minWL float64) {
	minObs, minWR, minWL = stockpickerWinrateMinObservations, stockpickerWinrateMinWinRate, stockpickerWinrateMinWilsonLower
	if e.MinObservations > 0 {
		minObs = e.MinObservations
	}
	if e.MinWinRate > 0 {
		minWR = e.MinWinRate
	}
	if e.MinWilsonLower > 0 {
		minWL = e.MinWilsonLower
	}
	return minObs, minWR, minWL
}

func (e StockpickerWinrateExecutor) source() string {
	if e.Source != "" {
		return e.Source
	}
	return stockpickerWinrateDefaultSource
}

func (e StockpickerWinrateExecutor) window() string {
	if e.Window != "" {
		return e.Window
	}
	return stockpickerWinrateDefaultWindow
}

func (e StockpickerWinrateExecutor) conditionID() string {
	if e.ConditionID != "" {
		return e.ConditionID
	}
	return stockpickerWinrateDefaultConditionID
}

func (e StockpickerWinrateExecutor) dbPath() string {
	if e.DBPath != "" {
		return e.DBPath
	}
	if p := os.Getenv("ATLAS_MCP_STOCKPICKER_DB"); p != "" {
		return p
	}
	return filepath.Join(e.workDir(), "data", "state", "atlas.db")
}

func (e StockpickerWinrateExecutor) flowsDir() string {
	if e.FlowDir != "" {
		return e.FlowDir
	}
	return filepath.Join(e.workDir(), "data", "state", "stock_flows")
}

func (e StockpickerWinrateExecutor) workDir() string {
	if e.WorkDir != "" {
		return e.WorkDir
	}
	if wd := os.Getenv("ATLAS_WORK_DIR"); wd != "" {
		return wd
	}
	return "."
}

// stockpickerSymbol strips the exchange-suffix variants callers may pass
// (.TW / .TWO) — the stockpicker ledger and flow files key on bare 4–6
// digit TWSE codes.
func stockpickerSymbol(symbol string) string {
	return strings.TrimSuffix(strings.TrimSuffix(symbol, ".TW"), ".TWO")
}

// convictionForWinRate maps the persisted win-rate evidence to a
// conviction: base 55 for passing every gate, +10 when the raw win rate
// shows a clear edge (>= 0.60), +10 when the Wilson lower bound clears
// 0.50 (sample-supported edge), +5 when the sample comfortably exceeds the
// minimum (>= 2×minObservations). Capped at 80 (the exact max reachable:
// 55 base + 10 strong-edge + 10 strong-lower + 5 large-sample) — the
// recommendation is aggregate-backed, not certainty.
func convictionForWinRate(s stockpicker.StockWinRateSummary, minObs int) int {
	conv := stockpickerWinrateConvictionBase
	if s.WinRate >= stockpickerWinrateStrongEdge {
		conv += stockpickerWinrateConvictionStep1
	}
	if s.WilsonLower >= stockpickerWinrateStrongLower {
		conv += stockpickerWinrateConvictionStep1
	}
	if minObs > 0 && s.Observations >= 2*minObs {
		conv += stockpickerWinrateConvictionStep2
	}
	if conv > stockpickerWinrateConvictionCap {
		conv = stockpickerWinrateConvictionCap
	}
	return conv
}

// winRateReason builds the recommendation reason: the win-rate evidence
// plus the flow-gateway verdict (the per-symbol foreign layer's reason).
func winRateReason(s stockpicker.StockWinRateSummary, verdict stockpicker.FlowVerdict) string {
	flow := flowVerdictReason(verdict)
	if flow == "" {
		flow = "flow_gate=pass"
	}
	return fmt.Sprintf("win_rate=%.3f observations=%d wilson_lower=%.3f calibration=%s; %s",
		s.WinRate, s.Observations, s.WilsonLower, s.CalibrationStatus, flow)
}

// flowVerdictReason extracts the foreign layer's verdict reason (the
// per-symbol layer this executor enforces). Returns "" defensively when no
// foreign layer verdict is present.
func flowVerdictReason(v stockpicker.FlowVerdict) string {
	for _, lv := range v.Layers {
		if lv.Layer == stockpicker.FlowLayerForeign {
			return lv.Reason
		}
	}
	return ""
}
