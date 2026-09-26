#!/usr/bin/env bash
# ============================================================
# test-fullwidth-var-expansion.sh — 「$VAR 緊鄰全形/中日韓字元」陷阱的自我測試
# ------------------------------------------------------------
# 【來源】移植自 **a2a-dev**（`tests/fullwidth-var-expansion-selftest.sh`，F28 護欄；
#   2026-09-25 獨立審查以多輪對抗案例實證）。atlas-go 端擴充：檔案類型（`*.sh` / `Makefile` /
#   `*.mk` / `*.py` / YAML 的 `run:` 區塊）＋ 精確 exit code 斷言。
#
# 【為什麼有這支測試】bash 在 **UTF-8 LC_CTYPE** 下解析展開時，會把緊接在後的非 ASCII 位元組
#   吃進變數名 ⇒ `set -u` 的腳本**直接中止**（即使變數已定義）、沒開 `set -u` 則**靜默**吞掉值。
#   危害形態＝「護欄在該印警告的那條路徑上崩潰」或訊息殘缺。
#
# 【提供什麼】
#   ① locale 相依的負向對照、② 正向對照、③ 靜默變體（這三項在沒有會重現的 UTF-8 locale 時標 ⏭️）
#   ④ repo 全掃必須 0 命中（exit 0）     ④b 對抗式 fixture：真陷阱必命中、正常寫法不誤報
#   ④c/④d 掃描器鑑別力的正反控制        ⑦ YAML `run:` 區塊要命中且**行號正確**、`name:` 不得誤報
#   ⑤ 具名迴歸（本次修正的檔）          ⑥ 歷史變數逐名證明（locale 相依）
#
# 【本檔必須自我乾淨】掃描器會掃 `*.sh`（含本檔）。因此樣本一律用**八進位轉義**生成：
#   `\044` = `$`、`\357\274\210` = 全形左括號、`\357\274\211` = 全形右括號。
#   ⚠️ bash `printf` 的八進位只吃 **3 位**（`\0357` 會被截成 `\035` + 字面 `7`）。
# ============================================================
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
BASH_BIN="${BASH_BIN:-/bin/bash}"
SCANNER="$ROOT/scripts/ci/check_fullwidth_var_expansion.py"

pass=0; fail=0; skip=0
ok()   { echo "  ✅ $1"; pass=$((pass+1)); }
bad()  { echo "  ❌ $1"; fail=$((fail+1)); }
skp()  { echo "  ⏭️  $1"; skip=$((skip+1)); }
chk()  { if [ "$2" = "$3" ]; then ok "$1（=$2）"; else bad "$1（期望 $3 得到 $2）"; fi; }

echo "── fullwidth-var-expansion 自我測試（bash=${BASH_BIN}）──"

[ -f "$SCANNER" ] || { bad "找不到掃描器 ${SCANNER}"; echo "❌ FAIL"; exit 1; }

# ── 樣本（八進位轉義生成；本檔不含該樣式字面）─────────────────────────
printf 'set -u\nV=abc\necho "\044V\357\274\211x"\n'      > "$TMP/bad.sh"      # 未加括號（set -u）→ 應爆
printf 'V=abc\necho "\044V\357\274\211x"\n'              > "$TMP/silent.sh"   # 未加括號（無 set -u）→ 靜默
printf 'set -u\nV=abc\necho "\044{V}\357\274\211x"\n'    > "$TMP/good.sh"     # 已加括號 → 應正常

# ── 找出「會重現此陷阱」的 UTF-8 locale（找不到 → 這三項標 ⏭️）──────────
[ -x "$BASH_BIN" ] || { echo "  ⚠️  BASH_BIN 不可執行（${BASH_BIN}）→ 改用 command -v bash" >&2; BASH_BIN="$(command -v bash)"; }
UTF8_LOCALE=""
for cand in C.UTF-8 en_US.UTF-8 UTF-8 en_US.utf8; do
  probe=$(LC_ALL="$cand" "$BASH_BIN" "$TMP/bad.sh" 2>&1)
  case "$probe" in
    *"unbound variable"*) UTF8_LOCALE="$cand"; break ;;
  esac
done
if [ -n "$UTF8_LOCALE" ]; then
  echo "  （locale（樣本執行用）：${UTF8_LOCALE}）"
