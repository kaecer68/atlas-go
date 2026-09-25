#!/usr/bin/env bash
# secret-scan.sh — 掃「tracked 檔」的機密樣式（public repo 護欄）
#
# ⚠️ 本檔是 **a2a-dev `scripts/secret-scan.sh` 的整檔副本**（SSOT 在那邊）。
#    上游版本: a2a-dev @ fix/redact-dev-literals-q2 4b332b1, sha256 8f4666252e41361975d846de4f05d41328bf978209c7cd5dd0d8fd8b5f533356
#    為何用「副本」而不是「引用上游」：① 本 repo 是 **PUBLIC**，CI 不該為了掃描去 clone 另一個 repo
#    （多一條網路依賴 + 供應鏈面）② 兩個 repo 都要能在**離線**狀態跑完 CI ③ 副本同步成本極低（整檔覆蓋）。
#    同步方式：`cp <a2a-dev>/scripts/secret-scan.sh scripts/secret-scan.sh` 後更新上面的 sha256。
#    漂移偵測：兩份 sha256 不同即代表需要人工確認（本行即為錨點）。
#
#    2026-09-25（任務 Q）同步 ①：新增 url_with_inline_credential / config_secret_literal /
#    env_default_secret_literal 三個通用樣式，並把「可部署設定檔（*.yml/*.yaml，非範例）」
#    從 warn-only 改成 block 級 —— 本 repo 的 docs/operations/docker-compose.{prod,crons}.yml
#    就是靠這條才被抓到明文 DB 密碼。
#    同步 ②：新增 SENTINEL_VALUE 過濾 —— 全大寫＋底線且不含數字的值（`PROXY_MANAGED` 這類
#    語意旗標）不算機密。它曾讓 `${OPENAI_API_KEY:-PROXY_MANAGED}` 被誤判成明文憑證，
#    而「拿掉模板預設值」的修法會**改變行為**（誤判比漏抓更危險）。
#
#
# 為什麼需要它：a2a-dev / atlas-go 都是 **PUBLIC** repo，任何一次誤 commit 就是**永久外洩**
# （歷史不會消失，且 bot token 之類的憑證無法「刪 commit 就回收」）。此腳本把「機密不進版控」
# 變成 CI 硬閘門，而不是靠人記得。
#
# 用法:
#   bash scripts/secret-scan.sh                  # 掃本 repo（tracked 檔）
#   bash scripts/secret-scan.sh --strict         # 文件/範例檔命中也算失敗
#   bash scripts/secret-scan.sh --root DIR       # 掃指定目錄（測試用；無 git 時改走 find）
#   bash scripts/secret-scan.sh --allowlist FILE # 指定 allowlist（預設 scripts/secret-scan-allowlist.txt）
#   bash scripts/secret-scan.sh --quiet          # 只印結論
#
# 三種降噪機制（缺一就會變成噪音 → 被繞過）:
#   ① 路徑類別：文件/範例類（*.md、docs/**、*.example、*sample*、*template* …）預設 **warn-only**：
#      會列出來但不讓 CI 失敗（文件裡寫 `sk-xxxx` 範例是正當需求）。`--strict` 才把它們算失敗。
#      **例外：可部署的設定檔（*.yml / *.yaml，且非 *.example*/*sample*/*template*）一律 block 級** ✓
#      —— 理由：docs/ 底下的 compose/設定 YAML 是**會被拿去跑/部署**的檔，不是散文；
#      在那裡出現字面憑證就是真的外洩，不該只印警告。2026-09-25 實證：
#      `docs/operations/docker-compose.prod.yml` 的 `DATABASE_URL` / `POSTGRES_PASSWORD` 預設值
#      帶明文 DB 密碼，而舊掃描器既沒涵蓋它、也沒有能命中的樣式（兩者都補）。
#   ② 行內豁免：同一行含 `secret-scan-allow` 註解 → 該行跳過（必須寫理由）。
#   ③ allowlist 檔：`scripts/secret-scan-allowlist.txt` 逐條 `glob [pattern…] # 理由`。
#
# 輸出永遠**遮蔽**命中值（只留前 4 後 4）——避免把機密二次寫進 CI log。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STRICT=0
QUIET=0
ALLOWLIST=""

