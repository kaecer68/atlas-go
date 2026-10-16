#!/usr/bin/env bash
# ============================================================
# test-layer3-bench-zero-executed.sh — `scripts/ci/check_layer3_benchmarks.sh` 的
#                                      「executed=0 ⇒ 紅」契約測試
# ------------------------------------------------------------
# 【來源】FU-20261016-28（缺陷 ②）：真檔 `:67-80,131`。7 個 target **全部**因為
#   `testdata/` 不存在或 testdata 內沒有 `*_bench.txt` baseline 而走 SKIP 時，`FAILED` 仍是 0
#   ⇒ 收尾無條件印 `[layer3-bench] all benchmarks within 5% threshold (0 passed, 7 skipped)`
#   且 **rc=0** ⇒「一條都沒比到」被當成通過（與 FU-20261016-24 的 `ci-slow` 同族：
#   `executed=0 ⇒ 紅`）。
#   修前逐字原文（真檔複製進空 fixture，2026-10-16 本車道實跑）：
#     $ BENCHSTAT=/usr/bin/true bash scripts/ci/check_layer3_benchmarks.sh
#     [layer3-bench] SKIP internal/config: no testdata/ directory
#     ... （7 行 SKIP）
#     [layer3-bench] all benchmarks within 5% threshold (0 passed, 7 skipped)
#     rc=0
#
# 【為什麼複製真檔而不是改寫】同上：抄一份的話真檔改回來時測試還在驗舊行為。
#   本測試把**真檔**複製進 mktemp fixture（腳本的 REPO_ROOT 由 BASH_SOURCE 推導 ⇒ 自然落在
#   fixture），只以 `BENCHSTAT`（stub）與 `LAYER3_BENCH_COUNT` 控制外部依賴 ⇒ 不連網、不跑真
#   repo 的 benchmark。目標清單由真檔內文推導（`declare -a TARGETS=`）。
#
# 【覆蓋】
#   ① 全 SKIP（無任何 target 目錄）⇒ 必紅 ＋ 逐字說明「沒有任何 benchmark 被實際檢查」＋不再印
#      「within ... threshold」（修前是 rc=0 ＋ 印 threshold）
#   ② 全 SKIP 的第二條路徑（target 目錄存在、testdata 內沒有 baseline）⇒ 同樣必紅
#   ③ 正常（真的跑一個 target 的 `go test -bench`；stub benchstat exit 0）⇒ rc=0、印
#      `executed=1/N targets` 與 `within ... threshold`（不誤紅）
#   ④ benchstat 判 regression ⇒ rc=1（原有牙齒不得被本修法放寬）
#   ⑤ 真檔靜態契約：`EXECUTED=$((EXECUTED + 1))` 與 `if [ "${EXECUTED}" -eq 0 ]; then` 都在
#   ⑥ mutation 自證：拿掉 fail-closed 判定 ⇒ ① 的假綠復現（rc=0 ＋ 印 threshold），
#      且 ① 的判準在該結果上**變紅**
#
# 執行： bash tests/scripts/test-layer3-bench-zero-executed.sh
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/scripts/ci/check_layer3_benchmarks.sh"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }
log_show() { negproof_show_log "${1:-}"; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/fu28-layer3bench.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

negproof_require_file "$SRC" "受測檔案 scripts/ci/check_layer3_benchmarks.sh" || exit 1
command -v go >/dev/null 2>&1 || fail "找不到 go（③ 需要真的跑一次 go test -bench）"

# 目標清單由真檔內文推導（不在測試裡寫死）
NTARGETS="$(sed -n '/^declare -a TARGETS=(/,/^)/p' "$SRC" | grep -cE '^\s*"' || true)"
[ "${NTARGETS:-0}" -ge 1 ] || fail "抽不到真檔的 TARGETS 清單 ⇒ 空集合不是通過"
ok "真檔宣告 ${NTARGETS} 個 target"

# stub benchstat：③ 用它回報 PASS，④ 用它回報 regression（不需真的安裝 benchstat）
STUB_OK="$TMP/benchstat-ok"
STUB_FAIL="$TMP/benchstat-fail"
printf '#!/bin/sh\necho "□ no regression"\nexit 0\n' >"$STUB_OK"
printf '#!/bin/sh\necho "✗ regression" >&2\nexit 1\n' >"$STUB_FAIL"
chmod +x "$STUB_OK" "$STUB_FAIL"

# new_fixture <dir> [<腳本檔，預設真檔>] — 只放 scripts/ci/ 下的受測腳本（REPO_ROOT = fixture）
new_fixture() {
  local d="$1" src="${2:-$SRC}"
  mkdir -p "$d/scripts/ci"
  cp "$src" "$d/scripts/ci/check_layer3_benchmarks.sh"
}

# make_go_target <dir> — 在 fixture 內造出「可真的跑 benchmark」的最小 Go target
#   （internal/config 是 TARGETS 的第一支；baseline 檔名符合 `*_bench.txt`）
make_go_target() {
  local d="$1"
  cat >"$d/go.mod" <<'EOF'
module fu28fixture

go 1.21
EOF
  mkdir -p "$d/internal/config/testdata"
  cat >"$d/internal/config/fixture_test.go" <<'EOF'
package config

import (
	"strconv"
	"testing"
)

func BenchmarkFixture(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = strconv.Itoa(i)
	}
}
EOF
  cat >"$d/internal/config/testdata/fixture_bench.txt" <<'EOF'
goos: darwin
goarch: arm64
pkg: fu28fixture/internal/config
cpu: fixture
BenchmarkFixture-8   	  100000	        30.00 ns/op
EOF
}

