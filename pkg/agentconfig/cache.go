package agentconfig

import (
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

// Simple in-memory cache for agent configurations
var (
	configCache      = make(map[string]*cacheEntry)
	configCacheMutex sync.RWMutex
)

type cacheEntry struct {
	config    *agent.AgentConfig
	expiresAt time.Time
}

// getFromCache retrieves a config from cache if it exists and isn't expired
func getFromCache(key string) *agent.AgentConfig {
	configCacheMutex.RLock()
	defer configCacheMutex.RUnlock()

	entry, exists := configCache[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil
	}

	// Return a deep copy to prevent modification and shared state
	return deepCopyAgentConfig(entry.config)
}

// cacheConfig stores a config in the cache
func cacheConfig(key string, config *agent.AgentConfig, ttl time.Duration) {
	configCacheMutex.Lock()
	defer configCacheMutex.Unlock()

	// Store a deep copy to prevent external modifications from affecting cache
	configCache[key] = &cacheEntry{
		config:    deepCopyAgentConfig(config),
		expiresAt: time.Now().Add(ttl),
	}
}

// ClearCache clears all cached configurations
func ClearCache() {
	configCacheMutex.Lock()
	defer configCacheMutex.Unlock()

	configCache = make(map[string]*cacheEntry)
}

// ClearCacheEntry removes a specific cache entry
func ClearCacheEntry(key string) {
	configCacheMutex.Lock()
	defer configCacheMutex.Unlock()

	delete(configCache, key)
}

// GetCacheStats returns cache statistics for monitoring
func GetCacheStats() map[string]int {
	configCacheMutex.RLock()
	defer configCacheMutex.RUnlock()

	totalEntries := len(configCache)
	expiredEntries := 0
	validEntries := 0

	now := time.Now()
	for _, entry := range configCache {
		if now.After(entry.expiresAt) {
			expiredEntries++
		} else {
			validEntries++
		}
	}

	return map[string]int{
		"total":   totalEntries,
		"valid":   validEntries,
		"expired": expiredEntries,
	}
}

// CleanupExpiredEntries removes expired cache entries
func CleanupExpiredEntries() {
	configCacheMutex.Lock()
	defer configCacheMutex.Unlock()

	now := time.Now()
	for key, entry := range configCache {
		if now.After(entry.expiresAt) {
			delete(configCache, key)
		}
	}
}