package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Migrate: eski tek-eksenli `lifecycle` → `status` + `scope`, satır bazlı ve
// yerinde; satır sonu ve sondaki newline KORUNUR.

func migrateArea(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	mem := filepath.Join(root, "mem")
	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: m\nmemory_path: ../mem\nrole: current\n")
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "x\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "x\n")
	for yol, metin := range files {
		writeFile(t, filepath.Join(mem, filepath.FromSlash(yol)), metin)
	}
	return proj
}

func TestMigrateConverts(t *testing.T) {
	proj := migrateArea(t, map[string]string{
		"facts/a.md": "---\ntitle: A\nlifecycle: product\ndescription: D\n---\n\ngovde\n",
		"facts/b.md": "---\ntitle: B\nlifecycle: current\n---\n\ngovde\n",
		"facts/c.md": "---\ntitle: C\nlifecycle: legacy\n---\n\ngovde\n",
		"facts/d.md": "---\ntitle: D\nlifecycle: historical\n---\n\ngovde\n",
		"facts/e.md": "---\ntitle: E\nlifecycle: uydurma\n---\n\ngovde\n",
		"facts/f.md": "---\ntitle: F\nstatus: active\n---\n\nzaten yeni\n",
	})
	env, _, _ := testEnv()
	if code := Migrate(env, MigrateOptions{Repo: proj}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	mem := filepath.Join(proj, "..", "mem")
	check := func(yol, want string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(mem, filepath.FromSlash(yol)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s: %q yok:\n%s", yol, want, raw)
		}
		if strings.Contains(string(raw), "lifecycle:") && yol != "facts/e.md" {
			t.Errorf("%s: lifecycle kalmış:\n%s", yol, raw)
		}
	}
	check("facts/a.md", "status: active")
	check("facts/b.md", "scope: [current]")
	check("facts/c.md", "scope: [legacy]")
	check("facts/d.md", "status: archived")
}

func TestMigrateDryAndUnknown(t *testing.T) {
	proj := migrateArea(t, map[string]string{
		"facts/e.md":    "---\ntitle: E\nlifecycle: uydurma\n---\n\ngovde\n",
		"facts/crlf.md": "---\r\ntitle: G\r\nlifecycle: current\r\n---\r\n\r\nCRLF\r\n",
		"facts/son.md":  "---\ntitle: H\nlifecycle: current\n---\n\nson newline yok",
	})
	before := map[string]string{}
	for _, f := range []string{"facts/e.md", "facts/crlf.md", "facts/son.md"} {
		before[f] = readFile(t, filepath.Join(proj, "..", "mem", filepath.FromSlash(f)))
	}
	env, _, _ := testEnv()
	if code := Migrate(env, MigrateOptions{Repo: proj, Dry: true}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	mem := filepath.Join(proj, "..", "mem")
	for f, metin := range before {
		if got := readFile(t, filepath.Join(mem, filepath.FromSlash(f))); got != metin {
			t.Errorf("kuru koşum %s yazmış", f)
		}
	}

	env2, _, _ := testEnv()
	Migrate(env2, MigrateOptions{Repo: proj})
	// Bilinmeyen eksen atlanır, CRLF + sonsuz-newline korunur.
	if got := readFile(t, filepath.Join(mem, "facts", "e.md")); !strings.Contains(got, "lifecycle: uydurma") {
		t.Errorf("bilinmeyen eksen durmalı:\n%s", got)
	}
	raw, _ := os.ReadFile(filepath.Join(mem, "facts", "crlf.md"))
	if !strings.Contains(string(raw), "\r\n") || !strings.Contains(string(raw), "scope: [current]") {
		t.Errorf("CRLF korunmalı + çevrilmeli: %q", raw)
	}
	raw2, _ := os.ReadFile(filepath.Join(mem, "facts", "son.md"))
	if strings.HasSuffix(string(raw2), "\n") {
		t.Errorf("son newline eklenmemeli: %q", raw2)
	}
}
