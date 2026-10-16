#!/usr/bin/env python3
"""check_monitoring_single_source.py — 「監控設定只有一棵權威樹」的 PR 階段斷言。

為什麼要有它（2026-08-27 實證事故 + 2026-09-25 盤查）
----------------------------------------------------
atlas 的 Prometheus / Alertmanager / Grafana 設定曾有**兩棵樹**：
  * `monitoring/`（repo 內，權威 ✓，生產容器實際掛載的就是它）
  * `~/workspace/atlas-monitoring/`（iMac 時代殘留 ✗，`rules/` 只有 5 檔 vs repo 7 檔）
2026-08-27 有人把「補 target-down 規則」的修法寫進**錯的樹** ⇒ 生產 Prometheus
**27 天沒有載入該規則**（2026-08-27 進版控 → 2026-09-23T08:44:14Z 才第一次被評估）。

a2a-dev `scripts/drift-check.sh [9/9]` 已在**生產外側**用 `docker inspect` 斷言掛載源；
本檢查補上**PR 階段**（不需要 docker、不需要生產）就攔下同一類錯誤的能力。

四條規則
--------
R1 唯一設定樹：basename 為 prometheus.yml / alertmanager.yml / otel-collector.yaml 的 tracked 檔
              只准出現在 `monitoring/`；`*/rules/*.yml|yaml` 只准出現在 `monitoring/rules/`。
R2 掛載源落點：解析 tracked 的 `docker-compose*.yml` 之 bind mount source，凡引用監控設定者，
              其路徑必須含 `/monitoring/`（或開頭就是 `monitoring/`）且該檔在 repo 內**存在**，
              前綴只准是 repo 本身（`./`、`${HOME}/workspace/<repo>/`、`~/workspace/<repo>/`…）。
              ⇒ 任何指向 `atlas-monitoring/…` 或 repo 外路徑的掛載一律 FAIL。
R3 舊樹路徑：tracked 檔出現 `atlas-monitoring/` 路徑時，該行必須帶「反例/警示」標記
              （✗ / 不是 / 不要 / 勿 / 歷史 / 已退役 / RETIRED / deprecated / never …），
              否則 FAIL（= 有人又在把 agent/人往錯的樹帶）。
R4 runbook_url 目標：`monitoring/rules/**.yml` 內的 `runbook_url` 必須指到**存在且可判定**的目標
              （issue FU-20260927-04/05；2026-10-16）。
              · FAIL：值不是 http(s) URL／host 不合法／指向**本 repo** 的 `/blob/<ref>/<path>`
                而 `<path>` 不存在或是**目錄**（`blob/main/` 結尾斜線）／指向本 repo 的其他形狀
                （`/issues/N`、`/tree/…`）。
              · WARN（不影響 rc）：外部 host 不在白名單內、in-repo 目標不是 `.md`、
                in-repo blob 的 `<ref>` 不是 `main`。
              設計取捨（白名單可設定）：外部 URL **不做 DNS／HTTP 檢查**——CI 連外會製造新的
              flake（本 repo 已有 `check_jev_contract.sh` 連外服務一 flake 就擋合法 push 的實證，
              見 FU-20260926-21）。因此未知外部 host 預設只 **warn**；要把某一類 host 收緊成
              FAIL，有兩條路（都不需改程式）：
                ① `scripts/ci/monitoring-runbook-allowlist.json` 的 `"external_policy": "fail"`
                ② CLI `--strict-external`
              白名單檔本身可設定 `external_hosts`（清單）；**未列在清單且不在本 repo** 的 URL
              就是上面那條 warn/fail 的對象。白名單檔不存在 = 空清單 + `warn`（fail-open 於
              外部、仍然對 in-repo 目標 fail-closed）。

已知限制（誠實列出）
--------------------
* R2 用**行掃描**找 compose 的 bind mount（`- src:dst[:ro]` 與 `- source: <path>` 兩種形式），
  沒有引入 YAML 依賴（GitHub runner 不保證有 PyYAML）。若未來有人用其他寫法（例如
  自訂 anchor/merge key 後再改寫），可能漏抓 ⇒ 生產外側仍有 drift-check [9/9] 做第二道。
* R3 是「散文層」啟發式標記比對，不是語意分析；正常情況下反例警示一定帶 ✗/不要/歷史 等字。
* R4 只掃 `monitoring/rules/`（權威樹的**執行期**規則檔）。`monitoring/tests/*_test.yml`
  的 `runbook_url` 是 promtool fixture 的資料，promtool 不驗證 annotation ⇒ 刻意不納入。
  R4 只驗「目標存在、形狀正確」，**不驗**「目標內容真的涵蓋該告警的處置」——後者不可能離線判定。

用法:
    python3 scripts/ci/check_monitoring_single_source.py                 # 掃 repo
    python3 scripts/ci/check_monitoring_single_source.py --root DIR      # 掃指定目錄（測試用）
    python3 scripts/ci/check_monitoring_single_source.py --strict-external  # 未知外部 host 由 warn 升為 FAIL
exit code: 0 = PASS；1 = 有違規；2 = 用法錯誤
"""
import argparse
import json
import os
import re
import subprocess
import sys
from urllib.parse import urlsplit

