package tracing

import (
	"crypto/sha256"
	"fmt"
)

// hashString creates a SHA256 hash of the input string for privacy-safe tracing
func hashString(s string) string {
	if s == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", hash)
}
