package main

import (
	"strings"
	"testing"
)

func TestRun_RejectsEmptyWorkdir(t *testing.T) {
	err := run("")
	if err == nil {
		t.Fatal("expected error for empty workdir")
	}
	if !strings.Contains(err.Error(), "workdir") {
		t.Fatalf("expected error to mention workdir, got: %v", err)
	}
}