while [ $# -gt 0 ]; do
  case "$1" in
    --strict) STRICT=1 ;;
    --quiet) QUIET=1 ;;
    --root) ROOT="${2:-}"; shift ;;
    --allowlist) ALLOWLIST="${2:-}"; shift ;;
    -h|--help) sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "未知參數: $1" >&2; exit 2 ;;
  esac
  shift
done
[ -d "$ROOT" ] || { echo "❌ secret-scan: --root 不存在: $ROOT" >&2; exit 2; }
[ -n "$ALLOWLIST" ] || ALLOWLIST="$ROOT/scripts/secret-scan-allowlist.txt"

python3 - "$ROOT" "$STRICT" "$QUIET" "$ALLOWLIST" <<'PY'
import os, re, subprocess, sys, fnmatch

root, strict, quiet, allowlist = sys.argv[1], sys.argv[2] == "1", sys.argv[3] == "1", sys.argv[4]

# ── 樣式表：只放「高信心」的憑證形狀（寧可少抓，不要噪音）──────────────
PATTERNS = [
    ("telegram_bot_token", re.compile(r"\b\d{8,12}:[A-Za-z0-9_-]{33,}\b")),
    ("openai_sk_key",      re.compile(r"\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_-]{20,}")),
    ("github_token",       re.compile(r"\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b|\bgithub_pat_[A-Za-z0-9_]{22,}\b")),
    ("aws_access_key_id",  re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    ("private_key_block",  re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
    ("slack_token",        re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}")),
    ("google_api_key",     re.compile(r"\bAIza[0-9A-Za-z_-]{35}\b")),
    # ── 2026-09-25 新增（任務 Q）────────────────────────────────────────
    # 為什麼要加：上面全是「特定廠商憑證形狀」，抓不到**通用**的自架服務憑證
    # （DB 連線字串內嵌密碼、設定檔內的 password/secret 字面值）——那正是
    # docs/operations/docker-compose.prod.yml 當時外洩的形狀。
    #
    # ② URL 內嵌 userinfo 密碼：`scheme://user:secret@host`  # secret-scan-allow: 本行是樣式說明文字（非真憑證）
    #    密碼段 ≥6 字且**不得以 $ % { " ' ` 開頭** → `${DB_PASSWORD:-atlas}` 這類插值不會誤命中 ✓
    #    （尾端一併納入 host：噪音過濾需要看主機才能排除 RFC 2606 / 本機的測試 DSN，
    #      否則 `alice:secret@db.example.com` 這種 fixture 會變成噪音）
    ("url_with_inline_credential",
     re.compile(r"[a-z][a-z0-9+.\-]{2,15}://[^:@\s/\"']{1,64}:[^@\s/\"'$%{`]{6,}@[A-Za-z0-9._\-]+", re.I)),
    # ④ `${VAR:-<default>}` 內的預設值：預設值寫成機密字面值 → 一樣是「現行檔案含明文」
    #    （2026-09-25 實證：`POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-<19 字密碼>}` 逃過前三條）
    #    門檻 12 字 + 噪音過濾（排除含 / 或空白者，例如 `${ATLAS_CONFIG_DIR:-/app/configs}`）
    #    ⚠️ 變數名必須**看起來像機密**（PASSWORD/TOKEN/SECRET/…）——否則 `${MACMINI_HOST:-…}`、
    #    `${CANARY_WHY:-…}`、`${CONTAINER:-…}` 這類「長但不是機密」的預設值會變成噪音
    #    （2026-09-25 實測：不加這道濾網 → a2a-dev 13 筆、atlas 12 筆全假警報）。
    ("env_default_secret_literal",
     re.compile(r"\$\{[A-Za-z_]*(?:PASSWORD|PASSWD|PWD|SECRET|TOKEN|APIKEY|API_KEY|PRIVATE_KEY|CREDENTIAL)[A-Za-z_]*:[-=]([^}\s]{12,})\}", re.I)),
    # ③ 設定檔內的 secret 字面值：KEY=value / KEY: value，值 ≥12 字、非插值、非 placeholder
    #    為什麼門檻這麼高（寧可少抓不要噪音）：`POSTGRES_PASSWORD=atlas`（dev 預設，5 字）
    #    與 `${...}` 插值都不該命中；`POSTGRES_PASSWORD=<19 字、含數字的字面值>` 才會命中 ✓
    ("config_secret_literal",
     #    邊界用 (?<![A-Za-z0-9]) / (?![A-Za-z0-9]) 而**不是** \b —— 否則 `POSTGRES_PASSWORD=`
     #    這種（底線前綴）在 `\bPASSWORD\b` 下不成立（`_` 是 word 字元）→ 漏抓最常見的真實形狀。
     re.compile(r"(?i)(?<![A-Za-z0-9])(?:password|passwd|pwd|secret|client_secret|api[_-]?key|apikey"
                r"|auth[_-]?token|access[_-]?token|bot[_-]?token|private[_-]?key|db[_-]?pass)(?![A-Za-z0-9])"
                r"\s*[:=]\s*[\"']?"
                r"(?![$<{%\s])"
                r"(?!(?i:change|example|placeholder|redacted|dummy|sample|fake|test|dev|admin|root|password|secret|none|null|unset|todo|xxxx|your))"
                r"[A-Za-z0-9][A-Za-z0-9._@!#%^&*+\-]{11,}[\"']?\s*$")),
]

# ── 路徑類別 ──────────────────────────────────────────────────────────
# warn-only = 文件／範例／模板：寫範例 token 是正當用途，預設不讓 CI 失敗。
WARN_ONLY_GLOBS = [
    "*.md", "*.markdown", "*.txt", "*.rst", "*.json.md",
    "docs/*", "*/docs/*",
    "*.example", ".env.example", "*.example.*", "*.sample", "*sample*", "*.template", "*template*",
    "*.json.sample", "*fixtures*",
]
# 可部署的設定檔：即使在 docs/ 底下也**不**降級為 warn-only（見檔頭 ① 的例外說明）。
CONFIG_GLOBS = ["*.yml", "*.yaml", "*.yml.j2", "*.yaml.j2"]
# 但「範例／模板」類的設定檔仍維持 warn-only（它們的用途就是放假值）。
CONFIG_EXAMPLE_GLOBS = ["*.example", "*.example.*", "*.sample", "*sample*", "*.template", "*template*",
                        "*fixtures*", ".env.example"]

# 直接跳過：不可能是手寫機密的產生物／二進位。
SKIP_GLOBS = [
    "*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.ico", "*.pdf", "*.zip", "*.gz", "*.tar",
    "*.woff", "*.woff2", "*.ttf", "*.mp4", "*.db", "*.sqlite", "*.lock", "*/package-lock.json",
    "*/node_modules/*", "*/.git/*", "*/__pycache__/*",
]

def matches(path, globs):
    return any(fnmatch.fnmatch(path, g) or fnmatch.fnmatch("/" + path, g) for g in globs)

# ── 檔案清單：優先用 git（tracked 檔才是會公開的東西）────────────────
def tracked_files():
    # ⚠️ 一定要 sanitize GIT_*：git hook（pre-push / pre-commit）會在環境注入 GIT_DIR /
    # GIT_WORK_TREE（SOP ★37）。若原樣傳下去，`git -C <tempdir> ls-files` 會**越界列出呼叫端
    # repo 的檔案**，於是 fixture 掃描看到一個不存在的清單 → 空命中 → 誤判 PASS ✗
    # （2026-09-25 實測：pre-push 下 selftest 由 15/15 PASS 變成 6/15，A1–A5 全「得到 0」）。
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    try:
        out = subprocess.run(["git", "-C", root, "ls-files", "-z"],
                             capture_output=True, text=True, timeout=60, env=env)
        if out.returncode == 0 and out.stdout.strip("\0"):
            listed = [p for p in out.stdout.split("\0") if p]
            # 再確認列出的路徑真的在 --root 底下（雙重防護；跨 repo 洩漏時直接忽略）
            inside = [p for p in listed if os.path.lexists(os.path.join(root, p))]
            if inside:
                return inside
    except Exception:
        pass
    # 非 git 目錄（測試 fixture）→ find
    found = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "__pycache__")]
        for fn in filenames:
            found.append(os.path.relpath(os.path.join(dirpath, fn), root))
    return sorted(found)

