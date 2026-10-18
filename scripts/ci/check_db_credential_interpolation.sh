#!/usr/bin/env bash
# scripts/ci/check_db_credential_interpolation.sh
# 部署 compose 的 DB 憑證必須是**插值**（${DB_USER} / ${DB_PASSWORD}），不得寫死；
# 且 .env.example 只能有空值 key（它是模板，真值只放 git-ignored 的 .env）。
#
# 【為什麼需要這支】（2026-10-18）
#   生產（Mac Mini ~/workspace/atlas）的 clone 曾出現 **2 個未提交編輯**（M .env.example、M docker-compose.yml）
#   —— DB 密碼輪替時就地改的，內容是「把字面 user/password 換成 ${DB_USER}/${DB_PASSWORD} 插值」。
#   那些編輯是 load-bearing（生產正靠它們跑），但**未進版控** ⇒ 下一次部署的 `git pull` / `checkout`
#   可能衝突或覆蓋 ✗。本 PR 把它們上游化，並用這支守門把不變式釘住，防止字面憑證再長回來。
#
# 【既有守門為何抓不到】`scripts/secret-scan.sh` 掃的是「**像機密**」的字串（token / sk- / 私鑰…）。
#   字面 DB user/password 是 `atlas` 這種**不像機密**的值 ⇒ 實測（2026-10-18）餵
#   `POSTGRES_USER=atlas` / `POSTGRES_PASSWORD=atlas` / `postgres://atlas:atlas@…` 給 secret-scan
#   得到 `✅ 無命中`、rc=0 ⇒ 這條不變式**沒有**既有守門。
#
# 【釘住的契約】
#   A. 部署 compose（repo 根 `docker-compose.yml` ＋ `docs/operations/docker-compose*.yml`）：
#      每個 `POSTGRES_USER=` / `POSTGRES_PASSWORD=` 的值必須以 `${` 開頭
#   B. 同一組 compose：每個 DSN（`postgres://` / `postgresql://`）的 userinfo 中，
#      user 與 password 兩段都必須以 `${` 開頭（`${DB_PASSWORD:-atlas}` 這種帶預設值的寫法算合規）
#   C. .env.example：`DB_USER=` 與 `DB_PASSWORD=` 必須存在且為**空值**
#   註解行（第一個非空白字元是 `#`）一律跳過。
#
# 【2026-10-18 擴大掃描範圍（#2288 的缺口）】#2288 只涵蓋 repo 根 `docker-compose.yml`，
#   但同一條部署路徑有**第二份** compose：`docs/operations/docker-compose.prod.yml`
#   （`DATABASE_URL` / `POSTGRES_USER` 當時仍是**字面 user `atlas`**）與
#   `docs/operations/docker-compose.crons.yml`（`DATABASE_URL` ×3 同病）
#   ⇒ 病一樣（憑證在版控裡 ＋ 輪替只改一處會靜默壞掉），但守門**掃不到** ⇒ 假綠。
#   本檔改為 glob 掃描（新增部署 compose 會自動納入，不會靜默漏掃）；`docs/operations/`
#   存在卻一檔都沒掃到 ⇒ rc=2（覆蓋消失也是 fail-closed，不是通過）。
#   `.env.example` 的空值 key 只走契約 C ⇒ **不會**被 A/B 誤判（A/B 只掃 compose）。
#
# 用法:
#   bash scripts/ci/check_db_credential_interpolation.sh              # 掃本 repo
#   bash scripts/ci/check_db_credential_interpolation.sh --root DIR   # 掃指定目錄（測試用）
# exit: 0 = 合規；1 = 有違規；2 = 用法／環境錯誤（未知旗標、受測檔不存在）
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --root) ROOT="${2:-}"; shift ;;
    -h|--help) sed -n '2,37p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "unknown option: $1" >&2
       echo "usage: check_db_credential_interpolation.sh [--root DIR]" >&2
       exit 2 ;;
  esac
  shift
done

# 受測 compose：repo 根 ＋ docs/operations/ 底下所有 `docker-compose*.yml`
# （用 glob 而非硬編清單 ⇒ 新增的部署 compose 自動納入掃描，不會靜默漏掃）
COMPOSE_FILES=()
for f in "$ROOT"/docker-compose*.yml "$ROOT"/docs/operations/docker-compose*.yml; do
  [ -f "$f" ] && COMPOSE_FILES+=("$f")
done
ENV_EXAMPLE="$ROOT/.env.example"
FAIL=0

echo "═══ check_db_credential_interpolation ═══"

# 受測檔不存在 ⇒ fail loud（不可當成「掃到 0 筆 ⇒ 通過」）
missing=0
for f in "$ROOT/docker-compose.yml" "$ENV_EXAMPLE"; do
  if [ ! -f "$f" ]; then
    echo "  ❌ 找不到受測檔：$f ⇒ 掃不到檔案不是通過（fail-closed）"
    missing=1
  fi
