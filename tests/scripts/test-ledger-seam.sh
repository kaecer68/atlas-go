#!/usr/bin/env bash
# tests/scripts/test-ledger-seam.sh — ledger store seam 守門的 hermetic 回歸測試（T1/S1）
#
# 為什麼需要：這個檢查的價值全在「能抓到新增的直接／間接 JSONL store 站點」＋「不對合法寫法誤報」。
# 只跑 repo 掃描證明不了前者（main 上沒有故意違規），因此本測試用自帶 fixture（不碰 git、
# 不碰 production、不需要 Go toolchain）證明下列行為：
#
#   direct（ledger.NewStore）
#     1. fixture（allowlist 完全對得上）→ PASS，且輸出「掃到 2 個」的計數——證明量具真的看到東西
#     2. 新增一個裸呼叫 → FAIL（exit 1），訊息含 檔案:行、機制與應改用的 factory
#     3. 同檔新增「與既有站點一字不差」的裸呼叫（數量超出）→ FAIL（數量也要對）
#     4. allowlist 有、掃不到（站點已遷移）→ FAIL（stale）
#     5. allowlist reason 留 TODO → FAIL（理由必填）
#     6. 只出現在註解／字串／_test.go 的 ledger.NewStore 不算違規 → PASS 且「掃到 0 個」
#     7. import alias（ledgerpkg "…/internal/ledger"）一樣被抓
#
#   indirect（未注入就落到 JSONL 的建構子，例：service.NewLiveService）
#     8. 未登錄的間接站點 → FAIL（exit 1），訊息標明「間接」＋機制＋注入修法
#     9. 登錄後 → PASS（同一份 fixture 只差 allowlist 一筆）
#
#   通用
#    10. 執行錯誤 → exit 2（掃描根不存在／allowlist 不存在／掃不到任何 Go 檔／allowlist 壞掉／
#        allowlist 列了未知 target）
#    11. 既有 repo 掃描 → PASS（allowlist 生效，歷史站點不讓檢查紅燈）
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECKER="$ROOT/scripts/ci/check_ledger_seam.py"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/ledger-seam-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {  # file pattern
  grep -Fq -- "$2" "$1" || fail "$1 應該包含: $2"
}

assert_not_contains() {  # file pattern
  if grep -Fq -- "$2" "$1"; then
    fail "$1 不應該包含: $2"
  fi
}

command -v python3 >/dev/null 2>&1 || fail "需要 python3"

# --- fixture 產生器 ---------------------------------------------------------
# make_fixture <dir>：cmd/app/main.go（裸呼叫 + 註解提及 + 字串提及）、
# internal/seam/x.go（alias 裸呼叫）、internal/indirect/y.go（間接建構子）、
# internal/seam/x_test.go（測試檔裸呼叫）、allowlist（v2）。
make_fixture() {
  local dir="$1"
  mkdir -p "$dir/cmd/app" "$dir/internal/seam" "$dir/internal/indirect"

  cat >"$dir/internal/seam/x.go" <<'EOF'
package seam

import ledgerpkg "github.com/kaecer68/atlas-go/internal/ledger"

// Load 回傳一個 outcome store。
func Load(dir string) ledgerpkg.OutcomeStore {
	return ledgerpkg.NewStore(dir)
}
EOF

  cat >"$dir/internal/seam/x_test.go" <<'EOF'
package seam

import "github.com/kaecer68/atlas-go/internal/ledger"

func TestLoad(t *testing.T) {
	_ = ledger.NewStore(t.TempDir())
}
EOF

  cat >"$dir/internal/indirect/y.go" <<'EOF'
package indirect

import "github.com/kaecer68/atlas-go/internal/monitoring/service"

// Build 建構 LiveService —— 未注入 History/TradeStore 時內部會落到 JSONL。
func Build(workDir, ledgerDir string) *service.LiveService {
	return service.NewLiveService(workDir, ledgerDir)
}
EOF

  cat >"$dir/cmd/app/main.go" <<'GOEOF'
package main

import "github.com/kaecer68/atlas-go/internal/ledger"

const doc = "ledger.NewStore(dir) 只出現在字串裡"

// 舊寫法：store := ledger.NewStore(dir) 只出現在註解裡
func run(dir string) {
	store := ledger.NewStore(dir)
	_ = store
	_ = doc
}
GOEOF

  cat >"$dir/allowlist.json" <<'EOF'
{
  "version": 2,
  "targets": {
    "ledger.NewStore": [
      {
        "file": "cmd/app/main.go",
        "line": 1,
        "snippet": "store := ledger.NewStore(dir)",
        "reason": "fixture：刻意保留的 JSONL 站點"
      },
      {
        "file": "internal/seam/x.go",
        "line": 1,
        "snippet": "return ledgerpkg.NewStore(dir)",
        "reason": "fixture：import alias 站點（證明 alias 也認得）"
      }
    ],
    "service.NewLiveService": [
      {
        "file": "internal/indirect/y.go",
        "line": 1,
        "snippet": "return service.NewLiveService(workDir, ledgerDir)",
        "reason": "fixture：間接建構子（未注入 ⇒ 落到 JSONL），已登錄"
      }
    ]
  }
}
EOF
}