# ── allowlist ────────────────────────────────────────────────────────
ALLOW = []   # (glob, frozenset(pattern_names) | None, reason)
if allowlist and os.path.isfile(allowlist):
    for raw in open(allowlist, encoding="utf-8"):
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue                      # 整行註解（檔頭說明）
        body, _, reason = raw.partition("#")
        parts = body.split()
        if len(parts) < 2:
            continue
        glob, names = parts[0], frozenset(parts[1:])
        ALLOW.append((glob, names or None, reason.strip()))

def allowed(path, name):
    for glob, names, _ in ALLOW:
        if matches(path, [glob]) and (names is None or name in names):
            return True
    return False

def mask(s):
    return s[:4] + "…" + s[-4:] if len(s) > 12 else "…"

# ── 噪音過濾（只作用於 2026-09-25 新增的兩個「通用」樣式）──────────────────
# 為什麼需要：廠商憑證樣式（ghp_/sk-/AKIA…）本身就是高信心；但「URL 內嵌密碼」與
# 「設定檔 secret 字面值」是**形狀**規則，若不過濾會誤抓測試 fixture 與程式碼取值，
# 一旦有噪音就會被繞過（allowlist 濫用）→ 護欄等於沒有。過濾條件全部寫在下面。
NOISE_FILTERED = {"url_with_inline_credential", "config_secret_literal", "env_default_secret_literal"}
# 明確的合成主機（RFC 2606 保留域名 / 本機 / 測試常見字串）：這些不是「外洩」。
# 注意：**不**含 `host.docker.internal`（那正是 prod DSN 用的主機 ⇒ 必須保持會被抓到 ✓）。
SYNTHETIC_HOSTS = ("localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]",
                   "example.com", "example.org", "example.net", "db.invalid",
                   "nonexistent", "invalid")
