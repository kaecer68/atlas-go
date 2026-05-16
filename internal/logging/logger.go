package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var (
	mu     sync.RWMutex
	logger *slog.Logger
	logCtx context.Context
)

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logCtx = context.Background()
}

func SetLogContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	mu.Lock()
	logCtx = ctx
	mu.Unlock()
}

func Init(handler string, level slog.Level) {
	if level == 0 {
		level = slog.LevelInfo
	}
	var h slog.Handler
	switch handler {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	default:
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	l := slog.New(h)
	mu.Lock()
	logger = l
	mu.Unlock()
	slog.SetDefault(l)
}

func SetLogger(l *slog.Logger) {
	mu.Lock()
	logger = l
	mu.Unlock()
	slog.SetDefault(l)
}

func Default() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

func Info(component, event string, keyvals ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.LevelInfo, event,
		append(keyvals, "component", component)...)
}

func Error(component, event string, keyvals ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.LevelError, event,
		append(keyvals, "component", component)...)
}

func Critical(component, event string, keyvals ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.Level(12), event,
		append(keyvals, "component", component)...)
}

func Warn(component, event string, keyvals ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.LevelWarn, event,
		append(keyvals, "component", component)...)
}

func Debug(component, event string, keyvals ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.LevelDebug, event,
		append(keyvals, "component", component)...)
}

func With(component string, keyvals ...any) *slog.Logger {
	mu.RLock()
	l := logger
	mu.RUnlock()
	args := append([]any{"component", component}, keyvals...)
	return l.With(args...)
}

func Component(name string) any { return slog.String("component", name) }
func Event(name string) any     { return slog.String("event", name) }
func Symbol(ticker string) any  { return slog.String("symbol", ticker) }
func SessionID(id string) any   { return slog.String("session_id", id) }
func AgentID(id string) any     { return slog.String("agent_id", id) }
func DurationMs(ms float64) any { return slog.Float64("duration_ms", ms) }
func Err(err error) any {
	if err == nil {
		return nil
	}
	return slog.String("err", err.Error())
}
func FStr(key, val string) any             { return slog.String(key, val) }
func FInt(key string, val int) any         { return slog.Int(key, val) }
func FFloat64(key string, val float64) any { return slog.Float64(key, val) }
func FBool(key string, val bool) any       { return slog.Bool(key, val) }

func LegacyLog(component, format string, args ...any) {
	mu.RLock()
	l := logger
	ctx := logCtx
	mu.RUnlock()
	l.Log(ctx, slog.LevelInfo, "legacy",
		"component", component, "message", fmt.Sprintf(format, args...))
}
