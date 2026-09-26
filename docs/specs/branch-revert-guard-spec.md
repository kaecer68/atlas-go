# 合併結果驗證 + diff 衛生閘門 — 規格（revert-guard）

| 項目 | 內容 |
|---|---|
| 文件角色 | PR 閘門 `revert-guard` 的規格：**它回答哪兩個問題**、判定規則、severity 政策、allowlist 用法、誤報出路、以及本檢查**做不到**什麼 |
| 狀態 | v2（2026-09-26，issue [#1993](https://github.com/kaecer68/atlas-go/issues/1993)；v1 的「落後分支會回退」框架經獨立設計審查推翻後重新定性） |
| 實作 | [`../../scripts/ci/check_revert_guard.py`](../../scripts/ci/check_revert_guard.py)（純 stdlib Python3）＋薄殼 [`../../scripts/ci/check_revert_guard.sh`](../../scripts/ci/check_revert_guard.sh) |
| 機器 allowlist | [`../../scripts/ci/revert-guard-allowlist.json`](../../scripts/ci/revert-guard-allowlist.json)（每筆 `reason` 必填；**只豁免 WARN 相**） |
| 回歸測試 | [`../../tests/scripts/test-revert-guard.sh`](../../tests/scripts/test-revert-guard.sh)（hermetic：tempdir 內真 git repo，不連網、不碰本 repo、不碰 production） |
| CI 接入 | [`.github/workflows/quality.yml`](../../.github/workflows/quality.yml) 的 `revert-guard` job（無 path filter；含負向證明） |
| 本地接入 | `make revert-guard`、`make revert-guard-selftest`、`make ci-gate`（因此 `make pre-push` 涵蓋） |

## 1. 這個閘門是什麼（v2 重新定性）

**不是**「落後分支回退預測器」。它做兩件不同的事：

| 相 | 問題 | 判定 | 結果 |
|---|---|---|---|
| **A. 合併結果驗證** | 「把這個 PR 併進 `main`，會不會真的弄丟 `main` 上的東西？」 | `git merge-tree --write-tree <base> <head>` → 合併結果樹 T；`git diff <base> <T>` 對「本 PR 沒以普通 commit 引進」的路徑必須完全等於 base | **FAIL**（exit 1）|
| **B. diff 衛生** | 「這份 diff 會不會誤導 review／agent？」 | `git diff <base> <head>` 的「純刪除／整檔刪除」且該檔在 base 上有本分支沒有的改動 | **WARN**（exit 0；`--strict` 才失敗）|

### 1.1 v1 的錯誤框架（本節存在的理由是**不要重犯**）

v1 寫「分支落後 `main` ⇒ squash／rebase merge 是把 `base..head` 當 patch 套用 ⇒ **真的刪掉**別人的修正」。
**這是事實錯誤**，已由獨立審查以 git 2.52 實測推翻（同款實驗見 §7 表 A）：

| 對同一個落後分支的落地方式 | `main` 在分岔後改過的檔（分支沒動過） | `main` 在那之後新增的檔 |
|---|---|---|
| `git merge --no-ff`（merge commit） | **保留** | **保留** |
| `git merge --squash`（＝GitHub 的 squash 按鈕） | **保留** | **保留** |
| `git rebase main`（＝GitHub 的 rebase-merge） | **保留** | **保留** |
| `git diff <base>..<head> \| git apply`（把 two-dot diff 當 patch 用） | **被回退** | **被刪掉** |
| `git merge-tree --write-tree <base> <head>`（合併結果 oracle） | 對 `main` 的淨效果＝只有本 PR 自己的改動 | 無變化 |

對「分支沒動過的檔」，三方合併的 merge-base 版本就是分支的版本 ⇒ 結果取 `main` 的版本。
**真正的危害是那份 diff 對讀者說謊**（本 session 7 次「看起來在刪別人東西」全是這個假象；
`main` 至今仍含所有「會被回退」的檔案）。**唯一會真的刪的是「把 two-dot diff 當 patch 套用」**
（`gh pr diff`／GitHub「Files changed」用的是 three-dot diff，不會顯示這些假刪除；
受害的是按本 repo SOP 手動跑 `git diff origin/main <branch>` 的人與 agent）。
**唯一會真的回退 `main` 的路徑是 evil merge**：落後分支在本地 `git merge origin/main` 解衝突時解錯
（或 `-X ours/theirs` 硬吞），把 `main` 的修正悄悄丟掉——此時分支「已對齊」、two-dot diff 乾淨，
但合併真的把 `main` 的修正退回舊版。§2.4 就是為了抓它。

### 1.2 因此兩個相的結論相反

- **A 相（合併結果）**：可以單純由 merge-tree 判定 ⇒ 有資格 FAIL。
- **B 相（diff 衛生）**：v1 讓「共用資產的假刪除」FAIL。**v2 移除這個判定**，理由三條：
  1. 它與回退風險無關（未動過的檔在合併時 100% 安全，§1.1 表）；
  2. 本 repo **已強制 require-up-to-date**（branch protection），這個判定只是重複既有保護；
  3. 它造成 **treadmill**：`main` 近 7 日 70 commits、54% 碰共用資產 ⇒ 對齊後數小時就又 FAIL，
     每次代價＝`gh pr update-branch` ＋ 55 個 check 重跑。
  ⇒ 落後分支偵測改為 **WARN-only（exit 0）**；`--strict` 保留給本機（想嚴格的人自己加）。
  共用資產清單（`.github/`、`monitoring/`、`configs/`、`docs/reference/`、`scripts/ci/`、`Makefile`、
  `docker-compose*.yml`／`*.yaml`）**降級為訊息優先序**：同為 WARN，但它們的假刪除最容易誤導人
  （CI 閘門、監控設定、參數、規範文件、部署 compose），所以先列並標「共用資產」。

## 2. 判定規則

輸入是兩個 ref：`base`（`main` tip）與 `head`（PR 分支 tip）。全部判定都由 git 產生，
**不讀 GitHub API、不依賴網路**（merge-tree 只寫 object DB，不動 ref、不動工作樹）。

### 2.1 B 相第一階段：候選 = 「純刪除」或「整檔刪除」

對 `git diff --name-status --no-renames <base> <head>` 的每個檔案：

| 候選種類 | 條件 |
|---|---|
| `file-deleted` | status = `D`（整個檔案在 head 不存在）|
| `pure-deletion` | numstat `added == 0 && deleted > 0`（檔案還在，內容被淨刪除）|

`--no-renames` 是刻意的：rename 判定是啟發式（相似度門檻），會讓路徑在 diff 兩側對不起來；
本檢查要逐路徑判讀「這條路徑的內容是不是被淨刪除」，寧可看到「A 刪、B 增」兩個獨立項。

`added > 0` 的改動**不列入候選**（可能真的動了手），這是已知邊界，見 §5.2。

### 2.2 B 相第二階段：候選必須是「落後」造成的

> `git rev-list --no-merges <base> ^<head> -- <path>` 非空

`base ^head` = 可由 `base` 抵達、但 `head` 沒有的 commit。非空代表「**`main` 上該檔有本分支沒有的
改動**」，也就是「該檔在 `main` 的最近一次改動不是本 PR 引進的」。分支若已含 `main` 的該次改動，
diff 就不會是純刪除；分支完全沒碰該檔時，diff 的刪除**必然**全部來自落後。

### 2.3 B 相第三階段：訊息優先序（**不是** severity）

共用資產先列、標「共用資產」，其餘標「其他檔案」。**兩者都是 WARN（exit 0）**，理由見 §1.2。
輸出每筆含：檔案、種類、numstat、`main` 上該檔最後的 commit（`sha` ＋ 日期 ＋ subject）、
以及「本分支動過沒動過」。`--strict` 把 WARN 升級成 exit 1（本機用；CI 不用）。

### 2.4 A 相（FAIL）：evil merge

1. `git merge-tree --write-tree <base> <head>` → 合併結果樹 T。
   * 有衝突（git 回 rc≠0）⇒ **只出 WARN** 並列出衝突檔：GitHub 自己就會擋衝突，不會靜默落地；
     訊息指向 `gh pr update-branch`（遇衝突會拒絕）。
   * 本機 git < 2.38（沒有 `--write-tree`）⇒ **fail-closed，exit 2**（不回報通過；見 §5.5）。
2. `owned` = `merge-base(base, head)..head` 之間**普通 commit**（`git log --no-merges`）動過的路徑集合。
   merge commit 的衝突解法**不算** PR 的意圖——那正是要抓的東西。
3. 對 `git diff --name-status --no-renames <base> <T>` 的每個路徑 p：
   **p ∉ owned 且 `base:p` ≠ `T:p` ⇒ FAIL**（exit 1），訊息附 merge-tree 樹 sha、merge-base、
   numstat、以及「本 PR 的普通 commit 從未動過該檔」。
   兩個**豁免**（否則 rename 會造成大規模假紅；`R7` 釘住）：
   * 本 PR 端 rename：`owned` 由 `git log --no-merges -M --name-status` 取得，rename 的**舊名與
     新名都算 owned**（本 PR 把 `a.txt` 改名成 `c.txt`，則「合併結果少了 `a.txt`」是 git 把改動
     重導到新名字，不是 evil merge）。
   * `main` 端 rename：若 p 是 `git diff -M --diff-filter=R <mb> <base>` 的 rename **新名**，
     而它的舊名 ∈ `owned` ⇒ 跳過（並在輸出列一則 `ℹ️` 說明差異來自 rename 重導向）。
     內容等價的退路：`base:p` 的 blob 等於某個 owned 路徑在 `mb` 的 blob ⇒ 同樣跳過。

為什麼這是對的：合併的正確語意是「`base` ＋ 這個 PR 自己的改動」。未被本 PR 的普通 commit 碰過的
路徑，合併結果就必須逐位元等於 `base`；不同 ⇒ 有人把 `main` 的內容弄丟了，而唯一能造成這件事的
現實路徑就是 merge commit 的衝突解法（evil merge）。

**allowlist 不能豁免 A 相**：豁免它等於允許真的回退 `main`（§4.1）。

### 2.5 信心訊號（只影響訊息）

- `branch_touched_file`（B 相）：以 merge-base 的 blob 對照 head 的 blob。`False` ⇒ 分支根本沒碰
  這個檔 ⇒ diff 裡的刪除 100% 是 `main` 上別人近期的改動（經典案例）；`True` ⇒ 分支動過該檔
  （例如真的刪掉它）⇒ 提示「可能是刻意的刪除」。
- `rerouted_paths`（A 相）：被判定為「rename 重導向」而豁免的路徑（§2.4），列在 JSON 與輸出
  （可稽核：不是靜默放行）。
- `possible_rename_from`（B 相，`file-deleted` 候選才查）：若 diff 同時有「新增且**自 fork 以來
  沒被本分支動過**」的路徑 Q，則附提示「本分支還留著舊路徑 Q ⇒ `main` 可能是把該檔 rename
  （這裡報的是新檔名）」。只用「沒動過」的舊路徑當證據，避免把本 PR 自己新增的檔案誤當舊路徑。

### 2.6 假綠防線：合成 merge ref

GitHub 在 `pull_request` 事件預設 checkout 的是合成 merge ref（`refs/pull/<N>/merge`），它的
**第一個 parent 就是 base tip**，樹已經含 `main` ⇒ 拿它當 `head` 會**永遠 PASS**。因此：

- **新鮮**的合成 ref（`parents[0] == base_sha`）⇒ **拒絕執行**（exit 2），要求傳真正的 PR head；
- **過期**的合成 ref（`refs/pull/<N>/merge` 會隨 `main` 前進而變舊：`parents[0]` 是 base 的**祖先**
  但 ≠ base）⇒ **不拒絕**（它不含**現在**的 base，不是假綠），只多印一則 `ℹ️` 提示；
  這種輸入會被當成一般的落後分支處理（B 相 WARN、A 相照常判定）。

`base` 解不到時 **fail-closed**（exit 2，不靜默跳過）；無 remote 的環境必須明示
`--allow-missing-base`，而且輸出會說清楚「這不是通過」。

## 3. 執行與接入

```bash
make revert-guard                                  # 自我測試 + 檢查（base=origin/main, head=HEAD）
make revert-guard-selftest                         # 只跑 hermetic 回歸測試
bash scripts/ci/check_revert_guard.sh --json       # 機器可讀
bash scripts/ci/check_revert_guard.sh --base refs/remotes/origin/main --head refs/remotes/origin/pr-head
bash scripts/ci/check_revert_guard.sh --strict     # 本機：WARN 也視為失敗
python3 scripts/ci/check_revert_guard.py --help
```

exit code：

| code | 意義 |
|---|---|
| 0 | PASS（可能含 WARN）|
| 1 | 有 FAIL（evil merge），或 allowlist 有未填理由的條目 |
| 2 | 用法／環境錯誤：`base`／`head` 解不到、**新鮮**的合成 merge ref、allowlist 格式錯誤、git < 2.38 |

CI（`.github/workflows/quality.yml` 的 `revert-guard` job，**無 path filter**，所以 docs- 或
scripts-only 的 PR 也會跑）：

1. `actions/checkout`（`fetch-depth: 0`）；
2. 明示抓兩個 ref：`refs/heads/<base>` 與 `refs/pull/<N>/head`（**不用**預設的 merge ref，理由見 §2.6）；
3. 跑 hermetic 回歸測試；
4. 跑檢查（gate）；
5. **負向證明**（[`../../scripts/ci/revert-guard-negative-proof.sh`](../../scripts/ci/revert-guard-negative-proof.sh)）：
   從 `base` 的樹（`git ls-tree`）挑一個共用資產檔，用**暫存 index ＋ `git write-tree` / `commit-tree`**
   造一個「已對齊的 evil merge」合成 commit（merge commit，其中一個 parent 就是 base；
   `base..NEG` 之間沒有任何普通 commit 動過該檔；**不切換工作樹、不動任何 ref**），
   再用 `git merge-tree --write-tree` 證明它的合併結果樹就是合成樹（即「合併真的少掉那個檔」），
   最後要求檢查 **以 exit 1 擋下**，且輸出指出 `evil merge` 與該檔名。
   為什麼要這步：hermetic fixture 只證明程式邏輯，這步證明**CI 的 ref 解析與真 repo 歷史**也能擋。

   ⚠️ **負向證明自己也會踩 `pipefail` × SIGPIPE**：2026-09-26 這支腳本在 CI 以 **exit 141**
   （128+13 = SIGPIPE）失敗——`git ls-tree … | head -1` 在輸出超過管線緩衝區時讓 `git` 吃 SIGPIPE，
   `set -o pipefail` 把 141 傳成整個 step 的失敗碼（本機 macOS 因時序/緩衝區沒重現，本機是綠的）。
   修法：下游改成把輸入讀完的 `sed -n '1p'`。回歸測試另加靜態守門 `NP1`（非註解行不得出現
   `| head`）、`NP2`（仍設 pipefail，NP1 才有意義）、`NP3`（仍斷言「恰好 exit 1」）。
   同族通則：`pipefail` 之下任何「提早關閉管線」的下游（`head`、`grep -q` 之後接大輸出…）都可能
   把成功變成 141。

   ⚠️ **負向證明本身必須做 exit code 精確斷言**：v1 是 inline 版，它把工作樹 checkout 到舊 commit，
   於是檢查腳本在該 commit 不存在 ⇒ `bash: …: No such file or directory`（127）⇒ `if ! cmd` 把它算成
   「擋下了」並印 ✅ ——**一次假綠**（2026-09-26 首次 PR CI 的 log 實證）。修法三道並用：
   工作樹不動（腳本一定在）＋ 斷言 `rc -eq 1`（127／2 都會讓 step 紅燈）＋ 再斷言輸出指出的
   是那個 fixture（`evil merge` ＋ 檔名；**擋錯東西＝沒證明到**）。
   通則：負向證明的判定是「**恰好**回報預期的失敗碼」，不是「非 0」。

回歸測試是 **hook-safe 的 hermetic fixture**：`tests/scripts/test-revert-guard.sh` 會建 throwaway git repo，
所以它必須 `unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR
GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_NAMESPACE`（git 對 **linked worktree** 的 hook 會 export
絕對路徑的 `GIT_DIR`），並在建立 fixture repo 後斷言它解析到**自己**（洩漏時大聲失敗、不 commit）。
這是 issue #1927 的既有要求；2026-09-26 本 PR 的第一版測試漏了這一步，一次 pre-push 就把 fixture
commit 送進被推的分支並移動 `refs/heads/main`（見
[`../operations/pr-lifecycle.md`](../operations/pr-lifecycle.md) §2.4）。

效能（2026-09-26 量測，MacBook）：**0.1–0.3s**（含 Python 啟動；B 相 diff 只跑兩次，
A 相外加一次 `merge-tree` 與一次 `git log --name-only`）。

## 4. allowlist 與誤報出路（理由必填）

### 4.1 四條出路

1. **update-branch（正解）**：`gh pr update-branch <PR>`（GitHub 端三方合併；遇衝突會拒絕），
   或本地 `git fetch origin main && git merge origin/main`。改完重跑，假刪除消失。
   ⚠️ **本地 merge 遇衝突不要猜、不要用 `-X ours/theirs` 硬吞**：解錯＝evil merge，
   `main` 的修正會被悄悄丟掉（那正是 A 相要抓的東西）。
2. **先對齊、再重做刪除**：若本 PR 真的要刪掉該檔，先 update-branch，再把刪除重新套用一次
   （這樣 `main` 上別人的改動不會被一起帶走）。
3. **allowlist（例外，只對 B 相）**：以上都不可行時才登錄，並寫清楚理由。
4. **A 相沒有出路**：evil merge 是正確性問題，修法只有一條——重新對齊（`gh pr update-branch`）
   或把解錯的衝突解法改回來。

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

- `path`：精確路徑或 `fnmatch` glob（例 `docs/reference/*`；注意 `fnmatch` 的 `*` **跨目錄**）。
- `reason`：**必填**，空白或以 `TODO` 開頭 ⇒ 檢查失敗（`allowlist-reason-missing`，exit 1）。
- `ticket`、`added`：選填，方便日後稽核排序。
- **stale 語意**：不再命中的條目只印警告、**不讓 CI 紅燈**（跨 PR 耦合）。stale 清單由維護者人工複核。
- 命中的條目會以 `↷ [allowlist]` 明列在輸出，仍是可稽核的例外。
- **只豁免 B 相**：A 相（evil merge）不看 allowlist。

## 5. 這個檢查**不能**取代什麼（誠實邊界）

1. **evil merge 落在本 PR 也改過的路徑上 ⇒ A 相看不到**（本閘門最大的盲區）：該路徑屬於 `owned`，
   改動既可能是「本 PR 真的想這樣改」，也可能是 merge commit 解錯，本檢查不猜——交給 review。
   例：本 PR 改了 `docs/reference/traps.md`，同時在某次解衝突時把 `main` 新加的一列弄丟 ⇒ A 相 PASS。
2. **同一檔案內的部分回退**：`main` 的改動若同時新增與刪除行，diff 會出現 `added > 0` ⇒ B 相不列入
   候選（§2.1）。這是刻意的保守選擇：把 `added > 0` 也算進去會讓「本 PR 自己的工作」全面誤報。
   此邊界由回歸測試的 `P5` 釘住。
3. **`main` 對共用資產的「純修改」（numstat `added>0 && deleted>0`）在 B 相不觸發**（形狀依賴）：
   例如 `main` 改 workflow 一行 ⇒ 落後分支不會被提醒（R1 釘住）。也就是說「在共用資產上落後」的
   覆蓋是不一致的：只有 `main` 的改動呈**刪除形狀**（整檔刪除、砍行、新增檔造成的假刪除）才會提醒。
4. **`main` 端 rename 共用資產**：落後分支會在**新檔名**上被判 `file-deleted`（語意上沒有東西被刪，
   訊息易誤導）。§2.5 的 `possible_rename_from` 會在偵測到「自 fork 以來沒動過的舊路徑」時加一句
   提示（R4 釘住），但**修法仍是 update-branch**。
5. **rename 重導向的路徑**（§2.4 的兩個豁免）：這些路徑被排除在 A 相之外 ⇒ 若 evil merge 剛好
   落在這種「main／本 PR 對同一檔案做過 rename 且本 PR 另一端也動過」的路徑上，本檢查看不到。
   取捨：rename 在大型 repo 很常見，而**假紅（擋正常 PR）比漏報這種窄情境嚴重得多**（R7 釘住）。
6. **git < 2.38** 沒有 `merge-tree --write-tree` ⇒ A 相無法判定 ⇒ 本檢查 **fail-closed（exit 2）**，
   不回報通過。CI（ubuntu-latest ≥ 2.43）與本機（2.52）都高於門檻。
7. **分支已對齊 `main` 但內容仍矛盾**：例如 cherry-pick 出不同實作。那要靠測試與 review。
8. **非 git 可見的回退**：production 上的檔案、DB 內容、config map、另一個 repo（前端、Hermes）。
9. **極端 cherry-pick**：分支把 `main` 的某次改動 cherry-pick 成新 sha（內容相同），之後又真的刪掉
   那些行 ⇒ B 相會判成落後。用 allowlist 明示（本檢查不比對內容等效性）。
10. **不判斷 `base` 本身對不對**：PR base 設錯（stacked PR）由
   [`pr-base-guard.yml`](../../.github/workflows/pr-base-guard.yml) 負責。
11. **`--strict` 之外的 WARN 不紅燈**：B 相只警告；要不要對齊仍靠人讀輸出。

因此：本檢查是**必要不充分**。它把「人工跑兩條 git 指令」變成自動閘門 + 一個真正的合併結果檢查，
但不保證「合併後一切正常」。

## 6. 誤報處理 SOP

1. 先看輸出是 `❌ [evil merge]` 還是 `⚠️ [落後造成的假刪除／…]`。
2. A 相（FAIL）⇒ 看 `merge-tree` 的樹與那個路徑：那是合併真的會改掉的東西。跑
   `gh pr update-branch <PR>`；若曾在本地解過衝突，把解錯的解法改回來（不要留在 merge commit 裡）。
3. B 相 + `branch_touched_file = False` ⇒ 幾乎一定是落後造成的 diff 假象：跑 `gh pr update-branch`
   後重跑（**合併本身不會回退**，不改也不會壞，只是 diff 會誤導人）。
4. B 相 + `branch_touched_file = True` ⇒ 本 PR 真的動過該檔：先 update-branch 再重新套用你的改動；
   真的必須維持現狀才登 allowlist（`reason` 必填）。
5. 若規則本身造成系統性誤報 ⇒ 改規則（並更新本規格 §2／§5），不要用 allowlist 掩蓋。
6. 改完 `bash tests/scripts/test-revert-guard.sh` 必須全綠（51 項），`make ci-gate` 必須綠。

## 7. 驗收證據（2026-09-26，本檢查落地 PR）

### 表 A：機制實測（推翻 v1 的框架）

| 情境 | 命令（temp repo，git 2.52） | 結果 |
|---|---|---|
| `main` 在分岔後改 `monitoring/am.yml` 並新增 `newrule.yml`，落後分支只動 `g.txt` | `git merge --squash` / `git merge --no-ff` / `git rebase main` | `am.yml` **保留** `main` 的版本、`newrule.yml` **保留**（三方合併）|
| 同上 | `git diff main..feat \| git apply`（在 `main` 上） | `am.yml` 退回舊版、`newrule.yml` **被刪**（`apply` 成功但內容回退）——**唯一會真的刪**的落地方式 |
| evil merge：分支合併 `main` 後，在 merge commit 的解法裡把 `main` 的修正拿掉 | `git merge-tree --write-tree main feat` | 合成/evil head：`git merge-base --is-ancestor main feat` 為真（已對齊、two-dot diff 看不出落後），而合併結果**少掉那行** |

### 表 B：閘門行為（R1–R6 與正反案例）

| 情境 | 命令 | 結果 |
|---|---|---|
| **R2／F① evil merge head**（已對齊但解法把 `main` 的修正丟掉） | `bash tests/scripts/test-revert-guard.sh`（`E1`） | **exit 1**；`❌ [evil merge] monitoring/alertmanager.yml` ＋ merge-tree 樹 sha（`E1c` 釘住證據存在）|
| **F② 正常落後分支** | 同上（`L1`／`L2`） | **WARN 且 exit 0**（不擋）；`--strict` 才 exit 1（`L1g`／`L2b`）|
| **F③ 已對齊正常 PR** | 同上（`P2`、`E3`） | exit 0、**零 finding**（`P2b` 釘住「無 evil merge、無落後假刪除」）|
| **R1** `main` 純修改共用資產（`added=1 deleted=1`） | 同上（`R1`／`R1b`） | exit 0、**完全無 finding**（形狀依賴已釘住；哪天放寬會紅）|
| **R3** 過期的合成 merge ref（`parents[0]` 是 base 的祖先但 ≠ base） | 同上（`R3`／`R3b`） | **exit 0**（不拒絕）＋ `ℹ️` 提示「可能是過期的合成 merge ref」；**新鮮**的合成 ref 仍 exit 2（`N5`）|
| **R4** `main` rename 共用資產 | 同上（`R4`／`R4b`／`R4c`） | exit 0（WARN 在新檔名上），訊息明確提示「`main` 可能是把該檔 rename」|
| **R5** 本 PR 對齊前的 head `2247fc6c` vs 當時 `main` `bfd2352d`（scenario 記錄） | `python3 scripts/ci/check_revert_guard.py --base bfd2352d --head 2247fc6c --allowlist /dev/null` | v1：`⚠️ 2 筆 WARN`、exit 0（訊息句「併他人已合併的修正會被回退」是錯的）；v2：`⚠️ 2 筆 WARN`（`adapter_finmind_holiday_semantics_test.go`、`channel_contract.go`）、exit 0，訊息改為「是 diff 假象，合併會保留 main 的版本」 |
| **R7** rename 的兩向（main 端 rename + 本 PR 改舊名；本 PR rename + main 改舊名）| 同上（`R7a`／`R7b`）| exit 0、**無 FAIL**（差異被歸類為 rename 重導向 `rerouted_paths`）。這一條是**本 PR 自己在開發中實測到的假紅**：樸素的「未 owned 路徑不得改變」不變式在 rename 上會誤判（R7 釘住）|
| **R6** 全部納入 | `bash tests/scripts/test-revert-guard.sh` | **51/51 PASS**（E1–E3／L1–L2／R1／R3／R4／R7／P1–P6／N3／N4／N5／NP1–NP3）|
| 既有 negative proof（真 repo 歷史，走 CI 的 ref 解析） | `bash scripts/ci/revert-guard-negative-proof.sh origin/main` | 合成「已對齊的 evil merge」head ⇒ `merge-tree` 結果樹 == 合成樹 ⇒ 檢查 **exit 1**，輸出指出 `evil merge` ＋ `.github/CODEOWNERS`；腳本另斷言「恰好 1」 |
| 本 PR 自己的 head | `bash scripts/ci/check_revert_guard.sh`（base=origin/main） | exit 0；`✅ revert-guard PASS（無 evil merge、無落後假刪除）`，耗時 0.15s |
| 執行時間 | 上述情境 | 0.09–0.3s（皆 < 30s）|

### 表 C：已知殘留（不由本 PR 修）

- ~~`.github/workflows/quality.yml` 的 `revert-guard` job 註解與 step 名稱仍寫著 v1 框架~~
  **已解除（#2046 / E18 併入 2026-09-26）**：註解與 step 名稱已改為 v2 框架；更正實測兩句 v1 字串在
  `quality.yml` 內 `grep -c` 皆為 **0**（唯一殘留是刻意保留的歷史證據行）。`remediation-manifest.md`
  與 `FOLLOWUPS.md` 的 `E18` 條目仍記為「待派」，屬 root 單一寫者，本次未動。
- 隔壁兩支負向證明（`secret-scan`、`monitoring-single-source`）仍是
  `if bash <check>; then …` 寫法（任何非零 exit 都算「擋下了」）；`--strict` 之外的 WARN 語意變更
  不影響它們。建議另開 PR 統一改成「恰好 exit 1」斷言。

allowlist 現況（真檔）：0 筆（本 PR 未需要任何豁免 —— 需要豁免才代表閘門被繞過）。
