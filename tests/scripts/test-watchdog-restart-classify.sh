#!/usr/bin/env bash
# tests/scripts/test-watchdog-restart-classify.sh — scripts/ops/imac-container-watchdog.sh 的
# 「計畫性部署 vs 異常重啟」分類契約測試（FU-20260930-10 方案②；2026-09-30）
#
# 為什麼要有它
# ------------
# 這支 watchdog 是 Mac Mini 上**唯一**的容器自動復原機制，它的 restart ledger 是「重啟歸因」
# 的唯一證據。本次改動的價值全在**分類**：同一個「狀態變化」事件要分成
#   RESTART-DETECT  （同映像重啟 ⇒ warning；崩潰迴圈不得靜音）
#   RESTART-EXPECTED（計畫性部署 ⇒ expected／INFO；**仍寫 ledger**）
# 分錯邊的兩種代價都很大，而且**當下沒有任何徵兆**（ledger 多一行、少一則通知）：
#   · 異常重啟被歸成計畫性 ⇒ 崩潰迴圈被靜音（嚴重）
#   · 計畫性部署被歸成異常 ⇒ 每次部署一則 Telegram warning（= 本次要修的問題）
# 因此這裡用 fixture 把每個判定方向釘住，再用 mutation 證明斷言有牙齒。
#
# case 清單（8）
#   1 image-change        換映像、無窗口標記            ⇒ RESTART-EXPECTED／why=image-change ＋**仍寫 ledger**
#   2 same-image          同映像、無窗口標記            ⇒ RESTART-DETECT／why=same-image（warning 不變）
#                         （fixture 的舊 ledger 刻意有**兩筆**、image 不同 ⇒ 釘住「以最後一筆為基準」）
#   3 same-image-marker   同映像 ＋ 窗口標記            ⇒ RESTART-EXPECTED／why=deploy-window
#   4 legacy-ledger       **舊格式 ledger 行**（無 image=）⇒ RESTART-DETECT／why=image-unknown
#                         ★ 保守預設：未知 ≠ 變更；「我不知道」不得降級（業主指定）
#   5 legacy-ledger-marker 舊格式行 ＋ 窗口標記         ⇒ RESTART-EXPECTED／why=deploy-window
#                         （窗口標記是**獨立**證據；未知映像不得「單獨」降級 ⇒ 兩者不衝突，見腳本檔頭）
#   6 baseline            無 state 檔                   ⇒ 仍寫 BASELINE 行（含 image=）＝偵測能力未變
#   7 no-event            無狀態變化、但 ledger 的 image 落後 ⇒ **一行都不寫**
#                         （比較鍵仍是 info 6 欄 ⇒ 升級當天不會替每個容器多發一筆假重啟）
#   8 consumer-parse      既有消費者（a2a-dev 通知橋）的解析契約（見下）
#
# case 8 為什麼是斷言而不是註解：改 ledger 欄位時真正的風險是**別的 repo 的解析器**。
# 被抄進本檔的是 a2a-dev `scripts/ops/watchdog-ledger-notify.sh`（Mac Mini ~/bin/ 安裝副本）
# scan() 內那兩條規則的**原樣**（2026-09-30 版；本檔只去掉它的 time-floor 那一段）：
#   · kind = $3（第 3 欄；`gsub(/:$/, "", kind)`）
#   · name = 行內**第一個** `atlas-[a-z0-9-]+`
# 並斷言三件事：① 預設 pattern（RESTART-DETECT）只咬到異常那筆（計畫性預設不通知）
# ② 容器名不會被 `image_name=atlas-…` 搶走 ③ 新寫的 `INFO:` 行不會被主 ledger 的
# pattern（WATCH|OK|ERROR|WARN）咬到（避免同一件事被通知兩次）。
# 選配：A2A_DEV_WLN=/path/to/watchdog-ledger-notify.sh 會**真的**跑那支腳本做端到端複核
# （跨 repo，預設不跑 —— CI 沒有 a2a-dev checkout；本檔的 inline 規則才是常駐契約）。
#
# mutation（自證斷言有牙齒；目標行必須**恰好命中一次**，否則 FAIL —— 改錯地方比不改更糟）
#   image-compare-inverted  映像比較反過來（!= 改 =）⇒ case 1 必須由 EXPECTED 翻成 DETECT
#   marker-ignored          窗口標記判定改成恆假        ⇒ case 3 必須由 EXPECTED 翻成 DETECT
#
# hermetic
# --------
# 只寫 mktemp 目錄；**不打真 docker**（受測副本只把 `DOCKER=` 這一行指向 stub —— 刻意不在
# 生產腳本上加「可換掉 docker」的環境變數：那等於在唯一的自動復原路徑上開一個測試孔）、
# 不連網、不碰 production、不動呼叫者 repo（跑完比對受測腳本 sha256 未變）。
# 生產腳本的 `HOME` 只用來推導 LOG／STATE_DIR／RESTART_LEDGER，所以 fixture 用假 HOME 即可
# 完全隔離；窗口標記一律以 `A2A_DEPLOY_MARKER` 指到 fixture 路徑（**絕不**讀生產的
# /tmp/atlas-deploy-window）。
#
# 執行： bash tests/scripts/test-watchdog-restart-classify.sh   （make ci-gate / make test-scripts 會跑）
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/scripts/ops/imac-container-watchdog.sh"

