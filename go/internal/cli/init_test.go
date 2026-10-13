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
		".agents/skills/ctx-search/SKILL.md",
		".agents/skills/ctx-write/SKILL.md",
		".agents/skills/ctx-close/SKILL.md",
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

// Tüketici repoda yeniden koşum: context.yaml'daki memory_path kullanılır; repo
// içinde ikinci, boş bir .memory açılmaz ve .gitignore'a satır eklenmez.
func TestInitHonoursExistingMemoryPath(t *testing.T) {
	alan := t.TempDir()
	root := filepath.Join(alan, "sunucu")
	writeFile(t, filepath.Join(root, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../.memory\n")
	chdir(t, root)
	env, _, _ := testEnv()
	Init(env, InitOptions{Project: "p", Role: "current", Components: "server"})
	if _, err := os.Stat(filepath.Join(root, ".memory")); !os.IsNotExist(err) {
		t.Errorf("repo içinde ikinci hafıza açılmamalı (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(alan, ".memory", "facts")); err != nil {
		t.Errorf("hafıza context.yaml'daki yolda olmalı: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("repo dışındaki hafıza için .gitignore yazılmamalı (err=%v)", err)
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
	skill := filepath.Join(root, ".agents", "skills", "ctx-search", "SKILL.md")
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

// Claude Code projenin skill'lerini .claude/skills'ten yükler, Codex/agents
// .agents/skills'ten: ikisine de AYNI içerik kurulmalı.
func TestInitInstallsSkillsInBothLocations(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	env, _, _ := testEnv()
	Init(env, InitOptions{Project: "deneme", Role: "current", Components: "app"})
	for _, name := range skillNames {
		a := readFile(t, filepath.Join(root, ".agents", "skills", name, "SKILL.md"))
		c := readFile(t, filepath.Join(root, ".claude", "skills", name, "SKILL.md"))
		if a != c || a == "" {
			t.Errorf("%s: iki konumda aynı içerik olmalı", name)
		}
	}
}

func TestInitReportsDifferingSkillAndUpdateOverwrites(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	opts := InitOptions{Project: "deneme", Role: "current", Components: "app"}
	env, _, _ := testEnv()
	Init(env, opts)
	skill := filepath.Join(root, ".claude", "skills", "ctx-write", "SKILL.md")
	writeFile(t, skill, "elle değişti\n")

	env2, out2, _ := testEnv()
	Init(env2, opts)
	if got := readFile(t, skill); got != "elle değişti\n" {
		t.Errorf("--update-skills yokken ezildi:\n%s", got)
	}
	if !strings.Contains(out2.String(), "farklı — şablondan eski ya da elle değiştirilmiş") {
		t.Errorf("fark raporlanmalı:\n%s", out2.String())
	}

	opts.UpdateSkills = true
	env3, _, _ := testEnv()
	Init(env3, opts)
	want, _ := template("skills/ctx-write/SKILL.md")
	if got := readFile(t, skill); normalizeNewlines(got) != normalizeNewlines(want) {
		t.Errorf("--update-skills şablonla değiştirmeli:\n%s", got)
	}
}

func TestSyncSkillsMigratesLegacyNames(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	old := filepath.Join(root, ".agents", "skills", "ctx-ara", "SKILL.md")
	writeFile(t, old, "---\nname: ctx-ara\ndescription: x\n---\n\n# ctx-ara — ara\n\nelle yazılmış gövde\n")

	// Kuru: yalnız raporlar.
	created, _, err := syncSkills(root, false, false)
	if err != nil || !strings.Contains(strings.Join(created, "\n"), "ctx-ara -> ctx-search") {
		t.Fatalf("kuru koşum taşımayı raporlamalı: %v %v", created, err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("kuru koşum taşımamalı: %v", err)
	}

	created, _, err = syncSkills(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(old)); !os.IsNotExist(err) {
		t.Errorf("eski dizin kalmamalı (err=%v)", err)
	}
	got := readFile(t, filepath.Join(root, ".agents", "skills", "ctx-search", "SKILL.md"))
	if !strings.Contains(got, "name: ctx-search") || !strings.Contains(got, "# ctx-search — ara") ||
		!strings.Contains(got, "elle yazılmış gövde") || strings.Contains(got, "ctx-ara") {
		t.Errorf("name/başlık düzeltilmeli, gövde korunmalı:\n%s", got)
	}
	if !strings.Contains(strings.Join(created, "\n"), "eski ad tasindi") {
		t.Errorf("taşıma raporlanmalı: %v", created)
	}
}

func TestSyncSkillsLegacyAndNewBothExist(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	oldFile := filepath.Join(root, ".agents", "skills", "ctx-kapat", "SKILL.md")
	newFile := filepath.Join(root, ".agents", "skills", "ctx-close", "SKILL.md")
	writeFile(t, oldFile, "eski\n")
	writeFile(t, newFile, "yeni\n")
	_, skipped, err := syncSkills(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, oldFile) != "eski\n" || readFile(t, newFile) != "yeni\n" {
		t.Error("ikisi de varken hiçbirine dokunulmamalı")
	}
	if !strings.Contains(strings.Join(skipped, "\n"), "ikisi de var") {
		t.Errorf("rapor beklenir: %v", skipped)
	}
}
