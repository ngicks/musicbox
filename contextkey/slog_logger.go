package contextkey

import (
	"context"
	"log/slog"
)

type keyTy string

func ref[V any](v V) *V {
	return &v
}

var (
	KeySlogLogger = ref(keyTy("*slog.Logger"))
)

func SetSlogLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, KeySlogLogger, logger)
}

func GetSlogLogger(ctx context.Context) (logger *slog.Logger, ok bool) {
	val := ctx.Value(KeySlogLogger)
	if l, ok := val.(*slog.Logger); ok {
		return l, true
	}
	return nil, false
}

func GetSlogLoggerFallback(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	l, ok := GetSlogLogger(ctx)
	if ok {
		return l
	}
	return fallback
}

func GetSlogLoggerDefault(ctx context.Context) *slog.Logger {
	return GetSlogLoggerFallback(ctx, slog.Default())
}