TMP="$(mktemp -d)"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0; FAIL=0
ok()   { PASS=$((PASS + 1)); }
fail() { echo "    FAIL: $*"; FAIL=$((FAIL + 1)); }

if [ ! -f "$SRC" ]; then
  echo "❌ 找不到 $SRC"
  exit 1
fi
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}
SRC_SHA_BEFORE="$(sha256_of "$SRC")"

# ── 受測腳本的契約常數（腳本改了這裡就要跟著改；不一致 ⇒ FAIL，不靜默）──────────
CONTAINERS='atlas-go atlas-postgres'
NAME_GO=atlas-go
NAME_PG=atlas-postgres
DOCKER_LINE='DOCKER=/usr/local/bin/docker'
IMGNAME='atlas-atlas:v2'   # 刻意以 atlas- 開頭：證明 image_name= 不會搶走容器名（case 8）

# 6 欄狀態字串 = RestartCount|StartedAt|FinishedAt|ExitCode|OOMKilled|Error（與腳本同一條 format）
INFO_A_OLD='0|2026-09-29T08:00:00.000000000Z|0001-01-01T00:00:00Z|0|false|'
INFO_A_EVEN_OLDER='0|2026-09-29T01:00:00.000000000Z|0001-01-01T00:00:00Z|0|false|'
INFO_A_NEW='0|2026-09-30T05:33:46.000000000Z|0001-01-01T00:00:00Z|0|false|'
INFO_PG='0|2026-09-23T01:00:00.000000000Z|0001-01-01T00:00:00Z|0|false|'
IMG_OLD='111111111111'
IMG_NEW='222222222222'
IMG_PG='333333333333'
IMGNAME_OLD='atlas-atlas:v1'

hex64() {  # hex64 <前 12 碼> <填充字元> → `sha256:<64 碼>`（模擬 docker 的 image ID）
  local out="$1" i=0
  while [ "$i" -lt 52 ]; do out="${out}$2"; i=$((i + 1)); done
  printf 'sha256:%s' "$out"
}

# ── fake docker（stub）：只回答 watchdog 真的會問的 4 種問題 ────────────────────
STUB="$TMP/docker-stub"
cat > "$STUB" <<'STUBEOF'
#!/usr/bin/env bash
# fake docker（僅測試用）：答案來自 $FAKE_DOCKER_DATA/<name>.{info,image,imagename,status}
sub="$1"; shift
case "$sub" in
  inspect)
    fmt=""; name=""
    while [ $# -gt 0 ]; do
      case "$1" in
        -f) fmt="$2"; shift 2 ;;
        *)  name="$1"; shift ;;
      esac
    done
    f="$FAKE_DOCKER_DATA/$name"
    case "$fmt" in
      *RestartCount*) [ -f "$f.info" ]      || exit 1; cat "$f.info" ;;
      *State.Status*) [ -f "$f.status" ]    || exit 1; cat "$f.status" ;;
      *Config.Image*) [ -f "$f.imagename" ] || exit 1; cat "$f.imagename" ;;
      *.Image*)       [ -f "$f.image" ]     || exit 1; cat "$f.image" ;;
      *) exit 1 ;;
    esac
    ;;
  start) exit 0 ;;
  *) exit 1 ;;
