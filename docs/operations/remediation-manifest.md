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
| E4 | pre-push 取不到 `origin/main` 即放行 4 gate | `.githooks/pre-push` | ✅ **#2049**（已併）| 修法＝fail-closed（fetch 失敗 ⇒ exit 1 ＋ 列出「哪些閘門為何不能跑」＋ 原始錯誤 ＋ `--no-verify` 提示）＋ 新增 **Gate 0b**（`check-binary-freshness.sh --host-only --diff-base`：**完全不碰 docker**、只判 `bin/atlas`＋`bin/atlas-mcp`、純 docs push 不擋、無 `bin/` 的 clone 軟跳過）。**原描述低估影響**：`exit 0` 另跳過 Gate 1c（frontend dist）、Gate 2（HEAD==origin/main）、Gate 3（zero-diff）。**併同修掉一個「無聲死亡」**：`test-binary-freshness-guard.sh` 在 `set -e` 下以 rc=1、**stdout/stderr 皆 0 bytes** 結束 ⇒ 操作者只看到「ci-gate FAILED」而無原因、唯一出路是 `--no-verify`（不可行動的假紅）|
| E5 | ~~危險指令 hook 預設 warn~~ **實為「死守門」**：`.claude/settings.json` 只有 `SessionStart`、**無 `PreToolUse`** ⇒ **沒有任何機制自動呼叫** `deny-dangerous.sh`（唯一 PreToolUse 在 **gitignored** 的 `.claude/settings.local.json`）；且該腳本用 `${CHECK,,}`（bash 4）⇒ macOS bash 3.2 **對每個指令誤判** | `.agent-hooks/` ＋ `.claude/settings.json` | **#2048** | ✅ **已併**（`fe779473`）：tracked `settings.json` 加 `PreToolUse`→薄 adapter（pattern 邏輯無第二份）、bash 3.2 語法修為 `tr`、`tests/scripts/test-agent-hook-wiring.sh`（11 組契約）納入 `make ci-gate`；**維持 warn 為預設**（見 E23）|
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
| E18 | `.github/workflows/quality.yml` 的 `revert-guard` job 註解/step 名稱仍是 v1（被否證的）框架 | `quality.yml` | **#2046** | ✅ **已併**（15:36Z，`2404dbe2`）：root 複查 main 上兩句 v1 字串 **0/0 命中**；spec 殘留條目已由 **#2050** 更新 |
| E19 | **孤兒腳本群（逐支溯源後處置）** | `scripts/` | ✅ **#2052**（已併）| **刪 10 支**（6 支無殘餘價值＋`verify-manifest.sh`、soak 鏈 3 支）；**新增 tracked `agent-guard` wrapper**（非 symlink —— 實驗證明 rel/abs symlink 都使 `BASH_SOURCE` 推導的 `REPO_ROOT` 高錯一層，wrapper `exec` 真檔才正確；`.gitignore` 條目同時移除）；**保留 2**（`sync-darwinian.sh`＝`local-deploy.md:82` 部署強制前置、`cleanup-manifests.sh`＝4 份現行文件在指示）；**待 owner 決定 2**（`daily-twse-fetch.sh` 併入 E25、`scripts/hooks/pre-commit` 見 **E31**）。連帶編輯 `deploy-staging.sh`（4 步→3 步，移除 soak 步驟與變數；`test-binary-freshness-guard.sh` 重跑 rc=0 PASS）。跨機前置查核已完成（a2a-dev）＋ 生產機 **cron daemon 未執行** ⇒ root crontab 殘留實質封閉 |
| E20 | ~~生產↔repo watchdog 不一致~~ **已解除**（操作經驗保留）：修好前 Mac Mini（`424944ce`、85 行）≠ repo 正本（`70ee1a45`、104 行）⇒ `make imac-watchdog-diff` **exit 2** | 生產主機 + `scripts/ops/imac-container-watchdog.sh` | —（已解除）| ✅ root 於 main `b1fc524d` 複測：`✅ 一致（無漂移）` **rc=0**、兩側 sha256 均 `cdb4a7d6`（已由跨機器 `make imac-watchdog-install` 收斂）。**診斷價值保留**：當時的 exit 2 被文件（`#2043`/`#2044`「兩個 target 都可直接跑」）讀成「target 壞」，實為檢查**正確地**報漂移；「可用」≠「必 exit 0」——本項為該區別的實證 |

