#!/usr/bin/env bash
# scripts/ci/frontend_smoke.sh — Atlas frontend smoke gate.
#
# 用途：根據 PR 改的檔案自動推導要 smoke 的 page，跑 Playwright headless
#       抓 #page-X innerText 掃 NaN/undefined/null，與 backend API 渲染一致。
#
# 設計理念：改 internal/marketdata/ 就只 smoke /overview 與 /live（這兩頁有
#           macroRadar）；改 internal/portfolio/ 就只 smoke /portfolio。避免
#           對無關改動做 full smoke，節省 CI token 與時間。
#
# 使用方式：
#   bash scripts/ci/frontend_smoke.sh [base_ref]
#   - base_ref: 預設 origin/main（本地對照）；CI 應傳 origin/${{ github.base_ref }}
#
# 環境變數：
#   ATLAS_PORT          — atlas listen port（預設 18080，避免與本機 8080 衝突）
#   SMOKE_FORCE_PAGES   — 跳過 diff 推論，直接 smoke 指定 pages（除錯用，逗號分隔）
#   SMOKE_FORCE_FRONTENDS — 跳過 diff 推論，直接指定要跑的 frontend： admin|client|both
#   SMOKE_SKIP          — 設為 1 時印 SKIP 並 exit 0
#
# 退出碼：
#   0 = smoke pass 或無相關 page（skip）
#   1 = smoke fail（任一 page 抓到 bad pattern 或 exception）
#   2 = build/server 啟動失敗
#   3 = diff 解析失敗

set -euo pipefail

BASE_REF="${1:-origin/main}"
PORT="${ATLAS_PORT:-18080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# 顏色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[smoke]${NC} $*"; }
ok() { echo -e "${GREEN}[ok]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
err() { echo -e "${RED}[err]${NC} $*"; }

cleanup() {
  if [[ -n "${ATLAS_PID:-}" ]] && kill -0 "$ATLAS_PID" 2>/dev/null; then
    log "Stopping atlas (pid=$ATLAS_PID)"
    kill "$ATLAS_PID" 2>/dev/null || true
    wait "$ATLAS_PID" 2>/dev/null || true
  fi
  if [[ -f /tmp/atlas-smoke.log ]]; then
    log "Atlas log preserved at /tmp/atlas-smoke.log"
  fi
}
trap cleanup EXIT

# 0. Skip 開關
if [[ "${SMOKE_SKIP:-0}" == "1" ]]; then
  log "SMOKE_SKIP=1, skipping smoke gate"
  exit 0
fi

cd "$REPO_ROOT"

# 1. 解析改動檔案 → 推導要跑的 frontend 與 page 清單
RUN_ADMIN=0
RUN_CLIENT=0
ADMIN_PAGES=""
CLIENT_PAGES=""

if [[ -n "${SMOKE_FORCE_FRONTENDS:-}" ]]; then
  case "$SMOKE_FORCE_FRONTENDS" in
    admin)  RUN_ADMIN=1; ADMIN_PAGES="${SMOKE_FORCE_PAGES:-home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config}" ;;
    client) RUN_CLIENT=1; CLIENT_PAGES="${SMOKE_FORCE_PAGES:-home,crossmarket,industry,narrative,capital-causality,strategies}" ;;
    both|*) RUN_ADMIN=1; RUN_CLIENT=1; ADMIN_PAGES="${SMOKE_FORCE_PAGES:-home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config}"; CLIENT_PAGES="${SMOKE_FORCE_PAGES:-home,crossmarket,industry,narrative,capital-causality,strategies}" ;;
  esac
  log "SMOKE_FORCE_FRONTENDS set, using override: admin=$RUN_ADMIN client=$RUN_CLIENT"
elif [[ -n "${SMOKE_FORCE_PAGES:-}" ]]; then
  # 保留舊行為：只強制 client_web 的 pages
  log "SMOKE_FORCE_PAGES set, using override for client: $SMOKE_FORCE_PAGES"
  RUN_CLIENT=1
  CLIENT_PAGES="$SMOKE_FORCE_PAGES"
