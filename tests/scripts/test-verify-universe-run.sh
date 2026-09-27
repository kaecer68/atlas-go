#!/usr/bin/env bash
# tests/scripts/test-verify-universe-run.sh — scripts/ops/verify-universe-run.{sh,py} 的契約測試
#
# 為什麼要有它（2026-09-27）
# -------------------------
# verify-universe-run 的價值**全部**在「判層」：它把「跑過 / 沒跑 / 跑了但輸出壞 /
# 跑了但 counter 沒發射 / 產物沒落地」分成互斥的層，並用 exit code 回答。這種工具的
# 失敗模式不是「崩潰」而是「**安靜地判錯層**」——把「我不知道」講成「綠」，或把
# 「重啟銷毀了證據」講成「沒跑」。那個錯誤在驗收當下是不可見的，所以必須用 fixture
# 把**每一個分支**釘住，再用 mutation 證明這些斷言**有牙齒**（改壞一個判定式 ⇒ 對應
# 的 fixture 必須紅）。
#
# 結構：
#   ① 每個 case = 一個 fixture 目錄（metrics.txt / rules.json / universe_snapshot.json /
#      logs.txt / increase_ranked.json / snapshot_mtime.txt / registry_mtime.txt …）
#      ＋ 期望的判層向量與 exit code
#   ② 每一層的判定式各有一個 mutation：把該判定式改成恆假 ⇒ 對應 case 的判層必須改變
#      （本輪新增的「L0 逐條比對 NEW_RULES 的載入集合」有**兩個方向**的 mutation：
#        已載入卻被當成沒部署 ⇒ WARN；沒載入卻被當成已部署 ⇒ 假綠）
#   ③ 唯讀斷言：跑完之後 fixture 目錄的內容（含 mtime）必須一字不差
#   ④ note / evidence 斷言：判層向量一樣但**字指錯條**也是一種騙人（例如「部分載入」時把
#      兩條新規則都講成沒部署）⇒ 這種錯誤只有比對 note 才看得見（check_field）
#
# 現有清單（22 個 fixture case）：
#   green / transport-family-missing / transport-rules-not-loaded / transport-new-rule-present /
#   transport-new-rule-partial / schedule-heartbeat-overdue / schedule-last-run-missed /
#   schedule-pending / holiday-closure / scoring-ranked-zero / scoring-universe-empty /
#   input-partial / emission-label-shape / emission-counter-silent / emission-legit-labels /
#   artifact-stale / artifact-missing / artifact-signal-zero / artifact-registry-signal /
#   legacy-schema / no-metrics / evidence-destroyed
# 現有清單（10 個 mutation）：
#   heartbeat-overdue / last-run-missing / output-family-missing / scoring-broken /
#   input-unusable / label-shape-bad / emission-missing / artifact-stale /
#   new-rule-deployed / new-rule-missing
#
# hermetic：不連網、不呼叫 docker、不碰 production、不改 repo（offline 模式 + mktemp 目錄）。
#
# 執行： bash tests/scripts/test-verify-universe-run.sh        （make test-scripts 會跑）
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TOOL_SH="$REPO/scripts/ops/verify-universe-run.sh"
TOOL_PY="$REPO/scripts/ops/verify-universe-run.py"
PY="${PYTHON:-python3}"

TMP="$(mktemp -d)"
# KEEP_FIXTURES=1 會保留 fixture 目錄（人工複核某個判層分支時很有用）:
#   KEEP_FIXTURES=1 bash tests/scripts/test-verify-universe-run.sh
#   bash scripts/ops/verify-universe-run.sh --offline-dir <印出的路徑>/f/<case> \
#        --workdir . --now "$(cat <印出的路徑>/f/<case>/now.txt)"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
fail() { echo "    FAIL: $*"; FAIL=$((FAIL + 1)); }
ok() { PASS=$((PASS + 1)); }

if [ ! -f "$TOOL_PY" ]; then
  echo "❌ 找不到 $TOOL_PY"
  exit 1
fi

# ── 固定的時間基準（用真實的 2026-09 日曆，讓 fixture 同時驗到交易日曆的解析）──────
E_SEP24_0600=1790229600   # 2026-09-24(四) 06:00Z 最後一次應執行（09-25 中秋、09-28 教師節休市）
E_SEP24_0602=1790229720
E_SEP28_0600=1790575200   # 2026-09-28(一) 06:00Z 教師節（休市）
E_SEP28_1200=1790596800
E_SEP29_0600=1790661600   # 2026-09-29(二) 06:00Z 母體驗收點
E_SEP29_0602=1790661720
E_SEP29_0605=1790661900
E_SEP29_0610=1790662200
E_SEP29_0900=1790672400
E_SEP30_0600=1790748000   # 若 09-29 已跑完，心跳會指向 09-30
E_SEP29_0750=1790668200   # 2026-09-29 07:50Z（觸發時刻之後才重啟的形狀）
E_BOOT=1790630000         # 09-28T21:13Z 的「暖機哨兵」（valid==0 的 stage 的 finished）

