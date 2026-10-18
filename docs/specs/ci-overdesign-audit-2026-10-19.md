# atlas-go CI 過設計審計與瘦身計畫

- **記錄日期**：2026-10-19
- **修訂版本**：v1
- **基準**：`origin/main` = `1b6c6798f7`（PR #2291）
- **業主反映**：「每次過 CI 非常多、非常久」
- **根本原因定性**：兩條 workflow (`ci-cd.yml` + `quality.yml`) 在 PR 上**重複觸發同名 job**，
  並各自跑一條 `frontend-build` 可重用 workflow ⇒ **同一個守門被擋兩次**，但只有其中一條
  觸發下游消費者需要它的產物 ⇒ 純粹的雙倍耗時且無附加保護。

## 量測（PR #2291；唯讀）

四個 workflow 觸發、總耗時 **790s（13m10s）**、總 check 數 **69**：

| Workflow | Run ID | Jobs | 總耗時 |
|---|---|---|---|
| **Code Quality** (quality.yml) | 39123045439 | 43（32 active, 11 skipped） | 530s = 8m50s |
| **CI/CD Pipeline** (ci-cd.yml) | 39123045444 | 23（11 active, 12 skipped） | 181s = 3m01s |
| **Constitution Check** (constitution.yml) | 39123045437 | 2 | 76s = 1m16s |
| **PR Base Guard** (pr-base-guard.yml) | 39123045432 | 1 | 3s |
| **總計** | — | **69** | **790s = 13m10s** |

### Code Quality 最慢的 7 個 job（>15s）

| Job | 耗時 | 觸發條件 |
|---|---|---|
| monitoring-config | 215s | `needs.changes.outputs.code == 'true'`（always-run 5 docker calls） |
| secret-scan | 33s | always-run（含 negative proof step） |
| check-orphan-artifacts | 29s | always-run |
| calibration-smoke | 25s | always-run（needs setup-go） |
| validate-parameters | 23s | always-run（needs setup-go） |
| inert-closure | 22s | always-run（含 hermetic contract test） |
| ledger-seam | 16s | always-run（含 hermetic contract test） |

### CI/CD Pipeline 最慢的 4 個 job

| Job | 耗時 | 觸發條件 |
|---|---|---|
| governance | 112s | always-run（4 個 sub-guard） |
| operations | 19s | always-run |
| commitlint | 8s | always-run |
| agents-md-drift | 6s | always-run |

## 重複 / 過時 / 可合併 識別

### A. 跨 workflow 同名重複（4 個）

| Job 名 | 兩處實作 | 真正差異 | 判定 |
|---|---|---|---|
| **changes** | ci-cd.yml:414（outputs: code/docs_only/integration） + quality.yml:18（outputs: code/ci/embedded_docs） | 不同 filter 集合 + 不同 outputs（互補） | **保留兩份，但同步 filter 內容**（見 §2.1） |
| **frontend-build** | ci-cd.yml:26 + quality.yml:80 | 都呼叫同一個 `_frontend-build.yml`（workflow_call） | **保留兩份**：兩 workflow 的下游各自需要上傳的 artifact（跨 workflow 不能共用 artifact） |
| **lint** | ci-cd.yml:153 + quality.yml:110 | ci-cd：`only-new-issues: false`（全量） / quality：預設（只新 issue）| **刪除 quality 的 lint**（ci-cd 的全量是嚴格超集；PR >20k lines 時已驗證 quality 預設會壞）|
| **generate** | ci-cd.yml:189（go generate ./... 含 git diff） + quality.yml:332（go generate . + ./cmd/atlas-mcp/... 含 git status --porcelain） | ci-cd 跑得較廣（`./...`），quality 用 porcelain 多抓 untracked | **刪除 quality 的 generate**（ci-cd 的 `./...` 涵蓋 quality 的 `.`；`git diff --exit-code` 對 tracked 檔漂移仍偵測得到）|

> **重複的 code 路徑**：ci-cd 的 `code` filter 多列了 `Makefile` / `scripts/**` / `.github/workflows/**`，
> quality 把這三項獨立成 `ci` filter。**§2.1 將兩邊 filter 同步為超集**，避免 silent drift
> （FU-20261015-01 同型事故：漏列目錄 ⇒ 守門靜默跳過）。

### B. 過時 / 死 gate（不在本次範圍）

- `scripts/verify-sector-allocation-closure.sh` 於 2026-09-26 由 E16 lane 改為 no-op、明示停用；
  CI workflow 無人叫它（已驗證：`grep -rn 'verify-sector-allocation-closure' .github/` 無命中）。
  ⇒ **本次 PR 不再動**。

### C. 可合併的秒級小 job（**不建議**）

每個 6s 級 job 的實際結構（以 `jev-contract` 為例）：

```
Set up job         (~1s, runner 啟動)
Run actions/checkout  (~2s, 含 LFS)
Run script         (~1s, 實際檢查 <0.1s)
Post checkout      (~1s, cleanup)
Complete job       (~1s, runner 結束)
─────────────────
合計：~6s（其中 2-3s 是 runner 啟動/收尾 overhead）
```

合併的預估節省：

| 策略 | Job 數 | 預估耗時 | 省下 |
|---|---|---|---|
| 現況（25 個 fast jobs 在 quality.yml） | 25 | 167s | — |
| 全部合成 1 個 `quality-fast` | 1 | ~50s | ~117s（包含單一 setup/checkout）+ 失去失敗訊號粒度 |
| **判定** | — | — | **不採用**：失敗訊號會變粗，連審查者無法快速定位；7 個獨立守門的契約測試各需要 1 個 job 才能在 PR 評論顯示具體哪一支壞了 |

