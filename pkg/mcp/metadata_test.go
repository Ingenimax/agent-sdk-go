package mcp

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test server metadata discovery functionality
func TestMCPServerMetadataDiscovery(t *testing.T) {
	tests := []struct {
		name           string
		serverInfo     *interfaces.MCPServerInfo
		capabilities   *interfaces.MCPServerCapabilities
		expectError    bool
	}{
		{
			name: "full_metadata_discovery",
			serverInfo: &interfaces.MCPServerInfo{
				Name:    "test-server",
				Title:   "Test MCP Server",
				Version: "v1.0.0",
			},
			capabilities: &interfaces.MCPServerCapabilities{
				Tools: &interfaces.MCPToolCapabilities{
					ListChanged: true,
				},
				Resources: &interfaces.MCPResourceCapabilities{
					Subscribe:   true,
					ListChanged: true,
				},
			},
			expectError: false,
		},
		{
			name: "minimal_metadata",
			serverInfo: &interfaces.MCPServerInfo{
				Name: "minimal-server",
			},
			capabilities: nil,
			expectError:  false,
		},
		{
			name:         "no_metadata",
			serverInfo:   nil,
			capabilities: nil,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock server with the specified metadata
			mockServer := &mockMCPServer{}

			// Setup expectations for GetServerInfo
			if tt.serverInfo != nil {
				mockServer.On("GetServerInfo").Return(tt.serverInfo, nil)
			} else {
				mockServer.On("GetServerInfo").Return((*interfaces.MCPServerInfo)(nil), nil)
			}

			// Setup expectations for GetCapabilities
			if tt.capabilities != nil {
				mockServer.On("GetCapabilities").Return(tt.capabilities, nil)
			} else {
				mockServer.On("GetCapabilities").Return((*interfaces.MCPServerCapabilities)(nil), nil)
			}

			// Test GetServerInfo method
			info, err := mockServer.GetServerInfo()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.serverInfo != nil {
					require.NotNil(t, info)
					assert.Equal(t, tt.serverInfo.Name, info.Name)
					assert.Equal(t, tt.serverInfo.Title, info.Title)
					assert.Equal(t, tt.serverInfo.Version, info.Version)
				}
			}

			// Test GetCapabilities method
			caps, err := mockServer.GetCapabilities()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.capabilities != nil {
					require.NotNil(t, caps)
					if tt.capabilities.Tools != nil {
						assert.Equal(t, tt.capabilities.Tools.ListChanged, caps.Tools.ListChanged)
					}
					if tt.capabilities.Resources != nil {
						assert.Equal(t, tt.capabilities.Resources.Subscribe, caps.Resources.Subscribe)
						assert.Equal(t, tt.capabilities.Resources.ListChanged, caps.Resources.ListChanged)
					}
				}
			}

			mockServer.AssertExpectations(t)
		})
	}
}

// Test lazy MCP tool with server metadata
func TestLazyMCPToolWithMetadata(t *testing.T) {
	// Create a lazy MCP server config
	config := LazyMCPServerConfig{
		Name:    "test-server",
		Type:    "stdio",
		Command: "test-command",
	}

	// Create a lazy MCP tool
	tool := NewLazyMCPTool("test-tool", "Test tool description", nil, config)
	lazyTool, ok := tool.(*LazyMCPTool)
	require.True(t, ok)

	// Test initial state
	assert.Equal(t, "test-tool", lazyTool.Name())
	assert.Equal(t, "Test tool description", lazyTool.Description())

	// Test fallback description when tool description is empty
	emptyDescTool := NewLazyMCPTool("test-tool-2", "", nil, config)
	lazyEmptyTool, ok := emptyDescTool.(*LazyMCPTool)
	require.True(t, ok)

	// Initially should use generic description
	assert.Equal(t, "test-tool-2 (MCP tool)", lazyEmptyTool.Description())

	// Simulate server metadata being loaded
	lazyEmptyTool.serverInfo = &interfaces.MCPServerInfo{
		Name:  "test-server",
		Title: "Test Server",
	}

	// Now description should include server context
	assert.Equal(t, "test-tool-2 (from Test Server)", lazyEmptyTool.Description())
}

// Test server metadata cache functionality
func TestServerMetadataCache(t *testing.T) {
	config := LazyMCPServerConfig{
		Name:    "cache-test-server",
		Type:    "stdio",
		Command: "test-command",
	}

	// Test getting metadata from cache when none exists
	metadata := GetServerMetadataFromCache(config)
	assert.Nil(t, metadata)

	// Simulate storing metadata in cache
	serverKey := "stdio:cache-test-server:test-command"
	globalServerCache.mu.Lock()
	globalServerCache.serverMetadata[serverKey] = &interfaces.MCPServerInfo{
		Name:    "cache-test-server",
		Title:   "Cached Test Server",
		Version: "v2.0.0",
	}
	globalServerCache.mu.Unlock()

	// Test getting metadata from cache
	metadata = GetServerMetadataFromCache(config)
	require.NotNil(t, metadata)
	assert.Equal(t, "cache-test-server", metadata.Name)
	assert.Equal(t, "Cached Test Server", metadata.Title)
	assert.Equal(t, "v2.0.0", metadata.Version)

	// Clean up
	globalServerCache.mu.Lock()
	delete(globalServerCache.serverMetadata, serverKey)
	globalServerCache.mu.Unlock()
}