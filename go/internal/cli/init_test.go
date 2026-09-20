package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kurulum sözleri: IDEMPOTENT (ikinci koşum dokunmaz) ve YIKICI DEĞİL
// (mevcut dosya ezilmez, kullanıcı ayarı korunur).

func TestInitScaffolds(t *testing.T) {
	alan := t.TempDir()
	root := filepath.Join(alan, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	env, out, _ := testEnv()
	if code := Init(env, InitOptions{Project: "deneme", Role: "current", Components: "app"}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	for _, yol := range []string{
		"context.yaml", "AGENTS.md", "CLAUDE.md",
		".agents/skills/ctx-ara/SKILL.md",
		".agents/skills/ctx-yaz/SKILL.md",
		".agents/skills/ctx-kapat/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, yol)); err != nil {
			t.Errorf("%s kurulmadı: %v", yol, err)
		}
	}
	// Hafıza proje YANINDA kurulur (../deneme-memory).
	for _, d := range []string{"facts", "decisions", "pitfalls", "tasks", "sessions"} {
		if _, err := os.Stat(filepath.Join(alan, "deneme-memory", d)); err != nil {
			t.Errorf("hafıza %s kurulmadı: %v", d, err)
		}
	}
	if got := out.String(); !strings.Contains(got, "olusturuldu") {
		t.Errorf("özet beklenir:\n%s", got)
	}
}

func TestInitDoesNotClobber(t *testing.T) {
	alan := t.TempDir()
	root := filepath.Join(alan, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "benim dosyam\n")
	env, _, _ := testEnv()
	Init(env, InitOptions{Project: "deneme", Role: "current", Components: "app"})
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != "benim dosyam\n" {
		t.Errorf("AGENTS.md ezildi:\n%s", got)
	}
	// Skill de aynı: kullanıcının düzenlemesi korunur.
	skill := filepath.Join(root, ".agents", "skills", "ctx-ara", "SKILL.md")
	writeFile(t, skill, "benim aramam\n")
	env2, out2, _ := testEnv()
	Init(env2, InitOptions{Project: "deneme", Role: "current", Components: "app"})
	if got := readFile(t, skill); got != "benim aramam\n" {
		t.Errorf("skill ezildi:\n%s", got)
	}
	if !strings.Contains(out2.String(), "DOKUNULMADI") {
		t.Errorf("çıktı %q", out2.String())
	}
}
