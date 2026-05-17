#!/bin/bash
# Pre-commit validation script for AI workflow
# Run this before creating PR to ensure all checks pass

set -euo pipefail

echo "🔍 Running pre-commit validation..."

# 1. gofmt
echo "  📐 gofmt check..."
if [ -n "$(gofmt -l .)" ]; then
    echo "    ❌ gofmt failed"
    gofmt -l .
    exit 1
fi
echo "    ✅ gofmt passed"

# 2. build
echo "  🔨 go build..."
go build ./... > /dev/null 2>&1
echo "    ✅ build passed"

# 3. vet
echo "  🩺 go vet..."
go vet ./... > /dev/null 2>&1
echo "    ✅ vet passed"

# 4. tests
echo "  🧪 go test..."
go test ./... > /dev/null 2>&1
echo "    ✅ tests passed"

echo ""
echo "✅ All pre-commit checks passed! Ready to create PR."
