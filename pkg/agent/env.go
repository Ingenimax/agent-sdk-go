package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// envVarCache stores environment variables loaded from .env files
var envVarCache = make(map[string]string)

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		// If file doesn't exist, it's not an error - just skip
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		// Store in cache (don't override actual environment variables)
		if _, exists := os.LookupEnv(key); !exists {
			envVarCache[key] = value
		}
	}

	return scanner.Err()
}

// ExpandEnv expands environment variables, checking both actual env vars and .env file
// This replaces os.ExpandEnv to support .env files
func ExpandEnv(s string) string {
	// First try to load .env file from current directory if not already loaded
	if len(envVarCache) == 0 {
		// Try multiple common locations for .env file
		envPaths := []string{
			".env",
			filepath.Join(".", ".env"),
		}

		// Also check if there's a working directory we should check
		if wd, err := os.Getwd(); err == nil {
			envPaths = append(envPaths, filepath.Join(wd, ".env"))
		}

		for _, path := range envPaths {
			_ = loadEnvFile(path) // Ignore errors, just try to load
		}
	}

	// Custom expansion function that checks both sources
	return os.Expand(s, func(key string) string {
		// First check actual environment variables
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		// Then check .env cache
		if value, exists := envVarCache[key]; exists {
			return value
		}
		// Return empty string if not found
		return ""
	})
}

// LoadEnvFile explicitly loads a .env file into the cache
// This can be called by applications to ensure a specific .env file is loaded
func LoadEnvFile(path string) error {
	return loadEnvFile(path)
}

// GetEnvValue gets an environment variable value from either env or .env cache
func GetEnvValue(key string) string {
	// First check actual environment variables
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	// Then check .env cache
	if value, exists := envVarCache[key]; exists {
		return value
	}
	return ""
}