esac
STUBEOF
chmod +x "$STUB"

# ── harness ───────────────────────────────────────────────────────────────────
# mutate_line <file> <完整舊行> <新行>：就地替換**整行**；命中必須恰好一次（否則回 1）。
# 用 bash 逐行讀寫（不用 sed／python）：舊行含 $、引號、反斜線，這裡全部原樣搬運。
mutate_line() {
  local file="$1" old="$2" new="$3" n=0 line out="$1.tmp"
  : > "$out"
  while IFS= read -r line || [ -n "$line" ]; do
    if [ "$line" = "$old" ]; then
      n=$((n + 1))
      printf '%s\n' "$new" >> "$out"
    else
      printf '%s\n' "$line" >> "$out"
    fi
  done < "$file"
  if [ "$n" != "1" ]; then
    rm -f "$out"
    return 1
  fi
  mv "$out" "$file"
  return 0
}

SCRIPT_UNDER_TEST="$SRC"

fresh_dir() {  # fresh_dir <dir>：骨架 + 受測副本（只換 DOCKER= 一行）
  local d="$1"
  rm -rf "$d"
  mkdir -p "$d/home/Library/Logs/atlas-watchdog-state" "$d/data"
  cp "$SCRIPT_UNDER_TEST" "$d/script"
  mutate_line "$d/script" "$DOCKER_LINE" "DOCKER=$STUB" || {
    fail "受測副本：$DOCKER_LINE 在受測腳本裡不是恰好一次命中"
    return 1
  }
  return 0
}

watchdog_log() { printf '%s' "$1/home/Library/Logs/atlas-watchdog.log"; }
ledger()       { printf '%s' "$1/home/Library/Logs/atlas-container-restarts.log"; }

set_container() {  # set_container <dir> <name> <info> <image> <image_name> <status>
  local d="$1" n="$2"
  printf '%s\n' "$3" > "$d/data/$n.info"
  printf '%s\n' "$4" > "$d/data/$n.image"
  printf '%s\n' "$5" > "$d/data/$n.imagename"
  printf '%s\n' "$6" > "$d/data/$n.status"
}

set_state() {  # set_state <dir> <name> <上次觀測到的 6 欄狀態>
  printf '%s\n' "$3" > "$1/home/Library/Logs/atlas-watchdog-state/$2.state"
}

# 模擬「前次寫入的歷史行」；image 給空字串 ⇒ 產生**舊格式**行（沒有 image= 欄位）
ledger_prev_line() {  # ledger_prev_line <kind> <name> <image 或空>
  if [ -z "$3" ]; then
    printf '2026-09-29 08:00:00 %s %s new=%s prev=%s\n' "$1" "$2" "$INFO_A_EVEN_OLDER" "$INFO_A_OLD"
  else
    printf '2026-09-29 08:00:00 %s %s new=%s prev=%s image=%s image_name=%s\n' \
      "$1" "$2" "$INFO_A_EVEN_OLDER" "$INFO_A_OLD" "$3" "$IMGNAME_OLD"
  fi
}

run_watchdog() {  # run_watchdog <dir>
  local d="$1"
  ( cd "$d" && HOME="$d/home" FAKE_DOCKER_DATA="$d/data" A2A_DEPLOY_MARKER="$d/marker" \
      bash "$d/script" > "$d/stdout.log" 2>&1 )
  return 0
}

last_line_of() {  # last_line_of <ledger> <container> → 最後一筆提到該容器的行
  grep -F " $2 " "$1" 2>/dev/null | tail -1
}
lines_of() {  # lines_of <file> <container> → 行數
  local n
  n=$(grep -c -F " $2 " "$1" 2>/dev/null)
  case "$n" in ''|*[!0-9]*) n=0 ;; esac
  printf '%s' "$n"
}
kind_of()   { awk '{ print $3 }' <<<"$1"; }
field_of() {  # field_of <line> <key> → key=value 的值（找不到 ⇒ 空）
  local k="$2" tok
  for tok in $1; do
    case "$tok" in "$k"=*) printf '%s' "${tok#"$k"=}"; return 0 ;; esac
  done
  return 0
}

