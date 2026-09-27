#!/usr/bin/env bash
# scripts/ci/promtool-mutation-negative-proof.sh — promtool 單元測試的**突變負向證明**
#
# 【為什麼需要這支（2026-09-27）】
#   `promtool test rules monitoring/tests/*.yml` 綠燈只證明「這一版規則與這一版 fixture 一致」，
#   它**不證明** fixture 有鑑別力：把一條規則的判定式改壞（例如把 `== 0` 改成 `== 2`），
#   只要沒有 case 會因此改變結論，整組測試照樣全綠 —— 那就是「看起來有守門、其實不會響」。
#   這個陷阱在本檔的主題上特別致命：第 10 條（AtlasUniverseSnapshotNotPersisted）覆蓋的是
#   **舊的 9 條完全沒有偵測器**的形狀（跑了、有產出、但產物沒落地）。它如果安靜地失效，
#   系統會回到「所有訊號都說健康、而下游讀的是無聲過期的母體」。
#
# 【做法】對規則檔做**外科式突變**，然後斷言 promtool 以恰為 1 的 exit code 失敗，
#   且失敗訊息指名**那條規則 / 那個 case**（擋下的必須是我們要證明的那件事，見 #2011）。
#   三個突變分別對應第 10 條的三個必要條件與第 6 條的輸入族守門：
#     M1  persisted == 0 → == 2  ⇒ case T（產物沒落地）必須紅
#     M2  ranked > 0 → >= 0      ⇒ case U（ranked == 0 且產物訊號 0）必須紅（互斥性被拿掉）
#     M3  刪掉第 6 條的 absent(atlas_universe_last_run_snapshot_persisted)
#                                ⇒ case V（產物訊號整族缺席）必須紅（第 10 條的輸入不再被守）
#
# 【紀律】
#   · 突變一律在 mktemp 目錄裡的**副本**上做，repo 內容只讀（負向證明不得改動受測物）。
#   · 突變目標必須在原始碼裡**恰好命中一次**；命中 0 或 ≥2 次一律視為失敗（fail-closed：
#     目標搬家了就代表這份證明已經不再證明它宣稱的事）。
#   · 先斷言「未突變的樹必須通過」——否則後面的「變紅」可能只是因為樹本來就壞的。
#   · 精確 exit code（`negproof_expect`）：127/2/124 都不算「擋下了」。
#   · promtool 版本與 CI / 生產一致（prom/prometheus:v3.14.0，釘版；見 quality.yml）。
#
# 【用法】
#   bash scripts/ci/promtool-mutation-negative-proof.sh
#   PROMTOOL=/usr/local/bin/promtool bash scripts/ci/promtool-mutation-negative-proof.sh
# exit：0 = 三個突變都被咬住且未突變為綠；1 = 證明失敗；2 = 用法 / 工具鏈不可用
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${HERE}/../.." && pwd)"
# shellcheck source=./negative-proof-lib.sh
. "${HERE}/negative-proof-lib.sh"
cd "${REPO_ROOT}" || { negproof_err "無法進入 ${REPO_ROOT}"; exit 2; }

PY="${PYTHON:-python3}"
command -v "${PY}" >/dev/null 2>&1 || { negproof_err "找不到 python3（PYTHON= 可覆寫）"; exit 2; }

PROMTOOL_IMAGE="prom/prometheus:v3.14.0"

# promtool 執行方式：優先 PROMTOOL（本機已安裝），否則用與 CI 同版的釘版容器。
# 兩者都是**唯讀**執行：只讀取 $WORK 底下的副本。
if [ -n "${PROMTOOL:-}" ]; then
  if [ ! -x "${PROMTOOL}" ]; then
    negproof_err "PROMTOOL='${PROMTOOL}' 不是可執行的檔案"
    exit 2
  fi
  run_promtool() { "${PROMTOOL}" "$@"; }
  RUNNER_DESC="本機 ${PROMTOOL}"
else
  if ! command -v docker >/dev/null 2>&1; then
    negproof_err "找不到 docker（也沒有設定 PROMTOOL）⇒ 無法執行 promtool；這是工具鏈不可用，不是『證明通過』"
    exit 2
  fi
  run_promtool() {
    docker run --rm -v "${WORK}:/work" -w /work --entrypoint /bin/promtool "${PROMTOOL_IMAGE}" "$@"
  }
  RUNNER_DESC="docker ${PROMTOOL_IMAGE}"
fi

