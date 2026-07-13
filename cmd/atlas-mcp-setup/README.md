# cmd/atlas-mcp-setup — 互動式 MCP 客戶端設定 wizard

> **版本**: v0.0.0.32+
> **目的**: 自動偵測已安裝的 MCP 客戶端，產生正確的 `atlas-mcp` 設定 snippet，並驗證 backend 連線。

## 為什麼需要這個工具

Hermes / OpenClaw / Claude Desktop / Cursor / OpenCode 5 種 client 的 MCP 設定檔位置、JSON 結構、env var 命名都不同。手刻設定容易出錯（2026-07-10 hermes agent 摸索 30+ 分鐘）。這個 wizard 解決這個問題。

## 用法

### 互動模式（推薦）
```bash
make setup-mcp
# 或直接
go run ./cmd/atlas-mcp-setup
```

Wizard 會：
1. 偵測已安裝的 MCP 客戶端（掃 `~/.hermes/` / `~/.openclaw/` / `~/Library/Application Support/Claude/` / `~/.cursor/` / `~/.config/opencode/`）
2. 列出找到的 client，讓你選一個
3. 跑 4 個 health probe（atlas-go backend / atlas-mcp binary / atlas-mcp admin / 目標 config 寫入權限）
4. 顯示要寫入的內容，請求確認
5. 寫入（mode 0600）
6. 印出後續步驟（重啟 / reload / 驗證指令）

### 非互動模式（CI / script 用）
```bash
go run ./cmd/atlas-mcp-setup --client opencode --no-prompt --dry-run
# dry-run 不會寫入，只印將產生的 JSON
```

### 完整旗標
| Flag | 說明 |
|------|------|
| `--client <name>` | 指定目標 client：`hermes` / `openclaw` / `claude-desktop` / `cursor` / `opencode` |
| `--no-prompt` | 非互動模式（必須搭配 `--client`） |
| `--dry-run` | 只印將寫入的內容，不修改檔案 |
| `--output <path>` | 覆寫預設 config 路徑 |
| `--binary <path>` | 覆寫自動偵測的 `bin/atlas-mcp` 路徑 |
| `--atlas-base-url <url>` | 覆寫 `ATLAS_BASE_URL` |
| `--atlas-api-key <key>` | 覆寫 `ATLAS_API_KEY` |
| `--force` | 即使 backend probe 失敗也繼續 |

## 偵測邏輯

| Client | 設定檔路徑 | 格式 | 頂層 key |
|--------|-------------|------|----------|
| Hermes | `~/.hermes/config.yaml` | YAML（wizard 寫 JSON，YAML subset）| `mcp_servers` |
| OpenClaw | `~/.openclaw/mcp.json` | JSON5 | `mcp.servers` |
| Claude Desktop (macOS) | `~/Library/Application Support/Claude/claude_desktop_config.json` | JSON | `mcpServers` |
| Claude Desktop (Linux) | `~/.config/Claude/claude_desktop_config.json` | JSON | `mcpServers` |
| Cursor | `~/.cursor/mcp.json` | JSON | `mcpServers` |
| OpenCode | `~/.config/opencode/opencode.json` | JSON | `mcp` |

> 純 YAML（非 JSON-prefixed）的 Hermes 設定檔會被偵測到，wizard 會提示「請手動合併」而非嘗試破壞性編輯。

## 4 個 health probe

1. **atlas-go backend on :18080** — 透過 `internal/portprobe.Probe` 檢查 port 狀態
2. **atlas-mcp binary** — `os.Stat` 確認存在且有執行權限
3. **atlas-mcp admin on :9090** — 透過 `internal/portprobe.Probe` 檢查
4. **target config writable** — 確認目標檔案路徑可寫

任一 probe 失敗時 wizard 會拒絕繼續（除非加 `--force`）。

## 測試

```bash
go test -count=1 ./cmd/atlas-mcp-setup/
```

11 個單元測試（render round-trip、detect 路徑、filterInstalled 排序、findClientByName）。

## 與 `make build-mcp` / `make mcp-status` 的關係

| 工具 | 用途 |
|------|------|
| `make build-mcp` | 編譯 `bin/atlas-mcp` binary |
| `make mcp-status` | 顯示 binary / backend / LLM router 三項健康 |
| `make setup-mcp` | 啟動本 wizard 設定 client |

三者是 MCP 接入的「三件套」。

## 限制

- 純 YAML 格式的 Hermes 設定檔不會被自動合併（會提示手動合併）
- 寫入的設定檔 mode 0600（不含 group-readable）
- 一次只設定一個 client（多 client 設定需重跑 wizard）

## 測試

**單元測試**（11 個，60% 覆蓋率；預設 `go test ./...` 會跑）：
- `render_test.go`：4 種 client config 格式 round-trip、merge 邏輯
- `detect_test.go`：5 種 client 路徑探測（用 `t.TempDir()` mock home dir）
- `coverage_test.go`：14 個補完測試（parseFlags、resolvePaths、Run、printBanner、effectiveBinaryPath 等）

**整合測試**（`//go:build integration` tag；opt-in）：
- `integration_test.go`：mock atlas-go HTTP backend（`httptest.NewServer`）→ 編譯 `bin/atlas-mcp` → 啟動 subprocess → 走 MCP `initialize` → `initialized` → `tools/list` → `tools/call` 完整 round-trip
- 驗證：89 tool count、mock backend 確實被 hit、無外部依賴

執行：
```bash
# 單元測試
go test -count=1 -coverprofile=cov.out ./cmd/atlas-mcp-setup/
go tool cover -func=cov.out

# 整合測試（CI 自動跑，本機 opt-in）
make test-integration
# 或
go test -tags=integration -v -count=1 ./cmd/atlas-mcp-setup/
```

> 整合測試用 `httptest.NewServer` mock backend + newline-delimited JSON-RPC over stdio（**非** LSP-style Content-Length — go-mcp SDK 拒絕 Content-Length framing）。無需 Redis / PostgreSQL / 外部網路。

## 相關資源

- **5 分鐘 SOP**: [`.claude/skills/atlas-mcp-integration/AGENT_quickstart.md`](../../.claude/skills/atlas-mcp-integration/AGENT_quickstart.md)
- **完整 MCP 指南**: [`../atlas-mcp/README.md`](../atlas-mcp/README.md)