# fixture 的 allowlist 改寫器：assert_json <file> <python-expr 用 data 這個變數>
rewrite_allowlist() {  # <file> <python snippet>
  python3 - "$1" "$2" <<'PY'
import json, sys
path, code = sys.argv[1], sys.argv[2]
data = json.load(open(path, encoding="utf-8"))
exec(code, {"data": data})  # noqa: S102 — 測試自身可控的 snippet
json.dump(data, open(path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
}

echo "== case 1: fixture（allowlist 對得上）應 PASS 且掃到 2 個直接站點"
FIX1="$TMP/fix1"
make_fixture "$FIX1"
set +e
python3 "$CHECKER" --root "$FIX1" --allowlist "$FIX1/allowlist.json" >"$TMP/pass.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/pass.out"; fail "fixture 應 exit 0，實際 $rc"; }
assert_contains "$TMP/pass.out" "直接站點 2 個、間接站點 1 個全部已登錄"
assert_contains "$TMP/pass.out" "ledger seam OK"
assert_not_contains "$TMP/pass.out" "::error"

echo "== case 2: 新增一個裸呼叫 → FAIL（exit 1，訊息含檔案:行 與 factory 建議）"
FIX2="$TMP/fix2"
make_fixture "$FIX2"
cat >"$FIX2/cmd/app/extra.go" <<'EOF'
package main

import "github.com/kaecer68/atlas-go/internal/ledger"

func extra(dir string) {
	_ = ledger.NewStore(dir)
}
EOF
set +e
python3 "$CHECKER" --root "$FIX2" --allowlist "$FIX2/allowlist.json" >"$TMP/new.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/new.out"; fail "新增裸呼叫應 exit 1，實際 $rc"; }
assert_contains "$TMP/new.out" "cmd/app/extra.go"
assert_contains "$TMP/new.out" "新增的直接 JSONL store 站點"
assert_contains "$TMP/new.out" "請改用 backend-aware factory"
assert_contains "$TMP/new.out" "ledger.NewFullStore(cfg)"

echo "== case 3: 同檔新增一字不差的裸呼叫（數量超出）→ FAIL"
FIX3="$TMP/fix3"
make_fixture "$FIX3"
cat >>"$FIX3/cmd/app/main.go" <<'EOF'

func run2(dir string) {
	store := ledger.NewStore(dir)
	_ = store
}
EOF
set +e
python3 "$CHECKER" --root "$FIX3" --allowlist "$FIX3/allowlist.json" >"$TMP/dup.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/dup.out"; fail "重複裸呼叫應 exit 1，實際 $rc"; }
assert_contains "$TMP/dup.out" "數量超出 allowlist"

echo "== case 4: allowlist 有、掃不到（站點已遷移）→ FAIL（stale）"
FIX4="$TMP/fix4"
make_fixture "$FIX4"
rm "$FIX4/internal/seam/x.go"
set +e
python3 "$CHECKER" --root "$FIX4" --allowlist "$FIX4/allowlist.json" >"$TMP/stale.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/stale.out"; fail "stale 條目應 exit 1，實際 $rc"; }
assert_contains "$TMP/stale.out" "已無對應站點"

echo "== case 5: allowlist reason 留 TODO → FAIL（理由必填）"
FIX5="$TMP/fix5"
make_fixture "$FIX5"
rewrite_allowlist "$FIX5/allowlist.json" 'data["targets"]["ledger.NewStore"][0]["reason"] = "TODO: 未填理由"'
set +e
python3 "$CHECKER" --root "$FIX5" --allowlist "$FIX5/allowlist.json" >"$TMP/todo.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/todo.out"; fail "TODO reason 應 exit 1，實際 $rc"; }
assert_contains "$TMP/todo.out" "allowlist 每筆必須填寫 reason"

echo "== case 6: 只有註解／字串／_test.go 提及 → PASS 且掃到 0 個"
FIX6="$TMP/fix6"
make_fixture "$FIX6"
rm "$FIX6/cmd/app/main.go" "$FIX6/internal/seam/x.go" "$FIX6/internal/indirect/y.go"
cat >"$FIX6/cmd/app/main.go" <<'GOEOF'
package main

const doc = "ledger.NewStore(dir) 只在字串裡"

// 註解提及 ledger.NewStore(dir) 也不算違規
func run() {
	_ = doc
}
GOEOF
printf '{"version": 2, "targets": {}}\n' >"$FIX6/allowlist.json"
set +e
python3 "$CHECKER" --root "$FIX6" --allowlist "$FIX6/allowlist.json" >"$TMP/clean.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/clean.out"; fail "只有註解/字串/_test.go 應 exit 0，實際 $rc"; }
assert_contains "$TMP/clean.out" "直接站點 0 個、間接站點 0 個全部已登錄"
assert_not_contains "$TMP/clean.out" "::error"

