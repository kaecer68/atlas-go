# Sector Allocation Simulation Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依 `docs/specs/sector-allocation-simulation-closure-spec.md` v1.0 與 `docs/manifests/sector-allocation-simulation-closure-manifest.md` v1.0 完成 SA00–SA12，把散戶定位審計 P1-4 升級為可驗證的模擬閉環；SA11 觀察期僅驗證工程穩定性，**永不**宣稱金融準確或預測命中率。

**Architecture:** 先以 `SA01` 的 closure verifier 鎖住狀態機與依賴，再依 Phase A/B/C 落地 namespace、prior、legacy split、canonical WeightEngine、capital-flow anti-corruption、composition-root wiring；Phase C 真實 current exposure、next-session policy、allocator consumption、cross-interface parity、F06 shadow ranking；Phase D 觀察與 close-out。每一個 ID 採 RED → impl → focused gate → implementation commit → evidence commit 五段式，狀態最多到 `implemented`，`observing`/`done` 必須在 runtime 證據完成後才能升級。

**Tech Stack:** Go 1.26、sectorallocation WeightEngine、portfolio SectorBudgetAllocator、orchestrator composition root、capitalflow CapitalFlowAssessment、ParametersConfig、JSONL state store、shared_web JS、atlas-mcp passthrough、shell verifier、GitNexus。

## Global Constraints

- Canonical spec：`docs/specs/sector-allocation-simulation-closure-spec.md` v1.0。
- 專屬 manifest：`docs/manifests/sector-allocation-simulation-closure-manifest.md` v1.0（SA00–SA12 + SA-INV-01–20 + Completion Contract）。
- 20 個 binding invariants 必須同時成立；任一違規 → 該 ID 不得升 `done`。
- `source=heuristic` 與 `calibration_status=calibrating` 為唯一可被 SA11 promotion 翻動的值；**不得**升 `empirical`。
- 6 個 `cmd/atlas/main.go` constructor callsite 必須全列入 wiring matrix；只有 admin/auto_daily/stress_daily/cli_sim 四條 simulation path 可進入 sector application gate；auto_experiment 與 live 必須 negative。
- CLI simulation 現行會把 positions 同步到 live store（`cmd/atlas/main.go:1889-1917`）；SA08 必須先把此 sync 移除或置於獨立 default-off opt-in gate，否則不得進入 observing。
- `currentSectorAllocations()` 不再允許回 nil；SA07 用 simulation closing positions × T 日價格 + symbol→L1 mapping 計算，unknown weight > 0 必須 `current_exposure_incomplete`。
- `ApplySectorRotation` 必須二選一：持久化可被下一 session 消費的 policy + 附 mutation receipt，或完全不修改並回 `Applied=false` + fallback reason。
- T 日 snapshot 的 `effective_from` 必須晚於 as-of；不得改寫同 session 已完成的 orders/outcomes；下一有效 session 必須在產單前真實消費 policy。
- 觀察期所有 `sac.*` metric 與 `state.sessions_completed` 只能反映工程穩定性，**禁止**用任何 metric 宣稱投資績效、Sharpe、命中率或預測準確度。
- live broker、實盤下單、broker adapter 不得因本閉環新增或修改。
- `heuristic` prior 在 observation 期間不得用於自動 simulation allocation（spec §4.1）；`empirical` 升級不在本 plan scope。
- 一個 ID 一個 commit，跨檔案邏輯性修改可同 commit；不得把兩個 ID 合併。
- push、PR、merge 各自需要使用者當次明確授權。
- `.omo/plans/**` 屬工作區內短期計畫，merge 後必須刪除；未來 Agent 只依 canonical spec 與專屬 manifest。

---

## File Structure

| 區塊 | 責任 | 備註 |
|------|------|------|
| `scripts/verify-sector-allocation-closure.sh` | 唯一 closure verifier | 整輪 plan 每次 commit 必跑 |
| `scripts/ci/sa12-negative-evidence.sh` | sunset 後 negative search | SA12.A |
| `cmd/experimental/sector-allocation-closure-preflight/main.go` | 5 auto + 3 manual checks | clone c07-preflight |
| `internal/sectorallocation/` | typed namespace、typed prior、closure、legacy compat、projector、policy、metrics emitter、legacy counter | WeightEngine 延伸，不重複 engine 公式 |
| `internal/portfolio/` | sector rotator 改為薄殼、`SectorBudgetAllocator` | 移除第二套 normalize |
| `internal/orchestrator/composition/` | shared WeightEngine、capital flow mapper、path-aware factory | 取代六 callsite 各自建 engine |
| `internal/orchestrator/strategy_evolver.go` | 真實 receipt + Applied 控管 | 不再恆回 true |
| `internal/orchestrator/sector_allocation_closure_metrics.go` | 11 個 `sac.*` events | slog 結構化 |
| `internal/capitalflow/action_mapper.go` | typed enum + mapper | anti-corruption layer |
| `internal/strategy/` | FileComparisonStore、ShadowStrategyEvaluator、WarmingUpState | 不動 strategy_ranker |
| `internal/marketdata/taiex_benchmark.go` | FileTAIEXBenchmarkProvider | 觀察期先 `unavailable` |
| `internal/monitoring/api/pipeline/sector_allocation_closure_state.go` | state manager | clone L2.4 pattern |
| `internal/monitoring/api/industry/handlers.go` | 改讀 persisted snapshot | 不重算 |
| `cmd/atlas/main.go` | 六 callsite 改走 composition root | 移除 simulation→live sync |
| `cmd/atlas/wire_recommender.go` | 注入 shared ComparisonEngine | 禁止 NewComparisonEngine |
| `shared_web/static/js/` | sector allocation snapshot view | deterministic view-model |
| `cmd/atlas-mcp/server/tools_recommendation.go` | passthrough + evaluation_mode 透出 | 仍 passthrough |
| `docs/operations/sector-allocation-closure-*.md` | 觀察期文件 | clone L2.4 |

每個檔案只負責一塊；SA08 同時擴大修改面時，spec/manifest 必先補設計點，實作禁止跨 ID 偷塞。

---

## 必跑全域 Gate（每次 commit 前 + promotion 前）

```bash
go test ./... -count=1
go build ./...
gofmt -l .
golangci-lint run --timeout=5m
go generate .
git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

觀察期 promotion 前額外必跑：

```bash
bash scripts/ci/sa12-negative-evidence.sh  # 觀察期開始時先驗證 baseline
curl -s http://localhost:18080/metrics | grep sector_allocation_legacy_compat_reads_total
```

---

## Task 1: SA01 — typed namespaces + closure verifier（基線鎖定）

**Files:**
- Create: `internal/sectorallocation/namespaces.go`、`internal/sectorallocation/namespaces_test.go`、`internal/sectorallocation/closure.go`、`internal/sectorallocation/closure_test.go`、`scripts/verify-sector-allocation-closure.sh`、`scripts/ci/check_sa01_invariants.sh`
- Modify: `internal/sectorallocation/doc.go`（新增 §4 Namespaces 註解，引用 spec §3.2）
- 不動：`WeightEngine`、`configs/parameters.json`、portfolio、orchestrator

**Interfaces:**

```go
type NamespaceKind string // equity_sector_l1 | research_theme_l2 | strategy_bucket | asset_class
type L1FinalTarget struct {
    Weights map[industry.SectorID]float64
    CalibrationStatus, ModelVersion string
}
type ThemeExposure struct {
    Theme string
    CanonicalSubsector *industry.SectorID
    ToL1 map[industry.SectorID]float64 // sum=1±1e-9
    Source, Version string
}
func ValidateL1FinalTarget(L1FinalTarget) error
func ValidateThemeExposure(ThemeExposure) error

type ClosureState struct {
    ManifestPath string
    InvariantsEvaluated map[string]ClosureStatus
    NamespaceTypesExist, TypedPriorPresent, LegacyCompatActive bool
    FinalL1TargetSum float64
    NoncanonicalKeyCnt int
    MissingEvidence []MissingEvidence
}
func VerifyClosure(ClosureState) []ClosureRuleResult
```

closure verifier 第一輪只放 5 個 base check：manifest_status_machine、id_done_requires_three_evidence、phase_dependency_complete、cross_id_dangling_dependency、source_label_lock。

**Step 1: Write the failing test**

`internal/sectorallocation/namespaces_test.go`：
```go
package sectorallocation_test

import (
    "testing"
    "github.com/kaecer68/atlas-go/internal/industry"
    "github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestNamespaceKind_OnlyFourCanonical(t *testing.T) {
    want := []sectorallocation.NamespaceKind{
        sectorallocation.NamespaceEquityL1,
        sectorallocation.NamespaceResearchThemeL2,
        sectorallocation.NamespaceStrategyBucket,
        sectorallocation.NamespaceAssetClass,
    }
    if len(want) != 4 {
        t.Fatalf("must remain 4 namespaces, got %d", len(want))
    }
    for _, n := range []sectorallocation.NamespaceKind{"equity_l1", "L2", "themes"} {
        if sectorallocation.IsValidNamespace(n) {
            t.Errorf("non canonical namespace accepted: %q", n)
        }
    }
}

func TestL1FinalTarget_RejectsNonCanonicalKeys(t *testing.T) {
    bad := sectorallocation.L1FinalTarget{
        Weights: map[industry.SectorID]float64{
            industry.SectorSemiconductor: 0.5,
            industry.SubIndustryIndustrial: 0.5,
        },
    }
    if err := sectorallocation.ValidateL1FinalTarget(bad); err == nil {
        t.Fatal("must reject L2 keys in L1 final target")
    }
}

func TestL1FinalTarget_RejectsLessThan20Keys(t *testing.T) {
    m := map[industry.SectorID]float64{industry.SectorSemiconductor: 1.0}
    if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err == nil {
        t.Fatal("must reject fewer than 20 L1 keys")
    }
}

func TestL1FinalTarget_FullyCanonicalSucceeds(t *testing.T) {
    m := make(map[industry.SectorID]float64, 20)
    s := 0.0
    for _, id := range industry.L1Sectors() {
        m[id] = 0.05
        s += 0.05
    }
    if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err != nil {
        t.Fatalf("fully canonical target should validate: %v", err)
    }
    if got := s; got < 0.9999 || got > 1.0001 {
        t.Fatalf("sum drifted: %.9f", got)
    }
}

func TestThemeExposure_RowMustSumToOne(t *testing.T) {
    if err := sectorallocation.ValidateThemeExposure(sectorallocation.ThemeExposure{
        Theme: "ai_supply_chain",
        ToL1: map[industry.SectorID]float64{industry.SectorSemiconductor: 0.7},
    }); err == nil {
        t.Fatal("must reject theme row not summing to 1")
    }
}

func TestThemeExposure_NoFuzzyIndustrialToIndustrials(t *testing.T) {
    if err := sectorallocation.ValidateThemeExposure(sectorallocation.ThemeExposure{
        Theme: "industrials_alias", ToL1: map[industry.SectorID]float64{industry.SubIndustryIndustrial: 1.0},
    }); err == nil {
        t.Fatal("must reject fuzzy mapping between L1 and L2 forms")
    }
}
```

`internal/sectorallocation/closure_test.go`：
```go
package sectorallocation_test

import (
    "os"
    "path/filepath"
    "testing"
    "github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestVerifyClosure_RejectsInProgressToDone(t *testing.T) {
    st := sectorallocation.ClosureState{ManifestPath: writeStubManifest(t, "in_progress", "todo")}
    if results := sectorallocation.VerifyClosure(st); !hasFailure(results, "manifest_status_machine") {
        t.Fatal("in_progress→done transition must be rejected")
    }
}

func TestVerifyClosure_RequiresThreeEvidenceForDone(t *testing.T) {
    st := sectorallocation.ClosureState{ManifestPath: writeStubManifest(t, "done", "")}
    if results := sectorallocation.VerifyClosure(st); !hasFailure(results, "id_done_requires_three_evidence") {
        t.Fatal("done ID without 3 evidence categories must fail")
    }
}

func TestVerifyClosure_RejectsEmpiricalSource(t *testing.T) {
    st := sectorallocation.ClosureState{ManifestPath: writeStubManifestWithSource("empirical")}
    if results := sectorallocation.VerifyClosure(st); !hasFailure(results, "source_label_lock") {
        t.Fatal("source=empirical must be rejected by closure verifier")
    }
}
```

helper：把測資 manifest 寫到 `t.TempDir()`，對應每個 check 構造不同 state。

**Step 2: Run RED**

```bash
go test ./internal/sectorallocation/ -run 'TestNamespaceKind|TestL1FinalTarget|TestThemeExposure|TestVerifyClosure' -count=1
```

預期：compile failure（型別尚未定義）。

**Step 3: 最小實作**

`internal/sectorallocation/namespaces.go`：
```go
package sectorallocation

import (
    "fmt"
    "github.com/kaecer68/atlas-go/internal/industry"
)

const (
    NamespaceEquityL1        NamespaceKind = "equity_sector_l1"
    NamespaceResearchThemeL2 NamespaceKind = "research_theme_l2"
    NamespaceStrategyBucket  NamespaceKind = "strategy_bucket"
    NamespaceAssetClass      NamespaceKind = "asset_class"
)

func IsValidNamespace(k NamespaceKind) bool {
    switch k {
    case NamespaceEquityL1, NamespaceResearchThemeL2, NamespaceStrategyBucket, NamespaceAssetClass:
        return true
    }
    return false
}

func ValidateL1FinalTarget(t L1FinalTarget) error {
    if len(t.Weights) != 20 {
        return fmt.Errorf("L1 final target must have 20 keys, got %d", len(t.Weights))
    }
    for id := range t.Weights {
        if !industry.IsL1(id) {
            return fmt.Errorf("non canonical L1 key: %s", id)
        }
    }
    s := 0.0
    for _, v := range t.Weights {
        if v < 0 {
            return fmt.Errorf("negative L1 weight: %f", v)
        }
        s += v
    }
    if s < 0.999999999 || s > 1.000000001 {
        return fmt.Errorf("L1 sum drift: %.12f", s)
    }
    return nil
}

func ValidateThemeExposure(t ThemeExposure) error {
    if len(t.ToL1) == 0 {
        return fmt.Errorf("theme %q must declare at least one L1 target", t.Theme)
    }
    s := 0.0
    for id, w := range t.ToL1 {
        if !industry.IsL1(id) {
            return fmt.Errorf("theme %q maps to non L1 key %s", t.Theme, id)
        }
        if w < 0 {
            return fmt.Errorf("theme %q has negative weight: %f", t.Theme, w)
        }
        s += w
    }
    if s < 0.999999999 || s > 1.000000001 {
        return fmt.Errorf("theme %q row sum drift: %.12f", t.Theme, s)
    }
    return nil
}
```

`internal/sectorallocation/closure.go`：
```go
package sectorallocation

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strings"
)

type ClosureStatus string
const (
    StatusPending ClosureStatus = "pending"
    StatusInProgress ClosureStatus = "in_progress"
    StatusImplemented ClosureStatus = "implemented"
    StatusObserving ClosureStatus = "observing"
    StatusDone ClosureStatus = "done"
    StatusBlocked ClosureStatus = "blocked"
)

type ClosureRuleResult struct {
    Rule string
    Passed bool
    Evidence string
}

func VerifyClosure(state ClosureState) []ClosureRuleResult {
    rows := readManifestRows(state.ManifestPath)
    out := []ClosureRuleResult{}
    out = append(out, checkStatusMachine(rows))
    out = append(out, checkThreeEvidence(rows))
    out = append(out, checkPhaseDependency(rows))
    out = append(out, checkCrossIDDependency(rows))
    out = append(out, checkSourceLabelLock(rows))
    return out
}
```

5 個 check 函式每個都從 `readManifestRows()` 拿 rows，掃描違規後回 `ClosureRuleResult`。`readManifestRows` 用 `bufio.Scanner` + 行 regex `^\|(SA\d{2})\s*\|\s*([^|]*)\|.*\|\s*(pending|in_progress|implemented|observing|done|blocked)\s*\|\s*([^|]*)\|$` 抓 ID、root cause、status、notes。

`scripts/verify-sector-allocation-closure.sh`：
```bash
#!/usr/bin/env bash
set -euo pipefail
MANIFEST="${1:-docs/manifests/sector-allocation-simulation-closure-manifest.md}"
if [[ ! -f "$MANIFEST" ]]; then echo "missing $MANIFEST" >&2; exit 2; fi
echo "OK: SA01 closure verifier scaffold validates $MANIFEST"
echo "      (full rule set lands in SA12.C)"
exit 0
```

**Step 4: Run GREEN**

```bash
go test ./internal/sectorallocation/ -run 'TestNamespaceKind|TestL1FinalTarget|TestThemeExposure|TestVerifyClosure' -count=1
gofmt -l internal/sectorallocation/namespaces.go internal/sectorallocation/closure.go
go vet ./internal/sectorallocation/...
shellcheck scripts/verify-sector-allocation-closure.sh
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

**Step 5: Commit**

```bash
git add internal/sectorallocation/namespaces.go internal/sectorallocation/namespaces_test.go \
  internal/sectorallocation/closure.go internal/sectorallocation/closure_test.go \
  scripts/verify-sector-allocation-closure.sh scripts/ci/check_sa01_invariants.sh \
  internal/sectorallocation/doc.go
git commit -m "feat(manifest): #SA01 typed namespaces + closure verifier scaffold"
```

**Step 6: Manifest evidence**

```bash
# 將 SA01 row 從 pending 改 implemented + 加 Notes
# implementation: <commit hash>
# observation: pending until SA11 enters observing
# negative: verifier check_source_label_lock verified
git add docs/manifests/sector-allocation-simulation-closure-manifest.md
git commit -m "docs(manifest): SA01 implemented evidence"
```

---

## Task 2: SA02 — 20-L1 strategic prior + provenance/calibration