# run_gate <fixture> <log> <stub benchstat> — 印出 rc（一律從這裡讀，不經管線）
run_gate() {
  local d="$1" log="$2" stub="$3" rc=0
  ( cd "$d" \
      && env BENCHSTAT="$stub" LAYER3_BENCH_COUNT=1 GOPROXY=off \
           bash scripts/ci/check_layer3_benchmarks.sh ) >"$log" 2>&1 || rc=$?
  printf '%s' "$rc"
}

# assert_zero_executed_red <rc> <log> — 核心判準：「一條都沒比到」必須是非 0 ＋ 指名原因
#   ＋ **不再**宣稱全部在門檻內。⑥ 拿它去驗突變後的假綠結果，證明它有鑑別力。
assert_zero_executed_red() {
  local rc="$1" log="$2"
  [ "$rc" -ne 0 ] || return 1
  grep -Fq '沒有任何 benchmark 被實際檢查' "$log" || return 1
  if grep -Fq 'all benchmarks within' "$log"; then return 1; fi
  return 0
}

echo "── ① 全 SKIP（無任何 target 目錄）⇒ 必紅 ────────────────────────"
D1="$TMP/f1"
new_fixture "$D1"
L1="$TMP/f1.log"
RC1="$(run_gate "$D1" "$L1" "$STUB_OK")"
assert_zero_executed_red "$RC1" "$L1" || {
  log_show "$L1"
  fail "① 全部 target SKIP 卻沒有紅（rc=${RC1}）——『一條都沒比到』不是通過（FU-20261016-28 未修？）"
}
negproof_expect_output "executed=0/${NTARGETS}" "$L1" \
  "① 紅燈要說出 executed=0/N（與 FU-20261016-24 的計數自述同語意）" || { log_show "$L1"; exit 1; }
ok "① rc=${RC1}（非 0），訊息逐字說明『沒有任何 benchmark 被實際檢查』且不再印 threshold"

echo "── ② 全 SKIP 的第二條路徑（有 testdata 但無 baseline）⇒ 必紅 ──"
D2="$TMP/f2"
new_fixture "$D2"
mkdir -p "$D2/internal/config/testdata" # 目錄在，但沒有 *_bench.txt
L2="$TMP/f2.log"
RC2="$(run_gate "$D2" "$L2" "$STUB_OK")"
negproof_expect_output 'no *_bench.txt baselines' "$L2" \
  "② 必須走到「無 baseline」這條 SKIP 路徑（否則本 case 沒測到第二條分支）" || { log_show "$L2"; exit 1; }
assert_zero_executed_red "$RC2" "$L2" || {
  log_show "$L2"
  fail "② 第二條 SKIP 路徑沒有紅（rc=${RC2}）"
}
ok "② rc=${RC2}（非 0），第二條 SKIP 路徑同樣 fail-closed"

