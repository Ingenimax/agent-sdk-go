package gemini

import (
	"context"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// collectEvents drains every event from eventCh until it is closed.
func collectEvents(eventCh <-chan interfaces.StreamEvent) []interfaces.StreamEvent {
	var events []interfaces.StreamEvent
	for event := range eventCh {
		events = append(events, event)
	}
	return events
}

// TestStreamResponse_ChunksAndReassembles verifies that streamResponse emits
// content-delta events that, when concatenated, reproduce the input exactly
// regardless of whether the length is a multiple of the chunk size.
func TestStreamResponse_ChunksAndReassembles(t *testing.T) {
	client := &GeminiClient{}

	tests := []struct {
		name     string
		response string
	}{
		{name: "empty", response: ""},
		{name: "shorter than chunk", response: "hello"},
		{name: "exactly one chunk", response: strings.Repeat("a", 50)},
		{name: "spans multiple chunks with remainder", response: strings.Repeat("b", 123)},
		{name: "multibyte content", response: strings.Repeat("héllo wörld ", 20)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventCh := make(chan interfaces.StreamEvent, 16)
			go func() {
				defer close(eventCh)
				client.streamResponse(context.Background(), tt.response, eventCh)
			}()

			events := collectEvents(eventCh)

			// Every emitted event must be a content delta, and concatenating
			// their content must reproduce the input exactly.
			var b strings.Builder
			for i, event := range events {
				if event.Type != interfaces.StreamEventContentDelta {
					t.Fatalf("event %d: type = %v, want StreamEventContentDelta", i, event.Type)
				}
				if event.Content == "" {
					t.Fatalf("event %d: emitted an empty content delta", i)
				}
				if event.Timestamp.IsZero() {
					t.Fatalf("event %d: missing timestamp", i)
				}
				b.WriteString(event.Content)
			}
			if got := b.String(); got != tt.response {
				t.Fatalf("reassembled content = %q, want %q", got, tt.response)
			}

			// Verify the exact number of chunks: ceil(len/chunkSize).
			const chunkSize = 50
			wantChunks := (len(tt.response) + chunkSize - 1) / chunkSize
			if len(events) != wantChunks {
				t.Fatalf("chunk count = %d, want %d", len(events), wantChunks)
			}
			// No chunk may exceed the chunk size.
			for i, event := range events {
				if len(event.Content) > chunkSize {
					t.Fatalf("event %d: chunk length %d exceeds chunkSize %d", i, len(event.Content), chunkSize)
				}
			}
		})
	}
}

// TestStreamResponse_StopsEarlyOnContextCancel verifies that streamResponse
// stops emitting after the context is cancelled mid-stream, rather than
// draining every chunk. The response is 200 bytes (4 chunks of 50); we read
// one chunk, cancel, and assert it does not deliver the remaining chunks.
func TestStreamResponse_StopsEarlyOnContextCancel(t *testing.T) {
	client := &GeminiClient{}

	const chunkSize = 50
	response := strings.Repeat("x", 4*chunkSize) // 4 chunks total

	// Unbuffered channel so each send blocks until we read it, giving us
	// precise control over when cancellation takes effect.
	eventCh := make(chan interfaces.StreamEvent)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.streamResponse(ctx, response, eventCh)
	}()

	// Read exactly one chunk, then cancel before reading the next.
	received := 0
	first, ok := <-eventCh
	if !ok {
		t.Fatal("channel closed before any chunk was sent")
	}
	if first.Type != interfaces.StreamEventContentDelta {
		t.Fatalf("first event type = %v, want StreamEventContentDelta", first.Type)
	}
	received++
	cancel()

	// Drain whatever is left. Because the send is select'd against ctx.Done(),
	// streamResponse must return without sending all 4 chunks. Note streamResponse
	// does not close eventCh itself, so we rely on `done` to know it returned.
	drain := make(chan int, 1)
	go func() {
		extra := 0
		for range eventCh {
			extra++
		}
		drain <- extra
	}()

	<-done         // streamResponse returned (no deadlock)
	close(eventCh) // safe: producer has returned, only the drain goroutine reads
	received += <-drain

	if received >= 4 {
		t.Fatalf("received %d chunks; expected early stop with fewer than 4", received)
	}
}
