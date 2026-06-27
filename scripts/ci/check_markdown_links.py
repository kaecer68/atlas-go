#!/usr/bin/env python3
"""
Markdown internal link validator.

Validates:
  1. Markdown link syntax: [text](relative/path.md)
  2. Bare paths in backticks: `docs/specs/foo.md`

Excludes (NOT validated as broken):
  - http(s)://, mailto:, #anchor
  - Home-directory paths starting with ~/
  - Template variables like <mod>, <topic>, ${X}
  - Glob patterns (paths containing *)
  - Paths already wrapped in []()
  - Files in .git/, vendor/, node_modules/, .omo/ (handled by find exclude)

Exit 0 = all valid; Exit 1 = broken links found
"""
import os
import re
import sys

EXTERNAL_PREFIXES = ("https://", "http://", "mailto:", "#", "data:", "javascript:", "~/")
TEMPLATE_PATTERN = re.compile(r"<[^>]+>|\$\{[^}]+\}")

# Strict match: must start with common markdown directories.
# Excludes home-directory paths (~/...) — those are external.
# The match captures paths that end in .md.
STRICT_MD_PATH = re.compile(
    r"`((?:docs|internal|cmd|scripts|web|data)/[^\s`]*?\.md)`"
    r"|`(/[^\s`]*?\.md)`"
)

# Markdown link syntax [text](url)
LINK_PATTERN = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")


def get_link_spans(content):
    """Return (start, end) of all []() links in content."""
    return [(m.start(), m.end()) for m in LINK_PATTERN.finditer(content)]


def is_in_link_span(pos, link_spans):
    """Check if pos is within any []() link span."""
    return any(start <= pos < end for start, end in link_spans)


def should_skip_path(path):
    """Check if path matches patterns that should be skipped (not validated as broken)."""
    # Glob patterns (contain * anywhere)
    if "*" in path:
        return True
    # Template variables
    if TEMPLATE_PATTERN.search(path):
        return True
    return False


def validate_path(md_file, path):
    """Validate that a relative path to a .md file exists."""
    if any(path.startswith(p) for p in EXTERNAL_PREFIXES):
        return True

    # Strip anchor
    path = path.split("#", 1)[0]
    if not path or not path.endswith(".md"):
        return True

    # Skip glob patterns and template variables
    if should_skip_path(path):
        return True

    # Handle absolute repo paths (start with /)
    if path.startswith("/"):
        resolved = "." + path
    else:
        base_dir = os.path.dirname(md_file)
        resolved = os.path.normpath(os.path.join(base_dir, path))

    return os.path.exists(resolved) or os.path.exists(path)


def main():
    if len(sys.argv) < 2:
        print("Usage: check_markdown_links.py <file.md> [<file.md> ...]")
        sys.exit(2)

    errors = []
    link_count = 0
    bare_count = 0
    skipped = 0

    for md_file in sys.argv[1:]:
        try:
            with open(md_file, "r", encoding="utf-8") as f:
                content = f.read()
        except (IOError, UnicodeDecodeError) as e:
            print(f"WARN: skip {md_file}: {e}", file=sys.stderr)
            continue

        link_spans = get_link_spans(content)

        # 1) Standard [text](url) links
        for match in LINK_PATTERN.finditer(content):
            url = match.group(2).strip()
            if should_skip_path(url):
                skipped += 1
                continue
            link_count += 1
            if not validate_path(md_file, url):
                errors.append((md_file, match.group(0), url, "LINK"))

        # 2) Bare paths in backticks
        lines = content.split("\n")
        for match in STRICT_MD_PATH.finditer(content):
            path = match.group(1) or match.group(2)
            if not path:
                continue
            if should_skip_path(path):
                skipped += 1
                continue
            if is_in_link_span(match.start(), link_spans):
                continue
            line_num = content[: match.start()].count("\n") + 1
            if line_num <= len(lines) and "取代對象" in lines[line_num - 1]:
                    skipped += 1
                    continue
            bare_count += 1
            if not validate_path(md_file, path):
                errors.append((md_file, match.group(0), path, "BARE"))

    if errors:
        print(f"Found {len(errors)} broken internal link(s):\n")
        for md_file, full_match, path, kind in errors:
            with open(md_file, "r", encoding="utf-8") as f:
                for i, line in enumerate(f, 1):
                    if full_match in line:
                        print(f"  {md_file}:{i} [{kind}]: {path}")
                        print(f"    → {line.rstrip()[:120]}")
                        break
        sys.exit(1)

    total = link_count + bare_count
    print(
        f"Checked {link_count} link + {bare_count} bare path = {total} reference(s) "
        f"({skipped} skipped: globs/templates)"
    )
    print("All internal markdown links are valid")


if __name__ == "__main__":
    main()
