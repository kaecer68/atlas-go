#!/usr/bin/env bash
# check_monitoring_single_source.sh — 薄殼；實作在 .py（只用標準庫，離線可跑）。
#
# 為什麼要有 .sh：`make ci-quick` 是以 `bash <script>` 逐支跑 `scripts/ci/check_*.sh`，
# GitHub Actions 也走同一支；維持與其他 check_*.sh 一致的呼叫面。
# 規則與理由見 .py 檔頭（R1 唯一設定樹 / R2 掛載源落點 / R3 舊樹路徑引用）。
set -uo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check_monitoring_single_source.py" "$@"