PLACEHOLDER_WORDS = ("test", "invalid", "dummy", "fake", "sample", "example", "placeholder",
                     "changeme", "redacted", "xxxx", "your-", "your_", "none", "todo")
# 程式碼取值（`p.config.APIKey`、`os.environ.X`）不是字面機密 → 這種「點串」形狀排除。
DOTTED_IDENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$")
# **sentinel 值**：全大寫 + 底線、且**不含數字**（`PROXY_MANAGED`、`MANAGED_BY_PROXY` 這類
# 「語意旗標」）。真憑證幾乎不會長這樣；但含數字的 `PROD_PASSWORD_2026` 仍會被攔 ✓。
# 2026-09-25 實證：`OPENAI_API_KEY="${OPENAI_API_KEY:-PROXY_MANAGED}"` 被誤判成機密，
# 導致有人打算「拿掉預設值」——那會**改變模板行為**（未設值時就沒有 fallback）⇒ 修掃描器，
# 不是改模板（誤判比漏抓更危險：它會讓人為了消噪音而破壞正確的東西）。
SENTINEL_VALUE = re.compile(r"^[A-Z][A-Z_]*$")

def _value_of(name, matched):
    """取出「值」的部分（供 sentinel/placeholder 判斷用）。"""
    if name == "url_with_inline_credential":
        seg = matched.split("://", 1)[-1]
        return seg.split("@", 1)[0].split(":", 1)[-1]
    if name == "config_secret_literal":
        v = matched.split("=", 1)[1] if "=" in matched else matched.split(":", 1)[1]
        return v.strip().strip("\"'")
    if name == "env_default_secret_literal":
        m = re.search(r":[-=]([^}\s]+)\}", matched)
        return m.group(1) if m else ""
    return ""

