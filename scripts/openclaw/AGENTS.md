# AGENTS.md — scripts/openclaw

> OpenClaw 治理引擎目錄。歷史操作文件已刪除（OpenClaw 已退役，2026-08-17 清理）；新計畫請見 `internal/AGENTS_INDEX.md`。

## ANTI-PATTERNS

- **不可手動修改 baseline_policy.json**：所有政策變更必須透過 `promote-baseline` / `revert-baseline`，禁止直接編輯 JSON。
- **不可跳過閘門**：`verify_governance_gates.sh` 失敗時禁止繼續 promote。
- **不可在生產環境執行 propose**：`propose_mutation.sh` 應在 dev/staging 執行。
- **不可忽略 dry-run**：`decide.sh --dry-run` 輸出應審查後再執行實際操作。

## 常用指令

```bash
bash ./scripts/openclaw/status.sh
bash ./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
bash ./scripts/openclaw/today_start.sh --dry-run
```

> 完整指令與 robot-communication 技能整合見操作文件（規劃中）。
