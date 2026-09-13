package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, content string, resources map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for path, body := range resources {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return dir
}

const triageSkill = `---
name: incident-triage
description: Triage a production incident and identify the failing component
version: "1.2"
tags: [ops, oncall]
---

# Triage

1. Check the error rate dashboard.
2. Correlate with recent deploys.
`

func TestParseReadsFrontmatterAndBody(t *testing.T) {
	s, err := Parse(triageSkill)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if s.Name != "incident-triage" {
		t.Errorf("Name = %q", s.Name)
	}
	if !strings.Contains(s.Description, "Triage a production incident") {
		t.Errorf("Description = %q", s.Description)
	}
	if s.Version != "1.2" {
		t.Errorf("Version = %q, want %q", s.Version, "1.2")
	}
	if len(s.Tags) != 2 {
		t.Errorf("Tags = %v, want two", s.Tags)
	}
	if !strings.Contains(s.Instructions, "error rate dashboard") {
		t.Errorf("Instructions did not capture the body: %q", s.Instructions)
	}
	if strings.Contains(s.Instructions, "description:") {
		t.Error("frontmatter leaked into the instructions")
	}
}

func TestParseRejectsMissingFrontmatter(t *testing.T) {
	if _, err := Parse("# Just markdown\n"); err == nil {
		t.Error("a skill without frontmatter should be rejected")
	}
}

func TestParseRejectsUnclosedFrontmatter(t *testing.T) {
	if _, err := Parse("---\nname: x\ndescription: y\n"); err == nil {
		t.Error("unclosed frontmatter should be rejected")
	}
}

// TestDescriptionIsRequired guards a subtle failure: a skill with no
// description can never be selected by the model, so it would sit in the
// library permanently unreachable.
func TestDescriptionIsRequired(t *testing.T) {
	_, err := Parse("---\nname: nameless\n---\nbody\n")
	if err == nil {
		t.Fatal("a skill without a description should be rejected")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error = %v, want it to name the missing field", err)
	}
}

func TestLoadDirLoadsEverySkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, nil)
	writeSkill(t, root, "release-notes", `---
name: release-notes
description: Draft release notes from a changelog
---
Write them.
`, nil)
	// A directory without a SKILL.md is not a skill and must be skipped.
	if err := os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o750); err != nil {
		t.Fatal(err)
	}

	lib, err := LoadDir(root)
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	if lib.Len() != 2 {
		t.Errorf("loaded %d skills, want 2", lib.Len())
	}
	if _, ok := lib.Get("incident-triage"); !ok {
		t.Error("incident-triage was not loaded")
	}
}

func TestLoadDirReportsBadSkillsWithoutLosingGoodOnes(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "good", triageSkill, nil)
	writeSkill(t, root, "bad", "no frontmatter here", nil)

	lib := NewLibrary()
	err := lib.LoadDir(root)

	if err == nil {
		t.Error("a malformed skill should be reported")
	}
	if lib.Len() != 1 {
		t.Errorf("loaded %d skills, want the one good skill to survive", lib.Len())
	}
}

// TestCatalogIsTheWholeContextCost is the property that makes a large skill
// library affordable: only names and descriptions are in context until a skill
// is actually invoked.
func TestCatalogIsTheWholeContextCost(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, nil)
	lib, _ := LoadDir(root)

	catalog := lib.Catalog()

	if !strings.Contains(catalog, "incident-triage") {
		t.Error("the catalog should name each skill")
	}
	if !strings.Contains(catalog, "Triage a production incident") {
		t.Error("the catalog should carry each description")
	}
	if strings.Contains(catalog, "error rate dashboard") {
		t.Error("the catalog leaked a skill's full instructions; only the name " +
			"and description belong in context until load_skill is called")
	}
}

func TestLoadSkillToolReturnsInstructionsOnDemand(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, map[string]string{
		"runbook.md": "# Runbook\nRestart the thing.",
	})
	lib, _ := LoadDir(root)

	tools := Tools(lib)
	if len(tools) != 2 {
		t.Fatalf("Tools() returned %d tools, want 2", len(tools))
	}

	out, err := tools[0].Execute(context.Background(), `{"name":"incident-triage"}`)
	if err != nil {
		t.Fatalf("load_skill error = %v", err)
	}
	if !strings.Contains(out, "error rate dashboard") {
		t.Error("load_skill did not return the instructions")
	}
	if !strings.Contains(out, "runbook.md") {
		t.Error("load_skill should list the bundled resources")
	}
}