MON_BASENAMES = {"prometheus.yml", "alertmanager.yml", "otel-collector.yaml"}
MON_ARTIFACT = re.compile(r"(?:^|/)(?:prometheus\.ya?ml|alertmanager\.ya?ml|otel-collector\.ya?ml)$|/(?:rules|grafana)$|/(?:rules|grafana)/")
LEGACY_PATH = re.compile(r"atlas-monitoring/")
WARNING_MARKERS = ("✗", "不是", "不要", "勿", "不得", "歷史", "已退役", "RETIRED", "retired",
                   "historical", "legacy", "deprecated", "never", "do not", "don't", "非生產",
                   "殘留")
# R2 允許的前綴（monitoring/ 之前的部分）——只准指向 repo 自己。
def allowed_prefix(prefix, reponame):
    """monitoring/ 之前的來源前綴是否指向「本 repo 的一份 checkout」。

    ⚠️ 刻意**不**比對目錄 basename：canonical 部署 checkout 是 `~/workspace/atlas`，而
    GitHub Actions 的 checkout 目錄是 `atlas-go`（repo 名）——用名字比對會在 CI 誤判。
    真正的錨點是後面「`monitoring/<tail>` 必須存在於本 repo」的存在性檢查。
    """
    p = prefix.rstrip("/")
    if p in ("", ".", "./", reponame):
        return True
    if re.fullmatch(r"(?:~|\$HOME|\$\{HOME\})/workspace/[^/]+", p):
        return True
    if re.fullmatch(r"/(?:Users|home)/[^/]+/workspace/[^/]+", p):
        return True
    if re.fullmatch(r"\$\{ATLAS_REPO(?:-|:-)[^}]*\}", p):
        return True
    return False

# R3 不掃「護欄自己」：本檢查的規則定義、薄殼與 fixture 必然含有被禁止的字面值
# （偵測樣式本身、以及刻意造出來的反例）。清單寫死並具名，避免變成「誰都能豁免」的漏洞。
SELF_FILES = {
    "scripts/ci/check_monitoring_single_source.py",
    "scripts/ci/check_monitoring_single_source.sh",
    "tests/scripts/test-monitoring-single-source.sh",
}

# `- SRC:DST[:ro|rw]`（bind mount 短形式）。DST 必須以 `/` 開頭才捕獲，避免吃到 volume 名。
COMPOSE_LINE = re.compile(r"^\s*-\s*(\S+?):(/[^\s:]+)(?::(?:ro|rw))?\s*$")
# 長形式 `source: <path>`（通常與 `target:` 成對）
COMPOSE_SOURCE_KEY = re.compile(r"^\s*(?:-\s*)?source:\s*(\S+)\s*$")
# 容器內的監控設定掛載點：只要目標落在這裡，來源就必須是 repo 的 monitoring/
DST_GUARDED = ("/etc/prometheus", "/etc/alertmanager", "/etc/grafana", "/var/lib/grafana")

# ── R4 runbook_url 目標存在性（FU-20260927-04/05）────────────────────────────
# R4 的掃描範圍：權威樹的執行期規則檔。`monitoring/tests/*_test.yml` 是 promtool
# fixture（promtool 不驗證 annotation）⇒ 刻意不收（見檔頭「已知限制」）。
R4_RULE_DIR = "monitoring/rules/"
R4_RULE_EXTS = (".yml", ".yaml")
# `runbook_url: "…"` / `runbook_url: …`（值可帶引號；引號外可帶行內註解）。
RUNBOOK_LINE = re.compile(r"""^\s*runbook_url\s*:\s*(.*)$""")
# 本 repo 的檔案 URL 三種形狀：/blob/<ref>/<path>（檔案）、/tree/<ref>/<path>（目錄）、其他。
GH_FILE_URL = re.compile(
    r"^https?://(?:www\.)?github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/"
    r"(?P<kind>blob|tree|raw)/(?P<ref>[^/]+)/(?P<path>.*)$")
GH_ANY_URL = re.compile(
    r"^https?://(?:www\.)?github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/(?P<rest>.*)$")