**Files:**
- Create: `internal/sectorallocation/strategic_prior.go`、`internal/sectorallocation/strategic_prior_test.go`、`internal/eventdriven/sector_predictor_prior_test.go`
- Modify: `internal/config/parameters.go`（新增 `EngineStrategicPriorParameters`）、`internal/config/defaults_engine.go`（新增 `defaultStrategicPriorParameters()`）、`parameters_merge.go`、`parameters_validate.go`、`parameters_test.go`、`testdata/parameters_api.golden.json`、`internal/eventdriven/sector_predictor.go`（**刪除** `_sectorWeights`，加 `SetStrategicPrior`）、`configs/parameters.json`

**Interfaces:**

```go
type StrategicSectorPrior struct {
    Weights           map[industry.SectorID]float64
    Source            string // 鎖死 "heuristic"；empirical 升級不在本 plan
    ModelVersion      string // semver
    CalibrationStatus string // 鎖死 "calibrating"
    AsOfDate          string
}
func (p *StrategicSectorPrior) PromotionGate() bool
func LoadStrategicPrior(*config.ParametersConfig) (*StrategicSectorPrior, error)
func ValidatePrior(*StrategicSectorPrior) error

type EngineStrategicPriorParameters struct {
    Weights, Source, ModelVersion, CalibrationStatus, AsOfDate ParameterMetadata[...]
}
```

第一版採用 `internal/eventdriven/sector_predictor.go:171-192` 的 `_sectorWeights` 數值（C07 heuristic seed，sum=1）。`Source="heuristic"`、`CalibrationStatus="calibrating"` 寫死在 default；任何 CI 嘗試改 `empirical` 必須被 `verify-sector-allocation-closure.sh` check_source_label_lock 拒絕。

**Step 1: Write RED**

`internal/sectorallocation/strategic_prior_test.go`：
```go
package sectorallocation_test

import (
    "testing"
    "github.com/kaecer68/atlas-go/internal/config"
    "github.com/kaecer68/atlas-go/internal/industry"
    "github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestLoadStrategicPrior_DefaultSeedIsHeuristicCalibrating(t *testing.T) {
    cfg := config.GetParametersConfig()
    p, err := sectorallocation.LoadStrategicPrior(cfg)
    if err != nil { t.Fatal(err) }
    if p.Source != "heuristic" || p.CalibrationStatus != "calibrating" {
        t.Fatalf("prior must be heuristic+calibrating, got source=%q status=%q", p.Source, p.CalibrationStatus)
    }
    if p.PromotionGate() {
        t.Fatal("heuristic calibrating must not promote")
    }
}

func TestValidatePrior_RequiresExactly20L1(t *testing.T) {
    p := &sectorallocation.StrategicSectorPrior{Weights: map[industry.SectorID]float64{industry.SectorSemiconductor: 1.0}}
    if err := sectorallocation.ValidatePrior(p); err == nil { t.Fatal("must reject fewer than 20 L1") }
}

func TestValidatePrior_RejectsNonL1Key(t *testing.T) {
    p := &sectorallocation.StrategicSectorPrior{Weights: map[industry.SectorID]float64{industry.SubIndustryIndustrial: 1.0}}
    if err := sectorallocation.ValidatePrior(p); err == nil { t.Fatal("must reject L2 key in prior") }
}

func TestValidatePrior_RejectsNonSemverVersion(t *testing.T) {
    m := make20L1SumOne()
    p := &sectorallocation.StrategicSectorPrior{Weights: m, ModelVersion: "c07-heuristic", Source: "heuristic"}
    if err := sectorallocation.ValidatePrior(p); err == nil { t.Fatal("must reject non semver") }
}

func TestValidatePrior_PromotionRequiresEmpirical(t *testing.T) {
    m := make20L1SumOne()
    p := &sectorallocation.StrategicSectorPrior{Weights: m, ModelVersion: "v1.0.0", Source: "heuristic", CalibrationStatus: "calibrated"}
    if p.PromotionGate() { t.Fatal("promotion must require empirical source") }
}

func TestDefaultPriorFromC07_SumsToOne(t *testing.T) {
    cfg := config.GetParametersConfig()
    p, _ := sectorallocation.LoadStrategicPrior(cfg)
    s := 0.0
    for _, v := range p.Weights { s += v }
    if s < 0.999999999 || s > 1.000000001 { t.Fatalf("sum drift: %.9f", s) }
}
```

`internal/eventdriven/sector_predictor_prior_test.go`：
```go
func TestSectorPredictor_ReadsPriorFromTypedLoader(t *testing.T) {
    p, _ := sectorallocation.LoadStrategicPrior(config.GetParametersConfig())
    sp := eventdriven.NewSectorPredictor(nil, nil)
    sp.SetStrategicPrior(p)
    w, _ := sp.PriorWeight(industry.SectorSemiconductor)
    if w <= 0 { t.Fatal("prior must feed sector weight") }
}

func TestSectorPredictor_PriorNilReturnsZeroNotFallback(t *testing.T) {
    sp := eventdriven.NewSectorPredictor(nil, nil)
    if w, _ := sp.PriorWeight(industry.SectorSemiconductor); w != 0 {
        t.Fatalf("nil prior must return 0, got %f", w)
    }
}
```

**Step 2: Run RED**

```bash
go test ./internal/sectorallocation/ -run TestLoadStrategicPrior -count=1
go test ./internal/eventdriven/ -run 'TestSectorPredictor_ReadsPriorFromTypedLoader|TestSectorPredictor_PriorNilReturnsZeroNotFallback' -count=1
```

預期：build failure。

**Step 3: 最小實作**

`internal/sectorallocation/strategic_prior.go`：
```go
package sectorallocation

import (
    "fmt"
    "regexp"
    "github.com/kaecer68/atlas-go/internal/config"
    "github.com/kaecer68/atlas-go/internal/industry"
)

type StrategicSectorPrior struct {
    Weights           map[industry.SectorID]float64
    Source            string
    ModelVersion      string
    CalibrationStatus string
    AsOfDate          string
}

func (p *StrategicSectorPrior) PromotionGate() bool {
    return p.Source == "calibrated" && p.CalibrationStatus == "calibrated"
}

var semverRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

func ValidatePrior(p *StrategicSectorPrior) error {
    if len(p.Weights) != 20 {
        return fmt.Errorf("prior must have 20 L1 keys, got %d", len(p.Weights))
    }
    for id := range p.Weights {
        if !industry.IsL1(id) {
            return fmt.Errorf("prior must not contain non L1 key %s", id)
        }
    }
    s := 0.0
    for _, v := range p.Weights {
        if v < 0 { return fmt.Errorf("prior has negative weight: %f", v) }
        s += v
    }
    if s < 0.999999999 || s > 1.000000001 {
        return fmt.Errorf("prior sum drift: %.12f", s)
    }
    if !semverRe.MatchString(p.ModelVersion) {
        return fmt.Errorf("prior ModelVersion must be semver: %s", p.ModelVersion)
    }
    return nil
}

func LoadStrategicPrior(cfg *config.ParametersConfig) (*StrategicSectorPrior, error) {
    p := cfg.Engine.SectorRotation.StrategicPrior
    weights := make(map[industry.SectorID]float64, 20)
    for k, v := range p.Weights.Value {
        id := industry.SectorID(k)
        weights[id] = v
    }
    prior := &StrategicSectorPrior{
        Weights:           weights,
        Source:            p.Source.Value,
        ModelVersion:      p.ModelVersion.Value,
        CalibrationStatus: p.CalibrationStatus.Value,
        AsOfDate:          p.AsOfDate.Value,
    }
    if err := ValidatePrior(prior); err != nil {
        return nil, err
    }
    return prior, nil
}
```

`internal/eventdriven/sector_predictor.go`：
```go
type SectorPredictor struct {
    macro *marketdata.MacroDataSnapshot
    cycle cycleScoreProvider
    prior *sectorallocation.StrategicSectorPrior
}

func (sp *SectorPredictor) SetStrategicPrior(p *sectorallocation.StrategicSectorPrior) { sp.prior = p }
func (sp *SectorPredictor) PriorWeight(id industry.SectorID) (float64, error) {
    if sp.prior == nil { return 0, nil }
    v, ok := sp.prior.Weights[id]
    if !ok { return 0, nil }
    return v, nil
}
```

刪除 line 171-192 `_sectorWeights` 與 `sectorWeight(sid)`（改成讀 `sp.prior`）。

`internal/config/parameters.go`：
```go
type EngineSectorRotationParameters struct {
    StrategicPrior          EngineStrategicPriorParameters `json:"strategic_prior"`
    MinAllocation, MaxAllocation, RebalanceThreshold ParameterMetadata[float64]
    // ...其他欄位仍由 SA03 補上
}
```

`internal/config/defaults_engine.go`：新增 `defaultStrategicPriorParameters()`，採用 C07 heuristic seed：

```go
func defaultStrategicPriorParameters() EngineStrategicPriorParameters {
    weights := map[string]float64{
        "semiconductor": 0.33, "electronics": 0.16, "financials": 0.13, "shipping": 0.08,
    }
    for _, id := range industryCanonicalL1Defaults() { weights[id] = 0.01875 }
    return EngineStrategicPriorParameters{
        Weights:           ParameterMetadata[map[string]float64]{Value: weights, Rationale: "C07 heuristic seed; SA11 promotion only flips flag, not source", Source: SourceEmpirical},
        Source:            ParameterMetadata[string]{Value: "heuristic", Source: SourceEmpirical, Rationale: "Permanent lock: empirical upgrade out of plan scope"},
        ModelVersion:      ParameterMetadata[string]{Value: "v0.0.0.32-c07-heuristic", Source: SourceEmpirical},
        CalibrationStatus: ParameterMetadata[string]{Value: "calibrating", Source: SourceEmpirical, Rationale: "Permanent lock during observation window"},
        AsOfDate:          ParameterMetadata[string]{Value: "2026-07-17", Source: SourceEmpirical},
    }
}
```

`internal/config/parameters_merge.go`：補上 strategic prior merge 條目；`parameters_validate.go`：在 `validateEngine` 內加 `if cfg.Engine.SectorRotation.StrategicPrior.Source.Value != "heuristic" { return error }`（對應 check_source_label_lock 機械化）。`parameters_test.go`：新增 case `TestEngineParameters_Defaults_StrategicPriorIsHeuristicCalibrating`。`testdata/parameters_api.golden.json`：同步新增 strategic_prior 區段。

`configs/parameters.json`（line 7080+）：
```json
"strategic_prior": {
  "weights": {
    "semiconductor": 0.33, "electronics": 0.16, "financials": 0.13, "shipping": 0.08,
    "opt oelectronics": 0.01875, "cement": 0.01875, ...
  },
  "source": {"value": "heuristic", "rationale": "...", "source": "empirical", "todo": "..."},
  "model_version": {"value": "v0.0.0.32-c07-heuristic", "source": "empirical"},
  "calibration_status": {"value": "calibrating", "source": "empirical"},
  "as_of_date": {"value": "2026-07-17", "source": "empirical"}
}
```

**Step 4: Run GREEN**

```bash
go test ./internal/sectorallocation/ -run 'TestLoadStrategicPrior|TestValidatePrior|TestDefaultPriorFromC07' -count=1
go test ./internal/eventdriven/ -run 'TestSectorPredictor_ReadsPriorFromTypedLoader|TestSectorPredictor_PriorNilReturnsZeroNotFallback' -count=1
go test ./internal/config/ -run 'TestEngineStrategicPrior|TestEngineParameters_Validate|TestEngineParameters_Defaults_StrategicPriorIsHeuristicCalibrating' -count=1

# SA-INV-05 negative static search
grep -rn "_sectorWeights" internal/eventdriven/   # expected: 0
grep -rn '"empirical"' internal/config/defaults_engine.go | grep -i "source\|calibration"  # expected: 0 in default values (only in marker)
gofmt -l internal/sectorallocation/strategic_prior.go internal/config/parameters.go internal/config/defaults_engine.go internal/config/parameters_merge.go internal/config/parameters_validate.go internal/eventdriven/sector_predictor.go
go vet ./internal/sectorallocation/... ./internal/eventdriven/... ./internal/config/...
```

**Step 5: Commit**

```bash
git add internal/config/parameters.go internal/config/defaults_engine.go \
  internal/config/parameters_merge.go internal/config/parameters_validate.go \
  internal/config/parameters_test.go internal/config/testdata/parameters_api.golden.json \
  internal/sectorallocation/strategic_prior.go internal/sectorallocation/strategic_prior_test.go \
  internal/eventdriven/sector_predictor.go internal/eventdriven/sector_predictor_prior_test.go \
  configs/parameters.json
git commit -m "feat(manifest): #SA02 20-L1 strategic prior with heuristic lock"
```

**Step 6: Manifest evidence**

manifest SA02 row `pending → implemented`；Notes：
- implementation: `<commit>`
- observation: `pending until SA11 promotion`
- negative: `grep _sectorWeights internal/ → 0; source never empirical`

---

## Task 3: SA03 — Legacy BaseAllocations split + compat metric

**Files:**
- Create: `internal/sectorallocation/legacy_compat.go`、`internal/sectorallocation/legacy_compat_test.go`、`internal/monitoring/metrics/legacy_compat.go`
- Modify: `internal/config/parameters.go`（`EngineSectorRotationParameters` 拆 4 sibling 欄位）、`internal/config/defaults_engine.go`、`parameters_merge.go`、`parameters_validate.go`、`parameters_test.go`、`testdata/parameters_api.golden.json`、`internal/portfolio/sector_rotator.go`（新增 `NewSectorRotatorWithL1Allocations`）、`configs/parameters.json`

**Interfaces:**

```go
type LegacyCompatReader struct { cfg *config.ParametersConfig; logger *slog.Logger; counter *prometheus.CounterVec }
func NewLegacyCompatReader(*config.ParametersConfig, *slog.Logger, *prometheus.CounterVec) *LegacyCompatReader
func (r *LegacyCompatReader) Read() map[string]float64            // fires counter + slog per key
func (r *LegacyCompatReader) L1KeysOnly() map[string]float64      // filters to industry.L1Sectors() only
func (r *LegacyCompatReader) PromotionGate() bool                 // 預設 false; SA11 promotion 觸發 sunset

type EngineSectorRotationParameters struct {
    EquityL1Allocations       ParameterMetadata[map[string]float64] `json:"equity_l1_allocations"`
    AssetOverlayAllocations   ParameterMetadata[map[string]float64] `json:"asset_overlay_allocations"`
    StrategySleeveAllocations ParameterMetadata[map[string]float64] `json:"strategy_sleeve_allocations"`
    LegacyCompatAllocations   ParameterMetadata[map[string]float64] `json:"legacy_compat_allocations"`
    MinAllocation, MaxAllocation, RebalanceThreshold ParameterMetadata[float64]
    StrategicPrior            EngineStrategicPriorParameters
}

func NewSectorRotatorWithL1Allocations(cfg config.EngineSectorRotationParameters, compat *LegacyCompatReader) *SectorRotator
```

`LegacyCompatAllocations` 預設值鏡像 `defaults_engine.go:866-870` verbatim（保留 `ai_supply_chain/robotics/consumer/industrial/leo_satellite/defensive/cash` 等 12 keys），僅供 SA11 observation 期 compat 讀取，sunset 後刪除。Prometheus counter：`sector_allocation_legacy_compat_reads_total{key}`，sunset 後必須 0。

**Step 1: Write RED**

`internal/sectorallocation/legacy_compat_test.go`：
```go
func TestLegacyCompatReader_PopulatesRawMap(t *testing.T) { ... }
func TestLegacyCompatReader_L1KeysOnly(t *testing.T) { ... }
func TestLegacyCompatReader_LogsStructuredField(t *testing.T) { ... }
func TestLegacyCompatReader_PromotionGateFalse(t *testing.T) { ... }
```

`internal/config/parameters_test.go` 新增：
- `TestEngineSectorRotation_SplitRejectsCashInL1`
- `TestEngineSectorRotation_AssetOverlayHoldsCash`
- `TestEngineSectorRotation_StrategySleeveHoldsDefensive`
- `TestEngineParameters_Validate_LegacyCompatSumTolerance`

`internal/portfolio/sector_rotator_test.go` 新增：
- `TestSectorRotator_NewConstructorReadsEquityL1Only`
- `TestSectorRotator_LegacyCompatReaderFiresMetric`

**Step 2: Run RED**

```bash
go test ./internal/sectorallocation/ -run TestLegacyCompatReader -count=1
go test ./internal/config/ -run 'TestEngineSectorRotation_SplitRejectsCashInL1|TestEngineSectorRotation_AssetOverlayHoldsCash|TestEngineSectorRotation_StrategySleeveHoldsDefensive|TestEngineParameters_Validate_LegacyCompatSumTolerance' -count=1
go test ./internal/portfolio/ -run 'TestSectorRotator_NewConstructorReadsEquityL1Only|TestSectorRotator_LegacyCompatReaderFiresMetric' -count=1
```

**Step 3: 最小實作**

`internal/sectorallocation/legacy_compat.go`：
```go
package sectorallocation

import (
    "log/slog"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/kaecer68/atlas-go/internal/industry"
)

type LegacyCompatReader struct {
    cfg     *config.ParametersConfig
    logger  *slog.Logger
    counter *prometheus.CounterVec
    sunset  bool
}

func (r *LegacyCompatReader) Read() map[string]float64 {
    out := make(map[string]float64, 12)
    for k, v := range r.cfg.Engine.SectorRotation.LegacyCompatAllocations.Value {
        out[k] = v
        r.counter.WithLabelValues(k).Inc()
        r.logger.Info("sector_allocation.legacy_compat_read", slog.String("key", k), slog.Float64("value", v))
    }
    return out
}

func (r *LegacyCompatReader) L1KeysOnly() map[string]float64 {
    raw := r.Read()
    out := make(map[string]float64, 20)
    for k, v := range raw {
        if id, ok := industry.SectorIDFromString(k); ok && industry.IsL1(id) {
            out[k] = v
        }
    }
    return out
}

func (r *LegacyCompatReader) PromotionGate() bool { return r.sunset }
```

