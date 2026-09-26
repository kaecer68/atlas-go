#!/usr/bin/env bash
# tests/scripts/test-embed-dirs.sh — `make embed-dirs` 的 **hermetic** 契約測試
#
# 為什麼 hermetic：本測試只用 `mktemp` 目錄（透過 Makefile 的 `EMBED_DIRS` 注入點），
#   不建立 git worktree、不碰 repo 內的 admin_web/ 與 client_web/、不改任何 tracked 檔。
#   （對照：Makefile `test-scripts` 註解記錄 #1927 —— 其他 tests/scripts fixture 尚不 hermetic，
#     因此那個 target 仍是 opt-in；本檔不依賴那條路徑。）
#
# 契約（三條都必須成立，失敗即 exit 1）：
#   ① 目錄不存在 ⇒ 建立（含 .keep）、exit 0、印出「本機 dev 佔位」說明
#   ② 冪等：目錄已存在 ⇒ 不建立、不覆蓋、不補 .keep、不印「已建立」訊息、exit 0
#   ③ CI 環境（環境變數 CI 非空）⇒ 什麼都不做（不建立目錄）、exit 0、印出跳過訊息
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
FAIL=0

ok()  { echo "  ✅ $1"; }
bad() { echo "  ❌ $1"; FAIL=1; }

# ① 目錄不存在 ⇒ 建立（刻意 unset 環境的 CI：GitHub runner 預設 CI=true，否則本條會 no-op）
env -u CI make -C "${ROOT}" embed-dirs EMBED_DIRS="${TMP}/a/dist ${TMP}/b/dist" > "${TMP}/1.log" 2>&1
rc=$?
[ "${rc}" -eq 0 ] || bad "① exit 應為 0，實得 ${rc}"
if [ -f "${TMP}/a/dist/.keep" ] && [ -f "${TMP}/b/dist/.keep" ]; then ok "① 建立 .keep（兩個目錄）"; else bad "① 未建立 .keep"; fi
if grep -q "本機 dev 佔位" "${TMP}/1.log"; then ok "① 印出本機佔位說明"; else bad "① 未印出佔位說明"; fi

# ② 冪等：既有目錄與內容不得被動到
printf 'REAL_BUILD\n' > "${TMP}/a/dist/index.html"
rm -f "${TMP}/a/dist/.keep"
env -u CI make -C "${ROOT}" embed-dirs EMBED_DIRS="${TMP}/a/dist" > "${TMP}/2.log" 2>&1
rc=$?
[ "${rc}" -eq 0 ] || bad "② exit 應為 0，實得 ${rc}"
if [ "$(cat "${TMP}/a/dist/index.html")" = "REAL_BUILD" ]; then ok "② 既有檔案未被覆蓋"; else bad "② 既有檔案被覆蓋"; fi
if [ -f "${TMP}/a/dist/.keep" ]; then bad "② 對既有目錄補了 .keep"; else ok "② 不為既有目錄補 .keep"; fi
if grep -q "已建立" "${TMP}/2.log"; then bad "② 既有目錄仍印出建立訊息"; else ok "② 未印建立訊息"; fi

# ③ CI 環境 ⇒ no-op（且不得遮蔽：缺 dist 時 CI 端的 go build 仍會紅 —— 由 build 步驟負責）
CI=true make -C "${ROOT}" embed-dirs EMBED_DIRS="${TMP}/c/dist" > "${TMP}/3.log" 2>&1
rc=$?
[ "${rc}" -eq 0 ] || bad "③ exit 應為 0，實得 ${rc}"
if [ -d "${TMP}/c/dist" ]; then bad "③ CI 環境下竟然建立了目錄"; else ok "③ CI 環境不建立目錄（no-op）"; fi
if grep -q "CI 環境" "${TMP}/3.log"; then ok "③ 印出 CI 跳過訊息"; else bad "③ 未印出 CI 跳過訊息"; fi

if [ "${FAIL}" -eq 0 ]; then
  echo "✅ test-embed-dirs PASS（3 契約）"
  exit 0
fi
echo "❌ test-embed-dirs FAIL"
exit 1
