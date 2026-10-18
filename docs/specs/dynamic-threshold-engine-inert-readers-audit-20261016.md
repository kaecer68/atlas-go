# `DynamicThresholdEngine` 讀取端全空：唯讀盤點與接線提案（2026-10-16）

> **任務**：`FU-20261016-36`（lane 唯讀調查子代理交付物）。
> **來源**：`FU-20261016-32` 的「已知限制 ②」——「`DynamicThresholdEngine` 讀取端全空 —— 比 `GetRegimeMultiplier` 大得多的缺口，需改生產門檻計算 ⇒ 未動」
> （`docs/operations/FOLLOWUPS.md:5811`）。本文件就是那條限制的逐條盤點。
> **基底**：`origin/main` = `45d2a6dbce191235e375111f4541ccd5b2cf396d`；lane branch `fix/20261016-positions-determinism-and-threshold-readers`。
> **性質**：**唯讀盤點 ＋ 只提案**。本車道未改任何 `.go`、未改 `docs/operations/FOLLOWUPS.md`、未 commit、未 push。
> **紅線遵守**：所有指令皆為 `grep` / `git log` / `git show` / 讀檔；無任何 git 寫入操作。

---

## 0. 機械式複現（逐條指令、逐字輸出、rc）

### 0.1 判定式（先寫清楚，否則「0」沒有意義）

> **生產呼叫者 = 0** 的判定式：
> 對符號 `S`，取 `grep -rn 'S(' --include=*.go .` 的全部命中，逐行分成三類：
> ① **定義行**（該行以 `func ` 開頭且函式名為 `S`）② **測試行**（檔名以 `_test.go` 結尾）
> ③ **其餘**。判 `S` 的生產呼叫者 = `|③|`，**但**若 `③` 的命中屬於**另一個型別的同名方法**，則須剔除（見 §0.3 命名碰撞）。
> 判定為「生產呼叫者 = 0」的條件：**剔碰撞後 `③` 為空集合**。
>
> 本判準與 `FU-20261016-32`（`docs/specs/exported-risk-symbols-audit-20261016.md` §1.1）一致，差別在於本條以**單一型別**為母體，而非全 repo 掃描。

### 0.2 逐條指令與輸出

全部在 `/Users/kaecer/workspace/atlas/.worktrees/fu35-36-positions-threshold` 下執行，`rc` 皆為 `0`。

**（1）`GetThreshold` 的呼叫者**
```console
$ grep -rn 'GetThreshold(' --include=*.go .
./internal/sim/dynamic_threshold.go:60:func (e *DynamicThresholdEngine) GetThreshold(vix float64, regime RegimeType) float64 {
./internal/sim/dynamic_threshold_test.go:32:func TestDynamicThresholdEngine_GetThreshold(t *testing.T) {
./internal/sim/dynamic_threshold_test.go:49:		got := e.GetThreshold(tt.vix, tt.regime)
./internal/sim/dynamic_threshold_test.go:62:	got := e.GetThreshold(100, RegimeNeutral)
./internal/sim/dynamic_threshold_test.go:73:	got := e.GetThreshold(0, RegimeBull)
./internal/sim/dynamic_threshold_test.go:111:	e.GetThreshold(25, RegimeBull)
./internal/sim/dynamic_threshold_test.go:195:	conservativeThreshold := e.GetThreshold(20, RegimeNeutral)
./internal/sim/dynamic_threshold_test.go:199:	balancedThreshold := e.GetThreshold(20, RegimeNeutral)
./internal/sim/dynamic_threshold_test.go:203:	aggressiveThreshold := e.GetThreshold(20, RegimeNeutral)
./internal/sim/dynamic_threshold_test.go:215:	e.GetThreshold(25, RegimeBull)
./internal/sim/dynamic_threshold_test.go:238:		e.GetThreshold(20, RegimeNeutral)
```
⇒ 1 定義行 ＋ 10 測試行 ＋ **0 生產行**。**生產呼叫者 = 0**（與 parent 敘述一致）。

**（2）`GetCurrentThreshold`**
```console
$ grep -rn 'GetCurrentThreshold(' --include=*.go .
./internal/sim/dynamic_threshold.go:84:func (e *DynamicThresholdEngine) GetCurrentThreshold() float64 {
./internal/sim/dynamic_threshold_test.go:108:func TestDynamicThresholdEngine_GetCurrentThreshold(t *testing.T) {
./internal/sim/dynamic_threshold_test.go:112:	got := e.GetCurrentThreshold()
./internal/sim/dynamic_threshold_test.go:119:		t.Errorf("GetCurrentThreshold() = %.4f, want %.4f", got, expected)
```
⇒ **0 生產行**（`:119` 是錯誤訊息字串，與 parent 敘述一致）。**生產呼叫者 = 0**。

**（3）`GetStats`（此型別）**
```console
$ grep -rn 'GetStats()' --include=*.go .
./internal/portfolio/style.go:289:func (s *StyleRotationStrategy) GetStats() RotationStats {
./internal/portfolio/style_test.go:54:	stats := s.GetStats()
./internal/sim/dynamic_threshold.go:170:func (e *DynamicThresholdEngine) GetStats() map[string]any {
./internal/sim/dynamic_threshold_test.go:217:	stats := e.GetStats()
./internal/sim/dynamic_threshold_test.go:239:		stats := e.GetStats()
```
⇒ `internal/portfolio/style.go:289` 是**另一個型別**（`StyleRotationStrategy`）。剔除碰撞後：1 定義行 ＋ 2 測試行 ＋ **0 生產行**。
**生產呼叫者 = 0**（與 parent 敘述一致）。

