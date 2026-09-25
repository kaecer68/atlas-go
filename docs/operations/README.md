# docs/operations/ — 操作文件索引

本目錄收錄 atlas-go 的 **操作 runbook 與事件後續追蹤文件**；稽核報告為內部文件（`.omo/audit/`，gitignored）。規範性 / 設計文件請見 [`docs/`](../) 根目錄；模組技術規格見 [`docs/specs/`](../specs/)。

---

## 稽核與盤查報告 (Audits)

稽核報告為內部文件（`.omo/audit/`）。歷史 verification 報告已移至 archive：

| 檔案 | 主題 |
|------|------|
| [tier-boundary.md](tier-boundary.md) | MCP tool / HTTP 端點 / Web UI 三層對照表 + Tier 3 Deprecated 標記彙整 |
| [stock-mcp-query-templates.md](stock-mcp-query-templates.md) | 投資人透過 OpenClaw/Hermes bot 查詢 stock_get_* 工具的查詢模板 |

> 已移至 archive: `v2-alert-redesign-verification-report.md`、`frontend-refactor-recovery-verification-report.md` → [`../archive/`](../archive/)

> 稽核報告為內部文件（.omo/audit/，gitignored）。

## Runbook（操作手冊）

| 檔案 | 主題 |
|------|------|
| [l2-4-runbook.md](l2-4-runbook.md) | L2.4 觀察窗口操作手冊 |
| [wave9-runbook.md](wave9-runbook.md) | Wave 9 observability 系統 runbook |

> 已移至 .omo/：`l2-4-observation-log.md`（.omo/evidence/）、`l2-4-followup.md`（.omo/manifests/）
| [mcp-deploy.md](mcp-deploy.md) | atlas-mcp server 部署手冊 |
| [loki-deployment.md](loki-deployment.md) | Loki 部署手冊 |

## 政策與流程

| 檔案 | 主題 |
|------|------|
| [cmd-atlas-coverage-policy.md](cmd-atlas-coverage-policy.md) | cmd/atlas 測試覆蓋率政策 |
| [rss-feed-replacement.md](rss-feed-replacement.md) | RSS feed 替代方案 |

## 事件後續追蹤與已知限制 (Follow-ups)

| 檔案 | 主題 |
|------|------|
| [FOLLOWUPS.md](FOLLOWUPS.md) | 待辦 / 已知限制**登記表**（刻意延後且只建議未實作的項目；每個條目附可重現的來源） |
| [universe-scoring-ranked-zero-20260925.md](universe-scoring-ranked-zero-20260925.md) | SmartUniverseBuilder `symbols_ranked=0` 根因調查報告（含 §6.2 未覆蓋缺口 / §8 防再犯檢查 / §9 已知限制） |

> **驗收判讀（2026-09-25 實證；已實際造成一次誤判，含 root 本人）**：部署／重啟後
> atlas-go 的 `/metrics` 是 **in-memory**（`internal/bootstrap/bootstrapper.go` 的 `InitMetrics()`）
> ⇒ **app 指標 family 會消失，直到下一次排程執行**（daily universe 最多 24 小時；跨週末/連假更久）。
> 所以驗收時看到 `curl -s localhost:18080/metrics | grep atlas_universe` 回 **0 不等於 wiring 壞掉**；
> 而且手動跑 `-build-universe run` **不會**更新 Prometheus 的 `atlas_universe_*`
> （CLI 自建 deps、不寫入服務 collector）。判讀規則與證據見
> [FOLLOWUPS.md](FOLLOWUPS.md) 的「判讀註記」與 FU-20260925-07 / FU-20260925-09。

---

## 命名規範

- **稽核 / 報告**：`audit-<日期>-<主題>.md` 或 `<主題>-<類型>.md`（類型如 `audit` / `runbook` / `verification-report`）
- **事件後續追蹤**：`<事件名>-followup.md`（單一事件）；跨事件的**待辦/已知限制登記**用 [FOLLOWUPS.md](FOLLOWUPS.md)
- **設計文件**：`docs/specs/`（不在本目錄）

## 維護原則

- 任何 PR 引用本目錄的檔案時，commit message 必須附實際檔名（例如 `Refs: docs/operations/l2-4-runbook.md`）
- 本目錄文件若引用其他文件，連結路徑必須以 `bash scripts/ci/check_markdown_links.sh` 驗證通過（CI 強制）
- 規範性內容優先放 [`docs/`](../) 根目錄；本目錄只放 **操作相關** 文件
