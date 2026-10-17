#!/usr/bin/env bash
# tests/scripts/test-test-repo-root-guard.sh — check_test_repo_root 的契約測試（FU-20261016-20）
#
# 為什麼要有它
# ------------
# 這道守門禁的是「測試用固定相對層數定位 repo 根」（`"../.."`）。這種缺陷的失敗模式是
# **本機綠、CI 紅**：lane worktree 在 `<主 clone>/.worktrees/<lane>`，從 lane 根往上兩層
# 剛好是主 clone（有 configs/parameters.json）⇒ 錯誤答案在本機看起來是對的。守門自己
# 也一樣容易假綠——掃不到檔案、baseline 讀不到、pattern 打錯，都會讓它「印 ✅ 而其實沒檢查」。
# 因此用 hermetic fixture 釘住：
#
#   ① Tier A（`root := filepath.Join("..", "..")` 等五種形狀）⇒ rc=1 且輸出指名 path:line
#   ② 同一 fixture 把那一行改成走 helper ⇒ rc=0（證明 ① 有鑑別力，不是「一律紅」）
#   ③ 註解行提到同樣字串 ⇒ rc=0（註解可以**說明**為什麼不要這樣寫，不該被當違規）
#   ④ Tier B 在額度內 ⇒ rc=0
#   ⑤ Tier B 多一行（額度不變）⇒ rc=1（新測試不得再引入固定層數）
#   ⑥ 新增檔不在 baseline（額度 0）⇒ rc=1
#   ⑦ Tier B 用量下降 ⇒ rc=0 但必須印 stale 提示（不得靜默）
#   ⑧ Tier A 那行同時也符合 Tier B ⇒ 仍 rc=1（Tier A 不得被額度吞掉）
#   ⑨ 空集合（掃不到任何 *_test.go）⇒ rc=2（掃不到 ≠ 通過）
#   ⑩ --json 的 tier_a／tier_b_growth 逐字可解析
#   ⑪ mutation 自證：把 Tier A2 的 `\.\./\.\.` 改成 `ZZZ` ⇒ ① 的紅燈消失（證明紅燈來自偵測器）；
#      把 Tier B 判定改成 false ⇒ ⑤ 的紅燈消失
#   ⑫ 本測試不動到呼叫者 repo（HEAD／index 不變、baseline 檔 sha256 不變）
#
# 執行： bash tests/scripts/test-test-repo-root-guard.sh
set -uo pipefail

# fixture 自建臨時 git repo；被 git hook 呼叫時環境的 GIT_* 會讓 `git -C` 指錯 repo
# ⇒ 一律 unset（同 test-check-markdown-links.sh 的 2026-09-29 事故）。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_PY="$REPO/scripts/ci/check_test_repo_root.py"
SRC_SH="$REPO/scripts/ci/check_test_repo_root.sh"
BASELINE_REAL="$REPO/scripts/ci/test-repo-root-baseline.json"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"
CALLER_BASELINE_SHA="$(shasum -a 256 "$BASELINE_REAL" 2>/dev/null | awk '{print $1}')"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/testreporoot-contract.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

negproof_require_file "$SRC_PY" "受測檢查器" || exit 1
negproof_require_file "$SRC_SH" "受測入口" || exit 1
negproof_require_file "$BASELINE_REAL" "baseline" || exit 1

# ── fixture ──────────────────────────────────────────────────────────────────
# 每個 case 自建一個**真的 git repo**（檢查器用 `git ls-files` 蒐集，無 .git 會走 walk，
# 兩條路都要被覆蓋 ⇒ 走 git 這條）。
new_fixture() {
  local name="$1"
  local dir="$TMP/$name"
  mkdir -p "$dir/scripts/ci" "$dir/internal/x"
  git -C "$dir" init -q
  git -C "$dir" config user.email fixture@example.test
  git -C "$dir" config user.name fixture
  cp "$SRC_PY" "$dir/scripts/ci/check_test_repo_root.py"
  # 預設 baseline：空額度（正式額度由各 case 自己寫）
  printf '{"files":{}}\n' > "$dir/scripts/ci/test-repo-root-baseline.json"
  ( cd "$dir" && git add -A && git -c user.email=f@e.test -c user.name=f commit -qm init )
  echo "$dir"
}

# add_file <fixture> <relpath> <body>：寫檔後**納入 index**（檢查器只看已追蹤檔）
add_file() {
  local dir="$1"
  local rel="$2"
  local body="$3"
  mkdir -p "$dir/$(dirname "$rel")"
  printf '%s\n' "$body" > "$dir/$rel"
  git -C "$dir" add -A
}

set_baseline() {
  local dir="$1"
  local json="$2"
  printf '%s\n' "$json" > "$dir/scripts/ci/test-repo-root-baseline.json"
}

run_guard() {
  local dir="$1"; shift
  python3 "$dir/scripts/ci/check_test_repo_root.py" --root "$dir" "$@"
}

TIER_A_BODY='package x

import (
	"path/filepath"
	"testing"
)

func TestRootByFixedDepth(t *testing.T) {
	root := filepath.Join("..", "..")
	_ = filepath.Join(root, "configs", "parameters.json")
}'

