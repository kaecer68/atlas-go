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
| D4 | revert-guard 設計修正（改為 diff 衛生 WARN + evil-merge FAIL）| kimi-k3 設計審查裁定 | **#2003** | ✅ **已併**（#2003，12:25Z）；root 併入後複驗：`Makefile` 守門 `check_followups_unique_ids`=1、`check_revert_guard.py` 的 `merge-tree` 引用=13、`traps.md`=329 行 ≤330、evil-merge 負向證明實跑 exit 1 |

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
| E8 | flaky 假紅：`WalkDir("internal")` 撞 apigateway 測試的相對 `data/` | `parameters_shadow_declarations_test.go` ↔ `register_adapters.go:401` | **本線 `fix/20260926-flaky-and-sa12`** | ✅ **已併**（#2036，13:24Z）；root 解衝突（取分支版）後複驗：`go test ./internal/config/ ./internal/apigateway/` 綠且跑完 **repo 樹不留 `internal/apigateway/data`** |
| E9 | `sa12-negative-evidence.sh` 2 條 FAIL 且未接 CI | 同上腳本 | **本線 `fix/20260926-flaky-and-sa12`** | ✅ **已併**（#2036）；`scripts/ci/check_sa12_negative_evidence.sh` root 實跑 **14/14 PASS** |
| E10 | 退役 iMac 殘留：`bin/a2a status` 永遠 offline + 30+ 處引用 | `bin/a2a`、docs、skills、a2a-dev | **`fix-E10-retired-imac`**（atlas-go PR **#2031**；a2a-dev PR 待開）→ registry **`FU-20260926-20`** | ✅ **已併**（#2031，11:11Z）；殘留（2 個 watchdog 操作入口）登記於 `FU-20260926-20`；root 複驗 `100.68.42.72`=0 命中、`Makefile` 取值已改 `kaecer@kmacmini` |
| E11 | `symbols_excluded` 無排除原因細分 | universe snapshot | **無** | 待派（小，可掛任一 child）|
| E12 | production `/annotate` 未收斂到 Router | `internal/llm` + dashboard | **無** | 待排（需 scoping）|
| E16 | **死 gate**：`scripts/verify-sector-allocation-closure.sh` 依賴的 manifest 已於 #1255 移出 `docs/`（現於 gitignored `.omo/`）＋ `check()` 的 `eval` 被移除（#1250）⇒ 今 `exit 2`、**呼叫端 0** ⇒ **明示停用為 no-op** | `scripts/verify-sector-allocation-closure.sh`；附帶誠實化 `cmd/experimental/sector-allocation-closure-preflight/main.go` 的假宣稱 | **`fix/20260926-dead-gate-closure`（本 PR）** | ✅ **已併**（#2037，11:24Z）；腳本明示停用（`⛔ 已停用（DISABLED）`、rc=0），依賴檔不可回復的理由見 `FU-20260926-23` |
| E13 | ~~`Makefile` `IMAC_HOST ?= kk@kimac` ⇒ `make imac-watchdog-diff` 必失敗~~ **已修** | `Makefile:105-108` | ✅ **#2041**（13:42Z）| 修法：`IMAC_HOST ?= kaecer@kmacmini`、`WATCHDOG_DST := /Users/kaecer/bin/…`（`#2043`/`#2044` 補文件）。root 實測舊因（`Could not resolve hostname kimac`）已消失。⚠️ **本表原記「隨 #2031 修」係歸因錯誤**，2026-09-26 由 root 以 `git log -- Makefile` 核對更正 |
| E14 | ~~`traps.md` / `quality.yml` 未掃退役主機殘留~~ **複查無殘留** | 同左 | — | ✅ 不需派工（root 複查 2026-09-26：兩檔 `kk@kimac|iMac` **0 命中**）|
| E15 | **活設定殘留（非 repo）**：`~/.prime/agent/models.json:140` 的 provider 名仍叫 `kimac` | 工作站設定（非 repo）| **無** | 待決（低風險：其 `baseUrl` 已是 `http://kmacmini:4000/v1` ⇒ **僅名稱歷史債、路由正確**）。⚠️ 原描述的懸空 symlink `~/bin/imac-recover` **複查不存在、不可重現 ⇒ 不登記為事實** |
| E17 | **排程空轉**：`scripts/darwinian_adjust.sh` 自述 `DEPRECATED` stub（核心計算註解於 L101），**實跑 rc=1**，但 `docker-compose.yml:453` 仍以 `CRON_COMMAND=/app/scripts/darwinian_adjust.sh --apply`（容器 `atlas-cron-darwinian`，`0 9 * * *`）**實際排程** | `scripts/darwinian_adjust.sh` + `docker-compose.yml:453` | **無** | 待派（root 2026-09-26 實查：stub header + `rc=1` + 呼叫端 grep；處置二選一：移除排程或讓 stub 明確 no-op+log）|
| E18 | `.github/workflows/quality.yml` 的 `revert-guard` job **註解與 step 名稱仍是 v1（被否證的）框架**：「照現狀合併就會回退」×1、「a stale branch deleting a shared asset must be blocked」×1；行為正確（僅呼叫兩支腳本）| `quality.yml` | **無**（等 E6/`atlas-cov-fix` 讓出）| 待派（純文字，−0 行為風險；root 複查：兩句各 1 命中）|
| E19 | **孤兒腳本群**：10 支在 `Makefile`/`.github/workflows`/`docker-compose*`/`scripts/` 中 **0 個可叫用引用**（`verify-atlas.sh`、`coverage.sh`、`daily-twse-fetch.sh`、`install-soak-automation.sh`、`reflexivity_report.sh`、`sync-darwinian.sh`、`prism_manage.sh`、`spawning_manage.sh`、`generate_replay_data.sh`、`cleanup-manifests.sh`）；`verify-manifest.sh` 唯一引用來自**同樣 0 引用的** `verify-atlas.sh`（孤兒互叫）⇒ 實質死碼 | `scripts/` | **無** | 待決（保留或刪除需 owner 決定；root 2026-09-26 實查引用數）|
| E20 | ~~生產↔repo watchdog 不一致~~ **已解除**（操作經驗保留）：修好前 Mac Mini（`424944ce`、85 行）≠ repo 正本（`70ee1a45`、104 行）⇒ `make imac-watchdog-diff` **exit 2** | 生產主機 + `scripts/ops/imac-container-watchdog.sh` | —（已解除）| ✅ root 於 main `b1fc524d` 複測：`✅ 一致（無漂移）` **rc=0**、兩側 sha256 均 `cdb4a7d6`（已由跨機器 `make imac-watchdog-install` 收斂）。**診斷價值保留**：當時的 exit 2 被文件（`#2043`/`#2044`「兩個 target 都可直接跑」）讀成「target 壞」，實為檢查**正確地**報漂移；「可用」≠「必 exit 0」——本項為該區別的實證 |

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

