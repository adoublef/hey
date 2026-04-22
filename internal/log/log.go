package log

import (
	"context"
	"log/slog"
)

type contextKey struct {
	name string
}

var (
	logContextKey = &contextKey{"slog-logger"}
)

// WithLogger
func WithLogger(parent context.Context, handler slog.Handler) context.Context {
	ctx := context.WithValue(parent, logContextKey, slog.New(handler))
	// if a logger exists, we just get thte attributes?
	// if not, we set a new root logger?
	return ctx
}

// Info
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	logA(ctx, slog.LevelInfo, msg, attrs...)
}

func logA(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	l, ok := ctx.Value(logContextKey).(*slog.Logger)
	if !ok {
		slog.Default().LogAttrs(ctx, level, msg, attrs...)
	} else {
		l.LogAttrs(ctx, level, msg, attrs...)
	}
}
