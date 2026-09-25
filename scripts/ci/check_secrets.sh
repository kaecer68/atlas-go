#!/usr/bin/env bash
# scripts/ci/check_secrets.sh
# 守門員：**機密不得進版控**（本 repo 是 PUBLIC ⇒ 誤 commit 一次就是永久外洩）。
#
# 這個 repo 的先例：`scripts/alertmanager-webhook/com.goluck.atlas-webhook-to-telegram.plist`
# 曾把**明文 telegram bot token** 寫進版控（2026-08-27 起），而且它同時出現在 public GitHub 上；
# 等到 2026-09-25 告警鏈沉默被發現時才處理。這條檢查把「靠人記得」變成 CI 硬閘門。
#
# 掃描器：`scripts/secret-scan.sh`（a2a-dev 的整檔副本，SSOT 在那邊；詳見該檔標頭）。
# 先跑負向自我測試（證明「真的會擋」），再掃真 repo（證明「現在是乾淨的」）。
#
# 用法:
#   bash scripts/ci/check_secrets.sh            # 掃描 + 自我測試
#   bash scripts/ci/check_secrets.sh --scan-only
set -uo pipefail   # 注意：不要用 -e，否則第一個失敗就看不到第二段結果

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCAN_ONLY=0
[ "${1:-}" = "--scan-only" ] && SCAN_ONLY=1

FAIL=0
echo "═══ check_secrets ═══"

if [ "$SCAN_ONLY" -eq 0 ]; then
  echo "── 1/3 掃描器負向自我測試（tests/scripts/test-secret-scan.sh）──"
  if ! bash "$ROOT/tests/scripts/test-secret-scan.sh"; then
    echo "  ❌ 掃描器自我測試失敗 —— 掃描器可能「看起來有、實際沒在用」"
    FAIL=1
  fi
fi

echo "── 2/3 tracked 檔機密樣式掃描 ──"
if ! bash "$ROOT/scripts/secret-scan.sh" --root "$ROOT"; then
  echo "  ❌ 有機密形狀的字串進入版控 —— 立刻移除並**輪替憑證**（歷史仍看得到）"
  echo "     範例/測試資料請用行內 \`secret-scan-allow\` 或 scripts/secret-scan-allowlist.txt（都要寫理由）"
  FAIL=1
fi

echo "── 3/3 告警 webhook 安裝機制（placeholder → 安裝期注入，永不進版控）──"
# 為什麼把「安裝流程」放進機密檢查 ✗：這條機制**就是**本 repo 的機密控制手段 ✓
# （repo plist 只放 placeholder，真 token 由 install-webhook.sh 注入安裝版並 chmod 600）。
# 它一旦退化（例如有人把真 token 貼回 plist、或安裝腳本不再設 600），機密護欄就等於不存在 ✗。
if [ "$SCAN_ONLY" -eq 0 ]; then
  if ! bash "$ROOT/tests/scripts/test-install-webhook.sh"; then
    echo "  ❌ 安裝機制回歸測試失敗"
    FAIL=1
  fi
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "✅ check_secrets PASS"
  exit 0
fi
echo "❌ check_secrets FAIL"
exit 1