else
  echo "  （此環境找不到會重現陷阱的 UTF-8 locale（LC_ALL 目前=${LC_ALL:-未設}）→ ①②③⑥ 標 ⏭️）"
fi
run_sample() {
  if [ -n "$UTF8_LOCALE" ]; then LC_ALL="$UTF8_LOCALE" "$BASH_BIN" "$1" 2>&1
  else "$BASH_BIN" "$1" 2>&1; fi
}

# ① 負向對照：必須真的 unbound variable（即使 V 已定義）
if [ -z "$UTF8_LOCALE" ]; then
  skp "① 負向對照（此環境無 UTF-8 locale → 陷阱不重現）"
else
  out=$(run_sample "$TMP/bad.sh"); rc=$?
  if [ "$rc" -ne 0 ]; then ok "① 負向對照：這類寫法真的會爆（exit=${rc}）"; else bad "① 負向對照：預期非 0 exit，得到 ${rc}"; fi
  case "$out" in
    *"unbound variable"*) ok "① 錯誤訊息是 unbound variable" ;;
    *) bad "① 錯誤訊息不含 unbound variable（實際='$(printf '%s' "$out" | head -1)'）" ;;
  esac
fi

# ② 正向對照：${VAR} 寫法（任何 locale 都成立）
out=$(run_sample "$TMP/good.sh"); rc=$?
chk "② 正向對照 exit 0" "${rc}" "0"
chk "② 正向對照輸出正確（abc + 全形括號）" "${out}" "abc）x"

# ③ 靜默變體：無 set -u → exit 0 但變數無聲消失
if [ -z "$UTF8_LOCALE" ]; then
  skp "③ 靜默變體（此環境無 UTF-8 locale）"
else
  out=$(run_sample "$TMP/silent.sh"); rc=$?
  chk "③ 靜默變體 exit 0（不報錯）" "${rc}" "0"
  case "${out}" in
    *abc*) bad "③ 靜默變體：變數竟然還在（與預期不符）" ;;
    *)     ok "③ 靜默變體：變數無聲消失（輸出不含 abc，且 exit 0）" ;;
  esac
fi

# ── ④ repo 全掃：0 命中且 exit 0（atlas-go 的檔案類型：sh/mk/Makefile/py/yml 的 run:）──
scanout=$(python3 "$SCANNER" --root "$ROOT" 2>&1); rc=$?
case "$scanout" in
  *"HITS=0") chk "④ repo 全掃 0 命中 ⇒ exit 0" "${rc}" "0" ;;
  *) bad "④ repo 全掃仍有命中：$(printf '%s' "$scanout" | head -6 | tr '\n' ' ')" ;;
esac

