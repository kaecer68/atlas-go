# docs/wave-11/ AGENTS.md

> **目錄角色**：Wave 11 工作目錄（含已 ship、未啟動、規劃中文件）。
> **維護**：本目錄只放「Wave 11 工作產出」。已 ship 的永久 reference 移到 `docs/specs/` 或 `docs/guides/`。

## 目前內容

| 檔案 | 狀態 | 用途 |
|------|------|------|
| `L2_4_OBSERVATION.md` | ⚠️ **PLANNED — not yet started** | L2.4 觀察期指標定義 |
| `L2_4_RUNBOOK.md` | ⚠️ **PLANNED — not yet started** | L2.4 ops 觸發流程 |

兩個檔案目前是「未啟動規劃」：`UseLLMSectorAgents` feature flag 預設 `false`（`configs/parameters.json:5860`），observation window 尚未開始。當啟用時改 banner 為「IN PROGRESS」。

## 已移到長期位置的檔案（Wave 11 ship 階段）

| 原路徑 | 新路徑 | 理由 |
|--------|--------|------|
| `AGENT_LOOP_STATE_MACHINE.md` | [`../specs/agent-loop-state-machine.md`](../specs/agent-loop-state-machine.md) | 永久 spec（PR #725、#726 已 ship） |
| `SEMICONDUCTOR_EXECUTOR.md` | [`../guides/adding-sector-agents.md`](../guides/adding-sector-agents.md) | 永久開發指南 |
| `L2_3_PLAN_REFLECT.md` | [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent.md) | L2.3 PoC 設計記錄（v0.0.0.21 已 ship） |

## 判斷準則（新檔進本目錄）

符合以下**任一**條件才放 `wave-11/`：

1. **PLANNED / IN PROGRESS**——未啟動或正在執行的觀察期/觀察 window
2. **Wave-specific 暫存**——例如 `l2-4-log.md`（觀察記錄，啟用後建立）
3. **Wave-specific cleanup**——例如歸檔報告（給 archive 而非 wave-11/）

**已 ship 的永久文件不進 wave-11/**，應直接進 `docs/specs/`、`docs/guides/`、`docs/architecture.md` 等。

## 完整 lifecycle

```
Wave worktree 完成 → 文件分類 → 
  ├─ 永久 reference → docs/specs/ 或 docs/guides/（PR #XXX）
  ├─ 觀察期文件 → docs/wave-11/（帶 status banner）
  └─ 一次性 audit → docs/archive/ 或 docs/branch-hygiene/

Wave 觀察期結束 → 文件歸位 → 
  ├─ 成功 promotion → spec 升級到 docs/architecture.md 或 docs/specs/
  └─ Rollback → 帶 status banner 移到 docs/archive/wave-11-RESOLVED/
```