func TestLoadSkillToolConstrainsNamesToTheLibrary(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, nil)
	lib, _ := LoadDir(root)

	spec := Tools(lib)[0].Parameters()["name"]
	if len(spec.Enum) != 1 {
		t.Fatalf("name enum = %v, want exactly the loaded skill", spec.Enum)
	}
	if spec.Enum[0] != "incident-triage" {
		t.Errorf("enum = %v, want the skill name", spec.Enum)
	}
}

func TestUnknownSkillIsRecoverable(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, nil)
	lib, _ := LoadDir(root)

	out, err := Tools(lib)[0].Execute(context.Background(), `{"name":"does-not-exist"}`)
	if err != nil {
		t.Fatalf("an unknown skill should not fail the run: %v", err)
	}
	if !strings.Contains(out, "incident-triage") {
		t.Error("the response should list what does exist, so the turn is recoverable")
	}
}

func TestReadResourceReturnsBundledFiles(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, map[string]string{
		"queries/slow.sql": "SELECT 1;",
	})
	lib, _ := LoadDir(root)

	args, _ := json.Marshal(map[string]string{"skill": "incident-triage", "path": "queries/slow.sql"})
	out, err := Tools(lib)[1].Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("read_skill_resource error = %v", err)
	}
	if out != "SELECT 1;" {
		t.Errorf("resource content = %q, want the file's contents", out)
	}
}

// TestResourceTraversalIsRefused guards a real attack surface: a skill folder
// arrives by git clone, so treating its resource names as trusted paths would
// let one read arbitrary files off the host.
func TestResourceTraversalIsRefused(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "evil", `---
name: evil
description: tries to escape its directory
---
body
`, nil)

	// A secret outside the skill directory.
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, _ := LoadDir(root)
	s, _ := lib.Get("evil")

	for _, attempt := range []string{
		"../secret.txt",
		"../../etc/passwd",
		"subdir/../../secret.txt",
	} {
		if content, err := s.ReadResource(attempt); err == nil {
			t.Errorf("ReadResource(%q) succeeded and returned %q; traversal must be refused",
				attempt, content)
		}
	}
}

// TestResourceSymlinkEscapeIsRefused covers the subtler form: a link inside the
// directory pointing outside it.
func TestResourceSymlinkEscapeIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	root := t.TempDir()
	dir := writeSkill(t, root, "linky", `---
name: linky
description: contains a symlink pointing outside
---
body
`, nil)

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	lib := NewLibrary()
	_ = lib.LoadDir(root)
	s, ok := lib.Get("linky")
	if !ok {
		t.Fatal("skill did not load")
	}

	if content, err := s.ReadResource("link.txt"); err == nil {
		t.Errorf("a symlink escaping the skill directory was followed, returning %q", content)
	}
}

func TestReadResourceRejectsUnknownFile(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "incident-triage", triageSkill, nil)
	lib, _ := LoadDir(root)

	args, _ := json.Marshal(map[string]string{"skill": "incident-triage", "path": "nope.md"})
	out, err := Tools(lib)[1].Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("a missing resource should not fail the run: %v", err)
	}
	if !strings.Contains(out, "Could not read") {
		t.Errorf("response = %q, want a readable explanation", out)
	}
}

func TestAddReplacesSkillOfTheSameName(t *testing.T) {
	lib := NewLibrary()
	_ = lib.Add(&Skill{Name: "x", Description: "first"})
	_ = lib.Add(&Skill{Name: "x", Description: "second"})

	if lib.Len() != 1 {
		t.Errorf("Len() = %d, want 1", lib.Len())
	}
	s, _ := lib.Get("x")
	if s.Description != "second" {
		t.Errorf("Description = %q, want the replacement", s.Description)
	}
}

func TestEmptyLibraryCatalogIsHonest(t *testing.T) {
	if got := NewLibrary().Catalog(); !strings.Contains(got, "No skills") {
		t.Errorf("Catalog() = %q, want it to say there are none", got)
	}
}
