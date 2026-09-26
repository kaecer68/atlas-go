# PR Lifecycle — 從本地修改到 production 驗收的全流程

> **文件角色**：AI agent + 人類 reviewer 共享的 PR 流程規範。取代過去散落在 `AGENTS.md`、各 runbook、Makefile 註解的 PR 紀律片段。
> **配套文件**：`AGENTS.md` 為入口索引（引用本檔），`docs/operations/local-deploy.md` 為 production 部署的權威說明（不在本檔範圍）。
> **本檔建立背景**：2026-08-05 v3.0 dispatch thread 暴露兩個洞 — (1) PR-F #1457 偷跑只跑 `ci-gate` 沒跑 `ci-full`,production 才暴露 recovery path 只處理 `status=ok` 不處理 `stale`; (2) PR merge 後沒跑 production 驗收就當 done。**這兩個洞不該再發生**。

## 1. Pre-PR — 本地修改 + 驗證

任何 code 變更 MUST 通過:

| 步驟 | 指令 | 目的 | 強制 |
|------|------|------|------|
| 1.1 讀 `atlas-pre-change-protocol` skill | （不適用） | 確認設計意圖,避免淺層 patch | MUST |
| 1.2 同步測試 | 修改 `internal/<x>.go` → 同 commit 改 `internal/<x>_test.go` | 不留「先 commit 功能測試晚點補」 | MUST |
| 1.3 本地 `go test` 紅綠燈驗證 | `go test ./internal/<x>/ -count=1 -race` | 確認測試 fail → fix → pass | MUST |
| 1.4 格式 | `gofmt -w .` | 不留 gofmt diff | MUST |

## 2. PR-Create — 推上 remote + 開 PR

### 2.1 完整 CI（必跑,非可選）

| 階段 | 指令 | 耗時 | 目的 | 強制 |
|------|------|------|------|------|
| 2.1.1 Fast gate | `make ci-gate` | <30s | 快速 sanity check | MUST（最低門檻） |
| 2.1.2 Full gate | `make ci-full` | ~5-8 min | golangci-lint v2.12.2 + staticcheck + go test -race + cmd/atlas 整合 + coverage ≥ 60% + orphan artifact | **MUST** |
| 2.1.3 Result captured in PR body | （人工） | — | 把 `make ci-full` 結果摘要寫進 PR body 的 "Verification" 段 | MUST |

**跳過 `make ci-full` 偷跑 = 偷工**。如必須跳過（如 rebuild 中 / 時間限制）：

- PR body **必須**明確標示 `KNOWN: ci-full not run, will run after rebase` 並 link 到 blocker
- 不可靜默跳過
- blocker 解除後 24 小時內補跑並把結果補進 PR comment

### 2.2 推上 remote

```bash
git push -u origin <branch>
```

### 2.3 開 PR

```bash
gh pr create --title "<type>(<scope>): <subject>" --body-file /tmp/pr-body-<N>.md
```

PR body **MUST 含三段**（對應 `.github/PULL_REQUEST_TEMPLATE.md` 的 Summary / Root Cause / Verification 段）：
- **Summary** — 修了什麼 / 加了什麼
- **Root Cause** — 為什麼壞（事實為本,不是猜測,link 到 issue / log / source code）
- **Verification** — 跑了什麼 test,結果是什麼（含 `make ci-full` 結果 + production 驗收 checklist 適用時）

### 2.4 pre-push hook 的環境陷阱（linked worktree 會 export `GIT_DIR`）

`git push` 跑 `.githooks/pre-push` 時，git 會把 **`GIT_DIR` 以絕對路徑 export 給 hook**——
在 **linked worktree**（`git worktree add`）裡，值是 `<main>/.git/worktrees/<name>`。
後果：任何在 hook 裡執行的測試，只要它建立 throwaway git repo（`git init` / `git -C <temp> …`），
這些命令會**打到呼叫者的 repo**（cwd 被當成 work tree）⇒ fixture 的 commit 直接落到**正在 push 的
分支**上，甚至移動 `refs/heads/main`。

