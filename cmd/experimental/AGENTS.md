# AGENTS.md — cmd/experimental

本目錄包含**驗證、演練與煙霧測試**類 CLI 工具，用於開發與部署前的安全檢查，**不參與日常生產流程**。

---

## OVERVIEW

`cmd/experimental/` 下的每個子命令都是獨立可執行的 `package main`，專注於單一驗證目標。這些工具在 CI 或本地開發時手動觸發，用於確認特定子系統的行為正確性。

| 命令 | 職責 | 觸發時機 |
|------|------|----------|
| `janus-backtest` | JANUS 權重調整的回測驗證 | 修改 JANUS 邏輯後 |
| `janus-status` | 檢視 JANUS 各 cohort 當前狀態 | 調試 regime 判定時 |
| `staging-drill` | 12 秒 live trading 煙霧測試（dry-run） | 部署前驗證 live 管線 |
| `test-hybrid` | HybridProvider 資料回退邏輯驗證 | 修改 provider 後 |
| `validate-broker` | Broker adapter HMAC-SHA256 簽名格式驗證 | 整合新券商 API 前 |
| `validate-narrative-shock` | 敘事事件衝擊場景驗證 | 新增 narrative detector 後 |
| `validate-phase3-integration` | Phase 3 整合測試 | 大版本發布前 |
| `validate-stress-index` | 台灣壓力指數計算驗證 | 修改 stress 邏輯後 |
| `validate-twse-capital-flow` | TWSE 法人買賣超資料解析驗證 | 修改 importer 後 |

---

## CONVENTIONS

- **獨立執行**：每個子命令皆為獨立 `main.go`，可直接 `go run ./cmd/experimental/<name>`。
- **隔離狀態**：涉及檔案系統的工具（如 `staging-drill`）必須使用 `os.MkdirTemp` 建立臨時目錄，禁止寫入生產狀態路徑（`data/state/`）。
- **環境變數**：需外部憑證時（如 `validate-broker`），一律從環境變數讀取，禁止硬編碼。
- **Dummy 模式**：憑證缺失時自動降級為 dummy 驗證（格式檢查），不報錯退出。

---

## ANTI-PATTERNS

- **不可寫入生產資料**：實驗命令禁止修改 `data/state/`、`data/ledger/` 或任何生產路徑。
- **不可跳過隔離**：`staging-drill` 若未使用臨時目錄，會污染生產 state，導致後續實驗結果混亂。
- **不可預設 live broker**：所有驗證命令預設必須是 dry-run 或 paper trading，除非顯式傳入 `-allow-live-broker`。

---

## 常用指令

```bash
# Broker 簽名格式驗證（dummy 模式）
go run ./cmd/experimental/validate-broker

# Live 管線煙霧測試（12 秒，臨時狀態）
go run ./cmd/experimental/staging-drill

# JANUS 回測對比（Baseline vs JANUS 加權）
go run ./cmd/experimental/janus-backtest -start 2026-03-26 -end 2026-03-27

# 壓力指數計算驗證
go run ./cmd/experimental/validate-stress-index
```
