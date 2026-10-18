#!/usr/bin/env bash
# tests/scripts/test-db-credential-interpolation.sh
# check_db_credential_interpolation.sh 的契約測試（hermetic fixtures；2026-10-18）
#
# 【為什麼要有它】
#   這條不變式（部署 compose 的 DB 憑證必須插值）沒有既有守門 —— `scripts/secret-scan.sh` 掃的是
#   「像機密」的字串，字面 `atlas` 這種值**不會**命中（2026-10-18 實測：餵 POSTGRES_USER=atlas ⇒ rc=0）。
#   守門若只會印 PASS 就等於沒有 ⇒ 用 hermetic fixture 把「有牙齒」釘成可執行斷言。
#
# 【釘住的契約（每條下面都有對應 case）】
#   ① 乾淨 fixture（全插值 ＋ .env.example 空值 key）⇒ rc=0，且印出合規宣告
#   ② 字面 POSTGRES_USER ⇒ rc=1，且指名欄位與值
#   ③ 字面 POSTGRES_PASSWORD ⇒ rc=1
#   ④ DSN 的 user 字面 ⇒ rc=1
#   ⑤ DSN 的 password 字面 ⇒ rc=1
#   ⑥ .env.example 的 DB_USER 帶值 ⇒ rc=1
#   ⑦ 受測檔不存在 ⇒ rc=2（fail loud，不得當成「掃到 0 筆 ⇒ 通過」）
#   ⑧ 未知旗標 ⇒ rc=2（用法錯誤，不是靜默忽略）
#   ⑨ 突變自證（資料層）：把乾淨 fixture 的 ${DB_USER} 改回字面 atlas ⇒ ① 的斷言必須翻紅
#   ⑩ 突變自證（守門層）：把守門的 exit 1 中和 ⇒ ② 的斷言必須翻紅（證明本測試有鑑別力）
#   ⑪ 接線契約：Makefile ci-quick 仍呼叫守門；ci-gate 仍跑本測試
#   ⑫ 真 repo 現況為綠（守門對 repo 根 ⇒ rc=0）
#   ⑬ 本測試不得動到呼叫者 repo（HEAD／index／受測腳本 sha256 皆不變）
#
# 執行： bash tests/scripts/test-db-credential-interpolation.sh
set -uo pipefail

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/scripts/ci/check_db_credential_interpolation.sh"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"
CALLER_SHA="$(shasum -a 256 "$SRC" 2>/dev/null | awk '{print $1}')"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/dbcred-contract.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); echo "  ✅ $*"; }
bad() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

negproof_require_file "$SRC" "受測守門" || exit 1

GREEN_MARKER="compose DB credentials are interpolated"
no_false_pass() { ! grep -qF "$GREEN_MARKER" "$1"; }

run_guard() { local root="$1"; shift; bash "$SRC" --root "$root" "$@"; }

# ── fixture 產生器 ────────────────────────────────────────────────────────────
mk_clean() {
  local d="$TMP/$1"; mkdir -p "$d"
  cat > "$d/docker-compose.yml" <<'YML'
services:
  postgres:
    environment:
      - POSTGRES_USER=${DB_USER}
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=atlas
  atlas:
    environment:
      - DATABASE_URL=postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/atlas?sslmode=prefer
  exporter:
    environment:
      - DATA_SOURCE_NAME=postgresql://${DB_USER}:${DB_PASSWORD:-atlas}@postgres:5432/atlas?sslmode=disable
YML
  printf 'DB_USER=\nDB_PASSWORD=\n' > "$d/.env.example"
  echo "$d"
}

echo "→ check_db_credential_interpolation 契約測試（hermetic fixtures；2026-10-18）"

# ── ① 乾淨 fixture ⇒ rc=0 ＋ 合規宣告 ────────────────────────────────────────
DIR_CLEAN="$(mk_clean clean)"
LOG="$TMP/01-clean.log"
if negproof_expect 0 "① 全插值 ＋ 空值 key ⇒ rc=0（不是一律紅）" "$LOG" -- run_guard "$DIR_CLEAN"; then
  ok "① 乾淨 fixture 回綠"
else
  bad "① 乾淨 fixture 被誤判紅（過度修正）"; negproof_show_log "$LOG"
fi
negproof_expect_output "$GREEN_MARKER" "$LOG" "① 回綠必須是掃過且合規的那一版在說的" \
  || bad "① 綠燈缺少合規宣告"

# ── ② 字面 POSTGRES_USER ⇒ rc=1、指名欄位與值 ────────────────────────────────
DIR_U="$(mk_clean user-literal)"
sed -i.bak 's/^      - POSTGRES_USER=\${DB_USER}$/      - POSTGRES_USER=atlas/' "$DIR_U/docker-compose.yml"
if grep -qF -- "- POSTGRES_USER=atlas" "$DIR_U/docker-compose.yml"; then
  ok "② fixture 真的把 user 改成字面（探針非空）"
else
  bad "② 突變沒生效 ⇒ 後續斷言是空的"
