#!/usr/bin/env bash
# test-monitoring-single-source.sh — 「監控設定只有一棵權威樹」檢查的自我測試（hermetic）
#
# 為什麼要有它：護欄本身必須有牙齒。這條檢查若只會印 PASS，就等於沒有——
# 2026-09-25（任務 P）的第一版 drift-check [9/9] 就是這樣抓到 false-green 的。
#
# 本測試在 tempdir 造 fixture，用 `--root` 掃（不碰真 repo、不碰生產、不需要 docker）：
#   P 正向：乾淨樹 → PASS
#   N 負向：舊樹掛載 / repo 外掛載 / 第二棵設定樹 / rules 跑到 monitoring 外 /
#           散文把 agent 帶去舊樹 / 掛載源指向不存在的檔 / 受守護目標卻給非 monitoring 來源 → 必 FAIL
#   R4（FU-20260927-04/05）：runbook_url 目標存在性 —— 缺檔／目錄／tree／非 http ／非完整 host → FAIL；
#           未知外部 host（warn 預設）＋ 非 .md 目標 → PASS 但**必須有 [R4-W] 警告**；
#           白名單命中 → 無警告；--strict-external → 未知外部 host 升為 FAIL。
#
# ⚠️ 本檔內的舊樹字串一律用「相鄰字串相接」（LEGACY_TREE）組出，讓測試檔**本身**
#    不含可被自己命中的 `atlas-monitoring/` 字面值（否則 R3 會掃到自己 ⇒ CI 假紅）。
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$ROOT/scripts/ci/check_monitoring_single_source.py"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

LEGACY_TREE="atlas-""monitoring"          # 舊樹目錄名（不在本檔形成可命中的字面值）

# 乾淨基底（每次重建）
base() {
  rm -rf "${TMP}/fx"; mkdir -p "${TMP}/fx/monitoring/rules"
  printf 'global:\n  scrape_interval: 15s\n'            > "${TMP}/fx/monitoring/prometheus.yml"
  printf 'route:\n  receiver: x\n'                      > "${TMP}/fx/monitoring/alertmanager.yml"
  printf 'groups: []\n'                                 > "${TMP}/fx/monitoring/rules/a.yml"
  cat > "${TMP}/fx/docker-compose.yml" <<'EOF'
services:
  prometheus:
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./monitoring/rules:/etc/prometheus/rules:ro
EOF
}
run_case() {   # $1=名稱 $2=期望 exit
  python3 "${CHECK}" --root "${TMP}/fx" --quiet > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" = "$2" ]; then ok "$1（exit=${rc}）"
  else bad "$1（exit=${rc}，期望 $2）"; sed 's/^/     /' "${TMP}/out.txt" | head -6; fi
}

# run_case_needle <名稱> <期望 exit> <輸出必須含> [額外參數…]
# 警告一律走 stdout（不受 --quiet 影響），所以這裡沿用 --quiet 仍可斷言 [R4-W]。
run_case_needle() {
  local name="$1" want="$2" needle="$3"; shift 3
  python3 "${CHECK}" --root "${TMP}/fx" --quiet "$@" > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" != "${want}" ]; then
    bad "${name}（exit=${rc}，期望 ${want}）"; sed 's/^/     /' "${TMP}/out.txt" | head -6; return
  fi
  if ! grep -qF -- "${needle}" "${TMP}/out.txt"; then
    bad "${name}（rc 正確但輸出未含 '${needle}'）"; sed 's/^/     /' "${TMP}/out.txt" | head -6; return
  fi
  ok "${name}（exit=${rc}，輸出含 '${needle}'）"
}

# run_case_absent <名稱> <期望 exit> <輸出必須「不」含> [額外參數…]
run_case_absent() {
  local name="$1" want="$2" needle="$3"; shift 3
  python3 "${CHECK}" --root "${TMP}/fx" --quiet "$@" > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" != "${want}" ]; then
    bad "${name}（exit=${rc}，期望 ${want}）"; sed 's/^/     /' "${TMP}/out.txt" | head -6; return
  fi
  if grep -qF -- "${needle}" "${TMP}/out.txt"; then
    bad "${name}（輸出不應含 '${needle}'）"; sed 's/^/     /' "${TMP}/out.txt" | head -6; return
  fi
  ok "${name}（exit=${rc}，輸出不含 '${needle}'）"
}

