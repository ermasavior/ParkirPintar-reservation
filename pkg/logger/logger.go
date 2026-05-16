package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"parkir-pintar/services/reservation/pkg/config"
	pkgContext "parkir-pintar/services/reservation/pkg/context"
	"go.opentelemetry.io/otel/trace"
)

var (
	Logger = slog.Default()
)

// SetupLogger initializes the slog logger with options from config.
func SetupLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		fallthrough
	default:
		level = slog.LevelInfo
	}

	replaceAttr := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Int64(slog.TimeKey, a.Value.Time().UnixMilli())
		}
		if a.Key == slog.LevelKey {
			return slog.String(slog.LevelKey, strings.ToLower(a.Value.String()))
		}
		return a
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr})
	}
	Logger = slog.New(handler)
	slog.SetDefault(Logger)
	return Logger
}

// SetLogger allows setting a custom slog.Logger instance.
func SetLogger(l *slog.Logger) {
	Logger = l
}

// Info logs an info message with optional trace/span context.
func Info(ctx context.Context, msg string, args ...any) {
	Logger.Info(msg, append(args, traceAttrs(ctx)...)...)
}

// Error logs an error message with optional trace/span context.
func Error(ctx context.Context, msg string, args ...any) {
	Logger.Error(msg, append(args, traceAttrs(ctx)...)...)
}

// Warn logs a warning message with optional trace/span context.
func Warn(ctx context.Context, msg string, args ...any) {
	Logger.Warn(msg, append(args, traceAttrs(ctx)...)...)
}

// Debug logs a debug message with optional trace/span context.
func Debug(ctx context.Context, msg string, args ...any) {
	Logger.Debug(msg, append(args, traceAttrs(ctx)...)...)
}

// traceAttrs combines file/method name, trace attributes, and request context values.
func traceAttrs(ctx context.Context) []any {
	pc, file, line, _ := runtime.Caller(2)
	details := runtime.FuncForPC(pc)

	attrs := []any{
		slog.String("file_name", fmt.Sprintf("%s:%d", file, line)),
		slog.String("method_name", details.Name()),
	}

	// Add request context values
	data := pkgContext.GetContextData(ctx)
	if data.TransactionID != "" {
		attrs = append(attrs, slog.String("transactionid", data.TransactionID))
	}
	if data.Msisdn != "" {
		attrs = append(attrs, slog.String("msisdn", data.Msisdn))
	}
	if data.AppVersion != "" {
		attrs = append(attrs, slog.String("appversion", data.AppVersion))
	}
	if data.OSVersion != "" {
		attrs = append(attrs, slog.String("osversion", data.OSVersion))
	}
	if data.DeviceID != "" {
		attrs = append(attrs, slog.String("deviceid", data.DeviceID))
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		)
	}

	return attrs
}
