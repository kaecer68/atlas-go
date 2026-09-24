---
title: sectorallocation 決定性規格（SA-DET-01）
status: active
updated: 2026-09-24
owner: sectorallocation
related:
  - docs/reference/traps.md（浮點累加禁止直接走訪 map）
  - docs/specs/sector-allocation-simulation-closure-spec.md（SA-INV-01/07 projection 契約）
  - issue #1961（production path ±1 ULP 非決定性）
---

# sectorallocation 決定性規格（SA-DET-01）

## 1. 問題（issue #1961 實證，2026-09-24）

`sectorallocation` 的 production projection 入口
`defaultEngine.ComputeProjectedTarget` → `Projector.Project`
在**同一 commit、同一輸入**下不是決定性的：連續執行會產生不同的輸出 JSON，
20 個 canonical L1 值**同時**位移 1–4 ULP。

實測（`origin/main` `88683f06`，production strategic prior：0.33/0.16/0.13/0.08 + 16×0.01875，
無任何 driver delta）：**20 個獨立 process 跑 20 次 → 4 個不同 hash**。

```
acc42a6b00e14ee54bbb2ffa30a5dbabe0f1308e918d49737b3c32155aaf43a3  ×10
cb37f12e743f78f67c24787ac20ef5e731afcf8954dce6589c2793e6fcb728d9  ×4   ← 正確值（等於宣告 prior）
8520b254c8dada50950b2da41114de90ce56263f80ea28fc95158e4e1ba1b4e2  ×4
4303fdc8cd6c8f5218ec0a26b00a042a08cfa4adbef3a5c7d061f2467da8c8e3  ×2
```

後果：

1. 「config off → 與合併前逐位元相同」這類驗收條件在**非均勻權重**路徑上數學不可達成。
2. golden / byte-identity 測試會**隨機紅燈**（#1959 的 `industry_hitrate_byte_identity_test.go`
   因此被迫改用**均勻** prior 當作「determinism anchor」）。
3. 下游回測重現性、快照比對、審計出現無法解釋的 1 ULP 漂移。

## 2. 根因

Go 的 `for ... range <map>` **每次執行的迭代順序都是隨機的**；IEEE-754 浮點加法**不具結合律**
（`(a+b)+c != a+(b+c)`）。因此任何「走訪 map 做浮點累加」的程式碼，其結果尾位都會逐次漂移。

修正前 `internal/sectorallocation/projector.go` 有兩處（`Projector.Project` 內）：

| # | 位置（修正前） | 問題 |
|---|---|---|
| 1 | `projector.go:142-159` | 8 個 driver map 被放進**一個 map** 後 `range`。同一 sector 被多個 driver 加 delta 時，`before + delta` 的順序隨機 → 尾位漂移；`AdjustmentLog` 的元素順序也跟著漂移。 |
| 2 | `projector.go:183-186`（正規化 sum）、`projector.go:199-202`（final sum check） | `for _, v := range target { s += v }`。20 個 sector 的總和 `s` 隨機落在 `1.0` 的 −2..+4 ULP；再 `target[id]/s` 會讓**全部** sector 同時位移 —— 正是 #1961 觀測到的現象。 |

實測該 prior 的總和在 20,000 種排列下取到 7 個不同值：
`0.9999999999999998`、`0.9999999999999999`、`1.0`、`1.0000000000000002`、
`1.0000000000000004`、`1.0000000000000007`、`1.0000000000000009`（= `1.0` 的 −2..+4 ULP），
故單次執行每個 sector 的偏差可達 4 ULP。

## 3. 契約（SA-DET-01，規範）

1. **禁止對 map 直接做浮點累加。** 任何 `s += v` / `x = x + d` 形式、且迭代來源是 map 的迴圈，
   必須先取得**排序後的 key slice** 再依序累加（`sortedSectorIDs` helper 為此而存在）。
2. **`Projector.Project` 的 driver 套用順序固定**為宣告順序
   `cycle → seasonal → linkage → narrative → macro → capital_flow → theme → prior`。
   新增 driver 必須加在這個 slice 的固定位置，不得改回 map。
3. **正規化加總、final sum check、`AdjustmentLog` 產生順序，一律以 sector key 字典序走訪。**
   `target` 的 key set 在 driver 套用後即固定，故排序一次、整個 maxIter 迴圈重用。
4. **確定性的可觀測定義**：同一輸入連續執行 N 次，`json.Marshal(ProjectedTarget)` 的
   sha256 必須完全相同（回歸閘門：`internal/sectorallocation/projector_determinism_test.go`）。
5. **數值語意**：修正只改變「浮點加法的結合順序」，不改變任何 sector 集合、clamp 邊界、
   driver 套用次數（SA-INV-08）或 sum 容差（SA-INV-07）。修正後的值必須落在修正前
   可達集合的 ULP 封包內（見 §5 證據）。

## 4. 修正

`internal/sectorallocation/projector.go`（commit 見 PR）：

- 新增 `driverApplication{name, weights}` 與固定順序 slice `driversInApplyOrder`（8 個 driver）。
- 新增 `sortedSectorIDs(map) []industry.SectorID`（`slices.Sort` 後回傳）。
- 正規化迴圈與 final sum check 改走 `ids := sortedSectorIDs(target)`。

