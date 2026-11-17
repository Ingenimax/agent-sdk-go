package sandbox

import (
	"errors"
	"testing"
)

func TestAllowlistCheck(t *testing.T) {
	al := NewAllowlist(
		[]string{"git", "npx", "/usr/local/bin/node", "rm"},
		[]string{"rm", "/bin/sh"},
	)

	tests := []struct {
		command string
		allowed bool
	}{
		{"git", true},
		{"npx", true},
		{"/usr/local/bin/node", true},
		{"/usr/local/bin/../bin/node", true}, // cleaned before matching
		// A bare-name entry does not allow paths that end in that name.
		{"/usr/bin/git", false},
		{"./git", false},
		{"bin/git", false},
		// A path entry does not allow the bare name or another path.
		{"node", false},
		{"/tmp/node", false},
		// Matching is case-sensitive, as Linux file names are.
		{"Git", false},
		// Denied wins over allowed, and matches by base name.
		{"rm", false},
		{"/usr/bin/rm", false},
		{"sh", false},
		{"/usr/bin/sh", false},
		{"curl", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			err := al.Check(tt.command)
			if tt.allowed && err != nil {
				t.Fatalf("Check(%q) = %v, want allowed", tt.command, err)
			}
			if !tt.allowed {
				if err == nil {
					t.Fatalf("Check(%q) = nil, want denied", tt.command)
				}
				if !errors.Is(err, ErrCommandDenied) {
					t.Fatalf("Check(%q) = %v, want ErrCommandDenied", tt.command, err)
				}
			}
		})
	}
}

func TestEmptyAllowlistDeniesEverything(t *testing.T) {
	al := NewAllowlist(nil, nil)
	if err := al.Check("git"); !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("Check with an empty allowlist = %v, want ErrCommandDenied", err)
	}
}
