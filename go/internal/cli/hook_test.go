package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Kancaların ilk kuralı SESSİZLİK: hafıza yoksa tek bayt basmaz, exit 0.
// Çıktı harness zarfıdır — bayt bayt sabit.

func hookArea(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	mem := filepath.Join(root, "mem")
	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n")
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "# acik\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "# devam\n")
	writeFile(t, filepath.Join(mem, "facts", "billing.md"),
		"---\ntitle: Fatura\ndescription: Fatura aciklama\nstatus: active\nupdated: 2026-08-13\n---\n\ngovde\n")
	return proj
}

func TestHookIndexPrintsMap(t *testing.T) {
	env, out, _ := testEnv()
	repo := hookArea(t)
	if code := HookIndex(env, repo); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var zarf map[string]any
	if err := json.Unmarshal([]byte(out.String()), &zarf); err != nil {
		t.Fatalf("zarf JSON olmalı: %v\n%s", err, out.String())
	}
	ic := zarf["hookSpecificOutput"].(map[string]any)
	if ic["hookEventName"] != "SessionStart" {
		t.Errorf("zarf: %v", ic)
	}
	baglam := ic["additionalContext"].(string)
	for _, want := range []string{"Hafıza haritası", "billing.md"} {
		if !strings.Contains(baglam, want) {
			t.Errorf("harita %q içermiyor", want)
		}
	}
}

func TestHookSilentWithoutMemory(t *testing.T) {
	for _, fn := range []func(Env, string) int{HookIndex, HookCaptureCheck} {
		env, out, _ := testEnv()
		if code := fn(env, t.TempDir()); code != 0 {
			t.Errorf("hafızasız projede exit 0 olmalı")
		}
		if out.String() != "" {
			t.Errorf("hafızasız projede kanca susmalı: %q", out.String())
		}
	}
}

func TestHookCaptureCheckWarns(t *testing.T) {
	repo := hookArea(t)
	// Kirli iş + karar yok = uyarı zarfı (kirli sayılması için git reposu şart).
	gitOK(t, repo, "init", "--quiet")
	gitOK(t, repo, "add", "-A")
	gitOK(t, repo, "commit", "-qm", "ilk")
	writeFile(t, filepath.Join(repo, "kirli.txt"), "kirli\n")
	env, out, _ := testEnv()
	if code := HookCaptureCheck(env, repo); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var zarf map[string]any
	if err := json.Unmarshal([]byte(out.String()), &zarf); err != nil {
		t.Fatalf("uyarı zarfı olmalı: %v\n%s", err, out.String())
	}
	if got := zarf["hookSpecificOutput"].(map[string]any)["additionalContext"].(string); !strings.Contains(got, "CAPTURE KARARI BEKLIYOR") {
		t.Errorf("zarf: %q", got)
	}
}

func writeJSONL(t *testing.T, path string, records []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(records, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHookFlushWritesDigest(t *testing.T) {
	repo := hookArea(t)
	tr := filepath.Join(t.TempDir(), "oturum.jsonl")
	writeJSONL(t, tr, []string{
		`{"type": "user", "message": {"content": "kaşe neden basılmadı"}}`,
		`{"type": "assistant", "message": {"content": [{"type": "text", "text": "hazırlık denetimine baktım"}]}}`,
		`{"type": "assistant", "message": {"content": [{"type": "tool_use", "name": "x"}]}}`,
	})
	env, _, _ := testEnv()
	if code := HookFlush(env, FlushOptions{Repo: repo, Transcript: tr, Tool: "claude", Reason: "sessionend"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	entries, err := os.ReadDir(filepath.Join(repo, "..", "mem", "sessions", ".pending"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("ham döküm yazılmalı: %v %v", entries, err)
	}
	govde := readFile(t, filepath.Join(repo, "..", "mem", "sessions", ".pending", entries[0].Name()))
	if !strings.Contains(govde, "kaşe neden") || strings.Contains(govde, "tool_use") {
		t.Errorf("yalnız düz metin alınmalı:\n%s", govde)
	}
}

func TestHookFlushSkipsShort(t *testing.T) {
	repo := hookArea(t)
	tr := filepath.Join(t.TempDir(), "kisa.jsonl")
	writeJSONL(t, tr, []string{`{"type": "user", "message": {"content": "selam"}}`})
	env, _, _ := testEnv()
	HookFlush(env, FlushOptions{Repo: repo, Transcript: tr, Tool: "claude", Reason: "sessionend"})
	if _, err := os.Stat(filepath.Join(repo, "..", "mem", "sessions", ".pending")); !os.IsNotExist(err) {
		t.Errorf("tek turluk oturum yazılmamalı")
	}
}

// Eski ham dökümler kanca her koştuğunda budanır; yenisi ve süresi dolmamışlar kalır.
func TestHookFlushPrunesOldDigests(t *testing.T) {
	repo := hookArea(t)
	pending := filepath.Join(repo, "..", "mem", "sessions", ".pending")
	writeFile(t, filepath.Join(pending, "claude-eski.md"), "eski\n")
	writeFile(t, filepath.Join(pending, "claude-taze.md"), "taze\n")
	now := time.Now()
	os.Chtimes(filepath.Join(pending, "claude-eski.md"), now.AddDate(0, 0, -10), now.AddDate(0, 0, -10))
	os.Chtimes(filepath.Join(pending, "claude-taze.md"), now.AddDate(0, 0, -2), now.AddDate(0, 0, -2))

	tr := filepath.Join(t.TempDir(), "oturum.jsonl")
	writeJSONL(t, tr, []string{
		`{"type": "user", "message": {"content": "bir"}}`,
		`{"type": "assistant", "message": {"content": [{"type": "text", "text": "iki"}]}}`,
	})
	env, _, _ := testEnv()
	HookFlush(env, FlushOptions{Repo: repo, Transcript: tr, Tool: "claude", Now: now})
	if _, err := os.Stat(filepath.Join(pending, "claude-eski.md")); !os.IsNotExist(err) {
		t.Error("7 günden eski döküm silinmeli")
	}
	if _, err := os.Stat(filepath.Join(pending, "claude-taze.md")); err != nil {
		t.Error("taze döküm kalmalı")
	}
}
