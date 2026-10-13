package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Bootstrap sözleri init'inkilerle aynı, artı: settings.json'a YALNIZ eksik
// kayıt eklenir; eski script kaydı yerinde güncellenir, ikinci kez eklenmez.

func bootstrapArea(t *testing.T) (home, proj string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	proj = filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHome(t, home)
	chdir(t, proj)
	return home, proj
}

func settingsCommands(t *testing.T, home string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ayar map[string]any
	if err := json.Unmarshal(raw, &ayar); err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, g := range ayar["hooks"].(map[string]any)["SessionStart"].([]any) {
		for _, e := range g.(map[string]any)["hooks"].([]any) {
			parts = append(parts, e.(map[string]any)["command"].(string))
		}
	}
	return strings.Join(parts, " ")
}

func TestBootstrapRegistersHooks(t *testing.T) {
	home, proj := bootstrapArea(t)
	env, _, _ := testEnv()
	// Çıkış doctor'undur: taze bir kurulum kendi sağlık kontrolünden geçmeli.
	if code := Bootstrap(env, BootstrapOptions{InitOptions: InitOptions{Project: "deneme", Role: "current"}}); code != 0 {
		t.Fatalf("exit = %d, taze kurulumda doctor 0 dönmeli", code)
	}
	_ = proj
	komutlar := settingsCommands(t, home)
	for _, want := range []string{"hook index", "hook capture-check"} {
		if !strings.Contains(komutlar, want) {
			t.Errorf("%q kaydedilmemiş: %s", want, komutlar)
		}
	}
}

func TestBootstrapUpdatesLegacyEntryInPlace(t *testing.T) {
	home, _ := bootstrapArea(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", `+
			`"command": "python \"/eski/ctx_index.py\"", "statusMessage": "benim"}]}]}}`)
	env, _, _ := testEnv()
	Bootstrap(env, BootstrapOptions{InitOptions: InitOptions{Project: "deneme", Role: "current"}})
	komutlar := settingsCommands(t, home)
	if strings.Count(komutlar, "hook index") != 1 {
		t.Errorf("kanca iki kez kayıtlı: %s", komutlar)
	}
	if strings.Contains(komutlar, "ctx_index.py") {
		t.Errorf("eski komut duruyor: %s", komutlar)
	}
}

func TestBootstrapSecondRunChangesNothing(t *testing.T) {
	home, proj := bootstrapArea(t)
	opts := BootstrapOptions{InitOptions: InitOptions{Project: "deneme", Role: "current"}}
	env, _, _ := testEnv()
	Bootstrap(env, opts)
	env2, out2, _ := testEnv()
	Bootstrap(env2, opts)
	got := out2.String()
	if !strings.Contains(got, "kayitli") {
		t.Errorf("ikinci koşum 'kayıtlı' demeli:\n%s", got)
	}
	if strings.Contains(got, "eklendi") || strings.Contains(got, "↻") {
		t.Errorf("ikinci koşum yazmamalı:\n%s", got)
	}
	_ = proj
	_ = home
}

func TestBootstrapKeepsUserSettings(t *testing.T) {
	home, _ := bootstrapArea(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"theme": "dark", "hooks": {"SessionStart": [{"hooks": [`+
			`{"type": "command", "command": "python benim_kancam.py"}]}]}}`)
	env, _, _ := testEnv()
	Bootstrap(env, BootstrapOptions{InitOptions: InitOptions{Project: "deneme", Role: "current"}})
	raw := readFile(t, filepath.Join(home, ".claude", "settings.json"))
	if !strings.Contains(raw, `"theme": "dark"`) {
		t.Errorf("ilgisiz ayar kayboldu:\n%s", raw)
	}
	if !strings.Contains(raw, "benim_kancam.py") || !strings.Contains(raw, "hook index") {
		t.Errorf("kanca birleşmesi yanlış:\n%s", raw)
	}
}

func TestBootstrapBrokenSettingsUntouched(t *testing.T) {
	home, _ := bootstrapArea(t)
	bozuk := "{ bu gecerli json degil"
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), bozuk)
	env, out, _ := testEnv()
	Bootstrap(env, optsOf("deneme"))
	if got := readFile(t, filepath.Join(home, ".claude", "settings.json")); got != bozuk {
		t.Errorf("bozuk ayar ezildi:\n%s", got)
	}
	if !strings.Contains(out.String(), "okunamadi") {
		t.Errorf("çıktı %q", out.String())
	}
}

func optsOf(proje string) BootstrapOptions {
	return BootstrapOptions{InitOptions: InitOptions{Project: proje, Role: "current"}}
}

// Eski Türkçe bayraklı kanca kaydı yerinde İngilizce bayraklıya çevrilir
// (ilk kelimeyle eşleşir), ikinci kez eklenmez.
func TestBootstrapRewritesOldTurkishFlagHooks(t *testing.T) {
	home, _ := bootstrapArea(t)
	eski := `"` + ctxWrapper() + `" hook flush --arac claude --sebep sessionend`
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"hooks": {"SessionEnd": [{"hooks": [{"type": "command", "command": `+jsonString(eski)+`}]}]}}`)
	env, _, _ := testEnv()
	Bootstrap(env, optsOf("deneme"))
	raw := readFile(t, filepath.Join(home, ".claude", "settings.json"))
	if strings.Contains(raw, "--arac") || strings.Contains(raw, "--sebep") {
		t.Errorf("eski bayraklar yerinde değişmeli:\n%s", raw)
	}
	if strings.Count(raw, "hook flush --tool claude --reason sessionend") != 1 {
		t.Errorf("tek ve İngilizce bayraklı kayıt beklenir:\n%s", raw)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestBootstrapDryRunReportsSkillsAndWritesNone(t *testing.T) {
	_, proj := bootstrapArea(t)
	writeFile(t, filepath.Join(proj, ".agents", "skills", "ctx-yaz", "SKILL.md"), "---\nname: ctx-yaz\n---\n")
	opts := optsOf("deneme")
	opts.Dry = true
	env, out, _ := testEnv()
	Bootstrap(env, opts)
	got := out.String()
	for _, want := range []string{"ctx-yaz -> ctx-write", ".claude/skills/ctx-close/SKILL.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("kuru koşum %q raporlamalı:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".agents", "skills", "ctx-yaz")); err != nil {
		t.Errorf("kuru koşum taşımamalı: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude")); !os.IsNotExist(err) {
		t.Errorf("kuru koşum .claude yazmamalı (err=%v)", err)
	}
}