CASE_LINE=""
expect_kind() {  # expect_kind <label> <line> <期望 kind>
  local got
  got="$(kind_of "$2")"
  if [ "$got" = "$3" ]; then ok; else fail "$1: kind=$got（期望 $3）；行：$2"; fi
}
expect_field() {  # expect_field <label> <line> <key> <期望值>
  local got
  got="$(field_of "$2" "$3")"
  if [ "$got" = "$4" ]; then ok; else fail "$1: $3=$got（期望 $4）；行：$2"; fi
}
expect_no_line() {  # expect_no_line <label> <ledger> <container>
  local n
  n="$(lines_of "$2" "$3")"
  if [ "$n" = "0" ]; then ok; else fail "$1: 不該有 $3 的行，卻有 $n 行"; fi
}
show_event() { echo "      產生的 ledger 行： $1"; }

# ── 8 個 case ─────────────────────────────────────────────────────────────────
# fixture 共用：兩台容器都在 running；postgres 的 state 一律等於現值（不干擾斷言）
base_containers() {  # base_containers <dir> <atlas-go 的 image> <atlas-go 的 info>
  set_container "$1" "$NAME_GO" "$3" "$2" "$IMGNAME" running
  set_container "$1" "$NAME_PG" "$INFO_PG" "$(hex64 "$IMG_PG" c)" "$IMGNAME" running
  set_state "$1" "$NAME_PG" "$INFO_PG"
}

case_1_image_change() {  # 換映像 ⇒ planned
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_OLD"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "$IMG_OLD" >> "$(ledger "$d")"
  run_watchdog "$d"
}

case_2_same_image() {  # 同映像 ⇒ 仍 warning（且以**最後一筆** image 為基準）
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_OLD"
  # 兩筆歷史：較舊的 image 不同（若實作抓「第一筆」就會被誤判成換映像 ⇒ 這行 fixture 有牙齒）
  ledger_prev_line RESTART-DETECT "$NAME_GO" "$IMG_OLD" >> "$(ledger "$d")"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "$IMG_NEW" >> "$(ledger "$d")"
  run_watchdog "$d"
}

case_3_same_image_marker() {  # 同映像 ＋ 部署窗口標記 ⇒ planned
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_OLD"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "$IMG_NEW" >> "$(ledger "$d")"
  : > "$d/marker"
  run_watchdog "$d"
}

case_4_legacy_ledger() {  # 舊格式行（無 image=）＋ 無標記 ⇒ warning／why=image-unknown（★）
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_OLD"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "" >> "$(ledger "$d")"
  run_watchdog "$d"
}

case_5_legacy_ledger_marker() {  # 舊格式行 ＋ 標記 ⇒ planned（標記是獨立證據）
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_OLD"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "" >> "$(ledger "$d")"
  : > "$d/marker"
  run_watchdog "$d"
}

case_6_baseline() {  # 無 state 檔 ⇒ BASELINE（含 image=）
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  run_watchdog "$d"
}

case_7_no_event() {  # state 與現值相同 ⇒ 一行都不寫（即使 ledger 的 image 落後）
  local d="$1"
  fresh_dir "$d" || return 1
  base_containers "$d" "$(hex64 "$IMG_NEW" b)" "$INFO_A_NEW"
  set_state "$d" "$NAME_GO" "$INFO_A_NEW"
  ledger_prev_line RESTART-DETECT "$NAME_GO" "$IMG_OLD" >> "$(ledger "$d")"
  run_watchdog "$d"
}

# ── 消費者的解析規則（原樣抄自 a2a-dev scripts/ops/watchdog-ledger-notify.sh scan()）──
# 只拿掉它的 time-floor（WLN_LOOKBACK_S）那一段：本檔要驗的是**欄位契約**，不是回溯窗。
consumer_scan() {  # consumer_scan <file> <kind-ERE> → 每行印 "kind<TAB>name"
  awk -v filter="$2" '
    {
      kind = $3; gsub(/:$/, "", kind)
      if (filter != "" && kind !~ ("^(" filter ")$")) next
      name = ""
      if (match($0, /atlas-[a-z0-9-]+/)) name = substr($0, RSTART, RLENGTH)
      printf "%s\t%s\n", kind, name
    }' "$1"
}