echo "── ③ 正常（真的跑 1 個 target）⇒ 綠 ＋ 計數 ────────────────────"
D3="$TMP/f3"
new_fixture "$D3"
make_go_target "$D3"
L3="$TMP/f3.log"
RC3="$(run_gate "$D3" "$L3" "$STUB_OK")"
[ "$RC3" -eq 0 ] || { log_show "$L3"; fail "③ 有 benchmark 真的被檢查卻誤紅（rc=${RC3}）——不得把正常情形染紅"; }
negproof_expect_output "executed=1/${NTARGETS} targets" "$L3" \
  "③ 計數行必須說出 executed=1/N" || { log_show "$L3"; exit 1; }
negproof_expect_output 'DIFF internal/config/fixture_bench.txt' "$L3" \
  "③ 真的走到 benchstat 比對（不是 micro-benchmark 分支）" || { log_show "$L3"; exit 1; }
negproof_expect_output 'all benchmarks within' "$L3" \
  "③ 正常情形仍要印原本的成功訊息（既有輸出契約不變）" || { log_show "$L3"; exit 1; }
ok "③ rc=${RC3}，executed=1/${NTARGETS}，照印 threshold 成功訊息"

echo "── ④ benchstat 判 regression ⇒ 仍必紅（原有牙齒不放寬）────────"
L4="$TMP/f4.log"
RC4="$(run_gate "$D3" "$L4" "$STUB_FAIL")"
[ "$RC4" -ne 0 ] || { log_show "$L4"; fail "④ benchstat 回報 regression 卻回綠（rc=${RC4}）——牙齒被放寬了"; }
negproof_expect_output 'one or more benchmarks regressed' "$L4" \
  "④ 原有的 regression 紅燈訊息必須保留" || { log_show "$L4"; exit 1; }
negproof_expect_output "executed=1/${NTARGETS} targets" "$L4" \
  "④ regression 的 FAIL summary 也要帶 executed 計數" || { log_show "$L4"; exit 1; }
ok "④ rc=${RC4}（非 0），regression 紅燈與計數皆保留"

echo "── ⑤ 真檔靜態契約：EXECUTED 計數與 fail-closed 判定都在 ────────"
grep -Fq 'EXECUTED=$((EXECUTED + 1))' "$SRC" \
  || fail "⑤ 真檔沒有『真的跑過』計數 EXECUTED（FU-20261016-28 回歸？）"
grep -Fq 'if [ "${EXECUTED}" -eq 0 ]; then' "$SRC" \
  || fail "⑤ 真檔收尾沒有 executed=0 的 fail-closed 判定（FU-20261016-28 回歸？）"
ok "⑤ EXECUTED 計數 ＋ executed=0 fail-closed 判定都在"

echo "── ⑥ mutation 自證：拿掉 fail-closed ⇒ ① 的假綠必須復現 ───────"
MUT="$TMP/mut.sh"
sed 's/if \[ "\${EXECUTED}" -eq 0 \]; then/if false; then/' "$SRC" >"$MUT"
CH=$(diff "$SRC" "$MUT" | grep -c '^[<>]' || true)
[ "${CH:-0}" -eq 2 ] || fail "⑥ mutation 沒有真的改到 fail-closed 判定（diff 行數=${CH:-0}，期望 2）⇒ 突變是裝飾品"
D6="$TMP/f6"
new_fixture "$D6" "$MUT"
L6="$TMP/f6.log"
RC6="$(run_gate "$D6" "$L6" "$STUB_OK")"
# 突變後重現**修前行為**：0 支被檢查仍印 threshold 並回 0
[ "$RC6" -eq 0 ] || { log_show "$L6"; fail "⑥ 拿掉 fail-closed 後 0 支竟仍非 0（rc=${RC6}）⇒ 突變沒改到判準"; }
negproof_expect_output "all benchmarks within 5% threshold (0 passed, ${NTARGETS} skipped)" "$L6" \
  "⑥ 突變後必須重現修前的假綠訊息逐字" || { log_show "$L6"; exit 1; }
if assert_zero_executed_red "$RC6" "$L6"; then
  fail "⑥ ①的判準對突變後（假綠）的結果竟然判「通過」⇒ 本測試沒有鑑別力"
fi
ok "⑥ 拿掉 fail-closed ⇒ rc=0 ＋ 重印修前 threshold 訊息（假綠復現），且 ① 的判準在此**變紅**"

echo "ALL PASS: layer3 benchmarks 判定機制契約（FU-20261016-28 缺陷②：0 筆被檢查仍報成功）"
