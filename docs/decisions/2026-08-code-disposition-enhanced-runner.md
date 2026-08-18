# 2026-08 — Code Disposition: 考古清理 (enhanced_experiment_runner.go + process_todos.py)

> **決策類型**: 死碼/孤兒代碼判定 (Code Disposition Protocol v1.0)
> **日期**: 2026-08-18
> **執行者**: prime-agent 子代理 (P3 考古清理, PR-E)
> **分支**: `chore/20260818-archaeological-cleanup`
> **規劃來源**: `/tmp/atlas-audit-2026-08-16/16-CLEANUP-PLAN-V4PRO.md` §P3
> **機制文件**: [`docs/reference/code-disposition-protocol.md`](../reference/code-disposition-protocol.md) (強制遵守)

---

## 摘要

本決策對兩個零消費者的歷史殘留做五維度判定, 並依判定處置:

| 候選 | 判定 | 處置 |
|---|---|---|
| A. `enhanced_experiment_runner.go` (模組根, 656 行) | **可退役** (有完整替代者且已接線驗證) | 刪除 (DEPRECATED 指標寫入本文件 + PR) |
| B. `scripts/process_todos.py` (178 行) | **可退役** (一次性腳本, 替代者已存在) | 刪除 |
| C. `integration_test.go` (模組根, k3 覆核補充) | **活碼** (測試存活元件, CI 有覆蓋) | **保留**, 隨根 package 遷移僅改 package 名 |
| 根 package `main` 殘留 | 歷史殘留 (`go install .` 產出錯誤 binary) | 遷移 `generate.go` package 名 (見 §5) |
| 31MB gitignored binary `enhanced_experiment_runner` | build artifact | 刪除 (未追蹤) |

**判定依據 (強制三項)**: ① 設計意圖證據 (A 有/B 無) ② 業務價值 (皆無) ③ 替代者 (皆有完整替代且已驗證) — 詳見 §2/§3 五維度表。

---

## 1. 候選背景

### A. `enhanced_experiment_runner.go` (模組根, `package main`, 656 行)

- **本質**: 獨立實驗 harness (自有 `func main`), 跑 10 輪「增強架構」smoke — Darwinian weights / PRISM / Reflexivity / Risk / Volatility / Integration 各項 ad-hoc 打分 + 綜合模擬交易, 純 `fmt.Println` demo, **非產品**。
- **git history (--follow, 3 commits)**:
  - `07a3f85f` (2026-06-15) — 與 CI+test 綑綁的 commit, 建立 harness。
  - `451c5a7c` (2026-06-15+) — swarm/MiroFish demote 時隨之調整。
  - `14ad4bc8` (2026-07-07) — swarm P1 cleanup: **刪減** swarm 引用 (31 行 → 3 行), 非新增功能。
- **消費者**: 零。`package main` 無法被 import; 全 repo 無 `go run`/`go build`/文件引用 (僅 `.gitignore`/`.claudeignore` 的 binary 忽略項)。
- **模組根副作用**: 根目前是 `package main` (GoFiles = `enhanced_experiment_runner.go` + `generate.go`), `go install .` / `go build .` 產出的 binary (module 名 `atlas-go`) 實際是這個實驗 harness, **不是真正的 atlas server** (server 在 `cmd/atlas`)。這是歷史殘留。

### B. `scripts/process_todos.py` (178 行)

- **本質**: 一次性維護 script — 掃 `configs/parameters.json` 的 TODO 參數, 依 TODO 關鍵字 (Calibrate/Literature/Reviewed/…) 分類寫入 citation (heuristic/empirical), 回寫 JSON。
- **git history**: 僅 1 commit `07a3f85f` (2026-06-15, CI+test 綑綁 commit), 無獨立設計意圖、無 header 設計敘述。
- **消費者**: 零 (全 repo 無引用; 無 Makefile target、無 CI、無 docs 提及)。

### C. `integration_test.go` (模組根, `//go:build integration`) — k3 覆核補充

- k3 審計指出原規劃決策文件漏掉此檔, 本決策一併判定 (§4)。

---

