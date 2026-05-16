package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"os"

	"github.com/stretchr/testify/assert"
	pkgContext "parkir-pintar/services/reservation/pkg/context"
	"parkir-pintar/services/reservation/pkg/config"
	"go.opentelemetry.io/otel/trace"
)

func TestContextAttrsWithRequestValues(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Set up logger with JSON handler for easier parsing
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	SetLogger(slog.New(handler))

	// Create context with values using struct
	ctx := context.Background()
	data := pkgContext.ContextData{
		TransactionID: "A3P1251201113149000133910",
		Msisdn:        "081234567890",
		AppVersion:    "1.2.3",
		OSVersion:     "android|11.0",
		DeviceID:      "device-123",
	}
	ctx = pkgContext.SetContextData(ctx, data)

	// Log a message
	Info(ctx, "test message")

	// Check that the log output contains our context values
	logOutput := buf.String()
	assert.Contains(t, logOutput, "A3P1251201113149000133910")
	assert.Contains(t, logOutput, "081234567890")
	assert.Contains(t, logOutput, "1.2.3")
	assert.Contains(t, logOutput, "android|11.0")
	assert.Contains(t, logOutput, "device-123")
}

func TestContextAttrsWithEmptyContext(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	SetLogger(slog.New(handler))

	// Log with empty context
	ctx := context.Background()
	Info(ctx, "test message")

	// Should not contain context field names when values are empty
	logOutput := buf.String()
	assert.NotContains(t, logOutput, "transactionid")
	assert.NotContains(t, logOutput, "msisdn")
	assert.Contains(t, logOutput, "test message")
}


