#!/usr/bin/env bash
# test-install-webhook.sh — 「機密不進版控」機制的回歸測試
#
# 兩個必須永遠成立的事實（否則機制會靜默退化 ✗）:
#   A. **repo 內的 plist 永遠不含真 token** ✓（只有 placeholder）
#   B. **安裝期注入真的有效** ✓：真 token 只落到 `~/Library/LaunchAgents/` 的實例、檔案 mode 600、
#      且**不會被印到 stdout/stderr**（log 也是外洩面）。
# 另外驗「壞輸入一定要失敗」✗：placeholder 當 token、形狀不對的 token。
#
# 全程在 mktemp 沙箱（`--home`）內操作，**不碰真的 launchd**（`--no-load`）✓。
# ⚠️ 假 token 用相鄰字串相接組出 → 本檔自身不含可被 secret-scan 命中的字面值。
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INSTALLER="$ROOT/scripts/alertmanager-webhook/install-webhook.sh"
REPO_PLIST="$ROOT/scripts/alertmanager-webhook/com.goluck.atlas-webhook-to-telegram.plist"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

FAKE_TOKEN="1234567890:""FAKEfakeFAKEfakeFAKEfakeFAKEfake12345"
SANDBOX="$TMP/home"; mkdir -p "$SANDBOX"
printf '%s\n' "$FAKE_TOKEN" > "$TMP/token"; chmod 600 "$TMP/token"

echo "── install-webhook 自我測試 ──"

# ── A. repo plist 不含真 token ────────────────────────────────────
A_OUT="$(python3 - "$REPO_PLIST" <<'PY'
import plistlib, sys
d = plistlib.load(open(sys.argv[1], "rb"))
print(d.get("EnvironmentVariables", {}).get("TELEGRAM_BOT_TOKEN", "<missing>"))
PY
)"
if [ "$A_OUT" = "__INJECT_AT_INSTALL__" ]; then ok "A1 repo plist 的 token 是 placeholder（無機密）"
else bad "A1 repo plist 內不是 placeholder（實際: ${A_OUT}）"; fi

# ── B. 安裝期注入 ─────────────────────────────────────────────────
if bash "$INSTALLER" --no-load --home "$SANDBOX" --token-file "$TMP/token" > "$TMP/out.txt" 2>&1; then
  ok "B1 安裝（--token-file）exit 0"
else
  bad "B1 安裝失敗"; sed 's/^/     /' "$TMP/out.txt" | head -5
fi
DEST="$SANDBOX/Library/LaunchAgents/com.goluck.atlas-webhook-to-telegram.plist"
if [ -f "$DEST" ]; then ok "B2 已安裝 plist"; else bad "B2 找不到已安裝 plist"; fi

# ⚠️ 平台差異：**GNU `stat -f` 是「檔案系統」模式**（不是 BSD 的「自訂格式」）✗
# 2026-09-25 實證：GH CI（ubuntu-latest）上 `stat -f '%Lp'` 會把檔案系統資訊印到 stdout 並回非 0，
# 於是 `||` 再跑 `stat -c '%a'` 把 "600" 接在後面 ⇒ 多行字串 ≠ "600" ⇒ 假失敗 ✗。
# 正確順序：先 GNU 的 `-c`，失敗才退 BSD 的 `-f`（macOS 上 `stat -c` 乾淨失敗、無 stdout ✓）。
file_mode() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null
}
MODE="$(file_mode "$DEST")"
if [ "$MODE" = "600" ]; then ok "B3 已安裝 plist mode=600（原本 644 是同機可讀 ✗）"
else bad "B3 已安裝 plist mode=${MODE}（期望 600）"; fi

INJ="$(python3 - "$DEST" <<'PY'
import plistlib, sys
print(plistlib.load(open(sys.argv[1], "rb")).get("EnvironmentVariables", {}).get("TELEGRAM_BOT_TOKEN", ""))
PY
)"
if [ "$INJ" = "$FAKE_TOKEN" ]; then ok "B4 真 token 已注入（介面名維持 TELEGRAM_BOT_TOKEN → a2a-dev macmini-recover #50 不受影響）"
else bad "B4 注入的 token 不符"; fi

if grep -q "/Users/kk" "$DEST"; then bad "B5 路徑未改成實際 home（仍有 /Users/kk）"
else ok "B5 路徑已改成實際 home"; fi

if [ -f "$SANDBOX/bin/atlas-alertmanager-webhook-to-telegram.py" ]; then ok "B6 relay 程式已就位"
else bad "B6 relay 程式未複製"; fi

if grep -qF "$FAKE_TOKEN" "$TMP/out.txt"; then bad "B7 安裝輸出洩漏 token ✗"
else ok "B7 安裝輸出不含 token（僅遮蔽顯示）"; fi

# ── C. 壞輸入必須失敗 ─────────────────────────────────────────────
bad_case() { # bad_case <說明> <args…>
  local desc="$1"; shift
  if bash "$INSTALLER" --no-load --home "$TMP/home2" "$@" > "$TMP/bad.txt" 2>&1; then
    bad "${desc}（期望非 0，得到 0）"
  else
    ok "$desc"
  fi
}
printf '%s\n' '__INJECT_AT_INSTALL__' > "$TMP/placeholder"; chmod 600 "$TMP/placeholder"
bad_case "C1 placeholder 當 token → 拒絕" --token-file "$TMP/placeholder"
printf '%s\n' 'not-a-telegram-token' > "$TMP/bad-token"
bad_case "C2 形狀不對的 token → 拒絕" --token-file "$TMP/bad-token"
bad_case "C3 完全不給來源 → 拒絕（不會靜默裝出壞服務）" --home "$TMP/home2" --token-file "$TMP/missing-file"

# ── D. --from-env-file 與既有 plist 備份 ──────────────────────────
printf 'PORT=9095\nTELEGRAM_BOT_TOKEN="%s"\n' "$FAKE_TOKEN" > "$TMP/.env"
if bash "$INSTALLER" --no-load --home "$SANDBOX" --from-env-file "$TMP/.env" > "$TMP/out2.txt" 2>&1; then
  ok "D1 --from-env-file 可解析 TELEGRAM_BOT_TOKEN= 行"
else
  bad "D1 --from-env-file 失敗"; sed 's/^/     /' "$TMP/out2.txt" | head -5
fi
if compgen -G "$DEST.bak-*" > /dev/null; then ok "D2 覆蓋前會備份既有 plist（.bak-<ts>）"
else bad "D2 沒有產生備份"; fi

echo ""
if [ "$fail" -eq 0 ]; then
  echo "✅ install-webhook selftest PASS（$pass 項）"
  exit 0
fi
echo "❌ install-webhook selftest FAIL（pass=${pass} fail=${fail}）"
exit 1