## 2. 五維度評估表 — A: `enhanced_experiment_runner.go`

| 維度 | 證據 | 判定 |
|---|---|---|
| 1. 設計意圖 | ⚠️ **有**: 對應 Phase 2/3 增強架構 (Darwinian/PRISM/Reflexivity/Risk/Volatility) 的 smoke harness, git history 顯示當時實驗用 | 有設計意圖 → 非真死碼; 進入替代者判定 |
| 2. 業務價值 | ❌ **無**: ad-hoc demo 打分, 非產品路徑; 投資人/散戶/系統穩定/UX 四項皆無 | 無價值 |
| 3. 重複/替代 | ✅ **有完整替代且已接線驗證**: PRISM Phase A (`8d77da92`, 2026-08-17) — `cmd/atlas` `prism_training` BTM task + apiMode 接真 replay executor + orchestrator 真 executor, 取代此 harness 的 PRISM/Replay 實驗角色; Darwinian 打分由正式 pipeline (`portfolio` DarwinianWeightManager 在 live 系統中運作) 取代 | 替代者完整且驗證 |
| 4. 實作狀態 | ⚠️ 可編譯但無人跑 (dev-only harness); 曾完整執行過, 非「接線中斷」的孤兒 | 已退役 (替代者上線後無執行者) |
| 5. 影響面 | ⚠️ 刪除後根 package main 失去唯一 `func main`; 31MB gitignored binary 為其 build artifact; `integration_test.go` 同屬根 package 需一併處理 (k3) | 影響可控, 已在 §4/§5 處理 |

**結論 (protocol 判定結論分類)**: **可退役** — 設計意圖存在所以**不是真死碼**, 但替代者 (PRISM Phase A replay executor + 正式 pipeline) 已完整接線驗證, 且此 harness 是 dev-only demo 非產品。依 protocol §「可退役 → 退役 (標 deprecated + 指向替代者), 可刪」。

- **DEPRECATED 指標 (寫入本決策文件, 因檔案已刪除)**: `superseded by PRISM Phase A replay executor — cmd/atlas prism_training BTM task (commit 8d77da92); Darwinian weights scoring superseded by internal/portfolio DarwinianWeightManager in live pipeline`.
- **為何不是 ORPHAN-DESIGNED**: ORPHAN-DESIGNED 適用於「設計意圖存在但接線未完」的孤兒 (需保留 + 排接線 backlog)。A 的接線**已完成且執行過**, 其功能已被**完整驗證的替代者**取代 → 屬可退役而非孤兒。依 protocol 執行規則 3, 孤兒才強制保留; 可退役允許刪除。

## 3. 五維度評估表 — B: `scripts/process_todos.py`

| 維度 | 證據 | 判定 |
|---|---|---|
| 1. 設計意圖 | ❌ **弱/無**: 無 header 設計敘述; 僅 1 個綑綁 commit, 無獨立設計文件 | 無設計意圖 |
| 2. 業務價值 | ❌ **無**: 一次性 TODO citation 處理, 已跑完; 非產品路徑 | 無價值 |
| 3. 重複/替代 | ✅ **有替代**: TODO 參數治理已由 config 參數系統 / `param_table` 治理接管; `configs/parameters.json` 的 citation 已是正式欄位 | 替代者存在 |
| 4. 實作狀態 | ⚠️ 可跑但一次性; 已無待處理 TODO 場景 | 已用完 |
| 5. 影響面 | ✅ 低: 無 import、無 CI、無 docs、無 Makefile target | 零風險 |

**結論**: **可退役** (零消費者、無設計意圖、替代者完整)。**低風險**, 依規劃不需 k3 覆核 (非業務/核心/跨模組)。

## 4. 五維度評估表 — C: `integration_test.go` (k3 覆核補充)

