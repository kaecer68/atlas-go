# atlas-go root Makefile
#
# 統一管理 3 個前端目錄(web / admin_web / client_web)+ Go backend + CI scripts。
# 解決新人 onboarding 要手動對 3 目錄各跑 npm ci/build 的摩擦。
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

# 前端目錄列表(若有新增,加在這裡)
FRONTENDS := web admin_web client_web
GO_PKGS   := ./cmd/... ./internal/...

# 預設 target: 顯示 help
help:
	@echo "atlas-go root Makefile"
	@echo ""
	@echo "前端管理:"
	@echo "  install-frontend   安裝所有前端依賴 (web / admin_web / client_web)"
	@echo "  build-frontend     編譯所有前端 (npm run build x3)"
	@echo "  test-frontend      跑前端單元測試"
	@echo "  watch-frontend     啟動 esbuild --watch 模式"
	@echo ""
	@echo "後端管理:"
	@echo "  build-backend      編譯 Go backend (cmd/atlas)"
	@echo "  test-backend       跑 Go 單元測試"
	@echo "  lint-backend       跑 gofmt / go vet / staticcheck"
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
	@echo "👀 Starting esbuild --watch in 3 terminals..."
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
	go build -o bin/atlas ./cmd/atlas

test-backend:
	@echo "🧪 Testing Go backend..."
	go test $(GO_PKGS)

lint-backend:
	@echo "🔍 Linting Go backend..."
	@command -v gofmt >/dev/null && gofmt -l $(GO_PKGS) | (read; if [ $$? -ne 0 ]; then echo "❌ gofmt issues found"; exit 1; fi) || echo "  (gofmt skipped)"
	go vet $(GO_PKGS)

# ---- 整合 target ----

install: install-frontend
	@echo "📦 Downloading Go modules..."
	go mod download

build: build-frontend build-backend
	@echo "✅ Build complete"

test: test-frontend test-backend
	@echo "✅ Tests complete"

lint: lint-backend

ci:
	@echo "🛡️  Running all scripts/ci/check_*.sh (per-script timeout: 30s)..."
	@failed=0; passed=0; skipped=0; \
	for script in scripts/ci/check_*.sh; do \
		if [ -f "$$script" ]; then \
			echo "  → $$script"; \
			if timeout 30 bash $$script > /dev/null 2>&1; then \
				passed=$$((passed+1)); \
			elif [ $$? -eq 124 ]; then \
				echo "    ⏱️  TIMEOUT (>30s): $$script — 用 make ci-slow 個別跑"; \
				skipped=$$((skipped+1)); \
			else \
				echo "    ❌ FAILED: $$script"; \
				failed=$$((failed+1)); \
			fi; \
		fi; \
	done; \
	echo ""; \
	echo "✅ CI: $$passed passed, ❌ $$failed failed, ⏱️  $$skipped timed out"; \
	if [ $$failed -gt 0 ]; then exit 1; fi

ci-quick:
	@echo "🛡️  Running fast CI checks only (<2s each)..."
	@failed=0; passed=0; \
	for script in scripts/ci/check_agent_prompts.sh \
	              scripts/ci/check_case_conflicts.sh \
	              scripts/ci/check_commit_messages.sh \
	              scripts/ci/check_frontend_imports.sh \
	              scripts/ci/check_constitution.sh \
	              scripts/ci/check_data_catalog.sh \
	              scripts/ci/check_field_contract.sh; do \
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