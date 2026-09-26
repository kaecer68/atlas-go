#!/usr/bin/env bash
# test-followups-unique-ids.sh — 「FOLLOWUPS id 不得重複」檢查的自我測試（hermetic）
#
# 為什麼要有它：護欄本身必須有牙齒。若它只會印 PASS，就等於沒有。
# 本測試在 tempdir 造 fixture，用 FOLLOWUPS_FILE 注入（**不碰真檔**、不碰生產、不需要 docker）：
#   P 正向：id 唯一 → PASS(0)
#   N1 負向：兩個同號 → 必 FAIL(1)，且訊息要指出號碼與可行動建議
#   N2 負向：檔案不存在 → 必 FAIL(1)
#   N3 負向：檔案存在但 0 個 `### FU-…`（格式漂移）→ 必 FAIL(1)（不得靜默變空檢查）
#   T  牙齒：負向案例必須是**真的非零 exit**（不可只印字而回 0）
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$ROOT/scripts/ci/check_followups_unique_ids.sh"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

echo "── FOLLOWUPS id 唯一性檢查自我測試（合成 fixture，不碰真檔）──"

run_case() {  # $1=名稱 $2=期望 exit $3=期望輸出子字串（可空）
  FOLLOWUPS_FILE="${TMP}/fx.md" bash "${CHECK}" > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" = "$2" ]; then ok "$1（exit=${rc}）"; else bad "$1（exit=${rc}，期望 $2）"; sed 's/^/     /' "${TMP}/out.txt" | head -8; fi
  if [ -n "${3:-}" ]; then
    if grep -q "$3" "${TMP}/out.txt"; then ok "$1：訊息含可辨識字串（$3）"
    else bad "$1：訊息缺少「$3」"; sed 's/^/     /' "${TMP}/out.txt" | head -8; fi
  fi
}

# P 正向：唯一 id（含不同日期序列並存 ⇒ 不是重複）
cat > "${TMP}/fx.md" <<'EOF'
## 遺留項目

### FU-20260925-01 — 第一條

- **狀態**：`open`

### FU-20260925-02 — 第二條

- **狀態**：`done`

### FU-20260926-01 — 不同日期的序列

- **狀態**：`open`
EOF
run_case "P 唯一 id PASS" 0 "PASS"

# N1：兩個同號（模擬兩個 lane 各寫一份後合併）
cat > "${TMP}/fx.md" <<'EOF'
### FU-20260926-07 — 條目 A

- **狀態**：`open`

### FU-20260926-08 — 條目 B

- **狀態**：`open`

### FU-20260926-08 — 條目 B（另一個 lane 也用了同號）

- **狀態**：`open`
EOF
run_case "N1 同號重複 FAIL" 1 "FU-20260926-08"
if grep -qE 'max\+1|併入' "${TMP}/out.txt"; then ok "N1：訊息含可行動建議（max+1／併入）"; else bad "N1：訊息缺少可行動建議"; fi

# N2：檔案不存在
rm -f "${TMP}/fx.md"
run_case "N2 檔案不存在 FAIL" 1 "找不到檔案"

# N3：格式漂移（有檔案但沒有 ### FU-… 標題）
printf '# FOLLOWUPS\n\n| id | 項目 |\n|---|---|\n| A1 | 改成表格了 |\n' > "${TMP}/fx.md"
run_case "N3 格式漂移（0 個 FU 標題）FAIL" 1 "找不到任何"

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-followups-unique-ids PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-followups-unique-ids FAIL（pass=${pass} fail=${fail}）"
exit 1
