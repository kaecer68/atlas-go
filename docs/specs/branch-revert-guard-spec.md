# 落後分支語意回退 — 靜態檢查規格

| 項目 | 內容 |
|---|---|
| 文件角色 | 「PR 分支落後 `main` ⇒ diff 把**其他 PR 已合併的改動顯示成刪除**」的**自動閘門**規格：判定規則、severity 分級、allowlist 用法、誤報出路、以及本檢查**做不到**什麼 |
| 狀態 | v1（2026-09-26，issue [#1993](https://github.com/kaecer68/atlas-go/issues/1993)） |
| 實作 | [`../../scripts/ci/check_revert_guard.py`](../../scripts/ci/check_revert_guard.py)（純 stdlib Python3）＋薄殼 [`../../scripts/ci/check_revert_guard.sh`](../../scripts/ci/check_revert_guard.sh) |
| 機器 allowlist | [`../../scripts/ci/revert-guard-allowlist.json`](../../scripts/ci/revert-guard-allowlist.json)（每筆 `reason` 必填） |
| 回歸測試 | [`../../tests/scripts/test-revert-guard.sh`](../../tests/scripts/test-revert-guard.sh)（hermetic：tempdir 內真 git repo，不連網、不碰本 repo、不碰 production） |
| CI 接入 | [`.github/workflows/quality.yml`](../../.github/workflows/quality.yml) 的 `revert-guard` job（無 path filter；含負向證明） |
| 本地接入 | `make revert-guard`、`make revert-guard-selftest`、`make ci-gate`（因此 `make pre-push` 涵蓋） |

## 1. 為什麼要做

GitHub 的合併性檢查回答的是「**能不能**自動合併」，不是「合併後**會不會刪掉別人的東西**」。
PR 分支落後 `main` 時：

- `gh pr view --json mergeable` 回 `MERGEABLE`（沒有文字衝突）；
- 但 `git diff origin/main <PR head>` 把 `main` 上**其他 PR 最近的改動**顯示成「刪除」——
  分支的樹還是分岔當時的舊版本；
- 於是「Files changed」看起來像本 PR 刪了別人的修正，而 **squash／rebase merge 會把
  `base..head` 當成一個 patch 套到 `main` 上**，那些「假刪除」就真的被套用 ⇒ 回退他人已合併的修正。

2026-09-26 單一 session 內實證 5 個 PR（全部靠人工 `git diff --numstat` + `git log origin/main -- <file>`
交叉比對才發現，再以 `gh pr update-branch` 修正）：

| PR | 被顯示成刪除的他人改動 |
|---|---|
| #1974 | `monitoring/alertmanager.yml`（−37）|
| #1979 | `.github/workflows/pr-base-guard.yml`（−92）、`quality.yml`（−66）|
| #1990 | `internal/monitoring/metrics_bridge.go`（−41）、`internal/monitoring/universe_scheduler.go`（−62）|
| #1991 / #1994 | `internal/monitoring/metrics_bridge.go`（−41）、`metrics_bridge_test.go`（−136）、`.claude/skills/.../SKILL.md`（−41）|

人力不可靠：要看兩條 git 指令、再逐檔判斷「這筆刪除是不是別人的改動」。本檢查把這件事變成每次 PR
自動執行的閘門。

## 2. 判定規則

輸入是兩個 ref：`base`（`main` tip）與 `head`（PR 分支 tip）。全部判定都由 git 產生，
**不讀 GitHub API、不依賴網路**。

### 2.1 第一階段：候選 = 「純刪除」或「整檔刪除」

對 `git diff --name-status --no-renames <base> <head>` 的每個檔案：

| 候選種類 | 條件 |
|---|---|
| `file-deleted` | status = `D`（整個檔案在 head 不存在）|
| `pure-deletion` | numstat `added == 0 && deleted > 0`（檔案還在，內容被淨刪除）|

`--no-renames` 是刻意的：rename 判定是啟發式（相似度門檻），會讓路徑在 diff 兩側對不起來；
本檢查要逐路徑判讀「這條路徑的內容是不是被淨刪除」，寧可看到「A 刪、B 增」兩個獨立項。

`added > 0` 的改動**不列入候選**（可能真的動了手），這是已知邊界，見 §5.1。

### 2.2 第二階段：候選必須是「落後」造成的

> `git rev-list --no-merges <base> ^<head> -- <path>` 非空

`base ^head` = 可由 `base` 抵達、但 `head` 沒有的 commit。非空代表「**`main` 上該檔有本分支沒有的
改動**」，也就是「該檔在 `main` 的最近一次改動不是本 PR 引進的」。

為什麼這條等價於「落後」而不是「本 PR 自己刪的」：分支若已含 `main` 的該次改動，diff 就不會是
純刪除（先有內容，才談淨刪除）；反過來，分支完全沒碰該檔時，diff 的刪除**必然**全部來自落後。
先取 `--no-merges`（回報真正的內容 commit，不報 merge 包裝），沒有才放寬。

### 2.3 第三階段：severity 由「檔案是不是共用資產」決定

| 類別 | 範圍 | 結果 |
|---|---|---|
| 共用資產 | `.github/`、`monitoring/`、`configs/`、`docs/reference/`、`scripts/ci/`、`Makefile`、`docker-compose*.yml`／`*.yaml`（basename 判定） | **FAIL**（exit 1）|
| 其他 | 其餘所有路徑 | **WARN**（不讓 CI 紅燈，但列出檔案、`main` 上該檔最後的 commit、修法）|

為什麼共用資產要 FAIL：這些檔案是**唯一真相**或跨 PR／跨機器的介面（監控設定樹、CI 閘門本身、
參數、規範文件、部署 compose）。被落後分支回退 = 靜默改設定或改閘門，代價遠高於一般程式碼。
清單刻意用「目錄前綴 + 具名檔 + basename glob」明列（不用 `**` 遞迴），要擴大範圍必須改程式常數
並更新本節，避免變成「誰都能豁免」的模糊地帶。

WARN 不紅燈的理由與 inert baseline 的 stale 同源：跨 PR 耦合——一般程式碼的假刪除已經在 diff 上
看得見，讓它紅燈只會逼人用 allowlist 掩蓋。`--strict` 可把 WARN 升級成失敗（CI 的負向證明用它）。

### 2.4 信心訊號 `branch_touched_file`（只影響訊息，不影響 severity）

以 merge-base 的 blob 對照 head 的 blob：

- **False** ⇒ 分支根本沒碰這個檔 ⇒ diff 裡的刪除 100% 是 `main` 上別人近期的改動（經典案例）；
- **True** ⇒ 分支動過該檔（例如真的刪掉它）⇒ 仍必須先 update-branch（否則會回退 `main` 的改動），
  但讀者要自己確認「這是不是有意刪除」。

輸出會照這兩種情況給不同句子，讓 `gh pr update-branch` 與「真的想刪」兩條路不會被混在一起。

### 2.5 假綠防線：拒絕合成 merge ref

GitHub 在 `pull_request` 事件預設 checkout 的是合成 merge ref（`refs/pull/<N>/merge`），
它的**第一個 parent 就是 base tip**，樹已經含 `main`。拿它當 `head` ⇒ diff 永不含「落後的假刪除」
⇒ 檢查會**永遠 PASS**。這是本檢查唯一可能出現假綠的路徑，所以：

- `head` 是 merge commit 且 `parents[0] == base` ⇒ **拒絕執行**（exit 2），並要求傳真正的 PR head；
- CI 明示取兩個 ref：`refs/remotes/origin/<base>` 與 `refs/pull/<N>/head`（見 §3）。

另外 `base` 解不到時**fail-closed**（exit 2，不靜默跳過）；無 remote 的環境必須明示
`--allow-missing-base`，而且輸出會說清楚「這不是通過」。

## 3. 執行與接入

```bash
make revert-guard                                  # 自我測試 + 檢查（base=origin/main, head=HEAD）
make revert-guard-selftest                         # 只跑 hermetic 回歸測試
bash scripts/ci/check_revert_guard.sh --json       # 機器可讀
bash scripts/ci/check_revert_guard.sh --base refs/remotes/origin/main --head refs/remotes/origin/pr-head
python3 scripts/ci/check_revert_guard.py --help
```

exit code：

| code | 意義 |
|---|---|
| 0 | PASS（可能含 WARN；`--strict` 時 WARN 也算失敗）|
| 1 | 有 FAIL（共用資產被回退），或 allowlist 有未填理由的條目 |
| 2 | 用法／環境錯誤：`base`／`head` 解不到、allowlist 格式錯誤、`head` 是合成 merge ref |

CI（`.github/workflows/quality.yml` 的 `revert-guard` job，**無 path filter**，所以 docs- 或
scripts-only 的 PR 也會跑）：

1. `actions/checkout`（`fetch-depth: 0`）；
2. 明示抓兩個 ref：`refs/heads/<base>` 與 `refs/pull/<N>/head`（**不用**預設的 merge ref，理由見 §2.5）；
3. 跑 hermetic 回歸測試；
4. 跑檢查（gate）；
5. **負向證明**：用真的 repo 歷史造一個落後分支——從「`main` 上最後一次動共用資產的那個 commit 的
   parent」開分支，再刪掉該 commit 動過的共用資產檔 ⇒ 檢查必須 exit 1。
   為什麼要這步：hermetic fixture 只證明程式邏輯，這步證明**CI 的 ref 解析與真 repo 歷史**也能擋
   （與 `secret-scan` / `monitoring-single-source` job 的 negative proof 同一套路）。

效能（2026-09-26 量測，MacBook）：**0.05–0.2s**（含 Python 啟動；每個候選檔 2–3 次 git 呼叫，
候選數通常只有個位數）。設計上 diff 只跑兩次（`--name-status`、`--numstat`），不對每個檔案掃 repo。

## 4. allowlist 與誤報出路（理由必填）

### 4.1 三條出路

1. **update-branch（正解）**：`gh pr update-branch <PR>`，或本地
   `git fetch origin main && git merge origin/main`（`git pull --ff-only`）。改完重跑，假刪除消失。
2. **先對齊、再重做刪除**：若本 PR 真的要刪掉該檔，先 update-branch，再把刪除重新套用一次
   （這樣 `main` 上別人的改動不會被一起帶走）。
3. **allowlist（例外）**：以上都不可行時才登錄，並寫清楚理由。

### 4.2 格式

`scripts/ci/revert-guard-allowlist.json`：

```json
{
  "version": 1,
  "entries": [
    {
      "path": "monitoring/rules/legacy.yml",
      "reason": "本 PR 是在對齊 main 後才刪除此檔；main 上的新改動已由 #2000 併入，刪除是有意的",
      "ticket": "#1993",
      "added": "2026-09-26"
    }
  ]
}
```

- `path`：精確路徑或 `fnmatch` glob（例 `docs/reference/*`）。
- `reason`：**必填**，空白或以 `TODO` 開頭 ⇒ 檢查失敗（`allowlist-reason-missing`，exit 1）。
  寫到「讀者能自己驗證」的程度（含 PR 號／機制／為什麼不是落後）。
- `ticket`、`added`：選填，方便日後稽核排序。
- **stale 語意**：不再命中的條目只印警告、**不讓 CI 紅燈**（跨 PR 耦合：不該因為別人把檔案接好、
  而你忘了刪一行而被擋）。stale 清單由維護者人工複核後刪除。
- allowlist 不會讓檢查「失效」：命中條目會以 `↷ [allowlist]` 明列在輸出，仍是可稽核的例外。

## 5. 這個檢查**不能**取代什麼（誠實邊界）

1. **同一檔案內的部分回退**：`main` 的改動若同時新增與刪除行，diff 會出現 `added > 0` ⇒ 不列入
   候選（§2.1）。例：分支與 `main` 都改同一個檔案的同一個欄位，分支把 `main` 的新值覆蓋回去。
   本檢查**看不到**這種「同檔混合 hunk 的語意衝突」。這是刻意的保守選擇：把 `added > 0` 也算進去
   會讓「本 PR 自己的工作」全面誤報。此邊界由回歸測試的 `P5` 釘住（哪天放寬規則，測試會紅）。
2. **分支已對齊 `main` 但內容仍矛盾**：例如 cherry-pick 出不同實作、或兩邊各自改了同一段。
   那要靠測試與 review，不是靠 diff 形狀。
3. **非 git 可見的回退**：production 上的檔案、DB 內容、config map、另一個 repo（前端、Hermes）。
4. **反向方向**：`main` 已刪除的檔案被落後分支「復活」（那是 `added > 0`）；或 `main` 的改動是
   純刪除行（分支只多不少）——這兩種 diff 形狀不會出現「假刪除」。
5. **極端 cherry-pick**：分支把 `main` 的某次改動 cherry-pick 成新 sha（內容相同），之後又真的刪掉
   那些行 ⇒ 會被判成落後。用 allowlist 明示（本檢查不比對內容等效性）。
6. **不判斷 `base` 本身對不對**：PR base 設錯（stacked PR、誤設上一個任務的分支）由
   [`pr-base-guard.yml`](../../.github/workflows/pr-base-guard.yml) 負責。
7. **`--strict` 之外的 WARN 不紅燈**：一般程式碼的假刪除只警告；`main` 整條線的風險仍要靠人讀輸出。

因此：本檢查是**必要不充分**。它把「人工跑兩條 git 指令」變成自動閘門，但不保證「合併後一切正常」。

## 6. 誤報處理 SOP

1. 先看輸出是 `❌ [共用資產]` 還是 `⚠️ [其他檔案]`，以及 `branch_touched_file` 是 True 還是 False。
2. `branch_touched_file = False` ⇒ 幾乎一定是落後造成的：跑 `gh pr update-branch`（或
   `git fetch origin main && git merge origin/main`）後重跑。
3. `branch_touched_file = True` ⇒ 本 PR 真的動過該檔：先 update-branch 再重新套用你的改動；
   若真的必須維持現狀，才登 allowlist（`reason` 必填）。
4. 若規則本身造成系統性誤報 ⇒ 改規則（並更新本規格 §2／§5），不要用 allowlist 掩蓋。
5. 改完 `bash tests/scripts/test-revert-guard.sh` 必須全綠（27 項），`make ci-gate` 必須綠。

## 7. 驗收證據（2026-09-26，本檢查落地 PR）

| 情境 | 命令 | 結果 |
|---|---|---|
| 故意造的落後分支（hermetic fixture，共用資產 `monitoring/**`、`.github/**` 被回退） | `bash tests/scripts/test-revert-guard.sh` | 27/27 PASS：落後 ⇒ exit 1、已對齊 ⇒ exit 0、allowlist 有理由 ⇒ exit 0、理由空白／TODO ⇒ exit 1、合成 merge ref ⇒ exit 2 |
| 已 `update-branch` 的正常分支 | `bash scripts/ci/check_revert_guard.sh`（本 PR 分支，base=origin/main） | exit 0；`✅ revert-guard PASS（無落後分支回退）`，耗時 0.05s |
| 真 repo 歷史造的落後分支（CI 負向證明同款） | 從 `main` 最後動共用資產的 commit 的 parent 開分支 + 刪除該檔 → `check_revert_guard.sh` | exit 1；`❌ [共用資產] docs/reference/traps.md（file-deleted，added=0 deleted=328 行）`，耗時 0.51s |
| 既有 main 不誤報 | 對齊 `origin/main` 的 `HEAD` | exit 0（WARN 0 筆）|
| 執行時間 | 上述三種情境 | 0.05s / 0.13s / 0.51s（皆 < 30s）|
| CI 接入 | `.github/workflows/quality.yml` → `revert-guard` job（PR 事件必跑，含負向證明） | GitHub CI 全綠（含 `inert-closure`）|

allowlist 現況（真檔）：0 筆（本 PR 未需要任何豁免 —— 需要豁免才代表閘門被繞過）。
