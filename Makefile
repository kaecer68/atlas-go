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
.PHONY: ci-gate ci-full pre-push import-history ci-quota check-production-host
.PHONY: test-makefile-imac-guard

# ── iMac prod guard (2026-08-27, 修 PR #1695 二次破壞事件) ─────────────────
# `rebuild-atlas` 跑 `docker compose up -d atlas` 沒指定 -f,在 iMac (KiMac)
# 主機上會走預設 dev compose,誤刪/重建 prod 容器、volume、密碼。
# 在 iMac 上預設 fail-closed,需顯式 ALLOW_DEV_REBUILD_ON_IMAC=1 才能跑。
# Linux/CI 不受影響 (LocalHostName 為空)。可用 test-makefile-imac-guard 驗證偵測。
IMAC_LOCAL_HOSTNAME ?= $(shell scutil --get LocalHostName 2>/dev/null)
ifeq ($(IMAC_LOCAL_HOSTNAME),KiMac)
IMAC_DETECTED := 1
endif

# pre-push — 本地 push 前驗證（由 .githooks/pre-push hook 自動執行）
#   ci-gate  (~30s)   永遠執行：gofmt/build/vet/generate-drift/ci-quick
#   ci-full  (~5-8min) 僅當 diff 含程式碼變更時執行：lint/staticcheck/test/race/coverage
#   目的：在本地先抓出 CI 會失敗的問題，避免 push→CI-fail 往返浪費
#   ★「要不要跑 ci-full」的判定只有一份實作：scripts/ci/ci-full-trigger.sh
#     （本 target 與 .githooks/pre-push 都呼叫它）。退出碼：0=含程式碼變更／
#     1=真的 docs-only（可跳過 ci-full）／2=git 失敗 ⇒ fail loud（見下方 else 分支）。
#     修前兩邊各寫一份 `git diff … 2>/dev/null | grep -qE …`，兩個後果：
#       (a) git 失敗（range 不可解析／無 merge base／git 缺失）的 stdout 是空的，
#           與「真的只改文件」逐字相同 ⇒ 被判 docs-only ⇒ **ci-full 靜默跳過**
#           （FU-20261016-21；與 E4、FU-20261016-19 同族）。
#       (b) 兩份 pattern 已分歧：`|^configs/`（PR #2176）只進了 hook ⇒ `make pre-push`
#           在只改 configs/** 的 push 上仍跳過 ci-full。共用一支即消滅此類漂移。
#   手動用法：make pre-push            （同 hook 邏輯：auto）
#             PRE_PUSH_FULL=always make pre-push   （強制全量）
#             PRE_PUSH_FULL=never  make pre-push   （只跑 ci-gate）
pre-push: ci-gate
	@echo ""
	@bash scripts/ci/check_frontend_dist.sh --diff origin/main...HEAD
	@trigger_rc=0; \
	bash scripts/ci/ci-full-trigger.sh origin/main...HEAD || trigger_rc=$$?; \
	if [ "$${trigger_rc}" -eq 0 ]; then \
		echo "🧪 diff 含程式碼變更 → 跑 make ci-full (~5-8 min)..."; \
		$(MAKE) ci-full || exit 1; \
	elif [ "$${trigger_rc}" -eq 1 ]; then \
		echo "ℹ️  docs-only diff — ci-gate 已足夠（ci-full 跳過）"; \
	else \
		echo ""; \
		echo "❌ make pre-push: 無法判定 diff 是否含程式碼變更（ci-full-trigger rc=$${trigger_rc}）"; \
		echo "   「取不到 diff」不等於「只改文件」⇒ 不以 docs-only 放行（fail loud）。"; \
		echo "   修復：git fetch origin main && git rev-parse origin/main 之後重跑。"; \
		echo ""; \
		exit 1; \
	fi


# 前端目錄列表(若有新增,加在這裡)
FRONTENDS := admin_web client_web
GO_PKGS   := ./cmd/... ./internal/...

