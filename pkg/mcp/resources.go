package mcp

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// ResourceManager provides high-level operations for MCP resources
type ResourceManager struct {
	servers []interfaces.MCPServer
	logger  logging.Logger
}

// NewResourceManager creates a new resource manager
func NewResourceManager(servers []interfaces.MCPServer) *ResourceManager {
	return &ResourceManager{
		servers: servers,
		logger:  logging.New(),
	}
}

// ListAllResources lists resources from all MCP servers
func (rm *ResourceManager) ListAllResources(ctx context.Context) (map[string][]interfaces.MCPResource, error) {
	result := make(map[string][]interfaces.MCPResource)

	for i, server := range rm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		resources, err := server.ListResources(ctx)
		if err != nil {
			rm.logger.Warn(ctx, "Failed to list resources from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		result[serverName] = resources
		rm.logger.Debug(ctx, "Listed resources from server", map[string]interface{}{
			"server":         serverName,
			"resource_count": len(resources),
		})
	}

	return result, nil
}

// FindResources searches for resources by pattern across all servers
func (rm *ResourceManager) FindResources(ctx context.Context, pattern string) ([]ResourceMatch, error) {
	var matches []ResourceMatch

	for i, server := range rm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		resources, err := server.ListResources(ctx)
		if err != nil {
			rm.logger.Warn(ctx, "Failed to list resources from server", map[string]interface{}{
				"server": serverName,
				"error":  err.Error(),
			})
			continue
		}

		for _, resource := range resources {
			if rm.matchesPattern(resource, pattern) {
				matches = append(matches, ResourceMatch{
					Server:   server,
					Resource: resource,
				})
			}
		}
	}

	rm.logger.Debug(ctx, "Found matching resources", map[string]interface{}{
		"pattern":     pattern,
		"match_count": len(matches),
	})

	return matches, nil
}

// GetResourceContent retrieves content for a resource, trying all servers if URI is ambiguous
func (rm *ResourceManager) GetResourceContent(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error) {
	for i, server := range rm.servers {
		serverName := fmt.Sprintf("server-%d", i)

		content, err := server.GetResource(ctx, uri)
		if err != nil {
			rm.logger.Debug(ctx, "Server doesn't have resource", map[string]interface{}{
				"server": serverName,
				"uri":    uri,
			})
			continue
		}

		rm.logger.Debug(ctx, "Successfully retrieved resource", map[string]interface{}{
			"server": serverName,
			"uri":    uri,
		})
		return content, nil
	}

	return nil, fmt.Errorf("resource not found on any server: %s", uri)
}

// WatchResources watches multiple resources for changes
func (rm *ResourceManager) WatchResources(ctx context.Context, uris []string) (<-chan ResourceUpdate, error) {
	updates := make(chan ResourceUpdate, 100)

	// Start watching each resource
	for _, uri := range uris {
		for i, server := range rm.servers {
			serverName := fmt.Sprintf("server-%d", i)

			serverUpdates, err := server.WatchResource(ctx, uri)
			if err != nil {
				rm.logger.Warn(ctx, "Failed to watch resource", map[string]interface{}{
					"server": serverName,
					"uri":    uri,
					"error":  err.Error(),
				})
				continue
			}

			// Forward server updates to our combined channel
			go func(serverName, uri string, serverUpdates <-chan interfaces.MCPResourceUpdate) {
				for update := range serverUpdates {
					updates <- ResourceUpdate{
						Server: serverName,
						Update: update,
					}
				}
			}(serverName, uri, serverUpdates)

			break // Found a server that can watch this resource
		}
	}

	// Close updates channel when context is done
	go func() {
		<-ctx.Done()
		close(updates)
	}()

	return updates, nil
}

// GetResourcesByType returns resources filtered by MIME type
func (rm *ResourceManager) GetResourcesByType(ctx context.Context, mimeType string) ([]ResourceMatch, error) {
	var matches []ResourceMatch

	for _, server := range rm.servers {
		resources, err := server.ListResources(ctx)
		if err != nil {
			continue
		}

		for _, resource := range resources {
			if rm.matchesMimeType(resource, mimeType) {
				matches = append(matches, ResourceMatch{
					Server:   server,
					Resource: resource,
				})
			}
		}
	}

	return matches, nil
}

// Helper types

// ResourceMatch represents a resource found on a specific server
type ResourceMatch struct {
	Server   interfaces.MCPServer
	Resource interfaces.MCPResource
}

// ResourceUpdate represents an update from any server
type ResourceUpdate struct {
	Server string
	Update interfaces.MCPResourceUpdate
}

// Helper methods

func (rm *ResourceManager) matchesPattern(resource interfaces.MCPResource, pattern string) bool {
	pattern = strings.ToLower(pattern)

	// Check URI, name, and description
	if strings.Contains(strings.ToLower(resource.URI), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(resource.Name), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(resource.Description), pattern) {
		return true
	}

	// Check file extension for file-based resources
	if strings.HasPrefix(resource.URI, "file://") {
		ext := filepath.Ext(resource.URI)
		if strings.Contains(strings.ToLower(ext), pattern) {
			return true
		}
	}

	return false
}

func (rm *ResourceManager) matchesMimeType(resource interfaces.MCPResource, targetType string) bool {
	if resource.MimeType == "" {
		return false
	}

	// Exact match
	if resource.MimeType == targetType {
		return true
	}

	// Parse MIME types for partial matching
	resourceType, _, err := mime.ParseMediaType(resource.MimeType)
	if err != nil {
		return false
	}

	targetTypeMain, _, err := mime.ParseMediaType(targetType)
	if err != nil {
		return false
	}

	// Match main type (e.g., "image/*" matches "image/png")
	if strings.HasSuffix(targetType, "/*") {
		targetMain := strings.TrimSuffix(targetType, "/*")
		resourceMain := strings.Split(resourceType, "/")[0]
		return targetMain == resourceMain
	}

	return resourceType == targetTypeMain
}

// Utility functions for common resource operations

// IsTextResource checks if a resource contains text content
func IsTextResource(resource interfaces.MCPResource) bool {
	if resource.MimeType == "" {
		return true // Assume text if no MIME type
	}

	return strings.HasPrefix(resource.MimeType, "text/") ||
		resource.MimeType == "application/json" ||
		resource.MimeType == "application/xml" ||
		strings.HasSuffix(resource.MimeType, "+json") ||
		strings.HasSuffix(resource.MimeType, "+xml")
}

// IsBinaryResource checks if a resource contains binary content
func IsBinaryResource(resource interfaces.MCPResource) bool {
	return !IsTextResource(resource)
}

// GetResourceExtension extracts file extension from resource URI
func GetResourceExtension(resource interfaces.MCPResource) string {
	return strings.ToLower(filepath.Ext(resource.URI))
}
