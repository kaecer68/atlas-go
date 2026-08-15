# atlas-go root Makefile
#
# 統一管理 2 個前端目錄(admin_web / client_web)+ Go backend + CI scripts。
# 解決新人 onboarding 要手動對 2 目錄各跑 npm ci/build 的摩擦。
#
# 用法:
#   make help          列出所有可用 target
#   make install       安裝所有前端 + Go 依賴
#   make build         編譯所有前端 + Go backend
#   make test          跑前端單元測試 + Go 測試
#   make lint          跑 Go linters
#   make ci            跑 scripts/ci/ 下所有檢查腳本
#   make clean         清除 node_modules / dist / build artifacts
#   make watch-frontend  啟動 esbuild --watch 模式(每個前端目錄)
#   make smoke         跑前端 smoke test(需 backend 已啟動)

.PHONY: help install build test lint ci clean watch-frontend smoke
.PHONY: install-frontend build-frontend test-frontend
.PHONY: build-backend test-backend lint-backend
.PHONY: build-mcp install-mcp mcp-status setup-mcp install-atlas-mcp-from-release
.PHONY: dev dev-stop dev-status dev-logs
.PHONY: status
.PHONY: ci-gate ci-full pre-push import-history

# pre-push — 本地 push 前驗證（由 .githooks/pre-push hook 自動執行）
#   ci-gate  (~30s)   永遠執行：gofmt/build/vet/generate-drift/ci-quick
#   ci-full  (~5-8min) 僅當 diff 含程式碼變更時執行：lint/staticcheck/test/race/coverage
#   目的：在本地先抓出 CI 會失敗的問題，避免 push→CI-fail 往返浪費
#   手動用法：make pre-push            （同 hook 邏輯：auto）
#             PRE_PUSH_FULL=always make pre-push   （強制全量）
#             PRE_PUSH_FULL=never  make pre-push   （只跑 ci-gate）
pre-push: ci-gate
	@echo ""
	@if git diff --name-only origin/main...HEAD 2>/dev/null | grep -qE '\.(go|ts|tsx|js|jsx|vue|py|sh|c|cpp|h)$$|(^|/)Makefile$$|Dockerfile|docker-compose|\.github/|\.githooks/'; then \
		echo "🧪 diff 含程式碼變更 → 跑 make ci-full (~5-8 min)..."; \
		$(MAKE) ci-full; \
	else \
		echo "ℹ️  docs-only diff — ci-gate 已足夠（ci-full 跳過）"; \
	fi


# 前端目錄列表(若有新增,加在這裡)
FRONTENDS := admin_web client_web
GO_PKGS   := ./cmd/... ./internal/...

# ---- 版本管理(VERSION 檔為 single source of truth)----
# 用法:
#   make version           印當前 VERSION
#   make sync-version      從 VERSION 同步所有 hardcoded 版本字串到 doc files
#   make bump-version      互動式 bump VERSION(prompt 輸入新版本)
# 在 cmd/*/main.go 用 ldflags 注入 -X main.Version=$(VERSION),見 build-backend。
VERSION := $(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo dev)
LDFLAGS_VERSION := -X main.Version=$(VERSION)

# ---- Buildinfo linker-injected runtime parity (E08)----
# internal/buildinfo exposes Version/Commit/BuildTime package vars intended to
# be overridden via -ldflags -X. These values are surfaced by
# SystemHealthResponse.Runtime so dashboards can audit a deployed binary
# against its source commit (CF-INV-12). When not running inside a git
# checkout (e.g. a release tarball), Commit falls back to "unknown".
export GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS_RUNTIME := -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=$(VERSION) \
                   -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=$(GIT_COMMIT) \
                   -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=$(BUILD_TIME)

.PHONY: version sync-version bump-version

version:
	@echo "$(VERSION)"

sync-version:
	@bash scripts/sync-version.sh

# ---- 雙機同步 (2026-08-15) ----
.PHONY: sync-imac
sync-imac: ## 同步 iMac production clone + 可選部署 (a2a-sync)
	@echo "→ 同步 GitHub → iMac (atlas)"
	@~/bin/a2a-sync
	@echo "✓ 完成 (部署用: make sync-imac-deploy)"

.PHONY: sync-imac-deploy
sync-imac-deploy: ## 同步 + 重建部署 atlas 到 iMac
	@~/bin/a2a-sync --deploy


bump-version:
	@read -p "Bump from $(VERSION) to (e.g. 0.0.0.30): " v; \
	echo "$$v" > VERSION; \
	echo "✓ Updated VERSION to $$v"; \
	echo "→ Run 'make sync-version' to update hardcoded doc references"

