package microservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUITraceCollector(t *testing.T) {
	t.Run("creates collector with default config", func(t *testing.T) {
		collector := NewUITraceCollector(nil, nil)
		assert.NotNil(t, collector)
		assert.NotNil(t, collector.config)
		assert.True(t, collector.config.Enabled)
		assert.Equal(t, 10240, collector.config.MaxBufferSizeKB)
		assert.Equal(t, "1h", collector.config.MaxTraceAge)
		assert.Equal(t, 100, collector.config.RetentionCount)
	})

	t.Run("creates collector with custom config", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 5120,
			MaxTraceAge:     "30m",
			RetentionCount:  50,
		}
		collector := NewUITraceCollector(config, nil)
		assert.NotNil(t, collector)
		assert.Equal(t, config, collector.config)
		assert.Equal(t, 5120*1024, collector.maxSizeBytes)
	})
}

func TestUITraceCollector_StartSpan(t *testing.T) {
	t.Run("creates new trace for root span", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")
		require.NotNil(t, span)

		// Check that trace was created
		traces, total := collector.GetTraces(10, 0)
		assert.Equal(t, 1, total)
		assert.Len(t, traces, 1)
		assert.Equal(t, "test-operation", traces[0].Name)
		assert.Equal(t, "running", traces[0].Status)

		// End the span
		span.End()

		// Check that trace is completed
		traces, _ = collector.GetTraces(10, 0)
		assert.Equal(t, "completed", traces[0].Status)
		assert.NotNil(t, traces[0].EndTime)
	})

	t.Run("creates child span with parent", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		ctx, parentSpan := collector.StartSpan(ctx, "parent-operation")

		// Create child span
		_, childSpan := collector.StartSpan(ctx, "child-operation")

		traces, _ := collector.GetTraces(10, 0)
		assert.Len(t, traces, 1)
		assert.Len(t, traces[0].Spans, 2)

		// Check parent-child relationship
		parentSpanData := traces[0].Spans[0]
		childSpanData := traces[0].Spans[1]
		assert.Equal(t, "parent-operation", parentSpanData.Name)
		assert.Equal(t, "child-operation", childSpanData.Name)
		assert.Equal(t, parentSpanData.ID, childSpanData.ParentID)
		assert.Equal(t, traces[0].ID, childSpanData.TraceID)

		childSpan.End()
		parentSpan.End()
	})

	t.Run("disabled collector returns no-op span", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled: false,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")
		require.NotNil(t, span)

		// Check that no trace was created
		traces, total := collector.GetTraces(10, 0)
		assert.Equal(t, 0, total)
		assert.Len(t, traces, 0)

		span.End() // Should not panic
	})
}

func TestUITraceCollector_SpanOperations(t *testing.T) {
	t.Run("AddEvent adds events to span", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")

		// Add events
		span.AddEvent("event1", map[string]interface{}{"key1": "value1"})
		span.AddEvent("event2", map[string]interface{}{"key2": "value2"})

		traces, _ := collector.GetTraces(10, 0)
		assert.Len(t, traces[0].Spans[0].Events, 2)
		assert.Equal(t, "event1", traces[0].Spans[0].Events[0].Name)
		assert.Equal(t, "value1", traces[0].Spans[0].Events[0].Attributes["key1"])

		span.End()
	})

	t.Run("SetAttribute sets attributes on span", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")

		// Set attributes
		span.SetAttribute("attr1", "value1")
		span.SetAttribute("attr2", 42)
		span.SetAttribute("input", "test input")
		span.SetAttribute("output", "test output")

		traces, _ := collector.GetTraces(10, 0)
		spanData := traces[0].Spans[0]
		assert.Equal(t, "value1", spanData.Attributes["attr1"])
		assert.Equal(t, 42, spanData.Attributes["attr2"])
		assert.Equal(t, "test input", spanData.Input)
		assert.Equal(t, "test output", spanData.Output)

		span.End()
	})

	t.Run("RecordError records error on span", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")

		// Record error
		testErr := assert.AnError
		span.RecordError(testErr)

		traces, _ := collector.GetTraces(10, 0)
		assert.Equal(t, "error", traces[0].Status)
		assert.NotNil(t, traces[0].Spans[0].Error)
		assert.Equal(t, testErr.Error(), traces[0].Spans[0].Error.Message)

		span.End()
	})
}

func TestUITraceCollector_TraceSession(t *testing.T) {
	t.Run("StartTraceSession creates root trace", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartTraceSession(ctx, "test-session")
		require.NotNil(t, span)

		traces, total := collector.GetTraces(10, 0)
		assert.Equal(t, 1, total)
		assert.Equal(t, "session:test-session", traces[0].Name)
		assert.Equal(t, "test-session", traces[0].Spans[0].Attributes["session_id"])
		assert.Equal(t, true, traces[0].Spans[0].Attributes["is_root"])

		span.End()
	})
}