`internal/portfolio/sector_rotator.go` 改：
```go
type SectorRotator struct {
    equityL1 map[string]float64
    legacyCompat *sectorallocation.LegacyCompatReader
    minAllocation, maxAllocation, rebalanceThreshold float64
}

func NewSectorRotatorWithL1Allocations(cfg config.EngineSectorRotationParameters, compat *sectorallocation.LegacyCompatReader) *SectorRotator {
    eq := make(map[string]float64, 20)
    for k, v := range cfg.EquityL1Allocations.Value { eq[k] = v }
    if compat != nil { /* merge L1 keys from legacy compat only if missing in eq */ }
    return &SectorRotator{
        equityL1: eq,
        legacyCompat: compat,
        minAllocation: cfg.MinAllocation.Value,
        maxAllocation: cfg.MaxAllocation.Value,
        rebalanceThreshold: cfg.RebalanceThreshold.Value,
    }
}
```

`internal/config/defaults_engine.go`：補 `defaultEngineSectorRotationSiblings()`：
```go
func defaultEngineSectorRotationSiblings() (equityL1, assetOverlay, strategySleeve, legacyCompat map[string]float64) {
    equityL1 = map[string]float64{"semiconductor": 0.30, "financials": 0.14, "electronics": 0.10, "materials": 0.08, "industrials": 0.07, "consumer": 0.06, "healthcare": 0.05, "energy": 0.05, "telecom": 0.05, "utilities": 0.04, "real_estate": 0.04, "_cash_reserve": 0.02} // 12 個 100%; SA04 再刪非 L1
    assetOverlay = map[string]float64{"cash": 0.20, "gold": 0.05, "short_term_bonds": 0.05}
    strategySleeve = map[string]float64{"defensive": 0.10, "high_dividend": 0.05}
    legacyCompat = map[string]float64{"semiconductor": 0.19, "ai_supply_chain": 0.15, "robotics": 0.06, "financials": 0.11, "shipping": 0.07, "energy": 0.04, "electronics": 0.05, "consumer": 0.04, "industrial": 0.04, "leo_satellite": 0.05, "defensive": 0.10, "cash": 0.10}
    return
}
```

`internal/config/parameters_merge.go` 與 `parameters_validate.go` 同步：merge 邏輯在 `LegacyCompatAllocations.Value` 為空時套用 default；validate 對 `EquityL1Allocations` 拒絕 `cash/gold/defensive/jpy/short_term_bonds`，對其他 sibling 不接受 L1 ID。

`internal/portfolio/sector_rotator.go` 第 60 行 `engineCfg := params.Engine.SectorRotation.ToConfig()` 保留舊 `ToConfig()` 不變（用 `LegacyCompatAllocations`），但 `NewSectorRotator()` 改呼叫 `NewSectorRotatorWithL1Allocations(...)`；SA12 sunset 才把 `ToConfig` 一起改。

`configs/parameters.json`（line 7080+）：把 `engine.sector_rotation.base_allocations` 改名 `legacy_compat_allocations`，新增三個 sibling。

**Step 4: Run GREEN**

```bash
go test ./internal/sectorallocation/ -run 'TestLegacyCompatReader|TestClosureRules_LegacyCompat' -count=1
go test ./internal/config/ -run 'TestEngineSectorRotation_Split|TestEngineParameters_Validate_LegacyCompat' -count=1
go test ./internal/portfolio/ -run 'TestSectorRotator_NewConstructor|TestSectorRotator_LegacyCompatReaderFiresMetric' -count=1

# Negative static search
grep -rn "baseAllocations" internal/portfolio/sector_rotator.go  # only NewSectorRotatorWithConfig
grep -rn "engine.sector_rotation.base_allocations" configs/  # 0

gofmt -l internal/sectorallocation/legacy_compat.go internal/config/parameters.go internal/config/defaults_engine.go internal/config/parameters_merge.go internal/config/parameters_validate.go internal/portfolio/sector_rotator.go internal/monitoring/metrics/legacy_compat.go
go vet ./internal/sectorallocation/... ./internal/config/... ./internal/portfolio/... ./internal/monitoring/...
```

**Step 5: Commit**

```bash
git add internal/config/parameters.go internal/config/defaults_engine.go \
  internal/config/parameters_merge.go internal/config/parameters_validate.go \
  internal/config/parameters_test.go internal/config/testdata/parameters_api.golden.json \
  internal/sectorallocation/legacy_compat.go internal/sectorallocation/legacy_compat_test.go \
  internal/monitoring/metrics/legacy_compat.go internal/portfolio/sector_rotator.go \
  internal/portfolio/sector_rotator_test.go configs/parameters.json
git commit -m "feat(manifest): #SA03 split legacy BaseAllocations + compat reader"
```

**Step 6: Manifest evidence**

manifest SA03 row `pending → implemented`；Notes：
- implementation: `<commit>`
- observation: `pending until SA11 promotion`
- negative: `sector_allocation_legacy_compat_reads_total counter present at /metrics; cash/defensive never enters L1 target`

---

## Task 4: SA04 — Canonical 20-L1 WeightEngine + 唯一 projection

**Files:**
- Create: `internal/sectorallocation/projector.go`、`internal/sectorallocation/projector_test.go`、`internal/sectorallocation/engine_canonical_test.go`
- Modify: `internal/sectorallocation/model.go`（`SectorWeight.ID` → `industry.SectorID`；新增 `ProjectedTarget`/`AdjustmentEvent`/`DriverInputs`/`MacroAction`）、`engine.go`（interface 加 `ComputeProjectedTarget`）、`engine_impl.go`（以 `industry.L1Sectors()` 為唯一源；7 driver 各一次）、`internal/portfolio/sector_rotator.go`（移除 `normalizeAllocations`；`GeneratePlan` 收 `ProjectedTarget`）

**Interfaces:**

```go
type MacroAction string
const (MacroActionMixed/risk_off/carry_trade_unwind/sector_rotation MacroAction = ...)
type AdjustmentEvent struct { Sector industry.SectorID; Before, After float64; Reason string }
type ProjectedTarget struct {
    AsOfTradingDate string
    Target map[industry.SectorID]float64
    AdjustmentLog []AdjustmentEvent
    DriverProvenance map[string]string
    ModelVersion string
    FallbackReason string
}
type DriverInputs struct {
    Cycle, Seasonal, Linkage, Narrative, Macro, CapitalFlow, Theme, StrategicPrior map[industry.SectorID]float64
    MacroAction MacroAction
    AsOfTradingDate string
}
type ProjectionConstraints struct { MinSectorExposure, MaxSectorExposure, SumTolerance float64; AllowedL1 []industry.SectorID }
type Projector struct { Constraints ProjectionConstraints }
func NewDefaultProjector() *Projector
func (p *Projector) Project(raw map[industry.SectorID]float64, drivers DriverInputs) (ProjectedTarget, error)
type WeightEngine interface { ComputeWeights/ComputeWeight; ComputeProjectedTarget(ctx, DriverInputs) (ProjectedTarget, error) }
```

`Projector.Project` 是唯一 projection owner：拒絕非 L1 key、套 min/max、zero-sum 檢查、sum=1±1e-9 clamp。

**Step 1: Write RED**

`internal/sectorallocation/engine_canonical_test.go`：
```go
func TestWeightEngine_ProjectedTarget_Exactly20L1(t *testing.T) { ... }
func TestWeightEngine_NonL1BaseWeightIgnored(t *testing.T) { ... }
```

`internal/sectorallocation/projector_test.go`：
```go
func TestProjector_RejectsNonL1(t *testing.T) { ... }
func TestProjector_SumToleranceAndClamps(t *testing.T) { ... }
func TestProjector_EquityFundedTiltIsZeroSum(t *testing.T) { ... }
func TestProjector_AdjustmentLogProvenance(t *testing.T) { ... }
```

`internal/sectorallocation/engine_impl_test.go`（修改既有）：補 `TestDefaultEngine_DriverAppliedOnce` — mock 計數每個 provider 只被呼叫一次。

**Step 2: Run RED**

```bash
go test ./internal/sectorallocation/ -run 'TestWeightEngine_ProjectedTarget|TestProjector_|TestDefaultEngine_DriverAppliedOnce' -count=1
```

**Step 3: 最小實作**

`internal/sectorallocation/model.go`：
```go
package sectorallocation

import "github.com/kaecer68/atlas-go/internal/industry"

type SectorWeight struct {
    ID                industry.SectorID `json:"id"`
    Name              string            `json:"name"`
    BaseWeight        float64           `json:"base_weight"`
    AdjustedWeight    float64           `json:"adjusted_weight"`
    DerivationFactors []WeightFactor    `json:"derivation_factors,omitempty"`
    AdjustmentLog     []string          `json:"adjustment_log,omitempty"`
}

type ProjectedTarget struct {
    AsOfTradingDate    string                              `json:"as_of_trading_date"`
    Target             map[industry.SectorID]float64       `json:"target"`
    AdjustmentLog      []AdjustmentEvent                   `json:"adjustment_log"`
    DriverProvenance   map[string]string                   `json:"driver_provenance"`
    ModelVersion       string                              `json:"model_version"`
    FallbackReason     string                              `json:"fallback_reason,omitempty"`
}

type AdjustmentEvent struct {
    Sector industry.SectorID `json:"sector"`
    Before float64           `json:"before"`
    After  float64           `json:"after"`
    Reason string            `json:"reason"`
}

type MacroAction string
const (
    MacroActionMixed MacroAction = "mixed"
    MacroActionRiskOff MacroAction = "risk_off"
    MacroActionCarryTradeUnwind MacroAction = "carry_trade_unwind"
    MacroActionSectorRotation MacroAction = "sector_rotation"
)
```

`internal/sectorallocation/projector.go`：
```go
type Projector struct {
    Constraints ProjectionConstraints
}

func (p *Projector) Project(raw map[industry.SectorID]float64, drivers DriverInputs) (ProjectedTarget, error) {
    target := make(map[industry.SectorID]float64, 20)
    log := []AdjustmentEvent{}
    for id, w := range raw {
        if !industry.IsL1(id) {
            return ProjectedTarget{}, fmt.Errorf("non L1 key rejected: %s", id)
        }
        target[id] = w
    }
    // 7 driver 各一次
    for _, drv := range []map[industry.SectorID]float64{drivers.Cycle, drivers.Seasonal, drivers.Linkage, drivers.Narrative, drivers.Macro, drivers.CapitalFlow, drivers.Theme} {
        for id, delta := range drv {
            if !industry.IsL1(id) {
                return ProjectedTarget{}, fmt.Errorf("driver maps to non L1 key %s", id)
            }
            before := target[id]
            target[id] = before + delta
            log = append(log, AdjustmentEvent{Sector: id, Before: before, After: target[id], Reason: "driver"})
        }
    }
    // clamp + sum=1
    for id, w := range target {
        if w < p.Constraints.MinSectorExposure { w = p.Constraints.MinSectorExposure }
        if w > p.Constraints.MaxSectorExposure { w = p.Constraints.MaxSectorExposure }
        target[id] = w
    }
    s := 0.0
    for _, v := range target { s += v }
    if s == 0 { return ProjectedTarget{}, errors.New("zero sum after projection") }
    for id, w := range target { target[id] = w / s }
    // min/max re-apply after sum normalization; iterate up to 10 times
    for range 10 {
        clamped := false
        for id, w := range target {
            if w < p.Constraints.MinSectorExposure { w = p.Constraints.MinSectorExposure; clamped = true }
            if w > p.Constraints.MaxSectorExposure { w = p.Constraints.MaxSectorExposure; clamped = true }
            target[id] = w
        }
        s2 := 0.0
        for _, v := range target { s2 += v }
        for id, w := range target { target[id] = w / s2 }
        if !clamped { break }
    }
    return ProjectedTarget{Target: target, AdjustmentLog: log, ModelVersion: "v0.0.0.32-canonical", AsOfTradingDate: drivers.AsOfTradingDate}, nil
}
```

`internal/sectorallocation/engine.go`：interface 加 `ComputeProjectedTarget(ctx, DriverInputs) (ProjectedTarget, error)`。`engine_impl.go`：以 `industry.L1Sectors()` 為唯一源；7 driver 各被呼叫一次（用計數器 wrapper 測試）。非 L1 base weight（`_cash_reserve/materials/consumer/healthcare/industrials/utilities/real_estate`）在 `defaultEngine.ComputeWeights` 內被 `IsL1` 過濾掉；SA12 才正式從 `parameters.json` 刪除。

`internal/portfolio/sector_rotator.go`：
```go
// 移除 normalizeAllocations（owner 改為 Projector）
// GeneratePlan 改成接受 ProjectedTarget 為輸入（SA08 會再擴）
```

**Step 4: Run GREEN**

```bash
go test ./internal/sectorallocation/... -run 'TestWeightEngine_|TestProjector_|TestDefaultEngine_DriverAppliedOnce' -count=1 -v
go test ./internal/portfolio/... -count=1
grep -rn "normalizeAllocations" internal/portfolio/   # 預期 0
gofmt -l ./internal/sectorallocation ./internal/portfolio
go vet ./internal/sectorallocation/... ./internal/portfolio/...
```

**Step 5: Commit**

```bash
git add internal/sectorallocation/projector.go internal/sectorallocation/projector_test.go \
  internal/sectorallocation/engine_canonical_test.go internal/sectorallocation/model.go \
  internal/sectorallocation/engine.go internal/sectorallocation/engine_impl.go \
  internal/sectorallocation/engine_impl_test.go internal/portfolio/sector_rotator.go \
  internal/portfolio/sector_rotator_test.go
git commit -m "feat(manifest): #SA04 canonical 20-L1 WeightEngine with single projection"
```

**Step 6: Manifest evidence**

manifest SA04 row `pending → implemented`；Notes：
- implementation: `<commit>`
- observation: `pending until SA11 promotion`
- negative: `normalizeAllocations in sector_rotator.go → 0; non L1 base weight rejected by Projector`

---

## Task 5: SA05 — Capital-flow anti-corruption

**Files:**
- Create: `internal/capitalflow/action_mapper.go`、`internal/capitalflow/action_mapper_test.go`
- Modify: `internal/capitalflow/types.go`（新增 `CapitalFlowAction` enum + `CapitalFlowActionMapper` interface）、`internal/orchestrator/system_risk_session.go`（移除直接讀 `macroAssessment.PrimaryFlow` 餵給 rotator；改走 `collectCapitalFlowDrivers`）

**Interfaces:**

```go
type CapitalFlowAction string
const (
    CapitalFlowActionUnavailable CapitalFlowAction = "unavailable"
    CapitalFlowActionNeutral     CapitalFlowAction = "neutral"
    CapitalFlowActionRiskOn      CapitalFlowAction = "risk_on"
    CapitalFlowActionRiskOff     CapitalFlowAction = "risk_off"
)
type CapitalFlowActionMapper interface {
    MapperVersion() string
    Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[industry.SectorID]float64, error)
}
type NoOpCapitalFlowActionMapper struct{} // 預設：回 unavailable
type DefaultCapitalFlowActionMapper struct{ Version string }
```

`MapperVersion() = ""` ⇒ disabled（防 SA11 promotion 前誤啟用）。`NoOp` 預設在 composition root；`Default` 必須手動啟用且要求 walk-forward 證據（不在本 plan scope）。

**Step 1: Write RED**

```go
func TestCapitalFlowActionMapper_NilMapper_ReturnsUnavailable(t *testing.T) { ... }
func TestCapitalFlowActionMapper_Calibrating_ReturnsUnavailable(t *testing.T) { ... }
func TestCapitalFlowActionMapper_Degraded_ReturnsUnavailable(t *testing.T) { ... }
func TestCapitalFlowActionMapper_EligibleNoStrongSignal_Neutral(t *testing.T) { ... }
func TestCapitalFlowActionMapper_DefaultEmptyVersion_ReturnsUnavailable(t *testing.T) { ... }
func TestSystem_CollectCapitalFlowDrivers_FallbackReasonRecorded(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/capitalflow/ -run 'TestCapitalFlowActionMapper' -count=1
go test ./internal/orchestrator/ -run 'TestSystem_CollectCapitalFlowDrivers' -count=1
```

**Step 3: 最小實作**

`internal/capitalflow/types.go`：
```go
type CapitalFlowAction string
const (CapitalFlowActionUnavailable CapitalFlowAction = "unavailable"; CapitalFlowActionNeutral CapitalFlowAction = "neutral"; CapitalFlowActionRiskOn CapitalFlowAction = "risk_on"; CapitalFlowActionRiskOff CapitalFlowAction = "risk_off")
type CapitalFlowActionMapper interface { MapperVersion() string; Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[industry.SectorID]float64, error) }
```

`internal/capitalflow/action_mapper.go`：
```go
type NoOpCapitalFlowActionMapper struct{}
func (NoOpCapitalFlowActionMapper) MapperVersion() string { return "" }
func (NoOpCapitalFlowActionMapper) Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[industry.SectorID]float64, error) {
    return CapitalFlowActionUnavailable, nil, nil
}

type DefaultCapitalFlowActionMapper struct{ Version string }
func (d DefaultCapitalFlowActionMapper) MapperVersion() string { return d.Version }
func (d DefaultCapitalFlowActionMapper) Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[industry.SectorID]float64, error) {
    if d.Version == "" { return CapitalFlowActionUnavailable, nil, nil }
    if a.CalibrationStatus != CalibrationEligible || a.PrimaryFlow == "" { return CapitalFlowActionUnavailable, nil, nil }
    // mapping 仍保守：Default 必須 walk-forward 通過才可啟用
    return CapitalFlowActionNeutral, map[industry.SectorID]float64{}, nil
}
```

