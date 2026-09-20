package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Xeressa7/agent-context/go/internal/cli"
)

// Komut satırı ayrıştırma: flag paketi ilk positional'da DURUR (reorder),
// `--yok` opsiyonel argüman alır (el ile parse), `--index-budget` verilişi
// hissedilir. Bunlar eskiden yalnız diller-arası paritede test ediliyordu.

func testEnv() (cli.Env, *bytes.Buffer) {
	var out bytes.Buffer
	return cli.Env{Out: &out, Err: &out}, &out
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	})
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCodes(t *testing.T) {
	env, _ := testEnv()
	for _, tc := range []struct {
		argv []string
		want int
	}{
		{nil, 2},
		{[]string{"yok-boyle-komut"}, 2},
		{[]string{"--help"}, 0},
		{[]string{"search"}, 2},
		{[]string{"hook"}, 2},
		{[]string{"hook", "bogus"}, 2},
		{[]string{"init"}, 2},
		{[]string{"bootstrap"}, 2},
	} {
		if got := run(env, tc.argv); got != tc.want {
			t.Errorf("run(%v) = %d, want %d", tc.argv, got, tc.want)
		}
	}
}

func searchFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	write(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n")
	mem := filepath.Join(root, "mem")
	write(t, filepath.Join(mem, "README.md"), "# h\n")
	write(t, filepath.Join(mem, "tasks", "OPEN.md"), "x\n")
	write(t, filepath.Join(mem, "sessions", "CURRENT.md"), "x\n")
	write(t, filepath.Join(mem, "facts", "billing.md"),
		"---\ntitle: Fatura\ndescription: x\nstatus: active\nupdated: 2026-08-13\n---\n\ngovde\n")
	return proj
}

func TestRunReorderHonorsLateFlags(t *testing.T) {
	proj := searchFixture(t)
	env1, out1 := testEnv()
	if code := run(env1, []string{"search", "--repo", proj, "billing", "--top", "1"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	env2, out2 := testEnv()
	// Bayrak sonda: reorder olmasa sessizce düşerdi.
	if code := run(env2, []string{"search", "billing", "--repo", proj, "--top", "1"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out1.String() != out2.String() {
		t.Errorf("sıra sonucu değiştirmemeli:\n%q\n%q", out1.String(), out2.String())
	}
	if !strings.Contains(out1.String(), "billing.md") {
		t.Errorf("çıktı %q", out1.String())
	}
}

func TestRunCloseYokForms(t *testing.T) {
	proj := searchFixture(t)
	chdir(t, proj)
	for _, argv := range [][]string{
		{"close", "--yok"},
		{"close", "--yok", "gerekce"},
		{"close", "--yok=gerekce"},
	} {
		env, _ := testEnv()
		if code := run(env, argv); code != 0 {
			t.Errorf("run(%v) = %d, want 0", argv, code)
		}
	}
	env, _ := testEnv()
	if code := run(env, []string{"close", "--not", "yok.md"}); code != 2 {
		t.Errorf("olmayan not = %d, want 2", code)
	}
}

func TestRunPreferencesBudgetGiven(t *testing.T) {
	proj := searchFixture(t)
	for _, argv := range [][]string{
		{"preferences", "--repo", proj, "--index-budget=0"},
		{"preferences", "--repo", proj, "--index-budget", "0"},
		{"preferences", "--repo", proj, "--index-budget=-5"},
	} {
		env, _ := testEnv()
		if code := run(env, argv); code != 2 {
			t.Errorf("run(%v) = %d, want 2", argv, code)
		}
	}
	env, out := testEnv()
	if code := run(env, []string{"preferences", "--repo", proj}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "profil: normal") {
		t.Errorf("çıktı %q", out.String())
	}
}