- 2026-09-23（issue #1927）：`test-binary-freshness-guard.sh` 的 fixture commit 落上被推的分支。
- 2026-09-26（issue #1993 工作）：`test-revert-guard.sh` 第一版沒有防護，一次 pre-push 把
  `refs/heads/main` 移到 fixture commit（`8c926b2b`）並改寫被推的分支；`main` 需以
  `git update-ref refs/heads/main <正確 sha>` 還原，並清掉 fixture 順手寫進 `.git/config` 的
  `core.bare`）與 `[user]`（`ci@test.invalid`）。**沒有 push 出去**（hook 自己失敗了）才沒有擴散。

**寫測試的強制要求**（fixture repo 一律如此）：

```bash
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX
# 並且在建立 fixture repo 後斷言它解析到自己（洩漏時大聲失敗、絕不 commit）：
test -d "$FIXTURE/.git" || fail "GIT_DIR leak?"
test "$(git -C "$FIXTURE" rev-parse --show-toplevel)" = "$(cd "$FIXTURE" && pwd -P)" || fail "GIT_DIR leak?"
```

參考實作：`tests/scripts/test-binary-freshness-guard.sh`、`tests/scripts/test-check-frontend-dist.sh`、
`tests/scripts/test-revert-guard.sh`。

## 3. PR-Review — Reviewer 檢查

### 3.1 Reviewer 必看

| 項目 | 標準 |
|------|------|
| Root cause 是事實還是猜測 | MUST 為事實,有 log / source code / 重現步驟佐證 |
| Test 是否真的 fail → fix → pass | 不可只有 happy path |
| 是否回歸既有功能 | 看 diff 影響面 |
| 是否有「東一塊西一塊」的補丁 | 若有,要求 PR 拆成多個 |
| 是否動到 `docs/operations/` 規範文件 | 若是,必須連規範一起梳理,不可補丁式 |

### 3.2 CI 通過條件

- [ ] `make ci-gate` 過
- [ ] `make ci-full` 過（**不可跳過**）
- [ ] GitHub CI 全綠（含 `monitoring-config` §3.4、`revert-guard` §3.5）
- [ ] Reviewer approve

### 3.3 PR 整個 CI 都沒跑（required checks 卡在 `Expected`）

> **實證 2026-09-25（PR #1975，任務 I）**。症狀:PR 開好之後 `gh pr checks` 一個檢查都沒有;
> `gh pr merge --squash --admin` 被拒,訊息是 `12 of 12 required status checks are expected`,
> 而 `gh run list --branch <branch>` 是空的。

**真因（不是 `paths` 過濾）**:三個 PR workflow（`ci-cd.yml`、`quality.yml`、`constitution.yml`）
的 `on.pull_request.branches` 只允許 `main` / `develop`。GitHub 的 `branches` 過濾比對的是
**PR 的 base 分支**;不匹配時 GitHub **不建立任何 workflow run**——連 `skipped` 記錄都沒有
（所以「檢查清單裡沒東西」與「job 被 skip」是兩件不同的事）。#1975 的 base 被設成上一個任務
的分支 `fix/monitoring-rules-hardening`（原意是 stacked PR，但任務其實已完成、要直接進 main）,
因此整個 PR 沒有任何 run。

**為什麼「把 base 改成 main」當下也沒用**:`base_ref_changed` **不是** `pull_request` 的預設
活動類型（預設只有 `opened` / `synchronize` / `reopened`）→ 只改 base 不會觸發任何 workflow。
#1975 當時是再推一個空 commit（`synchronize`）才讓 3 個 workflow 跑起來,12 項 required
checks 才回報（35 pass）。

**診斷指令（唯讀，30 秒定案）**:

```bash
BR=fix/example-branch
gh run list --branch "$BR"                                   # 空 = 完全沒有 run
gh api "repos/:owner/:repo/actions/runs?head_sha=<sha>"      # total_count 0
gh api repos/:owner/:repo/commits/<sha>/check-runs           # total_count 0
gh api repos/:owner/:repo/branches/main/protection \
  --jq '.required_status_checks.contexts'                    # 12 個 job 名（不是 workflow 名）
# base 變更史（誰把 base 改掉、改前是什麼）:
gh api graphql -f query='{repository(owner:"<owner>",name:"<repo>"){pullRequest(number:<N>){
  timelineItems(itemTypes:[BASE_REF_CHANGED_EVENT,HEAD_REF_FORCE_PUSHED_EVENT]){nodes{
  __typename ... on BaseRefChangedEvent{createdAt previousRefName currentRefName}}}}}}'
```