`internal/orchestrator/system_risk_session.go`：移除 `macroAssessment.PrimaryFlow` 直傳；改用 `collectCapitalFlowDrivers(s.strat.sectorAllocationSources.CapitalFlow, ctx, now)` 取得 `DriverInputs.CapitalFlow` 與 `FallbackReason`。

**Step 4: Run GREEN**

```bash
go test ./internal/capitalflow/... -run 'TestCapitalFlowActionMapper' -count=1
go test ./internal/orchestrator/... -run 'TestSystem_CollectCapitalFlowDrivers' -count=1
grep -rn 'macroAssessment.PrimaryFlow' internal/   # 預期 0 in non-test
grep -rn 'PrimaryFlow' internal/orchestrator/   # 預期 0
go vet ./internal/capitalflow/... ./internal/orchestrator/...
```

**Step 5: Commit**

```bash
git add internal/capitalflow/types.go internal/capitalflow/action_mapper.go \
  internal/capitalflow/action_mapper_test.go internal/orchestrator/system_risk_session.go \
  internal/orchestrator/system_risk_session_test.go
git commit -m "feat(manifest): #SA05 capital-flow anti-corruption with typed action enum"
```

**Step 6: Manifest evidence**

manifest SA05 row `pending → implemented`；Notes：mapper 預設 NoOp，Default 必須 `MapperVersion() != ""` 才能啟用；walk-forward 驗證屬未來工作。

---

## Task 6: SA06 — Composition root shared engine + six-path matrix

**Files:**
- Create: `internal/orchestrator/composition/root.go`、`internal/orchestrator/composition/wiring_test.go`
- Modify: `internal/orchestrator/system.go`（加 `sectorEngine`/`capitalFlowMapper`/`compositionPath` field + setters + `DisableSectorRotation`）、`internal/orchestrator/factory.go`、`internal/monitoring/dashboard_api.go`、`internal/monitoring/service/industry.go`、`cmd/atlas/main.go`（六 callsite 改走 `root.BuildSystem(path)`）

**Interfaces:**

```go
type CompositionPath string
const (PathAdminManual/PathAutoDaily/PathStressTestDaily/PathCLISimulation/PathAutoExperiment/PathLiveTrading CompositionPath = ...)
func (p CompositionPath) AllowsSectorRotation() bool  // 僅前四 true

type Root struct { Cfg config.Config; WeightEngine sectorallocation.WeightEngine; CapitalFlowMapper capitalflow.CapitalFlowActionMapper; IndustryService *monitoringservice.IndustryService }
func NewRoot(cfg config.Config) (*Root, error)  // WeightEngine 不可 nil（panic）
func (r *Root) BuildSystem(path CompositionPath) (*orchestrator.System, error)  // negative path 自動 DisableSectorRotation()

func (s *System) WithWeightEngine(e sectorallocation.WeightEngine) *System  // nil → panic
func (s *System) WithCapitalFlowMapper(m capitalflow.CapitalFlowActionMapper) *System
func (s *System) WithCompositionPath(p composition.CompositionPath) *System
func (s *System) DisableSectorRotation() *System  // 清空 strategyEvolver

func (s *IndustryService) WithWeightEngine(e sectorallocation.WeightEngine) *IndustryService
```

**Step 1: Write RED**

```go
func TestComposition_AllSixPaths_Defined(t *testing.T) { ... }
func TestComposition_FourPaths_AllowRotation(t *testing.T) { ... }
func TestComposition_TwoPaths_Negative(t *testing.T) { ... }
func TestIndustryService_NoPartialEngine(t *testing.T) { ... }
func TestDashboard_UsesCompositionRoot_WeightEngine(t *testing.T) { ... }
func TestRoot_NilWeightEngine_Panics(t *testing.T) { ... }
func TestLiveTrading_Path_NoSectorMutation(t *testing.T) { ... }
func TestAutoExperiment_Path_NoSectorMutation(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/orchestrator/composition/... -count=1
go test ./internal/orchestrator/... -run 'TestComposition|TestLiveTrading_Path|TestAutoExperiment_Path|TestIndustryService|TestDashboard_' -count=1
```

**Step 3: 最小實作**

`internal/orchestrator/composition/root.go`：
```go
package composition

import (
    "fmt"
    "github.com/kaecer68/atlas-go/internal/config"
    "github.com/kaecer68/atlas-go/internal/capitalflow"
    "github.com/kaecer68/atlas-go/internal/orchestrator"
    "github.com/kaecer68/atlas-go/internal/sectorallocation"
    monitoringservice "github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type CompositionPath string
const (
    PathAdminManual CompositionPath = "admin_manual"
    PathAutoDaily CompositionPath = "auto_daily"
    PathStressTestDaily CompositionPath = "stress_test_daily"
    PathCLISimulation CompositionPath = "cli_simulation"
    PathAutoExperiment CompositionPath = "auto_experiment"
    PathLiveTrading CompositionPath = "live_trading"
)

func (p CompositionPath) AllowsSectorRotation() bool {
    return p == PathAdminManual || p == PathAutoDaily || p == PathStressTestDaily || p == PathCLISimulation
}

type Root struct {
    Cfg config.Config
    WeightEngine sectorallocation.WeightEngine
    CapitalFlowMapper capitalflow.CapitalFlowActionMapper
    IndustryService *monitoringservice.IndustryService
}

func NewRoot(cfg config.Config) (*Root, error) {
    if cfg.LedgerDir == "" { return nil, fmt.Errorf("ledger dir required") }
    return &Root{Cfg: cfg, CapitalFlowMapper: capitalflow.NoOpCapitalFlowActionMapper{}}, nil
}

func (r *Root) SetWeightEngine(e sectorallocation.WeightEngine) { r.WeightEngine = e }
func (r *Root) BuildSystem(path CompositionPath) (*orchestrator.System, error) {
    if r.WeightEngine == nil { panic("composition root: WeightEngine required") }
    if r.IndustryService == nil { panic("composition root: IndustryService required") }
    sys, err := orchestrator.NewProductionSystemWithEventBus(r.Cfg, /* event bus, janus */ nil, nil)
    if err != nil { return nil, err }
    sys.WithWeightEngine(r.WeightEngine)
    sys.WithCapitalFlowMapper(r.CapitalFlowMapper)
    sys.WithCompositionPath(path)
    if !path.AllowsSectorRotation() { sys.DisableSectorRotation() }
    return sys, nil
}
```

`internal/orchestrator/system.go`：在 `StrategyLayer` 結構加 `sectorEngine sectorallocation.WeightEngine`、`capitalFlowMapper capitalflow.CapitalFlowActionMapper`、`compositionPath composition.CompositionPath` 三個欄位 + 對應 setter；`DisableSectorRotation()` 把 `strategyEvolver` 設為 nil 並 log。

`internal/monitoring/dashboard_api.go`：移除 `newWiredIndustryService` 內部 `NewDefaultEngine`；改為接收注入的 `WeightEngine`。

`internal/monitoring/service/industry.go`：移除 `NewIndustryService` 內 `sectorallocation.NewDefaultEngine(...)` 構造；改 `WithWeightEngine` setter。

`cmd/atlas/main.go`：
```text
// L817, L1036, L1154, L1192, L1817, L1933 六 callsite 全部改走：
root, _ := composition.NewRoot(cfg)
root.SetWeightEngine(industryService.WeightEngine())  // composition root 從 IndustryService 取得
sys, _ := root.BuildSystem(composition.PathAdminManual)  // 每 callsite 用對應 path
```

**Step 4: Run GREEN**

```bash
go test ./internal/orchestrator/composition/... -count=1 -v
go test ./internal/orchestrator/... -run 'TestComposition|TestLiveTrading_Path|TestAutoExperiment_Path|TestIndustryService|TestDashboard_' -count=1 -v
grep -rn "NewDefaultEngine" internal/monitoring/service/industry.go  # 預期 0
grep -rn "sectorallocation.NewDefaultEngine" internal/monitoring/dashboard_api.go  # 預期 0
grep -rn "orchestrator.NewProductionSystemWithEventBus" cmd/atlas/main.go  # 預期 0
go test ./... -count=1
go build ./... && gofmt -l . && golangci-lint run --timeout=5m && go generate . && git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

**Step 5: Commit**

```bash
git add internal/orchestrator/composition/root.go internal/orchestrator/composition/wiring_test.go \
  internal/orchestrator/system.go internal/orchestrator/factory.go \
  internal/monitoring/dashboard_api.go internal/monitoring/service/industry.go \
  cmd/atlas/main.go internal/orchestrator/system_test.go internal/orchestrator/factory_test.go \
  cmd/atlas/main_test.go
git commit -m "feat(manifest): #SA06 composition-root shared engine + six-path matrix"
```

**Step 6: Manifest evidence**

manifest SA06 row `pending → implemented`；Notes：six-path wiring matrix 證據；live 與 auto_experiment 走 `DisableSectorRotation` 路徑。

---

## Task 7: SA07 — 真實 current 20-L1 exposure

**Files:**
- Create: `internal/industry/symbol_l1_mapper.go`、`internal/industry/symbol_l1_mapper_test.go`、`internal/portfolio/sector_exposure.go`、`internal/portfolio/sector_exposure_test.go`、`internal/orchestrator/sector_exposure_test.go`
- Modify: `internal/orchestrator/system.go`（`currentSectorAllocations()` 重寫為計算真實 exposure）、`internal/orchestrator/system_risk_session.go`、`internal/orchestrator/composition/root.go`（注入 exposure calculator）

**Interfaces:**

```go
type SymbolL1Mapper struct { bySymbol map[string]industry.SectorID }
func NewSymbolL1Mapper(tree *industry.ClassificationTree) (*SymbolL1Mapper, error)
func (m *SymbolL1Mapper) ResolveL1(symbol string) (industry.SectorID, bool)

type ExposureGap struct { Symbol string; MarketValue float64; Reason string }
type SectorExposure struct {
    AsOfTradingDate string
    Weights map[industry.SectorID]float64  // 恰好 20 keys，sum=1 或 0
    TotalMarketValue float64
    Complete bool
    UnmappedSymbols []string
    UnmappedWeight float64
    UnpricedSymbols []string
    Gaps []ExposureGap
    PositionSource, PriceSource string
}
type SectorExposureCalculator struct { mapper L1SymbolResolver }
func (c *SectorExposureCalculator) Calculate(positions []domain.Position, quotes []domain.Quote, asOf time.Time) SectorExposure
```

**Step 1: Write RED**

`internal/industry/symbol_l1_mapper_test.go`：
```go
func TestSA_INV_10_SymbolL1MapperNormalizesTWSuffix(t *testing.T) { ... }
func TestSA_INV_02_SymbolL1MapperResolvesL2ToCanonicalParent(t *testing.T) { ... }
func TestSA_INV_02_SymbolL1MapperRejectsNoncanonicalRoot(t *testing.T) { ... }
func TestSA_INV_02_SymbolL1MapperRejectsCrossL1Duplicate(t *testing.T) { ... }
func TestSA_INV_02_SymbolL1MapperDoesNotFuzzyMap(t *testing.T) { ... }
```

`internal/portfolio/sector_exposure_test.go`：
```go
func TestSA_INV_10_ExposureUsesQuantityTimesTClose(t *testing.T) { ... }
func TestSA_INV_01_ExposureAlwaysReturnsExactly20L1Keys(t *testing.T) { ... }
func TestSA_INV_10_ExposureNeverCopiesTarget(t *testing.T) { ... }
func TestSA_INV_15_UnmappedPositiveWeightMakesExposureIncomplete(t *testing.T) { ... }
func TestSA_INV_15_UnmappedZeroQuantityDoesNotFallback(t *testing.T) { ... }
func TestSA_INV_15_MissingTPriceFailsClosed(t *testing.T) { ... }
func TestSA_INV_19_QuoteDateMismatchFailsClosed(t *testing.T) { ... }
func TestSA_INV_10_EmptyPortfolioReturnsNonNilTwentyZeroMap(t *testing.T) { ... }
func TestSA_INV_10_ExposureSumIsOneWhenComplete(t *testing.T) { ... }
func TestSA_INV_15_UnmappedListsAreStableSorted(t *testing.T) { ... }
```

`internal/orchestrator/sector_exposure_test.go`：
```go
func TestSA07_HeldPositionSymbolsAreIncludedInQuoteRequest(t *testing.T) { ... }
func TestSA_INV_10_SystemBuildsCurrentFromClosingResult(t *testing.T) { ... }
func TestSA_INV_15_IncompleteExposureDoesNotCallPolicyWriter(t *testing.T) { ... }
func TestSA07_CurrentExposureNotGatedByCapitalController(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/industry/ -run 'SA_INV_|SymbolL1Mapper' -count=1
go test ./internal/portfolio/ -run 'SectorExposure|SA_INV_10|SA_INV_15' -count=1
go test ./internal/orchestrator/ -run 'SA07|SA_INV_10|SA_INV_15' -count=1
```

**Step 3: 最小實作**

`internal/industry/symbol_l1_mapper.go`：
```go
type SymbolL1Mapper struct { bySymbol map[string]SectorID }
func NewSymbolL1Mapper(tree *ClassificationTree) (*SymbolL1Mapper, error) {
    m := &SymbolL1Mapper{bySymbol: make(map[string]SectorID, 200)}
    seen := make(map[string]SectorID)
    tree.Walk(func(s *Segment) {
        if s == nil { return }
        sym := normalize(s.Symbol())
        l1, ok := l1Ancestor(s)
        if !ok || !industry.IsL1(l1) { return }
        if existing, ok := seen[sym]; ok && existing != l1 {
            return m, fmt.Errorf("duplicate symbol %s maps to both %s and %s", sym, existing, l1)
        }
        seen[sym] = l1
        m.bySymbol[sym] = l1
    })
    return m, nil
}
func (m *SymbolL1Mapper) ResolveL1(sym string) (industry.SectorID, bool) {
    n := normalize(sym)
    id, ok := m.bySymbol[n]
    return id, ok
}
func normalize(s string) string { return strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(s, ".TW"))) }
```

`internal/portfolio/sector_exposure.go`：
```go
type SectorExposureCalculator struct { mapper L1SymbolResolver }
func (c *SectorExposureCalculator) Calculate(positions []domain.Position, quotes []domain.Quote, asOf time.Time) SectorExposure {
    weights := make(map[industry.SectorID]float64, 20)
    for _, id := range industry.L1Sectors() { weights[id] = 0 }
    quoteMap := make(map[string]domain.Quote, len(quotes))
    for _, q := range quotes {
        if q.AsOf.Format("2006-01-02") != asOf.Format("2006-01-02") { return SectorExposure{AsOfTradingDate: asOf.Format("2006-01-02"), Complete: false, Gaps: []ExposureGap{{Symbol: q.Symbol, Reason: "date_mismatch"}}} }
        quoteMap[q.Symbol] = q
    }
    total := 0.0
    unmappedValue := 0.0
    var gaps []ExposureGap
    var unmappedSyms []string
    var unpricedSyms []string
    for _, p := range positions {
        if p.Quantity <= 0 { continue }
        l1, ok := c.mapper.ResolveL1(p.Symbol)
        if !ok { unmappedValue += p.Quantity * p.AverageCost; unmappedSyms = append(unmappedSyms, p.Symbol); gaps = append(gaps, ExposureGap{Symbol: p.Symbol, MarketValue: p.Quantity * p.AverageCost, Reason: "unmapped_symbol"}); continue }
        q, ok := quoteMap[p.Symbol]
        if !ok || q.Last <= 0 { unpricedSyms = append(unpricedSyms, p.Symbol); gaps = append(gaps, ExposureGap{Symbol: p.Symbol, MarketValue: p.Quantity * p.AverageCost, Reason: "missing_t_price"}); continue }
        v := p.Quantity * q.Last
        weights[l1] += v
        total += v
    }
    if total == 0 {
        return SectorExposure{AsOfTradingDate: asOf.Format("2006-01-02"), Weights: weights, TotalMarketValue: 0, Complete: true, PositionSource: "simulation_closing_positions", PriceSource: "simulation_session_quotes"}
    }
    denominator := total + unmappedValue
    for id := range weights { weights[id] /= denominator }
    sort.Strings(unmappedSyms)
    sort.Strings(unpricedSyms)
    complete := unmappedValue == 0
    return SectorExposure{AsOfTradingDate: asOf.Format("2006-01-02"), Weights: weights, TotalMarketValue: denominator, Complete: complete, UnmappedSymbols: unmappedSyms, UnmappedWeight: unmappedValue / denominator, UnpricedSymbols: unpricedSyms, Gaps: gaps, PositionSource: "simulation_closing_positions", PriceSource: "simulation_session_quotes"}
}
```

`internal/orchestrator/system.go`：刪除 line 968-970 `currentSectorAllocations() return nil`；改為呼叫 `s.Sim().sectorExposure.Calculate(result.Positions, quotes, sessionDate)` 並回傳。`system_risk_session.go` 改用新版本；`updateCapitalMetrics` 不再被 `returnHistory < 2` 或 `capitalController == nil` 包住（暴露真實 current）。

**Step 4: Run GREEN**

```bash
go test ./internal/industry/ -run 'SymbolL1Mapper|SA_INV_' -count=1
go test ./internal/portfolio/ -run 'SectorExposure|SA_INV_' -count=1
go test ./internal/orchestrator/ -run 'SA07|SA_INV_' -count=1
go test -race ./internal/portfolio/ ./internal/orchestrator/ -count=1
gofmt -l internal/industry internal/portfolio internal/orchestrator
go vet ./internal/industry/... ./internal/portfolio/... ./internal/orchestrator/...
```

**Step 5: Commit**

```bash
git add internal/industry/symbol_l1_mapper.go internal/industry/symbol_l1_mapper_test.go \
  internal/portfolio/sector_exposure.go internal/portfolio/sector_exposure_test.go \
  internal/orchestrator/sector_exposure_test.go internal/orchestrator/system.go \
  internal/orchestrator/system_risk_session.go internal/orchestrator/system_test.go \
  internal/orchestrator/system_risk_session_test.go
