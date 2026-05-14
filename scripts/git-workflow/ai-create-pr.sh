#!/bin/bash
# AI PR Creator
# Usage: bash scripts/git-workflow/ai-create-pr.sh <commit-message>
# Example: bash scripts/git-workflow/ai-create-pr.sh "feat(apigateway): add geopolitical channel adapter"

set -euo pipefail

MSG=${1:-"feat: update"}

# Parse commit message for PR title
PR_TITLE="$MSG"

# Extract scope and description for body
if echo "$MSG" | grep -qE '^([a-z]+)\([^)]+\):\ .+$'; then
    TYPE=$(echo "$MSG" | sed -E 's/^([a-z]+)\(.+\):.*/\1/')
    SCOPE=$(echo "$MSG" | sed -E 's/^[a-z]+\(([^)]+)\):.*/\1/')
    DESC=$(echo "$MSG" | sed -E 's/^[a-z]+\([^)]+\):\ //')
else
    TYPE="feat"
    SCOPE="general"
    DESC="$MSG"
fi

# Get current branch
CURRENT_BRANCH=$(git branch --show-current)

if [[ "$CURRENT_BRANCH" == "main" ]]; then
    echo "❌ Error: You are on main branch!"
    echo "   Run: bash scripts/git-workflow/ai-feature-branch.sh $TYPE '$DESC'"
    exit 1
fi

# Stage and commit if there are changes
if [[ -n $(git status --porcelain) ]]; then
    echo "📦 Staging changes..."
    git add -A
    git commit -m "$MSG" || true
fi

# Push branch
echo "🚀 Pushing branch: $CURRENT_BRANCH"
git push -u origin "$CURRENT_BRANCH"

# Create PR using gh CLI if available
if command -v gh &> /dev/null; then
    echo "🔃 Creating PR..."
    gh pr create \
        --title "$PR_TITLE" \
        --body "## Summary
- **Type**: $TYPE
- **Scope**: $SCOPE
- **Description**: $DESC

## Changes
- $(git log --oneline main..HEAD | wc -l | xargs) commits ahead of main
- $(git diff --stat main..HEAD | tail -1)

## Verification
- [ ] go build ./...
- [ ] go test ./...
- [ ] gofmt -l .
- [ ] staticcheck ./...

## Risk Assessment
- [ ] Low: Minor changes, no API changes
- [ ] Medium: New features, configuration changes
- [ ] High: Architecture changes, data migrations" \
        --base main
else
    echo ""
    echo "⚠️  gh CLI not found. Please install it or create PR manually:"
    echo "   https://github.com/kaecer68/atlas-go/pull/new/$CURRENT_BRANCH"
fi
