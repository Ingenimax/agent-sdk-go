package sandbox

import (
	"fmt"
	"path"
	"strings"
)

// Allowlist decides which commands may start. Denied commands win over
// allowed ones, and an empty allow list allows nothing.
//
// Allowed entries without a slash match a bare command name exactly, which
// the container then looks up on its PATH. Allowed entries that are absolute
// paths match exactly that path. A command given as a path never matches a
// bare-name entry, so allowing "node" does not allow "/tmp/node" or "./node".
//
// Denied entries match by base name, so denying "sh" refuses "sh", "/bin/sh"
// and "/usr/bin/sh".
type Allowlist struct {
	names  map[string]bool
	paths  map[string]bool
	denied map[string]bool
}

// NewAllowlist returns an Allowlist for the given allowed and denied commands.
func NewAllowlist(allowed, denied []string) *Allowlist {
	a := &Allowlist{
		names:  make(map[string]bool, len(allowed)),
		paths:  make(map[string]bool, len(allowed)),
		denied: make(map[string]bool, len(denied)),
	}
	for _, cmd := range allowed {
		if strings.Contains(cmd, "/") {
			a.paths[path.Clean(cmd)] = true
		} else {
			a.names[cmd] = true
		}
	}
	for _, cmd := range denied {
		a.denied[path.Base(cmd)] = true
	}
	return a
}

// Check returns nil if command may start, and an error wrapping
// ErrCommandDenied if not.
func (a *Allowlist) Check(command string) error {
	if command == "" {
		return fmt.Errorf("%w: empty command", ErrCommandDenied)
	}
	// The container is Linux, so commands use forward slashes whatever the
	// host is.
	if a.denied[path.Base(command)] {
		return fmt.Errorf("%w: %q is denied", ErrCommandDenied, command)
	}
	if len(a.names) == 0 && len(a.paths) == 0 {
		return fmt.Errorf("%w: no commands are allowed", ErrCommandDenied)
	}
	if strings.Contains(command, "/") {
		if path.IsAbs(command) && a.paths[path.Clean(command)] {
			return nil
		}
		return fmt.Errorf("%w: %q is not in the allowlist (a path must match an allowed absolute path exactly)", ErrCommandDenied, command)
	}
	if a.names[command] {
		return nil
	}
	return fmt.Errorf("%w: %q is not in the allowlist", ErrCommandDenied, command)
}