echo "== case 7: import alias 也認得（把 alias 條目刪掉就必須紅）"
FIX7="$TMP/fix7"
make_fixture "$FIX7"
rewrite_allowlist "$FIX7/allowlist.json" 'data["targets"]["ledger.NewStore"] = [e for e in data["targets"]["ledger.NewStore"] if e["file"] != "internal/seam/x.go"]'
set +e
python3 "$CHECKER" --root "$FIX7" --allowlist "$FIX7/allowlist.json" >"$TMP/alias.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/alias.out"; fail "alias 站點未被認得（應 exit 1），實際 $rc"; }
assert_contains "$TMP/alias.out" "internal/seam/x.go"

echo "== case 8: 未登錄的間接站點 → FAIL（訊息標明間接＋機制＋注入修法）"
FIX8="$TMP/fix8"
make_fixture "$FIX8"
rewrite_allowlist "$FIX8/allowlist.json" 'data["targets"].pop("service.NewLiveService", None)'
set +e
python3 "$CHECKER" --root "$FIX8" --allowlist "$FIX8/allowlist.json" >"$TMP/indirect.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/indirect.out"; fail "未登錄的間接站點應 exit 1，實際 $rc"; }
assert_contains "$TMP/indirect.out" "internal/indirect/y.go"
assert_contains "$TMP/indirect.out" "新增的間接 JSONL store 站點（service.NewLiveService）"
assert_contains "$TMP/indirect.out" "WithHistory(service.NewSessionHistoryProvider(cfg))"

echo "== case 9: 同一份 fixture 只補上 allowlist 一筆 → PASS（紅→綠的最小差異）"
FIX9="$TMP/fix9"
cp -r "$FIX8" "$FIX9"
rewrite_allowlist "$FIX9/allowlist.json" 'data["targets"]["service.NewLiveService"] = [{"file": "internal/indirect/y.go", "line": 7, "snippet": "return service.NewLiveService(workDir, ledgerDir)", "reason": "fixture：已登錄"}]'
set +e
python3 "$CHECKER" --root "$FIX9" --allowlist "$FIX9/allowlist.json" >"$TMP/indirect-green.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/indirect-green.out"; fail "登錄後應 exit 0，實際 $rc"; }
assert_contains "$TMP/indirect-green.out" "間接站點 1 個全部已登錄"

echo "== case 10: 執行錯誤一律 exit 2（且不得被誤判成綠燈/紅燈）"
set +e
python3 "$CHECKER" --root "$TMP/does-not-exist" >"$TMP/e1.out" 2>&1; rc1=$?
python3 "$CHECKER" --root "$FIX1" --allowlist "$TMP/does-not-exist.json" >"$TMP/e2.out" 2>&1; rc2=$?
mkdir -p "$TMP/empty/cmd"
python3 "$CHECKER" --root "$TMP/empty" --allowlist "$FIX1/allowlist.json" >"$TMP/e3.out" 2>&1; rc3=$?
printf 'not json' >"$TMP/broken.json"
python3 "$CHECKER" --root "$FIX1" --allowlist "$TMP/broken.json" >"$TMP/e4.out" 2>&1; rc4=$?
printf '{"version": 2, "targets": {"ledger.NotReal": []}}\n' >"$TMP/unknown-target.json"
python3 "$CHECKER" --root "$FIX1" --allowlist "$TMP/unknown-target.json" >"$TMP/e5.out" 2>&1; rc5=$?
set -e
[ "$rc1" -eq 2 ] || { cat "$TMP/e1.out"; fail "掃描根不存在應 exit 2，實際 $rc1"; }
[ "$rc2" -eq 2 ] || { cat "$TMP/e2.out"; fail "allowlist 不存在應 exit 2，實際 $rc2"; }
[ "$rc3" -eq 2 ] || { cat "$TMP/e3.out"; fail "掃不到任何 Go 檔應 exit 2，實際 $rc3"; }
[ "$rc4" -eq 2 ] || { cat "$TMP/e4.out"; fail "allowlist 壞掉應 exit 2，實際 $rc4"; }
[ "$rc5" -eq 2 ] || { cat "$TMP/e5.out"; fail "allowlist 未知 target 應 exit 2，實際 $rc5"; }
assert_contains "$TMP/e5.out" "unknown target"

echo "== case 11: 既有 repo 掃描 → PASS（allowlist 生效）"
set +e
python3 "$CHECKER" --root "$ROOT" >"$TMP/repo.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/repo.out"; fail "既有 repo 掃描應 exit 0（allowlist 失效？），實際 $rc"; }
assert_contains "$TMP/repo.out" "ledger seam OK"

echo "✅ ledger seam 契約測試全數通過"
