# traps.md 索引機制改進 — Phase 5 M1 方案 (設計文件)

> **狀態**: 方案草案,待審計 (issue #1465 強制 Gate: 方案 → 審計 → 才 PR 實作)
> **基於**: `/tmp/phase5-index-mockup-2026-08-06.md` M1 (ephemeral staging, 不進 source tree)
> **日期**: 2026-08-06
> **範圍**: traps.md 加 status badge + 引用計數 + frontmatter,讓「哪個 trap 被引用過 / 哪個是新加」可追蹤

---

## 0. 背景真相 (2026-08-06 盤查)

- `docs/reference/traps.md` 目前 **300 行、無 frontmatter、無 status badge、無引用計數**
- 結構: 跨模組陷阱 (7 群組) + 3 個事件回顧 section + 模組特定陷阱 + 文件歸屬規則
- **無 FinMind trap**: grep `finmind` 0 結果 — 20+ 次 FinMind 修補循環未被 trap 框架吸收 (見 finmind-quota-collision.md §7)
- 外部引用 traps.md 的文件 **15+ 份** (docs/specs, docs/operations, docs/investigations 等) — 但無機制知道「哪些 trap 真的被引用」

---

## 1. 問題 (Problem)

| 症狀 | 根因 | 影響 |
|------|------|------|
| 新 trap 加了沒人知道 | 無引用計數 | trap 沉底, 下個 agent 重踩同坑 (FinMind 20+ 次 loop 實證) |
| trap 過時無法識別 | 無 status / 日期 | 無法分辨「現役 vs 已修復 vs 待驗證」 |
| 找不到 trap | 無 frontmatter / 索引 | agent 用 grep 大海撈針 |

---

## 2. 方案 (Design) — M1 三件套

### M1a: frontmatter

traps.md 開頭加 YAML frontmatter:

```yaml
---
title: traps.md — 高危陷阱參考
updated: 2026-08-06
status: active
referenced_by: 15+ 份文件 (docs/specs, docs/operations, docs/investigations)
---
```

### M1b: status badge

每個 trap 群組標題加 badge (表格標題列):

| Badge | 語意 |
|-------|------|
| `[ACTIVE]` | 現役陷阱, 仍可能踩到 |
| `[RESOLVED]` | 已修復, 保留作為歷史教訓 |
| `[NEW]` | 本輪新增 (含日期) |

### M1c: 引用計數

每個 trap 條目加 `引用: N` 欄位 — 由 script 掃描 repo 計算 (grep -rl "trap 名稱關鍵字" docs/ internal/)。

**方式**: 不手動維護 (會過時), 由 `scripts/ci/check_traps_refs.sh` 在 ci-gate 時驗證計數不為 0 的 trap 仍有引用 (防死 trap)。

---

## 3. 上游 / 下游 / 影響範圍

| 方向 | 內容 | 影響 |
|------|------|------|
| 上游 | AGENTS.md 陷阱節引用 traps.md | 無行為改變 (純 doc 標記) |
| 下游 | ci-gate 新增 script | ci-gate 多一個檢查 (輕量 grep) |
| 其他 | 15+ 份引用文件 | 無需改動 |

**相容性**: 純文件 + 1 個 script, 無 code 行為改變。

---

## 4. 測試計畫

| 測試 | 驗證 |
|------|------|
| `check_traps_refs.sh` 自測 | 已知引用陷阱 (如 MCP auth-free) count > 0 |
| ci-gate | 新增 script 通過 |
| markdown-links | traps.md 內 anchor 仍有效 |

---

## 5. 不做的事 (Non-goals)

- M2 (investigations/ INDEX.json) — 另案
- M3 (CI grep guard 改檔必引用) — 另案, 需審計 CI job 影響
- M4 (Atlas 自身對 agent 暴露索引) — 另案, 需 RFC

---

## 6. 審計問題 (給 kaecer)

1. M1 是否值得做? (純 doc 標記 vs 20+ 次 loop 的成本)
2. 引用計數用 script 自動算 (建議) 還是手動維護?
3. 與 M3 (CI guard) 是否該合併成一個 PR?
4. FinMind trap 群組 (finmind-quota-collision.md §7 要求) 是否併入 M1 一起加?

---

## 7. 參考

- `docs/investigations/2026-08-06-finmind-quota-collision.md` §7 (traps.md 影響)
- `/tmp/phase5-index-mockup-2026-08-06.md` M1 (ephemeral)
- Issue #1465 (Phase 5 mockup 候選)
