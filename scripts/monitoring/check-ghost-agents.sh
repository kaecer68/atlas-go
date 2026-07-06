#!/usr/bin/env bash
# scripts/monitoring/check-ghost-agents.sh
# 持續監控指令 - 檢查 growth-momentum-01 修復是否持續有效 + 偵測新幽靈 agent
#
# 用法:
#   bash scripts/monitoring/check-ghost-agents.sh           # 一次性檢查
#   */30 * * * * bash scripts/monitoring/check-ghost-agents.sh  # cron 每 30 分鐘
#
# 前置: atlas server 需在 localhost:18080 運行 (go run ./cmd/atlas -api -addr :18080)

set -e

API="${ATLAS_API:-http://localhost:18080}"
THRESHOLD_GHOST_DAYS="${THRESHOLD_GHOST_DAYS:-7}"
DATA_FILE="${ATLAS_DARWIN_STATE:-data/state/darwinian_weights.json}"

echo "=== Atlas Ghost-Agent Monitor ==="
echo "API: $API"
echo "Ghost threshold: $THRESHOLD_GHOST_DAYS days"
echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# 1. API 健康檢查
echo "--- 1. API health ---"
if ! curl -sf "$API/api/synergy/darwinian-status" -o /tmp/darwin-status.json; then
  echo "✗ FAIL: $API unreachable"
  exit 2
fi
TOTAL=$(python3 -c "import json; print(len(json.load(open('/tmp/darwin-status.json'))['agents']))")
echo "✓ API OK, $TOTAL agents registered"

# 2. growth-momentum-01 修復持久性
echo ""
echo "--- 2. growth-momentum-01 W0-Fix persistence ---"
GM=$(python3 -c "
import json
gm = json.load(open('/tmp/darwin-status.json'))['agents'].get('growth-momentum-01')
if not gm:
    print('MISSING')
else:
    print(f'signals={gm.get(\"total_signals\", 0)} status={gm.get(\"status\", \"?\")} sharpe={gm.get(\"rolling_sharpe\", 0):.2f}')
")
echo "growth-momentum-01: $GM"
if echo "$GM" | grep -q "MISSING"; then
  echo "✗ CRITICAL: growth-momentum-01 disappeared from runtime registry"
  exit 3
fi
if echo "$GM" | grep -q "signals=0 "; then
  echo "✗ WARNING: W0-Fix regression — signals=0 again"
  exit 4
fi

# 3. 全 agent ghost 偵測
echo ""
echo "--- 3. Ghost-agent detection (live API) ---"
GHOSTS=$(python3 -c "
import json
agents = json.load(open('/tmp/darwin-status.json'))['agents']
ghosts = [(a, v.get('status', '?')) for a, v in agents.items() if v.get('status') == 'ghost' or v.get('total_signals', 0) == 0]
if ghosts:
    for a, s in ghosts:
        print(f'  ✗ {a}: status={s}')
else:
    print('  ✓ No ghost agents')
")
echo "$GHOSTS"
GHOST_COUNT=$(echo "$GHOSTS" | grep -c "^  ✗" || true)
if [ "$GHOST_COUNT" -gt 0 ]; then
  echo "✗ $GHOST_COUNT ghost agent(s) detected"
  exit 5
fi

# 4. 磁碟狀態檔案的 7+ 天幽靈 (CI G11 邏輯複本)
echo ""
echo "--- 4. Ghost-agent detection (persisted state, $THRESHOLD_GHOST_DAYS day window) ---"
if [ -f "$DATA_FILE" ]; then
  STALE=$(python3 -c "
import json
from datetime import datetime, timezone, timedelta
threshold = datetime.now(timezone.utc) - timedelta(days=$THRESHOLD_GHOST_DAYS)
state = json.load(open('$DATA_FILE'))
stale = []
for agent_id, w in state.items():
    if w.get('total_signals', 0) > 0:
        continue
    last_adj = w.get('last_adjusted_at') or w.get('last_updated_at')
    if not last_adj:
        continue
    try:
        last_dt = datetime.fromisoformat(last_adj.replace('Z', '+00:00'))
        if last_dt < threshold:
            stale.append((agent_id, last_adj))
    except Exception:
        pass
if stale:
    for a, ts in stale:
        print(f'  ✗ {a}: last_adjusted_at={ts}')
else:
    print('  ✓ No stale ghost agents')
")
  echo "$STALE"
else
  echo "  (data file not found: $DATA_FILE)"
fi

echo ""
echo "=== Monitor complete ==="
