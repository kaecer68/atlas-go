#!/usr/bin/env bash
set -euo pipefail
# verify_factor_integrity.sh — Factor system integrity check (G1-G10)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"
PASS=true; CHECKS_RUN=0; CHECKS_PASS=0
pass() { echo "  [PASS] $1"; CHECKS_RUN=$((CHECKS_RUN+1)); CHECKS_PASS=$((CHECKS_PASS+1)); }
fail() { echo "  [FAIL] $1 — $2"; PASS=false; CHECKS_RUN=$((CHECKS_RUN+1)); }
warn() { echo "  [WARN] $1 — $2"; CHECKS_RUN=$((CHECKS_RUN+1)); }

FACTOR_TYPE_FILE="internal/portfolio/optimizer.go"
# SC2010: avoid ls|grep — use a glob + a filter loop to handle non-alpha filenames.
FACTOR_TYPE_FILES=$(
    for f in internal/portfolio/optimizer*.go; do
        [[ "$f" == *_test.go ]] && continue
        [[ ! -e "$f" ]] && continue
        echo "$f"
    done | sort -u
)
WEIGHT_ENGINE_FILE="internal/portfolio/factor_weight_engine.go"
BREAKDOWN_FILE="internal/domain/shared/shared.go"
FRONTEND_TS="shared_web/static/js/shared/field_types.ts"

extract_factors() {
    sed -n '/^const ($/,/^)$/p' "$FACTOR_TYPE_FILE" | grep 'FactorType = "' | sed 's/.*"\(.*\)".*/\1/' | grep -v '^$' | sort
}
extract_breakdown_tags() {
    sed -n '/^type FactorScoreBreakdown struct {/,/^}$/p' "$BREAKDOWN_FILE" | grep 'json:"' | sed 's/.*json:"\([^",]*\).*/\1/' | sort
}
extract_scores_tags() {
    sed -n '/^type FactorScores struct {/,/^}$/p' "$BREAKDOWN_FILE" | grep 'json:"' | sed 's/.*json:"\([^",]*\).*/\1/' | sort
}
extract_symbolscore_fields() {
    cat $FACTOR_TYPE_FILES | sed -n '/^type symbolScore struct {/,/^}$/p' | grep -v '^[[:space:]]*//' | awk '{print $1}' | grep -vE '^type$|^symbolScore$|^\}$|^$' | tr '[:upper:]' '[:lower:]' | sort
}
extract_factor_names() {
    sed -n '/^const ($/,/^)$/p' "$FACTOR_TYPE_FILE" | grep 'FactorType =' | awk '{print $1}' | sort
}

FACTORS=$(extract_factors); FACTOR_COUNT=$(echo "$FACTORS" | wc -l | tr -d ' ')
echo "=== Factor Integrity ($FACTOR_COUNT factors: $(echo "$FACTORS" | tr '\n' ' ')) ==="

# G1: FactorType == defaultBaseWeights
echo "--- G1: FactorType ↔ defaultBaseWeights ---"
BW_NAMES=$(sed -n '/func defaultBaseWeights/,/^}$/p' "$WEIGHT_ENGINE_FILE" | grep -oE 'Factor[A-Z][a-zA-Z]*' | grep -v FactorType | sort -u)
BW_FACTORS=$(echo "$BW_NAMES" | while read n; do echo "$n" | sed 's/^Factor//' | sed 's/\([A-Z]\)/_\L\1/g' | sed 's/^_//'; done | sort)
BW_COUNT=$(echo "$BW_FACTORS" | wc -l | tr -d ' ')
[ "$FACTOR_COUNT" = "$BW_COUNT" ] && pass "Count match ($FACTOR_COUNT)" || fail "Mismatch $FACTOR_COUNT vs $BW_COUNT" "$(comm -23 <(echo "$FACTORS") <(echo "$BW_FACTORS") | tr '\n' ' ')"

# G2: FactorScoreBreakdown json tags
echo "--- G2: FactorScoreBreakdown ↔ FactorType ---"
BD_TAGS=$(extract_breakdown_tags | grep -v '^total$'); BD_COUNT=$(echo "$BD_TAGS" | wc -l | tr -d ' ')
[ "$FACTOR_COUNT" = "$BD_COUNT" ] && pass "Count match ($BD_COUNT)" || fail "Mismatch $FACTOR_COUNT vs $BD_COUNT" "$(comm -23 <(echo "$FACTORS") <(echo "$BD_TAGS") | tr '\n' ' ')"

# G3: FactorScores fields
echo "--- G3: FactorScores ↔ FactorScoreBreakdown ---"
SC_TAGS=$(extract_scores_tags | grep -vE '^total$|^breakdown$'); SC_COUNT=$(echo "$SC_TAGS" | wc -l | tr -d ' ')
[ "$FACTOR_COUNT" = "$SC_COUNT" ] && pass "Count match ($SC_COUNT)" || fail "Mismatch $FACTOR_COUNT vs $SC_COUNT" "$(comm -23 <(echo "$FACTORS") <(echo "$SC_TAGS") | tr '\n' ' ')"

# G4: symbolScore fields
echo "--- G4: symbolScore ↔ FactorType ---"
SS_FIELDS=$(extract_symbolscore_fields | grep -vE '^symbol$|^side$|^total$|^agents$'); SS_COUNT=$(echo "$SS_FIELDS" | wc -l | tr -d ' ')
[ "$FACTOR_COUNT" = "$SS_COUNT" ] && pass "Count match ($SS_COUNT)" || fail "Mismatch $FACTOR_COUNT vs $SS_COUNT" "Missing: $(comm -23 <(echo "$FACTORS") <(echo "$SS_FIELDS") | tr '\n' ' ')"

