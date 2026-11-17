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

// resolvedTempDir returns t.TempDir() with its symlinks resolved, so a test
// about one particular link is not also exercising the platform's own (on
// macOS, t.TempDir() lives under /var, which links to /private/var).
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
}

// TestIsPathSafeAcceptsMissingFileUnderSymlinkedBase guards the bug that made
// FileStore.Save fail for every base path reached through a symlink, such as
// anything under /tmp or /var on macOS. Symlinks were resolved in the base but
// not in a target that did not exist yet, so the two sides never matched.
func TestIsPathSafeAcceptsMissingFileUnderSymlinkedBase(t *testing.T) {
	root := resolvedTempDir(t)
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	symlinkOrSkip(t, real, link)

	for _, target := range []string{
		filepath.Join(link, "new.tmpl"),
		filepath.Join(link, "not", "yet", "created.tmpl"),
		filepath.Join(real, "new.tmpl"),
	} {
		if !isPathSafe(target, link) {
			t.Errorf("%q does not exist yet but is inside base %q, and was rejected", target, link)
		}
	}
}

// TestIsPathSafeRejectsSymlinkEscapes covers links inside the base that point
// outside it. The dangling-link and directory-link cases used to pass: a target
// EvalSymlinks could not resolve (because it did not exist yet) was compared
// unresolved, while creating the file follows the link out of the base.
func TestIsPathSafeRejectsSymlinkEscapes(t *testing.T) {
	root := resolvedTempDir(t)
	base := filepath.Join(root, "prompts")
	outside := filepath.Join(root, "outside")
	for _, dir := range []string{base, outside} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A file link to an existing file outside the base.
	symlinkOrSkip(t, secret, filepath.Join(base, "existing.tmpl"))
	// A dangling link: writing through it creates its target outside the base.
	symlinkOrSkip(t, filepath.Join(outside, "planted.tmpl"), filepath.Join(base, "dangling.tmpl"))
	// A directory link: anything created beneath it lands outside the base.
	symlinkOrSkip(t, outside, filepath.Join(base, "sub"))

	for _, attempt := range []string{
		filepath.Join(base, "existing.tmpl"),
		filepath.Join(base, "dangling.tmpl"),
		filepath.Join(base, "sub", "secret.txt"),
		filepath.Join(base, "sub", "new.tmpl"),
		filepath.Join(base, "sub", "deeper", "new.tmpl"),
		filepath.Join(base, "missing", "..", "..", "outside", "new.tmpl"),
	} {
		if isPathSafe(attempt, base) {
			t.Errorf("%q escapes base %q through a symlink or traversal, and was accepted", attempt, base)
		}
	}
}
