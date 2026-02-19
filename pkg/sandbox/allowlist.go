package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Allowlist enforces which commands are permitted in the sandbox.
// Deny list takes precedence over allow list. Empty allow list denies all (fail-closed).
type Allowlist struct {
	allowed map[string]bool
	denied  map[string]bool
}

// NewAllowlist creates a new Allowlist from allow and deny lists.
func NewAllowlist(allowed, denied []string) *Allowlist {
	a := &Allowlist{
		allowed: make(map[string]bool, len(allowed)),
		denied:  make(map[string]bool, len(denied)),
	}
	for _, cmd := range allowed {
		a.allowed[strings.ToLower(cmd)] = true
	}
	for _, cmd := range denied {
		a.denied[strings.ToLower(cmd)] = true
	}
	return a
}

// Check returns nil if the command is permitted, ErrCommandDenied otherwise.
func (a *Allowlist) Check(command string) error {
	base := strings.ToLower(filepath.Base(command))

	if a.denied[base] {
		return fmt.Errorf("%w: %q is explicitly denied", ErrCommandDenied, base)
	}

	if len(a.allowed) == 0 {
		return fmt.Errorf("%w: no commands are allowed (empty allowlist)", ErrCommandDenied)
	}

	if !a.allowed[base] {
		return fmt.Errorf("%w: %q is not in the allowlist", ErrCommandDenied, base)
	}

	return nil
}
