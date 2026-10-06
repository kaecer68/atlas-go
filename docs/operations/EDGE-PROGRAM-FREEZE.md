# Edge 驗證計畫 Phase 0 凍結登記（EDGE-PROGRAM-FREEZE）

> **文件角色**：edge 驗證計畫 Phase 0 的凍結登記唯一 SSOT——凍結清單、解除條件、spawn 保留策略公式與命中率語彙定義以本檔為準，其他文件只引用、不複寫。規格來源：`PLAN-v2.md` §1.2／§3.0／§3.1／§3.7。

| 登記項 | 內容 |
|---|---|
| 權威來源 | `atlas-notes/implementation-plan-20261006/PLAN-v2.md`（v2, 2026-10-06）§1.2、§3.0、§3.1、§3.7 |
| 掛載 | `docs/documentation-map.md` Operations 區一行；`AGENTS.md` **不加行**（160 行上限，反膨脹規則） |
| 驗證命令 | `bash scripts/ci/check_docs_governance.sh && grep -n "EDGE-PROGRAM-FREEZE" docs/documentation-map.md` |
| 門檻 | 腳本 exit 0；`grep` 命中 1 行；本檔 ≤ 120 行 |
| 回滾 | `git revert`（純文件變更） |
| 依賴 | 無 |

---

## ① 凍結清單（PLAN-v2 §1.2 N1–N7）

凍結期間下列事項一律視為**越界**，不得以「順手」「必要」「只改一點」為由例外。

| # | 凍結內容 | 理由 |
|---|---|---|
| N1 | 不新增訊號家族、不新增 stockpicker condition | 現在的問題不是訊號不夠，是無法否證 |
| N2 | 不新增 MCP tool／不擴充 tool catalog（現 117 個） | 工具數已 117；新增只增加表面積 |
| N3 | 不新增或修改分類器（regime／壓力指數／期間偵測） | 分類器換手本身是污染源（2026-08-14） |
| N4 | 不新增頁面／前端功能 | 呈現層不是瓶頸 |
| N5 | 不新增 CI script／不新增 CI 檢查階段 | PIT 與統計正確性以既有 `go test` 落在 `make ci-full` 內 |
| N6 | 不改 `configs/parameters.json`、`configs/agents.json` 的**值**（含 golden） | 值變更需業主授權（FU-20260930-05 紀律） |
| N7 | 不重寫舊預測層、不移除既有量測家族、不新開 repo | root 決策；資料層是唯一被證明過的資產 |

### ①-1 spawn 產物保留策略（公式登記）

- 上限公式：`find . -name 'spawn_*.md' | wc -l` **≤ 138（tracked）＋ 500 × k**
- `k` = `PromptsDir` 目錄數；本規劃實測 **k = 4** ⇒ 上限 **2,138**
- 四個 `PromptsDir`：`prompts/agents`、`internal/orchestrator/prompts/agents`、`cmd/atlas/prompts/agents`、`cmd/experimental/staging-drill-strategy-techniques/prompts/agents`
- **紀律（強制）**：任一 PR 新增 `PromptsDir` 時，MUST 同步更新本公式**並在本文件登記**；未登記即視為越界。
- v1 的「≤638」與「≤500」兩個數字皆為錯（前者把 k 當 1，後者漏算 tracked 與 k）；**以本式為唯一門檻**。

---

## ② 解除條件

凍結只有兩種情形可解除，且**兩者缺一不可**：

1. **附上 gate report**：G2／G3／G4' **全數通過**的 gate report（PLAN-v2 §5、§11；G2 為對比檢定）。單項通過、部分通過或僅口頭結論皆不算。
2. **取得業主授權**：業主對解除凍結的明確授權。

- **未附 gate report，或未經業主授權 ⇒ 凍結持續**（N1–N7 與 ①-1 上限同時維持）。
- gate 結果為 FAIL 或 INSUFFICIENT **不構成解除理由**；此時處置依 PLAN-v2 §6（決策點）與 §11（kill gate）執行。
- 解除登記方式：在本文件新增一節，記錄解除日期、gate report 位置、授權來源；**未登記不得實作**。

---

## ③ 命中率語彙 SSOT

**本節為命中率語彙的唯一 SSOT**；Phase 1 的 `edge-lab-spec.md`（將建立於目錄 `docs/specs/`）**只引用、不得複寫定義**（避免雙 SSOT）。

| 語彙 | 定義 | 禁止用法 |
|---|---|---|
| `measured_net_hit_rate` | 由 `stock_signal_outcomes` 實測、已扣 `cost_rate`、附 n 與 Wilson CI，且**方向感知**（PLAN-v2 §9 `hit_dir`） | 不得用於 narrative 先驗 |
| `prior_hit_rate` | 手寫先驗常數（`templates.go`），**無量測** | 不得寫成「歷史命中率」「回測」 |
| `calibration_score` | 校準器輸出（如 `internal/calibration/predictor_calibrator.go` 的 hit-rate score） | 不得當成策略績效 |
| `observational_expectancy` | 未通過 G2/G3/G4' 的期望值，只能標「觀察性」 | 不得寫成「可交易 edge」 |
| `uncalibrated_distribution`（v2／M7 新增） | 未經 OOS 校準即產出的 `p_down/p_flat/p_up`；此狀態下**不得對外呈現機率** | 不得當成校準後機率或用於 Brier 比較 |