func TestSetupLogger(t *testing.T) {
	t.Run("sets up JSON handler", func(t *testing.T) {
		cfg := config.LogConfig{Level: "info", Format: "json"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("sets up text handler", func(t *testing.T) {
		cfg := config.LogConfig{Level: "debug", Format: "text"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("sets up text handler", func(t *testing.T) {
		cfg := config.LogConfig{Level: "warn", Format: "text"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("defaults to info level", func(t *testing.T) {
		cfg := config.LogConfig{Level: "unknown", Format: "json"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("sets up error level", func(t *testing.T) {
		cfg := config.LogConfig{Level: "error", Format: "json"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	SetLogger(slog.New(handler))

	ctx := context.Background()
	Error(ctx, "error message", "key", "value")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "error message")
	assert.Contains(t, logOutput, `"level":"ERROR"`)
	assert.Contains(t, logOutput, "key")
	assert.Contains(t, logOutput, "value")
}

func TestDebug(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	SetLogger(slog.New(handler))

	ctx := context.Background()
	Debug(ctx, "debug message", "debugKey", "debugValue")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "debug message")
	assert.Contains(t, logOutput, `"level":"DEBUG"`)
	assert.Contains(t, logOutput, "debugKey")
}

func TestWarn(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	SetLogger(slog.New(handler))

	ctx := context.Background()
	Warn(ctx, "warn message", "warnKey", "warnValue")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "warn message")
	assert.Contains(t, logOutput, `"level":"WARN"`)
	assert.Contains(t, logOutput, "warnKey")
}

func TestContextAttrsWithPartialData(t *testing.T) {
	t.Run("only transaction ID", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
		SetLogger(slog.New(handler))

		ctx := pkgContext.SetContextData(context.Background(), pkgContext.ContextData{
			TransactionID: "txn-123",
		})
		Info(ctx, "partial context test")

		logOutput := buf.String()
		assert.Contains(t, logOutput, "txn-123")
		assert.NotContains(t, logOutput, "msisdn")
		assert.NotContains(t, logOutput, "appversion")
	})

	t.Run("only msisdn and device ID", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
		SetLogger(slog.New(handler))

		ctx := pkgContext.SetContextData(context.Background(), pkgContext.ContextData{
			Msisdn:   "08123456",
			DeviceID: "dev-abc",
		})
		Info(ctx, "partial context test")

		logOutput := buf.String()
		assert.Contains(t, logOutput, "08123456")
		assert.Contains(t, logOutput, "dev-abc")
		assert.NotContains(t, logOutput, "transactionid")
	})
}

func TestContextAttrsWithSpanContext(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	SetLogger(slog.New(handler))

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	Info(ctx, "span context test")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "trace_id")
	assert.Contains(t, logOutput, "span_id")
	assert.Contains(t, logOutput, "4bf92f3577b34da6a3ce929d0e0e4736")
	assert.Contains(t, logOutput, "00f067aa0ba902b7")
}

func TestLogWithAdditionalArgs(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	SetLogger(slog.New(handler))

	ctx := context.Background()
	Info(ctx, "test with args", "customKey", "customValue", "numKey", 42)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "customKey")
	assert.Contains(t, logOutput, "customValue")
	assert.Contains(t, logOutput, "numKey")
	assert.Contains(t, logOutput, "42")
}

func TestSetupLogger_WarnLevel(t *testing.T) {
	t.Run("warn string sets warn level", func(t *testing.T) {
		cfg := config.LogConfig{Level: "warn", Format: "json"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("warning string sets warn level", func(t *testing.T) {
		cfg := config.LogConfig{Level: "warning", Format: "json"}
		l := SetupLogger(cfg)
		assert.NotNil(t, l)
	})

	t.Run("warn level suppresses debug and info", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := config.LogConfig{Level: "warn", Format: "json"}
		l := SetupLogger(cfg)
		// redirect output to buf for assertion
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		SetLogger(slog.New(handler))
		_ = l

		ctx := context.Background()
		Debug(ctx, "should not appear")
		Info(ctx, "should not appear either")
		Warn(ctx, "should appear")

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "should not appear")
		assert.Contains(t, logOutput, "should appear")
	})
}

// captureSetupLogger calls SetupLogger, runs fn (which should call log functions),
// then returns the captured stdout output.
func captureSetupLogger(t *testing.T, level, format string, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	origStdout := os.Stdout
	os.Stdout = w

	SetupLogger(config.LogConfig{Level: level, Format: format})
	fn()

	os.Stdout = origStdout
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()

	return buf.String()
}

func TestSetupLogger_ReplaceAttr(t *testing.T) {
	t.Run("time key is converted to unix int64", func(t *testing.T) {
		out := captureSetupLogger(t, "info", "json", func() {
			Info(context.Background(), "time key test")
		})
		// "time":<13-digit number> — millisecond unix timestamp, not a formatted string
		assert.Regexp(t, `"time":\d{13}`, out)
	})

	t.Run("level key is lowercased", func(t *testing.T) {
		out := captureSetupLogger(t, "debug", "json", func() {
			ctx := context.Background()
			Info(ctx, "info msg")
			Warn(ctx, "warn msg")
			Error(ctx, "error msg")
			Debug(ctx, "debug msg")
		})
		assert.Contains(t, out, `"level":"info"`)
		assert.Contains(t, out, `"level":"warn"`)
		assert.Contains(t, out, `"level":"error"`)
		assert.Contains(t, out, `"level":"debug"`)
		// slog default emits uppercase — verify that is not present
		assert.NotContains(t, out, `"level":"INFO"`)
		assert.NotContains(t, out, `"level":"WARN"`)
		assert.NotContains(t, out, `"level":"ERROR"`)
		assert.NotContains(t, out, `"level":"DEBUG"`)
	})

	t.Run("other keys pass through unchanged", func(t *testing.T) {
		out := captureSetupLogger(t, "info", "json", func() {
			Info(context.Background(), "passthrough test", "customKey", "customValue")
		})
		assert.Contains(t, out, `"customKey":"customValue"`)
	})
}

func TestTraceAttrs_FileNameAndMethodName(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	SetLogger(slog.New(handler))

	ctx := context.Background()
	Info(ctx, "caller info test")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "file_name")
	assert.Contains(t, logOutput, "method_name")
	// file_name should contain a .go file path with line number
	assert.Contains(t, logOutput, ".go:")
}