# ---- node for the frontend builds -------------------------------------------
# Why an explicit resolution instead of a bare `node`: on the deploy host
# (Mac Mini) node comes from nvm, and `make` may run from a non-interactive
# shell (launchd / cron / ssh command) whose PATH does NOT contain nvm's shim
# dir ⇒ bare `node` dies with "command not found" *after* the Go build already
# embedded the previous dist. Resolution order, first hit wins:
#   1. `command -v node`             — whatever the caller's PATH provides
#   2. /opt/homebrew/bin/node        — Homebrew (Apple Silicon) default prefix
#   3. ~/.nvm/versions/node/*/bin/node — newest nvm install (by dir mtime)
# An empty NODE_BIN is NOT "skip the frontend build": build-frontend fails
# loudly (see below). Override with `make NODE_BIN=/path/to/node ...`.
NODE_BIN ?= $(shell command -v node 2>/dev/null || { [ -x /opt/homebrew/bin/node ] && echo /opt/homebrew/bin/node; } || ls -1dt $$HOME/.nvm/versions/node/*/bin/node 2>/dev/null | head -1)

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

# ---- iMac 容器守護（正本在 repo；避免 iMac ~/bin 漂移）----
# 為什麼需要：這支腳本是 iMac 唯一的自動復原機制，過去只存在於 iMac 的 ~/bin（未版控），
# 無法回答「iMac 上跑的是哪一版」。2026-09-12 起正本為 scripts/ops/imac-container-watchdog.sh。
#
# 2026-09-26（E13）：production 自 2026-09-22 起是 **Mac Mini**（`kaecer@kmacmini`）；
# iMac（`kk@kimac` / `/Users/kk`）已退役、帳號不存在 ⇒ 以下預設值原本必失敗
# （`ssh: Could not resolve hostname kimac`，make 收尾 exit 2）。
# 變數名 `IMAC_HOST` 與 target 名 `imac-*` 屬歷史債 ⇒ 保留（外部覆寫 seam 與 muscle memory）。
# `WATCHDOG_DST` = Mac Mini 上的實查路徑（2026-09-26 ssh 唯讀盤查 `~/bin/` 確認存在；
# 同 docs/operations/local-deploy.md §容器守護腳本）。
IMAC_HOST ?= kaecer@kmacmini
WATCHDOG_SRC := scripts/ops/imac-container-watchdog.sh
WATCHDOG_DST := /Users/kaecer/bin/atlas-container-watchdog.sh

.PHONY: imac-watchdog-diff
imac-watchdog-diff: ## 比對 Mac Mini watchdog 與 repo 正本的 sha256（漂移檢查）
	@echo "→ repo 正本: $(WATCHDOG_SRC)"
	@local_sum=$$(shasum -a 256 $(WATCHDOG_SRC) | awk '{print $$1}'); \
	remote_sum=$$(ssh -o ConnectTimeout=15 $(IMAC_HOST) "shasum -a 256 $(WATCHDOG_DST) 2>/dev/null | awk '{print \$$1}'"); \
	echo "  repo  = $$local_sum"; \
	echo "  Mac Mini = $${remote_sum:-<missing>}"; \
	if [ "$$local_sum" = "$$remote_sum" ]; then \
		echo "  ✅ 一致（無漂移）"; \
	else \
		echo "  ❌ 不一致 — 執行 make imac-watchdog-install 同步"; exit 1; \
	fi

.PHONY: imac-watchdog-install
imac-watchdog-install: ## 安裝 repo 正本到 Mac Mini ~/bin 並重載 launchd
	@echo "→ 備份 Mac Mini 現有腳本"
	@ssh -o ConnectTimeout=15 $(IMAC_HOST) "test -f $(WATCHDOG_DST) && cp -p $(WATCHDOG_DST) $(WATCHDOG_DST).bak-\$$(date +%Y%m%dT%H%M%S) || true"
	@echo "→ 複製 $(WATCHDOG_SRC) → Mac Mini:$(WATCHDOG_DST)"
	@scp -q $(WATCHDOG_SRC) $(IMAC_HOST):$(WATCHDOG_DST)
	@ssh -o ConnectTimeout=15 $(IMAC_HOST) "chmod +x $(WATCHDOG_DST) && bash -n $(WATCHDOG_DST) && echo '  ✅ 語法檢查通過'"
	@echo "→ 重載 launchd job（plist: scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist）"
	@ssh -o ConnectTimeout=15 $(IMAC_HOST) "launchctl unload ~/Library/LaunchAgents/com.goluck.atlas-container-watchdog.plist 2>/dev/null; launchctl load ~/Library/LaunchAgents/com.goluck.atlas-container-watchdog.plist && echo '  ✅ launchd 已重載'"
	@$(MAKE) --no-print-directory imac-watchdog-diff


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
	@echo "  build-frontend     編譯所有前端 (client_web + admin_web 的 esbuild.config.mjs)"
	@echo "  test-frontend      跑前端單元測試"
	@echo "  watch-frontend     啟動 esbuild --watch 模式"
	@echo "  check-frontend-dist 檢查 dist 是否比來源樹舊 (rebuild-all 末段自動跑)"
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
	@echo "  ledger-seam-check  ledger store seam 守門 + 契約測試 (禁新增裸 ledger.NewStore)"
	@echo "  ci-quick           只跑快速 check(<2s,適合 PR 前)"
	@echo "  ci-slow            只跑慢速 check(data_naming/layer3/markdown)"
	@echo "  ci-quota            FinMind 每日 quota 檢查 (--strict, 週一開市前)"
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

# 前端建置（admin_web + client_web）。
#
# 直接呼叫 `node esbuild.config.mjs`（＝ 各前端 package.json 的 "build" script），
# 刻意不經過 `npm run build`：npm 是 `#!/usr/bin/env node` 的 shim，就算用絕對
# 路徑找到 npm，它仍需要 PATH 上有 node —— 正是非互動 shell 缺的東西。
#
# 這個 target 是部署路徑 `rebuild-all` 的第一步（必須早於任何 embed dist 的 Go
# 建置）：client_web/dist + admin_web/dist 被 gitignore，而 Go 以
# `//go:embed all:dist` 內嵌它們 ⇒ 少了這一步，部署出去的 binary 夾帶的是 build
# 主機上殘留的舊 dist。2026-10-07 實證：Phase 0 已從原始碼移除「錢潮預測卡」，
# 公開站 /client/js/home-*.js 卻仍含 home-predictions（下架未生效）。
build-frontend:
	@echo "🔨 Building frontend ($(FRONTENDS))..."
	@node_bin="$(NODE_BIN)"; \
	if [ -z "$$node_bin" ] || [ ! -x "$$node_bin" ]; then \
		echo "❌ build-frontend: 找不到 node（PATH / /opt/homebrew/bin / ~/.nvm/versions/node/*/bin 都沒有）" >&2; \
		echo "   dist 是 gitignore 的產物、由 go:embed 內嵌進 binary —— 沒有 node 就沒有新 dist，" >&2; \
		echo "   部署出去的 binary 會夾帶舊 UI（2026-10-07 事故）。" >&2; \
		echo "   修復：安裝 Node.js（brew install node，或 nvm install --lts），或明示指定 NODE_BIN。" >&2; \
		exit 1; \
	fi; \
	ver=$$("$$node_bin" --version 2>/dev/null || echo unknown); \
	echo "   node = $$node_bin ($$ver)"; \
	for dir in $(FRONTENDS); do \
		if [ ! -d "$$dir/node_modules/esbuild" ]; then \
			echo "❌ build-frontend: $$dir/node_modules/esbuild 不存在 —— 先跑 make install-frontend" >&2; \
			exit 1; \
		fi; \
		echo "  → $$dir"; \
		( cd "$$dir" && "$$node_bin" esbuild.config.mjs ) || { echo "❌ build-frontend: $$dir 建置失敗" >&2; exit 1; }; \
	done; \
	echo "✅ build-frontend: dist 已重建"

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

test-backend: embed-dirs
	@echo "🧪 Testing Go backend..."
	go test $(GO_PKGS)

test-integration:
	@echo "🧪 Running integration tests (mock backend, no external services)..."
	go test -tags=integration -timeout=60s ./cmd/atlas-mcp-setup/

lint-backend:
	@echo "🔍 Linting Go backend..."
	@# ⚠️ 舊寫法 `gofmt -l | (read; if [ $$? -ne 0 ])` 的 $$? 是 **read** 的（不是 gofmt 的），
	@# 而且整串被 `|| echo "(gofmt skipped)"` 吞掉 ⇒ 真的有格式問題時也只印 skipped 並回 0
	@# （閘門看似存在、實際不可能紅；issue #2011 同族的「別命名為 gate 的裝飾品」）。
	@command -v gofmt >/dev/null 2>&1 || { echo "    ❌ gofmt 不存在（Go toolchain 未安裝？）——閘門不得靜默消失"; exit 1; }
	@# 註：gofmt 不吃 Go 的 `...` pattern（GO_PKGS 對它會 `lstat ... no such file` 且輸出空 ⇒ 舊寫法
	@# 兩種壞法疊在一起：rc 被吞 + 根本沒掃到檔案）。改用 repo 根目錄（與 ci-gate 的 `gofmt -l .` 一致）。
	@gofmt_out="$$(gofmt -l .)"; gofmt_rc=$$?; \
	if [ "$$gofmt_rc" -ne 0 ]; then \
		echo "    ❌ gofmt 執行失敗（rc=$${gofmt_rc}）——這不是「乾淨」"; \
		exit 1; \
	fi; \
	if [ -n "$$gofmt_out" ]; then \
		echo "    ❌ gofmt 格式不符："; \
		echo "$$gofmt_out"; \
		echo "    執行 gofmt -w <檔案> 後重跑"; \
		exit 1; \
	fi; \
	echo "    ✅ gofmt 乾淨（gofmt -l . 無輸出）"
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

# ── 生產主機護欄（2026-09-23 事故後新增）────────────────────────────
# 在生產機（偵測 $HOME/.a2a/PRODUCTION_HOST）上跑 CI/測試會產生真實副作用
# （呼叫外部服務、寫真 DB、寄真信）。GitHub Actions 會設 CI=true，屬合法路徑。
# 真的必須在生產機跑：A2A_ALLOW_PROD_CI=1 make ci
.PHONY: check-production-host
check-production-host:
	@if [ -f "$$HOME/.a2a/PRODUCTION_HOST" ] && [ "$${CI:-}" != "true" ] && [ "$${A2A_ALLOW_PROD_CI:-0}" != "1" ]; then \
		echo "⛔ 這台是生產主機（$$HOME/.a2a/PRODUCTION_HOST）→ 拒絕互動式執行 make ci"; \
		echo "   請在複製機執行；必要時 A2A_ALLOW_PROD_CI=1 make ci"; \
		exit 1; \
	fi

# ── `make ci` 的三態（pass / skip / fail）＋ timeout 政策（FU-20260926-12）───────
# 每一支檢查的結果**必須落在恰好一個**分類，收尾的判定也只認這三態：
#   pass = rc 0
#   skip = **明確列在下面 SKIP (declared) 清單**（每一項都必須寫出「誰來覆蓋它」）
#   fail = 其他任何結束碼：timeout 124、被訊號帶走 130/143、找到但不可執行 126、找不到 127…
# 為什麼 timeout **不能**算 skip（F-item FU-20260926-12 的根因）：舊寫法把 rc=124 只做
# `skipped=$$((skipped+1))`，而收尾只檢查 `failed > 0` ⇒ 一支「永遠跑不完」的檢查（等網路／等鎖／
# 等 docker）在閘門眼裡等於通過（false-green，#2011 家族）。現在 timeout 獨立計數，
# 且**一律讓閘門回非 0**，訊息指名是哪一支、耗時多少秒。
# 三態的取值邊界（刻意不對稱）：①skip 只能來自宣告清單，**不能**由執行結果推得；
# ②timeout 是 fail 而不是 skip；③「0 支檢查被執行」也是 fail。
# CI_SCRIPT_TIMEOUT 只調「多久算掛住」；它**不改變**「掛住 ⇒ 紅燈」的判準。
CI_SCRIPT_TIMEOUT ?= 30
# 個別檢查的較長預算（**已量測**的慢檢查，見下方 case；其餘一律用 CI_SCRIPT_TIMEOUT）。
# 為什麼需要：`make ci` 會被 `ci-full`（pre-push）在 `go test -race` 之後以子 make 呼叫，機器負載
# 高時同一支檢查的 wall-clock 可以差 2–4 倍。**實證**：2026-10-16 本 PR 的 pre-push 實跑，
# `check_ledger_seam.sh` 空機 7.7s（冷啟動 19.2s）卻在該負載下吃滿 30s ⇒ 假紅（`ci-full` 因此失敗）。
# 量測（空機、逐支 `/usr/bin/time -p`，2026-10-16；單位 ms）：
#   check_secrets 13641／check_maturity 8287／check_ledger_seam 7713（冷 19000+）／check_shellcheck 7608，
#   其餘 ≤ 4.0s（次高：sa12 4012、doc_ports 3917、inert_closure 3138）。取「空機 ≥ 5s」為放寬門檻
#   （×4 負載餘裕仍 < 30s 者不必放寬）。
# 規則（違反其一就不要加鍵）：①只放量測過的；②放寬是**加大預算**，不是取消判定——超時仍 rc=124 ⇒
# 一樣紅、一樣印 elapsed；③不接受「為了讓紅燈變綠」而加鍵，真的不該在此跑的走 SKIP (declared)。
CI_SCRIPT_TIMEOUT_SLOW ?= 120

# 宣告式跳過清單（declared skip）—— 沒列在這裡的就一定要在預算內跑完：
#   *data_naming*|*layer3_*|*markdown_links* → 慢（>CI_SCRIPT_TIMEOUT）⇒ `make ci-slow` 逐支（無 timeout）跑
#   *critical_tasks* → `make ci-gate` 逐支明列跑（同一支腳本，無 timeout）
#   *finmind_quota*  → runtime state 依賴（#1742）⇒ 開市前 `STRICT=1 make ci-quota`
#   *task_liveness*  → 需 --tasks（無參數即用法錯誤、刻意 fail-closed）⇒ 由 .github/workflows/
#                      daily-maintenance.yml 帶參數呼叫；契約（逐項判定 + workflow 契約）由
#                      tests/scripts/test-check-task-liveness.sh 在 ci-gate 與 quality.yml 的
#                      daily-maintenance-contract job 以 hermetic（不連網）方式覆蓋
#   *frontend_dist*  → 本 target 開頭已用 `--diff origin/main...HEAD` 明列跑過（見下方註解）
ci: check-production-host
	@echo "🛡️  Running quick CI checks (slow scripts in 'make ci-slow')..."
	# check_frontend_dist.sh is scoped to the branch diff here, NOT run by the
	# glob below (2026-10-12 defect). Why: client_web/dist + admin_web/dist are
	# gitignored BUILD products, so in any fresh worktree/clone they are absent
	# by construction — the script's no-arg mode is deliberately strict (it is
	# what `make check-frontend-dist` / `rebuild-all` must run on a build host
	# after `build-frontend`), so running it unnscoped here turns every fresh
	# checkout red with "dist 內沒有建置產物". pre-push already runs it with
	# `--diff origin/main...HEAD`; `ci` mirrors that scope so the gate means
	# "did THIS branch forget to rebuild a dist it changed", which is the only
	# question a pre-push CI can answer.
	@bash scripts/ci/check_frontend_dist.sh --diff origin/main...HEAD
	@failed=0; passed=0; skipped=0; timedout=0; \
	for script in scripts/ci/check_*.sh; do \
		skip_reason=""; \
		case "$$script" in \
			*data_naming*|*layer3_*|*markdown_links*) skip_reason="慢檢查（>$(CI_SCRIPT_TIMEOUT)s）⇒ make ci-slow 逐支（無 timeout）跑";; \
			*critical_tasks*) skip_reason="make ci-gate 逐支明列跑（同一支腳本，無 timeout）";; \
			*finmind_quota*)  skip_reason="runtime state 依賴（#1742）⇒ STRICT=1 make ci-quota 開市前跑";; \
			*task_liveness*)  skip_reason="需 --tasks 參數 ⇒ daily-maintenance.yml 帶參數呼叫；契約測試在 ci-gate";; \
			*frontend_dist*)  skip_reason="本 target 開頭已用 --diff origin/main...HEAD 明列跑過";; \
		esac; \
		if [ -n "$$skip_reason" ]; then \
			echo "  ⏭️  SKIP (declared): $$script — $$skip_reason"; \
			skipped=$$((skipped+1)); \
			continue; \
		fi; \
		if [ ! -f "$$script" ]; then continue; fi; \
		script_timeout=$(CI_SCRIPT_TIMEOUT); \
		case "$$script" in \
			*/check_secrets.sh|*/check_maturity.sh|*/check_shellcheck.sh|*/check_ledger_seam.sh) script_timeout=$(CI_SCRIPT_TIMEOUT_SLOW);; \
		esac; \
		echo "  → $$script"; \
		start=$$(date +%s); \
		rc=0; timeout $$script_timeout bash "$$script" > /dev/null 2>&1 || rc=$$?; \
		elapsed=$$(( $$(date +%s) - start )); \
		if [ $$rc -eq 0 ]; then \
			passed=$$((passed+1)); \
		elif [ $$rc -eq 124 ]; then \
			echo "    ⏱️  TIMEOUT (>$${script_timeout}s, elapsed=$${elapsed}s): $$script"; \
			echo "       『掛住』不是『跳過』：它沒在預算內結束 ⇒ 閘門紅燈（FU-20260926-12）"; \
			echo "       合法慢檢查請加進本 target 的 SKIP (declared) 清單或改用 make ci-slow，不要靠超時放行"; \
			timedout=$$((timedout+1)); \
		else \
			echo "    ❌ FAILED (rc=$$rc, elapsed=$${elapsed}s): $$script"; \
			failed=$$((failed+1)); \
		fi; \
	done; \
	echo ""; \
	echo "✅ CI: $$passed passed, ⏭️  $$skipped skipped (declared), ❌ $$failed failed, ⏱️  $$timedout timed out"; \
	if [ $$passed -eq 0 ]; then \
		echo "    ❌ 0 支檢查被執行（glob 沒對到 scripts/ci/check_*.sh？）⇒『什麼都沒跑』不是通過"; \
		exit 1; \
	fi; \
	if [ $$timedout -gt 0 ]; then \
		echo "    ❌ $$timedout 支檢查在各自的預算內未結束（rc=124）⇒ 掛住是 fail、不是 skip，閘門回非 0"; \
		exit 1; \
	fi; \
	if [ $$failed -gt 0 ]; then exit 1; fi


