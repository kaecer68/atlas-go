#!/usr/bin/env bash
# test-shell-var-nonascii.sh — 「`$VAR` 緊接非 ASCII 字元」檢查的自我測試（hermetic）
#
# 為什麼要有它：護欄本身必須有牙齒。若它只會印 PASS，就等於沒有。
# 本測試在 tempdir 造 fixture，用 `--root`＋明確檔案清單注入（**不碰真檔**、不碰生產、不需要 docker）：
#   P1 正向：乾淨檔 → PASS(0)
#   P2 正向：`${VAR}（`、`$1（`、`$?（`、`$@（`、`$10（`、`$_` 以外的特例 → PASS(0)（**不得誤報**）
#   P3 正向：註解、單引號、`$'…'`、已轉義 `\$`、引號 delimiter 的 heredoc body → PASS(0)
#   N1..N6 負向：雙引號內／未加引號／`$_`／heredoc body／`$()` 內／`${…}` 內 → 必 FAIL(1)
#   N7 負向：`$()` 內多層引號（**回歸測試**：第一版掃描器在此失準 ⇒ 漏報後半個檔案）
#   B1/B2 baseline：命中 baseline ⇒ PASS(0) 且印 ⚠️；加 `--no-baseline` ⇒ FAIL(1)
#   F1 反向失敗：0 個檔案 ⇒ 必 FAIL(1)（不得靜默退化成「永遠 PASS 的空檢查」）
#   T  牙齒：每個負向案例斷言**精確 exit code**（#2011/#2020 的教訓：exit 2/126/127
#      也會被舊寫法算成「擋下了」⇒ 只斷言「非 0」等於沒斷言）
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$ROOT/scripts/ci/check_shell_var_nonascii.sh"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

echo "── shell-var-nonascii 檢查自我測試（合成 fixture，不碰真檔）──"

# fixture 目錄就是 `--root`；baseline 以 `<root>/scripts/ci/…` 解析 ⇒ 需在 FX 內建同樣結構
FX="${TMP}/fx"; mkdir -p "${FX}/scripts/ci" "${FX}/empty"

# run_case <名稱> <期望 exit> <期望輸出子字串|空> <檔案…>
run_case() {
  local name="$1" want="$2" needle="$3"; shift 3
  local out="${TMP}/out.txt"
  ( cd "${TMP}" && bash "${CHECK}" --root "${FX}" "$@" ) > "${out}" 2>&1
  local rc=$?
  if [ "${rc}" = "${want}" ]; then ok "${name}（exit=${rc}）"
  else bad "${name}（exit=${rc}，期望 ${want} ⇒ 可能就是「非 0 就算擋下」的假綠慣性）"; sed 's/^/     /' "${out}" | head -6; fi
  if [ -n "${needle}" ]; then
    if grep -q -- "${needle}" "${out}"; then ok "${name}：訊息含可辨識字串（${needle}）"
    else bad "${name}：訊息缺少「${needle}」"; sed 's/^/     /' "${out}" | head -6; fi
  fi
}

# ── P1 乾淨檔 ─────────────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'V=1' 'echo "${V}（乾淨）"' > "${FX}/clean.sh"
run_case "P1 乾淨檔 PASS" 0 "PASS" clean.sh

# ── P2 安全形式（**不得誤報**）─────────────────────────────────────
cat > "${FX}/safe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "${V}（已加括號）"
echo "$1（位置參數）" "$?（特殊參數）" "$@（全部參數）" "$10（$1+0）"
EOF
run_case "P2 安全形式不誤報" 0 "PASS" safe.sh

# ── P3 不展開的位置 ───────────────────────────────────────────────
cat > "${FX}/literal.sh" <<'EOF'
#!/usr/bin/env bash
# 註解裡的 $V（無害）
echo '$V（單引號）' $'$V（ANSI-C）' "\$V（已轉義）"
cat <<'EOS'
$V（引號 delimiter ⇒ 不展開）
EOS
EOF
run_case "P3 不展開的位置不誤報" 0 "PASS" literal.sh

# ── N1 雙引號內 ───────────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo "無法解析 $STATE_FILE（calls_today 缺失）"' > "${FX}/n1.sh"
run_case "N1 雙引號內 FAIL" 1 '$STATE_FILE' n1.sh

# ── N2 未加引號 ───────────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo $V（未加引號）' > "${FX}/n2.sh"
run_case "N2 未加引號 FAIL" 1 '$V' n2.sh

# ── N3 `$_`（底線是 name 字元）─────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo "$_（底線）"' > "${FX}/n3.sh"
run_case "N3 \$_ FAIL" 1 '$_' n3.sh

# ── N4 未加引號 delimiter 的 heredoc body ─────────────────────────
cat > "${FX}/n4.sh" <<'EOF'
#!/usr/bin/env bash
cat <<EOS
使用方式：$HOME（家目錄）
EOS
EOF
run_case "N4 heredoc body FAIL" 1 '$HOME' n4.sh

# ── N5 `${ … }` 內 ────────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo "${V:-$W（預設值）}"' > "${FX}/n5.sh"
run_case "N5 \${…} 內 FAIL" 1 '$W' n5.sh

# ── N6 反引號內 ───────────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo `echo $V（反引號）`' > "${FX}/n6.sh"
run_case "N6 反引號內 FAIL" 1 '$V' n6.sh

# ── N7 回歸測試：`$()` 內多層引號（第一版掃描器在此失準 ⇒ 漏報）──────
cat > "${FX}/n7.sh" <<'EOF'
#!/usr/bin/env bash
TOKEN="$(sed -n 's/^X=//p' "$ENV_FILE" | tr -d "'\"[:space:]")"
echo "無法解析 $TOKEN（第一版掃描器會漏掉這一行）"
EOF
run_case "N7 \$() 內多層引號後仍抓到違規" 1 '$TOKEN' n7.sh

# ── B1/B2 baseline ───────────────────────────────────────────────
printf '%s\n' '#!/usr/bin/env bash' 'echo "已知項 $LEGACY（baseline）"' > "${FX}/b1.sh"
printf 'b1.sh\techo "已知項 $LEGACY（baseline）"\t測試用：由別的 PR 負責修（fixture）\n' > "${FX}/scripts/ci/shell-var-nonascii-baseline.txt"
run_case "B1 baseline 命中 ⇒ PASS(0)" 0 "baseline" b1.sh
run_case "B2 --no-baseline ⇒ FAIL(1)" 1 '$LEGACY' --no-baseline b1.sh
: > "${FX}/scripts/ci/shell-var-nonascii-baseline.txt"

# ── F1 0 個檔案 ⇒ fail-closed ────────────────────────────────────
run_case "F1 掃到 0 個檔 ⇒ FAIL(1)" 1 '0 個 .sh' empty

# ── T 牙齒：負向案例必須是「精確 1」，不是任何非 0 ─────────────────
if [ "$( bash -c 'exit 127' >/dev/null 2>&1; echo $? )" = "127" ]; then
  ok "T 環境可區分 exit 1 / 127（本測試對每個案例斷言精確碼）"
else
  bad "T 無法量測 exit code（測試環境異常）"
fi

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-shell-var-nonascii PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-shell-var-nonascii FAIL（pass=${pass} fail=${fail}）"
exit 1
