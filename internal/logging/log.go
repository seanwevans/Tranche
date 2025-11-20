package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type contextKey string

const (
	loggerKey    contextKey = "logger"
	customerKey  contextKey = "customer_id"
	requestIDKey string     = "request_id"
)

// Logger wraps slog.Logger with convenience helpers that satisfy the existing
// printf-style interfaces used across the codebase.
type Logger struct {
	inner *slog.Logger
}

// New builds a structured JSON logger annotated with the service name.
func New(service string) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{inner: slog.New(handler).With("service", service)}
}

// With returns a child logger with additional attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{inner: l.inner.With(args...)}
}

// Debugf logs a debug-level formatted message.
func (l *Logger) Debugf(format string, args ...any) {
	l.inner.Debug(fmt.Sprintf(format, args...))
}

// Infof logs an info-level formatted message.
func (l *Logger) Infof(format string, args ...any) {
	l.inner.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a warning-level formatted message.
func (l *Logger) Warnf(format string, args ...any) {
	l.inner.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs an error-level formatted message.
func (l *Logger) Errorf(format string, args ...any) {
	l.inner.Error(fmt.Sprintf(format, args...))
}

// Printf logs an info-level formatted message for compatibility with the
// standard library logger interface.
func (l *Logger) Printf(format string, args ...any) {
	l.Infof(format, args...)
}

// Println logs an info-level message with default spacing.
func (l *Logger) Println(args ...any) {
	l.inner.Info(fmt.Sprint(args...))
}

// Fatalf logs an error-level formatted message then exits.
func (l *Logger) Fatalf(format string, args ...any) {
	l.inner.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// ContextWithLogger stores a logger on the context.
func ContextWithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext returns the logger stored on the context, or the fallback if
// absent.
func FromContext(ctx context.Context, fallback *Logger) *Logger {
	if ctxLogger, ok := ctx.Value(loggerKey).(*Logger); ok && ctxLogger != nil {
		return ctxLogger
	}
	return fallback
}

// ContextWithCustomer annotates the context logger with a customer ID.
func ContextWithCustomer(ctx context.Context, customerID int64) context.Context {
	if customerID <= 0 {
		return ctx
	}
	if l, ok := ctx.Value(loggerKey).(*Logger); ok && l != nil {
		ctx = ContextWithLogger(ctx, l.With("customer_id", customerID))
	}
	return context.WithValue(ctx, customerKey, customerID)
}

// CustomerIDFromContext extracts the customer ID if present.
func CustomerIDFromContext(ctx context.Context) (int64, bool) {
	if v, ok := ctx.Value(customerKey).(int64); ok {
		return v, true
	}
	return 0, false
}

// WithRequest annotates the logger with request level fields.
func (l *Logger) WithRequest(id string, path string) *Logger {
	attrs := []any{}
	if id != "" {
		attrs = append(attrs, requestIDKey, id)
	}
	if path != "" {
		attrs = append(attrs, "path", path)
	}
	if len(attrs) == 0 {
		return l
	}
	return l.With(attrs...)
}
