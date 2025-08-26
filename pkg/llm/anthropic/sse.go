package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// AnthropicSSEEvent represents the structure of Anthropic's SSE events
type AnthropicSSEEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// MessageStart event data
type MessageStartData struct {
	Type    string  `json:"type"`
	ID      string  `json:"id"`
	Role    string  `json:"role"`
	Content []any   `json:"content"`
	Model   string  `json:"model"`
	Usage   Usage   `json:"usage"`
}

// ContentBlockStart event data
type ContentBlockStartData struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text,omitempty"`
		ID    string                 `json:"id,omitempty"`    // For tool_use blocks
		Name  string                 `json:"name,omitempty"`  // For tool_use blocks
		Input map[string]interface{} `json:"input,omitempty"` // For tool_use blocks
	} `json:"content_block"`
}

// ContentBlockDelta event data
type ContentBlockDeltaData struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`    // Thinking content field
		PartialJSON string `json:"partial_json,omitempty"` // For input_json_delta events
	} `json:"delta"`
}

// ContentBlockStop event data
type ContentBlockStopData struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// MessageDelta event data
type MessageDeltaData struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason   string `json:"stop_reason,omitempty"`
		StopSequence string `json:"stop_sequence,omitempty"`
	} `json:"delta"`
	Usage Usage `json:"usage"`
}

// MessageStop event data
type MessageStopData struct {
	Type string `json:"type"`
}

// parseSSELine parses a single SSE line from Anthropic's API
func parseSSELine(line string) (*AnthropicSSEEvent, error) {
	line = strings.TrimSpace(line)
	
	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, ":") {
		return nil, nil
	}
	
	// Parse data lines
	if strings.HasPrefix(line, "data: ") {
		data := strings.TrimPrefix(line, "data: ")
		
		// Handle end of stream or empty data
		if data == "[DONE]" || data == "" || strings.TrimSpace(data) == "" {
			return &AnthropicSSEEvent{Type: "done"}, nil
		}
		
		// Parse JSON data
		var event AnthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("failed to parse SSE event: %w (data: %q)", err, data)
		}
		
		return &event, nil
	}
	
	// Parse event lines
	if strings.HasPrefix(line, "event: ") {
		eventType := strings.TrimPrefix(line, "event: ")
		return &AnthropicSSEEvent{Type: eventType}, nil
	}
	
	// Skip other SSE fields (id, retry, etc.)
	return nil, nil
}

// convertAnthropicEventToStreamEvent converts an Anthropic SSE event to our internal StreamEvent
func (c *AnthropicClient) convertAnthropicEventToStreamEvent(event *AnthropicSSEEvent, thinkingBlocks map[int]bool, toolBlocks map[int]struct {
	ID        string
	Name      string
	InputJSON strings.Builder
}) (*interfaces.StreamEvent, error) {
	if event == nil {
		return nil, nil
	}
	
	
	streamEvent := &interfaces.StreamEvent{
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
	
	switch event.Type {
	case "message_start":
		var msgStart MessageStartData
		if err := json.Unmarshal(event.Data, &msgStart); err != nil {
			return nil, fmt.Errorf("failed to parse message_start: %w", err)
		}
		
		streamEvent.Type = interfaces.StreamEventMessageStart
		streamEvent.Metadata["message_id"] = msgStart.ID
		streamEvent.Metadata["model"] = msgStart.Model
		streamEvent.Metadata["role"] = msgStart.Role
		streamEvent.Metadata["usage"] = msgStart.Usage
		
	case "content_block_start":
		var blockStart ContentBlockStartData
		if err := json.Unmarshal(event.Data, &blockStart); err != nil {
			return nil, fmt.Errorf("failed to parse content_block_start: %w", err)
		}
		
		
		// Handle different block types
		switch blockStart.ContentBlock.Type {
		case "thinking":
			streamEvent.Type = interfaces.StreamEventThinking
			thinkingBlocks[blockStart.Index] = true
			streamEvent.Content = blockStart.ContentBlock.Text
			
		case "tool_use":
			// Don't send tool call event immediately - track tool info and wait for complete input
			thinkingBlocks[blockStart.Index] = false // Not a thinking block
			
			// Store tool info to accumulate input arguments later
			info := struct {
				ID        string
				Name      string
				InputJSON strings.Builder
			}{
				ID:   blockStart.ContentBlock.ID,
				Name: blockStart.ContentBlock.Name,
			}
			
			// If there's initial input (rare but possible), add it
			if blockStart.ContentBlock.Input != nil && len(blockStart.ContentBlock.Input) > 0 {
				argsBytes, _ := json.Marshal(blockStart.ContentBlock.Input)
				info.InputJSON.WriteString(string(argsBytes))
			}
			
			toolBlocks[blockStart.Index] = info
			
			// Return nil to skip sending event now - will send when complete
			return nil, nil
			
		default: // "text" or other types
			streamEvent.Type = interfaces.StreamEventContentDelta
			thinkingBlocks[blockStart.Index] = false
			streamEvent.Content = blockStart.ContentBlock.Text
		}
		
		streamEvent.Metadata["block_index"] = blockStart.Index
		streamEvent.Metadata["block_type"] = blockStart.ContentBlock.Type
		
	case "content_block_delta":
		var blockDelta ContentBlockDeltaData
		if err := json.Unmarshal(event.Data, &blockDelta); err != nil {
			return nil, fmt.Errorf("failed to parse content_block_delta: %w", err)
		}
		
		// Check if this is an input_json_delta (tool argument streaming)
		if blockDelta.Delta.Type == "input_json_delta" {
			// Accumulate the partial JSON into the tool block
			if info, exists := toolBlocks[blockDelta.Index]; exists {
				info.InputJSON.WriteString(blockDelta.Delta.PartialJSON)
				toolBlocks[blockDelta.Index] = info
			}
			
			// Return nil to skip sending event now - will send complete tool call later
			return nil, nil
		}
		
		// Check if this block is a thinking block using our tracking
		if thinkingBlocks[blockDelta.Index] {
			streamEvent.Type = interfaces.StreamEventThinking
			// For thinking blocks, use the thinking field instead of text field
			streamEvent.Content = blockDelta.Delta.Thinking
		} else {
			streamEvent.Type = interfaces.StreamEventContentDelta
			streamEvent.Content = blockDelta.Delta.Text
		}
		streamEvent.Metadata["block_index"] = blockDelta.Index
		streamEvent.Metadata["delta_type"] = blockDelta.Delta.Type
		
	case "content_block_stop":
		var blockStop ContentBlockStopData
		if err := json.Unmarshal(event.Data, &blockStop); err != nil {
			return nil, fmt.Errorf("failed to parse content_block_stop: %w", err)
		}
		
		// Check if this was a tool block that we need to complete
		if info, exists := toolBlocks[blockStop.Index]; exists {
			// Create tool call event with complete arguments
			streamEvent.Type = interfaces.StreamEventToolUse
			streamEvent.ToolCall = &interfaces.ToolCall{
				ID:        info.ID,
				Name:      info.Name,
				Arguments: info.InputJSON.String(),
			}
			streamEvent.Metadata["block_index"] = blockStop.Index
			
			// Remove from tracking map
			delete(toolBlocks, blockStop.Index)
		} else {
			// Regular content block stop
			streamEvent.Type = interfaces.StreamEventContentComplete
			streamEvent.Metadata["block_index"] = blockStop.Index
		}
		
	case "message_delta":
		var msgDelta MessageDeltaData
		if err := json.Unmarshal(event.Data, &msgDelta); err != nil {
			return nil, fmt.Errorf("failed to parse message_delta: %w", err)
		}
		
		streamEvent.Type = interfaces.StreamEventContentDelta
		streamEvent.Metadata["stop_reason"] = msgDelta.Delta.StopReason
		streamEvent.Metadata["stop_sequence"] = msgDelta.Delta.StopSequence
		streamEvent.Metadata["usage"] = msgDelta.Usage
		
	case "message_stop":
		streamEvent.Type = interfaces.StreamEventMessageStop
		
	case "ping":
		// Ignore ping events
		return nil, nil
		
	case "error":
		// Parse error event
		var errorData map[string]interface{}
		if err := json.Unmarshal(event.Data, &errorData); err != nil {
			return nil, fmt.Errorf("failed to parse error event: %w", err)
		}
		
		streamEvent.Type = interfaces.StreamEventError
		streamEvent.Error = fmt.Errorf("anthropic api error: %v", errorData)
		streamEvent.Metadata["error_data"] = errorData
		
	case "input_json_delta":
		// Handle streaming tool input (arguments) - this case may not be used since 
		// input_json_delta events come as content_block_delta with delta.type="input_json_delta"
		var inputDelta struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			PartialJSON  string `json:"partial_json"`
		}
		if err := json.Unmarshal(event.Data, &inputDelta); err != nil {
			return nil, fmt.Errorf("failed to parse input_json_delta: %w", err)
		}
		
		// Accumulate the partial JSON into the tool block
		if info, exists := toolBlocks[inputDelta.Index]; exists {
			info.InputJSON.WriteString(inputDelta.PartialJSON)
			toolBlocks[inputDelta.Index] = info
		}
		
		// Return nil to skip sending event now - will send complete tool call later
		return nil, nil
		
	case "done":
		// End of stream
		streamEvent.Type = interfaces.StreamEventMessageStop
		
	default:
		// Unknown event type - log and ignore
		streamEvent.Type = interfaces.StreamEventContentDelta
		streamEvent.Metadata["unknown_event_type"] = event.Type
		streamEvent.Metadata["raw_data"] = string(event.Data)
	}
	
	return streamEvent, nil
}

// parseSSEStream parses an SSE stream and returns a channel of StreamEvents
func (c *AnthropicClient) parseSSEStream(scanner *bufio.Scanner, eventChan chan<- interfaces.StreamEvent) {
	defer close(eventChan)
	
	var currentEvent *AnthropicSSEEvent
	// Track which block indices are thinking blocks
	thinkingBlocks := make(map[int]bool)
	// Track tool blocks and accumulate their input arguments
	toolBlocks := make(map[int]struct {
		ID        string
		Name      string
		InputJSON strings.Builder
	})
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Empty line indicates end of current event
		if line == "" {
			if currentEvent != nil && len(currentEvent.Data) > 0 {
				// Process complete event
				if err := c.processCompleteSSEEvent(currentEvent, eventChan, thinkingBlocks, toolBlocks); err != nil {
					eventChan <- interfaces.StreamEvent{
						Type:      interfaces.StreamEventError,
						Error:     fmt.Errorf("failed to process SSE event: %w", err),
						Timestamp: time.Now(),
					}
					return
				}
				currentEvent = nil
			}
			continue
		}
		
		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}
		
		// Parse event type
		if strings.HasPrefix(line, "event: ") {
			if currentEvent == nil {
				currentEvent = &AnthropicSSEEvent{}
			}
			currentEvent.Type = strings.TrimPrefix(line, "event: ")
			continue
		}
		
		// Parse data
		if strings.HasPrefix(line, "data: ") {
			dataContent := strings.TrimPrefix(line, "data: ")
			
			// Handle end of stream
			if dataContent == "[DONE]" {
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventMessageStop,
					Timestamp: time.Now(),
				}
				return
			}
			
			// Skip empty data
			if strings.TrimSpace(dataContent) == "" {
				continue
			}
			
			if currentEvent == nil {
				currentEvent = &AnthropicSSEEvent{}
			}
			currentEvent.Data = json.RawMessage(dataContent)
			continue
		}
		
		// Parse other SSE fields (id, retry, etc.) - can be ignored for now
	}
	
	// Process any remaining event
	if currentEvent != nil && len(currentEvent.Data) > 0 {
		_ = c.processCompleteSSEEvent(currentEvent, eventChan, thinkingBlocks, toolBlocks)
	}
	
	// Check for scanner error
	if err := scanner.Err(); err != nil {
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventError,
			Error:     fmt.Errorf("scanner error: %w", err),
			Timestamp: time.Now(),
		}
	}
}

func (c *AnthropicClient) processCompleteSSEEvent(event *AnthropicSSEEvent, eventChan chan<- interfaces.StreamEvent, thinkingBlocks map[int]bool, toolBlocks map[int]struct {
	ID        string
	Name      string
	InputJSON strings.Builder
}) error {
	// Handle done event
	if event.Type == "done" || event.Type == "" {
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}
		return nil
	}
	
	// Convert to StreamEvent
	streamEvent, err := c.convertAnthropicEventToStreamEvent(event, thinkingBlocks, toolBlocks)
	if err != nil {
		return fmt.Errorf("failed to convert event: %w", err)
	}
	
	// Skip nil events (like ping)
	if streamEvent != nil {
		eventChan <- *streamEvent
	}
	
	return nil
}