package store

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifestShapes(t *testing.T) {
	m := ParseManifest(`
project: yapit          # trailing comment
memory_path: ../yapit-memory
scope: [current, server]
role: current
components:
  api: fastapi
  ui: electron
empty:
review_days: 45
`)

	if got := m.Str("project"); got != "yapit" {
		t.Errorf("project = %q, want %q — comment not stripped?", got, "yapit")
	}
	if got := m.List("scope"); len(got) != 2 || got[0] != "current" || got[1] != "server" {
		t.Errorf("scope = %v, want [current server]", got)
	}
	if got := m.Int("review_days", 90); got != 45 {
		t.Errorf("review_days = %d, want 45", got)
	}
	if got := m.Int("missing", 90); got != 90 {
		t.Errorf("missing review_days = %d, want fallback 90", got)
	}
	if nested := m["components"].Fields; nested["api"].Text != "fastapi" || nested["ui"].Text != "electron" {
		t.Errorf("nested block = %v, want api/ui entries", nested)
	}
	if m["empty"].Fields == nil {
		t.Error("a key with no value should parse as an empty block")
	}
}

// A comma-separated scalar and a bracket list have to mean the same thing —
// otherwise "server, worker" is read as ONE tag and scope matching fails
// silently, which is the bug the shared SplitList exists to prevent.
func TestListAcceptsBothShapes(t *testing.T) {
	bracket := ParseManifest("scope: [a, b]").List("scope")
	scalar := ParseManifest("scope: a, b").List("scope")
	if len(bracket) != 2 || len(scalar) != 2 || bracket[0] != scalar[0] || bracket[1] != scalar[1] {
		t.Errorf("bracket %v != scalar %v", bracket, scalar)
	}
}

func TestRepoScopeFallsBackToRole(t *testing.T) {
	if got := RepoScope(ParseManifest("role: legacy")); len(got) != 1 || got[0] != "legacy" {
		t.Errorf("role fallback = %v, want [legacy]", got)
	}
	if got := RepoScope(ParseManifest("scope: [a]\nrole: legacy")); len(got) != 1 || got[0] != "a" {
		t.Errorf("scope should win over role, got %v", got)
	}
	if got := RepoScope(ParseManifest("project: x")); got != nil {
		t.Errorf("no scope and no role should be nil, got %v", got)
	}
}

func TestFindManifestWalksUp(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ManifestName), "memory_path: ./mem\n")
	deep := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	found := FindManifest(deep)
	if found == "" {
		t.Fatal("manifest not found from a nested directory")
	}
	if filepath.Base(found) != ManifestName {
		t.Errorf("found %q", found)
	}

	ctx, err := LoadContext(deep)
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	want, _ := filepath.Abs(filepath.Join(root, "mem"))
	if ctx.MemoryPath != want {
		t.Errorf("MemoryPath = %q, want %q (must resolve against the MANIFEST, not the cwd)",
			ctx.MemoryPath, want)
	}
}

func TestLoadContextErrors(t *testing.T) {
	if _, err := LoadContext(t.TempDir()); err == nil {
		t.Error("a directory with no manifest should be an error")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, ManifestName), "project: x\n")
	if _, err := LoadContext(root); err == nil {
		t.Error("a manifest without memory_path should be an error")
	}
}

func TestWalkNotesSkipsAndSorts(t *testing.T) {
	mem := t.TempDir()
	write(t, filepath.Join(mem, "README.md"), "scaffolding")
	write(t, filepath.Join(mem, "ctx.yaml"), "skip_dirs: [gecis]\n")
	write(t, filepath.Join(mem, "tasks", "OPEN.md"), "state")
	write(t, filepath.Join(mem, "sessions", "CURRENT.md"), "state")
	write(t, filepath.Join(mem, "gecis", "eski.md"), "project-specific skip")
	write(t, filepath.Join(mem, "domain", "b.md"), "note")
	write(t, filepath.Join(mem, "domain", "a.md"), "note")
	write(t, filepath.Join(mem, "pitfalls", "deep", "c.md"), "note")

	notes, err := WalkNotes(mem)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(notes))
	for i, n := range notes {
		got[i] = n.Rel
	}
	want := []string{"domain/a.md", "domain/b.md", "pitfalls/deep/c.md"}
	if len(got) != len(want) {
		t.Fatalf("walked %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walked %v, want %v", got, want)
		}
	}
}

func TestLoadNotesSplitsHeaderAndBody(t *testing.T) {
	mem := t.TempDir()
	write(t, filepath.Join(mem, "domain", "x.md"),
		"---\ntitle: Baslik\nscope: [current]\n---\n\ngovde metni\n")

	notes, err := LoadNotes(mem)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes", len(notes))
	}
	n := notes[0]
	if n.FrontMatter["title"] != "Baslik" {
		t.Errorf("title = %q", n.FrontMatter["title"])
	}
	if n.FrontMatter["scope"] != "current" {
		t.Errorf("scope = %q, want the flattened %q", n.FrontMatter["scope"], "current")
	}
	if n.Body != "\ngovde metni\n" {
		t.Errorf("body = %q — front matter must not leak into the body", n.Body)
	}
}
