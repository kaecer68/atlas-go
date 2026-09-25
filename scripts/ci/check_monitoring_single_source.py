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

三條規則
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

已知限制（誠實列出）
--------------------
* R2 用**行掃描**找 compose 的 bind mount（`- src:dst[:ro]` 與 `- source: <path>` 兩種形式），
  沒有引入 YAML 依賴（GitHub runner 不保證有 PyYAML）。若未來有人用其他寫法（例如
  自訂 anchor/merge key 後再改寫），可能漏抓 ⇒ 生產外側仍有 drift-check [9/9] 做第二道。
* R3 是「散文層」啟發式標記比對，不是語意分析；正常情況下反例警示一定帶 ✗/不要/歷史 等字。

用法:
    python3 scripts/ci/check_monitoring_single_source.py                 # 掃 repo
    python3 scripts/ci/check_monitoring_single_source.py --root DIR      # 掃指定目錄（測試用）
exit code: 0 = PASS；1 = 有違規；2 = 用法錯誤
"""
import argparse
import os
import re
import subprocess
import sys

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

    if not args.quiet:
        print("monitoring-single-source: 掃描 %d 個 tracked 檔（root=%s）" % (len(files), root))
        print("  R1 唯一設定樹：%s" % ("OK" if r1 == 0 else "%d 筆違規" % r1))
        print("  R2 compose 掛載源：%s" % ("OK" if r2 == 0 else "%d 筆違規" % r2))
        print("  R3 舊樹路徑引用：%s" % ("OK" if r3 == 0 else "%d 筆違規" % r3))
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
