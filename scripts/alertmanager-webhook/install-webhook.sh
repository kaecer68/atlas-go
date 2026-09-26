#!/usr/bin/env bash
# install-webhook.sh — 安裝／更新 alertmanager → Telegram 告警轉送（launchd）
#
# 為什麼需要這支腳本 ✗
#   repo 內的 plist（`com.goluck.atlas-webhook-to-telegram.plist`）**永遠不得含機密** ✓ —
#   它曾經把明文 telegram bot token 寫進版控（2026-08-27 起，且本 repo 是 **PUBLIC**）✗。
#   因此 plist 只放 placeholder `__INJECT_AT_INSTALL__`，真實 token 由**這支腳本在安裝期注入**到
#   `~/Library/LaunchAgents/` 的實例，並 `chmod 600`（原本安裝版是 644 = 同機其他使用者可讀 ✗）。
#
# 為什麼是「注入到 plist」而不是改成 `TELEGRAM_BOT_TOKEN_FILE` ✗
#   a2a-dev 的 `macmini-recover` #50 會從**已安裝 plist 的 `EnvironmentVariables`** 讀
#   `TELEGRAM_BOT_TOKEN` 做 `getMe` 有效性檢查 ✗ → 換 key 名會讓它變成 WARN ✗
#   （而 recover 的 DoD 是 48 OK / **0 WARN** / 0 FAIL ✗）⇒ 介面必須維持不變 ✓。
#
# 用法
#   bash scripts/alertmanager-webhook/install-webhook.sh --token-file ~/.config/atlas-alert-webhook/telegram-bot-token
#   bash scripts/alertmanager-webhook/install-webhook.sh --from-env-file ~/.config/atlas-go/.env
#   TELEGRAM_BOT_TOKEN=<token> bash scripts/alertmanager-webhook/install-webhook.sh
#   bash scripts/alertmanager-webhook/install-webhook.sh --no-load --home /tmp/sandbox   # 沙箱（測試用）
#
# token 來源優先序（前者勝）: `--token-file` → `--from-env-file` → 環境變數 `TELEGRAM_BOT_TOKEN`
#                            → 預設檔 `$HOME/.config/atlas-alert-webhook/telegram-bot-token`
# 永不印出 token ✓（只印前 4 … 後 4 與長度）。
set -euo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_PLIST="$SELF_DIR/com.goluck.atlas-webhook-to-telegram.plist"
SRC_PY="$SELF_DIR/atlas-alertmanager-webhook-to-telegram.py"
LABEL="com.goluck.atlas-webhook-to-telegram"
PLACEHOLDER="__INJECT_AT_INSTALL__"

HOME_DIR="${HOME}"
DEST_DIR=""
BIN_DIR=""
LOG_DIR=""
TOKEN=""
TOKEN_FILE=""
ENV_FILE=""
CHAT_ID=""
NO_LOAD=0
VERIFY=0

usage() { sed -n '3,26p' "${BASH_SOURCE[0]}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --token-file)     TOKEN_FILE="${2:-}"; shift ;;
    --from-env-file)  ENV_FILE="${2:-}"; shift ;;
    --chat-id)        CHAT_ID="${2:-}"; shift ;;
    --home)           HOME_DIR="${2:-}"; shift ;;
    --bin-dir)        BIN_DIR="${2:-}"; shift ;;
    --dest-dir)       DEST_DIR="${2:-}"; shift ;;
    --no-load|--dry-run) NO_LOAD=1 ;;
    --verify)         VERIFY=1 ;;
    -h|--help)        usage; exit 0 ;;
    *) echo "未知參數: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

[ -d "$HOME_DIR" ] || { echo "❌ --home 不存在: $HOME_DIR" >&2; exit 2; }
[ -f "$SRC_PLIST" ] || { echo "❌ 找不到來源 plist: $SRC_PLIST" >&2; exit 2; }
[ -f "$SRC_PY" ]    || { echo "❌ 找不到來源腳本: $SRC_PY" >&2; exit 2; }
[ -n "$DEST_DIR" ] || DEST_DIR="$HOME_DIR/Library/LaunchAgents"
[ -n "$BIN_DIR" ]  || BIN_DIR="$HOME_DIR/bin"
[ -n "$LOG_DIR" ]  || LOG_DIR="$HOME_DIR/Library/Logs"

# ── 1. 取得 token（來源優先序；永不寫進 log）──────────────────────────
if [ -n "$TOKEN_FILE" ]; then
  [ -f "$TOKEN_FILE" ] || { echo "❌ --token-file 不存在: $TOKEN_FILE" >&2; exit 2; }
  TOKEN="$(tr -d '[:space:]' < "$TOKEN_FILE")"
elif [ -n "$ENV_FILE" ]; then
  [ -f "$ENV_FILE" ] || { echo "❌ --from-env-file 不存在: $ENV_FILE" >&2; exit 2; }
  TOKEN="$(sed -n 's/^[[:space:]]*TELEGRAM_BOT_TOKEN[[:space:]]*=[[:space:]]*//p' "$ENV_FILE" | head -1 | tr -d "'\"[:space:]")"
elif [ -n "${TELEGRAM_BOT_TOKEN:-}" ]; then
  TOKEN="$TELEGRAM_BOT_TOKEN"
else
  DEFAULT_TOKEN_FILE="$HOME_DIR/.config/atlas-alert-webhook/telegram-bot-token"
  if [ -f "$DEFAULT_TOKEN_FILE" ]; then
    TOKEN="$(tr -d '[:space:]' < "$DEFAULT_TOKEN_FILE")"
  fi
fi

