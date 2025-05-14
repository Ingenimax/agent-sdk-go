package logging

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger is an interface for logging
type Logger interface {
	Info(ctx context.Context, msg string, fields map[string]interface{})
	Warn(ctx context.Context, msg string, fields map[string]interface{})
	Error(ctx context.Context, msg string, fields map[string]interface{})
	Debug(ctx context.Context, msg string, fields map[string]interface{})
}

// ZeroLogger implements Logger using zerolog
type ZeroLogger struct {
	logger zerolog.Logger
}

// NoOpLogger is a logger that doesn't log anything (useful for testing)
type NoOpLogger struct{}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

// Info implements the Logger interface but does nothing
func (l *NoOpLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {}

// Warn implements the Logger interface but does nothing
func (l *NoOpLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {}

// Error implements the Logger interface but does nothing
func (l *NoOpLogger) Error(ctx context.Context, msg string, fields map[string]interface{}) {}

// Debug implements the Logger interface but does nothing
func (l *NoOpLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {}

// New creates a new ZeroLogger
func New() *ZeroLogger {
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	logger := zerolog.New(output).With().Timestamp().Logger()
	return &ZeroLogger{logger: logger}
}

// WithLevel creates a new ZeroLogger with the specified level
func WithLevel(level string) func(*ZeroLogger) {
	return func(l *ZeroLogger) {
		switch level {
		case "debug":
			l.logger = l.logger.Level(zerolog.DebugLevel)
		case "info":
			l.logger = l.logger.Level(zerolog.InfoLevel)
		case "warn":
			l.logger = l.logger.Level(zerolog.WarnLevel)
		case "error":
			l.logger = l.logger.Level(zerolog.ErrorLevel)
		default:
			l.logger = l.logger.Level(zerolog.InfoLevel)
		}
	}
}

// Info logs an info message
func (l *ZeroLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	event := l.logger.Info()

	// Add trace ID if available
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		event = event.Str("trace_id", traceID)
	}

	// Add organization ID if available
	if orgID, ok := ctx.Value("org_id").(string); ok {
		event = event.Str("org_id", orgID)
	}

	// Add all fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// Warn logs a warning message
func (l *ZeroLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	event := l.logger.Warn()

	// Add trace ID if available
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		event = event.Str("trace_id", traceID)
	}

	// Add organization ID if available
	if orgID, ok := ctx.Value("org_id").(string); ok {
		event = event.Str("org_id", orgID)
	}

	// Add all fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// Error logs an error message
func (l *ZeroLogger) Error(ctx context.Context, msg string, fields map[string]interface{}) {
	event := l.logger.Error()

	// Add trace ID if available
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		event = event.Str("trace_id", traceID)
	}

	// Add organization ID if available
	if orgID, ok := ctx.Value("org_id").(string); ok {
		event = event.Str("org_id", orgID)
	}

	// Add all fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// Debug logs a debug message
func (l *ZeroLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	event := l.logger.Debug()

	// Add trace ID if available
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		event = event.Str("trace_id", traceID)
	}

	// Add organization ID if available
	if orgID, ok := ctx.Value("org_id").(string); ok {
		event = event.Str("org_id", orgID)
	}

	// Add all fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}
