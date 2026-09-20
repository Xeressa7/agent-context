package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kurulum güncellemesi sahte ev + yerel origin ile test edilir: ağ yok,
// çıktıda hash/time LEAK'i yok — sürüm dizgileri + commit sayısı var.

func updateArea(t *testing.T, installed, remote string) (proj string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	proj = filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHome(t, home)
	kok := filepath.Join(home, ".agent-context")
	uzak := filepath.Join(root, "uzak")
	if err := os.MkdirAll(uzak, 0o755); err != nil {
		t.Fatal(err)
	}
	gitOK(t, uzak, "init", "--quiet")
	writeFile(t, filepath.Join(uzak, "VERSION"), installed+"\n")
	writeFile(t, filepath.Join(uzak, "x.txt"), "x\n")
	gitOK(t, uzak, "add", "-A")
	gitOK(t, uzak, "commit", "-qm", "kurulu surum")
	gitOK(t, root, "clone", "--quiet", uzak, kok)
	if remote != installed {
		writeFile(t, filepath.Join(uzak, "VERSION"), remote+"\n")
		gitOK(t, uzak, "add", "-A")
		gitOK(t, uzak, "commit", "-qm", "yeni surum")
	}
	return proj
}

func TestSelfUpdateCheckCurrent(t *testing.T) {
	updateArea(t, "9.9.8", "9.9.8")
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{Check: true}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "güncel (sürüm 9.9.8)") {
		t.Errorf("çıktı %q", out.String())
	}
}

func TestSelfUpdateCheckBehind(t *testing.T) {
	updateArea(t, "9.9.8", "9.9.9")
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{Check: true}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "güncelleme var: sürüm 9.9.8 → 9.9.9 (1 commit)") {
		t.Errorf("çıktı %q", got)
	}
}

func TestSelfUpdateApplies(t *testing.T) {
	updateArea(t, "9.9.8", "9.9.9")
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// TrimSpace: git checkout Windows'ta CRLF yazabilir (autocrlf).
	if got := strings.TrimSpace(readFile(t, filepath.Join(home, ".agent-context", "VERSION"))); got != "9.9.9" {
		t.Errorf("VERSION çekilmedi: %q", got)
	}
	if !strings.Contains(out.String(), "güncellendi: sürüm 9.9.8 → 9.9.9") {
		t.Errorf("çıktı %q", out.String())
	}
}

func TestSelfUpdateRefusesDirty(t *testing.T) {
	updateArea(t, "9.9.8", "9.9.9")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".agent-context", "kirli.txt"), "elle\n")
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{}); code != 1 {
		t.Errorf("kirli exit = %d, 1 olmalı: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "kirli (1 dosya)") {
		t.Errorf("çıktı %q", out.String())
	}
	if got := readFile(t, filepath.Join(home, ".agent-context", "VERSION")); got != "9.9.8\n" {
		t.Errorf("kirli ağaçta dosya değişmemeli: %q", got)
	}
}

func TestSelfUpdateNoGit(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	fakeHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".agent-context"), 0o755); err != nil {
		t.Fatal(err)
	}
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{}); code != 2 {
		t.Errorf("exit = %d, 2 olmalı", code)
	}
	if got := out.String(); !strings.HasPrefix(got, "HATA: kurulum bir git deposu değil: ") {
		t.Errorf("çıktı %q", got)
	}
}

func TestSelfUpdateNoUpstream(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	fakeHome(t, home)
	kok := filepath.Join(home, ".agent-context")
	if err := os.MkdirAll(kok, 0o755); err != nil {
		t.Fatal(err)
	}
	gitOK(t, kok, "init", "--quiet")
	writeFile(t, filepath.Join(kok, "VERSION"), "9.9.8\n")
	gitOK(t, kok, "add", "-A")
	gitOK(t, kok, "commit", "-qm", "ilk")
	env, out, _ := testEnv()
	if code := SelfUpdate(env, SelfUpdateOptions{Check: true}); code != 2 {
		t.Errorf("exit = %d, 2 olmalı", code)
	}
	if !strings.Contains(out.String(), "takibi yok") {
		t.Errorf("çıktı %q", out.String())
	}
}
