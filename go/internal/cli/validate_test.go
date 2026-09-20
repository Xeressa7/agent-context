package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// Doğrulama motoru: `ctx validate`, `ctx doctor` ve pre-commit AYNI Check'i
// koşar — mantık iki kez yazılmaz, test de bir kez yazılır.

func validNote(title, extra string) string {
	return "---\ntitle: " + title + "\ndescription: D aciklama\n" + extra +
		"updated: 2026-08-13\n---\n\ngovde metni\n"
}

func validArea(t *testing.T) string {
	t.Helper()
	mem := t.TempDir()
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "# acik\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "# devam\n")
	return mem
}

func findingsByPath(fs []Finding) map[string][]string {
	out := map[string][]string{}
	for _, f := range fs {
		out[f.Path] = append(out[f.Path], f.Level+": "+f.Msg)
	}
	return out
}

func TestCheckClean(t *testing.T) {
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "a.md"), validNote("A", "status: active\n"))
	fs, err := Check(mem, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Errorf("temiz korpusta bulgu: %v", fs)
	}
}

func TestCheckStructure(t *testing.T) {
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "fmsiz.md"), "frontmatter yok\n")
	writeFile(t, filepath.Join(mem, "facts", "eksik.md"),
		"---\ntitle: E\n---\n\ngovde\n")
	writeFile(t, filepath.Join(mem, "facts", "durum.md"),
		"---\ntitle: D\ndescription: x\nstatus: uydurma\nupdated: 2026-08-13\n---\n\ngovde\n")
	writeFile(t, filepath.Join(mem, "facts", "eski-eksen.md"),
		"---\ntitle: L\ndescription: x\nlifecycle: current\nupdated: 2026-08-13\n---\n\ngovde\n")
	writeFile(t, filepath.Join(mem, "facts", "kotueksen.md"),
		"---\ntitle: K\ndescription: x\nlifecycle: uydurma\nupdated: 2026-08-13\n---\n\ngovde\n")
	writeFile(t, filepath.Join(mem, "facts", "t1.md"), validNote("Ayni", ""))
	writeFile(t, filepath.Join(mem, "facts", "t2.md"), validNote("Ayni", ""))

	fs, err := Check(mem, false)
	if err != nil {
		t.Fatal(err)
	}
	byPath := findingsByPath(fs)
	check := func(path, substr string) {
		t.Helper()
		for _, m := range byPath[path] {
			if strings.Contains(m, substr) {
				return
			}
		}
		t.Errorf("%s: %q yok %v", path, substr, byPath[path])
	}
	check("facts/fmsiz.md", "frontmatter")
	check("facts/eksik.md", "description")
	check("facts/durum.md", "gecersiz status")
	check("facts/eski-eksen.md", "eskidi")
	check("facts/kotueksen.md", "gecersiz lifecycle")
	if n := len(byPath["facts/t2.md"]); n == 0 {
		t.Errorf("başlık tekrarı uyarılmalı")
	}
}

func TestCheckLinks(t *testing.T) {
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "a.md"),
		"---\ntitle: A\ndescription: x\nstatus: active\nupdated: 2026-08-13\n---\n\n"+
			"[kirik](yok.md) ve [[olmayan-not]] ve [saglam](b.md)\n")
	writeFile(t, filepath.Join(mem, "facts", "b.md"), validNote("B", ""))

	fs, _ := Check(mem, false)
	byPath := findingsByPath(fs)
	found := func(substr string) bool {
		for _, m := range byPath["facts/a.md"] {
			if strings.Contains(m, substr) {
				return true
			}
		}
		return false
	}
	if !found("kirik markdown link") {
		t.Errorf("kırık link yakalanmalı: %v", byPath["facts/a.md"])
	}
	if !found("cozumlenmeyen wiki-link") {
		t.Errorf("çözülemeyen wiki-link uyarılmalı: %v", byPath["facts/a.md"])
	}
}