| E21 | **CLI 靜默忽略未知子命令** ⇒ `daily-maintenance` 三 job 跑模擬 | `cmd/atlas/main.go` ＋ `.github/workflows/daily-maintenance.yml` | ✅ **#2053**（已併，`88f6b33d`）| CLI 對未知位置參數 **exit 2 ＋ usage**（bootstrap 前拒絕）；三個 job **改用真實來源**＝daemon 公開端點 `GET https://atlas.goluck.uk/api/dashboard/task-liveness` ＋ 新 gate `check_task_liveness.{sh,py}`。**保留 job（不退役）** 的理由：daemon 自身告警與被監控對象同主機同生共死，**GitHub runner 是唯一局外觀察者**。root 複驗：`test-check-task-liveness.sh` **28 passed**、`./atlas-go weights adjust --apply` ⇒ **rc=2 ＋ 明確訊息**、端點 **114 tasks／stale_count=0**、四個任務全 found |
| E22 | **`make ci-full` 假紅產生器**：coverage 步驟用**硬編共用 `/tmp` 路徑**（`Makefile:1042-1046` `/tmp/atlas-ci-full-coverage*.out`）＋ `rm -f` ⇒ **同機多 worktree 併發互踩** | `Makefile` | **由 `fix-prepush-gates`／sibling lane 處理**（`Makefile` coverage 段名義由 E1 佔用 ⇒ **串行**）| root 複驗存在；同族於 E4/FU-15（dev 工具鏈的假訊號）。`#2053` 的 CI 亦曾因 `golangci-lint` 共用快取回放鄰居 worktree 的 issue 而假紅 ⇒ 同一類問題 |
| E23 | **guard 切 enforce 會誤擋**（三面，皆 root 複驗）：pattern 4 只因指令含 `secret` 一字（`grep -rn secret internal/` 這種正常搜尋就中）；pattern 8 在 `ATLAS_ENV=production` 擋 `docker compose build/up`（**而部署腳本正是此組合**）與 `go test`/`make test` | `.agent-hooks/deny-dangerous.sh` | **無**（需 owner 決定是否收緊）| 現行決策：**維持 `warn` 為預設**（`#2048`）。收緊前置＝① pattern 4 改成「**目標**是 secret 檔」才判 ② 部署腳本與 `make ci-*` 列白名單 |
| E24 | **staging soak 鏈（已廢棄流程仍在跑）** | `scripts/` ＋ 開發機 LaunchAgent | ✅ **#2052**（已併）| 2026-07-15 的 7 天 post-merge soak，期限已過、**2026-08-15 專案自宣告「無 staging」**、Day-7 收尾未執行（`post-soak-cleanup.sh` 一生只跑過 `--dry-run`）。**root 已停本機 LaunchAgent**（`launchctl bootout` rc=0；停前 `runs=39838`、**每 60 秒約 5 次重生**、log 133 MB、報告停於 09-17）＋ plist 改名 ＋ log 歸檔。**真缺口另立**：4 條**語意值**斷言（`resonance_dir` 非空／`.predictions≥5`／scheduler≥30 含兩具名任務／detector scan 帶 key）在 CI 與生產**皆 0 覆蓋** |
| E25 | **replay 每日路徑寫入非交易日幻影列** | `cmd/daily-replay-sync`、`cmd/fetch-historical` | ✅ **#2057**（已併，`a3d54321`）| 兩段修：① 止血＝非交易日不抓不寫（`main.go:186 IsTaiwanTradingDay`）② 治本＝改用帶日期 `MI_INDEX?type=ALLBUT0999&date=`（`main.go:211 GetQuotesForDate`）＋回應日期守門；並修 `fetch-historical` 的 `tables` envelope（原本每天 0 筆卻 exit 0）。**root 複驗**：守門位置正確、**MI_INDEX `20260917` 與舊路徑寫出的 CSV 逐欄數值完全相同**（0050/2317/2330 volume+OHLC diff=0）⇒ 換來源零數值差異。**資料面仍待清理**：生產 CSV 既有 396 列（9 天×44 檔）須**部署後**跑 `clean-replay-weekends -output "" -rewrite`；`auto_backfill` 清理後恢復（`start<=end` 才會轉檔）。同族缺陷見 **E29** |
| E26 | **生產監控：repo 規則 ≠ 生效規則**。Prometheus 實際載入 **10 組／29 條**：`Universe` **6 條已載入**（`AtlasUniverseRankedZero`…）、**`Calibration` = 0**（容器內有 `calibration_freshness_alerts.yml` 卻未載入 ⇒ **#2016 的校準新鮮度監控在生產實際無效**）、`Replay` = 0；`AtlasGoTargetDown.runbook_url` 仍指已退役主機 | 生產 Prometheus 設定（非 repo）| **交 a2a-dev**（載入面由他們查）| root 複驗：`curl localhost:9090/api/v1/rules?type=alert` ⇒ 10 組/29 條；`Universe` 6 條**已載入 ⇒ 週一 #1995 驗收可行**（此點為正面確認）|
| E27 | **`scripts/verify-manifest.sh` 假綠 ＋ 致命前提** | 已刪 | ✅ **#2052**（已併）| root 複驗假綠：`done` ＋ Notes 空 ⇒ 仍 `OK` exit 0（awk `gsub` 只給 2 參數 ⇒ 改 `$0` 非 `$2` ⇒ status 永遠 `" done "` ⇒ 每列 continue）。致命前提＝讀 gitignored `.omo/`。**已刪檔 ＋ 修 2 處失效文件引用**（`documentation-standard.md:97`、`docs/manifests/README.md:11-20`）|
| E28 | **`internal/taiwanholidays` 日曆缺休市日** | `internal/taiwanholidays` ＋ `internal/monitoring/universe_scheduler.go` | ✅ **#2054**（已併，`db7fdb44`）| 已補關鍵日期（**09-28 教師節**、12-25 行憲紀念日 等）＋ **weekly 加 `weekly_skip_holiday` 閘門** ＋ daily 改用單一來源 `marketdata.IsTaiwanTradingDay`。**root 行為測試**（臨時 worktree 跑 `IsTradingDay`，非字串比對）：09-28／12-25／09-25／10-09／10-26／04-03／05-01／06-19 **皆正確判休市**；**仍缺 3 個過去日期**（`2026-01-02`、`02-11`、`02-23` 仍判交易日）。**09-29（二）與 10-05（一）皆為交易日** ⇒ 驗收時點成立 |
| E29 | **replay 路徑三個同型「靜默成功」缺陷**（#2057 只登記未動）：① `checkReplayHealth` 讀**最後一行**（非最新資料日）② `MarketVolumeProvider` 用**請求日**當資料日 ③ `degraded` 不自我升級 | `cmd/daily-replay-sync`、`internal/marketdata` 等 | **無** | 待派（三者皆屬「看似正常、實則判讀失真」家族；與 E25 同源）|
| E30 | **`git push --delete <branch>` 被 pre-push Gate 3 誤擋**：Gate 3（zero diff vs origin/main）未排除 delete refspec ⇒ 刪遠端分支必須 `--no-verify` | `.githooks/pre-push` | **無** | 待派（刪分支沒有內容可守 ⇒ Gate 3 應跳過 delete refspec。root 靜態複驗：Gate 3 段（`:149`）無 `delete`/refspec 例外；child 有實際重現）|
| E31 | **`scripts/hooks/pre-commit` = DEPRECATED `exit 0` stub、0 呼叫端**（與 #2052 已刪的 10 支同類）| `scripts/hooks/pre-commit` | **無** | 待 owner 決定（`#2052` child 刻意未刪：不在核准清單內；刪它**不需**連帶改任何東西，`developer-guide.md` 已不再指向它）|
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