def synthetic(name, matched):
    low = matched.lower()
    if any(w in low for w in PLACEHOLDER_WORDS):
        return True
    value = _value_of(name, matched)
    if value and SENTINEL_VALUE.match(value):
        return True
    if name == "url_with_inline_credential":
        return any(h in low for h in SYNTHETIC_HOSTS)
    if name == "config_secret_literal":
        if DOTTED_IDENT.match(value):          # 程式碼取值 p.config.APIKey
            return True
        # 純字母且 <16 字 → 視為假 key（2026-09-25 實測：某 LiteLLM proxy 的 dummy key 為
        # 「8 字母 + ./-」形狀，13 字且無數字；真憑證幾乎都含數字或更長）。
        # 註：**仍然攔**含數字者與 ≥16 字者 ⇒ `POSTGRES_PASSWORD=<19 字含數字>` 照樣被抓 ✓。
        if not any(ch.isdigit() for ch in value) and len(value) < 16:
            return True
        return False
    if name == "env_default_secret_literal":
        return "/" in matched or "\\" in matched   # `/app/configs`、`C:\\x` 這類路徑預設值不是機密
    return False

files = tracked_files()
block_hits, warn_hits, skipped_bin = [], [], 0
for rel in files:
    if matches(rel, SKIP_GLOBS):
        continue
    p = os.path.join(root, rel)
    try:
        with open(p, "rb") as fh:
            raw = fh.read()
    except OSError:
        continue
    if b"\0" in raw[:8192]:          # 二進位
        skipped_bin += 1
        continue
    try:
        text = raw.decode("utf-8", "replace")
    except Exception:
        continue
    warn_only = matches(rel, WARN_ONLY_GLOBS)
    # 例外：可部署的設定檔（*.yml/*.yaml，非範例/模板）→ block 級，即使路徑命中 warn-only glob。
    if matches(rel, CONFIG_GLOBS) and not matches(rel, CONFIG_EXAMPLE_GLOBS):
        warn_only = False
    for lineno, line in enumerate(text.splitlines(), 1):
        if "secret-scan-allow" in line:      # 行內豁免（須寫理由）
            continue
        for name, rx in PATTERNS:
            m = rx.search(line)
            if not m:
                continue
            if name in NOISE_FILTERED and synthetic(name, m.group(0)):
                continue
            if allowed(rel, name):
                continue
            (warn_hits if warn_only else block_hits).append((rel, lineno, name, mask(m.group(0))))

def show(hits, tag):
    if quiet:
        return
    for rel, lineno, name, masked in hits:
        print(f"  {tag} {rel}:{lineno}  [{name}]  {masked}")

print(f"secret-scan: 掃描 {len(files)} 個 tracked 檔"
      f"（跳過二進位 {skipped_bin}；warn-only 類別命中不擋 CI）")
if block_hits:
    print(f"❌ 阻擋級命中 {len(block_hits)} 筆：")
    show(block_hits, "❌")
if warn_hits:
    print(f"⚠️  文件/範例類命中 {len(warn_hits)} 筆（{'--strict 視為失敗' if strict else '不擋 CI；請確認是範例'}）：")
    show(warn_hits, "⚠️ ")
if not block_hits and not warn_hits:
    print("  ✅ 無命中")

if block_hits or (strict and warn_hits):
    print("→ 處置：移除機密並**輪替憑證**（歷史 commit 仍看得到）；範例請用 `secret-scan-allow` 或 allowlist 並寫理由。")
    sys.exit(1)
print("✅ secret-scan PASS")
PY
