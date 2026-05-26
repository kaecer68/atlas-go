# Atlas-Go 快速入門

> 5 分鐘內啟動 atlas-go 投資研究系統。

## 環境需求

- **Go** 1.25+
- **PostgreSQL** 15（持久化）
- **Redis** 7（快取 / nonce store）

## 第一步：確認環境

```bash
go version          # 需 ≥ 1.25
go build ./...      # 確認全部編譯通過
go test ./...       # 確認測試通過
```

## 第二步：啟動系統

```bash
# 啟動 HTTP server（預設 port 8080）
go run ./cmd/atlas

# 檢查健康狀態
curl http://localhost:8080/health
```

開啟瀏覽器：`http://localhost:8080` — Atlas 儀表板（含產業生態系、決策鏈、即時監控）。

## 第三步：執行實驗

```bash
# 1. 執行實驗
go run ./cmd/run-experiment -brief <brief-file>

# 2. 評判結果（自動尋找最新實驗）
go run ./cmd/judge-experiment

# 3. 若 accepted → 晉升 baseline
go run ./cmd/promote-baseline
```

## 第四步：執行回測

```bash
# 指定日期範圍的回測
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

## 第五步：資料匯入

```bash
# CSV → JSONL（供 replay 使用）
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

## 常用命令一覽

| 命令 | 用途 |
|------|------|
| `go run ./cmd/atlas` | 啟動 HTTP server |
| `go run ./cmd/run-experiment -brief <file>` | 執行實驗 |
| `go run ./cmd/judge-experiment` | 評判最新實驗 |
| `go run ./cmd/promote-baseline` | 晉升 baseline |
| `go run ./cmd/revert-baseline --list` | 查看可回滾版本 |
| `go run ./cmd/backtest-window -start -end` | 回測指定區間 |
| `go run ./cmd/import-replay` | CSV → JSONL 匯入 |
| `go run ./cmd/mapgen -map arch` | 生成系統架構圖 |
| `go run ./cmd/calibrate-seasonal` | 校準季節性模式 |

## 驗證檢查

```bash
# 格式檢查（CI blocker）
test -z "$(gofmt -l .)"

# 完整 CI 檢查
go build ./...
go test ./...
go vet ./...
staticcheck ./...
golangci-lint run ./...
```

## 關鍵檔案

| 檔案 | 用途 |
|------|------|
| `AGENTS.md` | 開發代理工作守則 |
| `ai_productivity_guide.md` | 常見陷阱與除錯指南 |
| `configs/agents.json` | 代理註冊表 |
| `.env` | 環境變數（`ATLAS_*` 前綴） |
| `docs/architecture.md` | 系統架構說明 |
| `docs/operations_playbook.md` | 操作手冊 |
| `docs/iteration_playbook.md` | 迭代指南 |

## 安全提醒

1. **Live trading**: 本地測試時切勿啟用 `-allow-live-broker`、`-allow-real-signor` 旗標
2. **API Key**: 所有外部 API key 透過 `.env` 管理，不提交至 git
3. **Baseline**: 實驗執行/評估前確認 `data/state/baseline_policy.json` 存在
