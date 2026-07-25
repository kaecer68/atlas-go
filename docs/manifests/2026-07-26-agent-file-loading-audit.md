# Audit Manifest: Agent 文件載入鏈審計

> **Audit source**: 使用者從 opencode 遷移到 oh-my-pi，發現文件讀取路徑不一致
> **Goal**: 確保所有 agent 環境都能讀到完整的專案紀律，消滅文件 drift 與重複
> **Created**: 2026-07-26
> **Status**: done（全部 F01-F10 完成，PR #1356）

---

## Invariant Tracker

| ID | Severity | Problem | Fix | Status |
|----|----------|---------|-----|--------|
| F01 | 🔴 | oh-my-pi 不載入 repo CLAUDE.md/AGENTS.md | copilot-instructions.md 加 `@CLAUDE.md` | ✅ done |
| F02 | 🔴 | ~/.agents/ 與 ~/.config/opencode/ AGENTS.md 雙副本分歧 | symlink ~/.config/opencode/ → ~/.agents/ | ✅ done |
| F03 | 🟡 | 3 處 broken ref 指向 ~/.config/opencode/AGENTS.md | 改為 ~/.agents/AGENTS.md | ✅ done |
| F04 | 🟡 | Tool count 四處不一致 | 全部指向 tool-catalog.md | ✅ done |
| F05 | 🟢 | AGENTS.md 版本資訊過時 | 更新到 v0.0.2.0 | ✅ done |
| F06 | 🟢 | copilot-instructions.md 與 go-core.instructions.md 指令重疊 | 刪除重疊，指向 go-core | ✅ done |
| F07 | 🟢 | AGENTS.md 逼近 160 行 hard limit | 提高至 180 | ✅ done |
| F08 | 🔴 | AI push 後未自動 gh pr create | AGENTS.md:64 加入明確步驟 | ✅ done |
| F09 | 🔴 | 分支命名無日期前綴，易混淆 | AGENTS.md:64 加入 YYYYMMDD 規範 | ✅ done |
| F10 | 🟡 | VERSION sync 機制存在但 CI 未接入，版本 drift 無人發現 | check_versions.sh 接入 quality.yml | ✅ done |

---

## Q&A（使用者三問）

### Q1: 全局與專案的 AGENTS.md 都會讀到嗎？

**會。** oh-my-pi 載入鏈：
- `~/.claude/CLAUDE.md` → `@~/.agents/AGENTS.md`（全局）
- `.github/copilot-instructions.md` → `@CLAUDE.md` → `@AGENTS.md`（專案）

### Q2: 子目錄的 AGENTS.md 會自動讀到嗎？

**不會。** oh-my-pi 不像 opencode 依工作目錄動態載入。`internal/*/AGENTS.md`（15 個）需透過 skills 系統或 AI 主動 read。

### Q3: 版本編號有同步機制嗎？

**有，但之前未接入 CI。** `VERSION` 檔 → `make sync-version` → `scripts/sync-version.sh` → 同步 5 個 TARGETS。`check_versions.sh` 檢查 drift 但未在 CI 中。F10 已修復。

---

## Change Log

| Date | Version | Change |
|------|---------|--------|
| 2026-07-26 | 1.0 | 初始審計，F01-F07 |
| 2026-07-26 | 2.0 | F08-F09（AI 紀律），F01-F07 執行 |
| 2026-07-26 | 3.0 | F10（VERSION sync CI 接上），Q&A 記錄 |