### 已對帳（2026-09-26 更新：main `12edcec0`，現有最大號 `FU-20260926-25`）

| 本表 | 對應 FU / PR | 備註 |
|---|---|---|
| **E3**（`make ci` timeout 124 只計 skipped）| **`FU-20260926-12`** | **他線已登記**（#2028）⇒ 本表不重複派遣 |
| E4（`.githooks/pre-push` 取不到 origin/main 即放行）| 無（但 **`FU-20260926-15` 也在同一檔**：pre-push 缺 host binary 新鮮度閘門）| ⚠️ **同檔衝突 ⇒ 必須串行**（先讓 FU-15 落地或用同一 PR）|
| E8（flaky：`WalkDir("internal")` × apigateway 相對 `data/`）| 無 | `FU-20260926-17`（fubonproxy）是**另一個** flaky，**不重複** |
| E2（負向證明假綠）| ✅ #2020 / #2022 | 已結案 |
| **E16**（死 gate：`verify-sector-allocation-closure.sh` 明示停用）| **`FU-20260926-23`** | 本表 §3 與 registry **同 PR** 更新 |
| D3（校準漂移）| `FU-20260926-16`（季節校準污染源）+ #2017 | 相關但不同面 |
| **D4**（revert-guard 重設計）| ✅ **#2003**（12:25Z 併入）| root 複驗：守門=1、`merge-tree`=13、`traps.md`=329 行、evil-merge 負向證明 exit 1 |
| **E8/E9**（flaky + sa12）| ✅ **#2036**（13:24Z 併入）| root 解衝突（第二次為 3 檔；`verify-sector-allocation-closure.sh` 取 main 停用版）|
| **E10**（退役 iMac 殘留）| ✅ **#2031**（11:11Z）＋殘留 `FU-20260926-20` | A 類已改 Mac Mini；B 類（2 個 watchdog 入口）待另一條 lane |
| **E13**（`IMAC_HOST` 必失敗）| ✅ **#2041**（13:42Z；`#2043`/`#2044` 補文件）| ⚠️ 原記「隨 #2031 修」係**歸因錯誤**，已更正 |
| **E16**（死 gate）| ✅ **#2037**（11:24Z）＋ **`FU-20260926-23`** | 明示停用＋登記重啟條件 |
| **E17/E18/E19/E20**（本次新增）| **`FU-20260926-25`** | 本表 §3 與 registry **同 PR** 更新 |
| **E20**（watchdog 漂移）| ✅ 已解除（root 於 `b1fc524d` 複測 rc=0）| 更正與複測記於 **`FU-20260926-26`** |

### `FU-` 號段分配規則（**強制，修正本表 §1 的過時配置**）

> **新增 FU 前必須先 `git fetch origin main` 並取當前最大號 +1**；**不得**用先前分配的號（本 session 已兩次撞號：`-10`、以及本表的 `-12/-13/-14` 在被使用前就被他線取走）。
> 並在 **同一 PR 內**同時更新本表 §3/E 與 registry，避免兩套號不一致。

| lane / child | 分配號 |
|---|---|
| `fix-E8-E9-flaky-sa12` | **FU-20260926-18（E8）、-19（E9）** |
| `fix-E10-retired-imac` | **FU-20260926-20**（原配置 -14 已被 Telegram token 取走）|
| `fix/20260926-dead-gate-closure` | **FU-20260926-23**（E16）|
| **`docs/20260926-manifest-e-status`（root，manifest 收尾）** | **`FU-20260926-25`**（E17/E18/E19/E20）|
| 其他 lane | 各自 fetch 後取 max+1 |