# ── ④b 對抗式 fixture（護欄的**鑑別力**）──────────────────────────────
# 為什麼要這一項：regex 護欄同時會偽陰性（`a#$Z`＋全形括號 的 `#` 在字中間不是註解、
# `${P##*/}` 的 `#` 是參數運算子、`\`+LF 行接續、跨行引號、here-string、`$( … '字面' … )`、
# `eval`/`trap`/`bash -c` 的遞延再解析）與偽陽性（`<<'EOF'` 不展開的主體、跨行單引號字面、
# `$Z` 後接 0x80 接續位元組、`$(cat <<'EOF' … EOF)`）⇒ 2026-09-25 由獨立審查實證。
# atlas-go 擴充：`.yml` 的 `run:` 區塊（真陷阱）、`.yml` 的 `name:`（不得誤報）、`.mk`（真陷阱）。
FIX="$TMP/fixtures"; mkdir -p "$FIX"
python3 - "$FIX" <<'PY'
import os, sys
d = sys.argv[1]
FW = b"\xef\xbc\x89"          # U+FF09 全形右括號（以位元組寫出，本 .sh 不含字面樣式）
flag = {
    "f01_midword_hash.sh":   b"echo a#$Z" + FW + b"x\n",
    "f02_param_op.sh":       b"echo ${P##*/} \"$Z" + FW + b"x\"\n",
    "f03_line_cont.sh":      b"echo \"pre $Z\\\n" + FW + b"post\"\n",
    "f04_multiline_dq.sh":   b"echo \"pre\n$Z" + FW + b"\"\n",
    "f05_plain.sh":          b"echo \"$Z" + FW + b"x\"\n",
    "f06_heredoc_exp.sh":    b"cat <<EOF\nval=$Z" + FW + b"x\nEOF\n",
    "f07_nonutf8.sh":        b"set -u\nZ=x\necho \"\xa4\xa4$Z" + FW + b"\"\n",
    "f08_cmdsubst.sh":       b"echo \"$(echo $Z" + FW + b"x)\"\n",
    "f09_herestring.sh":     b"read -r x <<< \"$Z" + FW + b"x\"\n",
    "f10_eval.sh":           b"eval 'echo \"$Z" + FW + b"x\"'\n",
    "f11_trap.sh":           b"trap 'echo \"$Z" + FW + b"x\"' EXIT\n",
    "f12_bashc.sh":          b"/bin/bash -c 'echo \"$Z" + FW + b"x\"'\n",
    "f13_nested_here.sh":    b"cat <<EOF\n$(cat <<INNER\n$Z" + FW + b"x\nINNER\n)\nEOF\n",
    # atlas-go 追加：`$()` 內多層引號之後仍要抓到（天真狀態機會在此失準）
    "f14_cmdsubst_quotes.sh": b'TOKEN="$(sed -n \'s/x/y/p\' "$F" | tr -d "\'"\\"[:space:]")"\n'
                              + b'echo "\xe7\x84\xa1\xe6\xb3\x95\xe8\xa7\xa3\xe6\x9e\x90 $TOKEN' + FW + b'"\n',
    # atlas-go 追加：YAML 的 `run:` 區塊（區塊純量）
    "f15_yml_run_block.yml":  b"jobs:\n  t:\n    steps:\n    - run: |\n        echo ok\n        echo \"$Z" + FW + b"x\"\n",
    # atlas-go 追加：Makefile 風（`$$Z` 經 make 展開後就是 `$Z`）
    "f16_makefile.mk":        b"target:\n\techo \"$$Z" + FW + b"x\"\n",
    # atlas-go 追加（**狀態漂移回歸**）：`bash "…"` 之後（reparse 區間內含雙引號）仍要抓到後面的真陷阱。
    # a2a-dev 原版在此把 reparse 的收尾一律寫回 normal ⇒ 收尾單引號被當成「開啟字面」⇒ 整檔遮蔽。
    # 實測受害：atlas-go 的 test-install-webhook.sh（3 處）、test-binary-freshness-guard.sh（2 處）。
    "f17_reparse_dq_drift.sh": b"if bash \"$INSTALLER\" --no-load > /dev/null 2>&1; then\n"
                               + b"  ok \"installed\"\n"
                               + b"fi\n"
                               + b"else bad \"mode=$MODE" + FW + b"\"; fi\n",
}
clean = {
    "g01_single_literal.sh": b"echo 'literal $Z" + FW + b"'\n",
    "g02_ascii_next.sh":     b"echo \"$Zx\"\n",
    "g03_comment.sh":        b"# comment $Z" + FW + b"x\necho ok\n",
    "g04_escaped_dollar.sh": b"echo \\$Z" + FW + b"x\n",
    "g05_heredoc_quoted.sh": b"cat <<'EOF'\nval=$Z" + FW + b"x\nEOF\n",
    "g06_multiline_sq.sh":   b"echo 'pre\n$Z" + FW + b"'\n",
    "g07_cont_byte.sh":      b"val=$Z\x80end\n",
    "g08_braced.sh":         b"echo \"${Z}" + FW + b"x\"\n",
    "g09_special_param.sh":  b"echo \"$1" + FW + b"\" \"$@" + FW + b"\"\n",
    "g10_backtick_dq.sh":    b"echo `echo 'lit $Z" + FW + b"'`\n",
    "g11_heredoc_dq.sh":     b"cat <<\"EOF\"\nval=$Z" + FW + b"x\nEOF\n",
    "g12_heredoc_esc.sh":    b"cat <<\\EOF\nval=$Z" + FW + b"x\nEOF\n",
    "g13_cmdsub_here.sh":    b"x=$(cat <<'EOF'\nval=$Z" + FW + b"x\nEOF\n)\n",
    "g14_dq_cmdsub_sq.sh":   b"echo \"$(echo '$Z" + FW + b"x')\"\n",
    "g15_dq_cmdsub_sq2.sh":  b"echo \"a$(echo 'b $Z" + FW + b"x')c\"\n",
    "g16_nested_q_here.sh":  b"cat <<EOF\n$(cat <<'IN'\n$Z" + FW + b"x\nIN\n)\nEOF\n",
    "g17_nested_cmdsubst.sh": b"ROOT=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/..\" && pwd)\"; echo 'lit $Z" + FW + b"x'\n",
    # atlas-go 追加：YAML 的 `name:` 是**資料**不是 shell ⇒ 不得誤報（這也是「只掃 run: 區塊」的理由）
    "g18_yml_name.yml":      b"jobs:\n  t:\n    steps:\n    - name: literal $Z" + FW + b"x\n      run: echo ok\n",
    # atlas-go 追加：Makefile 已加括號 ⇒ 安全
    "g19_braced.mk":         b"target:\n\techo \"$${Z}" + FW + b"x\"\n",
}
for name, data in list(flag.items()) + list(clean.items()):
    open(os.path.join(d, name), "wb").write(data)
