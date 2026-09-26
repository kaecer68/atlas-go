#!/usr/bin/env bash
# check_followups_unique_ids.sh — docs/operations/FOLLOWUPS.md 的條目 id 不得重複（**單檔**檢查）
#
# 【為什麼需要】（2026-09-26 實證：同一天內**三次**撞號）
#   兩個 lane 幾乎同時 push：各自「新增前都先查了最大號」✓，但**對方在查完之後才併入** ✗
#   ⇒ 同一條目被迫連續改號（-01 → -08 → -09）。光靠「push 前先查最大號」**不可能根治**（那是 race），
#   要在合併後仍然成立的**機制**：檔案層級的 id 唯一性。
#
# 【為什麼「單檔」就夠】（不需要跨 PR 狀態）
#   FOLLOWUPS 的 id 唯一性**只在單一檔案內成立** ⇒ 只要斷言「同一份檔案裡沒有兩個相同的
#   `### FU-YYYYMMDD-NN`」，就能抓到「合併後撞號」（正是今天發生的形狀）✓。
#
# 【刻意不做的事】
#   **不**斷言「新增條目必須是 max+1」：刻意留空號、或同一條目分階段合併（先開票後補內容）
#   都會被誤判 ⇒ 太嚴。唯一性（本檢查）已足以攔下真正要防的『兩個同號並存』。
#
# 【斷言】任一條不成立 → 印出可行動訊息 + exit 1（fail-closed）
#   ① 檔案存在（預期 docs/operations/FOLLOWUPS.md）
#   ② 抓到 ≥1 個 `### FU-…` 標題（0 個 ⇒ 格式可能已變 ⇒ 不得靜默變成「永遠 PASS 的空檢查」）
#   ③ 沒有重複的 id
#
# 【測試】tests/scripts/test-followups-unique-ids.sh（hermetic fixture；用 FOLLOWUPS_FILE 注入）
set -uo pipefail

FILE="${FOLLOWUPS_FILE:-docs/operations/FOLLOWUPS.md}"

if [ ! -f "${FILE}" ]; then
  echo "❌ FOLLOWUPS id 唯一性檢查：找不到檔案 ${FILE}"
  echo "   預期路徑：docs/operations/FOLLOWUPS.md（FOLLOWUPS_FILE 只給測試注入用，CI 內不得覆寫）"
  exit 1
fi

IDS="$(grep -oE '^### FU-[0-9]{8}-[0-9]{2}' "${FILE}" | sed 's/^### //' || true)"
COUNT="$(printf '%s\n' "${IDS}" | grep -c . || true)"

if [ "${COUNT}" -eq 0 ]; then
  echo "❌ FOLLOWUPS id 唯一性檢查：在 ${FILE} 找不到任何 \`### FU-YYYYMMDD-NN\` 標題"
  echo "   ⇒ 這通常代表**檔案格式已改**（標題層級或 id 樣式變了）。"
  echo "     請同步更新本檢查（scripts/ci/check_followups_unique_ids.sh），"
  echo "     不要讓它靜默退化成「永遠 PASS 的空檢查」（fail-closed）。"
  exit 1
fi

DUPS="$(printf '%s\n' "${IDS}" | sort | uniq -d)"
if [ -n "${DUPS}" ]; then
  echo "❌ FOLLOWUPS id 重複（${FILE}）："
  printf '%s\n' "${DUPS}" | sed 's/^/   - /'
  echo "   ⇒ 可行動處置（任一）："
  echo "      ① 把其中一個改成**同系列 max+1**（同一天的下一個可用號）；"
  echo "      ② 把內容**併入既有條目**（同一 id 之下寫清楚兩件事）；"
  echo "      ③ 兩個條目其實是同一件事被兩個 lane 各寫一次 ⇒ 合併後刪掉重複的那份。"
  echo "   成因：兩個 lane 各自「新增前查了最大號」但對方在查完之後才併入（race）⇒ 需要本機制。"
  exit 1
fi

echo "✅ FOLLOWUPS id 唯一性 PASS（${COUNT} 個條目，無重複）"