done
# 覆蓋完整性：docs/operations/ 在、底下卻沒有任何 compose 被掃到 ⇒ 部署路徑靜默失去覆蓋
if [ "$missing" -eq 0 ] && [ -d "$ROOT/docs/operations" ]; then
  covered=0
  for f in "${COMPOSE_FILES[@]}"; do
    case "$f" in "$ROOT/docs/operations/"docker-compose*) covered=1 ;; esac
  done
  if [ "$covered" -eq 0 ]; then
    echo "  ❌ $ROOT/docs/operations/ 存在但底下沒有 docker-compose*.yml 被掃到 ⇒ 覆蓋消失（fail-closed）"
    missing=1
  fi
fi
[ "$missing" -eq 0 ] || exit 2

echo "  受測 compose（${#COMPOSE_FILES[@]} 檔）："
for f in "${COMPOSE_FILES[@]}"; do
  echo "    - ${f#"$ROOT"/}"
done

# ── A + B：compose 的憑證插值 ──────────────────────────────────────────────────
compose_violations="$(
for f in "${COMPOSE_FILES[@]}"; do
awk '
{
  raw = $0
  t = raw; sub(/^[ \t]+/, "", t)
  if (t == "" || substr(t, 1, 1) == "#") next

  if (match(raw, /POSTGRES_USER=[^ \t]*/)) {
    v = substr(raw, RSTART + 14, RLENGTH - 14)
    if (substr(v, 1, 2) != "${")
      printf "  ❌ %s:%d POSTGRES_USER 不是插值（值=%s）\n", FILENAME, NR, v
  }
  if (match(raw, /POSTGRES_PASSWORD=[^ \t]*/)) {
    v = substr(raw, RSTART + 18, RLENGTH - 18)
    if (substr(v, 1, 2) != "${")
      printf "  ❌ %s:%d POSTGRES_PASSWORD 不是插值（值已遮蔽）\n", FILENAME, NR
  }
  if (match(raw, /(postgres|postgresql):\/\/[^@]*@/)) {
    seg = substr(raw, RSTART, RLENGTH)
    sub(/^[a-z]+:\/\//, "", seg)
    sub(/@$/, "", seg)
    # userinfo 內可能有 `${VAR:?訊息}`（冒號在 `${…}` 內）⇒ 只認「不在 `${…}` 內」的第一個冒號當分隔。
    # （naive 的 index(seg, ":") 會把 `${DB_USER:?…}:${DB_PASSWORD:?…}` 切成 `${DB_USER` / `?…}…` ⇒ 假紅）
    cut = 0; depth = 0
    for (i = 1; i <= length(seg); i++) {
      c = substr(seg, i, 1)
      if (c == "$" && substr(seg, i + 1, 1) == "{") { depth++; i++; continue }
      if (c == "}" && depth > 0) { depth--; continue }
      if (c == ":" && depth == 0) { cut = i; break }
    }
    if (cut > 0) { u = substr(seg, 1, cut - 1); p = substr(seg, cut + 1) } else { u = seg; p = "" }
    if (substr(u, 1, 2) != "${")
      printf "  ❌ %s:%d DSN 的 user 不是插值（user=%s）\n", FILENAME, NR, u
    if (p != "" && substr(p, 1, 2) != "${")
      printf "  ❌ %s:%d DSN 的 password 不是插值（password 已遮蔽）\n", FILENAME, NR
  }
}
' "$f"
done
)"
if [ -n "$compose_violations" ]; then
  echo "$compose_violations"
  echo "  ⇒ 部署 compose 的 DB 憑證必須寫成 \${DB_USER} / \${DB_PASSWORD}（輪替只改 .env 一處）"
  FAIL=1
else
  echo "  ✅ ${#COMPOSE_FILES[@]} 檔 compose 的 DB 憑證皆為插值（\${DB_USER} / \${DB_PASSWORD}）"
fi

# ── C：.env.example 的 DB_USER / DB_PASSWORD 必須存在且為空值 ────────────────────
for key in DB_USER DB_PASSWORD; do
  entry="$(grep -nE "^${key}=" "$ENV_EXAMPLE" | head -1 || true)"
  if [ -z "$entry" ]; then
    echo "  ❌ .env.example 缺少 ${key}= 條目（部署模板必須明列此鍵，且為空值）"
    FAIL=1
    continue
  fi
  val="${entry#*=}"
  if [ -n "$val" ]; then
    echo "  ❌ .env.example:${entry%%:*} ${key} 不是空值（模板不得帶值；真值只放 .env）"
    FAIL=1
  fi
done
[ "$FAIL" -ne 0 ] || echo "  ✅ .env.example 的 DB_USER / DB_PASSWORD 皆為空值 key"

echo ""
if [ "$FAIL" -ne 0 ]; then
  echo "❌ check_db_credential_interpolation FAILED"
  exit 1
fi
echo "✅ check_db_credential_interpolation PASS — compose DB credentials are interpolated; .env.example keys are empty"
exit 0
