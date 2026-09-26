# 缺陷收斂 Manifest（Remediation Manifest）— SSOT

> **用途**：atlas-go 的缺陷收斂**單一追蹤來源**。所有 lane 在動工前先讀此表。
> **規則（強制）**：
> 1. **未列此表者不派工**；看到問題先登記（D/E 分類），不即時修改。
> 2. **並行前必須切分命名空間**（見 §1）；做不到切分就**串行**。
> 3. 完成後把狀態改成 ✅ 並附 PR；**不得刪列**（保留歷史）。

## §1 命名空間切分表（避免撞號／撞檔）

| 共享資源 | 分配方式 | 現況 |
|---|---|---|
| `docs/operations/FOLLOWUPS.md` 的 `FU-<date>-NN` | **依 lane 事前分配號段**（同一日最多一個 lane 用一個連號區段）| 2026-09-26 已發生一次撞號（`FU-20260926-10` 被兩 lane 同取 → main 紅燈，由 #2022 修）|
| `docs/reference/traps.md`（**硬上限 330 行**）| 同時最多 **1 個 lane** 可改；其他人等它併入 | 目前 `atlas-quota-2014`(#2021) 佔用 |
| `Makefile` 的 `ci-gate` / `ci-quick` 區塊 | 同時最多 **1 個 lane** | 目前 `atlas-cov-fix`(#2009) 佔用 |
| `.github/workflows/quality.yml` | 同時最多 **1 個 lane** | 目前 `atlas-cov-fix` + `revert-guard`(#2003) 佔用 |
| `docs/specs/<topic>.md` 章節編號 | 新章節前先看是否已被同 PR 佔用 | #1990/#1991 曾同時寫「§10」|

## §2 設計問題（D）

| ID | 問題 | 證據 | 擁有者 | 狀態 |
|---|---|---|---|---|
| D1 | FinMind 配額未約束跨 process 總量（`used=12982 > 上限 12000`；並致 `auto_cycle_update` 產業循環斷料）| root 親查生產 log + `finmind_client.go:420` 在建構子內建 tracker（**per-process**）| **PR #2021** | 待 root 驗收 |
| D2 | Fubon 量能語意未定義（同標的同日少 6–11%；0050 66,000 vs 70,488 張）| k3 盤查量測；消費者以 `.Volume` 算周轉率/門檻（`cmd/backfill-industry-tree:42`、`cmd/experimental/plugin-e2e:42`）| 無 | **待量測判定**（快照 vs 累計）|
| D3 | 校準漂移偵測（生效值 vs 出廠值不可觀測；風控曾靜默由嚴→寬）| root 親查（overlay 在 bind mount；生產 `0.0108` vs repo `0.03`）| 他線 `atlas-calib-fix` + **#2017** | 在飛（不重複）|
| D4 | revert-guard 設計修正（改為 diff 衛生 WARN + evil-merge FAIL）| kimi-k3 設計審查裁定 | **#2003** | 解衝突中 |

## §3 明確錯誤清單（E）— 局部、可列舉、不需重構

| ID | 問題 | 位置 | 擁有者 | 狀態 |
|---|---|---|---|---|
| E1 | coverage 門檻空值 fail-open | `Makefile` + `ci-cd.yml` + `quality.yml` + `local-ci.sh` | 他線 `atlas-cov-fix` | 在飛（**佔用 Makefile/quality.yml**）|
| E2 | 負向證明把 exit 127/2 當「擋下了」 | 10 處 | — | ✅ 已併（#2020/#2022）|
| E3 | `timeout(124)` 只計 skipped ⇒ 掛住的檢查仍讓 `make ci` 回 0 | `Makefile` ci 段 | **他線（已登記 `FU-20260926-12`，#2028）** | **不重複** |
| E4 | pre-push 取不到 `origin/main` 即放行 4 gate | `.githooks/pre-push` | **無** | ⚠️ **與 `FU-20260926-15` 同檔 ⇒ 串行** |
| E5 | 危險指令 hook 預設 warn | `.agent-hooks/deny-dangerous.sh` | **無** | 待派（可並行）|
| E6 | `--warn-only` 使 job 不可能紅；shellcheck/frontend-smoke skip 出口 | `quality.yml` 等 | **無**（等 E1/#2003 讓出）| 待派（串行）|
| E7 | `$VAR（` 全形括號併入變數名（`set -u` 崩潰）| `check_finmind_quota.sh:65` | 他線 `atlas-shnonascii` | 在飛（不重複）|
| E8 | flaky 假紅：`WalkDir("internal")` 撞 apigateway 測試的相對 `data/` | `parameters_shadow_declarations_test.go` ↔ `register_adapters.go:401` | **無** | **待派（可並行）** |
| E9 | `sa12-negative-evidence.sh` 2 條 FAIL 且未接 CI | 同上腳本 | **無** | 待派（可並行）|
| E10 | 退役 iMac 殘留：`bin/a2a status` 永遠 offline + 30+ 處引用 | `bin/a2a`、docs、skills、a2a-dev | **`fix-E10-retired-imac`**（atlas-go PR **#2031**；a2a-dev PR 待開）→ registry **`FU-20260926-20`** | **在飛**（a2a-dev `bin/a2a` 三態探測＋45 條自測；atlas-go 20 檔；`Makefile`/`quality.yml`/`traps.md` 為禁改檔 ⇒ 殘留已登記在同 FU 的「仍存在的殘留」段） |
| E11 | `symbols_excluded` 無排除原因細分 | universe snapshot | **無** | 待派（小，可掛任一 child）|
| E12 | production `/annotate` 未收斂到 Router | `internal/llm` + dashboard | **無** | 待排（需 scoping）|

## §4 系統性稽核結論（2026-09-26，防止重複盤查）

- **`scripts/ci/` 51 支 gate：全部能以非零退出**（機械判定「有無失敗路徑」；過程中修正稽核器自身 2 個誤報：`-euo` regex、`exec` 未建模）⇒ **「閘門不能失敗」不是設計問題、不需重構**
- 風險面是 **29 支帶軟出口**（`||true`/`set +e`/`--warn-only`/`||echo`/`continue`）的 gate；已確認的 2 支已修（E2），其餘多數有明文理由
- **結論：本輪為「有界錯誤清單（§3）+ 複雜設計問題（§2）」，不需大規模重構**

## §5 明示不排（既有 backlog，不自動派工）

`#1944` inert 殘項（需 bounded 清單）、`#1756`、`#1659`

---

## §6 與 `FOLLOWUPS.md` 的 FU registry 的關係（對帳 2026-09-26，避免兩套追蹤）

**現況**：本表（D/E 分類 + 擁有者）與 `docs/operations/FOLLOWUPS.md` 的 `FU-<date>-NN`（詳細紀錄）**並存**。分工：
- **本表 = 分類/擁有者/狀態視圖**（回答「誰在做什麼、什麼沒人做」）
- **FU registry = 逐項詳細紀錄**（現象、證據、處置、殘項）
- **規則**：本表的每一項**必須**對應一個 FU 號或 PR；反之不要求（registry 可能有本表尚未收納的項）

### 已對帳（2026-09-26，main `905b4e95`，現有最大號 `FU-20260926-17`）

| 本表 | 對應 FU / PR | 備註 |
|---|---|---|
| **E3**（`make ci` timeout 124 只計 skipped）| **`FU-20260926-12`** | **他線已登記**（#2028）⇒ 本表不重複派遣 |
| E4（`.githooks/pre-push` 取不到 origin/main 即放行）| 無（但 **`FU-20260926-15` 也在同一檔**：pre-push 缺 host binary 新鮮度閘門）| ⚠️ **同檔衝突 ⇒ 必須串行**（先讓 FU-15 落地或用同一 PR）|
| E8（flaky：`WalkDir("internal")` × apigateway 相對 `data/`）| 無 | `FU-20260926-17`（fubonproxy）是**另一個** flaky，**不重複** |
| E2（負向證明假綠）| ✅ #2020 / #2022 | 已結案 |
| D3（校準漂移）| `FU-20260926-16`（季節校準污染源）+ #2017 | 相關但不同面 |

### `FU-` 號段分配規則（**強制，修正本表 §1 的過時配置**）

> **新增 FU 前必須先 `git fetch origin main` 並取當前最大號 +1**；**不得**用先前分配的號（本 session 已兩次撞號：`-10`、以及本表的 `-12/-13/-14` 在被使用前就被他線取走）。
> 並在 **同一 PR 內**同時更新本表 §3/E 與 registry，避免兩套號不一致。

| lane / child | 分配號 |
|---|---|
| `fix-E8-E9-flaky-sa12` | **FU-20260926-18（E8）、-19（E9）** |
| `fix-E10-retired-imac` | **FU-20260926-20**（原配置 -14 已被 Telegram token 取走）|
| 其他 lane | 各自 fetch 後取 max+1 |
