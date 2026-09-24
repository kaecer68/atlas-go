# Industry Hit-Rate Consumption Chain（PR-β, 2026-09-24）

| 項目 | 內容 |
|---|---|
| 文件角色 | PR-β 的**接線契約**：把 `#1942`/`#1948` canonical 扣成本口徑產業級命中率接到 sectorallocation recommendation 路徑與 capital_flow_assessment deprecation 路徑的中間層規格 |
| 狀態 | v1（2026-09-24） |
| Source-of-truth | gate：`configs/parameters.json` -> `sector_allocation.industry_hit_rate_consume_enabled`（default `false`）；code：`internal/sectorallocation/industry_hitrate_consume.go` + `internal/sectorallocation/industry_hitrate_assessment_decorator.go` + `internal/capitalflow/assessment_decorator.go`；數學：`internal/stockpicker/industry_winrate.go`（唯讀） |
| 相關票 | [#1942](https://github.com/kaecer68/atlas-go/issues/1942)（本票唯讀新介面 + 文件）；[#1948](https://github.com/kaecer68/atlas-go/issues/1948)（PR 合併）；[#1943](https://github.com/kaecer68/atlas-go/issues/1943)（canonical L1 namespace） |

> **一句話**：把 stockpicker 的「industry-level canonical hit-rate」透過 config-gated hook 接入 sectorallocation 的 recommendation 路徑（作為 `DriverInputs.CapitalFlow` 的 additive tilts）和 capital_flow_assessment 的 deprecation 路徑（新增 `IndustryHitRateEvidence` 區塊）。**預設關閉**以保證與 `dfc4e3a1` byte-identical 的 production 路徑。

---

## §0 為什麼需要這份規格

- **沒有 production 接線**：`#1942`/`#1948` 把「產業級命中率」唯讀介面與文件做出來了，但任何 production 路徑（sectorallocation recommendation、capital_flow_assessment）都**不讀**這個指標；同一個策略的 stockpicker 命中率與投組配置完全脫鉤。
- **兩條不同的下游 surface**：
  - **(a) sectorallocation recommendation**：依 `ProjectedTarget.Target` 給最終投組權重；影響面是「哪個 L1 產業多配／少配」。
  - **(b) capital_flow_assessment deprecation**：依 `CapitalFlowAssessment` 給自動化決策的 eligibility 旗標；影響面是「自動化門檻的開／關」。
- **風險**：兩條 surface 都直接接生產管線，**預設關閉**是唯一可在 promotion 前保證 byte-identical 的安全閥。

本規格只描述接線契約與預設行為；不改 stockpicker 算式、不改 sectorallocation 投影公式、不改 capitalflow assessment 的 CalibrationStatus 語意。

---

## §1 安全閥（核心）

| Gate | 預設 | 違反的後果 | 修正動作 |
|---|---|---|---|
| `configs/parameters.json` -> `sector_allocation.industry_hit_rate_consume_enabled` | **`false`** | production 路徑偏離 `dfc4e3a1` 的 byte-identical 保證；root 會獨立驗並擋 PR | 翻回 `false`；如要永久改 default，**另開 issue 業主核准**，不得在本 PR 內改 |

驗證位置（code-level backstop，root 也會獨立驗）：

| 路徑 | 第一行檢查 |
|---|---|
| `BuildIndustryHitRateTilt` | `if !config.GetIndustryHitRateConsumeEnabled() { return nil, nil }` |
| `ApplyIndustryHitRateToDrivers` | `if !config.GetIndustryHitRateConsumeEnabled() { return drivers, nil }` |
| `industryHitRateAssessmentDecorator` | `if !config.GetIndustryHitRateConsumeEnabled() { return }` |
| `applyAssessmentDecorators` | 無註冊時 `return assessment`（true zero-copy no-op） |

每條路徑都做了**前置 gate check**，任何一條失守都會被 root 的 config-off regression 抓出來。

---

## §2 兩條下游 surface 的接線契約

### 2.1 Surface (a)：sectorallocation recommendation 路徑

**位置**：`internal/sectorallocation/engine_impl.go` 的 `ComputeProjectedTarget`。

**接線點**：`ComputeProjectedTarget` 在 `collectFactorDeltas` 之後呼叫 `ApplyIndustryHitRateToDrivers`，把 hit-rate 衍生的 per-L1 tilts 加到 `DriverInputs.CapitalFlow`。

**tilts 計算**（`BuildIndustryHitRateTilt` + `TiltToDriverMap`）：

```
magnitude = (WilsonLower - 0.5) * 0.2
if Direction == "avoid": magnitude *= -1.0
```

- WilsonLower 0.5 -> 0.0 tilt（中位）
- WilsonLower 0.7 -> +0.04 tilt（看好）
- WilsonLower 0.3 -> -0.04 tilt（看壞）
- avoid direction 反轉

**約束**：
- 僅作用於 L1 keys（defensive `IsL1` check 阻擋 L2 / 非 sector key，符合 SA-INV-04）。
- additive 不覆寫：若 `DriverInputs.CapitalFlow` 已存在某 key，hit-rate tilt 不覆寫。
- 不跨 source 合併（每個 condition 一次呼叫，呼叫端決定要打幾個 source）。

**錯誤語意**：gate ON 但 provider 回 error -> `ApplyIndustryHitRateToDrivers` 回傳 `drivers, err`；`ComputeProjectedTarget` 在 err 時**容忍**，繼續投影（與 gate-off 行為一致）。錯誤由 assessment decorator 表面化（見 §2.2）。

### 2.2 Surface (b)：capital_flow_assessment deprecation 路徑

**位置**：`internal/capitalflow/service.go` 的 `Service.LatestAssessment`。

**接線點**：`LatestAssessment` 在 `daily.Assessment` 構建完成後呼叫 `applyAssessmentDecorators(daily.Assessment)`。sectorallocation 在 `init()` 註冊了 `industryHitRateAssessmentDecorator`，把 tilts 摘要寫進 `CapitalFlowAssessment.IndustryHitRateEvidence`。

**evidence 區塊結構**（`IndustryHitRateEvidence` struct）：

| 欄位 | 型別 | 內容 |
|---|---|---|
| `source` | string | 例如 `stockpicker-momentum-20d-positive` |
| `condition_id` | string | 例如 `momentum-20d-positive` |
| `rolling_window` | string | 例如 `120d` |
| `rows_total` | int | L1 列總數 |
| `rows_calibrated` | int | `observations >= 30` 的列數（符合 stockpicker spec §1.1 min_samples） |
| `mean_wilson_lower` | float64 | calibrated 列的 WilsonLower 平均，4 位小數 |
| `max_tilt` / `min_tilt` | float64 | per-L1 tilt 極值，6 位小數 |
| `enabled` | bool | mirror gate 狀態（gate off 時 evidence 為 nil，所以此欄位不會被序列化） |

**約束**：
- **advisory 不權威**：evidence 區塊**不會**改變 `CapitalFlowAssessment.CalibrationStatus`（仍由 E07 H-CF-02 pipeline 決定）。
- **omitempty**：當 gate off 或 provider 未接時，`IndustryHitRateEvidence` 為 `nil`，JSON 不會序列化整個區塊 -> byte-identical 到 `dfc4e3a1`。
- **錯誤容忍**：gate ON 但 provider error -> evidence 為 `nil`，`applyAssessmentDecorators` 不 panic。

---

## §3 觀察期紀律（PR-β promotion gate）

要把 `industry_hit_rate_consume_enabled` 從 `false` 翻到 `true`：

1. **觀察期 ≥20 sessions + 0 invariant violations**：sectorallocation 與 capitalflow 的現有 invariant 測試（`engine_canonical_test.go`、`validation_v2_family_test.go`）必須全綠。
2. **從 default 改起時另開 ticket**：本 PR 內**不得**改 default；改 default = 影響下一個合併的 production 路徑，必須業主核准。
3. **兩個下游模組各跑一輪 manual smoke**：
   - sectorallocation：`ComputeProjectedTarget` 跑一輪，檢查 `AdjustmentLog` 與 `DriverProvenance` 帶有 hit-rate provenance。
   - capitalflow：`/api/capital-flow/daily` 跑一輪，檢查 response 的 `industry_hit_rate_evidence` 不為 null 且 rows_total > 0。
4. **確認 root 的 byte-identical 驗證通過**：config off 啟動 + 跑現有 regression -> 與 `dfc4e3a1` 逐位元比對（root 執行）。

---

## §4 測試與驗收

| 測試 | 覆蓋 |
|---|---|
| `internal/sectorallocation/industry_hitrate_consume_test.go` | gate-off no-op（BuildIndustryHitRateTilt 回 nil；ApplyIndustryHitRateToDrivers 不變更 drivers）；行數學（buy/avoid × WilsonLower 邊界）；TiltToDriverMap 空輸入回 nil、非 L1 key 拒絕；gate-off 與 nil provider 容忍；`ComputeProjectedTarget` 在 gate-off 時 byte-identical（20 L1 keys + sum=1±1e-9） |
| `internal/capitalflow/assessment_decorator_test.go` | `RegisterAssessmentDecorator(nil)` 不 panic；`ResetAssessmentDecorators` 冪等；多 decorator 註冊 |
| `internal/config/testdata/parameters_api.golden.json` + `default_parameters_config.golden.json` | regenerated 包含新 field + getter + var |
| `internal/config/parameters_compliance_test.go`（既有） | default config 必須 `Validate()` 通過 |
| `internal/sectorallocation/engine_canonical_test.go`（既有） | SA-INV-01/04/05/07 不變（ComputeProjectedTarget 仍 20 L1 + sum=1±1e-9） |

**驗收（root 執行）**：

| 項目 | 期望 |
|---|---|
| config **off** 啟動 + 跑現有 regression | 與 `dfc4e3a1` byte-identical（或差 ≤ diff 容忍值，需註明） |
| `configs/parameters.json` 中 `industry_hit_rate_consume_enabled` 預設 | `false` |
| 新增任何 default-on config | 無 |
| `git diff --stat` 動到的檔案 | 見 PR body 改動檔案清單 |
| invariants 自守測試（#1940, #1942） | 全綠 |
| CI | 全綠 |

---

## §5 未在本 PR 範圍（明示）

1. **stocktools production provider wiring**：`internal/stocktools/industry_winrate.go` 的 `SQLiteWinRateProvider` 與本 PR 的 `IndustryHitRateProvider` interface 之間的 adapter 尚未寫。屬 promotion gate 任務，由 cmd/atlas owner 在觀察期開始時執行（避免 stockpicker → ledger → portfolio → sectorallocation import cycle）。
2. **`internal/monitoring/service/industry.go` 的 `generateRecommendation`**：dashboard 介面的 recommendation 邏輯尚未引用 hit-rate evidence。屬後續 PR-β surface，依賴 dashboard 介面契約確認。
3. **config knob for source/condition/window**：本 PR 把 source/condition/window 寫死成 `industryHitRateSource/Condition/RollingWindow` 包級私有常數。Promotion gate 後再決定是否暴露成 config。
4. **multi-source 合併**：本 PR 一次只打一個 condition；multi-condition 需要在 caller side 決定策略（取 mean、最大者、加權），屬後續設計。
5. **其他閉環**：Darwinian 權重、portfolio sizing、orchestrator macro factor 都**不**引用 hit-rate evidence；屬後續設計。

---

## §6 呈現紀律（強制）

1. **只有「H2b 命中率」數字可稱為「命中率」**：與 stockpicker spec §7 一致。`IndustryHitRateTilt.WilsonLower` 等欄位的對外呈現必須帶樣本數與 calibration status。
2. **`IndustryHitRateEvidence` 是 advisory，不權威**：dashboard 不可把它顯示為「自動化可用」的依據；仍須看 `EligibleForAutomation()`（CalibrationStatus 旗標）。
3. **gate 預設 false 是核心安全閥**：UI / 文件必須明確標示「生產路徑未啟用」。變更 default 必須走業主核准流程。
