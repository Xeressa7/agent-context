package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const prefManifest = "version: 1\nproject: deneme\nmemory_path: ../mem\n" +
	"role: current\ncomponents: [app]\n"

func prefRepo(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "proj", "context.yaml"), manifest)
	return filepath.Join(root, "proj")
}

func TestPreferencesShowDefault(t *testing.T) {
	env, out, _ := testEnv()
	repo := prefRepo(t, prefManifest)
	if code := Preferences(env, PreferencesOptions{Repo: repo}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"profil: normal", "16000", "açık"} {
		if !strings.Contains(got, want) {
			t.Errorf("çıktı %q %q içermiyor", got, want)
		}
	}
}

func TestPreferencesEconomicalWritesAndShows(t *testing.T) {
	env, out, _ := testEnv()
	repo := prefRepo(t, prefManifest)
	if code := Preferences(env, PreferencesOptions{Repo: repo, Profile: "ekonomik"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	after := norm(readFile(t, filepath.Join(repo, "context.yaml")))
	if !strings.Contains(after, "index_budget: 6000\n") {
		t.Errorf("index_budget yazılmadı:\n%s", after)
	}
	if strings.Contains(after, "auto_index") {
		t.Errorf("ekonomik profil auto_index'e dokunmamalı:\n%s", after)
	}

	env2, out2, _ := testEnv()
	if code := Preferences(env2, PreferencesOptions{Repo: repo}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "profil: ekonomik") {
		t.Errorf("çıktı %q", out2.String())
	}
	_ = out
}

func TestPreferencesManualSilencesHook(t *testing.T) {
	env, _, _ := testEnv()
	repo := prefRepo(t, prefManifest)
	if err := os.MkdirAll(filepath.Join(repo, "..", "mem"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Preferences(env, PreferencesOptions{Repo: repo, Profile: "manuel"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	hookEnv, hookOut, _ := testEnv()
	if code := HookIndex(hookEnv, repo); code != 0 {
		t.Fatalf("hook exit = %d", code)
	}
	if hookOut.String() != "" {
		t.Errorf("manuel profilde kanca susmalı, bastı: %q", hookOut.String())
	}
}

func TestPreferencesNormalRemovesLines(t *testing.T) {
	env, _, _ := testEnv()
	repo := prefRepo(t, prefManifest+"index_budget: 6000\nauto_index: false\n")
	if code := Preferences(env, PreferencesOptions{Repo: repo, Profile: "normal"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := norm(readFile(t, filepath.Join(repo, "context.yaml"))); got != prefManifest {
		t.Errorf("satırlar silinmedi:\n%s", got)
	}
}

func TestPreferencesCustomBudgetIsOzel(t *testing.T) {
	env, out, _ := testEnv()
	repo := prefRepo(t, prefManifest)
	if code := Preferences(env, PreferencesOptions{Repo: repo, Budget: 4000, BudgetGiven: true}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	env2, out2, _ := testEnv()
	Preferences(env2, PreferencesOptions{Repo: repo})
	if got := out2.String(); !strings.Contains(got, "profil: özel") || !strings.Contains(got, "4000") {
		t.Errorf("çıktı %q", got)
	}
	_ = out
}

func TestPreferencesInvalidBudgetWritesNothing(t *testing.T) {
	for _, n := range []int{0, -5} {
		env, out, _ := testEnv()
		repo := prefRepo(t, prefManifest)
		if code := Preferences(env, PreferencesOptions{Repo: repo, Budget: n, BudgetGiven: true}); code != 2 {
			t.Errorf("bütçe %d: exit = %d, 2 olmalı", n, code)
		}
		if !strings.Contains(out.String(), "HATA") {
			t.Errorf("bütçe %d: HATA basılmadı: %q", n, out.String())
		}
		if got := norm(readFile(t, filepath.Join(repo, "context.yaml"))); got != prefManifest {
			t.Errorf("bütçe %d: dosya değişmemeliydi:\n%s", n, got)
		}
	}
}

func TestPreferencesProfilePlusExplicitBudget(t *testing.T) {
	env, out, _ := testEnv()
	repo := prefRepo(t, prefManifest)
	opts := PreferencesOptions{Repo: repo, Profile: "ekonomik", Budget: 3000, BudgetGiven: true}
	if code := Preferences(env, opts); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	after := norm(readFile(t, filepath.Join(repo, "context.yaml")))
	if !strings.Contains(after, "index_budget: 3000\n") {
		t.Errorf("açık bütçe profili ezmeli:\n%s", after)
	}
	if got := out.String(); !strings.Contains(got, "profil: ekonomik") || !strings.Contains(got, "3000") {
		t.Errorf("çıktı %q", got)
	}
}

func TestPreferencesUnreadableBudgetFallsBack(t *testing.T) {
	env, out, _ := testEnv()
	repo := prefRepo(t, prefManifest+"index_budget: abc\n")
	if code := Preferences(env, PreferencesOptions{Repo: repo}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "profil: normal") || !strings.Contains(got, "16000") {
		t.Errorf("çıktı %q", got)
	}
}

func TestPreferencesKeepsCommentsAndOrder(t *testing.T) {
	env, _, _ := testEnv()
	in := "# proje manifesti\nversion: 1\nproject: deneme  # ad\n" +
		"memory_path: ../mem\nrole: current\n"
	repo := prefRepo(t, in)
	if code := Preferences(env, PreferencesOptions{Repo: repo, Profile: "ekonomik"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := norm(readFile(t, filepath.Join(repo, "context.yaml"))); got != in+"index_budget: 6000\n" {
		t.Errorf("yorum/sıra bozuldu:\n%s", got)
	}
}

func TestSetManifestLineSkipsIndentedAndComments(t *testing.T) {
	in := "retrieval:\n  index_budget: 1\nrole: current\n"
	if got := setManifestLine(in, "index_budget", strp("6000")); got != in+"index_budget: 6000\n" {
		t.Errorf("blok-içi satır eşleşmemeliydi:\n%q", got)
	}
	in2 := "# index_budget: 5\nrole: current\n"
	if got := setManifestLine(in2, "index_budget", strp("6000")); got != in2+"index_budget: 6000\n" {
		t.Errorf("yorum satırı eşleşmemeliydi:\n%q", got)
	}
}

func TestSetManifestLineKeepsCRLF(t *testing.T) {
	// Girdi ham CRLF de olsa çıktı normalize \n'dir — platform satır sonu
	// yazımda geri konur (writeText), tıpkı Python'un metin kipi gibi.
	// Böylece iki taraf da aynı baytları üretir.
	in := "role: current\r\nindex_budget: 5\r\n"
	if got := setManifestLine(in, "index_budget", strp("6000")); got != "role: current\nindex_budget: 6000\n" {
		t.Errorf("normalize çıktı: %q", got)
	}
	if got := setManifestLine(in, "index_budget", nil); got != "role: current\n" {
		t.Errorf("silme: %q", got)
	}
	if got := setManifestLine("role: current\n", "index_budget", nil); got != "role: current\n" {
		t.Errorf("bulunmayan anahtarın silinmesi dosyayı değiştirmemeli: %q", got)
	}
}