# ── fixture 產生器 ────────────────────────────────────────────────────────────
# 全部讀環境變數；未指定的就是「健康值」。刻意用未加引號的 heredoc（變數在產生時展開），
# 而且每個變數展開都放在行尾（避免全形字元緊鄰展開的守門誤判）。
write_metrics() {
  local out="$1"
  {
    printf '# HELP atlas_universe_last_run_valid 0 until a run of that stage completes.\n'
    printf '# TYPE atlas_universe_last_run_valid gauge\n'
    if [ "${NO_FAMILY:-0}" != "1" ]; then
      printf 'atlas_universe_last_run_valid{stage="daily"} %s\n' "${D_VALID:-1}"
      printf 'atlas_universe_last_run_valid{stage="weekly"} %s\n' "${W_VALID:-1}"
      printf 'atlas_universe_last_run_finished_timestamp_seconds{stage="daily"} %s\n' "${D_FIN:-$E_SEP29_0602}"
      printf 'atlas_universe_last_run_finished_timestamp_seconds{stage="weekly"} %s\n' "${W_FIN:-$E_SEP24_0602}"
      printf 'atlas_universe_last_run_symbols_gathered{stage="daily"} %s\n' "${D_GATHERED:-1599}"
      printf 'atlas_universe_last_run_symbols_filtered{stage="daily"} %s\n' "${D_FILTERED:-1599}"
      printf 'atlas_universe_last_run_symbols_ranked{stage="daily"} %s\n' "${D_RANKED:-3}"
      printf 'atlas_universe_last_run_ranked_trustworthy{stage="daily"} %s\n' "${D_TRUST:-1}"
      printf 'atlas_universe_last_run_symbols_gathered{stage="weekly"} %s\n' "${W_GATHERED:-1599}"
      printf 'atlas_universe_last_run_symbols_filtered{stage="weekly"} %s\n' "${W_FILTERED:-1599}"
      printf 'atlas_universe_last_run_symbols_ranked{stage="weekly"} %s\n' "${W_RANKED:-3}"
      printf 'atlas_universe_last_run_ranked_trustworthy{stage="weekly"} %s\n' "${W_TRUST:-1}"
      if [ -n "${D_PERSISTED:-}" ]; then
        printf 'atlas_universe_last_run_snapshot_persisted{stage="daily"} %s\n' "$D_PERSISTED"
      fi
      if [ -n "${W_PERSISTED:-}" ]; then
        printf 'atlas_universe_last_run_snapshot_persisted{stage="weekly"} %s\n' "$W_PERSISTED"
      fi
      if [ -n "${D_REG_PERSISTED:-}" ]; then
        printf 'atlas_universe_last_run_registry_persisted{stage="daily"} %s\n' "$D_REG_PERSISTED"
      fi
      if [ -n "${W_REG_PERSISTED:-}" ]; then
        printf 'atlas_universe_last_run_registry_persisted{stage="weekly"} %s\n' "$W_REG_PERSISTED"
      fi
    fi
    printf 'atlas_universe_next_run_timestamp_seconds %s\n' "${NEXT_RUN:-$E_SEP30_0600}"
    printf 'atlas_universe_symbols_gathered_total{stage="daily"} %s\n' "${D_GATHERED:-1599}"
    printf 'atlas_universe_symbols_filtered_total{reason="industry_filter",stage="daily"} %s\n' "${D_FILTERED:-1599}"
    printf 'atlas_universe_symbols_screened_total{result="passed",stage="daily"} %s\n' "${D_RANKED:-3}"
    printf 'atlas_universe_symbols_screened_total{result="failed",stage="daily"} 1596\n'
    printf 'atlas_universe_symbols_ranked_total{stage="daily"} %s\n' "${D_RANKED:-3}"
    printf 'atlas_universe_quotes_fetched_total{stage="daily"} %s\n' "${QUOTES_RETURNED:-1581}"
    printf 'atlas_universe_snapshot_persisted_total{stage="daily"} 1\n'
    printf 'up{job="atlas-go"} 1\n'
    if [ -n "${LEGACY_LABEL_LINE:-}" ]; then
      printf '%s\n' "$LEGACY_LABEL_LINE"
    fi
    if [ -n "${EXTRA_METRIC_LINES:-}" ]; then
      printf '%s\n' "$EXTRA_METRIC_LINES"
    fi
  } > "$out"
}

write_rules() {
  local out="$1" names
  names="AtlasUniverseRankedZero AtlasUniverseScreeningAllRejected AtlasUniverseQuotesMissing \
AtlasUniverseScreenedFamilyMissing AtlasUniverseEmptyFiltered AtlasUniverseMetricsFamilyMissing \
AtlasUniverseEmptyUniverse AtlasUniverseRunOverdue AtlasUniverseCounterEmissionMissing"
  if [ "${RULES_MODE:-core}" = "none" ]; then
    printf '{"status":"success","data":{"groups":[{"name":"atlas_container_liveness","rules":[]}]}}\n' > "$out"
    return
  fi
  # 本輪新增的兩條規則（= 工具裡的 NEW_RULES）：all = 兩條都載入（部署完成的世界），
  # core_plus_snapshot = 只有第一條載入（**部分載入**的形態）。
  # 刻意用獨立的模式名而不是改寫既有模式：core / all / none 的語意（與既有 case 的
  # 期望向量）必須一字不動。
  if [ "${RULES_MODE:-core}" = "all" ]; then
    names="$names AtlasUniverseSnapshotNotPersisted AtlasUniverseRegistryNotPersisted"
  fi
  if [ "${RULES_MODE:-core}" = "core_plus_snapshot" ]; then
    names="$names AtlasUniverseSnapshotNotPersisted"
  fi
  {
    printf '{"status":"success","data":{"groups":[{"name":"atlas_universe_scoring","interval":60,"rules":['
    local first=1 n
    for n in $names; do
      if [ "$first" = "1" ]; then first=0; else printf ','; fi
      printf '{"name":"%s","type":"alerting","state":"inactive","health":"ok"}' "$n"
    done
    printf ']}]}}\n'
  } > "$out"
}