fi
LOG="$TMP/02-user.log"
if negproof_expect 1 "② 字面 POSTGRES_USER ⇒ 必須紅（exit 1）" "$LOG" -- run_guard "$DIR_U"; then
  ok "② 守門對字面 user 有牙齒"
else
  bad "② 守門沒有紅 ⇒ 缺陷復活"; negproof_show_log "$LOG"
fi
negproof_expect_output "POSTGRES_USER 不是插值" "$LOG" "② 紅燈必須指名是哪個欄位" || bad "② 紅燈沒有指名欄位"
negproof_expect_output "值=atlas" "$LOG" "② 紅燈必須報出（非機密的）欄位值" || bad "② 紅燈沒有報值"
if no_false_pass "$LOG"; then ok "② 有違規時不再印合規宣告"; else bad "② 有違規仍印合規宣告（假綠）"; fi

# ── ③ 字面 POSTGRES_PASSWORD ⇒ rc=1 ──────────────────────────────────────────
DIR_P="$(mk_clean pass-literal)"
sed -i.bak 's/^      - POSTGRES_PASSWORD=\${DB_PASSWORD}$/      - POSTGRES_PASSWORD=hunter2/' "$DIR_P/docker-compose.yml"
LOG="$TMP/03-pass.log"
if negproof_expect 1 "③ 字面 POSTGRES_PASSWORD ⇒ rc=1" "$LOG" -- run_guard "$DIR_P"; then
  ok "③ 守門對字面 password 有牙齒"
else
  bad "③ 守門沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "POSTGRES_PASSWORD 不是插值" "$LOG" "③ 紅燈指名 password 欄位" || bad "③ 紅燈沒指名"
if grep -qF "hunter2" "$LOG"; then bad "③ 紅燈把密碼字面印出來了（不得外洩）"; else ok "③ 紅燈不印密碼字面（已遮蔽）"; fi

# ── ④ DSN 的 user 字面 ⇒ rc=1 ────────────────────────────────────────────────
DIR_DU="$(mk_clean dsn-user)"
sed -i.bak 's#postgres://\${DB_USER}:\${DB_PASSWORD}@#postgres://atlas:${DB_PASSWORD}@#' "$DIR_DU/docker-compose.yml"
LOG="$TMP/04-dsn-user.log"
if negproof_expect 1 "④ DSN 的 user 字面 ⇒ rc=1" "$LOG" -- run_guard "$DIR_DU"; then
  ok "④ 守門對 DSN user 有牙齒"
else
  bad "④ 守門沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "DSN 的 user 不是插值" "$LOG" "④ 紅燈指名 DSN user" || bad "④ 紅燈沒指名 DSN user"

# ── ⑤ DSN 的 password 字面 ⇒ rc=1 ────────────────────────────────────────────
DIR_DP="$(mk_clean dsn-pass)"
sed -i.bak 's#postgres://\${DB_USER}:\${DB_PASSWORD}@#postgres://${DB_USER}:s3cr3t@#' "$DIR_DP/docker-compose.yml"
LOG="$TMP/05-dsn-pass.log"
if negproof_expect 1 "⑤ DSN 的 password 字面 ⇒ rc=1" "$LOG" -- run_guard "$DIR_DP"; then
  ok "⑤ 守門對 DSN password 有牙齒"
else
  bad "⑤ 守門沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "DSN 的 password 不是插值" "$LOG" "⑤ 紅燈指名 DSN password" || bad "⑤ 紅燈沒指名 DSN password"
if grep -qF "s3cr3t" "$LOG"; then bad "⑤ 紅燈把 DSN 密碼字面印出來了"; else ok "⑤ 紅燈不印 DSN 密碼字面（已遮蔽）"; fi

# ── ⑥ .env.example 的 DB_USER 帶值 ⇒ rc=1 ────────────────────────────────────
DIR_E="$(mk_clean env-value)"
printf 'DB_USER=atlas\nDB_PASSWORD=\n' > "$DIR_E/.env.example"
LOG="$TMP/06-env.log"
if negproof_expect 1 "⑥ .env.example 的 DB_USER 帶值 ⇒ rc=1" "$LOG" -- run_guard "$DIR_E"; then
  ok "⑥ 守門對模板帶值有牙齒"
else
  bad "⑥ 守門沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "DB_USER 不是空值" "$LOG" "⑥ 紅燈指名 .env.example 的鍵" || bad "⑥ 紅燈沒指名 .env.example"

# ── ⑦ 受測檔不存在 ⇒ rc=2（fail loud，不是「掃到 0 筆 ⇒ 通過」）────────────────
DIR_MISS="$TMP/missing"; mkdir -p "$DIR_MISS"
LOG="$TMP/07-missing.log"
if negproof_expect 2 "⑦ 受測檔不存在 ⇒ rc=2（fail loud）" "$LOG" -- run_guard "$DIR_MISS"; then
  ok "⑦ 缺檔不會被當成通過"
else
  bad "⑦ 缺檔沒有 fail loud"; negproof_show_log "$LOG"
fi
if no_false_pass "$LOG"; then ok "⑦ 缺檔時不印合規宣告"; else bad "⑦ 缺檔竟印合規宣告（假綠）"; fi