**處置（依序）**:

1. 若 base 設錯:`gh pr edit <N> --base main`,**然後再推一個新 commit**
   （`git commit --allow-empty -m 'chore(ci): re-trigger workflows' && git push`）
   或 close + reopen PR。只改 base 不會產生 run。
2. 若 base 是刻意的（stacked PR）:CI 不會跑在這一顆 PR 上,由上游 PR 承載;不要為了讓
   required checks 出現而亂改 `branches` 過濾。

**為什麼不從 repo 端「拿掉 `branches` 過濾」根治**:`branches` 過濾同時扮演安全網——
PR base 設錯時 CI 完全不跑 + required checks 永遠停在 `Expected` → **merge 被擋住**,
「目標分支設錯」就不會靜默變成「合併到錯的分支」。真正的問題是這個訊號太隱晦
（只丟一句 `12 of 12 required status checks are expected`）。因此改為加裝 tripwire:
`.github/workflows/pr-base-guard.yml`（唯一一個不帶 `branches` 過濾的 workflow）在 PR
base 既不是 `main`/`develop`、也不是另一個 open PR 的 head 分支時**直接紅燈**,並把上面的
修法貼進 step summary。

### 3.4 監控設定 gate（promtool / amtool）

`quality.yml` 的 `monitoring-config` job（2026-09-25 任務 I 新增）對 `monitoring/**`
做四件事,全部 **blocking**（不得加 `|| true` / `continue-on-error`）:

| step | 指令（容器內） | 抓什麼 |
|------|----------------|--------|
| check config | `promtool check config monitoring/prometheus.yml` | scrape_configs 語法 + `rule_files` 全部可載入 |
| check rules | `promtool check rules monitoring/rules/*.yml` | 逐檔規則語法 / 表達式合法性 |
| test rules | `promtool test rules monitoring/tests/*.yml` | **該 firing 的真的會 firing**（合成序列）+ annotation render 結果 |
| check-config | `amtool check-config monitoring/alertmanager.yml` | route / receiver / inhibit 結構 |

- **版本紀律**:用釘版容器 `prom/prometheus:v3.14.0`、`prom/alertmanager:v0.27.0`
  （= Mac Mini production 同版）,永不用 `latest`。容器以 image 內建 `nobody` 執行,只讀工作區。
- **為什麼需要 `test rules`**:`promtool check rules` 只看 YAML/表達式語法,驗不到
  「結構上不可能 firing」（依賴不存在的指標源、regex 匹配不到生產容器名）與
  「annotation 求值才炸」（#1972 的 `{{.Mounts}}`）——這兩類都進過版控且沒被發現。
  測試檔在 `monitoring/tests/`,改規則文案/標籤時 expectation 會紅（刻意:強迫確認規則還真的會 firing）。
- **已知覆蓋缺口（有紀錄、非靜默吞掉）**:`promtool check rules` 的 lint（重複規則）
  **預設非致命**（exit 0）;目前 `main` 上有一個既存重複——
  `monitoring/rules/channel_health_latent_staleness.yml` 的 `ChannelHealthStatusError`
  四條同名同標籤規則。清理後把該 step 改成
  `promtool check rules --lint=all --lint-fatal ...` 即可收緊（`--lint-fatal` 讓 lint 以 exit 3 失敗）。

### 3.5 合併結果驗證 + diff 衛生 guard（revert-guard）

> **實證 2026-09-26（單一 session 內 5 個 PR／6 次）**。症狀:PR 分支落後 `main` 時,
> `git diff origin/main <branch>` 把 `main` 上**其他 PR 最近的改動顯示成刪除**（review／agent 會
> 誤判「這個 PR 在刪別人的東西」）。
> ⚠️ **但這不是回退**:對「分支沒動過的檔」,merge commit／`git merge --squash`（＝GitHub 的 squash
> 按鈕）／rebase 都走三方合併（merge-base 的版本＝分支版本）⇒ **保留** `main` 的版本。
> 真正的危害是**這份 diff 對讀者說謊**;唯一真的會刪的是「把 two-dot diff 當 patch 套用」
> （`git diff <base>..<head> | git apply`）。
> **唯一會真的回退 `main` 的路徑是 evil merge**:落後分支在本地 `git merge origin/main` 解衝突時
> 解錯（或 `-X ours/theirs` 硬吞）,把 `main` 的修正悄悄丟掉——此時分支「已對齊」、diff 乾淨,
> 但合併真的把 `main` 的修正退回舊版。