func TestUITraceCollector_GetTraces(t *testing.T) {
	t.Run("returns traces with pagination", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		// Create multiple traces
		for i := 0; i < 5; i++ {
			ctx := context.Background()
			_, span := collector.StartSpan(ctx, "operation")
			span.End()
			time.Sleep(10 * time.Millisecond) // Ensure different timestamps
		}

		// Test pagination
		traces, total := collector.GetTraces(2, 0)
		assert.Equal(t, 5, total)
		assert.Len(t, traces, 2)

		traces, total = collector.GetTraces(2, 2)
		assert.Equal(t, 5, total)
		assert.Len(t, traces, 2)

		traces, total = collector.GetTraces(2, 4)
		assert.Equal(t, 5, total)
		assert.Len(t, traces, 1)
	})

	t.Run("returns traces ordered by start time (newest first)", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		// Create traces with different timestamps
		ctx := context.Background()
		_, span1 := collector.StartSpan(ctx, "operation1")
		span1.End()
		time.Sleep(10 * time.Millisecond)

		_, span2 := collector.StartSpan(ctx, "operation2")
		span2.End()

		traces, _ := collector.GetTraces(10, 0)
		assert.Equal(t, "operation2", traces[0].Name)
		assert.Equal(t, "operation1", traces[1].Name)
	})
}

func TestUITraceCollector_GetTrace(t *testing.T) {
	t.Run("returns specific trace by ID", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")
		span.End()

		traces, _ := collector.GetTraces(10, 0)
		traceID := traces[0].ID

		trace, err := collector.GetTrace(traceID)
		require.NoError(t, err)
		assert.Equal(t, traceID, trace.ID)
		assert.Equal(t, "test-operation", trace.Name)
	})

	t.Run("returns error for non-existent trace", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		_, err := collector.GetTrace("non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trace not found")
	})
}

func TestUITraceCollector_DeleteTrace(t *testing.T) {
	t.Run("deletes trace by ID", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		ctx := context.Background()
		_, span := collector.StartSpan(ctx, "test-operation")
		span.End()

		traces, _ := collector.GetTraces(10, 0)
		traceID := traces[0].ID

		err := collector.DeleteTrace(traceID)
		require.NoError(t, err)

		// Verify trace is deleted
		_, err = collector.GetTrace(traceID)
		assert.Error(t, err)

		traces, total := collector.GetTraces(10, 0)
		assert.Equal(t, 0, total)
		assert.Len(t, traces, 0)
	})

	t.Run("returns error for non-existent trace", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		err := collector.DeleteTrace("non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trace not found")
	})
}

func TestUITraceCollector_GetStats(t *testing.T) {
	t.Run("returns trace statistics", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		// Create some traces
		ctx := context.Background()

		// Completed trace
		_, span1 := collector.StartSpan(ctx, "operation1")
		span1.End()

		// Error trace
		_, span2 := collector.StartSpan(ctx, "operation2")
		span2.RecordError(assert.AnError)
		span2.End()

		// Running trace
		_, span3 := collector.StartSpan(ctx, "operation3")

		// Tool call trace
		_, span4 := collector.StartSpan(ctx, "tool_call_function")
		span4.SetAttribute("tool_name", "test_tool")
		span4.End()

		stats := collector.GetStats()
		assert.Equal(t, 4, stats["total_traces"])
		assert.Equal(t, 1, stats["running_traces"])
		assert.Equal(t, 1, stats["error_count"])
		assert.Equal(t, 0.25, stats["error_rate"])

		toolUsage := stats["tool_usage"].(map[string]int)
		assert.Equal(t, 1, toolUsage["test_tool"])

		span3.End()
	})
}

func TestUITraceCollector_RetentionLimits(t *testing.T) {
	t.Run("enforces retention count limit", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  3,
		}
		collector := NewUITraceCollector(config, nil)

		// Create more traces than retention limit
		for i := 0; i < 5; i++ {
			ctx := context.Background()
			_, span := collector.StartSpan(ctx, "operation")
			span.End()
			time.Sleep(10 * time.Millisecond)
		}

		traces, total := collector.GetTraces(10, 0)
		assert.Equal(t, 3, total)
		assert.Len(t, traces, 3)
		// Should keep the newest traces
		assert.Equal(t, "operation", traces[0].Name)
	})

	t.Run("enforces size limit", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 1, // Very small buffer (1KB)
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		// Create large traces
		for i := 0; i < 10; i++ {
			ctx := context.Background()
			_, span := collector.StartSpan(ctx, "operation")
			// Add large content to span
			largeContent := make([]byte, 500) // 500 bytes
			for j := range largeContent {
				largeContent[j] = 'a'
			}
			span.SetAttribute("large_data", string(largeContent))
			span.End()
		}

		// Should have removed old traces to stay under size limit
		_, total := collector.GetTraces(10, 0)
		assert.Less(t, total, 10)
		assert.LessOrEqual(t, collector.currentSizeBytes, config.MaxBufferSizeKB*1024)
	})
}

func TestUITraceCollector_InferSpanType(t *testing.T) {
	t.Run("infers correct span types", func(t *testing.T) {
		config := &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240,
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
		collector := NewUITraceCollector(config, nil)

		testCases := []struct {
			name     string
			expected string
		}{
			{"generation", "generation"},
			{"llm_completion", "generation"},
			{"tool_call", "tool_call"},
			{"function_call", "tool_call"},
			{"event_processing", "event"},
			{"custom_span", "span"},
		}

		for _, tc := range testCases {
			ctx := context.Background()
			_, span := collector.StartSpan(ctx, tc.name)
			span.End()

			traces, _ := collector.GetTraces(10, 0)
			spanData := traces[0].Spans[0]
			assert.Equal(t, tc.expected, spanData.Type, "Failed for span name: %s", tc.name)

			// Clean up for next test
			_ = collector.DeleteTrace(traces[0].ID)
		}
	})
}