HOSTNAME_OK = re.compile(r"^(?=.{1,253}$)[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$")
# 白名單檔（可設定）：`external_hosts` 清單 + `external_policy`（warn|fail）。
RUNBOOK_ALLOWLIST_REL = "scripts/ci/monitoring-runbook-allowlist.json"
# 「本 repo」的 slug。以 `git remote get-url origin` 推導為主（CI checkout 目錄名是
# `atlas-go`、本機 worktree 名是 `fix-…` ⇒ **不能**用目錄 basename 比對）；推導不到
# （fixture tempdir、沒有 remote）才用這個常數。常數與實際 slug 不同時的失效方向是
# 「自家 URL 被當成外部」⇒ 只 warn、不 FAIL（安全的方向），且訊息會直說 slug 不符。
SELF_REPO_SLUGS = ("kaecer68/atlas-go",)
# source of record：白名單只影響**外部** URL；in-repo 目標永遠 fail-closed。
DEFAULT_EXTERNAL_POLICY = "warn"


def load_runbook_allowlist(root):
    """回傳 (hosts:set, policy:str).

    檔不存在 / 壞掉 / 欄位型別錯 ⇒ 空清單 + warn（fail-open 於外部判定，並且**明示**
    印出原因；不是靜默）。刻意不讓「白名單檔壞掉」把閘門變成紅燈：它管的是**外部**
    目標，而外部判定本來就不是離線可證的命題。
    """
    path = os.path.join(root, RUNBOOK_ALLOWLIST_REL)
    hosts, policy, note = set(), DEFAULT_EXTERNAL_POLICY, ""
    if not os.path.exists(path):
        return hosts, policy, ""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError) as exc:
        return hosts, policy, "白名單檔無法解析（%s）⇒ 視為空清單 + %s" % (type(exc).__name__, policy)
    raw_hosts = data.get("external_hosts", [])
    if isinstance(raw_hosts, list):
        hosts = {str(h).strip().lower() for h in raw_hosts if str(h).strip()}
    else:
        note = "白名單 external_hosts 不是清單 ⇒ 視為空清單"
    raw_policy = str(data.get("external_policy", DEFAULT_EXTERNAL_POLICY)).strip().lower()
    if raw_policy in ("warn", "fail"):
        policy = raw_policy
    else:
        note = note or ("白名單 external_policy 值無法識別（%s）⇒ 用 %s" % (raw_policy, policy))
    return hosts, policy, note


def parse_runbook_value(raw):
    """把 `runbook_url:` 之後的字串整理成 URL 字面值。

    規則（刻意寬鬆地只處理 YAML 的兩種實際寫法）：
      · 以引號開頭 ⇒ 取到配對的收尾引號（引號內允許 `#`，例如 `...#section`）。
      · 否則 ⇒ 砍掉空白後的 `#` 之後（YAML 行內註解）。
    刻意**不**引入 YAML 依賴：理由與 R2 相同（GitHub runner 不保證有 PyYAML）。
    """
    v = raw.strip()
    if not v:
        return ""
    if v[0] in ("\"", "'"):
        quote = v[0]
        end = v.find(quote, 1)
        return (v[1:end] if end != -1 else v[1:]).strip()
    for marker in (" #", "\t#"):
        idx = v.find(marker)
        if idx != -1:
            v = v[:idx]
    return v.strip()


def repo_file_verdict(root, path):
    """本 repo 相對路徑的存在性判定。回傳 (ok:bool, why:str)。"""
    rel = path.split("#", 1)[0].split("?", 1)[0].strip().lstrip("/")
    if rel == "":
        return False, "URL 沒有給檔案路徑（只指到目錄或空路徑）"
    full = os.path.join(root, rel)
    if os.path.isdir(full):
        return False, "目標是目錄（blob URL 必須指向檔案）：" + rel
    if not os.path.isfile(full):
        return False, "目標檔案在 repo 內不存在：" + rel
    return True, rel


def self_repo_slugs(root):
    """回傳「視為本 repo」的 `owner/repo` 集合（含常數；remote 推導成功時一併加入）。"""
    slugs = {s.lower() for s in SELF_REPO_SLUGS}
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    try:
        out = subprocess.run(["git", "-C", root, "remote", "get-url", "origin"],
                             capture_output=True, text=True, timeout=30, env=env)
        if out.returncode == 0:
            remote = out.stdout.strip()
            m = re.search(r"github\.com[:/]+([^/]+)/([^/]+?)(?:\.git)?$", remote)
            if m:
                slugs.add((m.group(1) + "/" + m.group(2)).lower())
    except Exception:
        pass
    return slugs


