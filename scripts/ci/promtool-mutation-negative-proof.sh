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
#   四個突變分別對應第 10/11 條（兩個產物）的必要條件與第 6 條的輸入族守門：
#     M1  snapshot persisted == 0 → == 2  ⇒ case T（主產物沒落地）必須紅
#     M2  ranked > 0 → >= 0      ⇒ case U（ranked == 0 且產物訊號 0）必須紅（互斥性被拿掉）
#     M3a 刪掉第 6 條的 absent(atlas_universe_last_run_snapshot_persisted)
#                                ⇒ case V（snapshot 的產物訊號缺席）必須紅
#     M3b 刪掉第 6 條的 absent(atlas_universe_last_run_registry_persisted)
#                                ⇒ case V2（registry 的產物訊號缺席）必須紅
#         （M3a/M3b 是**兩個 case 各守一行**：只留其中一個 case 或只刪其中一行，另一個守門
#          就沒有鑑別力 —— 這正是 2026-09-27 把 V 拆成 V/V2 的原因。）
#     M4  registry persisted == 0 → == 2  ⇒ case W（snapshot 落地、registry 沒落地）必須紅
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
# ⚠️ 容器以 image 內建的 nobody 執行（官方 Dockerfile:USER nobody）。`mktemp -d` 的 700
# 在 Linux runner 上會讓 nobody **連 traverse 都不行** ⇒ promtool 讀不到檔案、以非 0 結束，
# 於是「未突變必須為綠」這一步就紅（本腳本 2026-09-27 在 CI 上實際踩到）。
# macOS 的 Docker Desktop 會把 uid 對映掉，所以本機不會重現 —— 這裡必須自己把權限打開。
chmod 755 "${WORK}"
chmod -R a+rX "${WORK}/rules" "${WORK}/tests"

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
# 突變字面值全部取自規則檔（改規則文字時這裡必須同步；不同步會以「命中次數≠1」失敗）。
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

M3B_OLD='            or absent(atlas_universe_last_run_registry_persisted)
'
M3B_NEW=''

# 第 11 條的判定:registry == 0 → == 2 ⇒ 本條恆不成立 ⇒ case W 必須紅。
M4_OLD='            sum by (stage) (atlas_universe_last_run_registry_persisted{stage=~"daily|weekly"}) == 0'
M4_NEW='            sum by (stage) (atlas_universe_last_run_registry_persisted{stage=~"daily|weekly"}) == 2'

PROOF_FAILED=0

# ── 為什麼是「兩次突變執行」而不是「四個突變各一次」────────────────────────────────
#
# 事實（2026-09-27 實測）：CI 上這個 job 的 `timeout-minutes: 5` 會被「基準 + 3 個突變」的
# 4 次 promtool 呼叫吃光 —— monitoring-config **timeout 而 cancelled**（前一次 4 次呼叫的成功
# 記錄是 4m07s，加上 runner 差異就超時）。超時的閘門等於不可靠的閘門，比慢一點更糟。
#
# 但**不是**所有突變都能合併：M1（把 persisted 的 `== 0` 改成 `== 2`）會讓第 10 條變成
# 恆不成立，於是 M2（把 ranked 的 `> 0` 放寬成 `>= 0`）在 case U 上的「不該 firing 卻 firing」
# 就被 M1 遮蔽掉了 —— 合併後 M2 咬不住（本腳本 2026-09-27 實測：失敗 case 只剩 2 個）。
# ⇒ 依**互相獨立**分組：
#   第一輪：M1（第 10 條恆不成立）＋ M3a/M3b（第 6 條不再守兩個新系列）＋ M4（第 11 條恆不成立）
#           —— 四者作用在不同規則/不同 case
#   第二輪：M2（互斥性被拿掉）單獨一輪
#   （2026-09-27 新增 M4 與 M3b 時**仍然只呼叫 promtool 三次**：新增的突變被併進既有的
#    第一輪。monitoring-config 的 timeout 是 8 分鐘，而三次呼叫在 CI 上約 3m06s ⇒ 併輪是
#    唯一不增加預算的做法；併輪的前提是「互不遮蔽」，由每一輪的 expect_fail_case_count 證明。）
# 每一輪都斷言：(a) rc 恰為 1、(b) 失敗輸出的 case 名/happening alert 名逐項命中、
# (c) **失敗的 case 數恰好等於預期**（證明沒有別的 case 被連帶改變、也沒有漏咬）。
# 這比「每個突變各跑一次」只少了「同一輪內互不干擾」這個維度，而那個維度由分組本身保證。