# ── ⑧ 未知旗標 ⇒ rc=2 ────────────────────────────────────────────────────────
LOG="$TMP/08-flag.log"
if negproof_expect 2 "⑧ --bogus ⇒ rc=2（用法錯誤）" "$LOG" -- run_guard "$DIR_CLEAN" --bogus; then
  ok "⑧ 未知旗標被拒"
else
  bad "⑧ 未知旗標被靜默忽略"; negproof_show_log "$LOG"
fi
negproof_expect_output "unknown option: --bogus" "$LOG" "⑧ 必須指名旗標" || bad "⑧ 沒有指名旗標"

# ── ⑨ 突變自證（資料層）：${DB_USER} → atlas ⇒ ① 的斷言必須翻紅 ────────────────
DIR_MUT="$TMP/data-mutated"
cp -R "$DIR_CLEAN" "$DIR_MUT"
sed -i.bak 's/^      - POSTGRES_USER=\${DB_USER}$/      - POSTGRES_USER=atlas/' "$DIR_MUT/docker-compose.yml"
if cmp -s "$DIR_CLEAN/docker-compose.yml" "$DIR_MUT/docker-compose.yml"; then
  bad "⑨ 資料突變沒有生效（錨點找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/09-data-mutation.log"
  if negproof_expect 1 "⑨（自證）把插值改回字面 user ⇒ 守門必須紅" "$LOGM" -- run_guard "$DIR_MUT"; then
    ok "⑨ 同一輸入（改前 rc=0、改後 rc=1）⇒ 守門有鑑別力"
  else
    bad "⑨ 突變後守門仍不紅 ⇒ 守門沒有鑑別力"; negproof_show_log "$LOGM"
  fi
fi

# ── ⑩ 突變自證（守門層）：中和 exit 1 ⇒ ② 的斷言必須翻紅 ──────────────────────
MUT_GUARD="$TMP/neutered-guard.sh"
sed 's/^  exit 1$/  exit 0/' "$SRC" > "$MUT_GUARD"
if cmp -s "$SRC" "$MUT_GUARD"; then
  bad "⑩ 守門突變沒有生效（錨點 '^  exit 1$' 找不到）"
else
  LOGN="$TMP/10-guard-neutered.log"
  rc=0
  bash "$MUT_GUARD" --root "$DIR_U" > "$LOGN" 2>&1 || rc=$?
  if [ "$rc" -eq 0 ]; then
    ok "⑩ 中和 exit 1 後守門 rc=0 ⇒ ② 的 rc=1 斷言確實會抓到（本測試有鑑別力）"
  else
    bad "⑩ 中和 exit 1 後守門仍非 0（rc=$rc）⇒ ② 的斷言對『守門無牙』不敏感"
  fi
fi

# ── ⑪ 接線契約：ci-quick 呼叫守門；ci-gate 跑本測試 ──────────────────────────
if awk '/check_db_credential_interpolation\.sh/ { l=$0; sub(/^[[:space:]]*@?/, "", l); if (l !~ /^echo[[:space:]]/) print NR": "$0 }' "$REPO/Makefile" | grep -q 'scripts/ci/check_db_credential_interpolation.sh'; then
  ok "⑪ Makefile 的呼叫行含守門（ci-quick 會跑它）"
else
  bad "⑪ Makefile 找不到守門的呼叫行（接線斷了）"
fi
if grep -qF 'bash tests/scripts/test-db-credential-interpolation.sh' "$REPO/Makefile"; then
  ok "⑪ Makefile 的 ci-gate 會跑本測試"
else
  bad "⑪ Makefile 沒有跑本測試（契約沒被釘住）"
fi

# ── ⑫ 真 repo 現況為綠 ───────────────────────────────────────────────────────
LOG="$TMP/12-repo.log"
if negproof_expect 0 "⑫ 真 repo（--root ${REPO}）⇒ rc=0" "$LOG" -- bash "$SRC" --root "$REPO"; then
  ok "⑫ 現況合規（本 PR 上游化後的 compose/.env.example 通過守門）"
else
  bad "⑫ 真 repo 竟紅 ⇒ 上游化的內容沒過自己的守門"; negproof_show_log "$LOG"
fi

# ── ⑬ 本測試不得動到呼叫者 repo ───────────────────────────────────────────────
if [ "$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)" = "$CALLER_HEAD" ] \
   && [ "$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')" = "$CALLER_TRACKED" ]; then
  ok "⑬ 本測試沒有動到呼叫者 repo（HEAD 與 index 不變）"
else
  bad "⑬ 本測試動到了呼叫者的 repo"
fi
if [ "$(shasum -a 256 "$SRC" 2>/dev/null | awk '{print $1}')" = "$CALLER_SHA" ]; then
  ok "⑬ 受測守門沒有被本測試改寫（sha256 不變）"
else
  bad "⑬ 受測守門被本測試改寫了"
fi

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-db-credential-interpolation: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-db-credential-interpolation: $PASS passed（插值必綠／字面 user・password・DSN 兩段必紅／模板帶值必紅／缺檔 fail loud／未知旗標 rc=2／資料層＋守門層突變自證／接線契約／真 repo 綠／呼叫者不變）"
