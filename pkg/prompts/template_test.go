package prompts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestIsPathSafeRejectsSiblingDirectories guards a prefix-boundary bug.
//
// The check was a bare strings.HasPrefix, which has no notion of a path
// boundary: with a base of "/srv/prompts" it accepted "/srv/prompts-evil/x",
// because that string does start with the base. A sibling directory whose name
// merely begins with the base name escaped the sandbox.
func TestIsPathSafeRejectsSiblingDirectories(t *testing.T) {
	root := t.TempDir()

	base := filepath.Join(root, "prompts")
	sibling := filepath.Join(root, "prompts-evil")
	for _, dir := range []string{base, sibling} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	inside := filepath.Join(base, "ok.tmpl")
	escaped := filepath.Join(sibling, "steal.tmpl")
	for _, f := range []string{inside, escaped} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if !isPathSafe(inside, base) {
		t.Error("a file genuinely inside the base directory was rejected")
	}
	if isPathSafe(escaped, base) {
		t.Errorf("%q was accepted for base %q: a sibling directory whose name "+
			"starts with the base name must not pass", escaped, base)
	}
}

func TestIsPathSafeRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "prompts")
	if err := os.MkdirAll(base, 0o750); err != nil {
		t.Fatal(err)
	}

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, attempt := range []string{
		filepath.Join(base, "..", "secret.txt"),
		filepath.Join(base, "sub", "..", "..", "secret.txt"),
	} {
		if isPathSafe(attempt, base) {
			t.Errorf("traversal %q was accepted", attempt)
		}
	}
}

// TestIsPathSafeAcceptsWhenTheBaseIsItselfASymlink is the mirror-image bug:
// resolving symlinks on only one side rejects legitimate paths whenever a
// parent directory is a link, which is the default on macOS temp dirs.
func TestIsPathSafeAcceptsWhenTheBaseIsItselfASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	file := filepath.Join(real, "ok.tmpl")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !isPathSafe(file, link) {
		t.Error("a legitimate file was rejected because the base directory is a symlink")
	}
}

func TestIsPathSafeAcceptsTheBaseItself(t *testing.T) {
	base := t.TempDir()
	if !isPathSafe(base, base) {
		t.Error("the base directory itself should be considered inside the base")
	}
}