case_8_consumer_parse() {
  local d="$TMP/consumer-parse" led="$d/ledger" planned_line warn_line
  local WLN_RESTART_PATTERN_DEFAULT='RESTART-DETECT'    # 通知橋的預設值（照抄）
  local WLN_PATTERNS_DEFAULT='WATCH|OK|ERROR|WARN'      # 通知橋對**主** ledger 的預設值
  mkdir -p "$d"
  : > "$led"
  planned_line="$(grep -E ' RESTART-EXPECTED ' "$(ledger "$TMP/image-change")" | tail -1)"
  warn_line="$(grep -E ' RESTART-DETECT ' "$(ledger "$TMP/same-image")" | tail -1)"
  printf '%s\n%s\n' "$planned_line" "$warn_line" > "$led"
  echo "      抄自通知橋的輸入（2 行）："
  sed 's/^/        /' "$led"

  # ① 預設 pattern ⇒ 只有異常那筆會被當成 action（計畫性預設不通知）
  local def_count
  def_count="$(consumer_scan "$led" "$WLN_RESTART_PATTERN_DEFAULT" | wc -l | tr -d ' ')"
  if [ "$def_count" = "1" ]; then
    ok
  else
    fail "consumer-parse: 預設 pattern 命中 $def_count 行（期望 1：只有 RESTART-DETECT）"
  fi
  # ② kind 仍是第 3 欄、且 opt-in pattern 兩筆都咬得到
  local both
  both="$(consumer_scan "$led" 'RESTART-DETECT|RESTART-EXPECTED')"
  if [ "$(printf '%s\n' "$both" | grep -c '^RESTART-EXPECTED')" = "1" ] &&
     [ "$(printf '%s\n' "$both" | grep -c '^RESTART-DETECT')" = "1" ]; then
    ok
  else
    fail "consumer-parse: opt-in pattern 未正確解出兩個 kind：$both"
  fi
  # ③ 容器名 = 行內第一個 atlas-… ⇒ 不得被 image_name=atlas-atlas:v2 搶走
  local names
  names="$(printf '%s\n' "$both" | cut -f2 | sort -u | tr '\n' ',')"
  if [ "$names" = "$NAME_GO," ]; then
    ok
  else
    fail "consumer-parse: 容器名解析成 '$names'（期望 '$NAME_GO,'；image_name= 搶走了嗎？）"
  fi
  # ④ 新的 INFO: 行不得被主 ledger 的 pattern 咬到（否則同一件事會被通知兩次）
  local main_hits
  main_hits="$(consumer_scan "$(watchdog_log "$TMP/image-change")" "$WLN_PATTERNS_DEFAULT" | grep -c '^WARN' || true)"
  if [ "$main_hits" = "0" ]; then
    ok
  else
    fail "consumer-parse: 主 ledger 掃到 $main_hits 筆 WARN（計畫性重啟不該發 warning）"
  fi
  # 選配：真的跑 a2a-dev 的通知橋（跨 repo；預設不跑）
  if [ -n "${A2A_DEV_WLN:-}" ] && [ -f "${A2A_DEV_WLN}" ]; then
    local out
    out="$(WLN_RESTART_LEDGER="$led" WLN_LEDGER="$d/absent" WLN_STATE="$d/wln.state" \
           WLN_LOG="$d/wln.log" WLN_NOTIFY_CMD="$d/no-notify" WLN_LOOKBACK_S=0 \
           bash "${A2A_DEV_WLN}" --dry-run --from-start 2>&1)"
    echo "      （選配）真通知橋輸出：$out"
  fi
  return 0
}