ci-quick: check-production-host
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
	              scripts/ci/check_channel_consistency.sh \
	              scripts/ci/check_docs_governance.sh \
	              scripts/ci/check_agents_index.sh \
	              scripts/ci/check_jev_contract.sh \
	              scripts/ci/check_monitoring_single_source.sh; do \
		if [ ! -f "$$script" ]; then \
			echo "    ❌ 清單中的 $$script 不存在（改名／搬移？）⇒ 少跑一支不能算通過"; \
			failed=$$((failed+1)); \
			continue; \
		fi; \
		echo "  → $$script"; \
		if timeout 10 bash $$script > /dev/null 2>&1; then \
			passed=$$((passed+1)); \
		else \
			echo "    ❌ FAILED: $$script"; \
			failed=$$((failed+1)); \
		fi; \
	done; \
	echo ""; \
	echo "✅ CI-quick: $$passed passed, ❌ $$failed failed"; \
	if [ $$passed -eq 0 ]; then \
		echo "    ❌ 0 支檢查被執行 ⇒『什麼都沒跑』不是通過"; \
		exit 1; \
	fi; \
	if [ $$failed -gt 0 ]; then exit 1; fi

# ---- inert 閉環靜態檢查（#1944 建議 2，2026-09-25）----
# 目的：攔下「有寫入、無消費」的新增閉環（config 沒有 reader／writer 沒有 consumer／
#       claim 欄位硬寫 true）。規格：docs/specs/inert-static-check-spec.md
#       allowlist：scripts/ci/inert-baseline.json（每項 reason 必填）
#   make inert-check           檢查 + hermetic 回歸測試（~10s）
#   make inert-check-selftest  只跑回歸測試（fixture，不碰 git／production／Go toolchain）
#   make inert-check-update    把新違規寫進 baseline（reason 留 TODO，人工填寫後才 commit）
.PHONY: inert-check inert-check-selftest inert-check-update

