#!/usr/bin/env python3
"""Merge two darwinian_history.jsonl files into one (union, dedupe by line, sort by timestamp).

Usage: merge_darwinian.py <fileA> <fileB> <output>
- Preserves ALL unique lines from both files (no line dropped except exact duplicates).
- Sorts by snapshot timestamp (the "timestamp" field).
- Writes atomically (tmp + rename).
"""
import json
import hashlib
import sys
import os


def load_lines(path):
    seen = set()
    rows = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            h = hashlib.sha256(line.encode()).hexdigest()
            if h in seen:
                continue
            seen.add(h)
            try:
                ts = json.loads(line)["timestamp"]
            except Exception as e:
                print(f"WARN: unparseable line in {path}: {e}", file=sys.stderr)
                continue
            rows.append((ts, line))
    return rows


def main():
    if len(sys.argv) != 4:
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    a_path, b_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]

    rows_a = load_lines(a_path)
    rows_b = load_lines(b_path)
    print(f"file A ({a_path}): {len(rows_a)} unique lines")
    print(f"file B ({b_path}): {len(rows_b)} unique lines")

    # union (rowA unique + rowB not already present)
    seen_lines = {line for _, line in rows_a}
    added = 0
    for ts, line in rows_b:
        if line not in seen_lines:
            rows_a.append((ts, line))
            seen_lines.add(line)
            added += 1
    print(f"lines added from B (not in A): {added}")
    print(f"total merged lines: {len(rows_a)}")

    # stable sort by timestamp
    rows_a.sort(key=lambda r: r[0])
    print(f"timestamp range: {rows_a[0][0]} -> {rows_a[-1][0]}")

    # check monotonic
    tss = [r[0] for r in rows_a]
    monotonic = all(tss[i] <= tss[i + 1] for i in range(len(tss) - 1))
    print(f"timestamp monotonic: {monotonic}")
    if not monotonic:
        print("WARN: timestamps NOT monotonic after sort", file=sys.stderr)

    # atomic write
    tmp = out_path + ".tmp"
    with open(tmp, "w") as f:
        for _, line in rows_a:
            f.write(line + "\n")
    os.rename(tmp, out_path)
    print(f"wrote: {out_path} ({len(rows_a)} lines)")


if __name__ == "__main__":
    main()
