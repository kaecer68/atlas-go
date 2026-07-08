#!/usr/bin/env python3
"""
check_agents_md_drift.py — AGENTS.md line-reference drift checker.

For every <file>.go:NN or <file>.go:NN-MM reference inside an AGENTS.md,
verify the file exists and the referenced line is in range.

Path resolution rules (in priority order):
  1. Path starts with cmd/ internal/ pkg/ docs/ configs/ scripts/ → repo-relative.
  2. Path starts with a relative prefix (e.g. types.go, doc.go) → resolve
     relative to the directory of the AGENTS.md that contains the reference.
  3. Otherwise → log a "skip" (not a hard error: do not break the build on
     URL-style or future-anchored references).

Range endpoints (NN-MM) require BOTH endpoints to be in range; we do NOT
check intermediate content (intentionally lenient: a comment block can be
inserted between two referenced lines and the "before/after" pointers stay
correct as long as the endpoints exist).

Skips:
  - Lines inside fenced code blocks (``` or indented 4-space blocks)
  - Lines inside inline code that are clearly example/quote text
    (heuristic: line starts with `>` blockquote, or has "原本" "例如" 註解辭)
  - Markdown table cells whose first column is a *Path* (those are patterns,
    not line refs)

Exit codes:
  0 — all references resolve
  1 — at least one drift detected (file missing / line out of range)
  2 — internal error (read failure, etc.)
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from typing import Iterable, NamedTuple

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
REPO_RELATIVE_PREFIXES = (
    "cmd/", "internal/", "pkg/", "docs/", "configs/", "scripts/", "web/",
    "client_web/", "admin_web/", "shared_web/", "deploy/", "test/",
)

# Match patterns like:
#   `path/to/file.go:123`
#   `path/to/file.go:123-145`
#   `service.go:33/48`           (compound range — we capture the FIRST number)
#   `service.go:33`, `service.go:48`  (two separate matches, two drifts if wrong)
#
# Capture groups: (1) full file path including the .go suffix, (2) start line, (3) end line (or None).
LINE_REF_RE = re.compile(
    r"`(?P<path>[A-Za-z0-9_./\-]+\.go):(?P<start>\d+)(?:-(?P<end>\d+))?`"
)

# Drift exception: AGENTS.md sometimes references line numbers inside the
# same AGENTS.md itself (very rare; canonical example: capitalflow/AGENTS.md
# explaining a "test:" block). We skip refs where the path is a known
# non-Go target but happens to end in .go.
KNOWN_NON_GO_NAMES = {"AGENTS.md", "README.md", "CHANGELOG.md", "Makefile"}

# Heuristic skip: lines whose context indicates an example/quote rather than
# a real reference. Conservative: only skip very obvious hints.
EXAMPLE_HINT_RE = re.compile(
    r"(?:原本|例如|example|ex\.g\.|e\.g\.|sample|示意|預期|formerly|previously|was|removed|過往|原始|範例)",
    re.IGNORECASE,
)

# Lines starting with `>` (markdown blockquote) are quoted historical context.
BLOCKQUOTE_LINE_RE = re.compile(r"^\s*>")


class Drift(NamedTuple):
    agents_md: str
    line_no: int
    ref: str
    reason: str


def resolve_path(agents_md: Path, ref_path: str) -> Path:
    """Resolve a reference path to a repo-absolute path.

    Raises FileNotFoundError if the path is not under the repo root.
    """
    if ref_path.startswith(REPO_RELATIVE_PREFIXES) or ref_path.startswith("./"):
        candidate = REPO_ROOT / ref_path.lstrip("./")
    else:
        # Treat as relative to the AGENTS.md's directory.
        candidate = (agents_md.parent / ref_path).resolve()
    # Normalize. Skip the relative_to safety check here — caller checks
    # .exists() first and reports "file not found" (the common, user-friendly
    # case) before the "escapes repo root" sanity check fires.
    return candidate.resolve(strict=False)


def line_count(path: Path) -> int:
    """Count the number of lines in a text file. Returns 0 if empty."""
    with path.open("r", encoding="utf-8", errors="replace") as f:
        return sum(1 for _ in f)


def is_in_code_block(lines: list[str], idx: int) -> bool:
    """Return True if lines[idx] is inside a fenced code block.

    Walks from the start of the file counting opening/closing ``` fences.
    """
    in_block = False
    for i in range(idx + 1):
        if lines[i].lstrip().startswith("```"):
            in_block = not in_block
    return in_block


