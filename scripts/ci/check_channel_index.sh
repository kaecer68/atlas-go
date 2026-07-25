#!/usr/bin/env bash
# scripts/ci/check_channel_index.sh
# CI wrapper for scripts/ci/check_channel_index.py
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
python3 "${REPO_ROOT}/scripts/ci/check_channel_index.py"