# rule_fixture <runbook_url 值>：乾淨樹 ＋ `monitoring/rules/r4.yml` 一條帶 runbook_url 的規則
# ＋ `docs/operations/runbook.md`（存在的 in-repo 目標）。
R4_REPO="https://github.com/kaecer68/atlas-go/blob/main"
rule_fixture() {
  base
  mkdir -p "${TMP}/fx/docs/operations"
  printf '# runbook
' > "${TMP}/fx/docs/operations/runbook.md"
  cat > "${TMP}/fx/monitoring/rules/r4.yml" <<EOF
groups:
  - name: r4
    rules:
      - alert: R4Fixture
        annotations:
          runbook_url: "${1}"
EOF
}

echo "── monitoring 單一設定樹檢查自我測試 ──"

# ── 正向 ──────────────────────────────────────────────────────────
base; run_case "P1 乾淨樹 PASS" 0
base
mkdir -p "${TMP}/fx/monitoring/grafana/dashboards"
printf '{}\n' > "${TMP}/fx/monitoring/grafana/dashboards/d.json"
cat > "${TMP}/fx/docker-compose.vol.yml" <<'EOF'
services:
  prometheus:
    volumes:
      - prometheus-data:/prometheus
  grafana:
    volumes:
      - grafana-data:/var/lib/grafana
      - ./monitoring/grafana/dashboards:/var/lib/grafana/dashboards:ro
EOF
run_case "P2 具名 volume 不誤擋 PASS" 0
base
printf '⚠️ ~/workspace/%s/ 是歷史殘留 ✗ 不要用\n' "${LEGACY_TREE}" > "${TMP}/fx/docs-note.md"
run_case "P3 帶警示標記的舊樹提及 PASS" 0

# ── 負向 ──────────────────────────────────────────────────────────
base
mkdir -p "${TMP}/fx/docs/operations"
printf 'services:\n  prometheus:\n    volumes:\n      - ~/workspace/%s/prometheus.yml:/etc/prometheus/prometheus.yml:ro\n' "${LEGACY_TREE}" > "${TMP}/fx/docs/operations/docker-compose.prod.yml"
run_case "N1 掛載指回舊樹 FAIL" 1

base
printf 'services:\n  prometheus:\n    volumes:\n      - /tmp/foo/monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro\n' > "${TMP}/fx/docker-compose.extra.yml"
run_case "N2 掛載在 repo 外 FAIL" 1

base
mkdir -p "${TMP}/fx/monitoring2"; printf 'global: {}\n' > "${TMP}/fx/monitoring2/prometheus.yml"
run_case "N3 出現第二棵設定樹檔 FAIL" 1

base
mkdir -p "${TMP}/fx/other/rules"; printf 'groups: []\n' > "${TMP}/fx/other/rules/x.yml"
run_case "N4 rules 跑到 monitoring 外 FAIL" 1

base
printf '請改 ~/workspace/%s/rules/ 的檔案\n' "${LEGACY_TREE}" > "${TMP}/fx/docs-note.md"
run_case "N5 散文把 agent 帶去舊樹（無警示）FAIL" 1

base
printf 'services:\n  p:\n    volumes:\n      - ./monitoring/nope.yml:/etc/prometheus/prometheus.yml:ro\n' > "${TMP}/fx/docker-compose.missing.yml"
run_case "N6 掛載源指向不存在的檔 FAIL" 1

base
printf 'services:\n  p:\n    volumes:\n      - ./configs/prom.yml:/etc/prometheus/prometheus.yml:ro\n' > "${TMP}/fx/docker-compose.n7.yml"
run_case "N7 受守護目標卻給非 monitoring 來源 FAIL" 1

