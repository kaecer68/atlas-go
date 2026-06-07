#!/usr/bin/env bash
# scripts/ci/seed_smoke_data.sh — Pre-seed empty state files for frontend smoke test.
#
# 用途：在 CI 啟動 atlas server 前建立必要的空資料檔案，避免 API handler
#       因為檔案不存在而回傳 500，進而觸發 console error、污染 smoke gate。
#
# 設計原則：
# - 只建立「沒有就不是錯誤，只是資料尚不存在」的檔案。
# - 檔案內容為合法的 JSON（空物件或空陣列），避免 unmarshal 錯誤。
# - 執行位置在 repo root。

set -euo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SELF_DIR/../.." && pwd)"
cd "$REPO_ROOT"

log() { echo "[seed] $*"; }

# ── phase3_metrics.json ──────────────────────────────────────────
# HandlePhase3Status 讀取此檔案；無模擬執行過就不存在，但也不該是 500。
mkdir -p data/state
if [[ ! -f data/state/phase3_metrics.json ]]; then
  echo '{"swarm_running":false,"swarm_consensus_symbols":0,"prism_queued_tasks":0,"prism_completed_results":0,"prism_top_agent_id":"","prism_top_agent_sharpe":0,"spawning_active":0,"spawning_candidates":0,"reflexivity_active_loops":0,"adversarial_last_score":0,"adversarial_vulnerabilities":[],"recorded_at":"0001-01-01T00:00:00Z"}' \
    > data/state/phase3_metrics.json
  log "created data/state/phase3_metrics.json (empty default)"
fi

# ── macro/latest.json ────────────────────────────────────────────
# loadLatestMacroSnapshot 讀取此檔案；無 macro ingest 時不存在。
mkdir -p data/state/macro
if [[ ! -f data/state/macro/latest.json ]]; then
  echo '{"vix":{"value":15,"change_pct":0},"retail_margin_balance":{"value":3000000000000,"change_pct":0,"symbol":"TWM"},"retail_short_balance":{"value":100000000000,"change_pct":0},"foreign_investor_net":{"value":0,"change_pct":0},"domestic_fund_net":{"value":0,"change_pct":0},"retail_margin_maintenance":{"value":1500000000000}}' \
    > data/state/macro/latest.json
  log "created data/state/macro/latest.json (empty default)"
fi

# ── baseline_policy.json ─────────────────────────────────────────
# baseline.Load() 讀取此檔案；不存在時只加 warning，不噴 error。
mkdir -p "$(dirname "$(cat internal/config/config.go 2>/dev/null | grep -o 'data/state/baseline_policy.json' || echo 'data/state')")"
if [[ ! -f data/state/baseline_policy.json ]]; then
  echo '{"version":0,"policy":[],"updated_at":"0001-01-01T00:00:00Z"}' \
    > data/state/baseline_policy.json
  log "created data/state/baseline_policy.json (empty default)"
fi

log "Seed complete"