# 預設 target: 顯示 help
help:
	@echo "atlas-go root Makefile"
	@echo ""
	@echo "Dev workflow (單一指令 'go run ./cmd/atlas -api' 啟動):"
	@echo "  dev                啟 docker deps + stop atlas container + go run 原生"
	@echo "  dev-stop           收尾(停 docker deps)"
	@echo "  dev-status         看容器狀態 + port 18080/18081 占用 + native process"
	@echo "  status             只看 port 18080/18081 占用 + PID/命令(對應 /health JSON)"
	@echo "  dev-logs           tail atlas-go 啟動 log"
	@echo ""
	@echo "前端管理:"
	@echo "  install-frontend   安裝所有前端依賴 (admin_web / client_web)"
	@echo "  build-frontend     編譯所有前端 (npm run build x2)"
	@echo "  test-frontend      跑前端單元測試"
	@echo "  watch-frontend     啟動 esbuild --watch 模式"
	@echo ""
	@echo "後端管理:"
	@echo "  build-backend      編譯 Go backend (cmd/atlas)"
	@echo "  test-backend       跑 Go 單元測試"
	@echo "  lint-backend       跑 gofmt / go vet / staticcheck"
	@echo ""
	@echo "MCP server (atlas-mcp) — 給外部 AI agent 接入用:"
	@echo "  build-mcp          編譯 bin/atlas-mcp"
	@echo "  install-mcp        安裝到 ~/.local/bin (MCP client command 路徑)"
	@echo "  mcp-status         檢查 binary / atlas-go / LLM router 三項健康"
	@echo "  setup-mcp          啟動互動式 wizard (開發者用)"
	@echo "  setup-mcp-agent    hermes 等 agent 用的一條龍安裝 + 設定 + 驗證"
	@echo "                    (自動 source ~/.config/atlas-go/.env 取共用 dev key)"
	@echo "  verify-mcp-setup   驗證 hermes 真的能用 atlas-mcp (89 tools 連線)"
	@echo "  install-atlas-mcp-from-release"
	@echo "                    從 GitHub Release 下載 atlas-mcp + SHA256 verify"
	@echo ""
	@echo "整合:"
	@echo "  install            install-frontend + 下載 Go 模組"
	@echo "  build              build-frontend + build-backend"
	@echo "  test               test-frontend + test-backend"
	@echo "  lint               lint-backend"
	@echo "  ci                 跑 scripts/ci/ 下所有 check_*.sh (per-script timeout 30s)"
	@echo "  ci-quick           只跑快速 check(<2s,適合 PR 前)"
	@echo "  ci-slow            只跑慢速 check(data_naming/layer3/markdown)"
	@echo "  smoke              跑前端 smoke test(需 backend 已啟動)"
	@echo "  clean              清除 build artifacts"
	@echo ""
	@echo "範例:"
	@echo "  make install build   # 一次性安裝 + 編譯所有"
	@echo "  make dev             # 啟動 dev 環境(推薦用於本地開發)"
	@echo "  make ci              # 跑所有 CI 檢查(PR 前必跑)"

# ---- 個別 target ----

install-frontend:
	@echo "📦 Installing frontend dependencies..."
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir"; \
		(cd $$dir && npm ci) || exit 1; \
	done

build-frontend:
	@echo "🔨 Building frontend..."
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir"; \
		(cd $$dir && npm run build) || exit 1; \
	done

test-frontend:
	@echo "🧪 Testing frontend..."
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir (skip if no test script)"; \
		(cd $$dir && npm test --if-present) || exit 1; \
	done

watch-frontend:
	@echo "👀 Starting esbuild --watch in 2 terminals..."
	@echo "   (建議改用 'make watch-frontend-$$dir' 分別啟動)"
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir"; \
		(cd $$dir && npm run watch) & \
	done
	@wait

watch-frontend-%:
	@echo "👀 Starting esbuild --watch for $* ..."
	cd $* && npm run watch

build-backend:
	@echo "🔨 Building Go backend (cmd/atlas)..."
	go build -ldflags "$(LDFLAGS_VERSION) $(LDFLAGS_RUNTIME)" -o bin/atlas ./cmd/atlas

# ---- MCP server (atlas-mcp) management ----
# 給外部 AI agent (Hermes/OpenClaw/Claude Desktop/Cursor/OpenCode) 用的 binary。
# 對應 PR 系列: feature/atlas-mcp-onboarding-2026q3
#
# 用法:
#   make build-mcp    編譯 bin/atlas-mcp
#   make install-mcp  編譯後安裝到 ~/.local/bin (給 MCP client command 設定用)
#   make mcp-status   檢查 binary 存在 + atlas-go backend + LLM health 三項
#   make setup-mcp    啟動互動式 wizard
#   make install-atlas-mcp-from-release [-- VERSION=vX.Y.Z]
#                     從 GitHub Release 下載 atlas-mcp binary + SHA256 verify
#                     (給投資人 agent 用，不需 Go toolchain / source tree)

build-mcp:
	@echo "🔨 Building atlas-mcp binary..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS_VERSION) $(LDFLAGS_RUNTIME)" -o bin/atlas-mcp ./cmd/atlas-mcp

install-mcp: build-mcp
	@echo "📦 Installing atlas-mcp to $(HOME)/.local/bin/..."
	@install -m 0755 bin/atlas-mcp $(HOME)/.local/bin/atlas-mcp
	@echo "  ✓ Installed to $$(command -v atlas-mcp 2>/dev/null || echo '$(HOME)/.local/bin/atlas-mcp')"
	@echo "  → Make sure $(HOME)/.local/bin is in your PATH (add to ~/.zshrc or ~/.bashrc)"

