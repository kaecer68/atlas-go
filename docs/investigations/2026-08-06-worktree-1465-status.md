# Worktree 狀態 — Issue #1465 (2026-08-06)

> **目的**: 記錄 2026-08-06 對 issue #1465 的盤查結論 + 為何 worktree 暫無新工作
> **狀態**: Read-only placeholder, 等 kaecer 決策

---

## 1. Issue #1465 當前實際狀態 (2026-08-06, codegraph + git log 驗證)

對照 issue body 與 main (HEAD=e878a10b) 真實狀態:

| Issue body 要求 | 當前 main 狀態 | 證據 |
|----------------|----------------|------|
| A1 Reviewer 8 finding doc amend | ✅ 已完成 | `docs/investigations/2026-08-06-finmind-quota-collision.md` line 361 「✅ amended 2026-08-06」 |
| A2 P1.10 production 驗證 | ✅ 已完成 | 同 doc §5.1.1 (P1.10 實證 2026-08-06 05:10 UTC 完成) |
| A3 14-11-16 industry count 修正 doc | ✅ 已完成 | `docs/investigations/2026-08-06-industry-count-correction.md` (66 行完整 doc) |
| Phase 4 hotfix (HF-1) 方案 | ✅ 已完成 | `docs/investigations/2026-08-06-finmind-402-hotfix-plan.md` (含 HF-1a/1b/1c + 4 個審計問題) |
| Phase 4 hotfix PR 實作 | ✅ 部分完成 | PR #1472 (finmind hf1-ab) + PR #1473 (finmind hf1c-ctx) 已 merge |
| Phase 5 traps-index mockup | ✅ 部分完成 | `docs/investigations/2026-08-06-traps-index-mockup-plan.md` + PR #1474 (traps-index M1) 已 merge |
| Binary freshness | ❌ stale | 6 binary 過期 (HEAD e878a10b, docker image 98947773) |
| fubon-proxy test port conflict | ⚠️ flaky | Kimi.app daemon 佔 port 58853 — 與 PR #1463 無關 |

---

## 2. 剩餘工作 (kaecer 決策後才能動)

### 2.1 Phase 4 hotfix 審計問題 (待 kaecer)

`docs/investigations/2026-08-06-finmind-402-hotfix-plan.md` §6 列 4 個問題:
1. HF-1a + HF-1b + HF-1c 是否一個 PR？拆開？
2. 402 歸類 `quota` (與 ErrQuotaExhausted 同類) 是否正確？
3. HF-1c 選項 A (10s ctx) vs 選項 B (rate limiter 調整)？
4. HF-1d (402 metric) 是否併入？

**AI 不能�過**: 需 kaecer 回答後才能進 PR 實作。

### 2.2 Binary freshness

issue #1465 §F 明確: 「AI 禁止自己跑 `make rebuild-all` 或 `docker compose`」

→ 等 kaecer 在主 worktree 跑 `make rebuild-all`。

### 2.3 PR #1464 link 修復

issue #1465 第二則 comment: PR #1464 是 kaecer 開的, link 引用 `.omo/` 路徑在 `.gitignore` 內永遠 broken。

**責任歸屬**: kaecer 自己修。AI 無權擅自改 PR #1464。

---

## 3. 為何 worktree 仍存在

雖然「無新工作可做」,但保留 worktree 理由:
1. **future reference**: 若 kaecer 對 §2.1 4 個問題給決策,worktree 已就緒可立即進 PR 實作
2. **避免重建成本**: orca worktree create 有 setup hook 開銷,留下比重建便宜
3. **明確狀態**: 寫這份 status note 讓人或未來 session 知道「不是空 worktree,是有意識保留」

---

## 4. 給下個 session 的指引

若開新 session 看到這份 note:
- **不要直接動 production code** — Phase 4 hotfix 剩餘項目需 kaecer 決策
- **不要跑 docker rebuild** — 二級禁令
- **不要改 PR #1464** — 責任在 kaecer
- **可做**: 若 kaecer 已回答 §2.1 4 個問題,可依答案進 PR 實作 (但 binary 仍需 fresh)

---

## 5. Worktree metadata

- Branch: `fix/20260806-finmind-doc-amend`
- Path: `wt-1465-finmind-doc/`
- Created: 2026-08-06
- Created by: orca-cli worktree create
- HEAD: e878a10b44a676b8a568703ef4b5afd497bade1f
- Uncommitted changes: 0
