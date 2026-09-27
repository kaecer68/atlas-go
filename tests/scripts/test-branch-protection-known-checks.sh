#!/usr/bin/env bash
# tests/scripts/test-branch-protection-known-checks.sh
# `scripts/openclaw/setup_branch_protection.sh` 的 --checks 探針 **hermetic** 契約測試
# （E39, 2026-09-27）。
#
# 為什麼 hermetic：`gh` 以 stub 取代（永遠 exit 1），所以 live contexts 與 recent check
#   runs 都貢獻 0 個名字，剩下的只有從 `.github/workflows` 解析出來的 job 名。不連
#   GitHub、不建立 git worktree、不動任何 tracked 檔（fixture 全部在 mktemp 下）。
#
# 契約（失敗即 exit 1）：
#   ① workflow_job_names 列出 workflow 的 job id 與 job 級 `name:` 標籤（含 `needs:` /
#      `if:` 的 job、`jobs:` 後方的 column-0 註解、`jobs: # inline` 註解形式），且不得
#      誤收非 job 的頂層 2-space key（`contents:`、`push:` ...）。
#      迴歸：reset 規則 `/^[A-Za-z]/` 排在 `jobs:` 規則之前 ⇒ `jobs:` 這一行先把自己
#      的狀態吃掉，該函式對**每個** workflow 都回傳 0 個名字。
#   ② 後果（行為層）：沒有 live context 時，一個「workflow 裡有、但還沒有人回報」的
#      job 名（同一 PR 新增的 job）必須被接受（rc=0）。修好前此情境是 exit 21。
#   ③ fail-closed 不變：不存在的名字仍然 exit 21（不可因修好而放行）。
#
# 註：測試 ② / ③ 直接跑真實 repo 的 `.github/workflows`（不是 fixture），所以它們同時
#     驗證了「真實 workflow 集合」能被完整列出。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="${ROOT}/scripts/openclaw/setup_branch_protection.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
FAIL=0

ok()  { echo "  ✅ $1"; }
bad() { echo "  ❌ $1"; FAIL=1; }

[ -f "${SRC}" ] || { echo "❌ 找不到 ${SRC}"; exit 1; }

# gh stub：`require_cmd gh` 必須通過，但任何呼叫都不得真的連上 GitHub。
mkdir -p "${TMP}/bin"
printf '#!/usr/bin/env bash\necho "gh-stub: GitHub access disabled by tests/scripts" >&2\nexit 1\n' > "${TMP}/bin/gh"
chmod +x "${TMP}/bin/gh"
export PATH="${TMP}/bin:${PATH}"

# known 名單 = 腳本輸出扣掉章節標題與說明行。
known_names() {  # <script> [args...]
  local script="$1"; shift
  bash "${script}" "$@" 2>/dev/null \
    | sed -e '/^== /d' -e '/^Names /d' -e '/^[[:space:]]*$/d' \
    | sort -u
}

# --- fixture：形狀齊全的 workflow（同時釘住「不得誤收」） ----------------------
FIX="${TMP}/fixture"
mkdir -p "${FIX}/scripts/openclaw" "${FIX}/.github/workflows"
cp "${SRC}" "${FIX}/scripts/openclaw/"
cat > "${FIX}/.github/workflows/shapes.yml" <<'YAML'
name: Shapes
on:
  pull_request:
env:
  CI: "true"
permissions:
  contents: read
# column-0 註解不得結束 jobs 區塊
jobs:
  build:
    name: "Build and test"
    runs-on: ubuntu-latest
  docs-embed:
    needs: [build]
    if: needs.changes.outputs.embedded_docs == 'true'
    runs-on: ubuntu-latest
    steps:
    - name: Regenerate embedded docs
      run: go generate ./cmd/atlas-mcp/...
  single-quoted:
    name: 'Single quoted label'
    runs-on: ubuntu-latest
YAML
# `jobs:` 帶行內註解，且 job id 含連字號
cat > "${FIX}/.github/workflows/inline.yml" <<'YAML'
name: Inline
on: push
jobs: # job ids follow
  pr-base-guard:
    runs-on: ubuntu-latest
YAML
# 完全沒有 jobs 的 workflow 不得讓函式出錯，也不得貢獻名字
cat > "${FIX}/.github/workflows/nojobs.yml" <<'YAML'
name: No jobs
on: push
YAML

got="$(known_names "${FIX}/scripts/openclaw/setup_branch_protection.sh" \
  --owner fixture --repo fixture --branch main --list-known-checks)"
want="$(printf 'Build and test\nSingle quoted label\nbuild\ndocs-embed\npr-base-guard\nsingle-quoted\n' | sort -u)"
if [ "${got}" = "${want}" ]; then
  ok "① fixture workflow 的 job 名集合正確（含 job 級 name: 去引號、含 jobs: 行內註解）"