# $1=dir  $2=snapshot 模式（healthy|zero|empty|legacy|old|none）
write_snapshot() {
  local out="$1/universe_snapshot.json" mode="$2"
  case "$mode" in
    none) return 0 ;;
    healthy)
      cat > "$out" <<'JSON'
{
  "ranked": [{"symbol": "2330"}, {"symbol": "2317"}, {"symbol": "2454"}],
  "result": {
    "symbols_built": 1599, "symbols_filtered": 1599, "symbols_ranked": 3, "symbols_excluded": 0,
    "full_rebuild": false, "timestamp": "2026-09-29T06:00:05Z",
    "quotes_status": "ok", "quotes_returned": 1581, "quotes_requested": 1599,
    "quotes_chunks": 32, "quotes_chunks_failed": 0,
    "quotes_missing_no_data": 12, "quotes_missing_not_covered": 6,
    "quotes_missing_fetch_error": 0, "quotes_missing_not_attempted": 0,
    "ranked_trustworthy": true
  }
}
JSON
      ;;
    zero)
      cat > "$out" <<'JSON'
{
  "ranked": [],
  "result": {
    "symbols_built": 1599, "symbols_filtered": 1599, "symbols_ranked": 0, "symbols_excluded": 0,
    "full_rebuild": false, "timestamp": "2026-09-29T06:00:05Z",
    "quotes_status": "ok", "quotes_returned": 1581, "quotes_requested": 1599,
    "quotes_chunks": 32, "quotes_chunks_failed": 0,
    "ranked_trustworthy": true
  }
}
JSON
      ;;
    empty)
      cat > "$out" <<'JSON'
{
  "ranked": [],
  "result": {
    "symbols_built": 0, "symbols_filtered": 0, "symbols_ranked": 0, "symbols_excluded": 0,
    "full_rebuild": false, "timestamp": "2026-09-29T06:00:02Z",
    "quotes_status": "not_attempted", "quotes_returned": 0, "quotes_requested": 0,
    "ranked_fallback_reason": "empty_universe", "ranked_trustworthy": false
  }
}
JSON
      ;;
    partial)
      cat > "$out" <<'JSON'
{
  "ranked": [],
  "result": {
    "symbols_built": 1599, "symbols_filtered": 1599, "symbols_ranked": 0, "symbols_excluded": 0,
    "full_rebuild": false, "timestamp": "2026-09-29T06:00:09Z",
    "quotes_status": "partial", "quotes_returned": 1200, "quotes_requested": 1599,
    "quotes_chunks": 32, "quotes_chunks_failed": 2,
    "quotes_missing_fetch_error": 399, "quotes_missing_not_attempted": 0,
    "ranked_fallback_reason": "quote_fetch_partial", "ranked_trustworthy": false
  }
}
JSON
      ;;
    # 2026-09-17 的舊 schema（quotes_status / ranked_trustworthy 都還不存在）
    legacy)
      cat > "$out" <<'JSON'
{
  "ranked": [],
  "result": {
    "symbols_built": 27, "symbols_filtered": 27, "symbols_ranked": 0, "symbols_excluded": 0,
    "full_rebuild": false, "timestamp": "2026-09-17T06:00:05Z"
  }
}
JSON
      ;;
  esac
}

write_logs() {
  local out="$1" mode="$2"
  case "$mode" in
    none) return 0 ;;
    ran)
      cat > "$out" <<'LOG'
time=2026-09-29T05:30:00.000000000Z level=INFO msg=quote_backfill_ok component=quote_backfill
time=2026-09-29T06:00:01.000000000Z level=INFO msg=daily_refresh_start component=universe_scheduler
time=2026-09-29T06:00:02.000000000Z level=INFO msg=symbols_gathered count=1599 full_rebuild=false component=universe_scheduler
time=2026-09-29T06:02:00.000000000Z level=INFO msg=daily_refresh_ok built=1599 filtered=1599 ranked=3 excluded=0 component=universe_scheduler
LOG
      ;;
    quiet)
      # 日誌涵蓋應執行時刻，但沒有任何 universer 事件（= 缺席可被否證的形狀）
      cat > "$out" <<'LOG'
time=2026-09-29T05:30:00.000000000Z level=INFO msg=quote_backfill_ok component=quote_backfill
time=2026-09-29T07:40:00.000000000Z level=INFO msg=server_startup_ok component=gateway
LOG
      ;;
    holiday)
      cat > "$out" <<'LOG'
time=2026-09-28T06:00:00.000000000Z level=INFO msg=weekly_skip_holiday date=2026-09-28 weekday=Monday criterion=marketdata.IsTaiwanTradingDay component=universe_scheduler
LOG
      ;;
    restarted_after)
      # 觸發時刻（06:00Z）之後才重啟：日誌與 process 都涵蓋不到那個時刻 ⇒ 缺席不可否證。
      cat > "$out" <<'LOG'
time=2026-09-29T07:50:30.000000000Z level=INFO msg=server_startup_ok component=gateway
time=2026-09-29T07:51:00.000000000Z level=INFO msg=QuoteCache load ok component=marketdata
LOG
      ;;
  esac
}

