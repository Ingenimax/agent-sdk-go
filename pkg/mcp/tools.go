package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// MCPToolResponse represents the response from an MCP tool call
type MCPToolResponse struct {
	Content interface{} `json:"content"`
	Error   string      `json:"error,omitempty"`
	Text    string      `json:"text,omitempty"`
}

// RunStdioToolCommand executes a command for an MCP stdio server
func RunStdioToolCommand(cmd *exec.Cmd, payload []byte) ([]byte, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Write payload to stdin
	_, err = stdin.Write(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to write to stdin: %w", err)
	}

	// Close stdin and handle any error
	if err := stdin.Close(); err != nil {
		return nil, fmt.Errorf("failed to close stdin: %w", err)
	}

	// Read output from stdout
	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read from stdout: %w", err)
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return output, nil
}

// FetchMCPToolsFromServer fetches the list of tools from an MCP server
func FetchMCPToolsFromServer(ctx context.Context, url string) ([]map[string]interface{}, error) {
	// Create an HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/tools", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers - use application/json for tool listing
	// MCP uses different content types for different operations
	req.Header.Set("Accept", "application/json")

	// Create a client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check the status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-OK status: %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content type: %s (expected application/json)", contentType)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response
	var tools []map[string]interface{}
	if err := json.Unmarshal(body, &tools); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return tools, nil
}