**（4）`calculateThresholdUnsafe`**
```console
$ grep -rn 'calculateThresholdUnsafe' --include=*.go .
./internal/sim/dynamic_threshold.go:87:	return e.calculateThresholdUnsafe(e.lastVIX, e.currentRegime)
./internal/sim/dynamic_threshold.go:90:func (e *DynamicThresholdEngine) calculateThresholdUnsafe(vix float64, regime RegimeType) float64 {
./internal/sim/dynamic_threshold.go:123:// GetThreshold / calculateThresholdUnsafe (`regimeAdjustment :=
./internal/sim/dynamic_threshold.go:179:		"current_threshold": e.calculateThresholdUnsafe(e.lastVIX, e.currentRegime),
```
⇒ 只被同檔 `:87`（`GetThreshold` 內）與 `:179`（`GetStats` 內）呼叫（與 parent 敘述一致）。
**它的呼叫者本身也是零生產引用** ⇒ 這條內部鏈整條不在生產上跑。

**（5）`thresholdEngine`（`internal/sim` 內）**
```console
$ grep -rn 'thresholdEngine' --include=*.go .
./internal/sim/engine.go:44:	thresholdEngine           *DynamicThresholdEngine
./internal/sim/engine.go:124:	e.thresholdEngine = te
./internal/sim/engine.go:328:	return e.thresholdEngine
./internal/orchestrator/system_dispatcher.go:121:		if topStrategy != nil && s.strat.thresholdEngine != nil {
./internal/orchestrator/system_dispatcher.go:122:			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
./internal/orchestrator/system_dispatcher.go:134:			if s.strat.thresholdEngine != nil {
./internal/orchestrator/system_dispatcher.go:135:				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
./internal/orchestrator/system.go:85:	thresholdEngine   *sim.DynamicThresholdEngine
./internal/orchestrator/system.go:418:	thresholdEngine := sim.NewDynamicThresholdEngine()
./internal/orchestrator/system.go:436:			strat:           buildStrategyLayer(cfg.LedgerDir, thresholdEngine),
./internal/orchestrator/system.go:818:		if topStrategy != nil && s.strat.thresholdEngine != nil {
./internal/orchestrator/system.go:819:			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
./internal/orchestrator/system.go:831:			if s.strat.thresholdEngine != nil {
./internal/orchestrator/system.go:832:				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
./internal/orchestrator/system_plugin_accessors.go:63:	return s.strat.thresholdEngine
./internal/orchestrator/system_coverage_test.go:43:				thresholdEngine:   sim.NewDynamicThresholdEngine(),
./internal/orchestrator/composition.go:295:func buildStrategyLayer(ledgerDir string, thresholdEngine *sim.DynamicThresholdEngine) StrategyLayer {
./internal/orchestrator/composition.go:306:		thresholdEngine:   thresholdEngine,
```
⇒ `internal/sim/engine.go` 內**只有宣告 `:44`、賦值 `:124`、getter `:328`**，沒有任何讀取用法（與 parent 敘述一致）。
`internal/orchestrator` 的 4 個命中（`system.go:819,832`、`system_dispatcher.go:122,135`）**全部是 `SetRiskAppetite`**（寫入端）。

**（6）`GetThresholdEngine()`**
```console
$ grep -rn 'GetThresholdEngine()' --include=*.go .
./internal/sim/engine.go:327:func (e *Engine) GetThresholdEngine() *DynamicThresholdEngine {
./internal/orchestrator/system_plugin_accessors.go:62:func (s *System) GetThresholdEngine() *sim.DynamicThresholdEngine {
./internal/orchestrator/system_coverage_test.go:75:	if got := sys.GetThresholdEngine(); got == nil {
./internal/orchestrator/system_coverage_test.go:76:		t.Error("GetThresholdEngine() returned nil")
```
```console
$ grep -rn '\.GetThresholdEngine()' --include=*.go .
./internal/orchestrator/system_coverage_test.go:75:	if got := sys.GetThresholdEngine(); got == nil {
```
⇒ 兩個 getter（`sim.Engine:327`、`orchestrator.System:62`）**各有 0 個生產呼叫者**。
（parent 只點名了 `orchestrator.System` 那個；`sim.Engine.GetThresholdEngine` **同樣是 0 生產呼叫者**——見 §0.3 更正 ③。）

**（7）生產的寫入端：`SetRiskAppetite`**
```console
$ grep -rn 'SetRiskAppetite(' --include=*.go .
./internal/sim/dynamic_threshold.go:147:func (e *DynamicThresholdEngine) SetRiskAppetite(ra RiskAppetite) {
./internal/sim/dynamic_threshold_test.go:182:		e.SetRiskAppetite(ra)
./internal/sim/dynamic_threshold_test.go:194:	e.SetRiskAppetite(RiskAppetiteConservative)
./internal/sim/dynamic_threshold_test.go:198:	e.SetRiskAppetite(RiskAppetiteBalanced)
./internal/sim/dynamic_threshold_test.go:202:	e.SetRiskAppetite(RiskAppetiteAggressive)
./internal/orchestrator/system_dispatcher.go:122:			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
./internal/orchestrator/system_dispatcher.go:135:				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
./internal/orchestrator/system.go:819:			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
./internal/orchestrator/system.go:832:				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
```
⇒ 4 個生產寫入端（與 parent 敘述一致）。

**（8）建構與注入**
```console
$ grep -rn 'NewDynamicThresholdEngine' --include=*.go .
...（14 測試行略）
./internal/orchestrator/system.go:418:	thresholdEngine := sim.NewDynamicThresholdEngine()
./internal/orchestrator/composition.go:76:		WithThresholdEngine(sim.NewDynamicThresholdEngine()).

$ grep -rn 'WithThresholdEngine' --include=*.go .
./internal/sim/engine.go:122:// WithThresholdEngine attaches a dynamic threshold engine for correlation-based filtering.
./internal/sim/engine.go:123:func (e *Engine) WithThresholdEngine(te *DynamicThresholdEngine) *Engine {
./internal/orchestrator/composition.go:76:		WithThresholdEngine(sim.NewDynamicThresholdEngine()).
```
⇒ 生產**有**兩個建構點、一個注入點（與 parent 敘述一致）。

**（9）其餘 exported 讀取端（parent 未列，本次補齊）**
```console
$ grep -rn 'GetRegimeMultiplier(' --include=*.go .
./internal/sim/dynamic_threshold.go:135:func (e *DynamicThresholdEngine) GetRegimeMultiplier(regime RegimeType) float64 {
./internal/sim/dynamic_threshold_test.go:142:func TestDynamicThresholdEngine_GetRegimeMultiplier(t *testing.T) {
./internal/sim/dynamic_threshold_test.go:156:		got := e.GetRegimeMultiplier(tt.regime)
./internal/sim/dynamic_threshold_test.go:158:			t.Errorf("GetRegimeMultiplier(%s) = %.2f, want %.2f", tt.regime, got, tt.want)

$ grep -rn 'GetRiskAppetite()' --include=*.go .
./internal/sim/dynamic_threshold.go:153:func (e *DynamicThresholdEngine) GetRiskAppetite() RiskAppetite {
./internal/sim/dynamic_threshold_test.go:183:		got := e.GetRiskAppetite()

$ grep -rn 'SetBaseThreshold(' --include=*.go .
./internal/sim/dynamic_threshold.go:141:func (e *DynamicThresholdEngine) SetBaseThreshold(threshold float64) {
./internal/sim/dynamic_threshold_test.go:163:func TestDynamicThresholdEngine_SetBaseThreshold(t *testing.T) {
./internal/sim/dynamic_threshold_test.go:165:	e.SetBaseThreshold(0.60)

$ grep -rn 'RegimeFromDomain(' --include=*.go .
./internal/sim/dynamic_threshold.go:184:func RegimeFromDomain(r domain.Regime) RegimeType {
./internal/sim/dynamic_threshold_test.go:123:func TestRegimeFromDomain(t *testing.T) {
./internal/sim/dynamic_threshold_test.go:135:		got := RegimeFromDomain(tt.regime)
./internal/sim/dynamic_threshold_test.go:137:			t.Errorf("RegimeFromDomain(%s) = %s, want %s", tt.regime, got, tt.want)
```
⇒ 三個讀取端 ＋ 一個轉換函式，**皆 0 生產呼叫者**。

### 0.3 判定表（本型別的完整公開面）

以 `/tmp/fu35-36-repro.py`（本車道自製、唯讀）對每個符號做「定義 / 測試 / 生產」三分類，輸出：

```console
$ python3 /tmp/fu35-36-repro.py /Users/kaecer/workspace/atlas/.worktrees/fu35-36-positions-threshold
symbol                       defs  test  prod  verdict
GetThreshold                    1    10     0  PROD CALLERS = 0
GetCurrentThreshold             1     3     0  PROD CALLERS = 0
GetStats                        3    8     1  prod callers: 1     ← 命名碰撞，剔除後 0
calculateThresholdUnsafe        1     0     2  prod callers: 2     ← 同檔自用（:87/:179）
DetectRegime                    2    5     2  prod callers: 2     ← 屬另一型別，剔除後 0
GetRegimeMultiplier             1     3     0  PROD CALLERS = 0
GetRiskAppetite                 1     2     0  PROD CALLERS = 0
SetBaseThreshold                1     2     0  PROD CALLERS = 0
SetRiskAppetite                 1     4     4  prod callers: 4     ← 唯一生產寫入端
RegimeFromDomain                1     3     0  PROD CALLERS = 0
NewDynamicThresholdEngine       1    14     2  prod callers: 2     ← 建構（非讀取）
WithThresholdEngine             1     0     1  prod callers: 1     ← 注入（非讀取）
GetThresholdEngine              2     2     0  PROD CALLERS = 0     ← 兩個 getter 皆 0
```

碰撞剔除的逐字證據：
```console
$ grep -rn '\.GetStats()' --include=*.go .
./internal/portfolio/style_test.go:54:	stats := s.GetStats()
./internal/sim/dynamic_threshold_test.go:217:	stats := e.GetStats()
./internal/sim/dynamic_threshold_test.go:239:		stats := e.GetStats()

$ grep -rn '\.DetectRegime(' --include=*.go .
./internal/realtime/realtime_test.go:287:		regime := detector.DetectRegime(data)
./internal/realtime/realtime_test.go:316:		regime := detector.DetectRegime(data)
./internal/realtime/regime_adapter.go:321:		newRegime := rta.detector.DetectRegime(window)
./internal/sim/dynamic_threshold_test.go:100:		got := e.DetectRegime(tt.indicators)
```
`internal/realtime/regime_adapter.go:321` 的 `rta.detector` 是 `internal/realtime` 的 `RegimeDetector`（定義在 `regime_adapter.go:75`），**不是** `sim.DynamicThresholdEngine`。

### 0.4 對 parent 敘述的更正（3 處）

| # | parent 敘述 | 實測結果 | 判定 |
|---|---|---|---|
| ① | 「`GetThreshold(` 的呼叫者：全 repo **只有** `internal/sim/dynamic_threshold_test.go`（10 處）」 | 成立（10 處測試呼叫 ＋ 1 定義行） | **成立，不更正** |
| ② | 「`GetStats()`（此型別）：只有 `internal/sim/dynamic_threshold_test.go:217,239`」 | 成立；但全 repo 另有一個**同名不同型別**的 `GetStats`（`internal/portfolio/style.go:289`） | **成立，補註碰撞** |
| ③ | 「`internal/sim/engine.go` 內部**也沒有**用 `e.thresholdEngine` 做任何事（只有 :44 宣告、:124 賦值、:328 getter）」 | 成立；**且**該 getter（`engine.go:327 GetThresholdEngine`）本身也**0 生產呼叫者**（parent 只點名了 `orchestrator` 那個） | **成立，擴大** |
| ④ | 「`GetCurrentThreshold(`：只有 `internal/sim/dynamic_threshold_test.go:112`（＋`:119` 的錯誤訊息字串）」 | 成立 | **成立，不更正** |
| ⑤ | 「`calculateThresholdUnsafe` 只被 `GetThreshold`（`:87`）與 `GetStats`（`:179`）呼叫」 | 成立 | **成立，不更正** |
| ⑥ | 「生產唯一對它的動作是 `SetRiskAppetite`」 | 成立（4 處），但**另有 `DetectRegime` / `RegimeFromDomain` / `SetBaseThreshold` / `GetRiskAppetite` 也全零** ⇒ 「唯一動作」是對的，但「全空」的範圍比 parent 列的更大 | **成立，擴大** |
| ⑦ | （parent 未提）兩個建構點是否同一個物件 | **不是**：`system.go:418` 的實例進 `StrategyLayer`（收 `SetRiskAppetite`），`composition.go:76` 的**另一個**實例進 `sim.Engine` | **新事實（見 §1.4）** |

---

## 1. 引擎實際算了什麼

### 1.1 定義面（`internal/sim/dynamic_threshold.go`）

| 成員 | 行 | 語意 |
|---|---|---|
| `NewDynamicThresholdEngine` | `:43` | 預設 `baseThreshold=0.70`、`minThreshold=0.40`、`maxThreshold=0.85`、`currentRegime=RegimeNeutral`、`lastVIX=20.0`、`riskAppetite=RiskAppetiteBalanced` |
| `GetThreshold(vix, regime)` | `:60` | **寫入 ＋ 計算**：先 `e.lastVIX=vix`、`e.currentRegime=regime`、`e.updateCount++`，再回傳門檻 |
| `GetCurrentThreshold()` | `:84` | 純讀取：用**已存的** `lastVIX`/`currentRegime` 重算 |
| `calculateThresholdUnsafe` | `:90` | 內部算式（無鎖） |
| `DetectRegime(indicators)` | `:106` | `VIX>30`⇒highvol；`VIX>25 && SPXTrend<0`⇒bear；`VIX<15 && SPXTrend>0`⇒bull；其餘 neutral |
| `GetRegimeMultiplier` | `:135` | 讀 `regimeMultipliers[regime]` |
| `SetBaseThreshold` | `:141` | 寫 `baseThreshold` |
| `SetRiskAppetite` | `:147` | 寫 `riskAppetite` |
| `GetRiskAppetite` | `:153` | 讀 `riskAppetite` |
| `getAppetiteAdjustment` | `:159` | conservative `+0.10`、aggressive `-0.05`、其餘 `0.00` |
| `GetStats` | `:170` | 回 7 個鍵的 map（含 `current_threshold`） |
| `RegimeFromDomain` | `:184` | `domain.Regime` → `RegimeType`（risk_on⇒bull、risk_off⇒bear、其餘 neutral） |

**算式（`dynamic_threshold.go:68-79`，與 `:91-101` 逐字相同）**：
```
vixAdjustment     = (vix - 20.0) / 100.0
regimeAdjustment  = regimeMultipliers[regime]      // bull -0.05 / bear +0.10 / neutral 0.00 / highvol +0.15
appetiteAdjustment= getAppetiteAdjustment()        // conservative +0.10 / balanced 0.00 / aggressive -0.05
threshold         = clamp(baseThreshold + vixAdjustment + regimeAdjustment + appetiteAdjustment, 0.40, 0.85)
```
⇒ 輸出值域固定落在 **[0.40, 0.85]**，單位是**無因次的 0–1 分數**（不是 0–100 的信心值）。

**方向語意**：VIX 越高 ⇒ 門檻越高（更嚴）；bear/highvol ⇒ 更高；conservative ⇒ 更高；aggressive ⇒ 更低。
即「市場越危險、胃口越保守 ⇒ 進場門檻越高」。

### 1.2 生產實際餵進去的只有一樣東西

| 狀態 | 生產是否被寫入 | 證據 |
|---|---|---|
| `riskAppetite` | **是** | `SetRiskAppetite` 4 處生產呼叫（§0.2(7)） |
| `lastVIX` | **否**（恆 `20.0`） | `GetThreshold` 是唯一寫入者，而它 0 生產呼叫者 |
| `currentRegime` | **否**（恆 `RegimeNeutral`） | 同上；`DetectRegime` 亦 0 生產呼叫者 |
| `updateCount` | **否**（恆 `0`） | 同上 |
| `baseThreshold` | **否**（恆 `0.70`） | `SetBaseThreshold` 0 生產呼叫者 |

⇒ 就算今天有生產端去讀 `GetCurrentThreshold()`，它也只會回
`clamp(0.70 + 0 + 0 + appetiteAdjustment, 0.40, 0.85)`，即
conservative `0.80`、balanced `0.70`、aggressive `0.65`。**「動態」的那兩個維度（VIX、regime）在生產上永遠是預設值。**

### 1.3 誰在生產餵它（`SetRiskAppetite` 的來源鏈）

```
strategy.Strategy.RiskAppetite            （靜態字面值，internal/strategy/registry.go:32,42,52,62,72）
        │
        ├─ StrategyAllocator.Allocate(regime, vix) → mix → 取最大權重者 topStrategy
        │     internal/orchestrator/system.go:804 / system_dispatcher.go:107
        │
        └─ 或 Selector.Select(ctx, vix, regime) → selectedStrategy
              internal/orchestrator/system.go:822 / system_dispatcher.go:125
        │
        ▼
SetRiskAppetite(sim.RiskAppetite(x.RiskAppetite))
        system.go:819,832 / system_dispatcher.go:122,135
```

`internal/strategy/registry.go` 的 5 個預設策略與其 `RiskAppetite`：

| 策略 | `RiskAppetite` | 行 |
|---|---|---|
| `all_weather` 全天候 | `RiskAppetiteBalanced` | `:32` |
| `growth` 成長動能 | `RiskAppetiteAggressive` | `:42` |
| `value` 價值投資 | `RiskAppetiteConservative` | `:52` |
| `defensive` 防御型 | `RiskAppetiteConservative` | `:62` |
| `momentum` 純動能 | `RiskAppetiteAggressive` | `:72` |

⇒ 生產餵入的是一個**靜態枚舉**（1/2/3），不是量測值。`Regime` 的資訊只用在「選哪個策略」，不進引擎。

**同一份 `RiskAppetite` 另有真正的生產消費者**（與本引擎無關）：
`internal/portfolio/factor_weight_engine.go:457 strategyDeltas(ra)`（`:479`/`:485` 分支），
經 `ApplyStrategyMix` / `ApplyStrategy` 生效（`system.go:806,829`、`system_dispatcher.go:109,132`）。
⇒ 「胃口」這個輸入在生產**是活的**，只是它沒有走到 `DynamicThresholdEngine`。

### 1.4 ★ 新事實：生產上有**兩個**互不相干的 `DynamicThresholdEngine`

| 實例 | 建構點 | 進到哪 | 生產對它做什麼 |
|---|---|---|---|
| **A** | `internal/orchestrator/composition.go:76`（`buildSimEngine`，經 `buildSimulationCore` `composition.go:33`） | `sim.Engine.thresholdEngine`（`internal/sim/engine.go:44,124`） | **什麼都沒有**（引擎內部不讀，外部 getter 也 0 呼叫者） |
| **B** | `internal/orchestrator/system.go:418` | `StrategyLayer.thresholdEngine`（`system.go:436` → `composition.go:295,306`） | 只收 `SetRiskAppetite`（4 處） |

⇒ `SetRiskAppetite` 寫的是 **B**；`WithThresholdEngine` 掛在 sim 引擎上的是 **A**。
**兩者不是同一個物件**（`composition.go:76` 各自 `NewDynamicThresholdEngine()`）。
所以「把 `sim.Engine.GetThresholdEngine()` 接出來讀」這個直覺解法，讀到的會是**從未被寫入過的 A**，不是策略層剛設定的 B。
這個事實是 §4 選項設計的關鍵約束。

---

## 2. 誰「應該」讀它——候選消費者清單（現況，非提案）

方法：以 `grep -n` ＋ 讀碼找出**現在**決定「進場/篩選門檻」的地方。
「LIVE」＝該門檻在生產路徑上真的會被評估；「INERT」＝讀取端本身 0 生產呼叫者（接上去也不會生效）。

| # | 候選 | 檔:行 | 現行門檻來源（逐字） | LIVE? |
|---|---|---|---|---|
| **C1** | CRO 控制層的信心門檻 | `internal/orchestrator/plugin_control.go:41`（`floor := effectiveConvictionFloor(policy, regime)`）；算式在 `internal/orchestrator/executor_control.go:329`；被 `executor_control.go:104` 的 `plugins.ApplyControl` 呼叫 | `policy.ConvictionFloor`，未設則 `config.GetParametersConfig().Orchestrator.ConvictionFloorDefault.Value`＝**60**（`internal/config/defaults_engine.go:146`）；**`regime == domain.RegimeRiskOff && floor < 70` ⇒ `floor = 70`**（`executor_control.go:334-336`） | **LIVE**（`ExecuteWithContext`：`executor_pipeline.go:17`，由 `system.go:766`／`system_dispatcher.go:70`／`live_execution_input.go:59` 呼叫） |
| **C2** | 模擬引擎買入門檻 | `internal/sim/engine.go:909`（`executeLegacyBuys`）；同值另用於 `:801`（`executeOptimizerBuys` 覆蓋率統計）；算式 `engine.go:198` | `e.constraints.MinRecommendationConviction`（`internal/domain/types.go:30`；預設 **60**，`internal/config/defaults_engine.go:442`）＋ CharterMode 期間加成（`engine.go:186` `WithConvictionFloorAdjustment`，生產接線 `system.go:694,698`；RISK_OFF +10／black_swan +20） | **LIVE**（`system.go:853,855`／`system_dispatcher.go:156,158` 的 `RunWithState`/`Run`） |
| **C3** | 盤前風控閘門的**相關性上限** | `internal/risk/pre_trade.go:23`（欄位）`:38`（載入）`:47`（`MaxCorrelation()` getter） | `cfg.RiskGate.PreTrade.MaxCorrelation.Value`＝**0.70**（`internal/config/defaults_engine.go:643-647`，理由逐字：「與現有持倉相關性 > 0.7 則降低權重」） | **INERT**：`PreTradeGate.Check`（`pre_trade.go:66`）只跑 6 條規則（`ruleMaxPosition:140`／`ruleSectorExposure:166`／`ruleVaRLimit:182`／`ruleCashBuffer:217`／`ruleRetailSentiment:230`／`ruleMaxOpenPositions:263`），**沒有任何 correlation 規則**；`MaxCorrelation()` 全 repo 唯一引用是 `pre_trade_test.go:285` |
| **C4** | 部位管理器（Sizer）的相關性門檻 | `internal/portfolio/sizing.go:284`（`if absCorr > runtimeParams.Sizing.CorrelationThreshold`） | `Sizing.CorrelationThreshold`＝**0.70**（`internal/config/defaults_portfolio.go:342-346`） | **INERT（整個型別）**：`NewSizerFromConfig`（`sizing.go:72`）0 呼叫者、`NewSizer`（`:57`）只被它呼叫、`CalculateSize`（`:111`）只被同檔 `:402` 呼叫、`SetCorrelationThreshold`（`:104`）0 呼叫者；`Sizer` 在 `internal/`＋`cmd/` 的非測試檔中**沒有任何引用** |
| **C5** | 最佳化器的危機／跨市場相關性 | `internal/portfolio/optimizer_pipeline.go:233-248`（crisis：對角線 ×1.5、`wMax/2`）、`:250-266`（`crossRho > 0.5` ⇒ 放大非對角） | 寫入端 `Optimizer.SetCrisisMode`（`optimizer.go:212`）、`SetCrossMarketCorrelation`（`optimizer.go:224`） | **INERT**：兩個 setter 的生產註冊點是 `internal/monitoring/dashboard_api.go:2114`（`SetCrisisModeSetter`）與 `:2127`（`SetCorrelationSetter`），而**這兩個註冊函式在 `cmd/` 與 `internal/` 非測試檔皆 0 呼叫者** ⇒ `crisisMode` 恆 false、`crossMarketRho` 恆 0.5。**但 `Optimizer` 本身 LIVE**（`system.go:416-417` → `composition.go:33,75`） |
| **C6** | Alpha 探索的因子分數門檻 | `internal/orchestrator/alpha_discovery.go:22`（`factorThreshold: 0.4`）、`:60`（`if score < e.factorThreshold`） | 硬編碼 **0.4**；`SetFactorThreshold`（`:27`）0 生產呼叫者 | **LIVE**（`system_dispatcher.go:144` `applyAlphaDiscovery`；建構 `composition.go:289`） |
| **C7** | 擁擠度／反相關後處理 | `internal/orchestrator/executor_control.go:185`（`applyCrowdingPenalty`）、`:186`（`applyAntiCorrelationLayer`） | `config.GetParametersConfig().Engine.Executors`（`:214`、`:259`）：`CrowdingPenaltyAgents3/4`、`MinTradeAmount` | **LIVE**（在 `ApplyControl` 之後、同一條控制層管線上執行） |

**單位相容性備註**（決定「哪個候選接得上」的關鍵）：
`DynamicThresholdEngine` 輸出 ∈ `[0.40, 0.85]`（0–1 分數）。
**只有 C3／C4／C5 是 0–1 量綱**（且三者預設值都是 **0.70**，與 `baseThreshold` 逐字相同）。
C1／C2 是 **0–100 的信心值**，C6 是 0–1 但語意是「多因子分數」。

---

## 3. 讀了會影響什麼（逐候選）

| # | 若接線，生產門檻值會怎麼變 | 風險 | 是否實盤／選股路徑 |
|---|---|---|---|
| **C1** | 需把 0–1 換算成 0–100（×100 ⇒ 40–85）才能取代 `effectiveConvictionFloor`。RISK_OFF 現行固定 70，而引擎在 `RegimeHighVol` 給 `0.70+0.15=0.85`（⇒85）、`RegimeBear` 給 80 ⇒ **比現行嚴**。 | **高**：這是所有推薦的總閘門；門檻上移會直接減少進場標的數。且 C1 已有一個**獨立且已校準**的 regime 邏輯（RISK_OFF ⇒ 70），接線等於**兩套 regime 規則疊加**，可能互相打架。 | **是**（`ExecuteWithContext` 同時服務模擬與實盤輸入 `live_execution_input.go:59`） |
| **C2** | 同上換算；`executeLegacyBuys` 的門檻由 60 變成 65–85 之間（依策略胃口）。 | **中高**：會改**選股結果**（哪些標的被買）與回測績效，屬實盤/績效語意。 | **是**（模擬引擎是每日模擬與回測的執行層，`internal/backtest/window.go:73` 也走 `RunDailySimulation`） |
| **C3** | `MaxCorrelation` 目前**完全沒被讀**。若先補上「相關性 > 上限 ⇒ 降權或擋單」的規則、再把 0.70 動態化，門檻會隨 VIX/regime 在 0.40–0.85 間移動。 | **高，且前置條件未滿足**：`PreTradeGate` 目前**沒有**任何相關性資料（`PortfolioState` 只有 `SectorExposure`/`Positions`/`Var95`，`internal/risk/pre_trade.go:66` 的 `Check` 參數），要接線得先建資料面。 | **是**（`PreTradeGate` 在模擬引擎 `composition.go:78`／`factory.go:85` 與實盤 `cmd/atlas/main.go:2452` 都掛著） |
| **C4** | `Sizing.CorrelationThreshold` 目前**完全沒被讀**（整個 `Sizer` 無生產消費者）。接上只會改一個沒人用的物件。 | **低（因為沒有效果）** ⇒ 沒有價值，除非同時把 `Sizer` 接進生產（那是另一個決策）。 | 否（目前） |
| **C5** | `crisisMode`／`crossRho` 目前恆為預設值。接線會讓最佳化器在 VIX 高時把 `wMax` 減半、放大非對角共變異數 ⇒ 權重更分散。 | **中**：改的是**配置權重**（選股之後、下單之前），會改實際持倉比例。且它與 `DynamicThresholdEngine` 語意重疊（都是「VIX 高 ⇒ 更保守」）⇒ 接線前應先決定**要留哪一套**。 | **是**（`Optimizer` 在模擬引擎的 `executeOptimizerBuys` 路徑上） |
| **C6** | 門檻由 0.4 變成 0.65–0.85 ⇒ Alpha 探索會**幾乎不再觸發**（`:60` 是 `score < threshold` 就跳過）。 | **中高**：會減少新增標的，但方向是「更嚴」，且該值目前是硬編碼未校準值。 | **是**（選股路徑） |
| **C7** | 目前無門檻語意（懲罰/去重），接線需重新定義語意，不是換一個數字。 | **低（純結構）** | 是 |

**跨候選的共同風險**：`DynamicThresholdEngine` 的 VIX/regime 輸入**在生產從未被寫入**（§1.2）。
任何接線都必須**同時**補上「誰把 VIX 與 regime 餵進 `GetThreshold`」——否則接上去只會得到一個固定值（依策略胃口 0.65/0.70/0.80），
那與現行的靜態常數**沒有語意差別**，卻多了「看起來很動態」的假象。

---

## 4. 建議接線點（**只提案，未實作**）

> ⚠️ 以下三個選項**全部未實作**。任何一個都會改動生產門檻語意，依 `FU-20261016-32` 的先例屬「涉生產風控行為 ⇒ 只提案」。

### 選項 A（**最低風險**）：只把 `GetStats()` 接到觀測面，零行為變更

- **位置**：`internal/sim/engine.go`（新增一個 getter 呼叫點）→ `internal/orchestrator/system.go` 的 trace/dashboard 輸出；`GetStats` 定義在 `internal/sim/dynamic_threshold.go:170`。
- **改動量**：小（1–2 檔，新增唯讀呼叫）。
- **風險**：**低**。不觸碰任何門檻計算；只讓「這個引擎算了什麼」變成可觀測。
- **可驗收方式**：
  1. 端到端跑一次 `RunDailySimulation`，trace 中出現 `current_threshold`／`update_count` 事件（**修前沒有這個事件**）。
  2. 守門測試：斷言 trace 事件存在，且 `update_count == 0`、`last_vix == 20.0`（把「輸入未被餵」這個事實**釘在測試裡**，避免日後誤以為它在生效）。
- **附帶效果**：可作為選項 B/C 的前置量測（先有基線才知道接線後漂移多少）。

### 選項 B（**中等風險，需先決定語意**）：把 C2（模擬引擎信心門檻）動態化

- **位置**：`internal/sim/engine.go:198`（`effectiveConvictionFloor`）＋ 呼叫端 `internal/sim/engine.go:909`；注入點 `internal/orchestrator/composition.go:76`。
- **改動量**：中（`sim` 1 檔 ＋ 需**先解掉 §1.4 的雙實例問題**，否則讀到的是未被寫入的 A）。
- **風險**：**中高**。直接改「哪些標的被買」⇒ 改選股/績效結果，屬實盤語意。
- **可驗收方式**：
  1. 單元測試：固定 `vix`/`regime` ⇒ 門檻值逐字斷言（含 clamp 邊界 0.40/0.85 與換算後的整數化規則）。
  2. 回歸測試：同一份 replay 資料在 `--charter-mode off` 下，修前/修後的 `Orders` 與 `EndingCash` 差異清單（**差異必須可解釋**，且數量級應與門檻移動幅度相符）。
  3. 必須附「VIX 從哪來」的資料鏈證明（`vixFromQuotes`：`system.go:802`、`system_dispatcher.go:105`）。

### 選項 C（**高風險，前置條件未滿足**）：把 C3（盤前相關性上限）補齊並動態化

- **位置**：`internal/risk/pre_trade.go`（新增 `ruleCorrelation`）＋ `internal/risk/pre_trade.go:66` 的 `Check` 規則串 ＋ 動態來源 `internal/sim/dynamic_threshold.go:60`。
- **改動量**：**大**（需先建相關性資料面；`PortfolioState` 目前不含相關性矩陣）。
- **風險**：**高**。這會在實盤盤前閘門新增一條會擋單的規則；且 `MaxCorrelation=0.70` 與引擎 `baseThreshold=0.70` 的巧合**不足以證明**兩者是同一設計意圖（本文件未找到任何 commit message／doc 把兩者連起來）。
- **可驗收方式**：
  1. 表驅動測試：`absCorr` 跨過上限 ⇒ 逐字 `VerdictBlock` 或降權，並附 `Reason`。
  2. 端到端：3 個高相關持倉 ＋ 1 筆同族群新單 ⇒ 被擋；移除相關性資料 ⇒ 明示降級（不得靜默放行）。
  3. 需要**先**有一個「相關性來源」的決策（`portfolio/volatility_manager.go:32 correlationMatrix` 是現成候選，但其生產接線同樣待查）。

### 已排除的選項（**不建議**）

| 選項 | 為什麼排除 |
|---|---|
| 接到 C4（`portfolio.Sizer`） | `Sizer` 整個型別 0 生產消費者（§2 C4）⇒ 接上去沒有任何可觀測效果，只是把一個死旋鈕接到另一個死旋鈕。 |
| 直接刪除 `DynamicThresholdEngine` | ① 會撞 API surface golden（`internal/sim/testdata/sim_api.golden.json:71-117,225,253,302-312,327` 逐字釘住此型別與 `Engine.thresholdEngine` 欄位）；② `FU-20261016-32` 已定調「未移除任何符號」；③ 這是一個**意圖聲明**，刪掉會失去「原設計想做什麼」的線索。 |
| 只把 `SetRiskAppetite` 的實例與 sim 引擎的實例合併 | 兩者目前都無人讀 ⇒ 合併**沒有任何可觀測效果**；但它是選項 B 的必要前置（見 §1.4）。 |

---

## 5. 明示未實作

- 本文件**只是提案**。本車道**沒有**改任何 `.go` 檔、**沒有**新增或修改任何測試、**沒有**動 `docs/operations/FOLLOWUPS.md`、**沒有** commit、**沒有** push。
- 任何實作（含選項 A 的「只接觀測面」）**需業主決定**後另開車道。
- 依 `FU-20261016-32` 的判準，本條屬「該接線，只提案」類：涉生產風控行為（門檻值）者不自行動工。
- **特別聲明**：連選項 A 都沒有實作。原因是「接到觀測面」也需要決定**輸出到哪**（trace？dashboard？MCP tool？），
  那是一個介面決策，不該由唯讀調查車道單方面決定。

---

## 6. 已知限制（誠實列出未查證處）

1. **未查證「誰應該餵 VIX/regime」**：本文件確認 `GetThreshold`/`DetectRegime` 在生產 0 呼叫者，
   但**沒有**判定「應該由哪個元件餵」（候選：`vixFromQuotes` `system.go:802`、`internal/realtime/regime_adapter.go:75 RegimeDetector`、`internal/portfolio/regime.go`）。
   這是選項 B 的前置設計問題，未解。
2. **未查證 C3／C4 的 0.70 是否「本來就是」引擎 `baseThreshold` 的同一個設計意圖**：
   三處都是 0.70（`dynamic_threshold.go:45`、`defaults_portfolio.go:344`、`defaults_engine.go:644`），
   而 `WithThresholdEngine` 的註解寫「for correlation-based filtering」（`internal/sim/engine.go:122`）——**這只是線索，不是證據**。
   本文件未找到任何 commit message 或設計文件把兩者連起來（`git log -S 'DynamicThresholdEngine'` 的 5 筆中無相關敘述）。
3. **未查證「動態門檻在哪些路徑本來就被別的機制取代」**：已確認 C1（CRO 的 RISK_OFF⇒70）是**活的、且已校準**的替代機制
   （`executor_control.go:325-338` 註明「A6 (perf audit 2026-08-21)」），但**未**逐一確認其他路徑
   （`internal/experiment`、`internal/backtest`、`cmd/experimental/*`）是否各有自己的門檻邏輯。
4. **未查證 C5 的兩個 setter 是否「本來就該由 `cmd/atlas/main.go` 註冊」**：
   本文件只證明 `SetCrisisModeSetter`/`SetCorrelationSetter` 在 `cmd/` 與 `internal/` 非測試檔 0 呼叫者，
   **沒有**判定這是不是另一個獨立的缺陷（若是，應另開 FU）。
5. **未查證 `internal/monitoring` 或 MCP 是否已把 `current_threshold` 暴露給人看**：
   只確認 `cmd/atlas-mcp` 的 `risk_get_*` 工具中**沒有**這個欄位（`tools_risk_alert.go` 的 `GetStats` 命中是 `handleAlertGetStats`，與本引擎無關）。
6. **文件與程式碼不一致（本文件只記錄、未修）**：
   - `docs/specs/sim-engine-spec.md:19` 寫「`DynamicThresholdEngine` 對高度相關的訊號套用相關性過濾」——**生產上沒有**。
   - `docs/specs/sim-engine-spec.md:21` 寫「若 `thresholdEngine` 已設定，`RunDay` 會過濾重複符號的建議，保留信心度最高者」——**`RunDay` 完全不使用 `thresholdEngine`**（`internal/sim/engine.go:363-470` 無任何引用）。
   - `docs/specs/sim-engine-spec.md:33` 的核心流程把「3. 透過 thresholdEngine 過濾重複符號」列為執行步驟——**該步驟不存在**（且流程圖的買/賣順序與 `RunDay` 相反：程式是賣 `// 2. Sell logic`（`:406`）先於買 `// 3. Buy logic`（`:504`））。
   - `docs/ATLAS_METHODOLOGY.md:24` 把「模擬動態閾值」列為現行機制。
   這些是**文件層的假綠**，屬另一條缺陷（未修，僅記錄）。
7. **已知的先前記錄**（不是本文件的新發現）：
   `docs/decisions/2026-08-21-performance-root-cause-audit.md:132` 已列「動態風險閾值 DynamicThresholdEngine ❌ GetThreshold 零非測試呼叫」；
   `docs/specs/exported-risk-symbols-audit-20261016.md:134,240` 與 `docs/operations/FOLLOWUPS.md:5811` 已把它記為 `-32` 的已知限制。
   本文件的增量是：**完整公開面逐符號判定（§0.3）＋ 兩個實例的事實（§1.4）＋ 候選消費者與單位相容性盤點（§2/§3）＋ 選項（§4）**。

---

## 附錄 A：本文件的複現指令（唯讀）

```console
$ cd /Users/kaecer/workspace/atlas/.worktrees/fu35-36-positions-threshold
$ git rev-parse HEAD            # 45d2a6dbce191235e375111f4541ccd5b2cf396d
$ git status --short            # 本車道（唯讀調查）只新增本文件；
#   worktree 內另有 parent 車道 FU-20261016-35 的 in-flight 改動（internal/live/store/store.go 與其新測試），
#   與本文件範圍無關，本車道未碰。實測輸出：
#    M internal/live/store/store.go
#   ?? docs/specs/dynamic-threshold-engine-inert-readers-audit-20261016.md
#   ?? internal/live/store/store_positions_determinism_test.go
$ python3 /tmp/fu35-36-repro.py .        # §0.3 的三分類判定表
$ grep -rn 'GetThreshold(' --include=*.go .            # §0.2(1)
$ grep -rn 'thresholdEngine' --include=*.go .          # §0.2(5)
$ grep -rn 'SetRiskAppetite(' --include=*.go .         # §0.2(7)
$ grep -rn 'MaxCorrelation()' --include=*.go .         # §2 C3
$ grep -rn 'crisisMode' --include=*.go . | grep -v _test.go   # §2 C5
```
`/tmp/fu35-36-repro.py` 為本車道自製的唯讀腳本（只做 `grep` ＋ 行分類，不寫入 repo）。