# 一個 fixture 目錄 = 一份世界狀態。所有可變項都透過環境變數傳入。
build_case() {
  local dir="$TMP/f/$1"
  mkdir -p "$dir"
  printf '%s\n' "${NOW:-$E_SEP29_0610}" > "$dir/now.txt"
  if [ "${NO_METRICS:-0}" != "1" ]; then write_metrics "$dir/metrics.txt"; fi
  write_rules "$dir/rules.json"
  printf '{"status":"success","data":{"alerts":[]}}\n' > "$dir/alerts.json"
  write_logs "$dir/logs.txt" "${LOGS_MODE:-ran}"
  write_snapshot "$dir" "${SNAPSHOT:-healthy}"
  printf '{"symbols":[],"updated_at":"2026-09-29T06:00:05Z"}\n' > "$dir/universe.json"
  printf '{"daily": %s, "weekly": %s}\n' "${INC_DAILY:-3}" "${INC_WEEKLY:-3}" > "$dir/increase_ranked.json"
  printf '%s\n' "${SNAP_MTIME:-$E_SEP29_0602}" > "$dir/snapshot_mtime.txt"
  printf '%s\n' "${REG_MTIME:-$E_SEP29_0602}" > "$dir/registry_mtime.txt"
  if [ -n "${EXTRA_ARGS:-}" ]; then printf '%s\n' "$EXTRA_ARGS" > "$dir/extra_args.txt"; fi
}

# ── 跑一個 case 並比對期望 ─────────────────────────────────────────────────────
summarize() {
  # $1 = json 檔 → 印出 exit / L0..L5 的判層（排序固定）
  "$PY" - "$1" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
lines = ["exit=%d" % d["exit_code"]]
for layer in d["layers"]:
    lines.append("%s=%s" % (layer["key"], layer["verdict"]))
print("\n".join(lines))
PY
}

# fixture 目錄的內容指紋（內容 sha256 + size + mtime）——唯讀斷言用。
# 刻意用 python3 而不是 shasum/stat：後兩者在 BSD/GNU 的旗標不同，會讓這支測試
# 只在 macOS 上成立（hermetic 測試不該綁平台）。
fingerprint() {
  "$PY" - "$1" <<'PY'
import hashlib, os, sys
root = sys.argv[1]
rows = []
for dirpath, _, files in os.walk(root):
    for name in files:
        path = os.path.join(dirpath, name)
        st = os.stat(path)
        with open(path, "rb") as fh:
            digest = hashlib.sha256(fh.read()).hexdigest()
        rows.append("%s %d %d %s" % (os.path.relpath(path, root), st.st_size, int(st.st_mtime), digest))
print("\n".join(sorted(rows)))
PY
}

# 從 --json 輸出取某一層的某個欄位（note / predicate / evidence）。判層向量看不到
# 「note 把規則指錯條」這種錯誤（判層一樣、字卻錯）⇒ 這一節補上那個盲點。
field_of() {
  "$PY" - "$1" "$2" "$3" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
for layer in d["layers"]:
    if layer["key"] == sys.argv[2]:
        v = layer[sys.argv[3]]
        print(v if isinstance(v, str) else "\n".join(v))
        break
PY
}

check_field() {
  # $1=case $2=layer $3=欄位 $4=必須出現的子字串 $5=（可選）必須**不**出現的子字串
  local case_name="$1" layer="$2" field="$3" want="$4" unwanted="${5:-}"
  local json="$TMP/out-$case_name.json" got
  if [ ! -f "$json" ]; then
    # 全形字元緊鄰 `$var` 時，bash 會把它吃進變數名 ⇒ set -u 下這一整行直接中止
    # （F28；`scripts/ci/check_fullwidth_var_expansion.py` 會擋）⇒ 訊息裡一律用 ${var}。
    fail "${case_name}: 找不到 ${json}（run_case 沒有產出 --json）"
    return
  fi
  got="$(field_of "$json" "$layer" "$field")"
  if [[ "$got" != *"$want"* ]]; then
    fail "${case_name}: ${layer}.${field} 未包含「${want}」"
    printf '      got: %s\n' "$got"
    return
  fi
  if [ -n "$unwanted" ] && [[ "$got" == *"$unwanted"* ]]; then
    fail "${case_name}: ${layer}.${field} 竟包含「${unwanted}」（不該出現）"
    printf '      got: %s\n' "$got"
    return
  fi
  ok
}

run_case() {
  local name="$1" extra="${2:-}"
  local dir="$TMP/f/$name"
  local now; now="$(cat "$dir/now.txt")"
  local json="$TMP/out-$name.json"
  # 唯讀斷言：跑之前把 fixture 目錄的內容指紋記下來
  local before after
  before="$(fingerprint "$dir")"
  # shellcheck disable=SC2086
  bash "$TOOL_SH" --offline-dir "$dir" --workdir "$REPO" --now "$now" --json $extra > "$json" 2> "$TMP/err-$name.txt"
  local rc=$?
  after="$(fingerprint "$dir")"
  if [ "$before" != "$after" ]; then
    fail "$name: 工具改動了 fixture 目錄（唯讀契約被破壞）"
  fi
  if [ "$rc" -ge 3 ]; then
    fail "$name: exit=${rc}（用法/設定錯誤）"
    cat "$TMP/err-$name.txt" | head -5
  fi
  summarize "$json" > "$TMP/got-$name.txt" 2>/dev/null
  if ! diff -u "$dir/expect.txt" "$TMP/got-$name.txt" > "$TMP/diff-$name.txt"; then
    fail "$name: 判層與期望不符"
    sed -n '1,25p' "$TMP/diff-$name.txt" | sed 's/^/      /'
  else
    ok
  fi
}

expect_file() {
  local dir="$TMP/f/$1"
  shift
  printf '%s\n' "$@" > "$dir/expect.txt"
}


# ══════════════════════════════════════════════════════════════════════════════
# ① 每一個判層分支的 fixture（每個 case 都是一個「世界狀態」）
# ══════════════════════════════════════════════════════════════════════════════

