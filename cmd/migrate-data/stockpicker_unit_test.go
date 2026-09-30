package main

import (
	"strings"
	"testing"
)

func TestCompareDigestsMatch(t *testing.T) {
	want := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 10, s1: 7, s2: 10},
		{a: "condA", b: "2026-08"}: {n: 5, s1: 2, s2: 4},
	}
	if got := compareDigests(want, want); len(got) != 0 {
		t.Fatalf("identical digests must match, got %v", got)
	}
}

func TestCompareDigestsMismatchKinds(t *testing.T) {
	want := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 10, s1: 7, s2: 10},
		{a: "condB", b: "2026-09"}: {n: 3, s1: 1, s2: 3},
	}
	got := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 9, s1: 7, s2: 10}, // differs
		{a: "condC", b: "2026-09"}: {n: 3, s1: 1, s2: 3},  // extra (condB missing)
	}
	msgs := compareDigests(want, got)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 mismatch messages, got %d: %v", len(msgs), msgs)
	}
	joined := strings.Join(msgs, "\n")
	for _, wantSub := range []string{"condA|2026-09 differs", "condB|2026-09 missing", "condC|2026-09 extra"} {
		if !strings.Contains(joined, wantSub) {
			t.Fatalf("expected message containing %q, got:\n%s", wantSub, joined)
		}
	}
}