| PR | 被顯示成刪除的他人改動 |
|---|---|
| #1974 | `monitoring/alertmanager.yml`（−37）|
| #1979 | `.github/workflows/pr-base-guard.yml`（−92）、`quality.yml`（−66）|
| #1990 | `internal/monitoring/metrics_bridge.go`（−41）、`internal/monitoring/universe_scheduler.go`（−62）|
| #1991 / #1994 | `internal/monitoring/metrics_bridge.go`（−41）、`metrics_bridge_test.go`（−136）、`.claude/skills/.../SKILL.md`（−41）|

**為什麼 `MERGEABLE` 幫不上忙**:它只回答「**能不能**自動合併」,不回答「合併本身**會不會弄丟 `main` 上的東西**」
（那是上面兩個相各自回答的問題)。

**診斷指令（唯讀,<1 秒;不必等 CI、也不必連 GitHub）**:

```bash
git fetch origin main
git diff --numstat origin/main <branch>        # added==0 && deleted>0 的檔案 = 嫌疑（純刪除）
git log -1 --no-merges origin/main -- <file>   # 該檔在 main 上最後的改動是誰;不是本 PR 引進的 ⇒ 那條刪除是假象
```

**處置（依序）**:

1. `gh pr update-branch <PR>`（本地等效:`git fetch origin main && git merge origin/main`,或 `git pull --ff-only`）
   → 重跑檢查,假刪除消失。**本地解衝突時不要猜、不要用 `-X ours/theirs` 硬吞**（解錯＝evil merge,
   `main` 的修正會被悄悄丟掉;`gh pr update-branch` 遇衝突會直接拒絕,所以它不會產生 evil merge）。
2. 若本 PR **真的要刪**該檔:先 update-branch,再把刪除重新套用一次（否則會把 `main` 的新改動一起帶走）。
3. 例外才登 `scripts/ci/revert-guard-allowlist.json`（`reason` 必填,寫到別人能自行驗證）。