RULES_REL="rules/atlas_universe_scoring_alerts.yml"
TESTS_REL="tests/atlas_universe_scoring_gaps_test.yml"

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
mkdir -p "${WORK}/rules" "${WORK}/tests"
cp monitoring/rules/*.yml "${WORK}/rules/"
cp monitoring/tests/*.yml "${WORK}/tests/"

LOG="${WORK}/promtool.log"
MUT_LOG="${WORK}/mut.log"

echo "→ promtool mutation negative proof（runner: ${RUNNER_DESC}；副本：${WORK}）"

# mutate <檔案> <old> <new> — 突變必須在原始碼裡恰好命中一次，否則失敗（fail-closed）。
mutate() {
  "${PY}" - "$1" "$2" "$3" <<'PYEOF'
import sys
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
src = open(path, encoding="utf-8").read()
n = src.count(old)
if n != 1:
    print("MUTATION_TARGET_NOT_UNIQUE:%d" % n)
    sys.exit(2)
open(path, "w", encoding="utf-8").write(src.replace(old, new))
PYEOF
}

# ── 0. 基準：未突變的樹必須通過（否則後面的「變紅」沒有意義）──────────────────────
if ! negproof_expect 0 "未突變的規則樹必須通過 promtool test rules" "${LOG}" \
      -- run_promtool test rules "${TESTS_REL}"; then
  negproof_show_log "${LOG}"
  exit 1
fi

# ── 突變定義 ─────────────────────────────────────────────────────────────────
# 三組字面值都取自規則檔（改規則文字時這裡必須同步；不同步會以「命中次數≠1」失敗）。
M1_OLD='            sum by (stage) (atlas_universe_last_run_snapshot_persisted{stage=~"daily|weekly"}) == 0'
M1_NEW='            sum by (stage) (atlas_universe_last_run_snapshot_persisted{stage=~"daily|weekly"}) == 2'

M2_OLD='            sum by (stage) (atlas_universe_last_run_symbols_ranked{stage=~"daily|weekly"}) > 0
          )
          and on (stage)
          (
            sum by (stage) (atlas_universe_last_run_snapshot_persisted{stage=~"daily|weekly"}) == 0'
M2_NEW='            sum by (stage) (atlas_universe_last_run_symbols_ranked{stage=~"daily|weekly"}) >= 0
          )
          and on (stage)
          (
            sum by (stage) (atlas_universe_last_run_snapshot_persisted{stage=~"daily|weekly"}) == 0'

M3_OLD='            or absent(atlas_universe_last_run_snapshot_persisted)
'
M3_NEW=''

PROOF_FAILED=0

prove_mutation() {
  local label="$1" old="$2" new="$3" want_alert="$4" want_case="$5"
  cp monitoring/rules/atlas_universe_scoring_alerts.yml "${WORK}/rules/atlas_universe_scoring_alerts.yml"
  if ! mutate "${WORK}/rules/atlas_universe_scoring_alerts.yml" "${old}" "${new}" > "${MUT_LOG}" 2>&1; then
    negproof_err "[${label}] 突變目標沒有恰好命中一次：$(cat "${MUT_LOG}") ⇒ 這份證明已與規則檔脫節（fail-closed）"
    PROOF_FAILED=1
    return
  fi
  if ! negproof_expect 1 "[${label}] 改壞判定式之後 promtool 必須失敗" "${LOG}" \
        -- run_promtool test rules "${TESTS_REL}"; then
    negproof_show_log "${LOG}"
    PROOF_FAILED=1
    return
  fi
  negproof_expect_output "${want_alert}" "${LOG}" \
    "[${label}] 擋下的必須是 ${want_alert}" || PROOF_FAILED=1
  negproof_expect_output "${want_case}" "${LOG}" \
    "[${label}] 擋下的必須是那個 case（${want_case}）" || PROOF_FAILED=1
}

prove_mutation "M1 persisted==0 被改成 ==2" "${M1_OLD}" "${M1_NEW}" \
  "AtlasUniverseSnapshotNotPersisted" "T 產物沒落地"
prove_mutation "M2 ranked>0 被放寬成 >=0" "${M2_OLD}" "${M2_NEW}" \
  "AtlasUniverseSnapshotNotPersisted" "U 沒有產出"
prove_mutation "M3 第 6 條不再守 last_run_snapshot_persisted" "${M3_OLD}" "${M3_NEW}" \
  "AtlasUniverseMetricsFamilyMissing" "V 產物訊號整族缺席"

if [ "${PROOF_FAILED}" -ne 0 ]; then
  echo "❌ promtool mutation negative proof 失敗（見上方 ::error::）" >&2
  exit 1
fi
echo "✅ promtool mutation negative proof：3 個突變全部被咬住，且未突變為綠"