| 維度 | 證據 | 判定 |
|---|---|---|
| 1. 設計意圖 | ✅ 有: Phase 2/3 元件整合測試 (DarwinianWeights/Superinvestor/Spawning/PRISM/Reflexivity/E2E/Perf/Bench) | 有設計意圖 |
| 2. 業務價值 | ✅ 有: 測試**存活元件** (`internal/portfolio`, `prism`, `reflexivity`, `spawning`), 提供整合覆蓋 | 有價值 |
| 3. 重複/替代 | ❌ 無替代: 此整合測試角色未被其他測試取代 | 無替代者 |
| 4. 實作狀態 | ✅ 活碼: `go test -tags=integration ./...` (ci-cd.yml integration job) 會執行; 本 PR 實測 `go test -tags=integration .` PASS | 活碼 |
| 5. 影響面 | ⚠️ 與 A 同屬根 package main; 根 package 遷移時需同步改 package 名 (非行為改變) | 已處理 |

**結論**: **保留 (活碼)** — 此檔**不是** enhanced_experiment_runner 的測試 (它不呼叫 harness 任何函式, 直接測 internal 存活元件), 是 CI 有覆蓋的整合測試。k3 修正「決策文件漏 integration_test.go 也要處理」→ 本表即為其判定處理。保留在根, 僅隨根 package 遷移改 `package main` → `package atlas`。

## 5. 根 package `main` 殘留處置 (generate.go 遷移)

**問題**: A 刪除後, 根只剩 `generate.go` (`package main`, 無 `func main`) → `go build ./...` / ci-gate 失敗 (`function main is undeclared in the main package` — 實測驗證)。

**方案選項**:
- ❌ 降為「空 main」: 實測 `go build ./...` 直接失敗 → 不可行。
- ❌ 新增 stub `func main`: 根仍會 `go install .` 產出無用 `atlas-go` binary → 殘留未清。
- ✅ **遷移 generate.go (選用)**: `package main` → `package atlas`, **保留檔案位置與 `//go:generate go run ./cmd/gentags` directive 原樣**。實測驗證: `go generate .` 仍正確執行 gentags (quality.yml:195 的 drift check 不受影響); 根不再被 `go build`/`go install` 當作可執行檔 → 殘留清除。`integration_test.go` 同步改 `package atlas` (同目錄 package 名必須一致)。

**為何「勿碰 generate.go」仍被滿足**: 父指令禁止的是破壞 go generate 掛鉤; 本遷移保留檔案、保留 directive、`go generate .` 實測正常 → 掛鉤完好。此為規劃文件明列的「遷移 generate.go」選項 (規劃 §P3 影響面:「刪除後根 package 需降為空 main 或遷移 generate.go」)。

## 6. 31MB binary 清理

- `enhanced_experiment_runner` (31,770,866 bytes, Mach-O, 2026-07-07 build) 位於主 checkout, **gitignored 未追蹤** (`.gitignore:105` 已於本 PR 移除該忽略項, `.claudeignore:55` 同)。
- 處置: 直接刪除 (build artifact, 源碼已退役)。
- 驗證: `git ls-files` 無此檔; ci-full orphan artifact check (掃 `git ls-files` Mach-O/ELF) 不受影響。

## 7. 承諾條款與複查

- 本決策無「保留待接線」項目 (C 是活碼, 非孤兒/儲備), 故 protocol §承諾條款不觸發。
- 複查點: 若 PRISM Phase A replay executor 未來被移除, harness 亦無需復活 (其角色僅為 dev smoke demo, 正式 pipeline 已涵蓋)。

## 8. 驗證證據 (本 PR 實測)

| 檢查 | 結果 |
|---|---|
| `go build ./...` (需前端 dist) | ✅ PASS |
| `go vet ./...` | ✅ PASS |
| `go test -tags=integration .` (根整合測試保留) | ✅ PASS (0.5s) |
| `go generate ./...` + `git diff --exit-code` (gentags drift) | ✅ 無 diff |
| `go test` 全模組 (ci-full 範圍) | ✅ (見 PR Verification) |
| `make ci-full` | ✅ (見 PR Verification) |

## 9. 範圍外觀察 (非本 PR 處理)

- 模組根有兩個被追蹤的疑似雜訊檔: `=` (0 bytes) 與 `?_pragma=journal_mode(WAL)&...` (200KB, SQLite) — 屬其他 wave 的環境雜訊清單, 非 P3 範圍, 未動。
