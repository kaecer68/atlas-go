package llm_annotator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestJSONLStore_WritesRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "annotations.jsonl")

	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	defer store.Close()

	recs := []AnnotationRecord{
		{ID: "ann-1", Timestamp: time.Now(), Label: "alerts", Tokens: 100, Outcome: "success", LatencyMs: 50},
		{ID: "ann-2", Timestamp: time.Now(), Label: "summaries", Tokens: 200, Outcome: "success", LatencyMs: 75},
		{ID: "ann-3", Timestamp: time.Now(), Label: "", Tokens: 0, Outcome: "circuit_open", LatencyMs: 1},
	}
	for _, r := range recs {
		if err := store.Write(r); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var got []AnnotationRecord
	for scanner.Scan() {
		var r AnnotationRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != len(recs) {
		t.Fatalf("read %d records, want %d", len(got), len(recs))
	}
	for i, r := range recs {
		if got[i].ID != r.ID {
			t.Errorf("[%d] ID = %q, want %q", i, got[i].ID, r.ID)
		}
		if got[i].Tokens != r.Tokens {
			t.Errorf("[%d] Tokens = %d, want %d", i, got[i].Tokens, r.Tokens)
		}
	}
}

func TestJSONLStore_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "annotations.jsonl")
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	defer store.Close()

	const goroutines = 8
	const perG = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(gid int) {
			defer wg.Done()
			for j := range perG {
				rec := AnnotationRecord{ID: fmt.Sprintf("g%d-%d", gid, j), Tokens: int64(j)}
				if err := store.Write(rec); err != nil {
					t.Errorf("Write g=%d j=%d: %v", gid, j, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	store.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := bytes.Count(data, []byte{'\n'})
	want := goroutines * perG
	if lines != want {
		t.Errorf("lines in file = %d, want %d", lines, want)
	}
}

func TestJSONLStore_RotatesOnSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "annotations.jsonl")

	store, err := NewJSONLStoreWithRotation(path, 256, 3)
	if err != nil {
		t.Fatalf("NewJSONLStoreWithRotation: %v", err)
	}
	defer store.Close()

	bigRec := AnnotationRecord{ID: "big", Label: "xxxxxxxxxxxx", Tokens: 1, Outcome: "success", LatencyMs: 1, Timestamp: time.Now()}
	for range 100 {
		_ = store.Write(bigRec)
	}
	store.Close()

	files, err := filepath.Glob(filepath.Join(dir, "annotations.jsonl*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(files) < 2 {
		t.Errorf("expected rotated files, got %v", files)
	}
}

func TestKimiClient_PersistsAnnotationsToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "annotations.jsonl")

	k := &KimiClient{}
	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	defer store.Close()
	k.SetAnnotationStore(store)

	for i := range 5 {
		k.appendAnnotation(AnnotationRecord{ID: fmt.Sprintf("ann-%d", i), Label: "test", Tokens: int64(i * 10)})
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for scanner.Scan() {
		count++
	}
	if count != 5 {
		t.Errorf("persisted %d lines, want 5", count)
	}
}
