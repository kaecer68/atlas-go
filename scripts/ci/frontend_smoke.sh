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

# 1. 解析改動檔案 → 推導 page 清單
if [[ -n "${SMOKE_FORCE_PAGES:-}" ]]; then
  log "SMOKE_FORCE_PAGES set, using override: $SMOKE_FORCE_PAGES"
  PAGES="$SMOKE_FORCE_PAGES"
else
  log "Computing diff against $BASE_REF"
  if ! git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
    warn "Base ref $BASE_REF not found locally; falling back to full smoke"
    PAGES="overview,narrative,live,portfolio"
  else
    CHANGED=$(git diff --name-only "$BASE_REF"...HEAD 2>/dev/null || true)
    if [[ -z "$CHANGED" ]]; then
      log "No diff (empty commit or base==HEAD)"
      PAGES=""
    else
      log "Changed files:"
      echo "$CHANGED" | sed 's/^/    /'

      # 覆蓋表：路徑前綴 → page 清單
      PAGES_RAW=""
      declare -A PATH_MAP=(
        ["internal/marketdata/"]="overview,live"
        ["internal/baseline/"]="overview,live"
        ["internal/narrative/"]="narrative"
        ["internal/portfolio/"]="overview,portfolio,decision"
        ["internal/risk/"]="overview,portfolio,decision"
        ["internal/recommendation/"]="overview,portfolio,decision"
        ["internal/orchestrator/"]="overview,portfolio,decision"
        ["internal/industry/"]="industry"
        ["internal/monitoring/"]="overview,narrative,live,portfolio"
        ["internal/config/"]="overview,narrative,live,portfolio"
        ["cmd/atlas/"]="overview,narrative,live,portfolio"
        ["web/"]="overview,narrative,live,portfolio"
      )

      while IFS= read -r file; do
        for prefix in "${!PATH_MAP[@]}"; do
          if [[ "$file" == "$prefix"* ]]; then
            PAGES_RAW+="${PATH_MAP[$prefix]},"$'\n'
            break
          fi
        done
      done <<< "$CHANGED"

      # 去重、合併、移除空字串
      PAGES=$(echo "$PAGES_RAW" | tr ',' '\n' | sed '/^$/d' | sort -u | paste -sd ',' -)
    fi
  fi
fi

# 2. 決定最終行為
if [[ -z "$PAGES" ]]; then
  log "No relevant page mapped from diff → SKIP"
  ok "Smoke gate skipped (no relevant changes)"
  exit 0
fi

log "Smoke pages: $PAGES"
export SMOKE_PAGES="$PAGES"

# 3. Build frontend（產 web/dist/，go:embed 編譯時需要）
log "Building frontend (npm ci + npm run build)"
(cd web && npm ci --no-audit --no-fund 2>&1 | tail -20)
(cd web && npm run build 2>&1 | tail -5)
ok "Frontend built"

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
(cd web && npx playwright install --with-deps chromium 2>&1 | tail -10)

# 8. 跑 Playwright smoke
log "Running Playwright smoke"
SMOKE_EXIT=0
(cd web && ATLAS_PORT="$PORT" SMOKE_PAGES="$PAGES" SMOKE_TIMEOUT=5 npm run --silent test:smoke) || SMOKE_EXIT=$?

# 9. 結果
if [[ $SMOKE_EXIT -eq 0 ]]; then
  ok "Smoke PASSED for pages: $PAGES"
  exit 0
else
  err "Smoke FAILED (exit=$SMOKE_EXIT) for pages: $PAGES"
  err "See atlas log: /tmp/atlas-smoke.log"
  exit 1
fi