if [ -z "$TOKEN" ]; then
  cat >&2 <<'EOS'
❌ 找不到 telegram bot token（不會有任何憑證進入版控，這是刻意的）。
   任選一種提供方式:
     1) --token-file <path>        例: ~/.config/atlas-alert-webhook/telegram-bot-token（chmod 600）
     2) --from-env-file <path>     例: ~/.config/atlas-go/.env（讀 TELEGRAM_BOT_TOKEN= 那一行）
     3) TELEGRAM_BOT_TOKEN=<token> 以環境變數提供
     4) 預設檔 $HOME/.config/atlas-alert-webhook/telegram-bot-token
EOS
  exit 2
fi
case "$TOKEN" in
  # ⚠️ 變數展開一律加 ${} 大括號：bash 會把緊跟其後的非 ASCII 字元（例：全角「（」）
  #    當成變數名的一部分 ⇒ `$PLACEHOLDER）` 會展開成未定義變數，在 `set -u` 下直接崩潰，
  #    使用者看到的是 "unbound variable" 而不是下面這句可行動訊息（2026-09-26 issue #2011 實測）。
  *"$PLACEHOLDER"*) echo "❌ 提供的 token 就是 placeholder（${PLACEHOLDER}）" >&2; exit 2 ;;
esac
if ! printf '%s' "$TOKEN" | grep -Eq '^[0-9]{8,12}:[A-Za-z0-9_-]{30,}$'; then
  echo "❌ token 形狀不像 telegram bot token（應為 <digits>:<35 字元>）；長度=${#TOKEN}" >&2
  exit 2
fi
TOKEN_MASKED="$(printf '%s' "$TOKEN" | cut -c1-4)…$(printf '%s' "$TOKEN" | rev | cut -c1-4 | rev)"

echo "atlas alertmanager webhook 安裝"
echo "  home       : $HOME_DIR"
echo "  plist      : $DEST_DIR/$LABEL.plist   (mode 600)"
echo "  program    : $BIN_DIR/atlas-alertmanager-webhook-to-telegram.py"
echo "  token      : $TOKEN_MASKED (len=${#TOKEN})"

if [ "$VERIFY" -eq 1 ]; then
  TM="$(curl -s -m 10 "https://api.telegram.org/bot${TOKEN}/getMe" || true)"
  case "$TM" in
    *'"ok":true'*) echo "  getMe      : ok ✓" ;;
    *) echo "  getMe      : ❌ 非 ok（$(printf '%s' "$TM" | head -c 120)）—— 這個 token 送不出告警" >&2; exit 1 ;;
  esac
fi

# ── 2. 渲染 plist（placeholder → token；/Users/kk → 實際 home）────────
mkdir -p "$DEST_DIR" "$BIN_DIR" "$LOG_DIR"
TMP_PLIST="$(mktemp)"; trap 'rm -f "$TMP_PLIST"' EXIT
DEST_PLIST="$DEST_DIR/$LABEL.plist"

sed -e "s|$PLACEHOLDER|$TOKEN|g" -e "s|/Users/kk|$HOME_DIR|g" "$SRC_PLIST" > "$TMP_PLIST"
if [ -n "$CHAT_ID" ]; then
  sed -i.bak -e "s|^\\( *<string>\\)[0-9]\\{6,\\}\\(</string> *\\)$|\\1${CHAT_ID}\\2|" "$TMP_PLIST" && rm -f "$TMP_PLIST.bak"
fi
if grep -q "$PLACEHOLDER" "$TMP_PLIST"; then
  echo "❌ 渲染後仍有 placeholder —— 中止（避免裝出一支永遠失敗的服務）" >&2
  exit 1
fi
if command -v plutil >/dev/null 2>&1; then
  plutil -lint "$TMP_PLIST" >/dev/null || { echo "❌ 渲染出的 plist 不是合法 plist" >&2; exit 1; }
fi

if [ -f "$DEST_PLIST" ]; then
  BACKUP="$DEST_PLIST.bak-$(date +%Y%m%d-%H%M%S)"
  cp -p "$DEST_PLIST" "$BACKUP"
  echo "  既有 plist 備份: $BACKUP"
fi

install -m 600 "$TMP_PLIST" "$DEST_PLIST"
install -m 755 "$SRC_PY" "$BIN_DIR/atlas-alertmanager-webhook-to-telegram.py"
echo "  ✅ plist 已寫入（mode 600）+ 程式已就位"

# ── 3. 載入 launchd（--no-load 時只印計畫）──────────────────────────
UID_NUM="${UID:-$(id -u)}"
if [ "$NO_LOAD" -eq 1 ]; then
  echo "  (--no-load: 未碰 launchd；正式載入請移除本旗標)"
  echo "    將執行: launchctl bootout gui/$UID_NUM/$LABEL ; launchctl bootstrap gui/$UID_NUM $DEST_PLIST"
  exit 0
fi
if [ "$(uname -s)" != "Darwin" ]; then
  echo "⚠️  非 macOS —— 只完成檔案安裝，略過 launchctl" >&2
  exit 0
fi
launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || true
if launchctl bootstrap "gui/$UID_NUM" "$DEST_PLIST" 2>/dev/null; then
  echo "  ✅ launchctl bootstrap 完成"
else
  echo "  ⚠️  bootstrap 失敗 → 退回 legacy load"
  launchctl load -w "$DEST_PLIST"
fi
sleep 2
launchctl print "gui/$UID_NUM/$LABEL" 2>/dev/null | grep -E "^[[:space:]]*(state|pid) = " || true
echo "  驗證: tail -5 $LOG_DIR/atlas-webhook.err.log（應見 'listening on :9095'）"