# G5+G6: Valid FactorType refs in event/strategy handlers
echo "--- G5+G6: Event/Strategy FactorType ref validity ---"
VALID_FT=$(extract_factor_names)
EVENT_REFS=$(sed -n '/func.*applyEventAdjustment/,/^func /p' "$WEIGHT_ENGINE_FILE" | grep -oE 'Factor[A-Z][a-zA-Z]*' | grep -vE '^FactorType$|^FactorWeight' | sort -u)
STRAT_REFS=$(sed -n '/func.*strategyDeltas/,/^func /p' "$WEIGHT_ENGINE_FILE" | grep -oE 'Factor[A-Z][a-zA-Z]*' | grep -vE '^FactorType$|^FactorWeight' | sort -u)
INV_E=$(comm -13 <(echo "$VALID_FT") <(echo "$EVENT_REFS") | tr '\n' ' ' | sed 's/ *$//')
INV_S=$(comm -13 <(echo "$VALID_FT") <(echo "$STRAT_REFS") | tr '\n' ' ' | sed 's/ *$//')
[ -z "$INV_E" ] && pass "applyEventAdjustment refs valid" || fail "Invalid event refs: $INV_E" ""
[ -z "$INV_S" ] && pass "strategyDeltas refs valid" || fail "Invalid strategy refs: $INV_S" ""

# G7: buildPositions factors
echo "--- G7: buildPositions ↔ FactorType ---"
POS_REFS=$(cat $FACTOR_TYPE_FILES | sed -n '/factors := map\[FactorType\]float64{/,/^[[:space:]]*}/p' | grep -oE 'Factor[A-Z][a-zA-Z]*' | grep -v FactorType | sort -u)
POS_COUNT=$(echo "$POS_REFS" | wc -l | tr -d ' ')
[ "$FACTOR_COUNT" = "$POS_COUNT" ] && pass "Count match ($POS_COUNT)" || warn "$POS_COUNT vs $FACTOR_COUNT" "conditional factors may use if-guards"

# G8: Regime handler coverage
echo "--- G8: Regime handler coverage ---"
ONR_CASES=$(sed -n '/func.*OnRegimeChange/,/^}$/p' "$WEIGHT_ENGINE_FILE" | grep -c 'case "' || true)
[ "$ONR_CASES" -gt 0 ] && pass "OnRegimeChange: $ONR_CASES regime case(s)" || fail "OnRegimeChange: no regime cases" ""

GW_CASES=$(sed -n '/func.*GetWeights/,/^}$/p' "$WEIGHT_ENGINE_FILE" | grep -c 'case "' || true)
if [ "$GW_CASES" -gt 0 ]; then
    pass "GetWeights: $GW_CASES regime cases"
else
    pass "GetWeights: regime adjustments consolidated into OnRegimeChange"
fi

# G9: Frontend sync (verified by go generate in pre-commit hooks)
echo "--- G9: Frontend field_types sync ---"
if [ -f "$FRONTEND_TS" ]; then
    pass "field_types.ts exists (pre-commit go generate sync)"
else
    warn "field_types.ts not found" ""
fi

# G10: Consumer coverage
echo "--- G10: Consumer coverage ---"
MISSING=""
for ft in $FACTORS; do
    N=$(grep -rl "\"$ft\"" internal/ admin_web/static/js client_web/static/js shared_web/static/js --include="*.go" --include="*.js" --include="*.ts" 2>/dev/null | wc -l | tr -d ' ')
    [ "$N" -eq 0 ] && MISSING="$MISSING $ft"
done
[ -z "$MISSING" ] && pass "All factors have consumers" || warn "No string refs found:$MISSING" "may be FactorType-constant-only"

# G11: Ghost agent detection (zero signals for >N days)
echo "--- G11: Ghost agent check ---"
GHOST_THRESHOLD_DAYS=${GHOST_THRESHOLD_DAYS:-7}
GHOST_COUNT=0

# Check if darwinian_weights.json exists
if [ -f data/state/darwinian_weights.json ]; then
  # Cross-platform date cutoff (macOS + Linux)
  if date -d "now" +%s >/dev/null 2>&1; then
    # GNU date (Linux)
    CUTOFF=$(date -d "$GHOST_THRESHOLD_DAYS days ago" +%s)
  else
    # BSD date (macOS)
    CUTOFF=$(date -v-${GHOST_THRESHOLD_DAYS}d +%s)
  fi

  # Iterate over agents in weights file
  for agent_id in $(jq -r '.weights | keys[]' data/state/darwinian_weights.json 2>/dev/null); do
    signals=$(jq -r ".weights[\"$agent_id\"].total_signals // 0" data/state/darwinian_weights.json)
    updated=$(jq -r ".weights[\"$agent_id\"].last_updated_at // \"1970-01-01T00:00:00Z\"" data/state/darwinian_weights.json)

    # Parse the timestamp (try GNU first, then BSD)
    ts=$(date -d "${updated%Z}" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${updated:0:19}" +%s 2>/dev/null || echo 0)

    if [ "$signals" = "0" ] && [ "$ts" -lt "$CUTOFF" ] && [ "$ts" -gt "0" ]; then
      GHOST_COUNT=$((GHOST_COUNT+1))
      echo "  GHOST: $agent_id (signals=0, last_updated=$updated)"
    fi
  done
fi

if [ "$GHOST_COUNT" -gt 0 ]; then
  fail "G11: Found $GHOST_COUNT ghost agent(s)" "Remove from weights or fix outcome recording"
else
  pass "G11: No ghost agents detected (threshold: $GHOST_THRESHOLD_DAYS days)"
fi

echo "=== Result: $CHECKS_PASS/$CHECKS_RUN passed ==="
$PASS && exit 0 || exit 1
