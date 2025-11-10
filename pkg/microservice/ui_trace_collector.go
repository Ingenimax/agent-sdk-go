package microservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/google/uuid"
)

// UITrace represents a trace in the UI
type UITrace struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	StartTime      time.Time              `json:"start_time"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	Duration       int64                  `json:"duration_ms"`
	Status         string                 `json:"status"` // running, completed, error
	Spans          []UITraceSpan          `json:"spans"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	OrgID          string                 `json:"org_id,omitempty"`
	SizeBytes      int                    `json:"size_bytes"`
}

// UITraceSpan represents a span in a trace
type UITraceSpan struct {
	ID         string                 `json:"id"`
	TraceID    string                 `json:"trace_id"`
	ParentID   string                 `json:"parent_id,omitempty"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // generation, tool_call, span, event
	StartTime  time.Time              `json:"start_time"`
	EndTime    *time.Time             `json:"end_time,omitempty"`
	Duration   int64                  `json:"duration_ms"`
	Events     []UITraceEvent         `json:"events,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Error      *UITraceError          `json:"error,omitempty"`
	Input      string                 `json:"input,omitempty"`
	Output     string                 `json:"output,omitempty"`
}

// UITraceEvent represents an event in a span
type UITraceEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// UITraceError represents an error in a span
type UITraceError struct {
	Message    string    `json:"message"`
	Type       string    `json:"type,omitempty"`
	Stacktrace string    `json:"stacktrace,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// UITracingConfig contains configuration for UI tracing
type UITracingConfig struct {
	Enabled         bool   `json:"enabled"`
	MaxBufferSizeKB int    `json:"max_buffer_size_kb"` // Default: 10240 (10MB)
	MaxTraceAge     string `json:"max_trace_age"`       // Default: "1h"
	RetentionCount  int    `json:"retention_count"`     // Default: 100 traces
}

// UITraceCollector collects traces for the UI
type UITraceCollector struct {
	config          *UITracingConfig
	wrappedTracer   interfaces.Tracer
	traces          map[string]*UITrace
	activeSpans     map[string]*uiSpanContext
	mu              sync.RWMutex
	maxSizeBytes    int
	currentSizeBytes int
	maxAge          time.Duration
}

// uiSpanContext holds context for an active span
type uiSpanContext struct {
	span       *UITraceSpan
	trace      *UITrace
	wrappedSpan interfaces.Span
}

// uiCollectorSpan wraps a span and collects data
type uiCollectorSpan struct {
	collector   *UITraceCollector
	spanContext *uiSpanContext
}

// NewUITraceCollector creates a new UI trace collector
func NewUITraceCollector(config *UITracingConfig, wrappedTracer interfaces.Tracer) *UITraceCollector {
	if config == nil {
		config = &UITracingConfig{
			Enabled:         true,
			MaxBufferSizeKB: 10240, // 10MB
			MaxTraceAge:     "1h",
			RetentionCount:  100,
		}
	}

	// Parse max age duration
	maxAge, err := time.ParseDuration(config.MaxTraceAge)
	if err != nil {
		maxAge = time.Hour // Default to 1 hour
	}

	return &UITraceCollector{
		config:          config,
		wrappedTracer:   wrappedTracer,
		traces:          make(map[string]*UITrace),
		activeSpans:     make(map[string]*uiSpanContext),
		maxSizeBytes:    config.MaxBufferSizeKB * 1024,
		maxAge:          maxAge,
	}
}

// StartSpan starts a new span and collects it
func (c *UITraceCollector) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	log.Printf("[UITraceCollector] StartSpan called with name: %s", name)

	if !c.config.Enabled {
		if c.wrappedTracer != nil {
			return c.wrappedTracer.StartSpan(ctx, name)
		}
		return ctx, &noOpSpan{}
	}

	// Start span in wrapped tracer if available
	var wrappedSpan interfaces.Span
	if c.wrappedTracer != nil {
		ctx, wrappedSpan = c.wrappedTracer.StartSpan(ctx, name)
	}

	// Create UI span
	spanID := uuid.New().String()
	span := &UITraceSpan{
		ID:         spanID,
		Name:       name,
		Type:       c.inferSpanType(name),
		StartTime:  time.Now(),
		Events:     []UITraceEvent{},
		Attributes: make(map[string]interface{}),
	}

	// Find or create trace
	var trace *UITrace

	// Try to get parent span from context
	if parentSpanID := c.getParentSpanID(ctx); parentSpanID != "" {
		c.mu.RLock()
		if parentContext, exists := c.activeSpans[parentSpanID]; exists {
			trace = parentContext.trace
			span.TraceID = trace.ID
			span.ParentID = parentSpanID
		}
		c.mu.RUnlock()
	}

	// If no parent found, create new trace
	if trace == nil {
		traceID := uuid.New().String()
		trace = &UITrace{
			ID:        traceID,
			Name:      name,
			StartTime: time.Now(),
			Status:    "running",
			Spans:     []UITraceSpan{},
			Metadata:  make(map[string]interface{}),
		}
		span.TraceID = traceID

		// Extract context metadata
		if conversationID := c.getConversationID(ctx); conversationID != "" {
			trace.ConversationID = conversationID
		}
		if orgID := c.getOrgID(ctx); orgID != "" {
			trace.OrgID = orgID
		}

		c.mu.Lock()
		c.traces[traceID] = trace
		c.enforceRetentionLimits()
		c.mu.Unlock()
	}

	// Store span context
	spanContext := &uiSpanContext{
		span:        span,
		trace:       trace,
		wrappedSpan: wrappedSpan,
	}

	c.mu.Lock()
	c.activeSpans[spanID] = spanContext
	trace.Spans = append(trace.Spans, *span)
	c.updateTraceSize(trace)
	c.mu.Unlock()

	// Store span ID in context for child spans
	ctx = context.WithValue(ctx, spanIDKey{}, spanID)

	return ctx, &uiCollectorSpan{
		collector:   c,
		spanContext: spanContext,
	}
}

// StartTraceSession starts a root trace session
func (c *UITraceCollector) StartTraceSession(ctx context.Context, contextID string) (context.Context, interfaces.Span) {
	log.Printf("[UITraceCollector] StartTraceSession called with contextID: %s", contextID)

	if !c.config.Enabled {
		log.Printf("[UITraceCollector] Tracing disabled, delegating to wrapped tracer")
		if c.wrappedTracer != nil {
			return c.wrappedTracer.StartTraceSession(ctx, contextID)
		}
		return ctx, &noOpSpan{}
	}

	// Create a root span with the session name
	sessionName := fmt.Sprintf("session:%s", contextID)
	log.Printf("[UITraceCollector] Creating root span: %s", sessionName)
	ctx, span := c.StartSpan(ctx, sessionName)

	// Add session metadata
	span.SetAttribute("session_id", contextID)
	span.SetAttribute("is_root", true)

	log.Printf("[UITraceCollector] Root trace session started successfully")
	return ctx, span
}

// End ends the span
func (s *uiCollectorSpan) End() {
	log.Printf("[UITraceCollector] End() called for span: %s", s.spanContext.span.Name)

	if s.spanContext.wrappedSpan != nil {
		s.spanContext.wrappedSpan.End()
	}

	endTime := time.Now()

	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()

	// Find the actual span in the trace and update it
	found := false
	for i := range s.spanContext.trace.Spans {
		if s.spanContext.trace.Spans[i].ID == s.spanContext.span.ID {
			s.spanContext.trace.Spans[i].EndTime = &endTime
			s.spanContext.trace.Spans[i].Duration = endTime.Sub(s.spanContext.trace.Spans[i].StartTime).Milliseconds()
			log.Printf("[UITraceCollector] Updated span %s with duration %dms", s.spanContext.span.ID, s.spanContext.trace.Spans[i].Duration)
			found = true
			break
		}
	}
	if !found {
		log.Printf("[UITraceCollector] WARNING: Span %s not found in trace %s", s.spanContext.span.ID, s.spanContext.trace.ID)
	}

	// Remove from active spans
	delete(s.collector.activeSpans, s.spanContext.span.ID)
	log.Printf("[UITraceCollector] Removed span %s from active spans", s.spanContext.span.ID)

	// Update trace if all spans are complete
	isComplete := s.collector.isTraceComplete(s.spanContext.trace)
	log.Printf("[UITraceCollector] Trace %s complete: %v", s.spanContext.trace.ID, isComplete)
	if isComplete {
		s.spanContext.trace.Status = "completed"
		traceEndTime := s.collector.getTraceEndTime(s.spanContext.trace)
		s.spanContext.trace.EndTime = &traceEndTime
		s.spanContext.trace.Duration = traceEndTime.Sub(s.spanContext.trace.StartTime).Milliseconds()
		log.Printf("[UITraceCollector] Trace %s completed with duration %dms", s.spanContext.trace.ID, s.spanContext.trace.Duration)
	}

	// Update trace size
	s.collector.updateTraceSize(s.spanContext.trace)
	log.Printf("[UITraceCollector] Trace %s size updated, total traces: %d", s.spanContext.trace.ID, len(s.collector.traces))
}

// AddEvent adds an event to the span
func (s *uiCollectorSpan) AddEvent(name string, attributes map[string]interface{}) {
	if s.spanContext.wrappedSpan != nil {
		s.spanContext.wrappedSpan.AddEvent(name, attributes)
	}

	event := UITraceEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attributes,
	}

	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()

	// Find the actual span in the trace and update it
	for i := range s.spanContext.trace.Spans {
		if s.spanContext.trace.Spans[i].ID == s.spanContext.span.ID {
			s.spanContext.trace.Spans[i].Events = append(s.spanContext.trace.Spans[i].Events, event)
			break
		}
	}
	s.collector.updateTraceSize(s.spanContext.trace)
}

// SetAttribute sets an attribute on the span
func (s *uiCollectorSpan) SetAttribute(key string, value interface{}) {
	if s.spanContext.wrappedSpan != nil {
		s.spanContext.wrappedSpan.SetAttribute(key, value)
	}

	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()

	// Find the actual span in the trace and update it
	for i := range s.spanContext.trace.Spans {
		if s.spanContext.trace.Spans[i].ID == s.spanContext.span.ID {
			s.spanContext.trace.Spans[i].Attributes[key] = value

			// Special handling for certain attributes
			switch key {
			case "input", "prompt":
				if str, ok := value.(string); ok {
					s.spanContext.trace.Spans[i].Input = str
				}
			case "output", "response", "completion":
				if str, ok := value.(string); ok {
					s.spanContext.trace.Spans[i].Output = str
				}
			case "error":
				s.spanContext.trace.Status = "error"
			}
			break
		}
	}

	s.collector.updateTraceSize(s.spanContext.trace)
}

// RecordError records an error on the span
func (s *uiCollectorSpan) RecordError(err error) {
	if s.spanContext.wrappedSpan != nil {
		s.spanContext.wrappedSpan.RecordError(err)
	}

	if err == nil {
		return
	}

	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()

	// Find the actual span in the trace and update it
	for i := range s.spanContext.trace.Spans {
		if s.spanContext.trace.Spans[i].ID == s.spanContext.span.ID {
			s.spanContext.trace.Spans[i].Error = &UITraceError{
				Message:   err.Error(),
				Type:      fmt.Sprintf("%T", err),
				Timestamp: time.Now(),
			}
			break
		}
	}

	s.spanContext.trace.Status = "error"
	s.collector.updateTraceSize(s.spanContext.trace)
}

// GetTraces returns all traces (newest first)
func (c *UITraceCollector) GetTraces(limit, offset int) ([]UITrace, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Convert map to slice and sort by start time (newest first)
	traces := make([]UITrace, 0, len(c.traces))
	for _, trace := range c.traces {
		traces = append(traces, *trace)
	}

	sort.Slice(traces, func(i, j int) bool {
		return traces[i].StartTime.After(traces[j].StartTime)
	})

	total := len(traces)

	// Apply pagination
	if offset >= total {
		return []UITrace{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return traces[offset:end], total
}

// GetTrace returns a specific trace by ID
func (c *UITraceCollector) GetTrace(traceID string) (*UITrace, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	trace, exists := c.traces[traceID]
	if !exists {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	// Return a copy
	traceCopy := *trace
	return &traceCopy, nil
}

// DeleteTrace deletes a trace by ID
func (c *UITraceCollector) DeleteTrace(traceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	trace, exists := c.traces[traceID]
	if !exists {
		return fmt.Errorf("trace not found: %s", traceID)
	}

	c.currentSizeBytes -= trace.SizeBytes
	delete(c.traces, traceID)
	return nil
}

// GetStats returns trace statistics
func (c *UITraceCollector) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalDuration int64
	var errorCount int
	toolUsage := make(map[string]int)

	for _, trace := range c.traces {
		if trace.Duration > 0 {
			totalDuration += trace.Duration
		}
		if trace.Status == "error" {
			errorCount++
		}

		// Count tool usage
		for _, span := range trace.Spans {
			if span.Type == "tool_call" {
				toolName := span.Name
				if name, ok := span.Attributes["tool_name"].(string); ok {
					toolName = name
				}
				toolUsage[toolName]++
			} else if contains(span.Name, []string{"tool", "function", "call"}) {
				// Also count spans with tool-like names
				toolName := span.Name
				if name, ok := span.Attributes["tool_name"].(string); ok {
					toolName = name
				}
				toolUsage[toolName]++
			}
		}
	}

	avgDuration := int64(0)
	if len(c.traces) > 0 {
		avgDuration = totalDuration / int64(len(c.traces))
	}

	return map[string]interface{}{
		"total_traces":       len(c.traces),
		"running_traces":     c.countRunningTraces(),
		"error_count":        errorCount,
		"error_rate":         float64(errorCount) / float64(max(len(c.traces), 1)),
		"avg_duration_ms":    avgDuration,
		"buffer_size_bytes":  c.currentSizeBytes,
		"buffer_usage":       float64(c.currentSizeBytes) / float64(c.maxSizeBytes),
		"tool_usage":         toolUsage,
	}
}

// Helper methods

func (c *UITraceCollector) inferSpanType(name string) string {
	// Infer span type from name patterns
	if contains(name, []string{"generation", "llm", "completion", "chat"}) {
		return "generation"
	}
	if contains(name, []string{"tool", "function", "call"}) {
		return "tool_call"
	}
	if contains(name, []string{"event"}) {
		return "event"
	}
	return "span"
}

func (c *UITraceCollector) updateTraceSize(trace *UITrace) {
	// Calculate approximate size of trace in bytes
	data, _ := json.Marshal(trace)
	oldSize := trace.SizeBytes
	trace.SizeBytes = len(data)
	c.currentSizeBytes += (trace.SizeBytes - oldSize)
}

func (c *UITraceCollector) enforceRetentionLimits() {
	// Remove old traces beyond max age
	cutoffTime := time.Now().Add(-c.maxAge)
	for id, trace := range c.traces {
		if trace.StartTime.Before(cutoffTime) {
			c.currentSizeBytes -= trace.SizeBytes
			delete(c.traces, id)
		}
	}

	// Remove oldest traces if over retention count
	if len(c.traces) > c.config.RetentionCount {
		// Get sorted trace IDs by start time
		type traceTime struct {
			id        string
			startTime time.Time
		}

		traceTimes := make([]traceTime, 0, len(c.traces))
		for id, trace := range c.traces {
			traceTimes = append(traceTimes, traceTime{id: id, startTime: trace.StartTime})
		}

		sort.Slice(traceTimes, func(i, j int) bool {
			return traceTimes[i].startTime.Before(traceTimes[j].startTime)
		})

		// Remove oldest traces
		toRemove := len(c.traces) - c.config.RetentionCount
		for i := 0; i < toRemove; i++ {
			trace := c.traces[traceTimes[i].id]
			c.currentSizeBytes -= trace.SizeBytes
			delete(c.traces, traceTimes[i].id)
		}
	}

	// Remove oldest traces if over size limit
	for c.currentSizeBytes > c.maxSizeBytes && len(c.traces) > 0 {
		// Find oldest trace
		var oldestID string
		var oldestTime time.Time
		for id, trace := range c.traces {
			if oldestID == "" || trace.StartTime.Before(oldestTime) {
				oldestID = id
				oldestTime = trace.StartTime
			}
		}

		if oldestID != "" {
			trace := c.traces[oldestID]
			c.currentSizeBytes -= trace.SizeBytes
			delete(c.traces, oldestID)
		}
	}
}

func (c *UITraceCollector) isTraceComplete(trace *UITrace) bool {
	// Check if all spans in trace are complete
	for _, span := range trace.Spans {
		if span.EndTime == nil {
			// Check if span is still active
			if _, exists := c.activeSpans[span.ID]; exists {
				return false
			}
		}
	}
	return true
}

func (c *UITraceCollector) getTraceEndTime(trace *UITrace) time.Time {
	var latestEnd time.Time
	for _, span := range trace.Spans {
		if span.EndTime != nil && span.EndTime.After(latestEnd) {
			latestEnd = *span.EndTime
		}
	}
	if latestEnd.IsZero() {
		return time.Now()
	}
	return latestEnd
}

func (c *UITraceCollector) countRunningTraces() int {
	count := 0
	for _, trace := range c.traces {
		if trace.Status == "running" {
			count++
		}
	}
	return count
}

func (c *UITraceCollector) getParentSpanID(ctx context.Context) string {
	if spanID, ok := ctx.Value(spanIDKey{}).(string); ok {
		return spanID
	}
	return ""
}

func (c *UITraceCollector) getConversationID(ctx context.Context) string {
	if id, ok := memory.GetConversationID(ctx); ok {
		return id
	}
	return ""
}

func (c *UITraceCollector) getOrgID(ctx context.Context) string {
	if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
		return orgID
	}
	return ""
}

// Context key for span ID
type spanIDKey struct{}

// noOpSpan is a no-op implementation of interfaces.Span
type noOpSpan struct{}

func (s *noOpSpan) End()                                                  {}
func (s *noOpSpan) AddEvent(name string, attributes map[string]interface{}) {}
func (s *noOpSpan) SetAttribute(key string, value interface{})            {}
func (s *noOpSpan) RecordError(err error)                                 {}

// Utility functions
func contains(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) && s[:len(substr)] == substr {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}