else
  log "Computing diff against $BASE_REF"
  if ! git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
    warn "Base ref $BASE_REF not found locally; falling back to full smoke"
    RUN_ADMIN=1
    RUN_CLIENT=1
    ADMIN_PAGES="home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config"
    CLIENT_PAGES="home,crossmarket,industry,narrative,capital-causality,strategies"
  else
    CHANGED=$(git diff --name-only "$BASE_REF"...HEAD 2>/dev/null || true)
    if [[ -z "$CHANGED" ]]; then
      log "No diff (empty commit or base==HEAD)"
    else
      log "Changed files:"
      echo "$CHANGED" | sed 's/^/    /'

      # admin_web 專用覆蓋表（與 admin_web/static/index.html 保留的頁面對齊）
      declare -A ADMIN_PATH_MAP=(
        ["admin_web/"]="home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config"
        ["shared_web/"]="home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config"
        ["cmd/atlas/"]="home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config"
      )

      # client_web 專用覆蓋表
      declare -A CLIENT_PATH_MAP=(
        ["internal/marketdata/"]="crossmarket"
        ["internal/baseline/"]="crossmarket"
        ["internal/narrative/"]="narrative"
        ["internal/portfolio/"]="crossmarket"
        ["internal/risk/"]="crossmarket"
        ["internal/recommendation/"]="crossmarket"
        ["internal/orchestrator/"]="crossmarket"
        ["internal/industry/"]="industry"
        ["internal/monitoring/"]="crossmarket,narrative"
        ["internal/config/"]="crossmarket,narrative"
        ["cmd/atlas/"]="crossmarket,narrative"
        ["client_web/"]="crossmarket,narrative,capital-causality,strategies"
        ["shared_web/"]="crossmarket,narrative,capital-causality,strategies"
      )

      ADMIN_RAW=""
      CLIENT_RAW=""
      while IFS= read -r file; do
        for prefix in "${!ADMIN_PATH_MAP[@]}"; do
          if [[ "$file" == "$prefix"* ]]; then
            ADMIN_RAW+="${ADMIN_PATH_MAP[$prefix]},"$'\n'
            RUN_ADMIN=1
            break
          fi
        done
        for prefix in "${!CLIENT_PATH_MAP[@]}"; do
          if [[ "$file" == "$prefix"* ]]; then
            CLIENT_RAW+="${CLIENT_PATH_MAP[$prefix]},"$'\n'
            RUN_CLIENT=1
            break
          fi
        done
      done <<< "$CHANGED"

      ADMIN_PAGES=$(echo "$ADMIN_RAW" | tr ',' '\n' | sed '/^$/d' | sort -u | paste -sd ',' -)
      CLIENT_PAGES=$(echo "$CLIENT_RAW" | tr ',' '\n' | sed '/^$/d' | sort -u | paste -sd ',' -)
    fi
  fi
fi

# 2. 決定最終行為
if [[ $RUN_ADMIN -eq 0 && $RUN_CLIENT -eq 0 ]]; then
  log "No relevant frontend mapped from diff → SKIP"
  ok "Smoke gate skipped (no relevant changes)"
  exit 0
fi

log "Smoke plan: admin=$RUN_ADMIN pages=${ADMIN_PAGES:-(none)} | client=$RUN_CLIENT pages=${CLIENT_PAGES:-(none)}"

# 3. Build frontends（產 admin_web/client_web dist/，go:embed 編譯時需要）
log "Building frontends (admin_web + client_web)"
bash "$REPO_ROOT/scripts/ci/build_all_frontends.sh"
ok "Frontends built"

# 4. Build atlas binary
log "Building atlas binary"
go build -o atlas-go ./cmd/atlas
ok "Atlas binary built: $(ls -lh atlas-go | awk '{print $5}')"

