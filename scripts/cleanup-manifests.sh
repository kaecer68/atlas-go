#!/usr/bin/env bash
# cleanup-manifests.sh — 檢查 .omo/manifests/ 內的 stale manifest 並提示處理
# 用法: ./scripts/cleanup-manifests.sh [--stale-days N] [--auto-delete]
# 預設只報告，不做變更。--auto-delete 自動刪除無長期價值的 done manifest。

set -euo pipefail

STALE_DAYS=7
AUTO_DELETE=false
MANIFEST_DIR=".omo/manifests"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --stale-days) STALE_DAYS="$2"; shift 2 ;;
        --auto-delete) AUTO_DELETE=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--stale-days N] [--auto-delete]"
            echo "  --stale-days N   天數門檻，預設 7"
            echo "  --auto-delete    自動刪除無長期價值的 done manifest"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ ! -d "$MANIFEST_DIR" ]]; then
    echo "✅ $MANIFEST_DIR/ 目錄不存在或為空，無需清理"
    exit 0
fi

echo "=== Manifest 清理檢查 (stale > ${STALE_DAYS} 天) ==="
echo "時間: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

STALE_COUNT=0
DONE_COUNT=0
IN_PROGRESS_COUNT=0

for f in "$MANIFEST_DIR"/*.md; do
    [[ -f "$f" ]] || continue
    filename=$(basename "$f")
    mtime=$(stat -f%m "$f" 2>/dev/null || stat -c%Y "$f" 2>/dev/null)
    now=$(date +%s)
    age_days=$(( (now - mtime) / 86400 ))

    # 檢查狀態
    status=$(grep -i '^\s*[*_]\{0,2\}Status[*_]\{0,2\}[:：]' "$f" | head -1 | sed 's/.*[:：]\s*//' | sed 's/[*_>` ]//g' | cut -c1-40 || echo '')

    # 判斷是否為 done/completed
    if echo "$status" | grep -qiE '(done|completed|fixed|✅)'; then
        is_done=true
    else
        is_done=false
    fi

    # 判斷是否無長期價值（檢查是否有 spec-level invariant）
    has_spec=false
    if grep -qiE '(invariant.*spec|promote.*spec|binding.*invariant|canonical.*spec)' "$f" 2>/dev/null; then
        has_spec=true
    fi

    printf "%-50s age=%3dd  status=%-15s" "$filename" "$age_days" "${status:-(無標記)}"

    if $is_done && [[ $age_days -ge $STALE_DAYS ]]; then
        STALE_COUNT=$((STALE_COUNT + 1))
        if $has_spec; then
            echo " ⚠️  STALE+DONE+SPEC — 建議 promote 到 docs/specs/"
        else
            echo " 🗑️  STALE+DONE — 建議 archive 或 delete"
            if $AUTO_DELETE; then
                echo "    [AUTO-DELETE] rm $f"
                rm "$f"
            fi
        fi
    elif $is_done; then
        DONE_COUNT=$((DONE_COUNT + 1))
        if $has_spec; then
            echo " ✅ done+spec（考慮 promote）"
        else
            echo " ✅ done（${age_days}d 內，暫不處理）"
        fi
    else
        IN_PROGRESS_COUNT=$((IN_PROGRESS_COUNT + 1))
        echo " 🔄 ${status:-(無標記)}"
    fi
done

echo ""
echo "=== 統計 ==="
echo "Stale done (>${STALE_DAYS}d): $STALE_COUNT"
echo "Recent done:                $DONE_COUNT"
echo "In progress / other:        $IN_PROGRESS_COUNT"

if [[ $STALE_COUNT -gt 0 ]]; then
    echo ""
    echo "⚠️  $STALE_COUNT 個 stale manifest 需要處理。"
    echo "   手動處理: 歸檔 → docs/archive/ 或 promote → docs/specs/ 或 rm"
    echo "   自動處理: $0 --stale-days $STALE_DAYS --auto-delete"
fi
