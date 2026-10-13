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

// İngilizce bayraklar birincil, eski Türkçe adlar gizli takma ad: ikisi de
// AYNI değişkene bağlanır, yani çıktı ve çıkış kodu birebir aynı olmalı.
// (Kullanıcıların settings.json'unda kayıtlı kancalar eski adları çağırır.)
func TestRunLegacyFlagAliases(t *testing.T) {
	for _, tc := range []struct {
		name         string
		english, old []string
	}{
		{"close --note/--not", []string{"close", "--note", "facts/billing.md"}, []string{"close", "--not", "facts/billing.md"}},
		{"close --none/--yok", []string{"close", "--none", "gerekce"}, []string{"close", "--yok", "gerekce"}},
		{"close --none=/--yok=", []string{"close", "--none=gerekce"}, []string{"close", "--yok=gerekce"}},
		{"close --check/--kontrol", []string{"close", "--check"}, []string{"close", "--kontrol"}},
		{"index --write/--yaz", []string{"index", "--write"}, []string{"index", "--yaz"}},
		{"index --check/--kontrol", []string{"index", "--check"}, []string{"index", "--kontrol"}},
		{"search --history/--tarihce", []string{"search", "billing", "--history"}, []string{"search", "billing", "--tarihce"}},
		{"validate --full/--tam", []string{"validate", "--full"}, []string{"validate", "--tam"}},
		{"migrate --dry-run/--kuru", []string{"migrate", "--dry-run"}, []string{"migrate", "--kuru"}},
	} {
		var outs [2]string
		var codes [2]int
		for i, argv := range [][]string{tc.english, tc.old} {
			proj := searchFixture(t)
			chdir(t, proj)
			env, out := testEnv()
			codes[i] = run(env, append(argv, "--repo", proj))
			// close ayrıştırması --repo'yu da bilir; tüm komutlarda ortak.
			outs[i] = strings.ReplaceAll(out.String(), proj, "<proj>")
		}
		if codes[0] != codes[1] || outs[0] != outs[1] {
			t.Errorf("%s: farklı sonuç\n%d %q\n%d %q", tc.name, codes[0], outs[0], codes[1], outs[1])
		}
	}
}

func TestRunBootstrapProjectAndDryRunAliases(t *testing.T) {
	var outs [2]string
	for i, argv := range [][]string{
		{"bootstrap", "--project", "deneme", "--dry-run"},
		{"bootstrap", "--proje", "deneme", "--kuru"},
	} {
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		t.Setenv("USERPROFILE", dir)
		proj := filepath.Join(dir, "p")
		write(t, filepath.Join(proj, "x"), "")
		chdir(t, proj)
		env, out := testEnv()
		if code := run(env, argv); code != 0 {
			t.Fatalf("run(%v) = %d: %s", argv, code, out.String())
		}
		if _, err := os.Stat(filepath.Join(proj, "context.yaml")); err == nil {
			t.Errorf("%v: kuru koşum dosya yazmamalı", argv)
		}
		outs[i] = strings.ReplaceAll(out.String(), dir, "<dir>")
	}
	if outs[0] != outs[1] {
		t.Errorf("farklı çıktı:\n%s\n---\n%s", outs[0], outs[1])
	}
	env, _ := testEnv()
	if code := run(env, []string{"bootstrap", "--dry-run"}); code != 2 {
		t.Errorf("--project olmadan = %d, want 2", code)
	}
}

func TestRunHookFlushAliases(t *testing.T) {
	var outs [2]string
	var codes [2]int
	for i, argv := range [][]string{
		{"hook", "flush", "--tool", "claude", "--reason", "sessionend"},
		{"hook", "flush", "--arac", "claude", "--sebep", "sessionend"},
	} {
		proj := searchFixture(t)
		env, out := testEnv()
		codes[i] = run(env, append(argv, "--repo", proj))
		outs[i] = strings.ReplaceAll(out.String(), proj, "<proj>")
	}
	if codes[0] != 0 || codes[0] != codes[1] || outs[0] != outs[1] {
		t.Errorf("farklı sonuç\n%d %q\n%d %q", codes[0], outs[0], codes[1], outs[1])
	}
}

func TestUsageShowsOnlyEnglishFlags(t *testing.T) {
	for _, eski := range []string{"--yaz", "--kuru", "--kontrol", "--tam", "--not ", "--yok", "--tarihce", "--proje ", "--arac", "--sebep"} {
		if strings.Contains(usage, eski) {
			t.Errorf("usage eski bayrağı %q göstermemeli", eski)
		}
	}
}
