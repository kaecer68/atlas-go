# Worktree 狀態 — Issue #1465 (2026-08-06 修正)

> **目的**: 記錄 2026-08-06 對 issue #1465 的真實盤查結論（基於 grep 程式碼證據）
> **狀態**: ⚠️ 修正先前錯誤評估 — HF-1 **已全部 merge**, 剩餘工作 = 0 (除 binary rebuild 與 PR #1464 link)
> **修正時間**: 2026-08-06 (commit 04653e5d 之後, 重新 grep 程式碼)

---

## 1. 先前評估錯誤（承認）

先前 status note (commit 04653e5d) 寫「剩餘: Phase 4 hotfix 4 個審計問題待 kaecer」 — **這是錯的**。

**根因**: 我寫那份 note 時**只讀了方案文件** (`finmind-402-hotfix-plan.md`), **沒去 grep 程式碼驗證方案是否已實作**。

`方案文件 ≠ 已實作程式碼` — 這是盤查基本失誤。

---

## 2. 2026-08-06 grep 證據 — HF-1 全部已 merge

### 2.1 HF-1a+1b (error 透傳 + 402 分類)

| 證據 | 位置 | 內容 |
|------|------|------|
| Commit | `aadc354f` (merge PR #1472) | `fix(industry): #1465 HF-1a+b — 透傳 rate-limit/402 error + classify 402→quota` |
| `classifyFinMindError` 函式 | `internal/industry/data_aggregator.go:407-422` | 已有完整三段判斷 (ErrQuotaExhausted / ErrRateLimited / isFinMindQuotaOrRateLimited 字串) |
| `isFinMindQuotaOrRateLimited` 函式 | `internal/industry/data_aggregator.go:387-405` | 已有, 註解明確引用 Issue #1465 P1.10 與 HF-1b |
| `ErrRateLimited` sentinel | `internal/marketdata/errors.go:5-6` | `var ErrRateLimited = errors.New("rate limited")` |
| `ErrQuotaExhausted` sentinel | `internal/marketdata/finmind_client.go:48` | `var ErrQuotaExhausted = fmt.Errorf("finmind: daily quota exhausted")` |
| 402 字串比對位置 | `classifyFinMindError` line 420 | 走 `isFinMindQuotaOrRateLimited(err)` 後 return "quota" |

**結論**: **HF-1a + HF-1b 完全實作**, 不需任何 code 改動。

### 2.2 HF-1c (5s ctx → 10s ctx)

| 證據 | 位置 | 內容 |
|------|------|------|
| Commit | `74894b0d` (merge PR #1473) | `fix(industry): #1465 HF-1c — fetch ctx 5s→10s 對齊 rate limiter 6s token` |
| `fetchRevenueYoY` ctx | `internal/industry/data_aggregator.go:190` | `context.WithTimeout(ctx, 10*time.Second)` |
| `fetchProfitYoY` ctx | `internal/industry/data_aggregator.go:245` | `context.WithTimeout(ctx, 10*time.Second)` |

**結論**: **HF-1c 完全實作** (兩個函式都改完)。

### 2.3 HF-1d (402 Prometheus counter)

`docs/investigations/2026-08-06-finmind-402-hotfix-plan.md` §2.4 明文標 "**不納入** (獨立 PR, 涉及 monitoring 框架)" — **不是 scope**, 不算剩餘工作。

---

## 3. Issue #1465 完成定義對照 (2026-08-06)

| 定義項 | 狀態 | 證據 |
|--------|------|------|
| 1. Reviewer 8 個 finding 全部 amend commit | ✅ | commit `aadc354f` + `74894b0d` + finmind-quota-collision.md §362 |
| 2. P1.10 production 驗證結果寫進 doc section 5 | ✅ | finmind-quota-collision.md §5.1.1 (2026-08-06 05:10 UTC 完成) |
| 3. 14/11/16 個 industry 跨 doc 歷史污染完整修正 | ✅ | industry-count-correction.md (commit 內) |
| 4. Phase 4 hotfix 至少 1 條 (HF-1) 走完方案 → 審計 → PR → production | ✅ | 方案 c2efeb3d + PR #1472 (a+b) + PR #1473 (c) — **全部走完** |
| 5. Phase 5 mockup 至少 1 個 (M1) 走完方案 → 審計 → PR | ✅ | PR #1474 (traps-index M1) merge |
| 6. make check-binaries ALL BINARIES FRESH | ❌ | 仍 stale (HEAD e878a10b, docker image 98947773) |
| 7. make ci-full 過 | ❌ | binary stale 阻擋 |

---

## 4. 真實剩餘工作

### 4.1 Issue #1465 本身 (屬於 AI/方案 scope)

**0 項**。所有 issue body 列的 7 項完成定義中 5 項 ✅, 2 項 binary-stale 不在 AI scope。

### 4.2 不在 issue #1465 scope 但相關

| 項目 | 責任歸屬 | 為何不在 issue #1465 |
|------|----------|---------------------|
| Binary rebuild (docker image 98947773 → e878a10b) | kaecer 主 worktree | CLAUDE.md 「AI 禁止 docker 操作」 |
| PR #1464 link 修復 (.omo/ → .claude/agent-memory/) | kaecer | PR #1464 是 kaecer 開的, link 是 PR 內 commit 寫的 |
| fubon-proxy test port 58853 flaky (Kimi CLI daemon) | kaecer (test infrastructure) | pre-existing flaky, 與 #1465 無關 |

---

## 5. 為何 worktree 仍存在 + 不刪除

- **worktree 仍有 git 物件**: 2 個 commit (1 個 status note 錯誤版本 + 後續修正 commit)
- **history 保留**: 修正 commit 留下 audit trail, 未來 session 可讀 git log 看到「我先前寫錯 → 修正」
- **刪除成本 > 保留成本**: orca worktree rm + 重 create 有 setup 開銷
- **明確標記狀態**: 這份修正 note 讓人或未來 session 知道「#1465 worktree 已完成, 僅留 audit」

---

## 6. 給下個 session 的指引

若開新 session 看到這份 note:
- **不要重做 HF-1 任何子項** — 全部已 merge
- **不要重新設計 Phase 4 hotfix 方案** — 走完了
- **不要 grep 找 `classifyFinMindError` 改動空間** — 已是完整版
- **若 kaecer 指示 push**: `git push origin fix/20260806-finmind-doc-amend` 後 close
- **若 binary 仍未 fresh**: 不要做 cmd/atlas 整合測試驗證

---

## 7. Worktree metadata (修正版)

- Branch: `fix/20260806-finmind-doc-amend`
- Path: `wt-1465-finmind-doc/`
- Created: 2026-08-06
- Commits:
  - `04653e5d` (status note v1, 錯誤評估, 保留作 audit)
  - `<this commit>` (status note v2, 修正)
- HEAD: `<this commit>`
- 真實剩餘工作: **0**
- 關閉條件: 推送 + close
