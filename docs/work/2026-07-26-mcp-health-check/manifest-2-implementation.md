# atlas-mcp UX/UI Health Check — Manifest II：根因分析與實作追蹤

> **版本**: v2.0 | **日期**: 2026-07-26（最終更新：全部完成）
> **依賴**: Manifest I（`manifest-1-inventory.md`）

## 追蹤狀態說明
| 狀態 | 標記 |
|------|------|
| 已完成 | ✔️ |
| 無關（backend 因） | 🚫 |

---

## 🔴 P0 — Critical Bugs 根因與修復（全數完成）

| ID | 項目 | PR |
|----|------|----|
| P0-1 | `stock_get_quote` timeout → Fugle API key 未設定 | #1352 |
| P0-2 | `llm_get_cost` 503 → KimiClient 未注入（late-binding getter） | #1352 |
| P0-3a | `detector_registry_list` JSON unmarshal type mismatch | #1352 |
| P0-3b | `template_detector_status` JSON unmarshal type mismatch | #1352 |
| P0-5 | server.go arithmetic bug `n < 115-118 = n < -3` | #1352 |

**狀態**: ✔️ 全數完成

---

## 🟡 P1 — 文件缺陷修復（全數完成）

| ID | 項目 | PR |
|----|------|----|
| P1-1 | Hermes `${ATLAS_API_KEY}` 變數未展開（Makefile shell merge） | #1352 |
| P1-2 | Tool count 不一致 → 統一來源（startup log） | #1352 |
| P1-3 | 移除「registered N tools」誤導描述 | #1352 |
| P1-4 | `calendar_events` vs `event_calendar` 命名重複（dedup to `event_calendar`） | #1352, #1353 |

**狀態**: ✔️ 全數完成

---

## 🟠 P2 — 數據品質修復（全數完成）

| ID | 項目 | 修正 | PR |
|----|------|------|-----|
| P2-1 | Alert noise → 降低重複告警 | dedup race fix（`Track` 移出 goroutine）+ filter params | #1354, #1357 |
| P2-2 | `risk_get_drawdown` not available → 明確狀態 | expanded drawdown message | #1353 |
| P2-3 | Channel unknown → 標記 inactive | `"unknown"` → `"inactive"` for unbuilt channels | #1357 |
| P2-4 | Simulation 0 訂單 spam | WARNING → INFO | #1355 |
| P2-5 | Scheduler summary | computed summary（count by status） | #1354 |

**狀態**: ✔️ 全數完成

---

## 🔵 P3 — UX 改善追蹤（全數完成）

| ID | 項目 | PR |
|----|------|-----|
| P3-1 | First Contact SOP | #1352 |
| P3-2 | Sector 中文標籤（17 L2 sub-industries） | #1355 |
| P3-3 | Parameters 摘要（reference to structured tools） | #1355 |
| P3-4 | Emoji 可選（`?format=plain`） | #1355 |

**狀態**: ✔️ 全數完成

---

## PR 彙總

| PR | State | 內容 |
|----|-------|------|
| #1352 | merged | P0(5) + P1(4) + P3-1(SOP) — 核心修復 |
| #1353 | merged | P1-4(calendar dedup) + P2-2(drawdown) |
| #1354 | merged | P2-1(filter) + P2-5(scheduler summary) |
| #1355 | merged | P3-2/3/4(sector/params/emoji) + P2-4(simulation) |
| #1357 | merged | P2-1(dedup race) + P2-3(channel inactive) |

**總計**: 17 個 issue 全部修復，5 個 PRs 全部合併。