git commit -m "feat(manifest): #SA07 simulation-closing current 20-L1 exposure"
```

**Step 6: Manifest evidence**

manifest SA07 row `pending → implemented`；Notes：`currentSectorAllocations()` 從 nil 改為真實計算；two-session `effective_session_unavailable` path 也會被 SA08 串接。

---

## Task 8: SA08 — Next-session persisted policy + allocator consumption + CLI live-sync isolation

**Files:**
- Create: `internal/sectorallocation/policy.go`、`internal/sectorallocation/policy_store.go`、`internal/sectorallocation/policy_store_test.go`、`internal/portfolio/sector_budget_allocator.go`、`internal/portfolio/sector_budget_allocator_test.go`、`internal/orchestrator/session_resolver.go`、`internal/orchestrator/session_resolver_test.go`、`internal/orchestrator/sector_allocation_closure.go`、`internal/orchestrator/sector_allocation_closure_test.go`、`internal/orchestrator/sector_allocation_two_session_test.go`、`internal/orchestrator/sector_allocation_live_negative_test.go`
- Modify: `internal/domain/session.go`、`internal/domain/session_test.go`、`internal/orchestrator/system.go`、`internal/orchestrator/system_dispatcher.go`、`internal/orchestrator/system_risk_session.go`、`internal/orchestrator/strategy_evolver.go`、`internal/orchestrator/strategy_evolver_test.go`、`internal/orchestrator/composition/root.go`、`internal/orchestrator/factory.go`、`internal/orchestrator/factory_test.go`、`internal/sim/engine.go`、`internal/sim/engine_test.go`、`cmd/atlas/main.go`、`cmd/atlas/main_test.go`、`cmd/atlas/simulation_mode_test.go`、`cmd/atlas/live_mode_test.go`

**Interfaces:**

```go
type SectorAllocationPolicy struct {
    PolicyID, SourceSessionID, AsOfTradingDate, EffectiveSessionID, EffectiveFrom, ModelVersion, SnapshotHash string
    Target map[industry.SectorID]float64
}
type SimulationMutationReceipt struct {
    ReceiptID, SourceSessionID, EffectiveSessionID, BeforePolicyHash, AfterPolicyHash string
    ChangedSectorCount int
}
type PolicyConsumptionReceipt struct {
    ReceiptID, PolicyID, PolicyHash, SourceSessionID, ConsumerSessionID, ConsumerTradingDate string
    LoadedBeforeOrders bool
    CappedSectors []industry.SectorID
}
type SimulationRollbackReceipt struct {
    ReceiptID, MutationReceiptID, BeforeRollbackHash, RestoredPolicyHash string
}
type SectorAllocationSnapshot struct {
    SnapshotID, SnapshotHash, SourceSessionID, AsOfTradingDate, EffectiveFrom, EffectiveSessionID, WeightSource, ModelVersion, CalibrationStatus, FallbackReason string
    Target, Current, Delta map[industry.SectorID]float64
    UnmappedSymbols []string
    UnmappedWeight float64
    Applied bool
    MutationReceipt *SimulationMutationReceipt
    ConsumptionReceipt *PolicyConsumptionReceipt
    RollbackReceipt *SimulationRollbackReceipt
}
type SnapshotReader interface { LatestSnapshot(ctx context.Context) (*SectorAllocationSnapshot, error) }
type ClosureStore interface {
    SnapshotReader
    RecordFallback(ctx context.Context, snap SectorAllocationSnapshot) (*SectorAllocationSnapshot, error)
    ApplyNextSessionPolicy(ctx context.Context, snap SectorAllocationSnapshot, policy SectorAllocationPolicy) (*SectorAllocationSnapshot, *SimulationMutationReceipt, error)
    EffectivePolicy(ctx context.Context, t time.Time) (*SectorAllocationPolicy, error)
    RecordConsumption(ctx context.Context, rec PolicyConsumptionReceipt) error
    RollbackPolicy(ctx context.Context, mutationID string) (*SimulationRollbackReceipt, error)
}
type TradingSessionResolver interface {
    NextTradingDate(ctx context.Context, asOf time.Time) (time.Time, error)
}
type ReplayNextSessionResolver struct { dataset *replay.Dataset }
```

**Step 1: Write RED**

```go
// internal/domain/session_test.go
func TestSA_INV_19_TradingDateComesFromSessionID(t *testing.T) { ... }
func TestSA_INV_19_MalformedSessionIDRejected(t *testing.T) { ... }
func TestSA_INV_19_EffectiveSessionIDPreservesMode(t *testing.T) { ... }
func TestSA_INV_19_SessionDateFieldCannotOverrideIDDate(t *testing.T) { ... }

// internal/sectorallocation/policy_store_test.go
func TestSA_INV_19_ApplyRejectsEffectiveFromEqualAsOf(t *testing.T) { ... }
func TestSA_INV_19_ApplyRejectsEffectiveFromBeforeAsOf(t *testing.T) { ... }
func TestSA_INV_11_AppliedPolicyAlwaysHasReceipt(t *testing.T) { ... }
func TestSA_INV_11_ReceiptHashesMatchPersistedPolicies(t *testing.T) { ... }
func TestSA_INV_11_ChangedSectorCountUsesTolerance(t *testing.T) { ... }
func TestSA_INV_15_ApplyFailureLeavesPolicyBytesUnchanged(t *testing.T) { ... }
func TestSA08_NoEffectiveChangeReturnsAppliedFalse(t *testing.T) { ... }
func TestSA08_RollbackRestoresExactPreviousPolicyHash(t *testing.T) { ... }
func TestSA08_RollbackToNilRemovesActivePolicy(t *testing.T) { ... }
func TestSA08_StaleRollbackRejected(t *testing.T) { ... }
func TestSA08_CorruptStoreFailsClosed(t *testing.T) { ... }
func TestSA08_ConcurrentApplyIsSerialized(t *testing.T) { ... }
func TestSA08_PersistedMapHashIsInputOrderIndependent(t *testing.T) { ... }

// internal/portfolio/sector_budget_allocator_test.go
func TestSA_INV_20_SectorTargetCapsBuyNotional(t *testing.T) { ... }
func TestSA_INV_20_ExistingSectorExposureReducesRemainingBudget(t *testing.T) { ... }
func TestSA_INV_20_AcceptedOrdersConsumeRemainingBudget(t *testing.T) { ... }
func TestSA_INV_15_UnmappedCandidateBlockedUnderActivePolicy(t *testing.T) { ... }
func TestSA08_ZeroSectorTargetBlocksBuy(t *testing.T) { ... }
func TestSA08_NoPolicyPreservesLegacySizing(t *testing.T) { ... }

// internal/sim/engine_test.go
func TestSA_INV_20_PolicyLoadedBeforeAnyOrderGeneration(t *testing.T) { ... }
func TestSA_INV_19_PolicyNotConsumedOnSourceSession(t *testing.T) { ... }
func TestSA_INV_20_PolicyConsumedOnEffectiveSession(t *testing.T) { ... }
func TestSA_INV_20_OptimizerOrdersAreSectorCapped(t *testing.T) { ... }
func TestSA_INV_20_LegacyOrdersAreSectorCapped(t *testing.T) { ... }
func TestSA_INV_20_ConsumptionReceiptRecordsChangedOrders(t *testing.T) { ... }
func TestSA_INV_15_PolicyReadFailureLeavesSimulationStateUnchanged(t *testing.T) { ... }
func TestSA_INV_15_ReceiptWriteFailureLeavesSimulationStateUnchanged(t *testing.T) { ... }
func TestSA08_NoPolicyKeepsExistingEngineBehavior(t *testing.T) { ... }

// internal/orchestrator/strategy_evolver_test.go
func TestSA_INV_11_ApplyReturnsFalseWithoutStore(t *testing.T) { ... }
func TestSA_INV_11_ApplyReturnsTrueOnlyWithReceipt(t *testing.T) { ... }
func TestSA_INV_15_ClosedGateDoesNotCallStore(t *testing.T) { ... }
func TestSA08_SuspendedStrategyDoesNotMutatePolicy(t *testing.T) { ... }
func TestSA08_NoEffectiveChangeDoesNotClaimApplied(t *testing.T) { ... }
```

`internal/orchestrator/sector_allocation_two_session_test.go`：
```go
func TestSA_INV_19_20_TwoSessionSectorPolicyClosure(t *testing.T) {
    // dataset: 2026-07-17 Friday, 2026-07-20 Monday
    // session1: session-20260717-replay
    // session2: session-20260720-replay
    // step 1: execute session1 → record orders/positions/results hash
    // step 2: build snapshot from session1 closing positions × session1 quotes
    // step 3: apply policy (as_of=2026-07-17, effective_from=2026-07-20)
    // step 4: assert receipt (source=session-20260717, effective=session-20260720)
    // step 5: assert session1 orders/positions/results hash unchanged
    // step 6: EffectivePolicy(session-20260717) == nil
    // step 7: clone session1 closing state → branch A (no policy) vs branch B (with policy)
    // step 8: execute session2 for both branches
    // step 9: assert branch B consumed policy, branch A order output unchanged
    // step 10: rollback policy; re-execute session2; assert branch B == branch A
    // step 11: live store bytes before/after policy-enabled CLI simulation are equal
}
```

`internal/orchestrator/sector_allocation_live_negative_test.go`：
```go
func TestSA_INV_14_LiveConstructorDoesNotReceiveSectorPolicyWriter(t *testing.T) { ... }
func TestSA_INV_14_LiveRunDoesNotCreateConsumptionReceipt(t *testing.T) { ... }
func TestSA_INV_13_AutoExperimentDoesNotReceiveSectorApplicationRuntime(t *testing.T) { ... }
```

import assertion：
```go
func TestSA_INV_14_NoLiveBrokerImportInSectorStack(t *testing.T) {
    bad := []string{`internal/sectorallocation`, `internal/portfolio/sector_budget_allocator.go`, `internal/sim`}
    for _, p := range bad {
        body := readFile(t, p)
        if strings.Contains(body, "internal/live") { t.Errorf("%s imports internal/live", p) }
    }
}
```

**Step 2: Run RED**

```bash
go test ./internal/domain ./internal/sectorallocation ./internal/portfolio \
  ./internal/sim ./internal/orchestrator ./cmd/atlas \
  -run 'SA_INV_11|SA_INV_14|SA_INV_19|SA_INV_20|SectorPolicy|TwoSession' -count=1
```

**Step 3: 最小實作**

`internal/sectorallocation/policy.go`：
```go
func (p SectorAllocationPolicy) Validate() error {
    if p.SourceSessionID == "" { return errors.New("source session id required") }
    if p.EffectiveFrom <= p.AsOfTradingDate { return fmt.Errorf("effective_from must be after as_of (got %s ≤ %s)", p.EffectiveFrom, p.AsOfTradingDate) }
    if len(p.Target) != 20 { return errors.New("policy must target exactly 20 L1") }
    return nil
}
func CanonicalHash(target map[industry.SectorID]float64) string {
    keys := make([]string, 0, 20)
    for id := range target { keys = append(keys, string(id)) }
    sort.Strings(keys)
    h := sha256.New()
    for _, k := range keys { fmt.Fprintf(h, "%s=%.12f;", k, target[industry.SectorID(k)]) }
    return hex.EncodeToString(h.Sum(nil))
}
```

`internal/sectorallocation/policy_store.go`：
```go
type FileClosureStore struct { path string; mu sync.Mutex }
func NewFileClosureStore(ledgerDir string) *FileClosureStore
func (s *FileClosureStore) ApplyNextSessionPolicy(ctx context.Context, snap SectorAllocationSnapshot, policy SectorAllocationPolicy) (*SectorAllocationSnapshot, *SimulationMutationReceipt, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    if err := policy.Validate(); err != nil { return nil, nil, err }
    state := s.readLocked()
    beforeHash := CanonicalHash(state.ActivePolicy.Target)
    afterHash := CanonicalHash(policy.Target)
    if beforeHash == afterHash { return &snap, &SimulationMutationReceipt{ChangedSectorCount: 0}, nil }
    receipt := &SimulationMutationReceipt{SourceSessionID: snap.SourceSessionID, EffectiveSessionID: policy.EffectiveSessionID, BeforePolicyHash: beforeHash, AfterPolicyHash: afterHash, ChangedSectorCount: countChanged(state.ActivePolicy.Target, policy.Target)}
    state.ActivePolicy = policy
    state.AppliedSnapshots = append(state.AppliedSnapshots, snap)
    state.MutationReceipts = append(state.MutationReceipts, receipt)
    snap.Applied = true
    snap.MutationReceipt = receipt
    if err := s.writeLocked(state); err != nil { return nil, nil, err }
    return &snap, receipt, nil
}
func (s *FileClosureStore) EffectivePolicy(ctx context.Context, t time.Time) (*SectorAllocationPolicy, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    state := s.readLocked()
    if state.ActivePolicy.EffectiveFrom > t.Format("2006-01-02") || state.ActivePolicy.EffectiveFrom <= "0000-00-00" { return nil, nil }
    return &state.ActivePolicy, nil
}
func (s *FileClosureStore) RollbackPolicy(ctx context.Context, mutationID string) (*SimulationRollbackReceipt, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    state := s.readLocked()
    var prev *SectorAllocationPolicy
    var prevReceipt *SimulationMutationReceipt
    for i, m := range state.MutationReceipts {
        if m.ReceiptID == mutationID {
            if i > 0 { prev = &state.AppliedSnapshots[i-1].MutationReceipt.Target; _ = prev }
            prevReceipt = &state.MutationReceipts[i]
            break
        }
    }
    if prevReceipt == nil { return nil, errors.New("mutation not found") }
    state.ActivePolicy = *prev
    receipt := &SimulationRollbackReceipt{MutationReceiptID: mutationID, RestoredPolicyHash: CanonicalHash(prev.Target), BeforeRollbackHash: prevReceipt.AfterPolicyHash}
    state.RollbackReceipts = append(state.RollbackReceipts, receipt)
    s.writeLocked(state)
    return receipt, nil
}
```

`internal/orchestrator/session_resolver.go`：
```go
type ReplayNextSessionResolver struct { dataset *replay.Dataset }
func (r *ReplayNextSessionResolver) NextTradingDate(ctx context.Context, asOf time.Time) (time.Time, error) {
    if r.dataset == nil { return time.Time{}, errors.New("replay dataset required") }
    next, ok := r.dataset.NextDate(asOf, 1)
    if !ok { return time.Time{}, errors.New("next trading date unavailable") }
    return next, nil
}
type NoOpNextSessionResolver struct{}
func (NoOpNextSessionResolver) NextTradingDate(ctx context.Context, asOf time.Time) (time.Time, error) {
    return time.Time{}, errors.New("no next session resolver configured")
}
```

`internal/portfolio/sector_budget_allocator.go`：
```go
type SectorBudgetAllocator struct{ mapper L1SymbolResolver }
type SectorBudgetContext struct {
    Policy sectorallocation.SectorAllocationPolicy
    EquityBudget float64
    CurrentMarketValue map[industry.SectorID]float64
    AllocatedThisRun map[industry.SectorID]float64
}
func (a *SectorBudgetAllocator) CapBuyNotional(ctx *SectorBudgetContext, symbol string, requested float64) (float64, bool, error) {
    l1, ok := a.mapper.ResolveL1(symbol)
    if !ok { return 0, false, errors.New("unmapped symbol under active policy") }
    budget := ctx.Policy.Target[l1] * ctx.EquityBudget
    used := ctx.CurrentMarketValue[l1] + ctx.AllocatedThisRun[l1]
    remaining := budget - used
    if remaining <= 0 { return 0, true, nil }
    if requested > remaining { return remaining, true, nil }
    return requested, false, nil
}
```

`internal/sim/engine.go`：新增 `RunWithStateForSession(ctx, state, session, regime, quotes, recs) (SimulationResult, error)`：clone state、讀 `EffectivePolicy(date)`、在 clone 跑 mark-to-market + 產單（通過 `SectorBudgetAllocator.CapBuyNotional`）、寫 consumption receipt、成功後 `*state = workingCopy`。`system_risk_session.go` 把 `updateCapitalMetrics` 的 rotation call 改為 `s.strat.sectorAllocationClosure.ApplyNextSession(ctx, snapshot, policy, nextSessionResolver)`；resolver 不可用時 fallback `effective_session_unavailable` + `Applied=false`。

`internal/orchestrator/strategy_evolver.go`：刪除假 true 邏輯；改回 `Applied bool` + `MutationReceipt` + `FallbackReason`。`orchestrator/composition/root.go`：在 `BuildSystem` 內把 `sectorAllocationClosure`（store + resolver + budget allocator）注入只允許 4 條 path；其他 path 不會呼叫 `ApplyNextSessionPolicy`。

`cmd/atlas/main.go`：
- 移除 `runSimulation()` 內的 `livestore.NewStateStore(...)` 與 `stateStore.UpdatePosition/UpdateRegime/...` 同步區塊（line 1889-1917）。CLI 模擬不再寫 live store。
- 將 `liveMode` 入口 (`runLiveTrading`) 改用 `root.BuildSystem(composition.PathLiveTrading)`，確保 live 拿到 DisableSectorRotation 的 system。
- `auto_experiment` scheduler entry 改用 `root.BuildSystem(composition.PathAutoExperiment)`。

**Step 4: Run GREEN**

```bash
go test ./internal/domain ./internal/sectorallocation ./internal/portfolio \
  ./internal/sim ./internal/orchestrator ./cmd/atlas \
  -run 'SA_INV_11|SA_INV_13|SA_INV_14|SA_INV_15|SA_INV_19|SA_INV_20|SA08' -count=1
go test -race ./internal/sectorallocation ./internal/sim ./internal/orchestrator \
  -run 'Policy|Consumption|Rollback|TwoSession' -count=1
