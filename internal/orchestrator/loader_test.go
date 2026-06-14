package orchestrator

import (
	"testing"
)

func TestStaticLoader_LoadRegimeExecutors(t *testing.T) {
	var sl StaticLoader
	execs, err := sl.LoadRegimeExecutors()
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) < 2 {
		t.Fatalf("expected at least 2 builtin regime executors, got %d", len(execs))
	}
}

func TestStaticLoader_LoadAgentExecutors(t *testing.T) {
	var sl StaticLoader
	execs, err := sl.LoadAgentExecutors()
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) < 2 {
		t.Fatalf("expected at least 2 builtin agent executors, got %d", len(execs))
	}
}

func TestStaticLoader_LoadControlExecutors(t *testing.T) {
	var sl StaticLoader
	execs, err := sl.LoadControlExecutors()
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) < 2 {
		t.Fatalf("expected at least 2 builtin control executors, got %d", len(execs))
	}
}

func TestBuiltinAgentExecutors_Count(t *testing.T) {
	execs := builtinAgentExecutors()
	if len(execs) < 10 {
		t.Fatalf("expected at least 10 builtin agents, got %d", len(execs))
	}
}

func TestBuiltinRegimeExecutors_Count(t *testing.T) {
	execs := builtinRegimeExecutors()
	if len(execs) != 2 {
		t.Fatalf("expected 2 builtin regimes, got %d", len(execs))
	}
}

func TestBuiltinControlExecutors_Count(t *testing.T) {
	execs := builtinControlExecutors()
	if len(execs) != 3 {
		t.Fatalf("expected 3 builtin controls, got %d", len(execs))
	}
}
