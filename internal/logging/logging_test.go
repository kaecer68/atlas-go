package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	Init("json", slog.LevelDebug)
	if Default() == nil {
		t.Fatal("logger should not be nil after Init")
	}

	Init("text", slog.LevelInfo)
}

func TestSetLogger(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, nil))
	SetLogger(custom)

	if Default() != custom {
		t.Fatal("SetLogger did not set the logger")
	}

	Info("test", "test_event")
	if !strings.Contains(buf.String(), "test_event") {
		t.Fatal("custom logger should have captured the log")
	}

	Init("text", slog.LevelInfo)
}

func TestDefault(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("Default() should not return nil")
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	SetLogger(custom)

	Info("comp", "info_event", "key", "val")
	if !strings.Contains(buf.String(), "info_event") {
		t.Error("Info log not captured")
	}

	buf.Reset()
	Error("comp", "error_event", "key", "val")
	if !strings.Contains(buf.String(), "error_event") {
		t.Error("Error log not captured")
	}

	buf.Reset()
	Warn("comp", "warn_event", "key", "val")
	if !strings.Contains(buf.String(), "warn_event") {
		t.Error("Warn log not captured")
	}

	buf.Reset()
	Debug("comp", "debug_event", "key", "val")
	if !strings.Contains(buf.String(), "debug_event") {
		t.Error("Debug log not captured")
	}

	Init("text", slog.LevelInfo)
}

func TestWith(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, nil))
	SetLogger(custom)

	l := With("comp", "extra", "value")
	l.Info("with_event")

	output := buf.String()
	if !strings.Contains(output, "with_event") {
		t.Error("With logger did not log")
	}
	if !strings.Contains(output, "extra") {
		t.Error("With logger missing extra key")
	}

	Init("text", slog.LevelInfo)
}

func TestFieldHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want string
	}{
		{"Component", Component("test").(slog.Attr).Value.String(), "test"},
		{"Event", Event("test").(slog.Attr).Value.String(), "test"},
		{"Symbol", Symbol("AAPL").(slog.Attr).Value.String(), "AAPL"},
		{"SessionID", SessionID("sid").(slog.Attr).Value.String(), "sid"},
		{"AgentID", AgentID("aid").(slog.Attr).Value.String(), "aid"},
		{"FStr", FStr("k", "v").(slog.Attr).Value.String(), "v"},
		{"FInt", FInt("k", 42).(slog.Attr).Value.String(), "42"},
		{"FFloat64", FFloat64("k", 3.14).(slog.Attr).Value.String(), "3.14"},
		{"FBool", FBool("k", true).(slog.Attr).Value.Bool(), "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			switch v := tt.got.(type) {
			case string:
				got = v
			case bool:
				if v {
					got = "true"
				} else {
					got = "false"
				}
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErr(t *testing.T) {
	attr := Err(nil)
	if attr != nil {
		t.Error("Err(nil) should return nil")
	}

	attr = Err(context.Canceled)
	if attr == nil {
		t.Error("Err(err) should not return nil")
	}
}

func TestLegacyLog(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, nil))
	SetLogger(custom)

	LegacyLog("comp", "test %s %d", "arg1", 42)
	output := buf.String()
	if !strings.Contains(output, "legacy") {
		t.Error("LegacyLog missing 'legacy' marker")
	}
	if !strings.Contains(output, "test arg1 42") {
		t.Error("LegacyLog formatting incorrect")
	}

	Init("text", slog.LevelInfo)
}

func TestWithLogger(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, nil))

	ctx := context.Background()
	ctx = WithLogger(ctx, custom)

	l := FromContext(ctx)
	if l == nil {
		t.Fatal("FromContext returned nil")
	}

	l.Info("context_event")
	if !strings.Contains(buf.String(), "context_event") {
		t.Error("logger from context did not log")
	}
}

func TestFromContextFallback(t *testing.T) {
	ctx := context.Background()
	l := FromContext(ctx)
	if l == nil {
		t.Fatal("FromContext should fallback to slog.Default")
	}
}

func TestContextLogLevels(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := WithLogger(context.Background(), custom)

	InfoContext(ctx, "info_msg", "k", "v")
	if !strings.Contains(buf.String(), "info_msg") {
		t.Error("InfoContext not captured")
	}

	buf.Reset()
	ErrorContext(ctx, "error_msg", "k", "v")
	if !strings.Contains(buf.String(), "error_msg") {
		t.Error("ErrorContext not captured")
	}

	buf.Reset()
	WarnContext(ctx, "warn_msg", "k", "v")
	if !strings.Contains(buf.String(), "warn_msg") {
		t.Error("WarnContext not captured")
	}

	buf.Reset()
	DebugContext(ctx, "debug_msg", "k", "v")
	if !strings.Contains(buf.String(), "debug_msg") {
		t.Error("DebugContext not captured")
	}
}