HELPER_BODY='package x

import (
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/reporoot"
)

func TestRootByWalkUp(t *testing.T) {
	root, err := reporoot.FromCwd()
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	_ = filepath.Join(root, "configs", "parameters.json")
}'

TIER_B_BODY='package x

import (
	"path/filepath"
	"testing"
)

func TestReadsConfigByFixedDepth(t *testing.T) {
	_ = filepath.Join("..", "..", "configs", "parameters.json")
}'

echo "🔎 check_test_repo_root 契約測試（FU-20261016-20）"
echo ""

# ── ① Tier A ⇒ rc=1 且指名 path:line ────────────────────────────────────────
F="$(new_fixture tier-a)"
add_file "$F" internal/x/root_test.go "$TIER_A_BODY"
OUT="$(run_guard "$F" 2>&1)"; RC=$?
if [ "$RC" -eq 1 ] && printf '%s' "$OUT" | grep -q 'internal/x/root_test.go:9'; then
  ok "① Tier A（root := Join(\"..\",\"..\")）⇒ rc=1 且指名 internal/x/root_test.go:9"
else
  fail "① Tier A 必須 rc=1 且指名 path:line（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ② 改成走 helper ⇒ rc=0（鑑別力）────────────────────────────────────────
F2="$(new_fixture tier-a-fixed)"
add_file "$F2" internal/x/root_test.go "$HELPER_BODY"
OUT="$(run_guard "$F2" 2>&1)"; RC=$?
if [ "$RC" -eq 0 ]; then
  ok "② 同一位置改用 reporoot.FromCwd() ⇒ rc=0（① 不是「一律紅」）"
else
  fail "② helper 版本必須 rc=0（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ③ 註解行提到同樣字串 ⇒ rc=0 ─────────────────────────────────────────────
F3="$(new_fixture comment-only)"
add_file "$F3" internal/x/doc_test.go 'package x

// 反例說明：不要用 root := filepath.Join("..", "..") 或 os.Chdir("../..")。
// _ = filepath.Join("..", "..", "configs", "parameters.json")
func TestNothing(t *testing.T) {}'
OUT="$(run_guard "$F3" 2>&1)"; RC=$?
if [ "$RC" -eq 0 ]; then
  ok "③ 純註解行不算違規（註解要能說明反例）"
else
  fail "③ 註解行不得判違規（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ④ Tier B 在額度內 ⇒ rc=0 ────────────────────────────────────────────────
F4="$(new_fixture tier-b-within)"
add_file "$F4" internal/x/config_test.go "$TIER_B_BODY"
set_baseline "$F4" '{"files":{"internal/x/config_test.go":1}}'
git -C "$F4" add -A
OUT="$(run_guard "$F4" 2>&1)"; RC=$?
if [ "$RC" -eq 0 ]; then
  ok "④ Tier B 用量 = 額度（1/1）⇒ rc=0"
else
  fail "④ Tier B 在額度內必須 rc=0（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑤ Tier B 多一行 ⇒ rc=1 ─────────────────────────────────────────────────
add_file "$F4" internal/x/config_test.go "$TIER_B_BODY

