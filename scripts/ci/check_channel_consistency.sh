#!/usr/bin/env bash
# scripts/ci/check_channel_consistency.sh
# CI wrapper for the Go contract/schedule/alert consistency checker (#1877).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"
go run ./cmd/check-channel-consistency
