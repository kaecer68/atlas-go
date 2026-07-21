#!/usr/bin/env bash
# scripts/ci/render_release_body.sh
#
# Extract the CHANGELOG section for a given version so the release workflow
# can stop hardcoding release body text. Prints the section header + bullets
# to stdout; exits 1 if the version cannot be found.
#
# Usage:  bash render_release_body.sh <version>
#   e.g.  bash render_release_body.sh 0.0.2.0
#
# Recognized CHANGELOG shape (matches the format defined in the existing
# CHANGELOG.md history):
#   ## [0.0.2.0] - 2026-07-22
#   ### Fixed
#   - bullet
#   - bullet
#   ...
#   ## [0.0.1.0] - 2026-07-21
#   ...

set -euo pipefail

CHANGELOG="${CHANGELOG_PATH:-CHANGELOG.md}"
VERSION="${1:-}"

if [ -z "$VERSION" ]; then
  echo "render_release_body: VERSION argument required" >&2
  exit 2
fi

if [ ! -f "$CHANGELOG" ]; then
  echo "render_release_body: cannot find $CHANGELOG (run from repo root)" >&2
  exit 2
fi

# Strip the leading 'v' if a git tag was passed (release.yml uses ${{ github.ref_name }}).
TAG="${VERSION#v}"

# Print the matching "## [TAG]" header and everything below it up to the next
# "## " header. Use index() (not $0 ~ regex) because the bracket characters
# would otherwise be interpreted as a character class.
EXTRACTED=$(awk -v header="## [$TAG]" '
  index($0, header) == 1 {
    in_section = 1
    print
    next
  }
  /^## / {
    if (in_section) {
      in_section = 0
      exit
    }
    next
  }
  in_section {
    print
  }
' "$CHANGELOG")

if [ -z "$EXTRACTED" ]; then
  echo "render_release_body: no CHANGELOG section found for [$TAG]" >&2
  exit 1
fi

printf '%s\n' "$EXTRACTED"
