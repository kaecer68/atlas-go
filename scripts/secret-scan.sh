#!/usr/bin/env bash
# secret-scan.sh — 掃「tracked 檔」的機密樣式（public repo 護欄）
#
# ⚠️ 本檔是 **a2a-dev `scripts/secret-scan.sh` 的整檔副本**（SSOT 在那邊）。
#    上游版本: a2a-dev @ feat/ops-hardening-g 8c8145d, sha256 da728f43d87c46c19e11ff2e43cdce7f6d85df0babc9396211c58f133f480729
#    為何用「副本」而不是「引用上游」：① 本 repo 是 **PUBLIC**，CI 不該為了掃描去 clone 另一個 repo
#    （多一條網路依賴 + 供應鏈面）② 兩個 repo 都要能在**離線**狀態跑完 CI ③ 副本同步成本極低（整檔覆蓋）。
#    同步方式：`cp <a2a-dev>/scripts/secret-scan.sh scripts/secret-scan.sh` 後更新上面的 sha256。
#    漂移偵測：兩份 sha256 不同即代表需要人工確認（本行即為錨點）。
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
]

# ── 路徑類別 ──────────────────────────────────────────────────────────
# warn-only = 文件／範例／模板：寫範例 token 是正當用途，預設不讓 CI 失敗。
WARN_ONLY_GLOBS = [
    "*.md", "*.markdown", "*.txt", "*.rst", "*.json.md",
    "docs/*", "*/docs/*",
    "*.example", ".env.example", "*.example.*", "*.sample", "*sample*", "*.template", "*template*",
    "*.json.sample", "*fixtures*",
]
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
    for lineno, line in enumerate(text.splitlines(), 1):
        if "secret-scan-allow" in line:      # 行內豁免（須寫理由）
            continue
        for name, rx in PATTERNS:
            m = rx.search(line)
            if not m:
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