# ── R4：runbook_url 目標存在性（FU-20260927-04/05）─────────────────────
rule_fixture "${R4_REPO}/docs/operations/runbook.md"
run_case "R4-P1 in-repo 目標存在且是 .md PASS" 0

rule_fixture "${R4_REPO}/docs/operations/nope.md"
run_case "R4-N1 in-repo 目標檔案不存在 FAIL" 1

rule_fixture "${R4_REPO}/docs/operations/"
run_case "R4-N2 in-repo 目標是目錄（blob/main/docs/operations/）FAIL" 1

rule_fixture "${R4_REPO}/tree/main/docs/operations/runbook.md"
run_case "R4-N3 tree 形狀（目錄 URL）FAIL" 1

rule_fixture "https://github.com/kaecer68/atlas-go/issues/1"
run_case "R4-N4 本 repo 的 issue 連結 FAIL" 1

rule_fixture "file:///etc/passwd"
run_case "R4-N5 非 http(s) scheme FAIL" 1

rule_fixture "docs/operations/runbook.md"
run_case "R4-N6 沒有 scheme（相對路徑）FAIL" 1

rule_fixture "http://wiki/runbooks/x"
run_case "R4-N7 host 不是完整網域名稱且不在白名單 FAIL" 1

rule_fixture "https://runbooks.example.invalid/x"
run_case_needle "R4-W1 未知外部 host → PASS 但必須有 [R4-W]" 0 "[R4-W]"
run_case_needle "R4-W1b 同一個 fixture 加 --strict-external → FAIL" 1 "monitoring-single-source FAIL" --strict-external

rule_fixture "${R4_REPO}/monitoring/prometheus.yml"
run_case_needle "R4-W2 in-repo 但目標不是 .md → PASS ＋ [R4-W]" 0 "[R4-W]"

# allowlist <json 內容>：把白名單檔寫進 fixture（目錄由 rule_fixture 的 base 清掉 ⇒ 這裡重建）。
allowlist() {
  mkdir -p "${TMP}/fx/scripts/ci"
  printf '%s\n' "$1" > "${TMP}/fx/scripts/ci/monitoring-runbook-allowlist.json"
}

rule_fixture "${R4_REPO}/docs/operations/runbook.md"
allowlist '{"external_hosts": ["runbooks.example.invalid"], "external_policy": "fail"}'
run_case_absent "R4-W3 白名單檔 policy=fail 但只有 in-repo 目標 → PASS、無 [R4-W]" 0 "[R4-W]"

rule_fixture "https://runbooks.example.invalid/x"
allowlist '{"external_hosts": [], "external_policy": "fail"}'
run_case "R4-N8 白名單 policy=fail ＋ 未知外部 host → FAIL" 1

rule_fixture "https://runbooks.example.invalid/x"
allowlist '{"external_hosts": ["runbooks.example.invalid"], "external_policy": "warn"}'
run_case_absent "R4-P2 白名單命中 → PASS 且無 [R4-W]" 0 "[R4-W]"

# mutation 自證：把 R4 的判定停用 ⇒ R4-N2（目錄）必須由「1」變成「0」。
rule_fixture "${R4_REPO}/docs/operations/"
cp "${CHECK}" "${TMP}/check-orig.py"
python3 - "${TMP}/check-mut.py" "${CHECK}" <<'PY'
import sys
dst, src = sys.argv[1], sys.argv[2]
s = open(src, encoding="utf-8").read()
s2 = s.replace('violations.append(("R4"', 'None and violations.append(("R4"')
if s2 == s:
    sys.exit("mutation 目標（R4 的 violations.append）找不到 —— 檢查被重構過，請更新本測試")
open(dst, "w", encoding="utf-8").write(s2)
PY
CHECK_SAVED="${CHECK}"; CHECK="${TMP}/check-mut.py"
run_case "R4-M1 mutation（停用 R4 判定）⇒ 目錄 fixture 不再紅燈" 0
CHECK="${CHECK_SAVED}"
run_case "R4-M1b 還原後同一 fixture 必須回紅" 1

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-monitoring-single-source PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-monitoring-single-source FAIL（pass=${pass} fail=${fail}）"
exit 1
