#!/usr/bin/env bash
# ⛔ 已停用（DISABLED，2026-09-26）— verify-sector-allocation-closure.sh
#
# 本檔曾是 SA00–SA12 closure verifier（17 條 check）。實測（origin/main 0cc32d16，2026-09-26）後
# 判定它**從來沒有在守任何東西**，故明示停用為 no-op。事實與證據：
#
#   1) 依賴檔不存在 ⇒ 立即 exit 2（唯一 default 路徑）：
#      `docs/manifests/sector-allocation-simulation-closure-manifest.md` 已於 #1255
#      （bcc06abc「docs governance overhaul」）移出 `docs/`，現存於 `.omo/manifests/`（**gitignored**）。
#      **不能搬回 `docs/manifests/`**：`scripts/ci/check_docs_governance.sh` 明文只准 README.md + TEMPLATE.md
#      ⇒ 搬回去會讓 `make ci` 直接變紅；改成指向 `.omo/` 則在 fresh clone / CI 一律不存在 ⇒ 同樣 exit 2。
#   2) 就算把依賴檔補回去，12 條 `check()`（#06–#17）也**執行不到**：
#      b252b10c（#1250，shellcheck 修正）把 `check()` 內的 `eval "$@"` 改成 `"$@"`，
#      但呼叫端仍傳**整串 shell 字串**（例：`check "07 …" "grep -qE '…' '$MANIFEST'"`）
#      ⇒ `result=$("$@" 2>&1)` 會把整串字串當「命令名」⇒ rc=127（command not found），
#      配上 `set -e` 讓腳本在第一個 check 就中止。
#      ⇒ 17 條裡真正跑得動的只有「SA00–SA12 表格列存在」的 grep 迴圈，而它只印 stdout（passes 不計）。
#   3) 沒有任何執行路徑：`grep -rn verify-sector-allocation-closure` 在 origin/main 只命中
#      `cmd/experimental/sector-allocation-closure-preflight/main.go` 的**字串訊息**（不是 exec）與本檔自身。
#      且 `scripts/*.sh` 不在 `make ci` 的 glob 內（`Makefile:429` 只掃 `scripts/ci/check_*.sh`）。
#
# 【為什麼選「明示停用」而不是「修好並接線」】
#   要真的會跑，得同時：(i) 把依賴 manifest 變成**受版控**的檔（需要一個通過 docs governance 的新位置，
#   屬政策決定）、(ii) 修 `check()` 的 eval bug、(iii) 接線（`scripts/ci/check_*.sh` glob）。
#   而它的語意大多已被其他機制取代：
#     - #10/#11（source=heuristic / calibration_status 鎖）：Go 層鎖定 + 單元測試
#       （`internal/config`、`internal/sectorallocation`、`internal/eventdriven`）
#     - #13/#15（SA-INV-20 / SA-INV-11）：這兩個 ID 在 origin/main **只存在於本檔**
#       （spec 與程式碼都沒有）⇒ 沒有任何可驗證的標的
#     - #16/#17（檔案存在性）：`go test ./...` 已實際執行那些測試；存在性檢查不構成守門
#     - #01–#09：讀的是 gitignored 的 manifest 狀態表 ⇒ 在 CI 不可能成立
#   ⇒ 「修好」只會得到一個讀 gitignored 檔、在 CI 永遠 exit 2 的**假 gate**。
#
# 【現行契約】永遠 no-op：印出停用說明後 exit 0。
#   刻意**不**回傳非零：本檔已無呼叫端，非零只會製造「擋下來了」的假象
#   （同族事故：把 exit 127/2 當成「擋下了」，見 `docs/operations/remediation-manifest.md` E2）。
#
# 【重新啟用的條件】見 `docs/operations/FOLLOWUPS.md` FU-20260926-23 /
#   `docs/operations/remediation-manifest.md` E16：改成讀**受版控**的 artifact
#   （`docs/specs/sector-allocation-simulation-closure-spec.md` + Go 測試 + `configs/allowed_env_vars.md`）
#   並以 `scripts/ci/check_*.sh` 命名接入 `make ci` 的 glob。
#
# 歷史實作（不再執行，供對帳）：`git show 3049e519:scripts/verify-sector-allocation-closure.sh`

set -euo pipefail

echo "⛔ 已停用（DISABLED）：verify-sector-allocation-closure.sh 不執行任何檢查；本訊息即它的全部輸出。" >&2
echo "   理由／替代機制：本檔檔頭、docs/operations/FOLLOWUPS.md FU-20260926-23、remediation-manifest.md E16。" >&2
exit 0