def tracked_files(root):
    """tracked 檔優先（會公開的東西）；非 git 目錄（測試 fixture）→ find 走訪。"""
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    try:
        out = subprocess.run(["git", "-C", root, "ls-files", "-z"],
                             capture_output=True, text=True, timeout=60, env=env)
        if out.returncode == 0 and out.stdout.strip("\0"):
            listed = [p for p in out.stdout.split("\0") if p]
            inside = [p for p in listed if os.path.lexists(os.path.join(root, p))]
            if inside:
                return inside
    except Exception:
        pass
    found = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "__pycache__")]
        for fn in filenames:
            found.append(os.path.relpath(os.path.join(dirpath, fn), root))
    return sorted(found)


def read_text(path):
    try:
        with open(path, "rb") as fh:
            raw = fh.read()
    except OSError:
        return None
    if b"\0" in raw[:8192]:
        return None
    return raw.decode("utf-8", "replace")


def main():
    ap = argparse.ArgumentParser(add_help=True)
    ap.add_argument("--root", default=None)
    ap.add_argument("--quiet", action="store_true")
    ap.add_argument("--strict-external", action="store_true",
                    help="把「外部 host 不在白名單內」由 warning 升為 FAIL（預設只 warn）")
    args = ap.parse_args()

    root = os.path.abspath(args.root or os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
    if not os.path.isdir(root):
        print("❌ --root 不存在: " + root)
        return 2
    reponame = os.path.basename(root.rstrip("/"))

    files = tracked_files(root)
    violations = []

    # ── R1 唯一設定樹 ────────────────────────────────────────────────
    r1 = 0
    for rel in files:
        base = os.path.basename(rel)
        if base in MON_BASENAMES and not rel.startswith("monitoring/"):
            violations.append(("R1", rel, "監控設定檔不在 monitoring/ 底下：" + rel))
            r1 += 1
        if re.search(r"(?:^|/)rules/[^/]+$", rel) and not rel.startswith("monitoring/rules/"):
            violations.append(("R1", rel, "規則檔不在 monitoring/rules/ 底下：" + rel))
            r1 += 1

    # ── R2 compose 掛載源必須落在 repo 的 monitoring/ ────────────────
    r2 = 0
    for rel in files:
        base = os.path.basename(rel)
        if not (base.startswith("docker-compose") and base.endswith((".yml", ".yaml"))):
            continue
        txt = read_text(os.path.join(root, rel))
        if txt is None:
            continue
        for lineno, line in enumerate(txt.splitlines(), 1):
            m = COMPOSE_LINE.match(line)
            dst = m.group(2) if m else ""
            if not m:
                m = COMPOSE_SOURCE_KEY.match(line)
                if not m:
                    continue
            src = m.group(1)
            if "/" not in src:
                continue                      # 具名 volume（grafana-data/prometheus-data…）
            # 監控設定掛載的判定：來源路徑像監控設定檔，**或**目標落在受守護的設定目錄
            if not (MON_ARTIFACT.search(src) or dst.startswith(DST_GUARDED)):
                continue
            mm = re.search(r"(?:^|/)monitoring/(.+)$", src)
            if not mm:
                violations.append(("R2", rel + ":" + str(lineno),
                                   "掛載源不在 monitoring/ 底下（疑似指向第二棵樹）：" + src))
                r2 += 1
                continue
            prefix = src[: mm.start(1) - len("monitoring/")]
            tail = mm.group(1)
            if not allowed_prefix(prefix, reponame):
                violations.append(("R2", rel + ":" + str(lineno),
                                   "掛載源前綴不是本 repo：" + src))
                r2 += 1
                continue
            if not os.path.exists(os.path.join(root, "monitoring", tail)):
                violations.append(("R2", rel + ":" + str(lineno),
                                   "掛載源指向不存在的檔案：monitoring/" + tail))
                r2 += 1

    # ── R3 不得引用舊樹路徑（除非該行是反例警示）────────────────────
    r3 = 0
    for rel in files:
        if rel in SELF_FILES:
            continue
        txt = read_text(os.path.join(root, rel))
        if txt is None:
            continue
        for lineno, line in enumerate(txt.splitlines(), 1):
            if not LEGACY_PATH.search(line):
                continue
            if any(mk in line for mk in WARNING_MARKERS):
                continue
            violations.append(("R3", rel + ":" + str(lineno),
                               "引用舊監控樹路徑但沒有反例警示標記 ✗（會把 agent 帶去錯的樹）"))
            r3 += 1

    # ── R4 runbook_url 指向存在且可判定的目標（FU-20260927-04/05）────────
    r4 = 0
    r4_warnings = []
    self_slugs = self_repo_slugs(root)
    allow_hosts, allow_policy, allow_note = load_runbook_allowlist(root)
    if args.strict_external:
        allow_policy = "fail"
    if allow_note:
        r4_warnings.append(("<白名單>", allow_note))
    for rel in files:
        if not rel.startswith(R4_RULE_DIR) or not rel.endswith(R4_RULE_EXTS):
            continue
        txt = read_text(os.path.join(root, rel))
        if txt is None:
            continue
        for lineno, line in enumerate(txt.splitlines(), 1):
            m = RUNBOOK_LINE.match(line)
            if not m:
                continue
            where = rel + ":" + str(lineno)
            value = parse_runbook_value(m.group(1))
            if not value:
                violations.append(("R4", where, "runbook_url 沒有目標（空值）"))
                r4 += 1
                continue
            parts = urlsplit(value)
            if parts.scheme not in ("http", "https") or not parts.netloc:
                violations.append(("R4", where, "runbook_url 不是 http(s) 絕對 URL：" + value))
                r4 += 1
                continue
            host = (parts.hostname or "").lower()
            if not host or not HOSTNAME_OK.match(host):
                violations.append(("R4", where, "runbook_url 的 host 不合法：" + value))
                r4 += 1
                continue
            gh_any = GH_ANY_URL.match(value)
            gh_file = GH_FILE_URL.match(value)
            in_this_repo = bool(gh_any) and ((gh_any.group("owner") + "/" + gh_any.group("repo")).lower() in self_slugs)
            if in_this_repo:
                # 本 repo 的目標**離線可證** ⇒ 一律 fail-closed。
                if not gh_file or gh_file.group("kind") != "blob":
                    violations.append(("R4", where,
                                       "指向本 repo 但不是檔案 URL（必須是 /blob/<ref>/<path>，"
                                       "tree／目錄／issue 連結不算）：" + value))
                    r4 += 1
                    continue
                ok, detail = repo_file_verdict(root, gh_file.group("path"))
                if not ok:
                    violations.append(("R4", where, detail + "（" + value + "）"))
                    r4 += 1
                    continue
                if gh_file.group("ref") != "main":
                    r4_warnings.append((where, "blob ref 不是 main（分支被刪就變死鏈）：" + value))
                if not detail.endswith(".md"):
                    r4_warnings.append((where, "目標不是 .md（runbook 應是可讀散文）：" + detail))
                continue
            if host in allow_hosts:
                continue
            if "." not in host:
                violations.append(("R4", where,
                                   "外部 host 不是完整網域名稱且不在白名單內：" + host))
                r4 += 1
                continue
            msg = "外部 host 不在白名單內（%s；policy=%s）：%s" % (host, allow_policy, value)
            if allow_policy == "fail":
                violations.append(("R4", where, msg))
                r4 += 1
            else:
                r4_warnings.append((where, msg))

    if not args.quiet:
        print("monitoring-single-source: 掃描 %d 個 tracked 檔（root=%s）" % (len(files), root))
        print("  R1 唯一設定樹：%s" % ("OK" if r1 == 0 else "%d 筆違規" % r1))
        print("  R2 compose 掛載源：%s" % ("OK" if r2 == 0 else "%d 筆違規" % r2))
        print("  R3 舊樹路徑引用：%s" % ("OK" if r3 == 0 else "%d 筆違規" % r3))
        print("  R4 runbook_url 目標：%s（%d 筆警告）"
              % ("OK" if r4 == 0 else "%d 筆違規" % r4, len(r4_warnings)))
        print("     （外部 host 白名單 policy=%s；`--strict-external` 可升為 FAIL）" % allow_policy)
    # 警告**不因 --quiet 而消失**：warn 是本規則唯一的可見管道，安靜的警告等於沒有警告。
    for where, why in r4_warnings:
        print("  ⚠️ [R4-W] %s — %s" % (where, why))
    if not args.quiet:
        for rule, where, why in violations:
            print("  ❌ [%s] %s — %s" % (rule, where, why))
        if violations:
            print("→ 權威樹 = repo 的 `monitoring/`（生產容器實際掛載的就是它）。")
            print("  舊樹 `~/workspace/atlas-monitoring/` 是 iMac 時代殘留，改它等於改沒人讀的檔案。")
            print("  歷史事故與時間線：a2a-dev docs/operations/atlas-monitoring-gap-20260925.md")
    if violations:
        print("❌ monitoring-single-source FAIL（%d 筆）" % len(violations))
        return 1
    print("✅ monitoring-single-source PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