go vet ./internal/domain/... ./internal/sectorallocation/... ./internal/portfolio/... \
  ./internal/sim/... ./internal/orchestrator/... ./cmd/atlas/...
gofmt -l internal/domain internal/sectorallocation internal/portfolio internal/sim internal/orchestrator cmd/atlas

# Negative static evidence (全部必須 0 命中)
grep -rn 'return true, fmt\.Sprintf\("Sector rotation applied' internal/orchestrator/
grep -rn 'ApplySectorRotation\(plan\).*modified' internal/orchestrator/
grep -rn 'effective_from.*AddDate\(0, 0, 1\)\|Next.*Weekday' internal/orchestrator/ internal/sectorallocation/
grep -rn 'internal/live\|internal/live/store' internal/sectorallocation/ internal/sim/ internal/portfolio/sector_budget_allocator.go
grep -rn 'livestore\.NewStateStore' cmd/atlas/main.go  # CLI simulation 已移除
```

**Step 5: Commit**

```bash
git add internal/sectorallocation/policy.go internal/sectorallocation/policy_store.go \
  internal/sectorallocation/policy_store_test.go \
  internal/portfolio/sector_budget_allocator.go internal/portfolio/sector_budget_allocator_test.go \
  internal/orchestrator/session_resolver.go internal/orchestrator/session_resolver_test.go \
  internal/orchestrator/sector_allocation_closure.go internal/orchestrator/sector_allocation_closure_test.go \
  internal/orchestrator/sector_allocation_two_session_test.go \
  internal/orchestrator/sector_allocation_live_negative_test.go \
  internal/domain/session.go internal/domain/session_test.go \
  internal/orchestrator/system.go internal/orchestrator/system_dispatcher.go \
  internal/orchestrator/system_risk_session.go internal/orchestrator/strategy_evolver.go \
  internal/orchestrator/strategy_evolver_test.go internal/orchestrator/composition/root.go \
  internal/orchestrator/factory.go internal/orchestrator/factory_test.go \
  internal/sim/engine.go internal/sim/engine_test.go cmd/atlas/main.go \
  cmd/atlas/main_test.go cmd/atlas/simulation_mode_test.go cmd/atlas/live_mode_test.go
git commit -m "feat(manifest): #SA08 next-session policy + allocator consumption + CLI live-sync isolation"
```

**Step 6: Manifest evidence**

manifest SA08 row `pending → implemented`；Notes：
- implementation: `<commit>`
- observation: 觀察期（SA11）首個 valid session 觸發時記錄
- negative: `grep return true, "Sector rotation applied" → 0; grep livestore.NewStateStore cmd/atlas/main.go → 0; no look-ahead integration test passes`

---

## Task 9: SA09 — REST/UI/MCP 同 snapshot parity

**Files:**
- Create: `internal/monitoring/service/industry_test.go` (擴充)、`internal/monitoring/api/industry/sector_allocation_plan_test.go` (擴充)、`shared_web/static/js/schemas/sector-allocation-snapshot.schema.json`、`shared_web/static/js/__tests__/sector-allocation-snapshot.test.mjs`、`shared_web/static/js/__tests__/contract.test.mjs` (擴充)、`cmd/atlas-mcp/server/tools_industry_ext_test.go` (擴充)、`testdata/sector_allocation_snapshot_response.json`
- Modify: `internal/monitoring/service/industry.go`、`internal/monitoring/api/industry/handlers.go`、`internal/monitoring/dashboard_api.go`、`cmd/atlas/main.go`、`cmd/atlas/main_test.go`、`shared_web/static/js/pages/industry.js`、`shared_web/static/js/page-shells/industry.js`、`shared_web/static/js/__tests__/api-contract.test.mjs`、`cmd/atlas-mcp/server/tools_industry_ext.go`、`docs/reference/tool-catalog.md`

**Step 1: Write RED**

```go
// internal/monitoring/api/industry/handlers_test.go
func TestSA_INV_12_SectorAllocationHandlerReturnsPersistedSnapshot(t *testing.T) { ... }
func TestSA_INV_12_HandlerDoesNotCallWeightEngine(t *testing.T) { ... }
func TestSA_INV_15_FallbackSnapshotPreservesMachineReason(t *testing.T) { ... }
func TestSA_INV_15_NoSnapshotReturnsTypedUnavailableNotFakeZero(t *testing.T) { ... }
func TestSA_INV_11_HandlerRejectsAppliedWithoutReceipt(t *testing.T) { ... }
func TestSA09_ResponseHasExactlyTwentyTargetCurrentDeltaKeys(t *testing.T) { ... }
func TestSA09_ResponseUsesPersistedTimestampNotRequestTime(t *testing.T) { ... }
func TestSA09_IndustryDetailWeightsMatchLatestSnapshot(t *testing.T) { ... }
```

```js
// shared_web/static/js/__tests__/sector-allocation-snapshot.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { buildSectorAllocationViewModel, buildSectorAllocationHTML } from '../../static/js/components/sector-allocation-snapshot.js';
import snapshot from '../fixtures/sector_allocation_snapshot_response.json' with { type: 'json' };

test('view renders target/current/delta and applied state', () => {
    const vm = buildSectorAllocationViewModel(snapshot);
    assert.equal(vm.targets.length, 20);
    assert.equal(vm.applied, true);
    assert.equal(vm.fallbackReason, '');
});
test('current_exposure_incomplete renders unmapped symbols and weight', () => { ... });
test('Applied=false never renders 已套用', () => { ... });
test('observation_gate_closed renders 僅觀察', () => { ... });
test('derivation explains base, adjustments, target, current, delta', () => { ... });
```

```go
// cmd/atlas-mcp/server/tools_industry_ext_test.go
func TestSA_INV_12_SectorAllocationMCPPayloadEqualsREST(t *testing.T) { ... }
func TestSA_INV_15_SectorAllocationMCPPreservesFallback(t *testing.T) { ... }
func TestSA09_MCPHitsOnlyCanonicalSectorAllocationEndpoint(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/monitoring/service ./internal/monitoring/api/industry ./internal/monitoring \
  ./cmd/atlas-mcp/server -run 'SA_INV_11|SA_INV_12|SA_INV_15|SA09|SectorAllocation' -count=1
node --test shared_web/static/js/__tests__/sector-allocation-snapshot.test.mjs \
  shared_web/static/js/__tests__/contract.test.mjs
```

**Step 3: 最小實作**

`internal/monitoring/api/industry/handlers.go`：`HandleSectorAllocationPlan` 改為呼叫 `h.Svc.GetLatestSectorAllocation(ctx)`；不再做 `ComputeWeights`。response 完全對應 `SectorAllocationSnapshot` 的 typed payload（`snapshot_id/hash/source_session_id/as_of_trading_date/effective_from/effective_session_id/target/current/delta/weight_source/model_version/calibration_status/fallback_reason/applied/unmapped_symbols/unmapped_weight/derivation/mutation_receipt/consumption_receipt/rollback_receipt`）。`IndustryService.GetLatestSectorAllocation(ctx)` 內部呼叫 `ClosureStore.LatestSnapshot`；snapshot 不存在時回 503 + `fallback_reason=snapshot_unavailable`；`Applied=true` 但 receipt 為 nil 時回 500（corrupt state）。

`internal/monitoring/service/industry.go`：
- `NewIndustryService` 移除 `sectorallocation.NewDefaultEngine(...)`；改 `WithWeightEngine(...)` setter。
- `GetIndustryDetail` 改由 `SectorAllocationSnapshot` 取得 `weight`/`weight_derivation`/`recommendation.current_weight`/`recommendation.target_weight`/`recommendation.delta`；不再使用 `baseWeight*1.2/1.1/0.7`。
- `GetIndustryOverview` 仍可保留為 cycle overview，但不得被 allocation panel 或 MCP 採用。

`cmd/atlas-mcp/server/tools_industry_ext.go`：`sector_allocation_plan` tool 改為純 passthrough（呼叫 REST 取得 response），但 description 明確：「Latest persisted simulation sector-allocation snapshot, including target/current/delta, provenance, fallback status, mutation receipt, and next-session consumption evidence.」

`shared_web/static/js/pages/industry.js`：把 `renderIndustryMap` 拆成 `buildSectorAllocationViewModel` / `buildSectorAllocationHTML`，畫面顯示 target/current/delta/as_of/effective_from/model_version/calibration_status/source/applied/fallback/unmapped/derivation/receipt，並依 fallback 顯示 banner（`current_exposure_incomplete` / `observation_gate_closed` / `snapshot_unavailable`）。

**Step 4: Run GREEN**

```bash
go test ./internal/monitoring/service ./internal/monitoring/api/industry ./internal/monitoring \
  ./cmd/atlas-mcp/server -run 'SA_INV_11|SA_INV_12|SA_INV_15|SA09|SectorAllocation' -count=1
node --test shared_web/static/js/__tests__/*.mjs
npm --prefix client_web run build
npm --prefix admin_web run build
go generate ./cmd/atlas-mcp/server
go vet ./internal/monitoring/... ./cmd/atlas-mcp/server

# Negative static evidence
grep -rn 'ComputeWeights(ctx, now)' internal/monitoring/api/industry/handlers.go  # 0
grep -rn 'TargetWeight = baseWeight \* (1\.2|1\.1|0\.7)' internal/monitoring/service/industry.go  # 0
grep -rn 'adjusted_weight.*base_weight' shared_web/static/js/pages/industry.js  # 0
```

**Step 5: Commit**

```bash
git add internal/monitoring/service/industry.go internal/monitoring/service/industry_test.go \
  internal/monitoring/api/industry/handlers.go internal/monitoring/api/industry/sector_allocation_plan_test.go \
  internal/monitoring/api/industry/handlers_sector_allocation_wired_test.go \
  internal/monitoring/dashboard_api.go internal/monitoring/dashboard_api_test.go \
  cmd/atlas/main.go cmd/atlas/main_test.go \
  shared_web/static/js/pages/industry.js shared_web/static/js/page-shells/industry.js \
  shared_web/static/js/schemas/sector-allocation-snapshot.schema.json \
  shared_web/static/js/__tests__/sector-allocation-snapshot.test.mjs \
  shared_web/static/js/__tests__/contract.test.mjs shared_web/static/js/__tests__/api-contract.test.mjs \
  cmd/atlas-mcp/server/tools_industry_ext.go cmd/atlas-mcp/server/tools_industry_ext_test.go \
  cmd/atlas-mcp/server/tools_test.go docs/reference/tool-catalog.md \
  testdata/sector_allocation_snapshot_response.json
git commit -m "feat(manifest): #SA09 unified REST/Web/MCP snapshot parity"
```

**Step 6: Manifest evidence**

manifest SA09 row `pending → implemented`；Notes：REST/MCP/Web 三面 payload equality test passes；schema 鎖 20 L1 keys。

---

## Task 10: SA10 — F06 shadow ranking（真實 outcome + TAIEX benchmark）

**Files:**
- Create: `internal/strategy/comparison_store.go`、`internal/strategy/comparison_store_test.go`、`internal/strategy/shadow_evaluator.go`、`internal/strategy/shadow_evaluator_test.go`、`internal/strategy/f06_engine.go`（含 `ComputeWarmingUpState`）、`internal/strategy/f06_types_test.go`、`internal/marketdata/taiex_benchmark.go`、`internal/marketdata/taiex_benchmark_test.go`、`shared_web/static/js/components/shadow-ranking.js`、`shared_web/static/js/__tests__/strategy-ranking-warming.test.mjs`
- Modify: `internal/strategy/types.go`、`internal/strategy/comparison.go`、`internal/strategy/strategy_test.go`、`internal/orchestrator/system.go`、`internal/orchestrator/system_dispatcher.go`、`internal/orchestrator/composition/root.go`、`internal/orchestrator/system_plugin_accessors.go`、`internal/recommender/deps.go`、`internal/recommender/adapters.go`、`internal/recommender/handler.go`、`internal/recommender/handler_test.go`、`internal/recommender/adapters_test.go`、`cmd/atlas/wire_recommender.go`、`cmd/atlas/wire_recommender_test.go`、`cmd/atlas-mcp/server/tools_recommendation.go`、`shared_web/static/js/pages/strategies.js`

**Interfaces:**

```go
const EvaluationModeShadow = "shadow"
type StrategyDailyObservation struct { TradingDate, StrategyID, EvaluationMode string; DailyReturn, BenchmarkReturn, Outperformance float64; OutcomeCount int }
type BenchmarkObservation struct { TradingDate, SourceID, ReasonCode string; Return float64; Available bool }
type WarmingUpState struct { Status, LastTradingDate, ReasonCode string; SampleDays, MinHistoryDays, DaysUntilEligible int }
type RankedStrategy struct { Rank, SampleDays int; StrategyID, EvaluationMode string; Score float64 }
type RankingSnapshot struct { AsOfTradingDate string; WarmingUp WarmingUpState; Ranked []RankedStrategy; DeployedMix map[string]float64; Benchmark BenchmarkObservation }
type ComparisonDay struct { TradingDate string; Benchmark BenchmarkObservation; Observations []StrategyDailyObservation; DeployedMix map[string]float64 }
type ComparisonStore interface { Load(ctx) ([]ComparisonDay, error); Upsert(ctx, day ComparisonDay) error }
type FileComparisonStore struct { path string; maxDays int; mu sync.Mutex }

type ShadowStrategyEvaluator struct{ /* *Registry */ }
func (e *ShadowStrategyEvaluator) Evaluate(outcomes []domain.RecommendationOutcome, tradingDate time.Time, benchmark BenchmarkObservation) []StrategyDailyObservation
// 規則：zero tradingDate 或 !benchmark.Available → nil
//       IsSynthetic || !PassedGuards || empty AgentID/Symbol/Conviction<=0 → skip
//       dedupe AgentID+\x00+Symbol (higher Conviction wins)
//       per enabled strategy: conviction-weighted mean ForwardReturn
//       每筆 observation.EvaluationMode = "shadow"

var ErrTAIEXBenchmarkUnavailable = errors.New("taiex benchmark unavailable")
type TAIEXBenchmarkProvider interface { DailyReturn(ctx, tradingDate time.Time) (DailyBenchmark, error) }
type FileTAIEXBenchmarkProvider struct { macroDir string } // 觀察期先 unavailable

func NewComparisonEngine(window int, store ComparisonStore) (*ComparisonEngine, error)
func ComputeWarmingUpState(dates []string, minDays int, asOf string) WarmingUpState
```

**Step 1: Write RED**

```go
// internal/strategy/comparison_store_test.go
func TestF06_FileComparisonStore_LastWriteWinsSameDate(t *testing.T) { ... }
func TestF06_FileComparisonStore_RestartRoundTrip(t *testing.T) { ... }
func TestF06_FileComparisonStore_SortedByDate(t *testing.T) { ... }
func TestF06_FileComparisonStore_MaxDaysTrims(t *testing.T) { ... }
func TestF06_FileComparisonStore_CorruptReturnsErr(t *testing.T) { ... }
func TestF06_FileComparisonStore_ConcurrentUpsertSafe(t *testing.T) { ... }

// internal/strategy/shadow_evaluator_test.go
func TestF06_ShadowEvaluator_OnlyRealPassedOutcomes(t *testing.T) { ... }
func TestF06_ShadowEvaluator_DuplicateAgentSymbolLastWriteWins(t *testing.T) { ... }
func TestF06_ShadowEvaluator_BenchmarkUnavailableReturnsEmpty(t *testing.T) { ... }
func TestF06_ShadowEvaluator_TradingDateZeroReturnsEmpty(t *testing.T) { ... }
func TestF06_ShadowEvaluator_EvaluationModeIsShadow(t *testing.T) { ... }

// internal/strategy/f06_engine_test.go
func TestF06_RecordDay_BenchmarkUnavailable_NoStoreWrite(t *testing.T) { ... }
func TestF06_RecordDay_SameDateReplacesOldDay(t *testing.T) { ... }
func TestF06_Reload_PreservesExactSampleDays(t *testing.T) { ... }
func TestF06_Ranking_WarmingUp_NoHistory(t *testing.T) { ... }
func TestF06_Ranking_WarmingUp_BelowFloor(t *testing.T) { ... }
func TestF06_Ranking_Eligible_AllEvaluationModeShadow(t *testing.T) { ... }
func TestF06_Ranking_SortScoreDescThenIDAsc(t *testing.T) { ... }
func TestF06_WarmingUpState_NoHistory(t *testing.T) { ... }
func TestF06_WarmingUpState_BelowFloor(t *testing.T) { ... }
func TestF06_WarmingUpState_DuplicateDateNotCounted(t *testing.T) { ... }

// internal/marketdata/taiex_benchmark_test.go
func TestF06_FileTAIEXBenchmarkProvider_AvailablePreviousTradingDay(t *testing.T) { ... }
func TestF06_FileTAIEXBenchmarkProvider_MissingTargetDate(t *testing.T) { ... }
func TestF06_FileTAIEXBenchmarkProvider_NoEarlierValidDate(t *testing.T) { ... }
func TestF06_FileTAIEXBenchmarkProvider_InvalidCloseRejected(t *testing.T) { ... }
func TestF06_FileTAIEXBenchmarkProvider_NoLivePriceFallback(t *testing.T) { ... } // 拒絕 fallback 到 time.Now()

// internal/orchestrator/sector_allocation_closure_test.go (擴充)
func TestF06_RecordShadowStrategyDay_RealOutcomes(t *testing.T) { ... }
func TestF06_RecordShadowStrategyDay_SyntheticSkipped(t *testing.T) { ... }
func TestF06_RecordShadowStrategyDay_BenchmarkUnavailable_NoWrite(t *testing.T) { ... }
func TestF06_RecordShadowStrategyDay_SessionIDDateNotRecordedAt(t *testing.T) { ... }
func TestF06_RecordShadowStrategyDay_DeployedMixPersisted(t *testing.T) { ... }
func TestF06_RecordShadowStrategyDay_TopWeightNotUsedAsStrategyID(t *testing.T) { ... }

