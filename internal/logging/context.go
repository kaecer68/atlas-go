package logging

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// WithLogger stores a logger in the context for retrieval by FromContext.
// Use this at the top of a request/operation to propagate a configured logger.
//
//	ctx = logging.WithLogger(ctx, slog.Default())
//	slog.InfoContext(ctx, "operation started")
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext retrieves the logger from the context.
// It NEVER returns nil — if no logger is found, it falls back to slog.Default(),
// which is always initialized by this package's init().
//
// This is the preferred way to obtain a logger in new code:
//
//	logger := logging.FromContext(ctx)
//	logger.Info("event", "key", val)
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// Context functions are convenience wrappers around slog.*Context with the logger
// obtained from FromContext. These are for use in new code that receives a
// context.Context.

// InfoContext logs at Info level, reading the logger from ctx.
func InfoContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Log(ctx, slog.LevelInfo, msg, args...)
}

// ErrorContext logs at Error level, reading the logger from ctx.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Log(ctx, slog.LevelError, msg, args...)
}

// WarnContext logs at Warn level, reading the logger from ctx.
func WarnContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Log(ctx, slog.LevelWarn, msg, args...)
}

// DebugContext logs at Debug level, reading the logger from ctx.
func DebugContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Log(ctx, slog.LevelDebug, msg, args...)
}