# 1. 全綠：09-29(二) 06:00Z 這一輪跑完、產物落地、counter 有增量。
#    L0 = WARN 是刻意的：本輪新增的 AtlasUniverseSnapshotNotPersisted 還沒部署
#    （部署時序排在 09-29 驗收之後）⇒ 這是預期狀態，不是缺口。
case_green() {
  NOW=$E_SEP29_0610
  build_case green
  expect_file green exit=0 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 2. 傳輸面：服務活著、/metrics 有回應，但 last_run_* 輸出族完全不存在。
case_transport_family_missing() {
  NOW=$E_SEP29_0610 NO_FAMILY=1
  build_case transport-family-missing
  expect_file transport-family-missing exit=1 L0=RED L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 3. 傳輸面：Prometheus 沒載入規則群（2026-08-27 事故的形狀：規則寫進錯的樹 ⇒ 27 天沒被評估）。
case_transport_rules_not_loaded() {
  NOW=$E_SEP29_0610 RULES_MODE=none
  build_case transport-rules-not-loaded
  expect_file transport-rules-not-loaded exit=1 L0=RED L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 4. 傳輸面：本輪新增的規則也部署了（部署之後的世界）⇒ 不再有 WARN。
case_transport_new_rule_present() {
  NOW=$E_SEP29_0610 RULES_MODE=all
  build_case transport-new-rule-present
  expect_file transport-new-rule-present exit=0 L0=OK L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 5. 排程面：心跳逾時（= AtlasUniverseRunOverdue 的判定式），但當天的產物是好的 ⇒ 只有 L1 紅。
case_schedule_heartbeat_overdue() {
  NOW=$E_SEP29_0900 NEXT_RUN=$E_SEP29_0600
  build_case schedule-heartbeat-overdue
  expect_file schedule-heartbeat-overdue exit=1 L0=WARN L1=RED L2=OK L3=OK L4=OK L5=OK
}

# 6. 排程面：**沒跑，而心跳不會響**。最後一輪在 09-24，09-29 的觸發時刻過了卻沒有任何完成；
#    心跳已被「觸發時刻之後的重啟」重新發佈到 09-30 ⇒ RunOverdue 沉默（現行規則的結構性盲點）。
#    同時產物也停在 09-24 ⇒ L5 一起紅（正確：那是兩個不同的失敗面，但同一個根因的兩面）。
case_schedule_last_run_missed() {
  NOW=$E_SEP29_0900 NEXT_RUN=$E_SEP30_0600 LOGS_MODE=quiet \
  D_FIN=$E_SEP24_0602 W_FIN=$E_SEP24_0602 SNAP_MTIME=$E_SEP24_0602 \
  INC_DAILY=1 INC_WEEKLY=1
  build_case schedule-last-run-missed
  expect_file schedule-last-run-missed exit=1 L0=WARN L1=RED L2=OK L3=OK L4=OK L5=RED
}

# 7. 排程面：驗收當下（06:05，觸發時刻 06:00 剛過、還在寬限內）⇒ PENDING（exit 2，不是綠也不是紅）。
case_schedule_pending() {
  NOW=$E_SEP29_0605 NEXT_RUN=$E_SEP29_0600 LOGS_MODE=quiet \
  D_VALID=0 D_FIN=$E_BOOT W_VALID=1 W_FIN=$E_SEP24_0602 SNAP_MTIME=$E_SEP24_0602 \
  INC_DAILY=1 INC_WEEKLY=1
  build_case schedule-pending
  expect_file schedule-pending exit=2 L0=WARN L1=PENDING L2=OK L3=OK L4=OK L5=OK
}

# 8. 排程面：休市連假（09-25 中秋、09-28 教師節、中間夾週末）⇒ 最後一次應執行是 09-24，
#    那一輪跑過 ⇒ 不得因為「連假沒跑」而紅。日曆解析錯的話這個 case 會紅。
case_holiday_closure() {
  NOW=$E_SEP28_1200 NEXT_RUN=$E_SEP29_0600 LOGS_MODE=holiday \
  D_VALID=1 D_FIN=$E_SEP24_0602 W_VALID=1 W_FIN=$E_SEP24_0602 SNAP_MTIME=$E_SEP24_0602 \
  INC_DAILY=1 INC_WEEKLY=1
  build_case holiday-closure
  expect_file holiday-closure exit=0 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 9. 評分面：跑了、輸入可信，但 ranked == 0（市場判定）⇒ L3 紅（rules 1/2 的職責）。
case_scoring_ranked_zero() {
  NOW=$E_SEP29_0610 SNAPSHOT=zero D_RANKED=0 D_TRUST=1 W_RANKED=0 W_TRUST=1 \
  INC_DAILY=0 INC_WEEKLY=0
  build_case scoring-ranked-zero
  expect_file scoring-ranked-zero exit=1 L0=WARN L1=OK L2=OK L3=RED L4=OK L5=OK
}

# 10. 評分面：母體為空（Step 1 一檔都沒取到）⇒ L3 紅；報價因此 not_attempted ⇒ L2 也紅。
case_scoring_universe_empty() {
  NOW=$E_SEP29_0610 SNAPSHOT=empty D_GATHERED=0 D_FILTERED=0 D_RANKED=0 D_TRUST=0 \
  W_GATHERED=0 W_FILTERED=0 W_RANKED=0 W_TRUST=0 INC_DAILY=0 INC_WEEKLY=0
  build_case scoring-universe-empty
  expect_file scoring-universe-empty exit=1 L0=WARN L1=OK L2=RED L3=RED L4=OK L5=OK
}

# 11. 資料面：報價只回來一部分（partial）⇒ L2 紅；排名因此不可信 ⇒ L3 也紅。
#     兩層都紅是正確的：L2 講「輸入怎麼壞的」，L3 講「產出因此不可用」。
case_input_partial() {
  NOW=$E_SEP29_0610 SNAPSHOT=partial D_RANKED=0 D_TRUST=0 W_RANKED=0 W_TRUST=0 \
  QUOTES_RETURNED=1200 INC_DAILY=0 INC_WEEKLY=0
  build_case input-partial
  expect_file input-partial exit=1 L0=WARN L1=OK L2=RED L3=RED L4=OK L5=OK
}

# 12. 發射面①：counter 的標籤形狀壞掉。生產實例：`increase(symbols_screened_total[7d])` 讀到 4906，
#     但那其實是 legacy 系列 {daily="failed"} 的殘留；正確形狀 {stage=...} 全是 0。
#     ⇒ 只要出現這種 series，任何 increase() 的讀數都不可用 ⇒ L4 直接紅（不是「讀到 4906 很正常」）。
case_emission_label_shape() {
  NOW=$E_SEP29_0610 LEGACY_LABEL_LINE='atlas_universe_symbols_screened_total{daily="failed"} 4906'
  build_case emission-label-shape
  expect_file emission-label-shape exit=1 L0=WARN L1=OK L2=OK L3=OK L4=RED L5=OK
}

# 13. 發射面②：輸出說 ranked>0 且新鮮，但 counter 6 天視窗內沒有增量 ⇒ 記帳面斷線
#     （2026-09-25 的真實形狀；rules 9 的職責）。
case_emission_counter_silent() {
  NOW=$E_SEP29_0610 INC_DAILY=0 INC_WEEKLY=0
  build_case emission-counter-silent
  expect_file emission-counter-silent exit=1 L0=WARN L1=OK L2=OK L3=OK L4=RED L5=OK
}

# 14. 產物面：跑了、有產出、verdict 說在 06:02 完成過，但檔案 mtime 停在 09-24
#     ⇒ **「產物沒有落地」**（舊的 9 條規則完全沒有覆蓋這個形狀；本輪新增第 10 條規則補上）。
case_artifact_stale() {
  NOW=$E_SEP29_0610 SNAP_MTIME=$E_SEP24_0602
  build_case artifact-stale
  expect_file artifact-stale exit=1 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=RED
}

# 15. 產物面：檔案根本不存在（讀它的消費者會把「空」當成「市場沒有標的」）。
case_artifact_missing() {
  NOW=$E_SEP29_0900 SNAPSHOT=none LOGS_MODE=quiet \
  D_FIN=$E_SEP24_0602 W_FIN=$E_SEP24_0602 INC_DAILY=1 INC_WEEKLY=1
  build_case artifact-missing
  expect_file artifact-missing exit=1 L0=WARN L1=RED L2=UNKNOWN L3=UNKNOWN L4=OK L5=RED
}

# 16. 產物面：新的輸出訊號（last_run_snapshot_persisted）說 daily 那一輪沒有落地。
#     這是部署**之後** Prometheus 上會 firing 的同一個判定（規則 10），且是 per-stage 的：
#     weekly 的 1 不能替 daily 的 0 背書。
case_artifact_signal_zero() {
  NOW=$E_SEP29_0610 D_PERSISTED=0 W_PERSISTED=1
  build_case artifact-signal-zero
  expect_file artifact-signal-zero exit=1 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=RED
}

# 17. 舊 schema 的 artifact（2026-09-17 的形狀：沒有 quotes_status / ranked_trustworthy）
#     ⇒ L2 是 UNKNOWN 而不是 OK 或 RED：它**答不出**報價品質，那是 artifact 的能力界線，
#     不是「資料面健康」。L3 仍然紅（ranked == 0 就是壞）。
case_legacy_schema() {
  NOW=$E_SEP29_0610 SNAPSHOT=legacy D_RANKED=0 D_TRUST=0 W_RANKED=0 W_TRUST=0 \
  INC_DAILY=0 INC_WEEKLY=0
  build_case legacy-schema
  expect_file legacy-schema exit=1 L0=WARN L1=OK L2=UNKNOWN L3=RED L4=OK L5=OK
}

# 18. 完全讀不到 /metrics（服務沒回應）⇒ L0/L1/L4 都 UNKNOWN、exit 2。
#     這是「我不知道」與「壞了」分開的證明：沒有訊號不等於母體壞了，
#     但**也絕不等於綠燈**（所以不是 0）。
case_no_metrics() {
  NOW=$E_SEP29_0900 NO_METRICS=1 LOGS_MODE=none
  build_case no-metrics
  expect_file no-metrics exit=2 L0=UNKNOWN L1=UNKNOWN L2=OK L3=OK L4=UNKNOWN L5=OK
}

# 20. 發射面③（**假陽性迴歸**）：這一族裡有「合法但少見」的 label 名
#     （coverage 稽核的 `industry`、narrative 分類的 `error_type`）。形狀檢查若寫成
#     「label 名不在白名單裡就報警」，就會在**健康**的生產資料上誤報 —— 這個誤判是
#     2026-09-27 用生產的真實 /metrics 跑出來才發現的，因此必須有一個 case 釘住它。
case_emission_legit_labels() {
  NOW=$E_SEP29_0610
  EXTRA_METRIC_LINES='atlas_universe_coverage_mapped_total{industry="all",stage="daily"} 1599
atlas_universe_coverage_total{industry="all",stage="daily"} 1600
atlas_universe_narrative_errors_total{error_type="scrape_error",stage="daily"} 0
atlas_universe_narrative_events_scraped_total{stage="daily"} 4'
  build_case emission-legit-labels
  expect_file emission-legit-labels exit=0 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 19. 重啟把證據銷毀的形狀：應執行時刻（09-29 06:00）之後才重啟（09-29 07:30），
#     日誌只涵蓋到 07:30 之後、process 也才剛起來 ⇒ 「那一輪跑了沒有」**不可查**。
#     ⇒ L1 = UNKNOWN（不是 RED：把「我不知道」報成「壞了」會浪費一次值班）。
case_evidence_destroyed() {
  NOW=$E_SEP29_0900 D_VALID=0 D_FIN=$E_SEP29_0750 W_VALID=0 W_FIN=$E_SEP29_0750 \
  SNAP_MTIME=$E_SEP24_0602 INC_DAILY=0 INC_WEEKLY=0 LOGS_MODE=restarted_after
  build_case evidence-destroyed
  expect_file evidence-destroyed exit=1 L0=WARN L1=UNKNOWN L2=OK L3=OK L4=OK L5=RED
}

# 21. 傳輸面：**部分載入** —— 本輪新增的兩條規則只有一條在 /api/v1/rules 的載入集合裡。
#     這是「部署狀態由工具自己判」的關鍵證據：判層是 WARN，而且 note 只能指名**缺的那一條**
#     （把已載入的那條也講成沒部署 = 誤導）。同時釘住 NEW_RULES 的成員：漏列
#     AtlasUniverseRegistryNotPersisted 的話這個 case 會變成 OK（不該綠）。
case_transport_new_rule_partial() {
  NOW=$E_SEP29_0610 RULES_MODE=core_plus_snapshot
  build_case transport-new-rule-partial
  expect_file transport-new-rule-partial exit=0 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=OK
}

# 22. 產物面：registry（data/state/universe.json）的 mtime 明顯比 snapshot 舊 ⇒ 那一條
#     寫入路徑可能失敗。**判層仍然是 OK**：L5 的判定式比的是 snapshot 這一個檔（見
#     layer_artifact 的 predicate），本 PR 只把 registry 的落地訊號逐 stage 印出來、
#     並把 note 指向偵測它的規則（AtlasUniverseRegistryNotPersisted）。
#     這個 case 是**敘述契約**的釘子：note 必須指向那條規則，且不得再出現
#     「目前沒有對應告警」（偵測器落地之後那句話就是假的）。
case_artifact_registry_signal() {
  NOW=$E_SEP29_0610 REG_MTIME=$E_SEP24_0602 D_REG_PERSISTED=0 W_REG_PERSISTED=0
  build_case artifact-registry-signal
  expect_file artifact-registry-signal exit=0 L0=WARN L1=OK L2=OK L3=OK L4=OK L5=OK
}

# ══════════════════════════════════════════════════════════════════════════════
# ② mutation 自證：把某一層的判定式改壞 ⇒ 對應 case 的判層必須改變
# ══════════════════════════════════════════════════════════════════════════════
# 沒有這一節，「22 個 case 全綠」只證明斷言沒有牙齒。每一個 mutation 都必須
# (a) 在原始碼裡命中**恰好一次**（否則就是改錯地方，直接 fail），且
# (b) 讓指定的 case 在指定的層從期望值變成 mutated 值。
mutate_predicate() {
  local file="$1" old="$2" new="$3"
  "$PY" - "$file" "$old" "$new" <<'PY'
import sys
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
src = open(path, encoding="utf-8").read()
n = src.count(old)
if n != 1:
    print("MUTATION TARGET NOT UNIQUE (%d)" % n)
    sys.exit(2)
open(path, "w", encoding="utf-8").write(src.replace(old, new))
PY
}

check_mutation() {
  # $1=label $2=case $3=layer $4=mutated 期望值 $5=old $6=new
  local label="$1" case_name="$2" layer="$3" want_mut="$4" old="$5" new="$6"
  local dir="$TMP/f/$case_name" mut="$TMP/mut-$label.py" now got base_verdict
  cp "$TOOL_PY" "$mut"
  if ! mutate_predicate "$mut" "$old" "$new"; then
    fail "mutation/$label: 判定式在原始碼裡不是唯一命中（改錯地方）"
    return
  fi
  now="$(cat "$dir/now.txt")"
  "$PY" "$mut" --offline-dir "$dir" --workdir "$REPO" --now "$now" --json > "$TMP/mout-$label.json" 2>/dev/null
  got="$(summarize "$TMP/mout-$label.json" | grep "^$layer=" | cut -d= -f2)"
  base_verdict="$(grep "^$layer=" "$dir/expect.txt" | cut -d= -f2)"
  if [ "$base_verdict" = "$want_mut" ]; then
    fail "mutation/$label: 期望值與 baseline 相同（fixture 沒有隔離這一層）"
    return
  fi
  if [ "$got" = "$want_mut" ]; then
    echo "    mutation/${label}: ${layer} ${base_verdict} -> ${got}（咬住了）"
    ok
  else
    fail "mutation/${label}: 改壞判定式後 ${layer} = ${got}，期望 ${want_mut}（mutation 沒被咬住）"
  fi
}


# ══════════════════════════════════════════════════════════════════════════════
# ③ 執行
# ══════════════════════════════════════════════════════════════════════════════

echo "→ verify-universe-run 契約測試（fixture 判層 + mutation 自證；hermetic、唯讀）"

# --help 必須可用（exit 0），且不得寫任何檔案
if ! bash "$TOOL_SH" --help > /dev/null 2>&1; then
  fail "--help 失敗"
else
  ok
fi

( case_green )
( case_transport_family_missing )
( case_transport_rules_not_loaded )
( case_transport_new_rule_present )
( case_schedule_heartbeat_overdue )
( case_schedule_last_run_missed )
( case_schedule_pending )
( case_holiday_closure )
( case_scoring_ranked_zero )
( case_scoring_universe_empty )
( case_input_partial )
( case_emission_label_shape )
( case_emission_counter_silent )
( case_artifact_stale )
( case_artifact_missing )
( case_artifact_signal_zero )
( case_legacy_schema )
( case_no_metrics )
( case_evidence_destroyed )
( case_emission_legit_labels )
( case_transport_new_rule_partial )
( case_artifact_registry_signal )

for c in green transport-family-missing transport-rules-not-loaded transport-new-rule-present \
         schedule-heartbeat-overdue schedule-last-run-missed schedule-pending holiday-closure \
         scoring-ranked-zero scoring-universe-empty input-partial emission-label-shape \
         emission-counter-silent artifact-stale artifact-missing artifact-signal-zero \
         legacy-schema no-metrics evidence-destroyed emission-legit-labels \
         transport-new-rule-partial artifact-registry-signal; do
  run_case "$c"
done

# 判層向量之外的斷言：**note / evidence 也是契約**（判層相同但指名錯的規則同樣是騙人）。
# ① green：兩條新規則都還沒部署 ⇒ note 要一併指名（不是只講一條）。
check_field green L0 note 'AtlasUniverseSnapshotNotPersisted'
check_field green L0 note 'AtlasUniverseRegistryNotPersisted'
# ② 規則群整族沒載入 ⇒ 該 RED 的還是 RED，且 note 講的是「規則群沒載入」而不是「新規則沒部署」。
check_field transport-rules-not-loaded L0 note '沒有載入規則群 atlas_universe_scoring'
# ③ 兩條新規則都載入 ⇒ note 要講「含本輪新增的 N 條也已載入」。
check_field transport-new-rule-present L0 note '含本輪新增的 2 條也已載入'
# ④ **部分載入**：note 只准指名缺的那一條（已載入的那條被講成沒部署就是誤導）。
check_field transport-new-rule-partial L0 note 'AtlasUniverseRegistryNotPersisted' \
  'AtlasUniverseSnapshotNotPersisted'
# ⑤ registry 次要矛盾：note 指向偵測它的規則，且不得再出現「沒有對應告警」的舊敘述。
check_field artifact-registry-signal L5 note 'AtlasUniverseRegistryNotPersisted' '目前沒有對應告警'
# ⑥ registry 落地訊號要逐 stage 印出（值本身不改判層，但它必須看得見）。
check_field artifact-registry-signal L5 evidence \
  'verdict 的產物訊號 atlas_universe_last_run_registry_persisted: daily=0, weekly=0'

echo "→ mutation 自證（改壞一個判定式 ⇒ 對應的 fixture 必須紅）"

check_mutation heartbeat-overdue schedule-heartbeat-overdue L1 OK \
  'return next_run is not None and next_run + grace_seconds < now' 'return False'

check_mutation last-run-missing schedule-last-run-missed L1 PENDING \
  'return t_exp is not None and not done_after and now > t_exp + grace_seconds' 'return False'

check_mutation output-family-missing transport-family-missing L0 WARN \
  'return metrics_ok and not family_present' 'return False'

check_mutation scoring-broken scoring-ranked-zero L3 OK \
  'return _int_field(result, "symbols_ranked") == 0' 'return False'

check_mutation input-unusable input-partial L2 OK \
  'if status != "ok":
        return True
    return (_int_field(result, "quotes_missing_fetch_error") or 0) + (
        _int_field(result, "quotes_missing_not_attempted") or 0
    ) > 0' 'return False'

check_mutation label-shape-bad emission-label-shape L4 OK \
  'return bool(problems)' 'return False'

check_mutation emission-missing emission-counter-silent L4 OK \
  'return bool(ranked_positive) and bool(fresh_stages) and bool(zero_increase_stages)' 'return False'

check_mutation artifact-stale artifact-stale L5 OK \
  'return mtime is not None and claim is not None and mtime < claim - ARTIFACT_STALE_SECONDS' 'return False'

# 本輪新增的判定式（L0 對 NEW_RULES 的逐條比對：「已載入 = 已部署」）。兩個方向各一個
# mutation：只做一個方向的話，錯誤發生在另一邊時不會被咬住。
# ① 把「已載入即視為已部署」改成恆假（永遠把全部新規則當缺席）⇒ 部署完成的 case 必須變 WARN。
check_mutation new-rule-deployed transport-new-rule-present L0 WARN \
  'missing_new = [r for r in NEW_RULES if r not in loaded]' 'missing_new = list(NEW_RULES)'

# ② 把比對反過來（只把「已載入的」當缺席）⇒ 全部新規則都沒載入的 case 必須變 OK（假綠）。
check_mutation new-rule-missing green L0 OK \
  'missing_new = [r for r in NEW_RULES if r not in loaded]' 'missing_new = [r for r in NEW_RULES if r in loaded]'

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-verify-universe-run: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-verify-universe-run: $PASS passed（22 個 fixture case + 10 個 mutation + 7 個 note/evidence 斷言 + --help）"
