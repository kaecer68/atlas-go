#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

ERRORS_FILE=$(mktemp)
trap 'rm -f "$ERRORS_FILE"' EXIT

echo 0 > "$ERRORS_FILE"

while IFS= read -r -d '' file; do
    sed -n 's/.*](\([^)]*\)).*/\1/p' "$file" | while read -r link; do
        if [[ "$link" =~ ^(https?://|mailto:|#) ]]; then
            continue
        fi

        target="${link%%#*}"
        [ -z "$target" ] && continue

        if [[ "$target" == /* ]]; then
            resolved=".${target}"
        else
            dir=$(dirname "$file")
            resolved="$dir/$target"
        fi

        if [ ! -e "$resolved" ] && [ ! -e "$target" ]; then
            echo "BROKEN: $file -> $link"
            echo $(($(cat "$ERRORS_FILE") + 1)) > "$ERRORS_FILE"
        fi
    done
done < <(find . \
    -name '*.md' \
    -not -path './vendor/*' \
    -not -path './.gocache/*' \
    -not -path './.opencode/*' \
    -not -path '*/node_modules/*' \
    -not -path '*/.git/*' \
    -not -path './web/static/css/*' \
    -not -path './web/static/js/*' \
    -not -path '*/.omo/*' \
    -print0)

ERRORS=$(cat "$ERRORS_FILE")

if [ "$ERRORS" -gt 0 ]; then
    echo ""
    echo "Found $ERRORS broken internal link(s)"
    exit 1
fi

echo "All internal markdown links are valid"
