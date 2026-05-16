package config

// LogConfig holds configuration for the logger.
type LogConfig struct {
	Level  string // e.g., "info", "debug", "error"
	Format string // e.g., "text", "json"
}

// OTELConfig holds configuration for OpenTelemetry exporters and resources.
type OTELConfig struct {
	ServiceName  string
	ExporterType string // e.g., "otlp", "stdout"
	Endpoint     string // OTLP endpoint
	Insecure     bool   // Allow insecure connection (for local testing)
}

// Config holds configuration for all observability components.
type Config struct {
	Log  LogConfig
	OTEL OTELConfig
}