# ── mutation 自證 ─────────────────────────────────────────────────────────────
# 沒有這一節，「8 個 case 全綠」只證明斷言沒有牙齒。每個 mutation 都必須
# (a) 恰好命中一次（否則 FAIL）(b) 讓指定 case 的 kind 由 baseline 翻成 mutated 值。
MUT_IMAGE_OLD='        elif [ "$cur_image" != "$prev_image" ]; then'
MUT_IMAGE_NEW='        elif [ "$cur_image" = "$prev_image" ]; then'
MUT_MARKER_OLD='        if [ "$marker" = 1 ]; then'
MUT_MARKER_NEW='        if [ 1 = 0 ]; then'

check_mutation() {  # check_mutation <label> <case 函式名> <舊行> <新行> <baseline kind> <mutated kind>
  local label="$1" fn="$2" old="$3" new="$4" want_base="$5" want_mut="$6"
  local mdir="$TMP/mut-$label" mcopy="$TMP/mut-$label-script"
  cp "$SRC" "$mcopy"
  if ! mutate_line "$mcopy" "$old" "$new"; then
    fail "mutation/$label: 目標行在原始碼裡不是恰好一次命中（改錯地方）"
    return
  fi
  # ① baseline（用已跑過的 case 的輸出；呼叫端保證該 case 先跑過）
  local base_line
  base_line="$(last_line_of "$(ledger "$TMP/$fn")" "$NAME_GO")"
  if [ "$(kind_of "$base_line")" != "$want_base" ]; then
    fail "mutation/$label: baseline kind=$(kind_of "$base_line")（期望 $want_base）⇒ fixture 沒隔離這一層"
    return
  fi
  # ② 用受變異的副本重跑同一個 case
  SCRIPT_UNDER_TEST="$mcopy"
  "case_$fn" "$mdir" || { SCRIPT_UNDER_TEST="$SRC"; return; }
  SCRIPT_UNDER_TEST="$SRC"
  local mut_line
  mut_line="$(last_line_of "$(ledger "$mdir")" "$NAME_GO")"
  if [ "$(kind_of "$mut_line")" = "$want_mut" ]; then
    echo "      mutation/${label}: $(kind_of "$base_line") -> $(kind_of "$mut_line")（咬住了）"
    ok
  else
    fail "mutation/$label: 改壞判定式後 kind=$(kind_of "$mut_line")（期望 $want_mut）⇒ mutation 沒被咬住"
  fi
}

# ── 執行 ──────────────────────────────────────────────────────────────────────
echo "→ watchdog 重啟分類契約測試（fixture ＋ mutation；hermetic，不打真 docker）"

containers_line="$(grep -m1 '^CONTAINERS=' "$SRC")"
if [ "$containers_line" = "CONTAINERS=\"$CONTAINERS\"" ]; then
  ok
else
  fail "受測腳本的 CONTAINERS= 與本測試的 fixture 不一致（$containers_line）"
fi

# 1. 換映像 ⇒ INFO ＋ 仍寫 ledger
echo "  case 1 image-change（換映像 ⇒ RESTART-EXPECTED）"
case_1_image_change "$TMP/image-change"
C1_BEFORE=1     # fixture 先寫了 1 筆舊行
C1_LINE="$(last_line_of "$(ledger "$TMP/image-change")" "$NAME_GO")"
check_event_kind() { :; }   # （保留給未來擴充；本檔用下面的顯式斷言）
expect_kind  "case1" "$C1_LINE" RESTART-EXPECTED
expect_field "case1" "$C1_LINE" why image-change
expect_field "case1" "$C1_LINE" image "$IMG_NEW"
expect_field "case1" "$C1_LINE" prev_image "$IMG_OLD"
expect_field "case1" "$C1_LINE" marker 0
if [ "$(lines_of "$(ledger "$TMP/image-change")" "$NAME_GO")" = "$((C1_BEFORE + 1))" ]; then
  ok
else
  fail "case1: ledger 未增加（計畫性重啟仍必須寫 ledger）"
fi
if grep -q 'INFO: .*restart is planned' "$(watchdog_log "$TMP/image-change")"; then
  ok
else
  fail "case1: watchdog log 沒有 INFO 行（通知應降為 INFO，而不是消失）"
fi
if grep -q 'WARN:' "$(watchdog_log "$TMP/image-change")"; then
  fail "case1: 計畫性重啟不該產生 WARN"
else
  ok
fi
show_event "$C1_LINE"