else
  bad "① fixture job 名集合不符"
  echo "--- want ---"; printf '%s\n' "${want}"
  echo "--- got ---";  printf '%s\n' "${got}"
fi
for junk in contents push env permissions jobs on; do
  if printf '%s\n' "${got}" | grep -Fxq -- "${junk}"; then
    bad "① 誤收了非 job 的 key：${junk}"
  fi
done

# --- 真實 repo workflow：完整列出（獨立 oracle = jobs: 之後的 2-space key） ----
real="$(known_names "${SRC}" --owner fixture --repo fixture --branch main --list-known-checks)"

for wf in "${ROOT}"/.github/workflows/*.yml "${ROOT}"/.github/workflows/*.yaml; do
  [ -f "${wf}" ] || continue
  base="$(basename "${wf}")"
  # oracle 假設：`jobs:` 是檔內最後一個 column-0 top-level key（實測 9 個 workflow 皆然）。
  # 若未來有 workflow 違反此假設，這裡必須大聲紅燈，而不是給出模糊的「名單缺漏」。
  if awk '/^jobs:[[:space:]]*(#.*)?$/ {seen=1; next} seen && /^[A-Za-z_]/ {print; exit}' "${wf}" | grep -q .; then
    bad "${base}: oracle 假設被違反（jobs: 之後仍有 top-level key），請更新本測試"
    continue
  fi
  ids="$(awk '/^jobs:[[:space:]]*(#.*)?$/ {seen=1; next} seen' "${wf}" | sed -nE 's/^  ([A-Za-z0-9_.-]+):$/\1/p')"
  [ -n "${ids}" ] || { bad "${base}: oracle 抽不到任何 job id（測試 bug）"; continue; }
  missing=""
  while IFS= read -r id; do
    [ -n "${id}" ] || continue
    printf '%s\n' "${real}" | grep -Fxq -- "${id}" || missing="${missing} ${id}"
  done <<< "${ids}"
  if [ -z "${missing}" ]; then
    ok "${base}: job id 全數列出（$(printf '%s\n' "${ids}" | wc -l | tr -d ' ') 個）"
  else
    bad "${base}: known 名單缺少 job id:${missing}"
  fi
done

# --- ② 行為層：沒有 live context 時，未回報過的 workflow job 名必須被接受 -------
# 刻意重複 `build`：--checks 是「集合」，重複的名字不得進入 payload
# （E39 的實際用法是 "--checks <live 清單>,<新 job 名>"，而新 job 名往往已在 live 清單裡）。
bash "${SRC}" --non-interactive --owner fixture --repo fixture --branch main \
  --checks "build,build,docker,generate-docs-embed" > "${TMP}/ok.log" 2>&1
rc=$?
if [ "${rc}" -eq 0 ]; then
  ok "② 未回報過的 job 名（build,docker,generate-docs-embed）被接受 rc=0（修好前為 21）"
else
  bad "② 應為 rc=0，實得 ${rc}（修好前此情境 exit 21 = 新 job 無法在合併前驗證）"
  sed 's/^/     /' "${TMP}/ok.log"
fi
if grep -q 'Required checks (0 removal(s) planned): build,docker,generate-docs-embed' "${TMP}/ok.log"; then
  ok "② 重複的 --checks 名字已去重（payload 不帶重複 context）"
else
  bad "② --checks 未去重（重複名字會原樣 PUT 給 GitHub）"
  grep -n 'Required checks' "${TMP}/ok.log" | sed 's/^/     /'
fi

# --- ③ 負向：不存在的名字仍必須 fail-closed（exit 21，且訊息指名該名字） --------
bash "${SRC}" --non-interactive --owner fixture --repo fixture --branch main \
  --checks "build,docker,no-such-job-xyz" > "${TMP}/neg.log" 2>&1
rc=$?
if [ "${rc}" -eq 21 ]; then
  ok "③ 不存在的名字仍 exit 21（fail-closed 未弱化）"
else
  bad "③ 應為 exit 21，實得 ${rc}"
  sed 's/^/     /' "${TMP}/neg.log"
fi
if grep -q 'no-such-job-xyz' "${TMP}/neg.log"; then
  ok "③ 拒絕訊息指名了該未知名字（不是為別的原因而紅）"
else
  bad "③ rc=21 但訊息未指名 no-such-job-xyz"
fi

if [ "${FAIL}" -eq 0 ]; then
  echo "✅ test-branch-protection-known-checks PASS"
  exit 0
fi
echo "❌ test-branch-protection-known-checks FAIL"
exit 1
