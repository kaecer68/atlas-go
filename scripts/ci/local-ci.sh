#!/usr/bin/env bash
# scripts/ci/local-ci.sh
# 本地 CI 腳本：在推送前執行所有核心檢查，模擬 GitHub Actions 的 quality.yml + ci.yml。
#
# Usage:
#   bash scripts/ci/local-ci.sh              # 執行全部檢查
#   bash scripts/ci/local-ci.sh --quick      # 只執行快速檢查（格式、編譯、vet）
#   bash scripts/ci/local-ci.sh --no-lint    # 跳過 golangci-lint（較慢）

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

QUICK=false
NO_LINT=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quick) QUICK=true; shift ;;
    --no-lint) NO_LINT=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

PASS=0
FAIL=0

run_check() {
  local name="$1"
  shift
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo -e "${BLUE}▶ $name${NC}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  if "$@"; then
    echo -e "${GREEN}✅ $name passed${NC}"
    ((PASS++))
  else
    echo -e "${RED}❌ $name FAILED${NC}"
    ((FAIL++))
  fi
}

# ── Quick checks ──
run_check "gofmt formatting" bash -c '
  if [ -n "$(gofmt -l .)" ]; then
    echo "Please run gofmt -w . to fix:"
    gofmt -l .
    exit 1
  fi
'

run_check "go build" go build ./...

run_check "go vet" go vet ./...

if command -v staticcheck >/dev/null 2>&1; then
  run_check "staticcheck" staticcheck ./...
else
  echo -e "${YELLOW}⚠️  staticcheck not installed, skipping${NC}"
fi

# shellcheck: gate 真實風險 (rm -rf 變數空字串、未加引號的 word-split、cd 未檢查 等)
# 風格類 (SC2155, SC2034) 列印但不擋。沒裝 shellcheck 時 graceful skip。
run_check "shellcheck" bash scripts/ci/check_shellcheck.sh

run_check "case-conflict filenames" bash scripts/ci/check_case_conflicts.sh --strict

# ── Tests ──
if [ "$QUICK" = false ]; then
  run_check "go test" go test ./...

  # Coverage (exclude cmd/atlas — heavy integration)
  run_check "coverage threshold (>=60%)" bash -c '
    go test -coverprofile=coverage.out $(go list ./... | grep -v "/cmd/atlas$")
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk "{print \$3}" | tr -d "\r" | sed "s/%//")
    echo "Total coverage: $COVERAGE%"
    if echo "$COVERAGE 60" | awk "{exit !(\$1 < \$2)}"; then
      echo "Coverage is below 60% threshold"
      exit 1
    fi
  '
fi

# ── Lint ──
if [ "$NO_LINT" = false ] && [ "$QUICK" = false ]; then
  if command -v golangci-lint >/dev/null 2>&1; then
    run_check "golangci-lint" golangci-lint run --timeout=5m ./...
  else
    echo -e "${YELLOW}⚠️  golangci-lint not installed, skipping${NC}"
  fi
fi

# ── Go generate check ──
if [ "$QUICK" = false ]; then
  run_check "go generate" bash -c '
    go generate .
    if [ -n "$(git status --porcelain)" ]; then
      echo "ERROR: go generate produced uncommitted changes."
      git diff --stat
      exit 1
    fi
  '
fi

# ── Summary ──
echo ""
echo "════════════════════════════════════════════════════════════"
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}🎉 全部 $PASS 項檢查通過${NC}"
  echo "════════════════════════════════════════════════════════════"
  exit 0
else
  echo -e "${RED}💥 $FAIL 項檢查失敗，$PASS 項通過${NC}"
  echo "════════════════════════════════════════════════════════════"
  exit 1
fi
