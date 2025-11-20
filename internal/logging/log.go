package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps a slog.Logger with helpers for legacy print-style logging.
type Logger struct {
	base *slog.Logger
}

// New creates a structured JSON logger tagged with the given service name.
func New(service string) *Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level, AddSource: true})
	return &Logger{base: slog.New(handler).With("service", service)}
}

// FromContext returns the request-scoped logger if present.
func FromContext(ctx context.Context, fallback *Logger) *Logger {
	if ctx == nil {
		return fallback
	}
	if l := ctx.Value(loggerKey{}); l != nil {
		if logger, ok := l.(*Logger); ok {
			return logger
		}
	}
	return fallback
}

// ContextWithLogger injects the logger into the context.
func ContextWithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

type loggerKey struct{}

// With appends structured attributes to the logger.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{base: l.base.With(args...)}
}

// WithRequestID returns a logger annotated with a request identifier.
func (l *Logger) WithRequestID(requestID string) *Logger {
	if requestID == "" {
		return l
	}
	return l.With("request_id", requestID)
}

// WithCustomerID annotates the logger with a customer identifier.
func (l *Logger) WithCustomerID(customerID int64) *Logger {
	if customerID == 0 {
		return l
	}
	return l.With("customer_id", customerID)
}

func (l *Logger) Debug(msg string, args ...any) { l.base.Debug(msg, args...) }

func (l *Logger) Info(msg string, args ...any) { l.base.Info(msg, args...) }

func (l *Logger) Infof(format string, args ...any) { l.base.Info(fmt.Sprintf(format, args...)) }

func (l *Logger) Warn(msg string, args ...any) { l.base.Warn(msg, args...) }

func (l *Logger) Error(msg string, args ...any) { l.base.Error(msg, args...) }

func (l *Logger) Errorf(format string, args ...any) { l.base.Error(fmt.Sprintf(format, args...)) }

// Printf logs at info level for backwards compatibility.
func (l *Logger) Printf(format string, args ...any) { l.base.Info(fmt.Sprintf(format, args...)) }

// Println logs a concatenated message at info level.
func (l *Logger) Println(args ...any) { l.base.Info(fmt.Sprint(args...)) }

// Fatalf logs an error and exits.
func (l *Logger) Fatalf(format string, args ...any) {
	l.base.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Fatal logs a message and exits.
func (l *Logger) Fatal(args ...any) {
	l.base.Error(fmt.Sprint(args...))
	os.Exit(1)
}