inert-check-selftest:
	@echo "  → inert closure regression tests（hermetic fixtures）"
	@bash tests/scripts/test-inert-closure.sh

inert-check: inert-check-selftest
	@echo "  → inert closure static check（config / writer / claim）"
	@bash scripts/ci/check_inert_closure.sh

inert-check-update:
	@python3 scripts/ci/check_inert_closure.py --root . --update-baseline
	@echo "   → 請編輯 scripts/ci/inert-baseline.json 填寫 reason（TODO 開頭會讓檢查變紅）"

# ---- ledger store seam 守門（T1/S1，2026-10-16）----
# 目的：攔下**新增**的裸 `ledger.NewStore(dir)` **與**「未注入就落到 JSONL」的間接建構子
#       （service.NewLiveService／NewBacktestService、risk/system.NewHandlers、autobacktest.New*FromDir）。
#       backend-aware factory 是 opt-in，直建／未注入的站點在生產（ATLAS_STORE_BACKEND=postgres）
#       會安靜地讀錯後端：JSONL 對缺檔回 (nil, nil) ⇒ GET /api/backtest/signals 回 200 全 0、
#       err 為 nil ⇒ CIRCUIT_BREAKER 永不觸發。
#       allowlist：scripts/ci/ledger-seam-allowlist.json（每筆 reason 必填）
#       規格：scripts/ci/check_ledger_seam.py 的 docstring
#   make ledger-seam-check      檢查 + hermetic 契約測試（~5s）
#   make ledger-seam-selftest   只跑契約測試（fixture，不碰 git／production／Go toolchain）
#   make ledger-seam-update     以現況重寫 allowlist（新條目 reason 留 TODO，人工填寫後才 commit）
.PHONY: ledger-seam-check ledger-seam-selftest ledger-seam-update

ledger-seam-selftest:
	@echo "  → ledger seam regression tests（hermetic fixtures）"
	@bash tests/scripts/test-ledger-seam.sh

ledger-seam-check: ledger-seam-selftest
	@echo "  → ledger store seam check（禁新增裸 ledger.NewStore）"
	@bash scripts/ci/check_ledger_seam.sh

ledger-seam-update:
	@python3 scripts/ci/check_ledger_seam.py --root . --update
	@echo "   → 請編輯 scripts/ci/ledger-seam-allowlist.json 填寫 reason（TODO 開頭會讓檢查變紅）"

# ---- integration 路徑過濾器同步守門（FU-20261015-01，2026-10-16）----
#
#   ci-cd.yml 的 `integration` job 只在 `changes` 的 paths-filter 白名單命中時執行，
#   job 內跑整個 `go test -tags=integration -p 1 ./...`。白名單漏列「有 //go:build
#   integration 測試檔」的目錄 ⇒ 只改該目錄的 PR 讓 job 變 **skipping（不是 pass）**
#   ⇒ 那些測試在 CI 從未被觸發（本機綠、CI 未驗）。守門比對「repo 內帶 integration tag
#   的檔所在目錄」與白名單；例外必須逐條登錄理由（見 .py docstring）。
#   make integration-filter-check     檢查 + hermetic 契約測試（~2s）
#   make integration-filter-selftest  只跑契約測試（fixture，不碰 git／Go toolchain）
.PHONY: integration-filter-check integration-filter-selftest

integration-filter-selftest:
	@echo "  → integration 過濾器同步守門契約測試（hermetic fixtures）"
	@bash tests/scripts/test-integration-filter-sync.sh

integration-filter-check: integration-filter-selftest
	@echo "  → integration 路徑過濾器 vs 實際 integration 測試目錄"
	@bash scripts/ci/check_integration_filter_sync.sh

# ---- 合併結果驗證 + diff 衛生閘門（#1993，2026-09-26）----
# FAIL（evil merge）：用 `git merge-tree --write-tree` 算「併進 main 之後長怎樣」，若會改掉 main
#       上「本 PR 沒以普通 commit 引進」的內容 ⇒ 那是真的回退（唯一現實成因：落後分支在本地
#       解衝突時解錯）。修法：gh pr update-branch（遇衝突會拒絕）。
# WARN（diff 衛生）：分支落後 main 時 diff 會把別人已合併的改動顯示成刪除。⚠️ 那不是回退
#       （merge／squash／rebase 都走三方合併、保留 main 的版本），但會誤導 review／agent
#       （本 session 實證 #1974/#1979/#1990/#1991/#1994）。這一相不擋 CI；--strict 給本機用。
#       規格：docs/specs/branch-revert-guard-spec.md
#       allowlist：scripts/ci/revert-guard-allowlist.json（每項 reason 必填；只豁免 WARN 相）
#   make revert-guard           自我測試 + 檢查（base=origin/main, head=HEAD）
#   make revert-guard-selftest  只跑 hermetic 回歸測試（真 git fixture，不連網、不碰 production）
.PHONY: revert-guard revert-guard-selftest

revert-guard-selftest:
	@echo "  → revert guard regression tests（hermetic git fixtures）"
	@bash tests/scripts/test-revert-guard.sh

revert-guard: revert-guard-selftest
	@echo "  → 合併結果驗證 + diff 衛生檢查（#1993）"
	@bash scripts/ci/check_revert_guard.sh

# ---- 慢速檢查（逐支、無 timeout；清單為逐字明列的 declared slow checks）----
# FU-20261016-24（2026-10-16）：舊版只做 `[ -f "$$script" ]` 存在性判斷 ⇒ 清單內**四支全部
#   不存在**時（腳本被改名／搬到別的路徑／在非 repo 根目錄跑 make）迴圈 body 一次都沒進去，
#   收尾仍照印 `✅ ci-slow complete` 且 **rc=0** ⇒「什麼都沒跑」被當成通過（閘門說謊）。
#   實測（修前，真檔 Makefile）：`cd <空目錄> && make -f <repo>/Makefile ci-slow` ⇒ 只印 2 行、
#   rc=0；契約測試與兩段原文見 tests/scripts/test-ci-slow-zero-counter.sh。
# 三態判定（與 `ci:` 同紀律，但不引入 timeout —— 這幾支本來就慢）：
#   executed = 真的跑完且 rc=0 的支數；missing = 清單內找不到檔案的支數；total = 清單長度。
#   **executed=0 ⇒ 紅**（訊息逐字說明「沒有任何慢檢查被執行」）；missing>0 一律逐支印出，
#   但「部分缺失」**不**紅 —— 正常 checkout 四支都在，不誤紅（missing 只是可診斷的訊號）。
#   任何一支 rc≠0 ⇒ 立即紅（保留舊版的 fail-fast，不放寬）。
ci-slow:
	@echo "🛡️  Running slow CI checks (data_naming/layer3/markdown_links)..."
	@executed=0; missing=0; total=0; \
	for script in scripts/ci/check_data_naming.sh \
	              scripts/ci/check_layer3_benchmarks.sh \
	              scripts/ci/check_layer3_snapshots.sh \
	              scripts/ci/check_markdown_links.sh; do \
		total=$$((total+1)); \
		if [ ! -f "$$script" ]; then \
			echo "  ⏭️  MISSING: $$script — 清單內宣告的慢檢查不存在（被改名／搬走／工作目錄不對？）"; \
			missing=$$((missing+1)); \
			continue; \
		fi; \
		echo "  → $$script"; \
		rc=0; bash "$$script" || rc=$$?; \
		if [ $$rc -ne 0 ]; then \
			echo "    ❌ FAILED (rc=$$rc): $$script"; \
			echo "    ❌ ci-slow 中止：已完成 $$executed/$$total 支、$$missing 支不存在（本支 rc=$$rc 結束）"; \
			exit 1; \
		fi; \
		executed=$$((executed+1)); \
	done; \
	echo ""; \
	echo "✅ ci-slow: $$executed/$$total 支慢檢查被執行、$$missing 支不存在"; \
	if [ $$executed -eq 0 ]; then \
		echo "    ❌ 沒有任何慢檢查被執行 (executed=$$executed/$$total, missing=$$missing) ⇒『什麼都沒跑』不是通過（FU-20261016-24）"; \
		exit 1; \
	fi
	@echo "✅ ci-slow complete"

