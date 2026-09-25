#!/usr/bin/env bash
# test-secret-scan.sh — secret-scan 自我測試（atlas-go 版；副本，SSOT = a2a-dev tests/secret-scan-selftest.sh）
#
# 兩件事必須同時成立，否則這個護欄會「看起來有、實際沒用」：
#   A. **負向**：真的憑證形狀（telegram bot token / sk- / ghp_ / AKIA / PRIVATE KEY）必被擋 exit=1。
#   B. **不誤擋**：文件（*.md）、範例（*.example）、行內 `secret-scan-allow`、allowlist 不得讓 CI 失敗。
#   另加 D. **通用憑證形狀**（DSN 內嵌密碼／設定檔 secret 字面值／env 預設值）＋
#      「可部署設定檔（*.yml/*.yaml）不降級為 warn-only」的路徑政策（2026-09-25 任務 Q）。
#   另加 C. **輸出遮蔽**：命中值只留前 4 後 4，不得把完整憑證印進 log（log 本身也是外洩面）。
#
# 做法：在 tempdir 造合成檔案，用 `--root "$TMP"` 掃（不動真 repo；tempdir 無 git → scanner 走 find）。
# ⚠️ 本檔內所有假憑證都用「相鄰字串相接」組出，讓測試檔**本身**不含可被自己命中的字面值。
# ⚠️ 掃描器內部呼叫 git 前會 sanitize GIT_*（SOP ★37：git hook 注入 GIT_DIR 會讓 fixture 假 PASS）。
#
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"   # tests/scripts/ → repo root
SCAN="$ROOT/scripts/secret-scan.sh"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

TG_TOKEN="1234567890:""FAKEfakeFAKEfakeFAKEfakeFAKEfake12345"
SK_KEY="sk-""fakefakefakefakefake1234"
GH_TOKEN="ghp_""AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
AWS_KEY="AKIA""ABCDEFGHIJKLMNOP"
PK_HEAD="-----BEGIN OPENSSH ""PRIVATE KEY-----"

# scan_run <strict 0|1> → 把輸出放 "$TMP/out.txt"，回傳 exit code
scan_run() {
  local strict=""
  [ "$1" = "1" ] && strict="--strict"
  bash "$SCAN" --root "$TMP" --allowlist "$TMP/allowlist.txt" $strict >"$TMP/out.txt" 2>&1
}

# must_block <說明> <檔名> <內容> <樣式名>
must_block() {
  local desc="$1" fn="$2" body="$3" pat="$4"
  rm -rf "$TMP/src" "$TMP/docs"; mkdir -p "$(dirname "$TMP/$fn")"
  printf '%s\n' "$body" > "$TMP/$fn"
  if scan_run 0; then
    bad "${desc}（期望 exit=1，得到 0）"; sed 's/^/     /' "$TMP/out.txt" | head -3; return
  fi
  if grep -q "\[$pat\]" "$TMP/out.txt"; then ok "${desc}（exit=1，樣式 [$pat]）"
  else bad "${desc}（有擋但沒報 [$pat]）"; sed 's/^/     /' "$TMP/out.txt" | head -5; fi
}

# must_pass <說明> <檔名> <內容> [strict]
must_pass() {
  local desc="$1" fn="$2" body="$3" strict="${4:-0}"
  rm -rf "$TMP/src" "$TMP/docs"; mkdir -p "$(dirname "$TMP/$fn")"
  printf '%s\n' "$body" > "$TMP/$fn"
  if scan_run "$strict"; then ok "$desc"
  else bad "${desc}（期望 exit=0，卻被擋）"; sed 's/^/     /' "$TMP/out.txt" | head -5; fi
}

echo "── secret-scan 自我測試（atlas-go）──"

# ── A. 負向：五種憑證形狀都必須被擋 ──────────────────────────────
must_block "A1 telegram bot token 被擋"        "src/a1.py"  "BOT = '$TG_TOKEN'"        "telegram_bot_token"
must_block "A2 sk- 金鑰被擋"                    "src/a2.py"  "OPENAI_API_KEY='$SK_KEY'" "openai_sk_key"
must_block "A3 github token 被擋"               "src/a3.sh"  "TOKEN=$GH_TOKEN"          "github_token"
must_block "A4 AWS access key id 被擋"          "src/a4.json" '{"k":"'"$AWS_KEY"'"}'     "aws_access_key_id"
must_block "A5 PRIVATE KEY block 被擋"          "src/a5.pem" "$PK_HEAD"                  "private_key_block"

# ── B. 不誤擋 ────────────────────────────────────────────────────
must_pass  "B1 文件（*.md）內的 token 預設不擋 CI（warn-only）" "docs/note.md" "範例: $TG_TOKEN"
# strict 模式必須把文件命中算失敗（否則文件類就永久無護欄）
rm -rf "$TMP/src" "$TMP/docs"; mkdir -p "$TMP/docs"; printf '%s\n' "範例: $TG_TOKEN" > "$TMP/docs/note.md"
if scan_run 1; then bad "B2 --strict 時文件命中仍 ASCII 通過（應失敗）"
else ok "B2 --strict 時文件命中會失敗"; fi
grep -q "文件/範例類命中" "$TMP/out.txt" && ok "B3 warn-only 命中會被明確標示" || bad "B3 未標示 warn-only 類別"