open(os.path.join(d, ".want_flagged"), "w").write("\n".join(sorted(flag)) + "\n")
open(os.path.join(d, ".want_clean"), "w").write("\n".join(sorted(clean)) + "\n")
PY
if [ ! -f "$FIX/.want_flagged" ]; then
  bad "④b fixture 產生失敗"
else
  fixout=$(python3 "$SCANNER" --root "$FIX" 2>&1)
  printf '%s\n' "$(printf '%s' "$fixout" | sed -n 's/^\([^:]*\):.*/\1/p' | sort -u)" > "$TMP/got.txt"
  sort -u "$FIX/.want_flagged" > "$TMP/want.txt"
  sort -u "$FIX/.want_clean"  > "$TMP/clean.txt"
  fn=$(comm -23 "$TMP/want.txt" "$TMP/got.txt" | tr '\n' ' ')
  fp=$(comm -13 "$TMP/want.txt" "$TMP/got.txt" | tr '\n' ' ')
  nflag=$(grep -c . "$FIX/.want_flagged"); nclean=$(grep -c . "$FIX/.want_clean")
  chk "④b 偽陰性（${nflag} 個真陷阱全數命中）" "${fn}" ""
  chk "④b 偽陽性（${nclean} 個正常寫法不誤報）" "${fp}" ""
  chk "④b 掃描器 exit code（有命中 ⇒ 精確 1）" "$(python3 "$SCANNER" --root "$FIX" --quiet >/dev/null 2>&1; echo $?)" "1"
fi

# ④c 掃描器負向對照：把一個 clean fixture 改成真陷阱 → 必須被標記
#     注意：不能加 --quiet（那會只印摘要行，grep 檔名永遠找不到 → 假綠）
#     注意 2：不要寫成 `python3 … | grep -q …` —— `set -o pipefail` 下 grep 命中後提前結束會讓
#             python3 收到 SIGPIPE（非 0）⇒ 管線狀態變成失敗 → 條件永遠走 else（假紅）。
printf 'echo a#$Z\357\274\211x\n' > "$FIX/g02_ascii_next.sh"
c_out=$(python3 "$SCANNER" --root "$FIX" 2>/dev/null)
case "$c_out" in
  *g02_ascii_next.sh*) ok "④c 負向對照：把 clean 改成真陷阱後會被標記" ;;
  *)                    bad "④c 負向對照失敗：改成真陷阱後掃描器仍報 0 命中" ;;
esac
# ④d 反向：把一個真陷阱改成已修好的寫法 → 不應再被標記
printf 'echo "\044{Z}\357\274\211x"\n' > "$FIX/f05_plain.sh"
d_out=$(python3 "$SCANNER" --root "$FIX" 2>/dev/null)
case "$d_out" in
  *f05_plain.sh*) bad "④d 反向對照失敗：已修好的寫法仍被標記" ;;
  *)              ok "④d 反向對照：改成已加括號後不再被標記" ;;
esac