### 已對帳（2026-09-27 最終：main `14a3da03`，現有最大號 `FU-20260926-30`）

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
| **E5**（死守門：`deny-dangerous` 從未被自動呼叫）| ✅ **#2048**（`fe779473`；PR 含 bash 3.2 修正）| 維持 `warn` 預設的誤擋面另立 **E23** |
| **E18**（quality.yml v1 文案）| ✅ **#2046**（`2404dbe2`）＋ **#2050** 更新 spec 殘留 | root 複查 main：v1 字串 0/0 |
| **E4**（pre-push fail-open）＋ `FU-20260926-15` | ✅ **#2049**（已併）| 同檔 ⇒ 同一 PR 處理 |
| **E21**（CLI 靜默忽略未知子命令）| **#2053**（armed）| root 複驗：公開 task-liveness 200／114 tasks／stale=0；CI run `36255705561` 四 job 全 success |
| **E22**（`Makefile` coverage 共用 `/tmp` 假紅）| 無（**串行**：`Makefile` coverage 段由 E1 名義佔用）| 同族於 E4/FU-15 |
| **E23**（guard 切 enforce 的誤擋面）| 無（需 owner 決定）| 現行決策＝維持 `warn` |
| **E24**（staging soak 鏈）| **`delete-dead-scripts`（在飛）** | root 已停本機 LaunchAgent；真缺口（4 條語意斷言）另立 |
| **E25**（replay 幻影列）| **`fix-replay-dated-source`（在飛）** | 生產清理須在修復部署後執行 |
| **E26**（Calibration 告警未載入）| **交 a2a-dev** | 正面確認：`Universe` 6 條**已載入** ⇒ 週一驗收可行 |
| **E27**（`verify-manifest.sh` 假綠）| **`delete-dead-scripts`（在飛）** | 判定＝刪除＋修 2 處文件引用 |
| **E28**（taiwanholidays 缺 9 個休市）| 無（**刻意延後**至週一驗收後）| 會改變 `IsTradingDay` 行為 |
| **E21–E28 的登記** | **`FU-20260926-27`** | 本表 §3 與 registry **同 PR** 更新 |
| **E19**（孤兒腳本群）| ✅ **#2052**（`14a3da03`）| 刪 10 支 ＋ `agent-guard` 改 **tracked wrapper**；保留 2、待決 2 |
| **E21**（CLI 未知子命令）| ✅ **#2053**（`88f6b33d`）| root 複驗 28 hermetic ＋ 端點 114 tasks／stale=0 |
| **E24**（soak 鏈）| ✅ **#2052**（刪除）＋ **root 已停本機 LaunchAgent** | 真缺口（4 條語意斷言）另立 |
| **E25**（replay 幻影列）| ✅ **#2057**（`a3d54321`）| root 複驗數值逐欄相同；**資料面清理待部署後** |
| **E27**（verify-manifest 假綠）| ✅ **#2052** | 刪檔 ＋ 修 2 處文件引用 |
| **E28**（日曆缺休市）| ✅ **#2054**（`db7fdb44`）| root 行為測試：關鍵日期正確；**仍缺 3 個過去日期** |
| **E29/E30/E31**（本次新增）| **`FU-20260926-30`** | 三項皆待派／待 owner 決定 |
| **FU-20260926-15**（pre-push 缺 host binary 閘門）| ✅ **#2049**（Gate 0b）| 本 registry 同 PR 標 `done` |

