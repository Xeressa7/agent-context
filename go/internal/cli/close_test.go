package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Capture kararı: yargı ajanın, disiplin aracın. "Durable bulgu yok" geçerli
// karar, karar vermemek geçersiz. Kayıt sessions/capture-log.jsonl'dedir.

func closeArea(t *testing.T) (repo, mem string) {
	t.Helper()
	root := t.TempDir()
	repo = filepath.Join(root, "proj")
	mem = filepath.Join(root, "mem")
	gitOK(t, root, "init", "--quiet", "proj")
	writeFile(t, filepath.Join(repo, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n")
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "# acik\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "# devam\n")
	gitOK(t, repo, "add", "-A")
	gitOK(t, repo, "commit", "-qm", "ilk")
	return repo, mem
}

func TestClosePendingDecision(t *testing.T) {
	env, out, _ := testEnv()
	repo, _ := closeArea(t)
	writeFile(t, filepath.Join(repo, "kirli.txt"), "is\n")
	if code := Close(env, CloseOptions{Repo: repo}); code != 1 {
		t.Fatalf("kararsız exit = %d, 1 olmalı", code)
	}
	if got := out.String(); !strings.Contains(got, "KARAR BEKLENIYOR") {
		t.Errorf("çıktı %q", got)
	}
}

func TestCloseNone(t *testing.T) {
	env, out, _ := testEnv()
	repo, mem := closeArea(t)
	writeFile(t, filepath.Join(repo, "kirli.txt"), "is\n")
	if code := Close(env, CloseOptions{Repo: repo, None: true, NoneReason: "deneme"}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	log := readFile(t, filepath.Join(mem, "sessions", "capture-log.jsonl"))
	for _, want := range []string{`"karar": "none"`, `"gerekce": "deneme"`} {
		if !strings.Contains(log, want) {
			t.Errorf("kayıt %q içermiyor: %s", want, log)
		}
	}
	// Karar sonrası kontrol susar.
	env2, _, _ := testEnv()
	if code := Close(env2, CloseOptions{Repo: repo, Check: true}); code != 0 {
		t.Errorf("kararlı kontrol 0 dönmeli")
	}
}

func TestCloseNotes(t *testing.T) {
	env, _, _ := testEnv()
	repo, mem := closeArea(t)
	writeFile(t, filepath.Join(mem, "facts", "a.md"), "---\ntitle: A\ndescription: x\n---\n\ngovde\n")

	if code := Close(env, CloseOptions{Repo: repo, Notes: []string{"yok.md"}}); code != 2 {
		t.Errorf("olmayan not exit = %d, 2 olmalı", code)
	}
	env2, out2, _ := testEnv()
	if code := Close(env2, CloseOptions{Repo: repo, Notes: []string{"facts/a.md"}}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out2.String())
	}
	if got := readFile(t, filepath.Join(mem, "sessions", "capture-log.jsonl")); !strings.Contains(got, `"karar": "captured"`) {
		t.Errorf("kayıt:\n%s", got)
	}
}

func TestCloseCheckFresh(t *testing.T) {
	env, out, _ := testEnv()
	repo, _ := closeArea(t)
	// Temiz ağaç + kayıtsız: karar güncel sayılır.
	if code := Close(env, CloseOptions{Repo: repo, Check: true}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "guncel") {
		t.Errorf("çıktı %q", out.String())
	}
}

func TestWorkStateStaysInside(t *testing.T) {
	// Çatı repo değilse git araması EVE TIRMANMAMALI (2026-08-13 dersi):
	// alt repolar toplanır, üstteki kirli dosya sayılmaz.
	root := t.TempDir()
	gitOK(t, root, "init", "--quiet")
	writeFile(t, filepath.Join(root, "ev-copu.txt"), "ust kir\n")
	roof := filepath.Join(root, "cati")
	for _, ad := range []string{"a", "b"} {
		d := filepath.Join(roof, ad)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		gitOK(t, d, "init", "--quiet")
		writeFile(t, filepath.Join(d, "dosya.txt"), "x\n")
		gitOK(t, d, "add", "-A")
		gitOK(t, d, "commit", "-qm", "ilk")
		writeFile(t, filepath.Join(d, "kirli.txt"), "is\n")
	}

	name, head, dirty := workState(roof, roof)
	if name != "cati" {
		t.Errorf("repo = %q, ev dizinine tırmandı", name)
	}
	if dirty != 2 {
		t.Errorf("kirli = %d, alt repoların 2 dosyası toplanmalı (ev copu sayılmamalı)", dirty)
	}
	if !strings.Contains(head, "a@") || !strings.Contains(head, "b@") {
		t.Errorf("parmak izi alt repolardan gelmeli: %q", head)
	}
}
