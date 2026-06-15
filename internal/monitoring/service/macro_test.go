package service

import (
	"testing"
)

func TestNewMacroService(t *testing.T) {
	svc := NewMacroService("/tmp/workdir", nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.WorkDir != "/tmp/workdir" {
		t.Errorf("expected WorkDir /tmp/workdir, got %q", svc.WorkDir)
	}
}
