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
	// Hafıza proje İÇİNDE, gitignore'lu .memory/ klasöründe kurulur.
	for _, d := range []string{"facts", "decisions", "pitfalls", "tasks", "sessions"} {
		if _, err := os.Stat(filepath.Join(root, ".memory", d)); err != nil {
			t.Errorf("hafıza %s kurulmadı: %v", d, err)
		}
	}
	if got := readFile(t, filepath.Join(root, "context.yaml")); !strings.Contains(got, "memory_path: .memory") {
		t.Errorf("context.yaml memory_path .memory olmalı:\n%s", got)
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); !strings.Contains(got, "/.memory/") {
		t.Errorf(".gitignore /.memory/ taşımalı:\n%s", got)
	}
	if got := out.String(); !strings.Contains(got, "olusturuldu") {
		t.Errorf("özet beklenir:\n%s", got)
	}
}

func TestInitAppendsToExistingGitignoreOnce(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	// Sonunda satır sonu YOK: ekleme önceki satıra yapışmamalı.
	writeFile(t, filepath.Join(root, ".gitignore"), "node_modules/\r\n*.log")
	for i := 0; i < 2; i++ {
		env, _, _ := testEnv()
		Init(env, InitOptions{Project: "deneme", Role: "current", Components: "app"})
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != "node_modules/\r\n*.log\r\n/.memory/\r\n" {
		t.Errorf(".gitignore beklenmedik (kullanıcı satırları + satır sonu korunmalı, tek ekleme):\n%q", got)
	}
}

func TestInitRecognisesExistingGitignoreEntry(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeFile(t, filepath.Join(root, ".gitignore"), ".memory\n")
	env, out, _ := testEnv()
	Init(env, InitOptions{Project: "deneme", Role: "current", Components: "app"})
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != ".memory\n" {
		t.Errorf("eşdeğer satır varken dokunulmamalı:\n%q", got)
	}
	if !strings.Contains(out.String(), "zaten var") {
		t.Errorf("çıktı %q", out.String())
	}
}

func TestInitExternalMemoryLeavesGitignoreAlone(t *testing.T) {
	alan := t.TempDir()
	root := filepath.Join(alan, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	env, _, _ := testEnv()
	Init(env, InitOptions{Project: "deneme", MemoryPath: "../deneme-memory", Role: "current", Components: "app"})
	if _, err := os.Stat(filepath.Join(alan, "deneme-memory", "facts")); err != nil {
		t.Errorf("dış hafıza kurulmadı: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("dış hafızada .gitignore yazılmamalı (err=%v)", err)
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