### `FU-` 號段分配規則（**強制，修正本表 §1 的過時配置**）

> **新增 FU 前必須先 `git fetch origin main` 並取當前最大號 +1**；**不得**用先前分配的號（本 session 已兩次撞號：`-10`、以及本表的 `-12/-13/-14` 在被使用前就被他線取走）。
> 並在 **同一 PR 內**同時更新本表 §3/E 與 registry，避免兩套號不一致。

| lane / child | 分配號 |
|---|---|
| `fix-E8-E9-flaky-sa12` | **FU-20260926-18（E8）、-19（E9）** |
| `fix-E10-retired-imac` | **FU-20260926-20**（原配置 -14 已被 Telegram token 取走）|
| `fix/20260926-dead-gate-closure` | **FU-20260926-23**（E16）|
| **`docs/20260926-manifest-e-status`（root，manifest 收尾）** | **`FU-20260926-25`** |
| **`docs/20260927-manifest-status-sync`（root，狀態同步）** | **`FU-20260926-27`** |（E21–E28 登記）|
| **`docs/20260927-final-sync`（root，最終同步）** | **`FU-20260926-30`** | E4/E19/E21/E24/E25/E27/E28 結案 ＋ 新增 E29/E30/E31 ＋ FU-15 done |
| 其他 lane | 各自 fetch 後取 max+1 |