func TestSecondFixedDepth(t *testing.T) {
	_ = filepath.Join(\"..\", \"..\", \"configs\", \"agents.json\")
}"
OUT="$(run_guard "$F4" 2>&1)"; RC=$?
if [ "$RC" -eq 1 ] && printf '%s' "$OUT" | grep -q 'internal/x/config_test.go: 2 行，額度 1'; then
  ok "⑤ Tier B 成長（2 > 額度 1）⇒ rc=1 且講出檔案與行數"
else
  fail "⑤ Tier B 超額必須 rc=1 且指名檔案／行數（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑥ 新檔不在 baseline ⇒ rc=1 ─────────────────────────────────────────────
F5="$(new_fixture tier-b-new-file)"
add_file "$F5" internal/x/brand_new_test.go "$TIER_B_BODY"
OUT="$(run_guard "$F5" 2>&1)"; RC=$?
if [ "$RC" -eq 1 ] && printf '%s' "$OUT" | grep -q 'internal/x/brand_new_test.go: 1 行，額度 0'; then
  ok "⑥ 新檔未登錄額度 ⇒ rc=1（新測試不得引入固定層數）"
else
  fail "⑥ 新檔必須 rc=1（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑦ 用量下降 ⇒ rc=0 且印 stale ───────────────────────────────────────────
F6="$(new_fixture tier-b-stale)"
add_file "$F6" internal/x/config_test.go "$TIER_B_BODY"
set_baseline "$F6" '{"files":{"internal/x/config_test.go":3}}'
git -C "$F6" add -A
OUT="$(run_guard "$F6" 2>&1)"; RC=$?
if [ "$RC" -eq 0 ] && printf '%s' "$OUT" | grep -q 'baseline 過期'; then
  ok "⑦ 用量 < 額度 ⇒ rc=0，且印 stale 提示（不靜默）"
else
  fail "⑦ 用量下降必須 rc=0 且提示 stale（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑧ Tier A 那行也符合 Tier B ⇒ 仍 rc=1（不得被額度吞掉）─────────────────
F7="$(new_fixture tier-a-overlap)"
add_file "$F7" internal/x/root_test.go "$TIER_A_BODY"
set_baseline "$F7" '{"files":{"internal/x/root_test.go":1}}'
git -C "$F7" add -A
OUT="$(run_guard "$F7" 2>&1)"; RC=$?
if [ "$RC" -eq 1 ] && printf '%s' "$OUT" | grep -q 'Tier A 命中'; then
  ok "⑧ Tier A 命中時，額度不能把它吞掉（仍 rc=1）"
else
  fail "⑧ Tier A 不得被 Tier B 額度吸收（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑨ 掃不到任何 *_test.go ⇒ rc=2（掃不到 ≠ 通過）─────────────────────────
F8="$(new_fixture empty)"
mkdir -p "$F8/internal/empty"
OUT="$(run_guard "$F8" 2>&1)"; RC=$?
if [ "$RC" -eq 2 ]; then
  ok "⑨ 空集合（0 個 *_test.go）⇒ rc=2，不得回綠"
else
  fail "⑨ 空集合必須 rc=2（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑩ --json 可解析 ────────────────────────────────────────────────────────
OUT="$(run_guard "$F" --json 2>&1)"; RC=$?
if [ "$RC" -eq 1 ] && printf '%s' "$OUT" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["tier_a"], d' 2>/dev/null; then
  ok "⑩ --json 輸出可解析且 tier_a 非空"
else
  fail "⑩ --json 必須輸出可解析的 JSON（rc=${RC}）"; printf '%s\n' "$OUT" | sed 's/^/      /'
fi

# ── ⑪ mutation 自證 ────────────────────────────────────────────────────────
MUT="$TMP/mutant-a.py"
python3 - "$SRC_PY" "$MUT" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()
# 打 Tier A 的**判準行**（不是註解）：CLIMB_LITERAL／JOIN_TWO_DOTS 是 Tier A 六條規則
# 共用的兩個 pattern 原語 ⇒ 一起換成永不匹配的字串。
lines = text.splitlines()
mutated = 0
for i, line in enumerate(lines):
    if line.startswith("CLIMB_LITERAL = "):
        lines[i] = 'CLIMB_LITERAL = r"ZZZ_NEVER_MATCHES"'
        mutated += 1
    elif line.startswith("JOIN_TWO_DOTS = "):
        lines[i] = 'JOIN_TWO_DOTS = r"ZZZ_NEVER_MATCHES"'
        mutated += 1
if mutated != 2:
    raise SystemExit("pattern 原語定義行找不到（mutated=%d，期望 2）" % mutated)
open(dst, "w", encoding="utf-8").write("\n".join(lines) + "\n")
PY
python3 "$MUT" --root "$F" >/dev/null 2>&1; MUT_RC=$?
if [ "$MUT_RC" -eq 0 ]; then
  ok "⑪a mutation（CLIMB_LITERAL 改成永不匹配）⇒ ① 的紅燈消失（證明紅燈來自偵測器）"
else
  fail "⑪a mutation 後仍紅（rc=${MUT_RC}）⇒ 紅燈不是來自被突變的那段，斷言無鑑別力"
fi

MUT2="$TMP/mutant-b.py"
python3 - "$SRC_PY" "$MUT2" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
lines = open(src, encoding="utf-8").read().splitlines()
for i, line in enumerate(lines):
    if line.startswith("TIER_B = re.compile("):
        lines[i] = "TIER_B = re.compile(r'ZZZ_NEVER_MATCHES')"
        break
else:
    raise SystemExit("TIER_B definition not found")
open(dst, "w", encoding="utf-8").write("\n".join(lines) + "\n")
PY
python3 "$MUT2" --root "$F4" >/dev/null 2>&1; MUT2_RC=$?
if [ "$MUT2_RC" -eq 0 ]; then
  ok "⑪b mutation（Tier B 改成永不匹配）⇒ ⑤ 的紅燈消失（額度判定真的有牙齒）"
else
  fail "⑪b mutation 後仍紅（rc=${MUT2_RC}）"
fi

# ── ⑫ 不動到呼叫者 repo ────────────────────────────────────────────────────
AFTER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
AFTER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"
AFTER_BASELINE_SHA="$(shasum -a 256 "$BASELINE_REAL" 2>/dev/null | awk '{print $1}')"
if [ "$AFTER_HEAD" = "$CALLER_HEAD" ] && [ "$AFTER_TRACKED" = "$CALLER_TRACKED" ] && [ "$AFTER_BASELINE_SHA" = "$CALLER_BASELINE_SHA" ]; then
  ok "⑫ 呼叫者 repo 未被更動（HEAD／index 檔數／baseline sha256 不變）"
else
  fail "⑫ 本測試動到了呼叫者 repo（HEAD ${CALLER_HEAD}→${AFTER_HEAD}、tracked ${CALLER_TRACKED}→${AFTER_TRACKED}、baseline ${CALLER_BASELINE_SHA}→${AFTER_BASELINE_SHA}）"
fi

echo ""
echo "  通過 ${PASS}、失敗 $FAIL"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