# 範例檔（.example）同樣 warn-only
must_pass  "B4 *.example 內的 token 預設不擋" "src/x.env.example" "TELEGRAM_BOT_TOKEN=$TG_TOKEN"

# 行內豁免
must_pass  "B5 行內 secret-scan-allow 可豁免" "src/b5.py" "BOT = '$TG_TOKEN'  # secret-scan-allow: 測試 fixture"

# allowlist 檔（glob + 樣式名 + 理由）
rm -rf "$TMP/src"; mkdir -p "$TMP/src"
printf '%s\n' "TOKEN=$TG_TOKEN" > "$TMP/src/b6.sh"
printf '%s\n' "src/b6.sh telegram_bot_token   # 合成測試資料（無效值）" > "$TMP/allowlist.txt"
must_pass  "B6 allowlist（glob+樣式名）可豁免" "src/b6.sh" "TOKEN=$TG_TOKEN"
# allowlist 只豁免列出的樣式名 → 換一種樣式不可豁免
printf '%s\n' "src/b6.sh openai_sk_key   # 只豁免 sk-,不豁免 telegram" > "$TMP/allowlist.txt"
if scan_run 0; then bad "B7 allowlist 僅針對指定樣式（telegram 仍應被擋）"
else ok "B7 allowlist 依樣式名精準豁免（未列樣式仍擋）"; fi
rm -f "$TMP/allowlist.txt"

# 乾淨檔案
must_pass  "B8 乾淨檔案通過（exit=0）" "src/b8.py" "print('hello, no secrets here')"

# ── D. 通用憑證形狀 + 可部署設定檔的路徑政策（2026-09-25 任務 Q 新增）───────
# 為什麼要這組：舊樣式表只有「廠商憑證形狀」（telegram/sk-/ghp_/AKIA…），抓不到**自架服務的通用憑證**
# （DB 連線字串內嵌密碼、設定檔內 password/secret 字面值）——那正是
# docs/operations/docker-compose.prod.yml 當時外洩的形狀；而且 *.yml 在 docs/ 底下當時也算 warn-only。
# 這組同時釘住「**不誤擋**」：插值、範例設定檔、合成測試 DSN、程式碼取值都不得讓 CI 紅。
# 合成假值刻意**不含** test/fake/sample 等字（掃描器會把含這些字的命中視為合成值而略過 ——
# 這是刻意的降噪設計，所以測試值要用「像真憑證的形狀」才能證明護欄有牙齒）。
FAKE_PW="Zq7""Xk92LmQ7pR4t"
FAKE_DSN="postgres://atlas:${FAKE_PW}@host.docker.internal:55432/atlas?sslmode=disable"

must_block "D1 .yml 內 DSN 嵌明文密碼被擋"      "docs/ops/compose.prod.yml" "      - DATABASE_URL=${FAKE_DSN}" "url_with_inline_credential"
must_block "D2 .yml 內 password 字面值被擋"      "docs/ops/compose.prod.yml" "      - POSTGRES_PASSWORD=${FAKE_PW}" "config_secret_literal"
must_block "D3 .yml 內 env 預設值寫死密碼被擋"   "docs/ops/compose.prod.yml" "      - POSTGRES_PASSWORD=\${POSTGRES_PASSWORD:-${FAKE_PW}}" "env_default_secret_literal"
must_block "D4 docs/**/*.yml 不降級為 warn-only" "docs/ops/extra.yaml"       "password: ${FAKE_PW}" "config_secret_literal"

must_pass  "D5 \${VAR} 插值不誤擋"              "docs/ops/compose.prod.yml"   "      - POSTGRES_PASSWORD=\${POSTGRES_PASSWORD:-atlas}"
must_pass  "D6 *.example.yml 維持 warn-only"      "docs/ops/compose.example.yml" "      - POSTGRES_PASSWORD=${FAKE_PW}"
must_pass  "D7 合成測試 DSN（example.com）不誤擋" "internal/x_test.go" 't.Setenv("DATABASE_URL", "postgres://alice:secretpw1@db.example.com:5432/atlas")'
must_pass  "D8 程式碼取值（p.config.APIKey）不誤擋" "internal/ws.go" "auth.Data.APIKey = p.config.APIKey"

# ── C. 輸出遮蔽 ──────────────────────────────────────────────────
rm -rf "$TMP/src" "$TMP/docs"; mkdir -p "$TMP/src"
printf '%s\n' "TOKEN=$TG_TOKEN" > "$TMP/src/c1.py"
scan_run 0
if grep -qF "$TG_TOKEN" "$TMP/out.txt"; then bad "C1 輸出洩漏完整憑證（未遮蔽）"
else ok "C1 輸出已遮蔽（不含完整憑證）"; fi
HINT="1234""…""2345"
grep -qF "$HINT" "$TMP/out.txt" && ok "C2 遮蔽格式為「前4…後4」" || { bad "C2 遮蔽格式不符（未見 ${HINT}）"; sed 's/^/     /' "$TMP/out.txt" | head -3; }

echo ""
if [ "$fail" -eq 0 ]; then
  echo "✅ secret-scan selftest PASS（$pass 項）"
  exit 0
fi
echo "❌ secret-scan selftest FAIL（pass=$pass fail=${fail}）"
exit 1