# run_mutated <label> <old> <new> [<label> <old> <new> …]
#   刻意用「三個參數一組」而不是任何分隔符：突變目標本身含 `|`（PromQL 的
#   `stage=~"daily|weekly"`）⇒ 用分隔符串接會在 `|` 上被切斷（本腳本 2026-09-27 實際踩到，
#   症狀是「突變目標命中 0 次」但後續步驟照跑）。參數分組沒有這個問題。
run_mutated() {
  cp monitoring/rules/atlas_universe_scoring_alerts.yml "${WORK}/rules/atlas_universe_scoring_alerts.yml"
  local target="${WORK}/rules/atlas_universe_scoring_alerts.yml"
  while [ "$#" -gt 0 ]; do
    local label="$1" old="$2" new="$3"
    shift 3
    if ! mutate "${target}" "${old}" "${new}" > "${MUT_LOG}" 2>&1; then
      negproof_err "[${label}] 突變目標沒有恰好命中一次：$(cat "${MUT_LOG}") ⇒ 這份證明已與規則檔脫節（fail-closed）"
      return 1
    fi
  done
  return 0
}

expect_fail_case_count() {
  # $1 = 期望的失敗 case 數（$2 = 說明）
  local want="$1" desc="$2" got
  got="$(grep -c '^    name: ' "${LOG}" || true)"
  if [ "${got}" = "${want}" ]; then
    echo "  ✅ [${desc}] 失敗的 case 數恰為 ${want}（沒有別的 case 被連帶改變）"
    return 0
  fi
  negproof_err "[${desc}] 失敗的 case 數為 ${got}，期望 ${want} ⇒ 突變的影響面與預期不符"
  return 1
}

# ── 第一輪：M1 + M3a + M3b + M4（四個互不干擾的突變，一次 promtool 呼叫）────────────
if ! run_mutated \
    "M1 snapshot persisted==0 被改成 ==2" "${M1_OLD}" "${M1_NEW}" \
    "M3a 第 6 條不再守 last_run_snapshot_persisted" "${M3_OLD}" "${M3_NEW}" \
    "M3b 第 6 條不再守 last_run_registry_persisted" "${M3B_OLD}" "${M3B_NEW}" \
    "M4 registry persisted==0 被改成 ==2" "${M4_OLD}" "${M4_NEW}"; then
  PROOF_FAILED=1
elif ! negproof_expect 1 "[M1+M3a+M3b+M4] 改壞判定式之後 promtool 必須失敗" "${LOG}" \
      -- run_promtool test rules "${TESTS_REL}"; then
  negproof_show_log "${LOG}"
  PROOF_FAILED=1
else
  negproof_expect_output "    name: T 產物沒落地" "${LOG}" \
    "[M1] snapshot persisted == 0 被改成 == 2 ⇒ case T 必須紅" || PROOF_FAILED=1
  negproof_expect_output "    alertname: AtlasUniverseSnapshotNotPersisted" "${LOG}" \
    "[M1] 而且紅的必須是第 10 條" || PROOF_FAILED=1
  negproof_expect_output "    name: V snapshot 的產物訊號缺席" "${LOG}" \
    "[M3a] 第 6 條不再守 last_run_snapshot_persisted ⇒ case V 必須紅" || PROOF_FAILED=1
  negproof_expect_output "    name: V2 registry 的產物訊號缺席" "${LOG}" \
    "[M3b] 第 6 條不再守 last_run_registry_persisted ⇒ case V2 必須紅" || PROOF_FAILED=1
  negproof_expect_output "    alertname: AtlasUniverseMetricsFamilyMissing" "${LOG}" \
    "[M3a/M3b] 而且紅的必須是第 6 條" || PROOF_FAILED=1
  negproof_expect_output "    name: W registry 沒落地" "${LOG}" \
    "[M4] registry persisted == 0 被改成 == 2 ⇒ case W 必須紅" || PROOF_FAILED=1
  negproof_expect_output "    alertname: AtlasUniverseRegistryNotPersisted" "${LOG}" \
    "[M4] 而且紅的必須是第 11 條" || PROOF_FAILED=1
  expect_fail_case_count 4 "M1+M3a+M3b+M4" || PROOF_FAILED=1
fi

# ── 第二輪：M2（必須單獨一輪，見上面的遮蔽說明）────────────────────────────────
if ! run_mutated "M2 ranked>0 被放寬成 >=0" "${M2_OLD}" "${M2_NEW}"; then
  PROOF_FAILED=1
elif ! negproof_expect 1 "[M2] 改壞判定式之後 promtool 必須失敗" "${LOG}" \
      -- run_promtool test rules "${TESTS_REL}"; then
  negproof_show_log "${LOG}"
  PROOF_FAILED=1
else
  negproof_expect_output "    name: U 沒有產出" "${LOG}" \
    "[M2] ranked > 0 被放寬成 >= 0 ⇒ case U 必須紅（互斥性被拿掉）" || PROOF_FAILED=1
  negproof_expect_output "    alertname: AtlasUniverseSnapshotNotPersisted" "${LOG}" \
    "[M2] 而且紅的必須是第 10 條" || PROOF_FAILED=1
  expect_fail_case_count 1 "M2" || PROOF_FAILED=1
fi

if [ "${PROOF_FAILED}" -ne 0 ]; then
  echo "❌ promtool mutation negative proof 失敗（見上方 ::error::）" >&2
  exit 1
fi
echo "✅ promtool mutation negative proof：4 個突變全部被咬住，且未突變為綠（3 次 promtool 呼叫）"