# ── ⑦ YAML 覆蓋：`run:` 區塊要命中、且**回報的行號必須是原檔行號** ─────────
Y="$TMP/yamlmap"; mkdir -p "$Y"
python3 - "$Y" <<'PY'
import os, sys
d = sys.argv[1]
FW = b"\xef\xbc\x89"
body = b"name: ci\non: [push]\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n    - run: |\n        echo ok\n        echo \"$Z" + FW + b"x\"\n        echo tail\n"
open(os.path.join(d, "wf.yml"), "wb").write(body)
open(os.path.join(d, ".want_line"), "w").write(str(body.split(b"\n").index(b'        echo "$Z' + FW + b'x"') + 1) + "\n")
PY
want_line=$(cat "$Y/.want_line")
y_out=$(python3 "$SCANNER" --root "$Y" 2>/dev/null)
case "$y_out" in
  *"wf.yml:${want_line}:"*) ok "⑦ YAML run: 區塊命中且行號正確（wf.yml:${want_line}）" ;;
  *) bad "⑦ YAML run: 區塊行號不對（期望 wf.yml:${want_line}）：$(printf '%s' "$y_out" | head -3 | tr '\n' ' ')" ;;
esac

# ── ⑤ 具名迴歸：本次修正的檔（語法 OK、且以同一支掃描器確認已無此樣式）──
mkdir -p "$TMP/perfile"
for f in scripts/ci/check_finmind_quota.sh tests/scripts/test-install-webhook.sh \
         tests/scripts/test-binary-freshness-guard.sh tests/scripts/test-cron-entrypoint.sh \
         tests/scripts/test-secret-scan.sh tests/scripts/test-check-frontend-dist.sh; do
  if bash -n "$ROOT/$f" 2>/dev/null; then ok "⑤ ${f} bash -n OK"; else bad "⑤ ${f} bash -n 失敗"; fi
  mkdir -p "$TMP/perfile/$(dirname "$f")"
  cp "$ROOT/$f" "$TMP/perfile/$f"
done
for f in Makefile .github/workflows/quality.yml; do
  mkdir -p "$TMP/perfile/$(dirname "$f")"
  cp "$ROOT/$f" "$TMP/perfile/$f"
done
pf_out=$(python3 "$SCANNER" --root "$TMP/perfile" 2>/dev/null)
case "$pf_out" in
  *"HITS=0"*) ok "⑤ 修正過的檔（含 Makefile／workflow）以掃描器複查：0 命中" ;;
  *)          bad "⑤ 修正過的檔仍有命中：$(printf '%s' "$pf_out" | head -4 | tr '\n' ' ')" ;;
esac
chk "⑤ 乾淨 fixture 的 exit code（無命中 ⇒ 精確 0）" "$(python3 "$SCANNER" --root "$TMP/perfile" --quiet >/dev/null 2>&1; echo $?)" "0"
chk "⑤ 用法錯誤的 exit code（--root 不存在 ⇒ 精確 2）" "$(python3 "$SCANNER" --root "$TMP/__no_such_dir__" --quiet >/dev/null 2>&1; echo $?)" "2"

# ── ⑥ 歷史歸因：本次修正的變數名逐名證明（樣本以 printf 生成）──────────────
if [ -z "$UTF8_LOCALE" ]; then
  skp "⑥ 歷史變數逐名證明（此環境無 UTF-8 locale → 無法斷言）"
else
  for nm in STATE_FILE MODE desc fail DOC_MIN DOC_MAX gofmt_rc TOKEN; do
    printf 'set -u\n%s=x\necho "\044%s\357\274\211"\n' "$nm" "$nm" > "$TMP/hist.sh"
    out=$(run_sample "$TMP/hist.sh"); rc=$?
    if [ "${rc}" -ne 0 ]; then
      case "${out}" in
        *"unbound variable"*) ok "⑥ 歷史變數 ${nm}：未加括號會 unbound variable（exit=${rc}）" ;;
        *)                    bad "⑥ 歷史變數 ${nm}：exit=${rc} 但訊息不是 unbound variable" ;;
      esac
    else
      bad "⑥ 歷史變數 ${nm}：預期非 0 exit，得到 ${rc}"
    fi
  done
fi

echo ""
echo "── 結果：${pass} 通過 / ${fail} 失敗 / ${skip} 跳過（locale 相依）──"
[ "$fail" -eq 0 ] || exit 1
exit 0
