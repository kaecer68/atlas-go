# AGENTS.md — cmd/experimental

驗證與煙霧測試 CLI 工具，**不參與生產流程**。

## ANTI-PATTERNS

- 不可寫入生產資料：禁止修改 `data/state/`、`data/ledger/` 或任何生產路徑。
- 不可預設 live broker：預設 dry-run/paper，除非顯式傳入 `-allow-live-broker`。
- 憑證缺失 → dummy mode：自動降級為格式檢查，不報錯退出。

> 完整 CLI 目錄清單見 `docs/QUICKSTART.md`。
