package main

import (
	"strings"
	"testing"
)

func TestRun_RefusesUpdateWithSyntheticData(t *testing.T) {
	err := run([]string{"--update"})
	if err == nil {
		t.Fatal("expected error when --update is used without --replay (synthetic default)")
	}
	if !strings.Contains(err.Error(), "synthetic") {
		t.Fatalf("expected error to mention synthetic data, got: %v", err)
	}
}