#!/usr/bin/env bash
# tests/scripts/test-check-task-liveness.sh — daily-maintenance liveness 檢查的 **hermetic** 契約測試
#
# 受測對象：scripts/ci/check_task_liveness.sh（實作 check_task_liveness.py）
#
# 為什麼要有它（E21）：daily-maintenance.yml 的三個 job 原本每天呼叫**不存在**的 atlas CLI
# 子命令，CLI 靜默丟棄 ⇒ job 天天綠、artifact 天天空。修好之後，這個 job 唯一的閘門就是本檢查，
# 因此它的**鑑別力**（會不會把不健康判成健康、會不會在看不到快照時假綠）必須被釘住。
#
# 全程**不連網**：以 --file 餵入 fixture 快照；「抓不到端點」的情境用 loopback 的保留埠
# （127.0.0.1:9，discard）＋ --retries 1，屬確定性失敗，不需外網阻斷。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK="$ROOT/scripts/ci/check_task_liveness.sh"
WORKFLOW="$ROOT/.github/workflows/daily-maintenance.yml"

TMP=$(mktemp -d "${TMPDIR:-/tmp}/atlas-task-liveness.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

PASSED=0
FAILED=0
ok()  { echo "  ✓ $*"; PASSED=$((PASSED + 1)); }
bad() { echo "  ✗ $*" >&2; FAILED=$((FAILED + 1)); }

# chk DESC EXPECTED_ACTUAL
chk() {
  if [ "$2" = "$3" ]; then ok "$1 ($3)"; else bad "$1: got '$3', want '$2'"; fi
}

# task_json NAME STALE FAILURES ENABLED LAST_SUCCESS_JSON
task_json() {
  printf '{"name":"%s","stale":%s,"consecutive_failures":%s,"enabled":%s,"last_success_at":%s,"last_run_at":"2026-09-26T14:53:15Z","last_error":"","interval":"24h0m0s","source":"btm"}' \
    "$1" "$2" "$3" "$4" "$5"
}

# snapshot OUT TASKS_JSON... (TOTAL STALE_COUNT 由呼叫端負責傳入)
snapshot() {
  local out=$1 total=$2 stale=$3
  shift 3
  local tasks=""
  local t
  for t in "$@"; do
    [ -n "$tasks" ] && tasks="${tasks},"
    tasks="${tasks}${t}"
  done
  printf '{"generated_at":"2026-09-26T15:53:17Z","total":%s,"stale_count":%s,"tasks":[%s]}' \
    "$total" "$stale" "$tasks" > "$out"
}

run_check() { # OUT_FILE TASKS ...
  local out=$1 tasks=$2
  set +e
  bash "$CHECK" --label test --tasks "$tasks" --file "$SNAP" --artifact "$out" > "$TMP/stdout" 2> "$TMP/stderr"
  echo $?
  set -e
}

echo "── 受測腳本與 workflow 存在性 ──"
[ -f "$CHECK" ]    && ok "受測腳本存在: scripts/ci/check_task_liveness.sh"    || bad "受測腳本不存在"
[ -f "$WORKFLOW" ] && ok "受測 workflow 存在" || bad "workflow 不存在"

# ── ① 健康快照 → exit 0，artifact 標 pass ────────────────────────────────────
echo "── ① 健康快照 ──"
SNAP="$TMP/healthy.json"
snapshot "$SNAP" 2 0 "$(task_json auto_daily_simulation false 0 true '"2026-09-26T14:53:15Z"')" \
                        "$(task_json evolution_health false 0 true '"2026-09-26T14:58:14Z"')"
ART="$TMP/healthy-out.json"
chk "健康快照 ⇒ exit 0" "0" "$(run_check "$ART" auto_daily_simulation,evolution_health)"
if [ -f "$ART" ] && grep -q '"verdict": "pass"' "$ART"; then ok "artifact verdict=pass"; else bad "artifact 未產生或 verdict 不是 pass"; fi

# ── ② stale=true → exit 1（不得因為「有 last_success」就放行）────────────────
echo "── ② 逾期任務 ──"
SNAP="$TMP/stale.json"
snapshot "$SNAP" 1 1 "$(task_json auto_daily_simulation true 0 true '"2026-09-26T02:00:00Z"')"
ART="$TMP/stale-out.json"
chk "stale ⇒ exit 1" "1" "$(run_check "$ART" auto_daily_simulation)"
if grep -q 'stale' "$TMP/stdout"; then ok "輸出點名 stale 原因"; else bad "輸出未說明 stale"; fi
if grep -q '"verdict": "fail"' "$ART"; then ok "artifact verdict=fail"; else bad "artifact 未反映失敗"; fi

# ── ③ 任務缺席 → exit 1（改名／沒排程都要看得出來）─────────────────────────
echo "── ③ 必要任務缺席 ──"
SNAP="$TMP/missing.json"
snapshot "$SNAP" 1 0 "$(task_json some-other-task false 0 true '"2026-09-26T14:53:15Z"')"
ART="$TMP/missing-out.json"
chk "缺席 ⇒ exit 1" "1" "$(run_check "$ART" auto_daily_simulation)"
if grep -q 'missing from the liveness snapshot' "$TMP/stdout"; then ok "輸出點名缺席"; else bad "輸出未說明缺席"; fi

# ── ④ consecutive_failures 超標 → exit 1；--max-failures 可放寬 ─────────────
echo "── ④ 連續失敗門檻 ──"
SNAP="$TMP/failing.json"
snapshot "$SNAP" 1 0 "$(task_json prism_training false 3 true '"2026-09-26T14:52:57Z"')"
ART="$TMP/failing-out.json"
chk "失敗 3 次、門檻 0 ⇒ exit 1" "1" "$(run_check "$ART" prism_training)"
if grep -q 'consecutive_failures=3' "$TMP/stdout"; then ok "輸出帶失敗次數"; else bad "輸出未帶失敗次數"; fi
set +e
bash "$CHECK" --label test --tasks prism_training --file "$SNAP" --artifact "$TMP/tol.json" --max-failures 3 >/dev/null 2>&1
tolerant_rc=$?
set -e
chk "--max-failures 3 ⇒ exit 0（門檻是有效的）" "0" "$tolerant_rc"

# ── ⑤ 停用 / 從未成功 → exit 1 ─────────────────────────────────────────────
echo "── ⑤ 停用與從未成功 ──"
SNAP="$TMP/disabled.json"
snapshot "$SNAP" 1 0 "$(task_json evolution_health false 0 false '"2026-09-26T14:58:14Z"')"
chk "enabled=false ⇒ exit 1" "1" "$(run_check "$TMP/disabled-out.json" evolution_health)"
SNAP="$TMP/never.json"
snapshot "$SNAP" 1 0 "$(task_json evolution_health false 0 true null)"
chk "last_success_at=null ⇒ exit 1" "1" "$(run_check "$TMP/never-out.json" evolution_health)"

# ── ⑥ 看不到快照 = 無法驗證（exit 2），且**不得**寫出 pass artifact ───────────
echo "── ⑥ 無法驗證（fail-closed）──"
SNAP="$TMP/empty.json"
snapshot "$SNAP" 0 0
ART="$TMP/empty-out.json"
chk "tasks 為空 ⇒ exit 2" "2" "$(run_check "$ART" auto_daily_simulation)"
[ -f "$ART" ] && bad "無法驗證時竟然寫出 artifact" || ok "無法驗證時不寫 artifact"

printf '{"status":"degraded","error":"task liveness store unavailable","tasks":[]}' > "$TMP/degraded.json"
SNAP="$TMP/degraded.json"
chk "status=degraded ⇒ exit 2" "2" "$(run_check "$TMP/degraded-out.json" auto_daily_simulation)"

printf 'not-json-at-all' > "$TMP/broken.json"
SNAP="$TMP/broken.json"
chk "JSON 壞掉 ⇒ exit 2" "2" "$(run_check "$TMP/broken-out.json" auto_daily_simulation)"

set +e
bash "$CHECK" --label test --tasks auto_daily_simulation --url http://127.0.0.1:9/nope \
  --retries 1 --timeout 2 --artifact "$TMP/unreachable-out.json" > "$TMP/urlout" 2>&1
unreachable_rc=$?
set -e
chk "端點不可達 ⇒ exit 2（不是 0）" "2" "$unreachable_rc"
if grep -q 'CANNOT be verified' "$TMP/urlout"; then ok "輸出明示「無法驗證」"; else bad "輸出未明示無法驗證"; fi

set +e
bash "$CHECK" --file "$SNAP" >/dev/null 2>&1
usage_rc=$?
set -e
chk "缺 --tasks ⇒ exit 2（用法錯誤）" "2" "$usage_rc"

# ── ⑦ Markdown artifact（reflexivity job 用的是 .md glob）───────────────────
echo "── ⑦ Markdown artifact ──"
SNAP="$TMP/healthy.json"
chk "健康快照 ⇒ exit 0" "0" "$(run_check "$TMP/reflexivity.md" auto_daily_simulation)"
if grep -q '^| task |' "$TMP/reflexivity.md"; then ok "markdown 有表格"; else bad "markdown 缺表格"; fi
if grep -q 'verdict: PASS' "$TMP/reflexivity.md"; then ok "markdown 標 PASS"; else bad "markdown 未標 PASS"; fi

# ── ⑧ workflow 契約：閘門不得被關掉、也不得再呼叫不存在的子命令 ─────────────
echo "── ⑧ daily-maintenance workflow 契約 ──"
# 只看「活的」行：註解本身會提到這些字串（本 PR 的說明），不能算進契約計數。
ACTIVE="$TMP/workflow-active.yml"
grep -vE '^[[:space:]]*#' "$WORKFLOW" > "$ACTIVE"
calls=$(grep -c 'check_task_liveness.sh' "$ACTIVE" || true)
chk "三個 job 都呼叫本檢查" "3" "$calls"
failclosed=$(grep -c 'if-no-files-found: error' "$ACTIVE" || true)
chk "三個 upload 都維持 fail-closed" "3" "$failclosed"
swallowed=$(grep -c '|| true' "$ACTIVE" || true)
chk "沒有任何 || true（#2029 的 fail-closed 不被回退）" "0" "$swallowed"
# 舊寫法：`./atlas-go weights adjust --apply` / `./atlas-go prism status` / `./atlas-go reflexivity report`
# ⇒ 這些子命令不存在，必須不再出現在本 workflow（只允許 flag 式或 `prism worker`）。
bad_invocations=$(grep -hoE '\./atlas-go [a-z][a-z-]*( [a-z][a-z-]*)?' "$ACTIVE" | grep -v '^\./atlas-go prism worker$' || true)
chk "不再呼叫不存在的 CLI 子命令" "" "$bad_invocations"

echo ""
if [ "$FAILED" -eq 0 ]; then
  echo "✅ test-check-task-liveness: $PASSED passed"
  exit 0
fi
echo "❌ test-check-task-liveness: $PASSED passed, $FAILED failed" >&2
exit 1