### D. 不合理的設定

- **`monitoring-config` timeout 8 分鐘**：4 個 `promtool` docker 呼叫（每個 ~45s）+ 1 個 `amtool`。
  評估後判定**不修**——監控設定是 production 守門，promtool mutation negative proof 必須存在；
  8m 預算上界 + docker layer cache 是合理設計。
- **`channel-index` 70s**：constitution.yml 的 `check_channel_index.sh` 跑全 repo scan；
  不修——channel-index 是 spec/SSOT 同步的硬守門。
- **`governance` 112s**：ci-cd.yml 的 `verify_governance_gates.sh` 跑 4 個 sub-guard；
  不修——這條守 4 個 SSOT 守門的聚合，是 branch protection 的 12 個 required check 之一。

## 結論與處置

### 紅線（**不可放寬任何真閘門**）

本次瘦身的全部動作只有兩個：

1. **刪除 quality.yml 的 `lint` job**（被 ci-cd 的全量版 supersede）。
2. **刪除 quality.yml 的 `generate` job**（被 ci-cd 的 `./...` 版 supersede）。

加上兩個安全同步：

3. **同步 ci-cd.yml 的 `code` filter 與 quality.yml 的 `code`+`ci` filter**，確保兩份
   `changes` 的 path 過濾涵蓋完全一致（否則漏列目錄 ⇒ 守門靜默跳過，FU-20261015-01 同型事故）。
4. **不在 quality.yml 加 `Deprecated:`**——品質檢查沒有過期，是同一條閘門被兩條 workflow 重複觸發。

### 預估效益（PR 觸發；同 #2291 為基準）

| 項目 | 修前 | 修後 | 差 |
|---|---|---|---|
| 總 check 數 | 69 | 67 | **−2** |
| 總耗時（機器時間總和） | 790s | ~770s | **−20s** |

預期節省看似不大（只有 20s）——但**真正的效益是「不再每次跑同樣的全量 lint 兩次」**。
ci-cd 的 `lint` 本身在沒命中 patch filter 時跑全量約 60-90s；本次 PR 把 quality 的小 lint 砍掉
⇒ **該 lint 在 CI 上只跑一次**，且跑的是全量版（保護更強）。

### 為什麼不更激進

- **合併 workflow**：跨 workflow 不能共用 artifact；`_frontend-build.yml` 雖是 reusable workflow，
  每個 caller 仍會重跑一次 npm ci + build（cached on second，但首次仍耗時）。要徹底去重，必須
  合併 workflow ⇒ 觸及太多無關路徑，超出本次範圍。
- **合併小 job**：見 §C，會犧牲失敗訊號粒度。
- **移除 `release.yml` / `nightly-refresh.yml` / `daily-maintenance.yml`**：這些只在 tag/schedule
  觸發，不算 PR CI 耗時 ⇒ 不在本次範圍。

## 驗證（不會放寬保護的證明）

每項處置都對應一個**修前已被擋 / 修後仍被擋**的最小複現：

| 處置 | 修前保護 | 修後保護 | 證據 |
|---|---|---|---|
| 刪 quality `lint` | golangci-lint（全量，ci-cd） + golangci-lint（only-new，quality）| golangci-lint（全量，ci-cd）| ci-cd 的 `lint` 用 `only-new-issues: false` 是 PR >20k lines 的必要設定，本身就是 superset；quality 的 only-new 在該情境會壞（看 ci-cd 的註解）|
| 刪 quality `generate` | ci-cd 的 `go generate ./...` + drift check / quality 的 `go generate .` + `git status --porcelain` | ci-cd 的 `go generate ./...` + drift check | ci-cd 的 `./...` 涵蓋 quality 的 `.` + quality 多跑的 `./cmd/atlas-mcp/...`；drift check 對 tracked 檔完全等價。porcelain 對 untracked 多抓的能力由 `generate-docs-embed` 補（見 quality.yml:379）|
| 同步 filter | ci-cd `code` 多列 `Makefile`/`scripts/**`/`.github/workflows/**` / quality `code` 漏列 | 兩邊 `code` 同步含全部 | 預期兩份 changes 在同樣 PR 上得到同樣 `code=true`；驗證方式：跑同 PR 兩次比對 output |

### 驗證步驟（執行於實作 PR）

1. **修前**（PR #2291 基準）：69 checks / 790s 已記錄於本檔 §量測。
2. **修後**（同 PR 重觸發）：預期 67 checks / ~770s。
3. **守門未破的負向證明**：
   - 對 `lint`：拿一支故意觸犯 `errcheck` 的 fixture（既有 `tests/scripts/test-*.sh` 套件），
     確認修後只跑 ci-cd 全量仍紅；修前 quality 那條 small lint 也紅 ⇒ 兩條都會擋。
   - 對 `generate`：拿一支故意讓 `go generate ./...` 產出 diff 的 fixture（既有負向證明），
     確認修後 ci-cd 那條仍紅；quality 那條已刪 ⇒ 唯一擋的是 ci-cd。
4. **filter 同步**：用同 PR 跑兩次 workflow run，diff 兩份 changes job 的 `outputs.code` 必須相同。

## 已知限制 / 不在本次範圍

- **release.yml / nightly-refresh.yml / daily-maintenance.yml**：tag/schedule 觸發，
  不影響 PR CI 耗時，不動。
- **deploy-* / canary-test / integration / test-cmd-atlas**：業主明令**只檢視、不刪**。
- **deploy-staging / deploy-production / docker**：本來就在 skipped 狀態，不耗時。
- **跨 workflow 共用 artifact**：技術上不可行；合併 workflow 才能徹底去重 frontend-build 重複。

## 阻塞

無。
