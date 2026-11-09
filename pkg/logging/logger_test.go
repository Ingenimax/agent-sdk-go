package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBufferLogger returns a logger that writes JSON lines to buf.
func newBufferLogger(buf *bytes.Buffer) *ZeroLogger {
	return &ZeroLogger{logger: zerolog.New(buf)}
}

// decodeLines parses each JSON log line written to buf.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var m map[string]any
		require.NoError(t, dec.Decode(&m))
		lines = append(lines, m)
	}
	return lines
}

//nolint:staticcheck // SA1029: the logger reads plain string keys from the context.
func contextWithIDs() context.Context {
	ctx := context.WithValue(context.Background(), "trace_id", "trace-1")
	return context.WithValue(ctx, "org_id", "org-1")
}

func TestLevelMethods(t *testing.T) {
	tests := []struct {
		name  string
		level string
		log   func(*ZeroLogger, context.Context, string, map[string]interface{})
	}{
		{"debug", "debug", (*ZeroLogger).Debug},
		{"info", "info", (*ZeroLogger).Info},
		{"warn", "warn", (*ZeroLogger).Warn},
		{"error", "error", (*ZeroLogger).Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := newBufferLogger(&buf)

			tt.log(l, contextWithIDs(), "hello", map[string]interface{}{"count": 2, "key": "value"})

			lines := decodeLines(t, &buf)
			require.Len(t, lines, 1)
			got := lines[0]
			assert.Equal(t, tt.level, got["level"])
			assert.Equal(t, "hello", got["message"])
			assert.Equal(t, "trace-1", got["trace_id"])
			assert.Equal(t, "org-1", got["org_id"])
			assert.Equal(t, "value", got["key"])
			assert.Equal(t, float64(2), got["count"])
		})
	}
}

func TestLevelMethodsWithoutContextIDs(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferLogger(&buf)

	l.Info(context.Background(), "plain", nil)

	lines := decodeLines(t, &buf)
	require.Len(t, lines, 1)
	assert.NotContains(t, lines[0], "trace_id")
	assert.NotContains(t, lines[0], "org_id")
}

func TestContextIDsOfWrongTypeAreIgnored(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferLogger(&buf)

	//nolint:staticcheck // SA1029: matches the plain string key the logger reads.
	ctx := context.WithValue(context.Background(), "trace_id", 42)
	l.Warn(ctx, "typed", nil)

	lines := decodeLines(t, &buf)
	require.Len(t, lines, 1)
	assert.NotContains(t, lines[0], "trace_id")
}

func TestWithLevel(t *testing.T) {
	tests := []struct {
		level string
		want  zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"unknown", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			l := newBufferLogger(&buf)

			WithLevel(tt.level)(l)

			assert.Equal(t, tt.want, l.logger.GetLevel())
		})
	}
}

func TestWithLevelFiltersMessages(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferLogger(&buf)
	WithLevel("warn")(l)

	ctx := context.Background()
	l.Debug(ctx, "debug", nil)
	l.Info(ctx, "info", nil)
	l.Warn(ctx, "warn", nil)
	l.Error(ctx, "error", nil)

	var messages []any
	for _, line := range decodeLines(t, &buf) {
		messages = append(messages, line["message"])
	}
	assert.Equal(t, []any{"warn", "error"}, messages)
}

// newCaptured builds a logger with New while os.Stdout points at a temp file,
// writes one info line, and returns what was written.
func newCaptured(t *testing.T) string {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	orig := os.Stdout
	os.Stdout = f
	l := New()
	os.Stdout = orig

	l.Info(context.Background(), "hello", nil)
	out, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	return string(out)
}

func TestNew(t *testing.T) {
	// The package-level flag is shared, so restore it after the test.
	orig := zeroLogJsonEnable
	t.Cleanup(func() { zeroLogJsonEnable = orig })

	zeroLogJsonEnable = false
	console := newCaptured(t)
	assert.Contains(t, console, "hello")
	assert.False(t, json.Valid([]byte(console)), "console output is not JSON: %s", console)

	SetZeroLogJsonEnabled()
	assert.True(t, zeroLogJsonEnable)
	var line map[string]any
	require.NoError(t, json.Unmarshal([]byte(newCaptured(t)), &line))
	assert.Equal(t, "hello", line["message"])
	assert.Equal(t, "info", line["level"])
}
