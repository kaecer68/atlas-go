# Industry Hit-Rate Consumption Chain（2026-09-24）

| 項目 | 內容 |
|---|---|
| 文件角色 | 把 stockpicker-stage 的 canonical（扣成本口徑）**產業級命中率**接進消費鏈路的接線契約 |
| 狀態 | v2（2026-09-24）；取代同主題 v1（僅描述設計、與實作不一致） |
| Source-of-truth（gate） | `configs/parameters.json` → `sector_allocation.industry_hit_rate_consume_enabled`（預設 `false`） |
| Source-of-truth（code） | `internal/sectorallocation/industry_hitrate_consume.go`、`internal/sectorallocation/industry_hitrate_assessment_decorator.go`、`internal/sectorallocation/industry_hitrate_provider_registry.go`、`internal/capitalflow/assessment_decorator.go`、`internal/capitalflow/types.go`、`internal/stocktools/industry_hitrate_consume_provider.go` |
| 數學口徑（唯讀） | `internal/stockpicker/industry_winrate.go`（Wilson CI + `min_samples` 校準狀態；本鏈路不改算式） |
| 相關票 | [#1942](https://github.com/kaecer68/atlas-go/issues/1942)（產業級命中率聚合）、[#1948](https://github.com/kaecer68/atlas-go/issues/1948)（canonical 扣成本口徑）、[#1943](https://github.com/kaecer68/atlas-go/issues/1943)（canonical L1 命名空間，本鏈路只讀） |

> **一句話**：命中率過去只是報告；本鏈路讓 `sectorallocation` 的 applied 權重決策與 capital_flow_assessment 讀得到它，且**預設關閉**、可一行 config 復原。

---

## §0 問題

`#1942`/`#1948` 已產出 canonical 產業級命中率（命中 = `forward_return - cost_rate > 0`、固定持有期、Wilson CI、`min_samples` 校準閘門），但除 `/api/stock/industry_winrate` 報告外**沒有任何 production 路徑消費它**：同一策略的命中率與投組配置完全脫鉤。命中率越準，配置越不受影響 — 這是 #1944 系列「inert 閉環」的一項。

## §1 安全閥（核心）

| Gate | 預設 | 違反後果 |
|---|---|---|
| `sector_allocation.industry_hit_rate_consume_enabled` | **`false`** | production 路徑偏離改動前基準；本 PR 的驗收 hard gate |

gate 關閉時的行為必須與改動前**逐位元相同**，且由測試持續釘住，而不是靠人工比對：

1. **driver 層（決定性）**：`ApplyIndustryHitRateToDrivers` 在 gate off / fail-closed 時**原值回傳**（不複製、不寫 map）；測試比對 `DriverInputs` 的 JSON 是否完全相同，並確認 provider 一次都沒被呼叫（`provider.calls == 0`）。
2. **projection 層（逐位元快照）**：`internal/sectorallocation/testdata/production_path_off_baseline.golden.json` 是**改動前 revision**（`origin/main`）在同一劇本下產出的 `ProjectedTarget` JSON；gate off、gate off + 已註冊 provider、gate on + 證據不足（fail-closed）三種情境都必須與它**逐位元相同**。
3. **assessment 層**：`IndustryHitRateEvidence` 為 `nil` 時 `omitempty` 讓 `industry_hit_rate_evidence` 整個 key 不出現（測試斷言序列化結果不含該 key）。

> **為何快照劇本用均勻權重**：`Projector` 以 map 迭代順序累加再歸一化，非均勻權重的和會因 Go map 迭代順序在最後幾個 bit 擺動（實測：同一輸入在**改動前 revision** 50 次得到 5 種不同 JSON）。均勻 1/20 權重讓求和不敏感，快照比對才是真正的逐位元比對，而不是 flaky 的 ULP 比對。

## §2 兩條消費面

### 2.1 (a) sectorallocation recommendation 路徑

**位置**：`ComputeProjectedTarget`（`internal/sectorallocation/engine_impl.go`），在 `collectFactorDeltas` 之後呼叫 `ApplyIndustryHitRateToDrivers`。

**tilt 算式**（`tiltMagnitude`）：

```
magnitude = clamp((WilsonLower - 0.5) * 0.2, -0.05, +0.05)
if Direction == "avoid": magnitude = -magnitude
```

| WilsonLower | buy | avoid |
|---|---|---|
| 0.30 | −0.04 | +0.04 |
| 0.50 | 0.00 | 0.00 |
| 0.70 | +0.04 | −0.04 |
| 0.00 / 1.00 | −0.05 / +0.05（受 ±0.05 上限夾住） | 反向 |

- **夾制理由**：單一 driver 不得單靠自己把某產業推到 `ProjectionConstraints` 的曝險上下限（min 0.005 / max 0.50），故夾在 ±0.05。
- **寫入位置**：`DriverInputs.CapitalFlow`。gate off / fail-closed 時**原值回傳，不碰呼叫端的 map**；applied 時先複製 map 再寫入，呼叫端 map 不被改動。
- **同 key 累加**（不覆寫）：命中率證據與 E07 既有 capital-flow tilt 是兩個獨立證據，`Projector` 會歸一化總量；覆寫等於讓其中一個證據靜默消失。
- **只作用於 canonical L1**：`industry.IsL1` 檢查，非 L1 key 直接丟棄（否則 `Projector` 會以 SA-INV-04 拒收整個輸入）。
- **錯誤容忍**：applied 失敗（provider error）時 `ComputeProjectedTarget` 繼續投影，與 gate off 等價；錯誤由 §2.2 的 evidence 區塊表面化。

### 2.2 (b) capital_flow_assessment deprecation 路徑

**位置**：`Service.LatestAssessment`（`internal/capitalflow/service.go`）在建立 `daily.Assessment` 後呼叫 `applyAssessmentDecorators`；`sectorallocation` 在 `init()` 註冊 `industryHitRateAssessmentDecorator`。

**evidence 區塊**（`capitalflow.IndustryHitRateEvidence`，`omitempty`）：

| 欄位 | 內容 |
|---|---|
| `source` / `condition_id` / `rolling_window` | 實際讀取的查詢 tuple（`stockpicker-momentum-20d-positive` / `momentum-20d-positive` / `120d`） |
| `applied` | 是否真的產生 tilt |
| `reason` | 決定性原因（下表） |
| `rows_total` | 報告列數 |
| `rows_calibrated` | **可被消費**的列數（canonical L1 **且** `calibration_status == eligible`） |
| `mean_wilson_lower` | 被消費列的 WilsonLower 平均（4 位小數）；未 applied 時 0 |
| `max_tilt` / `min_tilt` | 被消費列的 tilt 極值（6 位小數）；未 applied 時 0 |

**約束**：advisory，不權威 — 不得改寫 `CalibrationStatus`、`EligibleForAutomation`（E07 校準管線擁有該判定）。gate off 時 evidence 為 `nil`，JSON 不序列化該 key。

### 2.3 失敗語意（fail-closed，必附 reason）

| 情境 | `applied` | `reason` | 行為 |
|---|---|---|---|
| gate off | false | `disabled` | provider **不被呼叫**；driver 與 assessment 皆為改動前基準 |
| gate on、未註冊 provider | false | `no_provider` | 不改任何 driver（WARN log：wiring bug） |
| gate on、provider 回錯 | false | `provider_error` | 不改任何 driver（WARN log） |
| gate on、報告 0 列 | false | `no_rows` | 不改任何 driver |
| gate on、有列但無 `eligible` | false | `insufficient_calibration` | 不改任何 driver（**不以 0 或猜測值替代**） |
| gate on、≥1 列 eligible | true | `applied` | 產生 tilt |

未 applied 時仍會發出 evidence 區塊（gate on 的情況下），讓操作者能分辨「關掉了」與「開了但證據不足」。這組 reason 字串是契約（`sectorallocation` 的 `IndustryHitRateReason*` 常數），測試直接斷言。

### 2.4 Provider 綁定（非 inert）

`cmd/atlas/main.go` 在建立 read-only 產業命中率 provider 的同一個分支內，呼叫 `sectorallocation.RegisterIndustryHitRateProvider(stocktools.NewSectorAllocationHitRateProvider(winRateProvider))`。綁定本身無條件執行但**無作用**（每次讀取前先檢查 gate），所以不存在「gate 開了卻沒人綁」的 inert 狀態。

## §3 可逆性

- 唯一開關是 config；`industry_hit_rate_consume_enabled=false` 後**下一個 config reload** 即回到基準行為，不需重新部署程式。
- 不寫入任何歷史資料、不改寫既有 snapshot/receipt；tilt 只存在於當次投影的記憶體。
- `ResetIndustryHitRateProvider()` 可在不 reload config 的情況下解除綁定（rollback 預備）。

## §4 測試與驗收

| 測試 | 覆蓋 |
|---|---|
| `internal/sectorallocation/industry_hitrate_consume_test.go` | gate off（且 provider 未被呼叫）／no_provider／provider_error／no_rows／insufficient_calibration／applied 行數學／±0.05 夾制／非 L1 丟棄／fail-closed 不動 driver／applied 累加且不汙染呼叫端 map |
| `internal/sectorallocation/industry_hitrate_byte_identity_test.go` | gate off（含已註冊 provider）與 gate on + fail-closed 三情境逐位元等於改動前快照；gate on + applied 確實改變投影且維持 20 L1 / sum=1±1e-9／方向正確 |
| `internal/sectorallocation/industry_hitrate_decorator_internal_test.go` | decorator gate off 留 nil／gate on fail-closed 帶 reason／no_provider／applied 摘要統計且不動 CalibrationStatus |
| `internal/capitalflow/assessment_decorator_test.go`、`assessment_decorator_internal_test.go` | registry 語意／無註冊時零成本 no-op／decorator 在副本上執行／evidence 的 JSON omitempty 契約 |
| `internal/stocktools/industry_hitrate_consume_provider_test.go` | canonical 列映射／found=false 視為空報告（非 error）／error 包裝／nil inner fail-closed |
| `internal/config/testdata/*.golden.json` | 新參數進入 public API snapshot 與 default config golden |

驗收指令：

```
go test ./internal/sectorallocation/... ./internal/capitalflow/... ./internal/stocktools/... -count=1
make ci-gate
```

## §5 非目標（明確界線）

- 不改 stockpicker 算式、Wilson 區間、`min_samples` 或成本口徑。
- 不改 #1943 定案的 canonical taxonomy / sectormap 映射（只讀）。
- 不改 `Projector` 的投影公式與歸一化；tilt 只是其中一個 driver delta。
- 不改 `CalibrationStatus` / `EligibleForAutomation` 的語意。
- 不新增 default-on 參數；promotion（default 翻 true）需另開票並經業主核准（§1）。

## §6 Promotion 前置（要打開 gate 前）

1. 觀察期 ≥20 sessions、0 invariant violations（`engine_canonical_test.go` 等既有 invariant 全綠）。
2. 確認 canonical 產業命中率報告在觀察窗內有 ≥1 個 `eligible` 列（否則開了也是 `insufficient_calibration`）。
3. 在 sectorallocation 與 capitalflow 兩個下游各跑一輪 smoke（`/api/capital-flow/daily` 的 `industry_hit_rate_evidence.applied == true`）。
4. 改 default 需另開票、經業主核准；本契約不得自行翻 true。
5. **延遲成本**：gate 開啟後，每次 `LatestAssessment`（`/api/capital-flow/daily`）與 `ComputeProjectedTarget` 都會多一次 canonical 聚合讀取（on-the-fly aggregation，見 `internal/stocktools/industry_winrate.go`）。promotion 前先量測端點延遲；若不可接受，先在 promotion PR 加一層 cache（本 PR 未加，因為 gate 預設 off 時完全不讀）。