func TestCheckReviewDueOnlyActive(t *testing.T) {
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "eski.md"),
		"---\ntitle: E\ndescription: x\nstatus: active\nupdated: 2020-01-01\n---\n\ngovde\n")
	writeFile(t, filepath.Join(mem, "facts", "arsiv.md"),
		"---\ntitle: R\ndescription: x\nstatus: archived\nupdated: 2020-01-01\n---\n\ngovde\n")

	fs, _ := Check(mem, false)
	byPath := findingsByPath(fs)
	has := func(path, substr string) bool {
		for _, m := range byPath[path] {
			if strings.Contains(m, substr) {
				return true
			}
		}
		return false
	}
	if !has("facts/eski.md", "review-due") {
		t.Errorf("aktif eski not review-due almalı: %v", byPath)
	}
	if has("facts/arsiv.md", "review-due") {
		t.Errorf("arşiv not tazelik sormamalı: %v", byPath["facts/arsiv.md"])
	}
}

func TestCheckFullSweep(t *testing.T) {
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "temiz.md"), validNote("T", ""))
	fs, _ := Check(mem, false)
	for _, f := range fs {
		if strings.Contains(f.Msg, "PII") || strings.Contains(f.Msg, "gizli") {
			t.Fatalf("hızlı modda PII bakılmamalı: %v", f)
		}
	}
	writeFile(t, filepath.Join(mem, "facts", "sızıntı.md"),
		"---\ntitle: S\ndescription: x\nstatus: active\nupdated: 2026-08-13\n---\n\n"+
			"iletisim ornek@example.com ve api_key: ABCDEF1234567890abcdef\n")
	full, _ := Check(mem, true)
	msgs := []string{}
	for _, f := range full {
		msgs = append(msgs, f.Msg)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "e-posta") {
		t.Errorf("e-posta yakalanmalı: %v", msgs)
	}
	if !strings.Contains(joined, "gizli anahtar") {
		t.Errorf("atanmış secret yakalanmalı: %v", msgs)
	}
}

func TestCheckTaskState(t *testing.T) {
	// OPEN yok → indeks hatası.
	bare := validArea(t)
	writeFile(t, filepath.Join(bare, "tasks", "is.md"),
		"---\ntitle: Is\ndescription: x\nstatus: active\nupdated: 2026-08-13\n---\n\nyapilacak\n")
	if fs, _ := Check(bare, false); !containsMsg(fs, "listelenmiyor") {
		t.Errorf("indekssiz aktif task yakalanmalı: %v", fs)
	}

	root := validArea(t)
	writeFile(t, filepath.Join(root, "tasks", "is.md"),
		"---\ntitle: Is\ndescription: x\nstatus: active\nupdated: 2026-08-13\n---\n\n- [x] bitti sayma\n")
	writeFile(t, filepath.Join(root, "tasks", "hayalet.md"),
		"---\ntitle: H\ndescription: x\nstatus: archived\nupdated: 2026-08-13\n---\n\nkapandi\n")
	writeFile(t, filepath.Join(root, "tasks", "OPEN.md"),
		"# acik\n- [Is](is.md)\n- [H](hayalet.md)\n")

	fs, _ := Check(root, false)
	byPath := findingsByPath(fs)
	closed := false
	for path, msgs := range byPath {
		for _, m := range msgs {
			if strings.HasPrefix(path, "tasks/is.md:") && strings.Contains(m, "kapanmis madde") {
				closed = true
			}
		}
	}
	if !closed {
		t.Errorf("işaretli madde yakalanmalı: %v", byPath)
	}
	has := func(path, substr string) bool {
		for _, m := range byPath[path] {
			if strings.Contains(m, substr) {
				return true
			}
		}
		return false
	}
	if !has("tasks/hayalet.md", "listeleniyor") {
		t.Errorf("arşiv task indekste kalmamalı: %v", byPath)
	}
}

func containsMsg(fs []Finding, substr string) bool {
	for _, f := range fs {
		if strings.Contains(f.Msg, substr) {
			return true
		}
	}
	return false
}

func TestValidateReport(t *testing.T) {
	env, out, _ := testEnv()
	mem := validArea(t)
	writeFile(t, filepath.Join(mem, "facts", "a.md"), validNote("A", ""))
	if code := Validate(env, ValidateOptions{Memory: mem}); code != 0 {
		t.Fatalf("exit = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "bulgu yok") {
		t.Errorf("çıktı %q", out.String())
	}
	env2, out2, _ := testEnv()
	if code := Validate(env2, ValidateOptions{Memory: mem, Full: true}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "PII kapsami") {
		t.Errorf("tam mod kapsamı yazmalı: %q", out2.String())
	}
}
