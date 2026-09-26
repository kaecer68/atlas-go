#!/usr/bin/env bash
# negative-proof-lib.sh — 「負向證明」的共用斷言庫：**精確 exit code**，不把 127/2 當成「擋下了」。
#
# 【為什麼需要這支】（issue #2011；由 PR #2003 的設計審查發現同類殘留）
#   舊寫法的負向證明長這樣：
#       if <受測命令>; then echo "❌ 沒擋下"; exit 1; fi
#       echo "✅ 擋下了"
#   它把「**非 0**」等同於「閘門有牙齒」。但下列非 0 **都不是**在擋東西：
#       2   = 用法／參數錯誤（受測命令根本沒走到判定邏輯）
#       124 = timeout
#       126 = 找到但不可執行
#       127 = 找不到命令（腳本被改名／搬走／相對路徑改了）
#   ⇒ 受測腳本被改名或參數寫錯時，負向證明照樣印 ✅ —— 這是「證明自己會擋」的測試**本身沒有鑑別力**。
#   2026-09-26 對修正前的兩處負向證明實測：餵「命令不存在(127)」與「用法錯誤(2)」兩種情境**都印 ✅**。
#
# 本庫只做一件事：把期望 exit code 寫成**精確相等**的斷言。任何其他值（含 0）都判為
# **測試本身失敗（不是「擋下了」）**，並在訊息裡說明那個 rc 代表什麼、怎麼修。
#
# 用法（在負向證明腳本內）：
#   . "$(dirname "${BASH_SOURCE[0]}")/negative-proof-lib.sh"
#   negproof_require_file "$REPO_ROOT/scripts/secret-scan.sh" "受測掃描器" || exit 1
#   negproof_expect 1 "含合成憑證的 tracked 檔必須被擋下" "$LOG" -- bash scripts/secret-scan.sh || { negproof_show_log "$LOG"; exit 1; }
#   negproof_expect_output '__negtest_secret__.py' "$LOG" "擋下的必須是那個 fixture" || { negproof_show_log "$LOG"; exit 1; }
#
# 契約：
#   * 呼叫端**不要**用 `set -e` 包住 negproof_expect（它自己處理 rc，回傳 0/1）。
#   * 呼叫端必須把回傳值當成腳本成敗（`... || exit 1`），否則本庫等於沒接線。
#   * 本檔不假設自己有執行權限（一律以 `.`/`source` 載入）。
set -uo pipefail

negproof_err() { echo "::error::$*" >&2; }

# negproof_rc_reason <rc> — 把結束碼翻譯成「它其實代表什麼」
negproof_rc_reason() {
  case "${1:-}" in
    ''|*[!0-9]*) echo "結束碼不是數字（'${1:-}'）" ;;
    0)   echo "受測命令**成功** ⇒ 它根本沒擋下任何東西" ;;
    2)   echo "用法／參數錯誤（exit 2）⇒ 受測命令沒走到判定邏輯" ;;
    124) echo "timeout（exit 124）⇒ 沒跑完" ;;
    126) echo "找到但不可執行（exit 126）⇒ 沒跑起來" ;;
    127) echo "找不到命令（exit 127）⇒ 受測腳本被改名／刪除／路徑不對" ;;
    *)   echo "非預期的結束碼（exit ${1}）" ;;
  esac
}

# negproof_require_file <path> <說明> — 受測檔案必須存在（缺了就不可能證明「它會擋」）
negproof_require_file() {
  local path="${1:-}" desc="${2:-受測檔案}"
  if [ -z "${path}" ] || [ ! -f "${path}" ]; then
    negproof_err "找不到 ${desc}：'${path}' ⇒ 受測命令不存在時，負向證明毫無意義（fail-closed）"
    return 1
  fi
  return 0
}

# negproof_expect <期望 rc> <說明> <log 檔> -- <命令…>
#   回傳 0 = 受測命令以**恰為期望值**的 rc 結束；1 = 測試本身失敗（含「沒擋下」與「非預期 rc」）
negproof_expect() {
  if [ "$#" -lt 4 ]; then
    negproof_err "negproof_expect 用法：negproof_expect <期望 rc> <說明> <log 檔> -- <命令…>（實際收到 $# 個參數）"
    return 1
  fi
  local expected="$1" desc="$2" log="$3"; shift 3
  [ "${1:-}" = "--" ] && shift
  if [ "$#" -eq 0 ]; then
    negproof_err "[${desc}] 沒有給受測命令（-- 之後是空的）"
    return 1
  fi
  # 空值防護：expected 空字串／非數字會讓 `[ "" -eq 1 ]` 變成 shell 錯誤（訊息會被吃掉，看起來像「沒擋下」）。
  case "${expected}" in
    ''|*[!0-9]*)
      negproof_err "[${desc}] 期望的 exit code 不是非負整數（'${expected}'）⇒ 空值/非數字一律視為測試失敗"
      return 1 ;;
  esac

  local rc=0
  "$@" >"${log}" 2>&1 || rc=$?

  if [ "${rc}" -eq "${expected}" ]; then
    echo "  ✅ [${desc}] 受測命令結束碼恰為 ${expected}（預期值）"
    return 0
  fi
  negproof_err "[${desc}] 期望 exit=${expected}（＝擋下），實際 exit=${rc}：$(negproof_rc_reason "${rc}")"
  echo "      受測命令：$*" >&2
  return 1
}

# negproof_expect_output <字串> <log 檔> <說明> — 證明「擋下的原因就是我們要證明的那件事」
#   （rc 對但檔錯／規則錯 ⇒ 也是假綠：證明了一個不相關的東西）
negproof_expect_output() {
  local pattern="${1:-}" log="${2:-}" desc="${3:-輸出內容}"
  if [ -z "${pattern}" ]; then
    negproof_err "[${desc}] 比對字串是空的 ⇒ 空字串會被 grep 當成「永遠命中」（空值即通過）"
    return 1
  fi
  if [ ! -f "${log}" ]; then
    negproof_err "[${desc}] 找不到 log 檔 '${log}'"
    return 1
  fi
  if grep -qF -- "${pattern}" "${log}"; then
    echo "  ✅ [${desc}] 輸出含 '${pattern}'"
    return 0
  fi
  negproof_err "[${desc}] 輸出不含 '${pattern}' ⇒ 擋下的**不是**我們要證明的東西（擋錯 = 沒證明到）"
  return 1
}

# negproof_show_log <log 檔> — 把受測命令輸出縮排印出（CI log 才看得到原因）
negproof_show_log() {
  local log="${1:-}"
  [ -f "${log}" ] || return 0
  echo "  ── 受測命令輸出（$(wc -l <"${log}" | tr -d ' ') 行）──"
  sed 's/^/     /' "${log}" | head -40
}
