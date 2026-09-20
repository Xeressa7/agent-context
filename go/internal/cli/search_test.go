package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// Arama sözleşmesi: alan ağırlığı, VEYA adaylığı, kapsam filtresi, determinizm.
// Skor formülünün kendisi vektörlerde kilitli (core); burada komutun
// uçtan uca davranışı: filtre, eşik, sıralama, çıktı biçimi.

func searchArea(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	mem := filepath.Join(root, "mem")
	writeFile(t, filepath.Join(proj, "context.yaml"), manifest)
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "x\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "x\n")
	writeFile(t, filepath.Join(mem, "facts", "billing.md"),
		"---\ntitle: Fatura\ndescription: Fatura aciklama\nstatus: active\nupdated: 2026-08-13\n---\n\nalakasiz govde\n")
	writeFile(t, filepath.Join(mem, "facts", "other.md"),
		"---\ntitle: Diger\ndescription: Diger aciklama\nstatus: active\nupdated: 2026-08-13\n---\n\nbilling gecen govde\n")
	return proj
}

const searchManifest = "version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n"

func TestSearchPathBeatsBody(t *testing.T) {
	env, out, _ := testEnv()
	repo := searchArea(t, searchManifest)
	if code := Search(env, SearchOptions{Repo: repo, Query: "billing", Top: 8}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("2 sonuç beklenir:\n%s", out.String())
	}
	if !strings.HasPrefix(lines[0], "facts/billing.md") {
		t.Errorf("yol isabeti govdeyi yenmeli:\n%s", out.String())
	}
	if !strings.HasPrefix(lines[1], "facts/other.md") {
		t.Errorf("ikinci sonuç other.md olmalı:\n%s", out.String())
	}
}

func TestSearchScopeFilterAndAll(t *testing.T) {
	env, _, _ := testEnv()
	repo := searchArea(t, searchManifest)
	mem := filepath.Join(repo, "..", "mem")
	writeFile(t, filepath.Join(mem, "facts", "eski.md"),
		"---\ntitle: Eski billing\ndescription: arsiv aciklama\nstatus: archived\nupdated: 2026-08-13\n---\n\ngovde\n")

	// Arşiv varsayılanda görünmez…
	env1, out1, _ := testEnv()
	Search(env1, SearchOptions{Repo: repo, Query: "billing", Top: 8})
	if strings.Contains(out1.String(), "facts/eski.md") {
		t.Errorf("arşiv not filtrelenmeli:\n%s", out1.String())
	}
	// …--all ile gelir.
	env2, out2, _ := testEnv()
	Search(env2, SearchOptions{Repo: repo, Query: "billing", Top: 8, All: true})
	if !strings.Contains(out2.String(), "facts/eski.md") {
		t.Errorf("--all arşivi getirmeli:\n%s", out2.String())
	}
	_ = env
}

func TestSearchTopAndMin(t *testing.T) {
	env, out, _ := testEnv()
	repo := searchArea(t, searchManifest)
	if code := Search(env, SearchOptions{Repo: repo, Query: "billing", Top: 1}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "--top N") {
		t.Errorf("taşma satırı beklenir:\n%s", got)
	}
	env2, out2, _ := testEnv()
	Search(env2, SearchOptions{Repo: repo, Query: "billing", Top: 8, Min: 1000})
	if got := out2.String(); !strings.Contains(got, "eslesme yok") {
		t.Errorf("yüksek eşik her şeyi elemeli:\n%s", got)
	}
}

func TestSearchErrors(t *testing.T) {
	env, out, _ := testEnv()
	if code := Search(env, SearchOptions{Repo: t.TempDir(), Query: "x", Top: 8}); code != 2 {
		t.Errorf("manifestsiz exit = %d, 2 olmalı", code)
	} else if !strings.Contains(out.String(), "HATA") {
		t.Errorf("hata stdout'a basılmalı (kanca sözleşmesi): %q", out.String())
	}
	env2, out2, _ := testEnv()
	repo := searchArea(t, searchManifest)
	if code := Search(env2, SearchOptions{Repo: repo, Query: "  ", Top: 8}); code != 2 {
		t.Errorf("boş sorgu exit = %d, 2 olmalı", code)
	} else if !strings.Contains(out2.String(), "bos sorgu") {
		t.Errorf("çıktı %q", out2.String())
	}
}