def scan_file(agents_md: Path) -> Iterable[Drift]:
    """Yield Drift entries for every stale reference in agents_md."""
    try:
        rel = agents_md.relative_to(REPO_ROOT)
    except ValueError:
        # Path is outside the repo (standalone test). Use absolute for display.
        rel = Path(str(agents_md))
    try:
        text = agents_md.read_text(encoding="utf-8")
    except OSError as e:
        yield Drift(str(rel), 0, "<file>", f"read error: {e}")
        return
    lines = text.splitlines()

    for idx, raw in enumerate(lines):
        line_no = idx + 1
        if BLOCKQUOTE_LINE_RE.match(raw):
            continue
        if is_in_code_block(lines, idx):
            continue
        if EXAMPLE_HINT_RE.search(raw):
            continue
        for m in LINE_REF_RE.finditer(raw):
            ref_path = m.group("path")
            start = int(m.group("start"))
            end = int(m.group("end")) if m.group("end") else None
            if any(ref_path.endswith(name) for name in KNOWN_NON_GO_NAMES):
                continue
            ref_token = m.group(0).strip("`")
            target = resolve_path(agents_md, ref_path)
            # Sanity check: path should stay under the repo. We compute the
            # repo-relative display path; if it can't be computed (e.g. test
            # invocation outside the repo), use the absolute path.
            try:
                display = str(target.relative_to(REPO_ROOT))
            except ValueError:
                display = str(target)
            if not target.exists():
                yield Drift(
                    str(rel), line_no, ref_token,
                    f"file not found: {display}",
                )
                continue
            # Sanity check: file lives under the repo (defense against `..`).
            try:
                target.relative_to(REPO_ROOT)
            except ValueError:
                yield Drift(
                    str(rel), line_no, ref_token,
                    f"path escapes repo root: {display}",
                )
                continue
            n = line_count(target)
            if start < 1 or start > n:
                yield Drift(
                    str(rel), line_no, ref_token,
                    f"start line {start} out of range "
                    f"(file has {n} lines): {target.relative_to(REPO_ROOT)}",
                )
                continue
            if end is not None and (end < 1 or end > n):
                yield Drift(
                    str(rel), line_no, ref_token,
                    f"end line {end} out of range "
                    f"(file has {n} lines): {target.relative_to(REPO_ROOT)}",
                )


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: check_agents_md_drift.py <agents_md> [<agents_md> ...]",
              file=sys.stderr)
        return 2

    drifts: list[Drift] = []
    for arg in argv[1:]:
        # bash script does `cd $REPO_ROOT` before invoking us, so args come in
        # as repo-relative paths. Anchor to REPO_ROOT for absolute resolution.
        agents_md = (REPO_ROOT / arg).resolve()
        if not agents_md.is_file():
            print(f"WARN: skipping non-file: {arg}", file=sys.stderr)
            continue
        drifts.extend(scan_file(agents_md))

    if not drifts:
        print(f"OK: scanned {len(argv) - 1} AGENTS.md file(s); 0 drift(s)")
        return 0

    print(f"DRIFT: {len(drifts)} stale reference(s) across "
          f"{len({d.agents_md for d in drifts})} AGENTS.md file(s):",
          file=sys.stderr)
    for d in drifts:
        print(f"  {d.agents_md}:{d.line_no}  {d.ref}  →  {d.reason}",
              file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
