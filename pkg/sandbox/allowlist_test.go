package sandbox

import (
	"errors"
	"testing"
)

func TestAllowlist_Check_AllowedCommand(t *testing.T) {
	al := NewAllowlist([]string{"git", "curl"}, nil)
	if err := al.Check("git"); err != nil {
		t.Errorf("expected git to be allowed, got: %v", err)
	}
}

func TestAllowlist_Check_AllowedAbsolutePath(t *testing.T) {
	al := NewAllowlist([]string{"git"}, nil)
	if err := al.Check("/usr/bin/git"); err != nil {
		t.Errorf("expected /usr/bin/git to be allowed, got: %v", err)
	}
}

func TestAllowlist_Check_DeniedCommand(t *testing.T) {
	al := NewAllowlist([]string{"git", "rm"}, []string{"rm"})
	err := al.Check("rm")
	if err == nil {
		t.Error("expected rm to be denied")
	}
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestAllowlist_Check_DenyTakesPrecedence(t *testing.T) {
	al := NewAllowlist([]string{"rm"}, []string{"rm"})
	err := al.Check("rm")
	if err == nil {
		t.Error("deny should take precedence over allow")
	}
}

func TestAllowlist_Check_NotInAllowlist(t *testing.T) {
	al := NewAllowlist([]string{"git"}, nil)
	err := al.Check("curl")
	if err == nil {
		t.Error("expected curl to be denied when not in allowlist")
	}
}

func TestAllowlist_Check_EmptyAllowlistDeniesAll(t *testing.T) {
	al := NewAllowlist(nil, nil)
	err := al.Check("git")
	if err == nil {
		t.Error("expected all commands denied when allowlist is empty (fail-closed)")
	}
}

func TestAllowlist_Check_CaseInsensitive(t *testing.T) {
	al := NewAllowlist([]string{"Git"}, nil)
	if err := al.Check("git"); err != nil {
		t.Errorf("expected case-insensitive match, got: %v", err)
	}
}