# #1742 P1: FinMind 每日 quota 檢查（runtime state 依賴，不進預設 make ci）。
# 用法: make ci-quota [STRICT=1] — prod/開市前用; 開發機無 state file 會跳過。
ci-quota:
	@echo "🛡️  Checking FinMind daily quota..."
	@if [ "$${STRICT:-0}" = "1" ]; then \
		bash scripts/ci/check_finmind_quota.sh --strict; \
	else \
		bash scripts/ci/check_finmind_quota.sh; \
	fi

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

.PHONY: rebuild-all rebuild-cron rebuild-atlas rebuild-host-bin check-binaries check-frontend-dist

HOST_GO       := $(shell which go 2>/dev/null || echo /opt/homebrew/bin/go)
export ATLAS_GIT_COMMIT ?= $(GIT_COMMIT)
BUILDTIME     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# CGO_ENABLED=0 keeps the linux binaries static: the atlas/cron images are
# alpine (musl), which cannot exec glibc-dynamic binaries (kernel ENOENT →
# "exec /app/atlas-go: no such file or directory"). Cross-builds default cgo
# off, but a native linux build (e.g. a linux runner) would default it ON.
GOENV_LINUX   := GOOS=linux GOARCH=arm64 CGO_ENABLED=0

LDFLAGS_BF := -w -s -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=dev \
              -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=$(GIT_COMMIT) \
              -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=$(BUILDTIME)

# Verify every deployed binary's buildinfo.Commit matches HEAD.
# Exit 0 = fresh, exit 1 = at least one stale.
check-binaries:
	@./scripts/check-binary-freshness.sh

# Frontend dist freshness gate: each frontend's dist/ must not be older than the
# newest file of the trees its esbuild config reads (<dir>/static +
# shared_web/static). The script names the offending source file and the newest
# dist file; it exits 1 on missing/placeholder-only dist (embed-dirs' `.keep`).
#
# Why a separate gate from check-binaries: check-binaries compares each deployed
# binary's buildinfo.Commit to HEAD. It cannot see inside the binary, so a fresh
# binary that embeds a STALE dist passes it. client_web/dist + admin_web/dist are
# gitignored build products embedded via `//go:embed all:dist`
# (cmd/atlas/main.go: fs.Sub(client_web.DistFS, "dist")) ⇒ nothing in git
# records which sources they were built from. 2026-10-07: Phase 0 removed the
# 錢潮預測卡 from the sources and the deployed /client/js/home-*.js still
# contained home-predictions, because the deploy path `make rebuild-all` did not
# rebuild the frontend and `git pull` cannot update a gitignored directory.
#
# Wired into `rebuild-all` right after check-binaries; also runnable standalone.
# Fix = `make build-frontend`.
.PHONY: check-frontend-dist
check-frontend-dist:
	@bash scripts/ci/check_frontend_dist.sh

# test-makefile-imac-guard — 驗 iMac fail-closed 防線是否生效
# 在 iMac 跑應看到:IMAC_DETECTED=1 + 模擬 rebuild-atlas 會 exit 1
# 在 MacBook 跑應看到:IMAC_DETECTED 為空 + 模擬 rebuild-atlas 會 exit 0
.PHONY: test-makefile-imac-guard
test-makefile-imac-guard:
	@echo "LocalHostName: $(IMAC_LOCAL_HOSTNAME)"
	@echo "IMAC_DETECTED: $(IMAC_DETECTED)"
	@echo "ALLOW_DEV_REBUILD_ON_IMAC: $(ALLOW_DEV_REBUILD_ON_IMAC)"
	@if [ "$(IMAC_DETECTED)" = "1" ] && [ "$(ALLOW_DEV_REBUILD_ON_IMAC)" != "1" ]; then \
		echo "→ iMac guard 啟用,'make rebuild-atlas' 會 fail-closed (期望行為)"; \
	else \
		echo "→ iMac guard 未啟用,'make rebuild-atlas' 會直接跑 (非 iMac 或顯式放行)"; \
	fi

# Quick integrity check: binary freshness + source formatting.
# Runs in < 30s; safe to run before every push.
check: check-binaries
	@echo "✓ make check passed"