# 2. 同映像 ⇒ warning（仍寫 ledger、kind 不變）
echo "  case 2 same-image（同映像 ⇒ RESTART-DETECT）"
case_2_same_image "$TMP/same-image"
C2_LINE="$(last_line_of "$(ledger "$TMP/same-image")" "$NAME_GO")"
expect_kind  "case2" "$C2_LINE" RESTART-DETECT
expect_field "case2" "$C2_LINE" why same-image
expect_field "case2" "$C2_LINE" prev_image "$IMG_NEW"
show_event "$C2_LINE"

# 3. 同映像 ＋ 標記 ⇒ INFO
echo "  case 3 same-image-marker（同映像＋窗口標記 ⇒ RESTART-EXPECTED）"
case_3_same_image_marker "$TMP/same-image-marker"
C3_LINE="$(last_line_of "$(ledger "$TMP/same-image-marker")" "$NAME_GO")"
expect_kind  "case3" "$C3_LINE" RESTART-EXPECTED
expect_field "case3" "$C3_LINE" why deploy-window
expect_field "case3" "$C3_LINE" marker 1
show_event "$C3_LINE"

# 4. 舊格式 ledger 行（無 image=）⇒ 不降級（★ 保守預設）
echo "  case 4 legacy-ledger（舊行無 image= ⇒ RESTART-DETECT／why=image-unknown）"
case_4_legacy_ledger "$TMP/legacy-ledger"
C4_LINE="$(last_line_of "$(ledger "$TMP/legacy-ledger")" "$NAME_GO")"
expect_kind  "case4" "$C4_LINE" RESTART-DETECT
expect_field "case4" "$C4_LINE" why image-unknown
show_event "$C4_LINE"

# 5. 舊格式行 ＋ 標記 ⇒ planned（標記是獨立證據）
echo "  case 5 legacy-ledger-marker（舊行＋標記 ⇒ RESTART-EXPECTED）"
case_5_legacy_ledger_marker "$TMP/legacy-ledger-marker"
C5_LINE="$(last_line_of "$(ledger "$TMP/legacy-ledger-marker")" "$NAME_GO")"
expect_kind  "case5" "$C5_LINE" RESTART-EXPECTED
expect_field "case5" "$C5_LINE" why deploy-window
show_event "$C5_LINE"

# 6. 無 state 檔 ⇒ BASELINE（偵測能力未變）
echo "  case 6 baseline（無 state 檔 ⇒ BASELINE 含 image=）"
case_6_baseline "$TMP/baseline"
C6_LINE="$(last_line_of "$(ledger "$TMP/baseline")" "$NAME_GO")"
expect_kind  "case6" "$C6_LINE" BASELINE
expect_field "case6" "$C6_LINE" image "$IMG_NEW"
show_event "$C6_LINE"

# 7. 無狀態變化 ⇒ 一行都不寫
echo "  case 7 no-event（state 與現值相同 ⇒ 不寫任何行）"
case_7_no_event "$TMP/no-event"
expect_no_line "case7" "$(ledger "$TMP/no-event")" "$NAME_GO"

# 8. 既有消費者的解析契約
echo "  case 8 consumer-parse（通知橋的解析規則）"
case_8_consumer_parse

echo "  mutation 自證（改壞判定式 ⇒ 對應 case 必紅）"
check_mutation image-compare-inverted image-change \
  "$MUT_IMAGE_OLD" "$MUT_IMAGE_NEW" RESTART-EXPECTED RESTART-DETECT
check_mutation marker-ignored same-image-marker \
  "$MUT_MARKER_OLD" "$MUT_MARKER_NEW" RESTART-EXPECTED RESTART-DETECT

# ── 唯讀斷言：受測腳本必須一字未改 ───────────────────────────────────────────
SRC_SHA_AFTER="$(sha256_of "$SRC")"
if [ "$SRC_SHA_AFTER" = "$SRC_SHA_BEFORE" ]; then
  ok
else
  fail "受測腳本被本測試改動了（$SRC_SHA_BEFORE -> $SRC_SHA_AFTER）"
fi

echo ""
echo "✅ test-watchdog-restart-classify: $PASS passed, $FAIL failed"
[ "$FAIL" = "0" ] || exit 1
exit 0