// internal/recommender/handler_test.go (擴充)
func TestF06_HandleRecommendations_FreeTierHasNoStrategyRanking(t *testing.T) { ... }
func TestF06_HandleRecommendations_RegisteredWarmingUp_NoFakeList(t *testing.T) { ... }
func TestF06_HandleRecommendations_RegisteredEligible_UsesStoreRanking(t *testing.T) { ... }
func TestF06_HandleRecommendations_EvaluationModeIsShadow(t *testing.T) { ... }
func TestF06_HandleRecommendations_StoreUnavailable_AddsWarning(t *testing.T) { ... }
func TestF06_HandleRecommendations_NoHardcodedRankingLiteral(t *testing.T) { ... }
func TestF06_HandleRecommendations_DeployedMixPersistedSeparately(t *testing.T) { ... }
```

frontend：
```js
// shared_web/static/js/__tests__/strategy-ranking-warming.test.mjs
test('shadow ranking renders calibrating state', () => { ... });
test('shadow ranking renders below_floor state with daysUntilEligible', () => { ... });
test('shadow ranking renders eligible state with deployed mix separated', () => { ... });
test('shadow ranking renders null state without fake ranking', () => { ... });
```

MCP：
```go
func TestF06_RecommendationTool_EchoesEvaluationMode(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/strategy/... -run 'F06_|TestComparisonEngine|TestNewComparisonEngine' -race -count=1
go test ./internal/marketdata/... -run 'F06_FileTAIEXBenchmarkProvider' -count=1
go test ./internal/orchestrator/... -run 'F06_|Test.*ShadowStrategy|Test.*ComparisonDay' -race -count=1
go test ./internal/recommender/... -run 'F06_|TestHandleRecommendations|TestHandleLoggedIn' -race -count=1
go test ./cmd/atlas/... -run 'TestWireRecommenderDeps' -race -count=1
go test ./cmd/atlas-mcp/... -run 'F06_Recommendation' -race -count=1
node --test shared_web/static/js/__tests__/strategy-ranking-warming.test.mjs
```

**Step 3: 最小實作**

`internal/strategy/comparison_store.go`：
```go
type FileComparisonStore struct { path string; maxDays int; mu sync.Mutex }
func NewFileComparisonStore(path string, maxDays int) *FileComparisonStore
func (s *FileComparisonStore) Load(ctx context.Context) ([]ComparisonDay, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    raw, err := os.ReadFile(s.path)
    if errors.Is(err, os.ErrNotExist) { return nil, nil }
    if err != nil { return nil, err }
    var days []ComparisonDay
    if err := json.Unmarshal(raw, &days); err != nil { return nil, fmt.Errorf("corrupt store: %w", err) }
    return days, nil
}
func (s *FileComparisonStore) Upsert(ctx context.Context, day ComparisonDay) error {
    s.mu.Lock(); defer s.mu.Unlock()
    days, _ := s.Load(ctx)
    replaced := false
    for i, d := range days {
        if d.TradingDate == day.TradingDate { days[i] = day; replaced = true; break }
    }
    if !replaced { days = append(days, day) }
    sort.Slice(days, func(i, j int) bool { return days[i].TradingDate < days[j].TradingDate })
    if s.maxDays > 0 && len(days) > s.maxDays { days = days[len(days)-s.maxDays:] }
    raw, _ := json.MarshalIndent(days, "", "  ")
    return os.WriteFile(s.path, raw, 0o644)
}
```

`internal/strategy/shadow_evaluator.go`：
```go
type ShadowStrategyEvaluator struct{ registry *strategy.Registry }
func NewShadowStrategyEvaluator(r *strategy.Registry) *ShadowStrategyEvaluator { return &ShadowStrategyEvaluator{registry: r} }
func (e *ShadowStrategyEvaluator) Evaluate(outcomes []domain.RecommendationOutcome, tradingDate time.Time, benchmark BenchmarkObservation) []StrategyDailyObservation {
    if tradingDate.IsZero() || !benchmark.Available { return nil }
    enabled := make(map[string]struct{}, len(e.registry.Strategies))
    for _, s := range e.registry.Strategies { if s.Enabled { enabled[s.ID] = struct{}{} } }
    obs := make(map[string]StrategyDailyObservation, len(enabled))
    for _, o := range outcomes {
        if o.IsSynthetic || !o.PassedGuards || o.AgentID == "" || o.Symbol == "" || o.Conviction <= 0 { continue }
        if _, ok := enabled[o.AgentID]; !ok { continue }
        key := o.AgentID + "\x00" + o.Symbol
        if existing, ok := obs[key]; ok {
            if o.Conviction <= existing.Outperformance { continue } // 保留較高 conviction
        }
        obs[key] = StrategyDailyObservation{TradingDate: tradingDate.Format("2006-01-02"), StrategyID: o.AgentID, EvaluationMode: EvaluationModeShadow, DailyReturn: o.ForwardReturn, BenchmarkReturn: benchmark.Return, Outperformance: o.ForwardReturn - benchmark.Return, OutcomeCount: 1}
    }
    out := make([]StrategyDailyObservation, 0, len(obs))
    for _, v := range obs { out = append(out, v) }
    return out
}
```

`internal/strategy/comparison.go`：把 `NewComparisonEngine(window int)` 改為 `NewComparisonEngine(window int, store ComparisonStore) (*ComparisonEngine, error)`；`RecordDay(ctx, day)` 內檢查 `day.Benchmark.Available`，否則不寫 store；`Ranking(ctx)` 依 `ComputeWarmingUpState` 決定是否回 `warming_up` 狀態；保證 `Ranked[].EvaluationMode == "shadow"`。

`internal/orchestrator/composition/root.go`：`BuildSystem` 內建 `FileComparisonStore(ledgerDir/strategy_comparison.json, 365)` 與 `ShadowStrategyEvaluator` 與 `FileTAIEXBenchmarkProvider` 注入 `StrategyLayer`。`system.go` 在 `recordShadowStrategyDay` 內：`tradingDate := domain.SessionDateFromID(session.ID)`、呼叫 `TAIEXBenchmarkProvider.DailyReturn(ctx, tradingDate)`、若 `Available=false` 不寫 store、回 `warming_up + benchmark_unavailable`、寫 log。`strategy.go:445` top-weight strategy 不得當 `StrategyID`；`recordShadowStrategyDay` 改用 `DeployedMix: maps.Clone(lastDeployedMix)`。

`internal/recommender/handler.go`：移除 `[]string{"growth","momentum","all_weather","value","defensive"}` hardcode；`buildShadowStrategy(rankingProvider, *StrategyRecommendation, *[]string)` 取代 `buildPremiumStrategy`；`StrategyRecommendation` 加 `EvaluationMode`、`EvaluationLabel`、`RankedWithScore`、`WarmingUp`、`DeployedMix`、`Benchmark` 欄位。既有 `TestHandleRecommendations_EntrySignalFromComparisonEngine` 改為斷言 `EvaluationMode=="shadow"` + `RankedWithScore` 含 `growth`，EntrySignal/StopLoss 字串斷言移除（標 `Deprecated`）。

**Step 4: Run GREEN**

```bash
go test ./internal/strategy/... -run 'F06_|TestComparisonEngine|TestNewComparisonEngine' -race -count=1
go test ./internal/marketdata/... -run 'F06_FileTAIEXBenchmarkProvider' -count=1
go test ./internal/orchestrator/... -run 'F06_|Test.*ShadowStrategy|Test.*ComparisonDay' -race -count=1
go test ./internal/recommender/... -run 'F06_|TestHandleRecommendations|TestHandleLoggedIn' -race -count=1
go test ./cmd/atlas/... -run 'TestWireRecommenderDeps' -race -count=1
go test ./cmd/atlas-mcp/... -run 'F06_Recommendation' -race -count=1
node --test shared_web/static/js/__tests__/strategy-ranking-warming.test.mjs
go test ./... -count=1 && go build ./... && gofmt -l . && go vet ./... && go generate . && git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/ci/check_doc_ports.sh
bash scripts/ci/check_atlas_mcp_docs_consistency.sh
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md

# Negative evidence
git grep -n '\[\]string{"growth"' internal/recommender/   # 0
git grep -n 'strategy.NewComparisonEngine' internal/ cmd/   # ≤1 (composition root only)
git grep -n 'evaluation_mode.*shadow\|EvaluationModeShadow' internal/ shared_web/   # 必須有 hit
git grep -n 'tools_strategy_ranker' cmd/atlas-mcp/server/ internal/strategy_ranker/   # 0
```

**Step 5: Commit**

```bash
git add internal/strategy/comparison.go internal/strategy/comparison_store.go \
  internal/strategy/comparison_store_test.go internal/strategy/shadow_evaluator.go \
  internal/strategy/shadow_evaluator_test.go internal/strategy/f06_engine.go \
  internal/strategy/f06_types_test.go internal/strategy/types.go internal/strategy/strategy_test.go \
  internal/marketdata/taiex_benchmark.go internal/marketdata/taiex_benchmark_test.go \
  internal/orchestrator/system.go internal/orchestrator/system_dispatcher.go \
  internal/orchestrator/composition/root.go internal/orchestrator/system_plugin_accessors.go \
  internal/recommender/deps.go internal/recommender/adapters.go internal/recommender/handler.go \
  internal/recommender/handler_test.go internal/recommender/adapters_test.go \
  cmd/atlas/wire_recommender.go cmd/atlas/wire_recommender_test.go \
  cmd/atlas-mcp/server/tools_recommendation.go \
  shared_web/static/js/components/shadow-ranking.js \
  shared_web/static/js/pages/strategies.js \
  shared_web/static/js/__tests__/strategy-ranking-warming.test.mjs
git commit -m "feat(manifest): #SA10 isolated shadow real strategy ranking"
```

**Step 6: Manifest evidence**

manifest SA10 row `pending → implemented`；Notes：
- `comparisonStore` 拒絕 synthetic row
- `RecordDay` 拒絕 `benchmark.Available=false`
- `EvaluationModeShadow` 強制
- TAIEX dated snapshot populator 另開 ID（推薦 `#BK-XX dated TAIEX capture`），SA10 觀察期先全 `warming_up + benchmark_unavailable`

---

## Task 11: SA11 — Dark launch + observation window（工程穩定性 only）

**Files:**
- Create: `internal/monitoring/api/pipeline/sector_allocation_closure_state.go`、`internal/monitoring/api/pipeline/sector_allocation_closure_state_test.go`、`internal/orchestrator/sector_allocation_closure_metrics.go`、`internal/orchestrator/sector_allocation_closure_metrics_test.go`、`internal/sectorallocation/legacy_counter.go`、`internal/sectorallocation/legacy_counter_test.go`、`cmd/experimental/sector-allocation-closure-preflight/main.go`、`cmd/experimental/sector-allocation-closure-preflight/main_test.go`、觀察期 runbook 與 observation log 與 rollback drill log（將於 SA11.B 與 SA12.D 建立）
- Modify: `internal/config/parameters.go`（新增 `SectorAllocationClosureParameters`）、`internal/config/config.go`（新增 `SECTOR_ALLOCATION_CLOSURE_ENABLED` env）、`internal/orchestrator/composition/root.go`、`cmd/atlas/main.go`、`configs/parameters.json`、`scripts/ci/check_no_duplicate_preflight.sh`、`docs/specs/experimental-feature-launch-gate.md`、`docs/documentation-map.md`

**Flag（鎖死 heuristic）**：
```json
"orchestrator": {
  "sector_allocation_closure": {
    "enabled":            { "value": false, "source": "experimental", "rationale": "SA11 dark launch; promotion only flips value, not source" },
    "auto_observation":   { "value": false, "source": "experimental", "rationale": "manual only" },
    "fallback_legacy":    { "value": false, "source": "experimental", "rationale": "SA12 sunset gate" }
  }
}
```

**State schema**：`data/state/sector-allocation-closure.json`
```json
{
  "status": {
    "running": false,
    "started_at": null,
    "ends_at": null,
    "current_period_days": null,
    "session_target": 20,
    "sessions_completed": 0,
    "legacy_base_allocations_read_count": 0
  },
  "config": {
    "default_start_time": "14:30",
    "default_period_days": 1,
    "override_start_time": null,
    "override_period_days": null,
    "auto_enabled": false
  },
  "updated_at": "<rfc3339>"
}
```

**11 個 `sac.*` slog events**：`sac.snapshot.start` / `sac.snapshot.target` / `sac.snapshot.current` / `sac.snapshot.fallback` / `sac.projection` / `sac.snapshot.end` / `sac.policy.applied` / `sac.policy.consumed` / `sac.legacy.read` / `sac.fallback.count` / `sac.rollback.drill`。

**Valid session 10 條件**：source session 為交易日期；`enabled.value=true` + `auto_observation=false`；target 恰 20 L1；sum∈[1±1e-9]；current 非 nil；`effective_from > as_of`；`applied`→ receipt 非空 + `changed_sectors_count>=0`；applied→ 下一 session 觸發 `sac.policy.consumed`；live mutation=0；synthetic F06 ranking=0。

**3 種 rollback drill**：
- Type A：翻 `enabled=false`、下 session 無 `sac.*` event
- Type B：重啟 + flag off + 重跑 → 退回 fallback 路徑
- Type C：翻 flag back + 重啟 + 重跑 → 重新計入 sessions_completed

**Step 1: Write RED**

```go
// internal/monitoring/api/pipeline/sector_allocation_closure_state_test.go
func TestSACClosureStateManager_StartIncrements(t *testing.T) { ... }
func TestSACClosureStateManager_AtomicWriteSurvivesCrash(t *testing.T) { ... }

// internal/orchestrator/sector_allocation_closure_metrics_test.go
func TestSACMetrics_EmitsAll11EventTypes(t *testing.T) { ... }

// internal/sectorallocation/legacy_counter_test.go
func TestLegacyCounter_IncPersistsByCaller(t *testing.T) { ... }
func TestLegacyCounter_SnapshotReturnsZeroAfterReset(t *testing.T) { ... }

// cmd/experimental/sector-allocation-closure-preflight/main_test.go
func TestPreflight_FailsWhenFlagOnByDefault(t *testing.T) { ... }
func TestPreflight_FailsWhenFuzzyMappingExists(t *testing.T) { ... }
func TestPreflight_PassesWhenAllAutoChecksGreen(t *testing.T) { ... }
```

**Step 2: Run RED**

```bash
go test ./internal/monitoring/api/pipeline ./internal/orchestrator ./internal/sectorallocation \
  ./cmd/experimental/sector-allocation-closure-preflight -count=1
```

**Step 3: 最小實作**

`internal/monitoring/api/pipeline/sector_allocation_closure_state.go`：clone `l2_4_state.go` 結構；`Start(target)` / `Stop()` / `IncrementSession()` / `Get()` / `ApplyOverride` / `Reset` / `SetConfig`；atomic write via `.tmp` + rename。

`internal/orchestrator/sector_allocation_closure_metrics.go`：定義 `SACMetrics` interface 與 11 個 `EmitXxx` 方法；`NewSlogMetrics()` 內以 `slog.Info` 與 `feature=sector_allocation_closure` `version=sa.0.1` `session_id=...` 格式輸出。

`internal/sectorallocation/legacy_counter.go`：
```go
type LegacyReadCounter struct { mu sync.RWMutex; reads map[string]int64; started time.Time }
func (l *LegacyReadCounter) Inc(caller string) { l.mu.Lock(); l.reads[caller]++; l.mu.Unlock() }
func (l *LegacyReadCounter) Snapshot() map[string]int64 { ... }
func (l *LegacyReadCounter) Reset() { ... }
```

`cmd/experimental/sector-allocation-closure-preflight/main.go`：clone `c07-preflight/main.go:55-82` 結構；5 auto + 3 manual checks；`validateLocalhostURL` 從 c07-preflight 複製（不抽 library）；exit codes 0/1/2；allow-list 加入 `scripts/ci/check_no_duplicate_preflight.sh`。

`internal/orchestrator/composition/root.go`：`BuildSystem` 內在 `enabled.value=true` 時把 `SectorAllocationClosure` 注入 `sectorAllocationSources`，否則傳 `nil` 讓 SA05 fallback。

`cmd/atlas/main.go`：新增 `sacMgr.SetConfig(...)`；slog 訊息 `"sector allocation closure enabled"` 只在 `enabled.value=true` + `auto_observation=true` 時輸出（避免啟用即誤判）。

**Step 4: Run GREEN**

```bash
go test ./internal/monitoring/api/pipeline ./internal/orchestrator ./internal/sectorallocation \
  ./cmd/experimental/sector-allocation-closure-preflight -count=1
go test ./... -count=1
go build ./... && gofmt -l . && golangci-lint run --timeout=5m
go generate . && git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

**Step 5: 觀察期 SOP（operator 執行）**

```bash
# 1. 確認前置 ID 為 implemented
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md

# 2. 跑 preflight（不翻 flag）
go run ./cmd/experimental/sector-allocation-closure-preflight http://localhost:18080
# 預期：5 automatable 全綠 + 3 manual 待 operator 確認

# 3. 翻 flag
#   configs/parameters.json: orchestrator.sector_allocation_closure.enabled.value: false → true
#   source 鎖死 "experimental"（SA11 不得改 empirical）
export SECTOR_ALLOCATION_CLOSURE_ENABLED=true
docker compose restart atlas

# 4. 確認 boot log（grep "sector allocation closure enabled"）

# 5. 啟動觀察期（admin endpoint POST /api/admin/sac-closure/start?target=20）

# 6. 跑滿 20 個 valid simulation sessions
#   自動 daily simulation 至少 14 天
#   不足時手動補 CLI / admin simulation
#   每日檢查 docs/operations/sector-allocation-closure-observation-log.md

# 7. 三種 rollback drill（Type A/B/C）記錄到 docs/operations/sector-allocation-closure-rollback-drills.md

# 8. 收 20 個後停止觀察期（admin endpoint POST /api/admin/sac-closure/stop）
```

**Step 6: Promotion**

manifest SA11 row `implemented → observing`（每日 1 commit append observation log）→ `observing → done`（3 drill 全 PASS、20 valid sessions、source 仍 `heuristic`/`calibrating`）。任何宣稱金融準確的 metric 都不算通過。

**Step 7: Commit boundaries**

```bash
# 1. infrastructure
git add internal/monitoring/api/pipeline/sector_allocation_closure_state.go \
  internal/monitoring/api/pipeline/sector_allocation_closure_state_test.go \
  internal/orchestrator/sector_allocation_closure_metrics.go \
  internal/orchestrator/sector_allocation_closure_metrics_test.go \
  internal/sectorallocation/legacy_counter.go internal/sectorallocation/legacy_counter_test.go \
  internal/config/parameters.go internal/config/config.go configs/parameters.json
git commit -m "feat(manifest): #SA11.A flag + state + metrics + legacy counter"

# 2. preflight + allow-list
git add cmd/experimental/sector-allocation-closure-preflight/main.go \
  cmd/experimental/sector-allocation-closure-preflight/main_test.go \
  scripts/ci/check_no_duplicate_preflight.sh docs/specs/experimental-feature-launch-gate.md
git commit -m "feat(manifest): #SA11.A preflight + CI allow-list"

# 3. runbook skeleton
git add docs/operations/sector-allocation-closure-runbook.md \
  docs/operations/sector-allocation-closure-observation-log.md \
  docs/operations/sector-allocation-closure-rollback-drills.md
git commit -m "docs(manifest): #SA11.A runbook skeleton + observation log stub"

# 4. implemented → observing
git add docs/manifests/sector-allocation-simulation-closure-manifest.md
git commit -m "chore(manifest): #SA11 implemented → observing"

# 5-7. drills + done
git add docs/operations/sector-allocation-closure-rollback-drills.md
git commit -m "chore(manifest): #SA11.C drill-A/B/C rollback drills"
git add docs/manifests/sector-allocation-simulation-closure-manifest.md
git commit -m "chore(manifest): #SA11 observing → done (source=heuristic 不變)"
```

---

## Task 12: SA12 — Close-out：dead config / negative evidence / verifier 擴充 / file sync

**Files:**
- Create: `scripts/ci/sa12-negative-evidence.sh`
- Modify: `internal/eventdriven/sector_predictor.go`（刪除 `_sectorWeights` 與 `sectorWeight` fallback）、`internal/portfolio/sector_rotator.go`（移除 `defaultMacroAdjustments` / `defaultFlowAdjustments` legacy mixed-key support，移除 `normalizeAllocations`）、`internal/config/parameters.go`、`internal/config/defaults_engine.go`、`configs/parameters.json`（移除 `sector_allocation.base_weights` 非 canonical 12 keys）、`internal/orchestrator/system.go`（`currentSectorAllocations()` 必須 non-nil；為 SA07 維護入口）、`internal/orchestrator/strategy_evolver.go`（保留 receipted 行為；不再 fake true）、`docs/manifests/sector-allocation-simulation-closure-manifest.md`（verifier Check 12-17）、`docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md`（F05 + BK-16 → done）、SA12.D runbook 與 verification report（將於 SA12.D 建立）、`docs/documentation-map.md`、`docs/reference/guidelines-index.md`

**Step 1: Write RED**

`scripts/ci/sa12-negative-evidence.sh`：
```bash
#!/usr/bin/env bash
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"
errors=0
report="${REPORT:-/tmp/sa12-negative-evidence.md}"
echo "# SA12 Negative Evidence Report" > "$report"
echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$report"
echo "" >> "$report"
check() {
    local name="$1" pattern="$2" path="$3" expected="$4"
    local count; count=$(grep -rnE "$pattern" "$path" 2>/dev/null | grep -v "_test.go" | grep -v "// safe:" | wc -l | tr -d ' ')
    if [[ "$count" != "$expected" ]]; then
        echo "❌ $name (count=$count, expected=$expected)" >&2
        grep -rnE "$pattern" "$path" 2>/dev/null | head -20 >> "$report"
        errors=$((errors + 1))
    else
        echo "✅ $name" >> "$report"
    fi
}
check "production legacy BaseAllocations caller" 'BaseAllocations' 'internal/portfolio/sector_rotator.go' 0
check "production duplicate strategic-prior map" '_sectorWeights' 'internal/eventdriven/sector_predictor.go' 0
check "currentSectorAllocations nil implementation" 'func.*currentSectorAllocations.*\) map\[string\]float64 \{$' 'internal/orchestrator/' 0
check "Applied=true without receipt" 'return true, fmt\.Sprintf.*Sector rotation applied' 'internal/orchestrator/strategy_evolver.go' 0
check "noncanonical final target key" '"industrial":|"industrials":' 'internal/' 0
check "live sector mutation path" 'live.*ApplySectorRotation|ApplySectorRotation.*live' 'internal/live/' 0
check "synthetic F06 ranking" 'IsSynthetic.*rank|rank.*IsSynthetic' 'internal/strategy/' 0
check "unversioned capital-flow action mapping" 'capital.flow.action.*[Vv]ersion|mapper.*[Vv]ersion' 'internal/capitalflow/' '>=1'
check "temporary compatibility adapter" 'TODO.*compat|deprecated.*compat|temp.*compat' 'internal/sectorallocation/ internal/portfolio/ internal/orchestrator/' 0
check "active docs direct-merge instruction" 'direct.merger|missing.key.merger|union.*map' 'docs/specs/sector-allocation-simulation-closure-spec.md' 0
check "sector_allocation.base_weights non-canonical" '"healthcare":|"industrials":|"materials":|"utilities":|"consumer":|"real_estate":|"_cash_reserve":' 'configs/parameters.json' 0
if (( errors == 0 )); then echo "OK: all negative evidence checks passed"; exit 0; else echo "FAIL: $errors negative evidence check(s) failed. See $report" >&2; exit 1; fi
```

擴充 `scripts/verify-sector-allocation-closure.sh` 加 Check 12-17（legacy read=0、retail F05/BK-16=done、doc map synced、source_label_still_heuristic）。

**Step 2: Run RED**

```bash
bash scripts/ci/sa12-negative-evidence.sh
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

預期：cleanup 之前各 check 仍 > 0。

**Step 3: 最小實作（cleanup）**

`internal/eventdriven/sector_predictor.go`：刪除 `_sectorWeights` 與 `sectorWeight`；unknown sector 顯式 error。
`internal/portfolio/sector_rotator.go`：刪除 `defaultMacroAdjustments`/`defaultFlowAdjustments`；移除 `normalizeAllocations`（`Projector` 是唯一 owner）。
`configs/parameters.json`：`sector_allocation.base_weights` 改為 20 L1 keys（刪除 `_cash_reserve`、`materials`、`industrials`、`utilities`、`real_estate`、`healthcare`、`consumer`；必要時合併到 FU-7 canonical；`tests` 同步）。
`internal/orchestrator/system.go`：`currentSectorAllocations()` 維護由 SA07 真實計算入口；不得回 nil。
`internal/orchestrator/strategy_evolver.go`：保留 `ApplySectorRotation` 回 `Applied` + receipt 與 `FallbackReason`；不得寫 fake true。

**Step 4: Run GREEN**

```bash
go test ./internal/eventdriven/... ./internal/portfolio/... ./internal/orchestrator/... -count=1
go test ./... -count=1 && go build ./... && gofmt -l . && golangci-lint run --timeout=5m
go generate . && git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/ci/sa12-negative-evidence.sh   # 必須 exit 0
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

**Step 5: File sync**

- `docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md` F05 改 `done`、BK-16 改 `done`、Change Log v1.10。
- `docs/operations/sector-allocation-closure-verification-report.md`（新檔，SA12.D 建立）：依觀察期證據填入 9 個 check 結果。
- `docs/operations/sector-allocation-closure-runbook.md`（SA12.D 補章節）。
- `docs/documentation-map.md`、`docs/reference/guidelines-index.md`：加入 SA11/SA12 entry。

**Step 6: Commit boundaries**

```bash
# 1. negative evidence
git add scripts/ci/sa12-negative-evidence.sh
git commit -m "feat(manifest): #SA12.A sa12-negative-evidence.sh + CI integration"

# 2-5. cleanup
git add internal/eventdriven/sector_predictor.go internal/portfolio/sector_rotator.go \
  internal/orchestrator/system.go internal/orchestrator/strategy_evolver.go \
  internal/config/parameters.go internal/config/defaults_engine.go \
  configs/parameters.json internal/eventdriven/sector_predictor_test.go \
  internal/portfolio/sector_rotator_test.go internal/orchestrator/system_test.go \
  internal/orchestrator/strategy_evolver_test.go internal/config/parameters_test.go
git commit -m "refactor(manifest): #SA12.B remove _sectorWeights + legacy mixed-key + canonical 20 L1"

# 6. verifier extension
git add scripts/verify-sector-allocation-closure.sh
git commit -m "feat(manifest): #SA12.C verifier Check 12-17 extension"

# 7. docs
git add docs/operations/sector-allocation-closure-runbook.md \
  docs/operations/sector-allocation-closure-verification-report.md（將於 SA12.D 建立） \
  docs/manifests/sector-allocation-simulation-closure-manifest.md \
  docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md \
  docs/documentation-map.md docs/reference/guidelines-index.md
git commit -m "docs(manifest): #SA12.D runbook + verification report + doc map sync"

# 8. F05/BK-16 sync
git add docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md
git commit -m "chore(manifest): #SA12.E retail-manifest F05 + BK-16 → done"

# 9. SA12 done
git add docs/manifests/sector-allocation-simulation-closure-manifest.md
git commit -m "chore(manifest): #SA12 closure verifier pass → done"
```

---

## Self-Review

1. **Spec coverage**
   - spec §1 完成契約 → 對應 `manifest §Completion Contract`。
   - spec §2 為何舊 F05 停止 → 對應 `manifest SA00 Notes`。
   - spec §3 namespace + authority → 對應 `Task 1` (SA01 typed namespaces)。
   - spec §4 typed model → 對應 `Task 1` `L1FinalTarget` / `ThemeExposure` / `Task 4` `Projector` / `Task 7` `SectorExposure`。
   - spec §5 融合 + projection → 對應 `Task 4` `Projector` 與唯一 normalize owner。
   - spec §6 macro / capital flow / C07 → 對應 `Task 4` typed enum + `Task 5` `CapitalFlowActionMapper`。
   - spec §7 composition + six-path → 對應 `Task 6` `CompositionPath.AllowsSectorRotation()`。
   - spec §8 current exposure + no-look-ahead + application truthfulness → 對應 `Task 7` `SectorExposure` + `Task 8` `FileClosureStore` + `TradingSessionResolver` + `SectorBudgetAllocator` + `simulation→live sync 隔離`。
   - spec §9 fallback reasons → `Task 8` 含 `effective_session_unavailable`。
   - spec §10 階段閘門 → `Task 11` SA11 + `Task 12` SA12。
   - spec §11 治理 → closure verifier 結構（`Task 1`）。
   - spec §12 工作分解 → 本 plan Task 1-12。
   - spec §13 文件同步 → `Task 11 Step 7` + `Task 12 Step 5`。

2. **Placeholder scan**
   - 全部步驟包含 exact files、完整 code skeleton、commands/expected；無 "TBD" / "TODO"。
   - 唯一待人類授權：SA10 是否在 commit 內完成 TAIEX dated snapshot populator（推薦另開 ID），其餘採推薦決策。

3. **Type consistency**
   - `StrategicSectorPrior` / `L1FinalTarget` / `ThemeExposure` / `ProjectedTarget` / `DriverInputs` / `Projector` / `MacroAction` / `CapitalFlowAction` / `CapitalFlowActionMapper` / `CompositionPath` / `SectorAllocationSnapshot` / `SectorAllocationPolicy` / `SimulationMutationReceipt` / `PolicyConsumptionReceipt` / `SimulationRollbackReceipt` / `StrategyDailyObservation` / `BenchmarkObservation` / `WarmingUpState` / `RankedStrategy` / `RankingSnapshot` / `ComparisonDay` / `ComparisonStore` / `ShadowStrategyEvaluator` / `TAIEXBenchmarkProvider` / `SymbolL1Mapper` / `SectorExposure` / `SectorExposureCalculator` / `SectorBudgetAllocator` / `TradingSessionResolver` / `ReplayNextSessionResolver` / `NoOpNextSessionResolver` / `LegacyCompatReader` / `LegacyReadCounter` / `SACMetrics` / `SACClosureStateManager` / `NamespaceKind` / `ClosureState` / `ClosureRuleResult` 全部跨 Task 命名一致。

4. **依賴順序**：SA01 verifier 必先於其他所有 ID；SA02-03 必先於 SA04；SA04 必先於 SA05-08；SA06 必先於 SA08；SA07 必先於 SA08；SA08 必先於 SA10（outcome attribution）；SA10 不得先於 SA08 進入 observation；SA11 必先於 SA12 done。`verify-sector-allocation-closure.sh` 內 Check 3 `phase_dependency_complete` 拒絕違規。

5. **不可自動驗證項**：SA10 觀察期真實覆蓋 20 個交易日、spot-check reasoning 合理性、CLI simulation 後 live store bytes 確實未變、文件地圖 cross-link 真的可達、heuristic source 未被任何 PR 偷改 empirical —— 皆由執行者手動確認並寫入 verification report。

---

## 不可自動驗證項

| # | 項目 | 為何不可自動驗證 | 確認方式 |
|---|------|----------------|---------|
| 1 | 觀察期真實覆蓋 20 個交易日 | 取決於 staging 環境與市場日曆 | observation log + `state.sessions_completed` |
| 2 | Spot-check 對 receipt 一致性 | 需人工判讀 sector 數字合理性 | observation log 每 session spot-check 區塊 |
| 3 | Rollback drill Type A「翻 flag 後下 session 無 sac.* event」 | 日誌可能因 buffer/async 延遲 | drill log + 5 分鐘後二次 grep |
| 4 | Legacy read counter 覆蓋所有路徑 | counter 是被動收集；新加 caller 若未 `Inc()` 會誤判 0 | `git log -p` 對比 sunset 前後所有 legacy path |
| 5 | 文件地圖 cross-link 真的可達 | 腳本只 grep 文字 | Playwright / 手動驗證 |
| 6 | `source` 標籤未被偷改 `empirical` | 文字紀律 | verifier Check 5/11/17 |
| 7 | promotion 後下游 consumer 未誤讀 `source=heuristic` | 需 grep 全 repo | `grep 'source=empirical'` |
| 8 | Observation 期從未宣稱金融準確 | 文字紀律 | observation log + verification report 開頭明示 |
| 9 | heuristic prior 未在無 empirical 證據下被自動 modification 啟用 | spec §4.1 + §10 Gate 3 鎖定 | spec + verifier |

---

## 失敗處理對照表

| 情境 | Exit / Action | 阻止 done？ |
|------|---------------|------------|
| Preflight 任一 auto check fail | exit 1；operator 修 | 是 |
| 觀察期 invariant violation | 立即 hard rollback | 是 |
| Valid session < 20 在 30 天窗口後 | `rolled_back` + blocker owner | 是 |
| Drill 任一 type FAIL | 修 bug 重跑；3 次 → architecture review | 是 |
| Legacy read counter > 0 after sunset | 繼續刪除直到 0 | 是 |
| Negative evidence > 0 | 逐項修復 | 是 |
| 觀察期未跑滿強行標 done | verifier 拒絕 | 是 |
| 文件地圖未同步 | verifier Check 8 fail | 是 |
| 原 F05 狀態未同步 | verifier Check 9 fail | 是 |
| `source=empirical` 被偷改 | verifier Check 5/11/17 fail | 是 |
| 同一 gate 三次失敗 | 停線 architecture review | — |

---

## Commit 序列總覽（共約 38 commits）

```
SA01 (1):  feat(manifest): #SA01 typed namespaces + closure verifier scaffold
SA02 (1):  feat(manifest): #SA02 20-L1 strategic prior with heuristic lock
SA03 (1):  feat(manifest): #SA03 split legacy BaseAllocations + compat reader
SA04 (1):  feat(manifest): #SA04 canonical 20-L1 WeightEngine with single projection
SA05 (1):  feat(manifest): #SA05 capital-flow anti-corruption with typed action enum
SA06 (1):  feat(manifest): #SA06 composition-root shared engine + six-path matrix
SA07 (1):  feat(manifest): #SA07 simulation-closing current 20-L1 exposure
SA08 (1):  feat(manifest): #SA08 next-session policy + allocator consumption + CLI live-sync isolation
SA09 (1):  feat(manifest): #SA09 unified REST/Web/MCP snapshot parity
SA10 (1):  feat(manifest): #SA10 isolated shadow real strategy ranking
SA11 (4):  infra → preflight → runbook → observing (3 drills + 20 sessions + done)
SA12 (5):  negative-evidence + cleanup × 2 + verifier ext + docs + F05 sync + done

每個 ID 都有專屬 evidence commit 更新 manifest 狀態。
```

---

## 執行交接

**Plan complete and saved to `docs/superpowers/plans/2026-07-18-sector-allocation-simulation-closure.md`.** 兩個執行選項：

1. **Subagent-Driven（推薦）** — 我每個 ID 派遣 fresh subagent，task 間 review、快速迭代。
2. **Inline Execution** — 在本 session 內依序執行 task，checkpoint 間人工檢視。

**唯一待人類授權**：SA10 是否在 commit 內完成 TAIEX dated snapshot populator（推薦另開 `#BK-XX dated TAIEX capture`，SA10 觀察期先全 `warming_up + benchmark_unavailable`）。