# Rebuild host bin/atlas-mcp only.
rebuild-host-bin:
	@echo "  building host bin/atlas-mcp (commit=$(GIT_COMMIT))"
	@$(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o bin/atlas-mcp ./cmd/atlas-mcp

# Rebuild the 5 atlas-atlas binaries on host (for sandbox use with Dockerfile.atlas.local).
# .build-atlas/ is gitignored. atlas-go/atlas-mcp/daily-replay-sync/calibrate-parameters/calibrate-seasonal.
.build-atlas: ; @mkdir -p $@
rebuild-atlas-bins: | .build-atlas
	@echo "  building 5 atlas-atlas binaries on host"
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/atlas-go ./cmd/atlas
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/atlas-mcp ./cmd/atlas-mcp
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/daily-replay-sync ./cmd/daily-replay-sync
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/calibrate-parameters ./cmd/calibrate-parameters
	@$(GOENV_LINUX) $(HOST_GO) build -mod=mod -ldflags="$(LDFLAGS_BF)" -o .build-atlas/calibrate-seasonal ./cmd/calibrate-seasonal

# Rebuild atlas-atlas image from host-built binaries + restart container.
rebuild-atlas: rebuild-atlas-bins
	@if [ "$(IMAC_DETECTED)" = "1" ] && [ "$(ALLOW_DEV_REBUILD_ON_IMAC)" != "1" ]; then \
		echo '❌ iMac (KiMac) 偵測到 make rebuild-atlas,但 docker 步會用 dev compose 預設值'; \
		echo '   (POSTGRES_PASSWORD=atlas / port 5432 / container_name atlas-postgres 撞 prod 同名)'; \
		echo '   會改寫 prod 狀態 (實證:2026-08-26 PR #1695 二次破壞事件)'; \
		echo '   (此 guard 只在本機 hostname=KiMac 時觸發;現行 prod = Mac Mini,hostname KMacMini 不觸發)'; \
		echo '   現行 prod 部署:在 Mac Mini 執行 make rebuild-all（步驟見 docs/operations/local-deploy.md）'; \
		echo '   legacy iMac 的 prod compose 在 docs/operations/docker-compose.prod.yml'; \
		echo '     ⚠ 該 compose 的 atlas service 只有 image、沒有 build 段 -> 不能用來重建映像'; \
		echo '   確認要在 iMac 跑 dev rebuild:'; \
		echo '     ALLOW_DEV_REBUILD_ON_IMAC=1 make rebuild-atlas'; \
		exit 1; \
	fi
	@if [ "$$(git rev-parse --git-dir)" != "$$(git rev-parse --git-common-dir)" ]; then \
		echo "❌ 含 docker 的 rebuild 只能在主 worktree 執行（linked worktree 拒絕）"; exit 1; \
	fi
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker build -t atlas-atlas:local -f Dockerfile.atlas.local .
	@# Every build-only compose service in the "atlas family" runs the same image
	@# (compose names a build-only image "<project>-<service>"): the `atlas`
	@# service and `prism-worker` (command `prism worker`). Tag both, otherwise
	@# `compose up -d` reuses a stale image — 實證 2026-09-23: atlas-prism-worker
	@# stayed on commit 7126c3a7 while atlas-go moved to 91fd474f, and
	@# `check-binaries` reported FRESH because it did not cover that image.
	@for tag in $(ATLAS_IMAGE_TAGS); do docker tag atlas-atlas:local $$tag; done
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker compose up -d atlas prism-worker

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
# Images built from Dockerfile.atlas.local and retagged after the single build.
# `prism-worker` (compose command `prism worker`) shares the atlas image, so a
# missing entry here means `compose up -d` silently keeps a stale binary.
ATLAS_IMAGE_TAGS := atlas-atlas:latest atlas-prism-worker:latest

# Every build-only cron service declared in docker-compose.yml must appear here.
# Compose names a build-only image "<project>-<service>" (project = repo dir
# basename = atlas), and rebuild-cron retags the single atlas-cron-rebuilt:local
# image to each name before `compose up --no-build`. History: the three
# atlas-cron-backfill-* services were retired, but atlas-cron-darwinian was
# dropped from this list in the same cleanup, and cron-darwinian then failed at
# deploy time ("No such image: atlas-cron-darwinian:latest", 實證 2026-09-23
# Mac Mini)。tests/scripts/test-binary-freshness-guard.sh 以 docker-compose.yml
# 為準逐一比對，防止同類漂移再發生。
# 2026-09-26（E17）: cron-darwinian 服務本身已從 docker-compose.yml 移除，故其
# tag 與 rebuild-cron 的 compose up 清單一併移除。守門要求 tag 數量與 compose
# 的 build-only cron service 數量**相等** ⇒ 留著多餘的 tag 也會紅燈。
CRON_IMAGE_TAGS := atlas-cron-quote-backfill:latest \
                   atlas-cron-geo-ingest:latest atlas-atlas-cron-c07-collect:latest \
                   atlas-cron-replay-sync:latest \
                   atlas-atlas-cron-c07-evaluate:latest \
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
	@ATLAS_GIT_COMMIT=$(GIT_COMMIT) docker compose up -d --force-recreate --no-build cron-macro-ingest cron-quote-backfill cron-replay-sync atlas-cron-c07-evaluate cron-geo-ingest atlas-cron-c07-collect 

# Full rebuild: host binaries + atlas image + cron images.
#
# Prerequisites are deliberately composed from the narrow, independently usable
# targets (build-backend / rebuild-host-bin / rebuild-atlas / rebuild-cron) so
# callers that run those targets directly keep their exact semantics.
#
# Why build-backend is here (#2117): the final check-binaries judges BOTH host
# binaries (scripts/check-binary-freshness.sh → HOST_BINARIES=bin/atlas
# bin/atlas-mcp). rebuild-host-bin only produces bin/atlas-mcp, so on a host
# where bin/atlas exists from an earlier build (production: Mac Mini) the
# canonical flow `git pull && make rebuild-all` ended with
# `✗ STALE bin/atlas` ⇒ build-backend rc≠0 for a perfectly updated tree.
# Measured 2026-09-29 on a stale bin/atlas: check --host-only rc=1, after
# rebuild-host-bin rc=1 (unchanged), after build-backend rc=0.
# Why build-frontend is FIRST (#2026-10-07 frontend-dist incident): every Go
# prerequisite below embeds client_web/dist + admin_web/dist (`//go:embed
# all:dist`). Those directories are gitignored ⇒ `git pull` never updates them,
# so without this prerequisite the canonical deploy flow `git pull && make
# rebuild-all` builds fresh Go code around whatever dist the host last produced
# (or an embed-dirs `.keep` placeholder). Order matters: build-frontend must
# complete before the first embedding Go build, and before the final
# check-frontend-dist.
rebuild-all: build-frontend build-backend rebuild-host-bin rebuild-atlas rebuild-cron
	@echo "✓ rebuilt all binaries"
	@$(MAKE) check-binaries
	@$(MAKE) check-frontend-dist

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
#                  agent-prompts + constitution + channel-index + agents-md-drift +
#                  inert-closure（#1944 建議 2）+ zero-metric guard（FU-20261016-26）
#   ci-full     → ci-gate + lint (golangci-lint) + staticcheck + test + race + coverage +
#                  cmd/atlas integration + ci (all scripts) + ci-slow + orphan check
#
# 不可本地化的 CI job（仍須 GitHub 驗證）：
#   Docker build/push (multi-platform)、deploy、gosec SARIF upload、
#   跨 repo contract check (routes/contracts)

# 可注入的目錄清單（供 hermetic 測試用 EMBED_DIRS=... 覆寫；預設＝repo 內兩個 embed 目錄）
EMBED_DIRS ?= admin_web/dist client_web/dist

# ── 本機 worktree bootstrap：`//go:embed all:dist` 的前置目錄 ────────────────────
# 為什麼需要：`admin_web/embed.go` 與 `client_web/embed.go` 使用 `//go:embed all:dist`
#   ⇒ 只要 `admin_web/dist` 或 `client_web/dist` **不存在**，`go build ./...`
#     （連帶 `go vet` / `go generate` / `go test`）就會失敗：
#       `pattern all:dist: no matching files found`
#   「全新 worktree」（`git worktree add`）第一次跑本機閘門時**必然**遇到
#   ⇒ 每一條新 lane 的第一次 push 都被擋（2026-09-26 一天內 3 次同型事故）。
# 設計邊界（不可違反）：
#   ① 只在目錄**不存在**時建立空佔位（`.keep`）⇒ 冪等；目錄已存在就完全不動它。
#   ② CI **不受影響、也不可被遮蔽**：CI 的真 dist 來自 frontend 建置步驟；本目標在
#      CI 環境（環境變數 `CI` 非空）時**什麼都不做** ⇒ CI 若真缺 dist，`go build` 仍會紅。
#   ③ 不在 `go build` / `embed` 上做任何手腳（不放寬 pattern、不吞錯誤）。
# 相關：`FU-20260926-15`（host/worktree 環境前置，同族）。
.PHONY: embed-dirs
embed-dirs:
	@if [ -n "$${CI:-}" ]; then \
		echo "  → embed-dirs: CI 環境（CI=$${CI}）⇒ 跳過（真 dist 由 frontend 建置流程產生）"; \
	else \
		created=""; \
		for d in $(EMBED_DIRS); do \
			if [ ! -d "$${d}" ]; then \
				mkdir -p "$${d}"; \
				touch "$${d}/.keep"; \
				created="$${created} $${d}"; \
			fi; \
		done; \
		if [ -n "$${created}" ]; then \
			echo "  → embed-dirs: 已建立本機 dev 佔位:$${created}（各含 .keep，冪等）"; \
			echo "     ⚠️  這只是本機 dev 佔位（//go:embed all:dist 需要目錄存在）；正式建置請跑"; \
			echo "         make build-frontend —— 真 dist 會覆蓋它。"; \
		fi; \
	fi

.PHONY: ci-gate
# Contract tests for repository scripts (tests/scripts/*.sh) — OPT-IN ONLY.
#
# NOT wired into ci-gate: on 2026-09-23 a run of these tests inside a linked
# worktree made the fixture's git commands land in the CALLER's repository and
# rewrote the current branch with fixture commits (reflog evidence). Until every
# fixture is proven hermetic, run this target explicitly.
# Tracking: issue #1927 ("tests/scripts fixtures are not hermetic").
# test-check-frontend-dist.sh got a defensive own-repo assertion in the same
# change; the other fixtures still need the same treatment.
.PHONY: test-scripts
test-scripts:
	@echo "  → scripts contract tests (tests/scripts/*.sh; opt-in, see Makefile comment)"
	@for t in tests/scripts/*.sh; do \
		bash "$$t" >/dev/null || { echo "    ❌ $$t FAILED"; bash "$$t"; exit 1; }; \
	done
	@echo "    ✅"

# 本機閘門（含 pre-push 路徑）先 bootstrap embed 目錄；CI 內為 no-op（見 embed-dirs）
ci-gate: embed-dirs
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
	@echo "  → docs 同步檢查（AGENTS.md line drift + 內部斷鏈：docs/ 與全 repo 的 .md）"
	@bash scripts/ci/check_agents_md_drift.sh
	@bash scripts/ci/check_doc_links.sh
	@echo "    ✅"
	@echo "  → FOLLOWUPS 條目 id 唯一性（單檔；兩個 lane 撞號 ⇒ FAIL）"
	@bash scripts/ci/check_followups_unique_ids.sh
	@bash tests/scripts/test-followups-unique-ids.sh
	@echo "    ✅"
	@echo "  → 負向證明 harness 自我測試（#2011；餵 exit 127/2 必須紅燈）"
	@bash tests/scripts/test-negative-proofs.sh
	@echo "    ✅"
	@echo "  → 關鍵背景任務存在於 binary（DCE 防再犯，2026-08-10 事故）"
	@bash scripts/ci/check_critical_tasks.sh
	@echo "    ✅"
	@echo "  → binary freshness guard contract tests（唯一 hermetic 的 tests/scripts 測試；見 #1927）"
	@bash tests/scripts/test-binary-freshness-guard.sh
	@echo "    ✅"
	@echo "  → pre-push 閘門契約測試（E4 fail-closed fetch ＋ FU-20260926-15 host binary 新鮮度 ＋ FU-20261016-21 git 失敗≠docs-only；hermetic fixtures）"
	@bash tests/scripts/test-prepush-gates.sh
	@bash tests/scripts/test-ci-full-trigger.sh
	@echo "    ✅"
	@echo "  → embed-dirs 契約測試（hermetic：只用 mktemp 目錄，不建 git worktree）"
	@bash tests/scripts/test-embed-dirs.sh
	@echo "    ✅"
	@echo "  → agent-guard PreToolUse 接線契約測試（hermetic：只餵 payload，不執行危險指令）"
	@bash tests/scripts/test-agent-hook-wiring.sh
	@echo "    ✅"
	@echo "  → inert 閉環靜態檢查（#1944 建議 2；allowlist 見 scripts/ci/inert-baseline.json）"
	@bash scripts/ci/check_inert_closure.sh
	@echo "    ✅"
	@echo "  → ledger store seam 守門（T1/S1：禁新增裸 ledger.NewStore；hermetic 契約測試）"
	@bash tests/scripts/test-ledger-seam.sh
	@bash scripts/ci/check_ledger_seam.sh
	@echo "    ✅"
	@echo "  → integration 路徑過濾器同步守門（FU-20261015-01：漏列目錄 ⇒ integration job 變 skipping）"
	@bash tests/scripts/test-integration-filter-sync.sh
	@bash scripts/ci/check_integration_filter_sync.sh
	@echo "    ✅"
	@echo "  → monitoring 單一設定樹自我測試（hermetic fixtures；PR 階段攔『改錯棵』）"
	@bash tests/scripts/test-monitoring-single-source.sh
	@echo "    ✅"
	@echo "  → daily-maintenance 活性檢查契約測試（hermetic，不連網；E21）"
	@bash tests/scripts/test-check-task-liveness.sh
	@echo "    ✅"
	@echo "  → 全形字元緊鄰變數展開的靜態檢查（移植自 a2a-dev）＋自我測試"
	@bash scripts/ci/check_fullwidth_var_expansion.sh
	@bash tests/scripts/test-fullwidth-var-expansion.sh
	@echo "    ✅"
	@echo "  → 建 throwaway repo 的 GIT_* 隔離結構斷言（(a)7 變數 unset ／(b) 顯式定址）＋契約測試（C 正向／B 負向／A mutation 自證）"
	@bash scripts/ci/check_git_env_hermeticity.sh
	@bash tests/scripts/test-git-env-hermeticity.sh
	@echo "    ✅"
	@echo "  → 母體執行真值盤查工具的契約測試（hermetic fixtures；判層 + mutation 自證；2026-09-27）"
	@bash tests/scripts/test-verify-universe-run.sh
	@echo "    ✅"
	@echo "  → make ci 三態閘門契約測試（hermetic fixtures：掛住=紅／宣告式 skip=綠／mutation 自證；FU-20260926-12）"
	@bash tests/scripts/test-ci-timeout-tri-state.sh
	@echo "    ✅"
	@echo "  → make ci-slow 計數守衛契約測試（hermetic fixtures：0 支 ⇒ 紅／部分缺失不誤紅／mutation 自證；FU-20261016-24）"
	@bash tests/scripts/test-ci-slow-zero-counter.sh
	@echo "    ✅"
	@echo "  → ci-full orphan artifact check 契約測試（hermetic fixtures：git 失敗必紅／真的沒有仍綠／mutation 自證；FU-20261016-25）"
	@bash tests/scripts/test-ci-full-orphan-git-fail.sh
	@echo "    ✅"
	@echo "  → watchdog 重啟分類契約測試（hermetic：stub docker／假 HOME；8 case + mutation 自證；FU-20260930-10）"
	@bash tests/scripts/test-watchdog-restart-classify.sh
	@echo "    ✅"
	@echo "  → Markdown 連結檢查器的 git-aware 蒐集契約測試（hermetic 臨時 git repo；#2114）"
	@bash tests/scripts/test-check-markdown-links.sh
	@echo "    ✅"
	@echo "  → docs 斷鏈入口（check_doc_links.sh）契約測試：壞連結必紅／工具缺失 fail loud（hermetic；FU-20261016-18）"
	@bash tests/scripts/test-check-doc-links.sh
	@echo "    ✅"
	@echo "  → data catalog 新鮮度（check_data_catalog.sh）契約測試：stale 必指名且不得假綠／工具 rc=2 fail loud（hermetic；FU-20261016-22）"
	@bash tests/scripts/test-check-data-catalog.sh
	@echo "    ✅"
	@echo "  → 檔名大小寫閘門（check_case_conflicts.sh）契約測試：--strict 有違規必紅且指名檔名與條數／預設仍 WARNING（hermetic；FU-20261016-23）"
	@bash tests/scripts/test-check-case-conflicts.sh
	@echo "    ✅"
	@echo "  → 合併結果/ diff 衛生閘門自我測試（hermetic git fixtures；#1993）"
	@bash tests/scripts/test-revert-guard.sh
	@echo "    ✅"
	@echo "  → branch-protection 腳本契約測試（hermetic：gh 以 stub 隔離、不連 GitHub；E39）"
	@bash tests/scripts/test-branch-protection-known-checks.sh
	@bash tests/scripts/test-generate-drift-coverage.sh
	@echo "    ✅"
	@echo "  → 落後分支 diff 衛生 + evil-merge 檢查（#1993；allowlist 見 scripts/ci/revert-guard-allowlist.json）"
	@bash scripts/ci/check_revert_guard.sh
	@echo "    ✅"
	@echo "  → 零指標靜態守門（zero-metric-guard --fail：審計頁面不得直接呼叫非安全格式化）"
	@bash scripts/ci/zero-metric-guard.sh --fail
	@echo "    ✅"
	@echo "  → 死旗標契約測試（--json ≡ 裸 json／未知旗標 rc=2／jev 薄殼轉發／zero-metric 有呼叫端；hermetic；FU-20261016-26）"
	@bash tests/scripts/test-dead-flag-contract.sh
	@echo "    ✅"
	@echo "  → local-ci 判定機制契約測試（hermetic：第一支通過後不得中止／總結必須印；FU-20261016-28）"
	@bash tests/scripts/test-local-ci-completes-all-checks.sh
	@echo "    ✅"
	@echo "  → layer3 benchmarks 判定機制契約測試（hermetic：全 SKIP ⇒ 必紅／正常不誤紅；FU-20261016-28）"
	@bash tests/scripts/test-layer3-bench-zero-executed.sh
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
	@# 快取綁到「本 worktree 自己的 git dir」（2026-09-27, E22）:
	@# 預設 ~/Library/Caches/golangci-lint 是全機共用 ⇒ 別棵 worktree（含已被刪除的）的快取
	@# issue 會被本 worktree 命中重放（cache key 與 worktree 路徑無關: 只看 module 內相對路徑
	@# + 內容 hash），症狀 = issue 路徑出現 ../<別的 worktree>/ 前綴 與
	@# "[runner/source_code] Failed to get line N for file ...: can't read file"；nolint /
	@# generated-file 這類過濾器讀不到檔案時是 fail-open，於是抑制失效、在 0 個 .go 變更的
	@# 分支上假紅。改成 per-worktree 後：快取仍跨次執行保溫、worktree 刪除時隨
	@# .git/worktrees/<name> 一起消失，不再與別條 lane 共用同一批 issue 快取。
	@# 註: golangci-lint 的 "parallel golangci-lint is running" 鎖在 $TMPDIR/golangci-lint.lock，
	@# 是全機層級、與 cache 目錄無關 —— 那一個假紅不在本行處理範圍。
	@# 要覆寫（例如自訂路徑）：GOLANGCI_LINT_CACHE=/path make ci-full
	@GOLANGCI_LINT_CACHE="$${GOLANGCI_LINT_CACHE:-$$(git rev-parse --absolute-git-dir 2>/dev/null || echo "$${TMPDIR:-/tmp}")/golangci-lint-cache}" golangci-lint run --timeout=5m
	@echo "    ✅"
	@echo "  → standalone staticcheck"
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo "    ❌ staticcheck 未安裝。go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	}
	@staticcheck ./...
	@echo "    ✅"
	@echo "  → go test (excluding cmd/atlas heavy)"
	@ATLAS_STORE_BACKEND=sqlite go test -count=1 $$(go list ./... | grep -v '/cmd/atlas$$')
	@echo "    ✅"
	@echo "  → go test -race (excluding cmd/atlas)"
	@ATLAS_STORE_BACKEND=sqlite go test -race -count=1 $$(go list ./... | grep -v '/cmd/atlas$$')
	@echo "    ✅"
	@echo "  → cmd/atlas integration tests"
	@ATLAS_STORE_BACKEND=sqlite go test -count=1 -timeout=10m ./cmd/atlas/...
	@echo "    ✅"
	@echo "  → cmd/atlas-mcp-setup integration tests"
	@go test -tags=integration -count=1 -timeout=60s ./cmd/atlas-mcp-setup/
	@echo "    ✅"
	@echo "  → CI scripts (all)"
	@$(MAKE) --no-print-directory ci
	@$(MAKE) --no-print-directory ci-slow
	@echo "    ✅"
	@echo "  → coverage threshold (≥60%)"
	@# 每次執行用「自己的」暫存目錄（2026-09-27, E22）: 舊寫法把 coverprofile/log 硬編在共用
	@# /tmp/atlas-ci-full-coverage.* 且頭尾都 rm -f ⇒ 同機另一條 lane 開頭/結尾的 rm 會把
	@# 還在跑的本次 go test 的 coverprofile 從目錄移除（go test 是把 fd 開著寫到收尾），
	@# 收尾時就變成「取不到覆蓋率：coverprofile 缺失或為空」的假紅燈。trap 負責清理。
	@set -e; \
	COV_DIR="$$(mktemp -d "$${TMPDIR:-/tmp}/atlas-ci-full-cov.XXXXXX")"; \
	trap 'rc=$$?; if [ "$${rc}" -eq 0 ]; then rm -rf "$${COV_DIR}"; else echo "    ℹ️  覆蓋率 log 保留（失敗除錯用）：$${COV_DIR}"; fi' EXIT; \
	COV_PROFILE="$${COV_DIR}/coverage.out"; \
	COV_LOG="$${COV_DIR}/coverage.log"; \
	COV_FUNC_LOG="$${COV_DIR}/coverage-func.log"; \
	if ! go test -coverprofile="$${COV_PROFILE}" $$(go list ./... | grep -v '/cmd/atlas$$') > "$${COV_LOG}" 2>&1; then \
		echo "    ❌ go test 失敗（覆蓋率步驟）— log: $${COV_LOG}"; \
		tail -n 40 "$${COV_LOG}"; \
		exit 1; \
	fi; \
	if [ ! -s "$${COV_PROFILE}" ]; then \
		echo "    ❌ 取不到覆蓋率：coverprofile 缺失或為空（$${COV_PROFILE}）"; \
		exit 1; \
	fi; \
	if ! go tool cover -func="$${COV_PROFILE}" > "$${COV_FUNC_LOG}" 2>&1; then \
		echo "    ❌ 取不到覆蓋率：go tool cover 解析失敗 — log: $${COV_FUNC_LOG}"; \
		tail -n 20 "$${COV_FUNC_LOG}"; \
		exit 1; \
	fi; \
	COVERAGE=$$(awk '/^total:/ {print $$3}' "$${COV_FUNC_LOG}" | tr -d '\r' | sed 's/%//'); \
	echo "    Total coverage: $${COVERAGE}%"; \
	case "$${COVERAGE}" in \
		''|*[!0-9.]*) echo "    ❌ 取不到覆蓋率：COVERAGE 非數字（$${COVERAGE}）"; exit 1 ;; \
	esac; \
	if awk -v c="$${COVERAGE}" 'BEGIN{exit !(c+0 < 60)}'; then \
		echo "    ❌ Coverage $${COVERAGE}% 低於 60% 閾值"; \
		exit 1; \
	fi
	@echo "    ✅"
	@# FU-20261016-25（2026-10-16）：舊版用「命令替換 git ls-files」當迴圈清單 —— git 失敗時該清單是
	@# 空集合，與「真的沒有 orphan artifact」逐字相同 ⇒ 迴圈 body 一次都沒進去，收尾照印 ✅ 且 rc=0
	@# （與 FU-20261016-21「git 失敗被當成空集合」同根因）。現在顯式接 git 的 rc：rc≠0 ⇒ fail loud
	@# 並帶可診斷訊息；真的沒有 ⇒ 維持綠。另補 file(1) 存在的 fail-closed —— 舊版沒有 file(1) 時
	@# 偵測結果是空字串 ⇒ 二進位偵測整段靜默失效、照樣印 ✅（同一形狀）。
	@# 契約測試：tests/scripts/test-ci-full-orphan-git-fail.sh（已納入 make ci-gate）。
	@echo "  → orphan artifact check"
	@ORPHANS=""; \
	git_rc=0; \
	TRACKED=$$(git ls-files) || git_rc=$$?; \
	TRACKED_PID=$$(git ls-files '*.pid') || git_rc=$$?; \
	TRACKED_OUT=$$(git ls-files '*.out') || git_rc=$$?; \
	if [ $$git_rc -ne 0 ]; then \
		echo "❌ orphan artifact check: git ls-files 失敗（rc=$${git_rc}）⇒ 拿不到已追蹤的檔案清單"; \
		echo "   「清單取不到（空集合）」不等於「沒有 orphan artifact」⇒ 不以通過放行（fail loud；FU-20261016-25）。"; \
		echo "   修復：確認在 git work tree 內執行 make、且 git 可用（git rev-parse --is-inside-work-tree）。"; \
		exit 1; \
	fi; \
	command -v file >/dev/null 2>&1 || { \
		echo "❌ orphan artifact check: 找不到 file(1) ⇒ 二進位 orphan 偵測無法執行"; \
		echo "   「偵測工具缺失」不等於「沒有 orphan artifact」⇒ 不以通過放行（fail loud；FU-20261016-25）。"; \
		exit 1; \
	}; \
	for f in $$TRACKED; do \
		[ -f "$$f" ] || continue; \
		ft=$$(file "$$f" 2>/dev/null); \
		case "$$ft" in \
			*"Mach-O"*|*"ELF"*) ORPHANS="$$ORPHANS  [BINARY] $$f";; \
		esac; \
	done; \
	for f in $$TRACKED_PID; do \
		ORPHANS="$$ORPHANS  [PID] $$f"; \
	done; \
	for f in $$TRACKED_OUT; do \
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