**為何選「排序後累加」而非「改容差驗收」**：容差式比較只掩蓋症狀（golden 仍會隨機紅燈、
審計仍有無法解釋的漂移），且無法恢復「逐位元可重現」這個下游（回測、快照、審計）真正需要的性質。
排序 20 個 key 的成本可忽略（見 §6）。

## 5. 驗收證據（2026-09-24，`fix/20260924-sectoralloc-determinism`）

### 5.1 跨 process（20 個獨立 process，production prior、無 driver delta）

| | 修正前（`88683f06`） | 修正後 |
|---|---|---|
| distinct hash / 20 runs | **4** | **1**（`cb37f12e…7f37`） |
| semiconductor 值 | `0.32999999999999985` / `0.32999999999999996` / `0.33`（−4..0 ULP 隨機下沉），其餘 19 個 sector 同步位移 | `0.33`（= 宣告 prior 值，0 ULP） |

修正後 hash `cb37f12e…` **本身就是修正前可達的 4 個 render 之一**（修正前 4/20 次跑出），
因此沒有引入任何新值。

### 5.2 數值等價性（未改變語意）

**無 driver delta（#1961 實際情境）**：修正後每個 sector 都等於宣告的 prior 值
（偏差 **0 ULP**；修正前是 −4..0 ULP 的隨機下沉，50 次中只有 12% 落在正確值）。

**driver delta 情境（合成 3 個 driver，多個 driver 命中同一 sector）**：
修正後 20/20 個 sector 的值都落在修正前 50 次可達值的 `[min, max]` 封包內，
且修正後那組值本身就是修正前可達的 render 之一。

**關於「≤1 ULP」**：修正前不存在「單一正確值」——同一個 key 在 50 次中的 ULP 散布寬度達
**6 ULP**，因此沒有任何修正能同時與所有舊 render 相差 ≤1 ULP。可達成且唯一合理的目標是
「（a）落在原可達集合內、（b）固定成一個值」；兩者都已驗證。修正後與修正前**單一** render
的最大差為 3 ULP，這個差值即原本 bug 的散布本身，不是修正引入的偏差。

### 5.3 回歸閘門（修正後必須全綠，修正前必須全紅）

`internal/sectorallocation/projector_determinism_test.go`：

| 測試 | 修正前 | 修正後 |
|---|---|---|
| `TestProductionPath_Deterministic_ZeroDriverDeltas` | FAIL（20 次出現 4 種 render） | PASS |
| `TestProductionPath_Deterministic_WithDriverDeltas` | FAIL（20 次出現 20 種 render） | PASS |
| `TestProjectorProject_ZeroDriverDeltas_ReturnsPriorExactly` | FAIL（`financials` 0.12999999999999995 ≠ 0.13） | PASS |
| `TestProjectorProject_DriverOrderIsDeclarationOrder` | FAIL（`AdjustmentLog[0].Reason = capital_flow`） | PASS |

`testdata/projected_target_production_prior.golden.json` 是 production prior、無 driver delta 的
逐位元 golden（sha256 `cb37f12e…`），讓「config off 逐位元」這類驗收條件變成可達成的 gate。

## 6. 效能

`go test -bench`（2000x，5 次取樣，`internal/sectorallocation`）：

| 情境 | 修正前 | 修正後 |
|---|---|---|
| `ComputeProjectedTarget`（無 driver delta） | 11.10–11.22 µs/op，77 allocs | 10.96–11.43 µs/op，78 allocs |
| `ComputeProjectedTarget`（3 driver delta） | 22.2–23.2 µs/op，95 allocs | 23.5–24.6 µs/op，99 allocs |

差異在量測噪音內（≤ ~3.5%），20 個 key 的排序成本可忽略；此路徑一次 session 只跑一次。

## 7. 同類掃描（本次一併盤點，未在本 PR 修正）

同一個「map 走訪順序 → 浮點累加」缺陷在本 repo 其他位置也存在；它們**不在** #1961 的
production projection 路徑上，故未納入本次修正（避免一次 PR 改動多個子系統的數值），
但列在這裡供後續處置：

| 位置（本 PR base = `88683f06`） | 說明 | 是否 production |
|---|---|---|
| `internal/sectorallocation/legacy_gics_bridge.go:65` | `proj.Weights[l1] += weight * share` 走訪 `base` map 與 `m.Targets` map；多個 GICS key 對同一 L1 貢獻時順序隨機 | 否（只被 `cmd/experimental/industry-namespace-audit` 使用） |
| `internal/sectorallocation/engine_impl.go:128-148` | legacy `ComputeWeights` 由 map 產生 slice 後用**不穩定** `sort.Slice`（:141）依權重排序：權重相同時輸出順序隨機 | 否（無 production caller，僅 interface 保留） |
| `internal/portfolio/sector_rotator.go:217` | `normalizeAllocations` 的 `total += alloc`（:230）走訪 map | 是（legacy sector rotation 路徑） |
| `internal/portfolio/factor_weight_engine.go:113-133` | `GetWeights` 的 `weights[ft] += delta`（:116/:122/:125/:128/:132）走訪 map 累加 | 是（factor weight 路徑） |

修法與本規格相同（`sortedSectorIDs` 形式的 helper）。

## 8. 防護

- 回歸閘門：`projector_determinism_test.go`（20 次 in-process 必 byte-identical + golden 比對）。
- 任何新的「對 map 做浮點累加」程式碼，review 時必須要求排序後累加；trap 條目見
  `docs/reference/traps.md`（浮點累加禁止直接走訪 map）。