mcp-status:
	@echo "🔍 atlas-mcp status:"
	@echo ""
	@if [ -x bin/atlas-mcp ]; then \
		size=$$(ls -lh bin/atlas-mcp | awk '{print $$5}'); \
		echo "  ✓ binary:        bin/atlas-mcp ($$size)"; \
	else \
		echo "  ✗ binary:        NOT BUILT  → run: make build-mcp"; \
	fi
	@if curl -fsS --max-time 2 http://127.0.0.1:18080/health >/dev/null 2>&1; then \
		status=$$(curl -fsS --max-time 2 http://127.0.0.1:18080/health | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null || echo "ok"); \
		echo "  ✓ atlas-go:      http://127.0.0.1:18080 (status: $$status)"; \
	else \
		echo "  ✗ atlas-go:      http://127.0.0.1:18080 DOWN  → run: go run ./cmd/atlas"; \
	fi
	@if curl -fsS --max-time 2 http://127.0.0.1:18080/api/llm/health >/dev/null 2>&1; then \
		router=$$(curl -fsS --max-time 2 http://127.0.0.1:18080/api/llm/health | python3 -c "import sys,json; print(json.load(sys.stdin).get('router_version','unknown'))" 2>/dev/null || echo "unknown"); \
		echo "  ✓ LLM router:    http://127.0.0.1:18080/api/llm/health (router: $$router)"; \
	else \
		echo "  ✗ LLM router:    http://127.0.0.1:18080/api/llm/health DOWN"; \
	fi

setup-mcp:
	@echo "🚀 Launching atlas-mcp-setup wizard..."
	go run ./cmd/atlas-mcp-setup

# 給投資人 hermes/openclaw agent 用的「一條龍」安裝 + 設定 + 驗證
#
# 自動從 ~/.config/atlas-go/.env 取共用 dev key（短期階段，#1068 商業化後
# 改用個人 key）。不需 Go toolchain / source tree，只需：
#   1. 已安裝的 atlas-mcp binary（make install-atlas-mcp-from-release）
#   2. hermes CLI（hermes mcp add / list / test 子命令）
#
# 對 hermes 等 agent:跑一次 `make setup-mcp-agent` 即可完成安裝 + 設定。
# 後續用 `make verify-mcp-setup` 驗證是否真的能用。
setup-mcp-agent:
	@ATLAS_MCP_BIN=$$(command -v atlas-mcp 2>/dev/null || echo $(HOME)/.local/bin/atlas-mcp); \
	if [ ! -x "$$ATLAS_MCP_BIN" ]; then \
		echo "❌ atlas-mcp binary not found at $$ATLAS_MCP_BIN"; \
		echo "   跑: make install-atlas-mcp-from-release"; \
		exit 1; \
	fi; \
	if [ ! -f "$(HOME)/.config/atlas-go/.env" ]; then \
		echo "⚠️  $(HOME)/.config/atlas-go/.env not found"; \
		echo "   安裝時不帶 ATLAS_API_KEY（只能跑 public tier tools）"; \
	else \
		echo "✓ 從 $(HOME)/.config/atlas-go/.env 讀取 ATLAS_* 環境變數"; \
		set -a && . $(HOME)/.config/atlas-go/.env && set +a; \
	fi; \
	echo "🚀 hermes mcp add atlas-mcp (command: $$ATLAS_MCP_BIN)..."; \
	printf "Y\n" | hermes mcp add atlas-mcp \
		--command $$ATLAS_MCP_BIN \
		$$( [ -n "$${ATLAS_BASE_URL:-}" ] && echo "--env ATLAS_BASE_URL=$$ATLAS_BASE_URL" ) \
		$$( [ -n "$${ATLAS_API_KEY:-}" ] && echo "--env ATLAS_API_KEY=$$ATLAS_API_KEY" ) \
		$$( [ -n "$${ATLAS_MCP_AUDIT_LOG:-}" ] && echo "--env ATLAS_MCP_AUDIT_LOG=$$ATLAS_MCP_AUDIT_LOG" ) \
		--connect-timeout 30; \
	hermes mcp configure atlas-mcp --enable-all 2>/dev/null || true; \
	echo ""; \
	echo "✅ Done. Verify with: make verify-mcp-setup"

# 驗證 hermes 真的能用 atlas-mcp（hermes agent 自己跑這個檢查）
verify-mcp-setup:
	@if ! command -v hermes >/dev/null 2>&1; then \
		echo "❌ hermes CLI not found in PATH"; \
		exit 1; \
	fi
	@if ! hermes mcp list 2>/dev/null | grep -q atlas-mcp; then \
		echo "❌ atlas-mcp 不在 hermes config"; \
		echo "   跑: make setup-mcp-agent"; \
		exit 1; \
	fi
	@if [ ! -f $(HOME)/.hermes/config.yaml ]; then \
		echo "❌ $(HOME)/.hermes/config.yaml not found"; \
		exit 1; \
	fi
	@echo "✓ hermes config.yaml (atlas-mcp section):"
	@grep -A5 "atlas-mcp:" $(HOME)/.hermes/config.yaml | head -8
	@echo ""
	@echo "✓ server response (should show 89 tools):"
	@hermes mcp test atlas-mcp 2>&1 | tail -3

# 給投資人 hermes/openclaw agent 用的單行安裝器（從 GitHub Release）
# 不需要 Go toolchain 或 source tree。詳見 scripts/install-atlas-mcp-from-release.sh。
install-atlas-mcp-from-release:
	@if [ ! -x "./scripts/install-atlas-mcp-from-release.sh" ]; then \
		echo "❌ scripts/install-atlas-mcp-from-release.sh not found or not executable"; \
		exit 1; \
	fi
	@VERSION_FLAG=""; \
	if [ -n "$(VERSION)" ]; then VERSION_FLAG="--version $(VERSION)"; fi; \
	./scripts/install-atlas-mcp-from-release.sh $$VERSION_FLAG

test-backend:
	@echo "🧪 Testing Go backend..."
	go test $(GO_PKGS)

test-integration:
	@echo "🧪 Running integration tests (mock backend, no external services)..."
	go test -tags=integration -timeout=60s ./cmd/atlas-mcp-setup/

lint-backend:
	@echo "🔍 Linting Go backend..."
	@command -v gofmt >/dev/null && gofmt -l $(GO_PKGS) | (read; if [ $$? -ne 0 ]; then echo "❌ gofmt issues found"; exit 1; fi) || echo "  (gofmt skipped)"
	go vet $(GO_PKGS)

# ---- CAL-1: rolling-store history import ----
import-history: ## Seed rolling store history from replay + T86 snapshots (CAL-1)
	@echo "→ Importing rolling-store history (replay calendar ∩ T86 real values)"
	@go run ./cmd/import-rolling-history
	@echo "✓ done — restart the atlas server to pick up the imported samples"

# ---- API / contract gate ----

check-routes:
	@bash scripts/check-routes.sh

hermes-smoke:
	@bash scripts/hermes-smoke.sh

canary:
	@python3 scripts/canary-check.py


.PHONY: check-routes hermes-smoke gate check-contracts canary release-check
gate:
	@echo "🔐 Running contract gate (check-routes + hermes-smoke)..."
	@bash scripts/check-routes.sh || (echo "❌ Route check failed" && exit 1)
	@bash scripts/hermes-smoke.sh || (echo "❌ Hermes smoke failed" && exit 1)
	@echo "✅ Contract gate passed"

release-check:
	@echo "🚀 Running release gate (CI-safe checks)..."
	@bash scripts/check-routes.sh || (echo "❌ Route check failed" && exit 1)
	@python3 scripts/gen-contracts.py --validate || (echo "❌ Contract check failed" && exit 1)
	@go build ./... || (echo "❌ Build failed" && exit 1)
check-contracts:
	@python3 scripts/verify-canary-vs-handler.py || (echo "❌ Canary-handler mismatch — fix canaryRoutes" && exit 1)
	@python3 scripts/gen-contracts.py --validate || (echo "❌ Contract validation failed" && exit 1)

build: build-frontend build-backend
	@echo "✅ Build complete"

test: test-frontend test-backend
	@echo "✅ Tests complete"

lint: lint-backend

ci:
	@echo "🛡️  Running quick CI checks (slow scripts in 'make ci-slow')..."
	@failed=0; passed=0; skipped=0; \
	for script in $$(ls scripts/ci/check_*.sh 2>/dev/null | grep -vE 'data_naming|layer3_|markdown_links|critical_tasks' | sort); do \
		if [ -f "$$script" ]; then \
			echo "  → $$script"; \
			if timeout 30 bash $$script > /dev/null 2>&1; then \
				passed=$$((passed+1)); \
			elif [ $$? -eq 124 ]; then \
				echo "    ⏱️  TIMEOUT (>30s): $$script"; \
				skipped=$$((skipped+1)); \
			else \
				echo "    ❌ FAILED: $$script"; \
				failed=$$((failed+1)); \
			fi; \
		fi; \
	done; \
	echo ""; \
	echo "✅ CI: $$passed passed, ❌ $$failed failed, ⏱️  $$skipped timed out"; \
	echo "    (slow scripts excluded — run 'make ci-slow' separately for those)"; \
	if [ $$failed -gt 0 ]; then exit 1; fi

ci-quick:
	@echo "🛡️  Running fast CI checks only (<2s each)..."
	@failed=0; passed=0; \
	for script in scripts/ci/check_agent_prompts.sh \
	              scripts/ci/check_case_conflicts.sh \
	              scripts/ci/check_commit_messages.sh \
	              scripts/ci/check_frontend_imports.sh \
	              scripts/ci/check_constitution.sh \
	              scripts/ci/check_methodology_constitution.sh \
	              scripts/ci/check_strategy_path.sh \
	              scripts/ci/check_constitution_drift.sh \
	              scripts/ci/check_data_catalog.sh \
	              scripts/ci/check_field_contract.sh \
	              scripts/ci/check_docs_governance.sh \
	              scripts/ci/check_agents_index.sh; do \
		if [ -f "$$script" ]; then \
			echo "  → $$script"; \
			if timeout 10 bash $$script > /dev/null 2>&1; then \
				passed=$$((passed+1)); \
			else \
				echo "    ❌ FAILED: $$script"; \
				failed=$$((failed+1)); \
			fi; \
		fi; \
	done; \
	echo ""; \
	echo "✅ CI-quick: $$passed passed, ❌ $$failed failed"; \
	if [ $$failed -gt 0 ]; then exit 1; fi

ci-slow:
	@echo "🛡️  Running slow CI checks (data_naming/layer3/markdown_links)..."
	@for script in scripts/ci/check_data_naming.sh \
	              scripts/ci/check_layer3_benchmarks.sh \
	              scripts/ci/check_layer3_snapshots.sh \
	              scripts/ci/check_markdown_links.sh; do \
		if [ -f "$$script" ]; then \
			echo "  → $$script"; \
			bash $$script || exit 1; \
		fi; \
	done
	@echo "✅ ci-slow complete"

smoke:
	@echo "🚬 Running frontend smoke tests (needs backend on :18080)..."
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir"; \
		(cd $$dir && npm run test:smoke) || exit 1; \
	done

clean:
	@echo "🧹 Cleaning build artifacts..."
	@for dir in $(FRONTENDS); do \
		echo "  → $$dir/dist + $$dir/node_modules"; \
		rm -rf $$dir/dist $$dir/node_modules; \
	done
	@rm -rf bin/
	@echo "✅ Clean complete"

# ---- Dev workflow: `go run ./cmd/atlas -api` 一鍵啟動 ----
#
# 設計前提(2026-06-28 從「用戶重複踩坑」事件抽出):
#
#   - atlas-go 已有 ProcessManager(internal/fubonproxy/manager.go)自動管 fubon-proxy
#     subprocess,且 shouldStartFubonProxy(mode, fubonAPIKey) 預設 live mode + 有 FUBON_API_KEY 才 spawn。
#     → 純 dev workflow 設 mode=-api 不帶 FUBON_API_KEY,ProcessManager 完全 idle,不需要碰 18081。
#
#   - postgres / redis 用 docker compose 管(隔離乾淨,自動 init,自動 restart)。
#     不在 Go binary 裡 embedded(記憶體 + init script 維護成本)。
#
#   - atlas-go 在 docker 跑會佔 port 18080,跟 `go run` native 搶 port。
#     解法:docker compose stop atlas(只停 atlas container,保留 postgres/redis/fubon-proxy 還在跑)。
#
# 用法:
#   make dev           # 起 docker deps + stop atlas container + go run 原生
#   make dev-stop      # 收尾(停 docker deps)
#   make dev-status    # 看容器狀態 + port 18080/18081 占用
#   make dev-logs      # tail atlas-go 啟動 log
#
# 為何這個 target 沒有「自動 kill 18080 佔用者」?
#   因為 port 18080 可能被 Google Chrome devtools / IDE / 其他 dev tool LISTEN。
#   atlas-go ProcessManager 也不處理 18080 auto-kill(同樣原因)。
#   → 用戶自己決定怎麼處理(本 Makefile 至少停掉 docker atlas-go 這個最常見衝突源)。

.PHONY: dev dev-stop dev-status dev-logs
.PHONY: status

# 確保 docker deps(postgres + redis)running,然後停 atlas container
# 把 port 18080 讓給 native atlas-go,最後 exec 進 go run 把 PID 交給 user。
#
# 不帶 fubon-proxy 到 docker deps — ProcessManager(internal/fubonproxy/manager.go)
# 會依 shouldStartFubonProxy(mode, fubonAPIKey)自動 spawn 本機 subprocess。
# ~/.config/atlas-go/.env 通常有 FUBON_API_KEY,ProcessManager 一定會 spawn,
# 此時若 docker fubon-proxy 還在跑 → port 18081 EADDRINUSE supervisor loop。
# 解法:讓 ProcessManager 唯一管 fubon-proxy,docker 那個停掉。
dev:
	@echo "🔧 Starting dev workflow..."
	@echo "  → docker compose up -d postgres redis(不帶 fubon-proxy,由 ProcessManager 管)"
	@docker compose up -d postgres redis
	@echo "  → waiting for postgres / redis to be healthy..."
	@docker compose ps --services --filter 'status=running' postgres redis | grep -q . || (echo "❌ postgres / redis not running"; exit 1)
	@for i in $$(seq 1 30); do \
		pg_ok=$$(docker inspect --format='{{.State.Health.Status}}' atlas-postgres 2>/dev/null); \
		rd_ok=$$(docker inspect --format='{{.State.Health.Status}}' atlas-redis 2>/dev/null); \
		if [ "$$pg_ok" = "healthy" ] && [ "$$rd_ok" = "healthy" ]; then \
			echo "    postgres + redis healthy"; break; \
		fi; \
		if [ $$i -eq 30 ]; then echo "❌ timeout waiting for postgres / redis"; exit 1; fi; \
		sleep 2; \
	done
	@echo "  → stopping atlas + fubon-proxy containers to free ports 18080/18081"
	@docker compose stop atlas fubon-proxy 2>/dev/null || true
	@echo "  → verifying port 18080 is free"
	@if lsof -nP -iTCP:18080 -sTCP:LISTEN >/dev/null 2>&1; then \
		echo "⚠️  port 18080 still held:"; \
		lsof -nP -iTCP:18080 -sTCP:LISTEN; \
		echo "    如要 dev workflow,請手動 kill 上述 process(ProcessManager 也不 auto-kill 18080 衝突 — 同樣原因)"; \
		exit 1; \
	fi
	@echo ""
	@echo "✅ Dev deps ready. Starting atlas-go natively via 'go run'..."
	@echo "   (CTRL+C to stop; postgres/redis 留 docker 跑;fubon-proxy 由 ProcessManager spawn/destroy)"
	@echo ""
	@exec go run ./cmd/atlas -api

dev-stop:
	@echo "🛑 Stopping dev deps (postgres + redis + fubon-proxy)..."
	@docker compose stop postgres redis fubon-proxy
	@docker compose down postgres redis fubon-proxy --remove-orphans
	@echo "🧹 Killing any leftover native 'atlas -api' process (prevents port 18080 leak)..."
	@if pgrep -f 'cmd/atlas -api' >/dev/null 2>&1; then \
		PIDS=$$(pgrep -f 'cmd/atlas -api'); \
		echo "    found atlas -api PIDs: $$PIDS"; \
		pkill -f 'cmd/atlas -api' 2>/dev/null || true; \
		sleep 1; \
		if pgrep -f 'cmd/atlas -api' >/dev/null 2>&1; then \
			echo "    ⚠️  still alive after SIGTERM, sending SIGKILL"; \
			pkill -9 -f 'cmd/atlas -api' 2>/dev/null || true; \
		fi; \
	fi
	@echo "✅ Dev deps stopped. (Note: native atlas-go 'go run' process is also auto-killed above.)"
	@if pgrep -f 'cmd/atlas' >/dev/null 2>&1; then \
		echo "    ⚠️  cmd/atlas still running (PID: $$(pgrep -f 'cmd/atlas'))"; \
	fi

dev-status:
	@echo "📊 Dev workflow status"
	@echo ""
	@echo "Containers:"
	@docker compose ps 2>/dev/null | grep -E '(postgres|redis|fubon-proxy|atlas)' || echo "  (none running)"
	@echo ""
	@echo "Port 18080 (atlas-go HTTP):"
	@lsof -nP -iTCP:18080 -sTCP:LISTEN 2>/dev/null | head -3 || echo "  (free)"
	@echo ""
	@echo "Port 18081 (fubon-proxy):"
	@lsof -nP -iTCP:18081 -sTCP:LISTEN 2>/dev/null | head -3 || echo "  (free)"
	@echo ""
	@echo "atlas-go native process (if 'go run' running):"
	@pgrep -fl 'go-build.*cmd/atlas|cmd/atlas -api' 2>/dev/null || echo "  (not running)"

dev-logs:
	@echo "📜 Tailing atlas-go native log (last 50 lines)..."
	@if pgrep -f 'cmd/atlas -api' >/dev/null 2>&1; then \
		echo "    (Note: 'go run' output goes to terminal where 'make dev' was started. This just confirms the process is alive.)"; \
		pgrep -fl 'cmd/atlas -api'; \
	else \
		echo "    atlas-go not running natively. Start with 'make dev'."; \
	fi

# Oracle 反駁 final plan PR 2: shell target showing PID + command for the
# atlas HTTP (18080) and fubon-proxy (18081) ports, mirroring the JSON shape
# emitted by GET /health (internally derived from portprobe.Probe).
status:
	@printf "  %-22s %-8s %s\n" "ADDR" "PID" "COMMAND"
	@printf "  %-22s %-8s %s\n" "----------------------" "--------" "------------------------------"
	@for p in 18080 18081; do \
		pid=$$(lsof -nP -iTCP:$$p -sTCP:LISTEN -Fpc 2>/dev/null | grep '^p' | head -1 | cut -c2-); \
		cmd=$$(lsof -nP -iTCP:$$p -sTCP:LISTEN -Fc 2>/dev/null | grep '^c' | head -1 | cut -c2-); \
		if [ -n "$$pid" ]; then \
			printf "  127.0.0.1:%-13s %-8s %s\n" "$$p" "$$pid" "$$cmd"; \
		else \
			printf "  127.0.0.1:%-13s %-8s %s\n" "$$p" "(free)" ""; \
		fi; \
	done
# ============================================================================
# Binary freshness (added 2026-07-21, manifest 2026-07-21-binary-freshness-protocol.md)
# ============================================================================
#
# Why: PR cycle #1244-#1248 showed that "fixed code but docker still runs old
# binary" caused hours of repeated debugging and risk of AI hallucination
# reverting working code. These targets ensure every binary tracks HEAD.
#
# Refresh strategy:
#   - All Docker builds use build-arg GIT_COMMIT and inject via internal/buildinfo ldflags
#   - On sandbox environments without proxy.golang.org, use Dockerfile.cron.local /
#     Dockerfile.atlas.local (host-built binaries via `go build`)
#   - On CI / dev boxes, native Dockerfile / Dockerfile.cron
#
# Every make rebuild-* target must be invoked during closing-time check if
# check-binaries fails (see ~/.config/opencode/AGENTS.md "Binary freshness gate").

.PHONY: rebuild-all rebuild-cron rebuild-atlas rebuild-host-bin check-binaries

HOST_GO       := $(shell which go 2>/dev/null || echo /opt/homebrew/bin/go)
export ATLAS_GIT_COMMIT ?= $(GIT_COMMIT)
BUILDTIME     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOENV_LINUX   := GOOS=linux GOARCH=arm64

LDFLAGS_BF := -w -s -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=dev \
              -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=$(GIT_COMMIT) \
              -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=$(BUILDTIME)

# Verify every deployed binary's buildinfo.Commit matches HEAD.
# Exit 0 = fresh, exit 1 = at least one stale.
check-binaries:
	@./scripts/check-binary-freshness.sh

# Quick integrity check: binary freshness + source formatting.
# Runs in < 30s; safe to run before every push.
check: check-binaries
	@echo "✓ make check passed"

# Rebuild host bin/atlas-mcp only.
rebuild-host-bin:
	@echo "  building host bin/atlas-mcp (commit=$(GIT_COMMIT))"
	@$(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o bin/atlas-mcp ./cmd/atlas-mcp

# Rebuild the 4 atlas-atlas binaries on host (for sandbox use with Dockerfile.atlas.local).
# .build-atlas/ is gitignored. atlas-go/atlas-mcp/daily-replay-sync/calibrate-seasonal.
.build-atlas: ; @mkdir -p $@
rebuild-atlas-bins: | .build-atlas
	@echo "  building 4 atlas-atlas binaries on host"
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/atlas-go ./cmd/atlas
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/atlas-mcp ./cmd/atlas-mcp
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/daily-replay-sync ./cmd/daily-replay-sync
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/calibrate-seasonal ./cmd/calibrate-seasonal

# Rebuild atlas-atlas image from host-built binaries + restart container.
rebuild-atlas: rebuild-atlas-bins
	@if [ "$$(git rev-parse --git-dir)" != "$$(git rev-parse --git-common-dir)" ]; then \
		echo "❌ 含 docker 的 rebuild 只能在主 worktree 執行（linked worktree 拒絕）"; exit 1; \
	fi
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker build -t atlas-atlas:local -f Dockerfile.atlas.local .
	@docker tag atlas-atlas:local atlas-atlas:latest
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker compose up -d atlas

# Rebuild the 6 cron binaries on host.
# daily-replay-sync/macro-ingest/geo-ingest/
# cron-quote-backfill/
# c07-obs-collector/c07-day-evaluator.
.build-cron: ; @mkdir -p $@
rebuild-cron-bins: | .build-cron
	@echo "  building 6 cron binaries on host"
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/daily-replay-sync ./cmd/daily-replay-sync
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/macro-ingest ./cmd/macro-ingest
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/geo-ingest ./cmd/geo-ingest
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/cron-quote-backfill ./cmd/cron-quote-backfill
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/c07-obs-collector ./cmd/experimental/c07-obs-collector
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-cron/c07-day-evaluator ./cmd/experimental/c07-day-evaluator

# Rebuild cron image + force-recreate all 6 cron containers.
CRON_IMAGE_TAGS := atlas-cron-quote-backfill:latest \
                   atlas-cron-geo-ingest:latest atlas-atlas-cron-c07-collect:latest \
                   atlas-cron-replay-sync:latest \
                   atlas-atlas-cron-c07-evaluate:latest atlas-cron-darwinian:latest \
                   atlas-cron-macro-ingest:latest
DOCKER_BIN ?= docker

.PHONY: retag-cron-images
retag-cron-images:
	@for image in $(CRON_IMAGE_TAGS); do $(DOCKER_BIN) tag atlas-cron-rebuilt:local $$image; done

rebuild-cron: rebuild-cron-bins
	@if [ "$$(git rev-parse --git-dir)" != "$$(git rev-parse --git-common-dir)" ]; then \
		echo "❌ 含 docker 的 rebuild 只能在主 worktree 執行（linked worktree 拒絕）"; exit 1; \
	fi
	@$(DOCKER_BIN) build -t atlas-cron-rebuilt:local -f Dockerfile.cron.local .
	@$(MAKE) retag-cron-images
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker compose up -d --force-recreate --no-build cron-macro-ingest cron-quote-backfill cron-replay-sync atlas-cron-c07-evaluate cron-darwinian cron-geo-ingest atlas-cron-c07-collect 

# Full rebuild: host bin + atlas image + cron images.
rebuild-all: rebuild-host-bin rebuild-atlas rebuild-cron
	@echo "✓ rebuilt all binaries"
	@$(MAKE) check-binaries

# ============================================================================
# CI preflight / full suite（2026-07-26，解決 PR-CI 往返耗時問題）
# ============================================================================
#
# 設計目標：
#   ci-gate — push 前必跑，< 30s，攔截 90% 的 CI 失敗（fmt/build/vet/generate/quick scripts）
#   ci-full — PR 前跑，~5-8 min，覆蓋 GitHub CI quality.yml + ci-cd.yml 所有可本地化的檢查
#
# 與 GitHub CI 對應關係：
#   ci-gate     → fmt + build + generate + naming + frontend-imports + field-contract +
#                  agent-prompts + constitution + channel-index + agents-md-drift
#   ci-full     → ci-gate + lint (golangci-lint) + staticcheck + test + race + coverage +
#                  cmd/atlas integration + ci (all scripts) + ci-slow + orphan check
#
# 不可本地化的 CI job（仍須 GitHub 驗證）：
#   Docker build/push (multi-platform)、deploy、gosec SARIF upload、
#   跨 repo contract check (routes/contracts)

.PHONY: ci-gate
ci-gate:
	@echo "🛡️  CI pre-push gate (fast, <30s)..."
	@echo ""
	@echo "  → gofmt check"
	@test -z "$$(gofmt -l .)" || { \
		echo "    ❌ gofmt 格式不符。執行: gofmt -w ."; \
		gofmt -l . | head -10; \
		exit 1; \
	}
	@echo "    ✅"
	@echo "  → go build ./..."
	@go build ./...
	@echo "    ✅"
	@echo "  → go vet ./..."
	@go vet ./...
	@echo "    ✅"
	@echo "  → go generate (drift check)"
	@go generate ./...
	@git diff --exit-code -- '*.go' '*.ts' '*.json' 2>/dev/null || { \
		echo "    ❌ 生成檔案過期。執行: go generate ./... 然後 commit"; \
		git diff --stat -- '*.go' '*.ts' '*.json'; \
		exit 1; \
	}
	@echo "    ✅"
	@echo "  → docs 同步檢查（AGENTS.md line drift + 斷鏈）"
	@bash scripts/ci/check_agents_md_drift.sh
	@bash scripts/ci/check_doc_links.sh
	@echo "    ✅"
	@echo "  → 關鍵背景任務存在於 binary（DCE 防再犯，2026-08-10 事故）"
	@bash scripts/ci/check_critical_tasks.sh
	@echo "    ✅"
	@echo "  → fast CI scripts"
	@$(MAKE) --no-print-directory ci-quick
	@echo ""
	@echo "✅ ci-gate passed — 可以 push"

.PHONY: ci-constitution
ci-constitution:
	@echo "📜 方法論憲章合規檢查..."
	@bash scripts/ci/check_methodology_constitution.sh
	@bash scripts/ci/check_strategy_path.sh
	@bash scripts/ci/check_constitution_drift.sh
	@echo "✅ ci-constitution passed"

.PHONY: ci-full
ci-full: ci-gate ci-constitution
	@echo "🧪 CI full suite (local, ~5-8 min)..."
	@echo ""
	@echo "  → golangci-lint (v2.12.2)"
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "    ❌ golangci-lint 未安裝。brew install golangci-lint"; \
		exit 1; \
	}
	@golangci-lint run --timeout=5m
	@echo "    ✅"
	@echo "  → standalone staticcheck"
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo "    ❌ staticcheck 未安裝。go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	}
	@staticcheck ./...
	@echo "    ✅"
	@echo "  → go test (excluding cmd/atlas heavy)"
	@go test -count=1 $$(go list ./... | grep -v '/cmd/atlas$$')
	@echo "    ✅"
	@echo "  → go test -race (excluding cmd/atlas)"
	@go test -race -count=1 $$(go list ./... | grep -v '/cmd/atlas$$')
	@echo "    ✅"
	@echo "  → cmd/atlas integration tests"
	@go test -count=1 -timeout=10m ./cmd/atlas/...
	@echo "    ✅"
	@echo "  → cmd/atlas-mcp-setup integration tests"
	@go test -tags=integration -count=1 -timeout=60s ./cmd/atlas-mcp-setup/
	@echo "    ✅"
	@echo "  → CI scripts (all)"
	@$(MAKE) --no-print-directory ci
	@$(MAKE) --no-print-directory ci-slow
	@echo "    ✅"
	@echo "  → coverage threshold (≥60%)"
	@go test -coverprofile=/tmp/atlas-ci-full-coverage.out $$(go list ./... | grep -v '/cmd/atlas$$') > /dev/null 2>&1; \
	COVERAGE=$$(go tool cover -func=/tmp/atlas-ci-full-coverage.out | awk '/^total:/ {print $$3}' | tr -d '\r' | sed 's/%//'); \
	echo "    Total coverage: $${COVERAGE}%"; \
	if echo "$$COVERAGE 60" | awk '{exit !($$1 < $$2)}'; then \
		echo "    ❌ Coverage $${COVERAGE}% 低於 60% 閾值"; \
		rm -f /tmp/atlas-ci-full-coverage.out; \
		exit 1; \
	fi; \
	rm -f /tmp/atlas-ci-full-coverage.out
	@echo "    ✅"
	@echo "  → orphan artifact check"
	@ORPHANS=""; \
	for f in $$(git ls-files); do \
		[ -f "$$f" ] || continue; \
		ft=$$(file "$$f" 2>/dev/null); \
		case "$$ft" in \
			*"Mach-O"*|*"ELF"*) ORPHANS="$$ORPHANS  [BINARY] $$f";; \
		esac; \
	done; \
	for f in $$(git ls-files '*.pid'); do \
		ORPHANS="$$ORPHANS  [PID] $$f"; \
	done; \
	for f in $$(git ls-files '*.out'); do \
		ORPHANS="$$ORPHANS  [COVERAGE] $$f"; \
	done; \
	if [ -n "$$ORPHANS" ]; then \
		echo "❌ 發現 orphan artifacts:"; \
		echo "$$ORPHANS"; \
		echo "執行: git rm --cached <file>（保留本地檔案）"; \
		exit 1; \
	fi
	@echo "    ✅"
	@echo ""
	@echo "✅ ci-full passed — 可以建立 PR，CI 基本一次過"

# Fresh-clone verification — clones repo to temp dir and does frontend→build→test.
# Verifies the repo is self-contained with no hidden local dependencies.
fresh-clone-check:
	@./scripts/fresh-clone-check.sh
