# AGENTS.md — cmd/experimental

本目錄放的是**驗證、drill、smoke-test 類 CLI**，不是日常主流程命令。

---

## OVERVIEW

這些命令的用途是快速驗證某個子系統是否可運作：JANUS、broker adapter、stress index、Phase 3 integration、staging live drill、資料 provider 混合路徑。它們通常：

- 直接 `config.Load()` 或手動組一份極小 `config.Config`
- 使用短時間窗口 / temp dir / mock provider
- 產出 markdown / json / console summary 作為 smoke proof

**不要**把這裡的 flag / hardcoded 路徑當成 production contract。

---

## WHERE TO LOOK

| 任務 | 位置 | 備註 |
|------|------|------|
| 驗證 JANUS 權重差異 | `janus-backtest/` | baseline run vs JANUS-enabled run，預設短窗口 |
| 做 live staging drill | `staging-drill/` | 強制 `paper` mode、temp state、12 秒 smoke test |
| 驗證 broker 簽名 | `validate-broker/` | 本地 `httptest` server 檢查 HMAC header 形狀 |
| 驗證 Phase 3 整合 | `validate-phase3-integration/` | PRISM / Swarm / Spawning / Reflexivity 短窗整合 |
| 驗證 stress index | `validate-stress-index/` | 從 replay CSV 推算台灣壓力指數 |
| 驗證 narrative shock / capital flow | `validate-narrative-shock/`, `validate-twse-capital-flow/` | 各自單一子系統驗證 |

---

## CONVENTIONS

- CLI 幾乎都支援 `--help`，輸出用途而不是完整操作手冊。
- 預設偏向**安全 smoke path**：temp dir、mock provider、paper mode、短窗口。
- 若需要真實憑證，通常只做格式驗證；像 `validate-broker` 沒有憑證時會退回 dummy mode。
- 常見輸出格式是 `markdown|json` 二選一，供人工閱讀或腳本串接。

---

## ANTI-PATTERNS

- **不要把 experimental CLI 當 production entrypoint**：正式流程仍以 `cmd/atlas`、`cmd/execute-experiment`、`cmd/judge-experiment`、`cmd/promote-baseline` 為主。
- **不要依賴 hardcoded replay path 永久存在**：多數命令內嵌 `data/replay/tw_extended_90days.csv` 或固定日期，只適合驗證。
- **不要拿 staging-drill 驗證 live broker 風險邊界**：它明確把 `BrokerMode` 強制成 `paper`。
- **不要把 smoke-test 成功解讀為策略有效**：這裡驗證的是 wiring / contract / integration，不是投資績效。
- **不要把這些命令的輸出 schema 當穩定 API**：它們是 operator tooling，欄位可隨驗證需求調整。

---

## NOTES

- 若要改 shared logic，優先回到 `internal/janus/`、`internal/live/`、`internal/narrative/`、`internal/orchestrator/` 修改；此目錄應保持薄。
- 若新增新的 validator / drill，命名延續 `validate-*`、`*-drill`、`test-*` 慣例，並提供 `--help`。
