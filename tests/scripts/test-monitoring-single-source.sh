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

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-monitoring-single-source PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-monitoring-single-source FAIL（pass=${pass} fail=${fail}）"
exit 1