**閘門(`quality.yml` 的 `revert-guard` job,2026-09-26 起）**——兩個相,結論相反:

| 相 | 判定 | 結果 |
|---|---|---|
| **合併結果驗證** | `git merge-tree --write-tree <base> <head>` 算合併結果樹 T,再 `git diff <base> <T>`;對「本 PR 沒以**普通 commit** 引進」的路徑,合併結果必須完全等於 `base`（不同 ⇒ **evil merge**）| **FAIL** |
| **diff 衛生** | `git diff <base> <head>` 的「純刪除／整檔刪除」且該檔在 `base` 上有本分支沒有的改動（= 落後的假刪除）| **WARN**（不擋；`--strict` 才失敗）|

共用資產(`.github/`、`monitoring/`、`configs/`、`docs/reference/`、`scripts/ci/`、`Makefile`、
`docker-compose*.yml`)在 WARN 相只是**訊息優先序**（最容易誤導人 ⇒ 先列）,不再是 FAIL —— 本 repo
已強制 require-up-to-date,再讓它紅只是重複保護並製造 treadmill。`gh pr update-branch` 遇衝突會
**拒絕**（⇒ 不會產生 evil merge）,**本地 merge 遇衝突不要猜**。
job 內附負向證明:用**真的 repo 歷史**以 plumbing 造一個「已對齊的 evil merge」合成 commit
（不動工作樹、不動 ref）,`git merge-tree` 證明合併結果真的少掉那個檔,檢查必須以 **exit 1** 擋下。
本機可用 `make revert-guard` 先跑（`make ci-gate` 也涵蓋）。規格與**誠實邊界**見
[`../specs/branch-revert-guard-spec.md`](../specs/branch-revert-guard-spec.md)。
**它抓不到**:evil merge 落在本 PR 也改過的路徑上（最大盲區）、同一檔案內的部分回退（`added>0`
的混合 hunk）、`main` 純修改共用資產（形狀依賴）、分支已對齊但內容仍矛盾、非 git 可見的回退。

## 4. PR-Merge — 合併到 main

### 4.1 Squash-merge

```bash
gh pr merge <N> --squash --delete-branch --admin
```

> **⚠️ `--admin` 繞過 GitHub branch protection 的必要審查批准**。**僅在以下三項全部滿足後**才可使用：
> 1. `make ci-gate` 過（§2.1.1）
> 2. `make ci-full` 過（§2.1.2）
> 3. Reviewer approve（§3.2,human reviewer 已明確 approve comment）
>
> 若有 1-2 項未滿足但仍要 merge,改用 `gh pr merge <N> --squash --delete-branch`（不帶 `--admin`）,會觸發 branch protection 要求 review。**不可靜默繞過審查**。

### 4.2 合併後 git state

```bash
git fetch --prune
git checkout main
git pull --ff-only
git log --oneline -3  # 確認 HEAD 是新 merge
```

### 4.3 刪除本地 branch + worktree（multi-cli protocol 規範）

**若用 worktree 開發**（multi-cli protocol 默認）：

```bash
# 1. 確認當前在 worktree 路徑
pwd  # 應在 /Users/kaecer/workspace/<slug>,非 main

# 2. 切回 main worktree
cd /Users/kaecer/workspace/atlas

# 3. 移除 worktree（force 必要時）
git worktree remove --force /Users/kaecer/workspace/<slug>

# 4. 刪除本地 branch（已被 --delete-branch 從 remote 刪）
git branch -d <branch>

# 5. 清理 stale worktree reference
git worktree prune
```

**若直接在 main worktree 開發**（不推薦）：

```bash
git branch -d <branch>
```

完整 checklist 見 `docs/multi-cli-protocol.md` §Post-merge cleanup。

## 5. Post-Merge — Production 驗收（**最容易漏的步驟**）

> **PR merge ≠ PR done**。合併後需 deploy + 在 production 跑驗證 checklist 才能視為 PR 完成。

### 5.1 Docker rebuild（Mac Mini production）

> **部署真相（2026-09-23 更新）**：production 已於 **2026-09-22 從 iMac 遷移到 Mac Mini**（`kaecer@kmacmini`）；iMac 已退役。本節指令經 2026-09-23 兩次實走驗證。
> **跨機一律用 Tailscale 名稱 `kmacmini`**（MagicDNS，解析為 Tailscale IP）；LAN IP 僅在 MacBook 位於同一網段時可用，外出時失效。
> ⚠️ 歷史指令（`ssh kk@kimac …`、`Makefile.prod`、`docker-compose.prod.yml`）**一律失效**；`docker-compose.prod.yml` 仍存在於 `docs/operations/` 但只是歷史產物（其 `atlas` service 沒有 build 段，無法用來重建映像）。

```bash
# Mac Mini (production) — rebuild + 重啟（實走驗證 2026-09-23）
ssh kaecer@kmacmini
export PATH="$HOME/.orbstack/bin:/usr/local/bin:/opt/homebrew/bin:$PATH"   # 非互動 shell 沒有 docker
export GOPROXY=https://goproxy.cn,direct                                   # 本機 router/HITN MITM proxy.golang.org
cd ~/workspace/atlas
git fetch origin main && git checkout main && git merge --ff-only origin/main
make rebuild-all

# MacBook 本機 dev 驗證（不影響 production）
make rebuild-all   # 在 MacBook 本機跑,起本地容器驗證
```

（repo 目錄 `.env` 需含 `GRAFANA_PORT=3001` 與 `ATLAS_POSTGRES_PORT=55432`；完整步驟與 10 個實踩坑見 `docs/operations/local-deploy.md` §Mac Mini production 部署。）

**執行者**：hermes（**Mac Mini 運維員**，`~/.hermes/hermes-agent/venv/bin/hermes chat -q …`）可執行 Mac Mini 的 rebuild（這是她的職責，2026-09-23 已實走成功）；MacBook 本機 dev 的
rebuild 由開發 agent / kaecer 執行。兩者皆**不會**直接操作對方機器的 production。

### 5.2 Binaries 對齊檢查

```bash
make check-binaries  # 應顯示 "ALL BINARIES FRESH"
```
> 2026-09-23 更新（#1931）：判準改為「binary 的 buildinfo.Commit 必須包含**最後一個觸及建置輸入**的 commit（`*.go`/`go.mod`/`go.sum`/`Dockerfile*`/`scripts/cron-entrypoint.sh`）」，因此 compose/docs-only 的合併不再誤報 STALE。輸出會印 `last build input: <sha>`。
> 陷阱：**非互動 ssh 沒有 docker 在 PATH**（`~/.orbstack/bin`），此時會誤報 `image unavailable`；`make check-binaries` 走互動 shell 不受影響。

若不 fresh,等 docker 重建完成。

### 5.3 Production Verification Checklist（每個 PR 都必跑，在 Mac Mini production 上驗證）

> **驗證位置**：Mac Mini（`ssh kaecer@kmacmini`，或派 hermes 代勞）。PR author 或 reviewer **MUST 給出
> 3-5 個 curl 指令**針對該 PR 修的 channel / endpoint。例如:

```bash
# 範例: PR-F #1457 修 crossmarket recovery
curl -s http://127.0.0.1:18080/api/dashboard/channel-health | jq '.channels[] | select(.channel_id=="us10y" or .channel_id=="vix")'
# 預期: status=ok, updated_at = 重建後時間

docker logs atlas-go 2>&1 | grep "recovery: cleared"
# 預期: [CrossMarket] recovery: cleared 10 macro channels (status=ok)
```

### 5.4 Done Criteria

PR 視為完成 **必須**所有三項：

- [ ] `make ci-full` 過（PR-Create 階段）
- [ ] Docker image rebuild + 重啟 + `make check-binaries` ALL FRESH
- [ ] Production verification checklist **全部**通過

若任一不通過,PR 狀態為「**merged-but-unverified**」,需開 follow-up issue 處理。

## 6. 自動化機制（roadmap,未實作）

目前 PR 流程依賴人 + AI 紀律。**未來可加**的自動化:

| 機制 | 工具 | 目的 |
|------|------|------|
| Pre-push hook 強制 `make ci-full` | `scripts/ci/pre-push.sh` | 偷跑時直接擋下,不靠人記 |
| `make verify-production <pr-number>` | Makefile target | 自動跑 production verification checklist + 結果寫進 PR comment |
| Dead link 自動掃 | `.github/workflows/doc-link-check.yml` | 規範文件東一塊西一塊時自動掃出孤立 reference |

## 7. 違反此規範的後果

- **AI agent 跳過 `make ci-full`**:下次 AI session 開工時,`AGENTS.md` L66 仍要求此項,規範違規會在 review 時被抓
- **PR merge 沒跑 production 驗收就當 done**:commit author 需在 24 小時內補做,或開 follow-up issue
- **東一塊西一塊的補丁式規範文件修改**:reviewer 必須 reject,要求作者重新梳理

## 8. 修訂紀錄

| 日期 | 修訂 | 作者 |
|------|------|------|
| 2026-08-05 | 初版建立,因 2026-08-05 v3.0 PR-F #1457 半失敗教訓 | kaecer dispatch + AI agent |
| 2026-09-25 | 新增 §3.3（PR base 設錯 → CI 靜默不跑 / required checks 卡在 Expected 的真因、診斷與處置 + `pr-base-guard` tripwire）、§3.4（promtool/amtool 監控 gate）;起因 PR #1975 任務 I | kaecer dispatch + AI agent |
| 2026-09-26 | 新增 §2.4（pre-push hook 在 linked worktree export GIT_DIR ⇒ 建 throwaway repo 的測試會把 fixture commit 送進呼叫者的 repo；#1927 同款，2026-09-26 再犯一次）;起因 issue #1993 工作 | kaecer dispatch + AI agent |
| 2026-09-26 | 新增 §3.5（合併結果驗證 + diff 衛生閘門：落後分支的 diff 會顯示假刪除但**不會回退**、唯一真回退是 evil merge、診斷指令、`gh pr update-branch` 處置、`revert-guard` CI 閘門與其誠實邊界）;起因 issue #1993（單一 session 實證 #1974／#1979／#1990／#1991／#1994） | kaecer dispatch + AI agent |