# 4.5 預先建立空資料檔案（避免 API 因檔案不存在而回傳 500）
log "Pre-seeding data directory with empty state files"
bash "$REPO_ROOT/scripts/ci/seed_smoke_data.sh"
ok "Data directory seeded"

# 5. 啟動 atlas server（背景）
log "Starting atlas server on :$PORT (log: /tmp/atlas-smoke.log)"
ATLAS_PORT="$PORT" \
  ./atlas-go -api -addr ":$PORT" \
    -log-format text \
    -broker-mode dry-run -broker-adapter mock -broker-signer placeholder \
    > /tmp/atlas-smoke.log 2>&1 &
ATLAS_PID=$!
log "Atlas pid=$ATLAS_PID"

# 6. 輪詢 /health 等就緒（deadline 120s 涵蓋 macro ingest + 外部 API 啟動）
log "Waiting for /health (deadline 120s)"
HEALTH_URL="http://localhost:$PORT/health"
DEADLINE=$(( $(date +%s) + 120 ))
READY=0
while [[ $(date +%s) -lt $DEADLINE ]]; do
  if curl -sf -m 2 "$HEALTH_URL" >/dev/null 2>&1; then
    READY=1
    break
  fi
  if ! kill -0 "$ATLAS_PID" 2>/dev/null; then
    err "Atlas process died before becoming healthy"
    tail -50 /tmp/atlas-smoke.log
    exit 2
  fi
  sleep 1
done

if [[ $READY -ne 1 ]]; then
  err "Atlas did not become healthy within 60s"
  tail -100 /tmp/atlas-smoke.log
  exit 2
fi
ok "Atlas healthy at $HEALTH_URL"

# 7. 安裝 playwright chromium（總是執行，npx 會善用快取；避免 CI 環境版本對不上導致找不到 binary）
log "Installing playwright chromium"
if [[ $RUN_ADMIN -eq 1 ]]; then
  (cd admin_web && npx playwright install --with-deps chromium 2>&1 | tail -10)
fi
if [[ $RUN_CLIENT -eq 1 ]]; then
  (cd client_web && npx playwright install --with-deps chromium 2>&1 | tail -10)
fi

# 8. 跑 Playwright smoke
OVERALL_EXIT=0

if [[ $RUN_ADMIN -eq 1 ]]; then
  log "Running admin_web Playwright smoke"
  ADMIN_EXIT=0
  (cd admin_web && ATLAS_PORT="$PORT" SMOKE_PAGES="$ADMIN_PAGES" SMOKE_TIMEOUT=5 npm run --silent test:smoke) || ADMIN_EXIT=$?
  if [[ $ADMIN_EXIT -ne 0 ]]; then
    err "admin_web smoke FAILED (exit=$ADMIN_EXIT) for pages: $ADMIN_PAGES"
    OVERALL_EXIT=1
  else
    ok "admin_web smoke PASSED for pages: $ADMIN_PAGES"
  fi
fi

if [[ $RUN_CLIENT -eq 1 ]]; then
  log "Running client_web Playwright smoke"
  CLIENT_EXIT=0
  (cd client_web && ATLAS_PORT="$PORT" SMOKE_PAGES="$CLIENT_PAGES" SMOKE_TIMEOUT=5 npm run --silent test:smoke) || CLIENT_EXIT=$?
  if [[ $CLIENT_EXIT -ne 0 ]]; then
    err "client_web smoke FAILED (exit=$CLIENT_EXIT) for pages: $CLIENT_PAGES"
    OVERALL_EXIT=1
  else
    ok "client_web smoke PASSED for pages: $CLIENT_PAGES"
  fi
fi

# 9. 結果
if [[ $OVERALL_EXIT -eq 0 ]]; then
  ok "All smoke PASSED"
  exit 0
else
  err "Smoke FAILED. See atlas log: /tmp/atlas-smoke.log"
  exit 1
fi
