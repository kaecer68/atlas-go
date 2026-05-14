#!/bin/bash
# AI Feature Branch Helper
# Usage: bash scripts/git-workflow/ai-feature-branch.sh <type> <description>
# Example: bash scripts/git-workflow/ai-feature-branch.sh feat "add channel adapter for geopolitical"

set -euo pipefail

TYPE=${1:-feat}
DESC=${2:-"update"}

# Validate type
if [[ ! "$TYPE" =~ ^(feat|fix|refactor|docs|test)$ ]]; then
    echo "Error: type must be one of: feat, fix, refactor, docs, test"
    exit 1
fi

# Generate branch name from description
BRANCH_NAME="${TYPE}/$(echo "$DESC" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | sed 's/[^a-z0-9-]//g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//')"

# Ensure we're on main and up to date
echo "📥 Pulling latest main..."
git checkout main
git pull origin main

# Create feature branch
echo "🌿 Creating branch: $BRANCH_NAME"
git checkout -b "$BRANCH_NAME"

echo ""
echo "✅ Feature branch created: $BRANCH_NAME"
echo ""
echo "Next steps:"
echo "  1. Make your changes"
echo "  2. git add -A"
echo "  3. git commit -m \"${TYPE}(scope): ${DESC}\""
echo "  4. bash scripts/git-workflow/ai-create-pr.sh \"${TYPE}(scope): ${DESC}\""
