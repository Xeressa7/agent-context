package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// Doctor kurulum sağlığına bakar, notlara değil (onlar Check'in işi).
// Uyarılar çıkışı BOZMAZ — kırmızıda kalan sağlık kontrolü okunmaz olur.

func doctorArea(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	mem := filepath.Join(root, "mem")
	writeFile(t, filepath.Join(proj, "context.yaml"), manifest)
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "# acik\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "# devam\n")
	return proj
}

const doctorManifest = "version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n"

func TestDoctorHealthy(t *testing.T) {
	env, out, _ := testEnv()
	repo := doctorArea(t, doctorManifest)
	if code := Doctor(env, repo); code != 0 {
		t.Fatalf("exit = %d:\n%s", code, out.String())
	}
	if got := out.String(); !strings.Contains(got, "dogrulamadan gecti") {
		t.Errorf("çıktı %q", got)
	}
}

// Kapsam alanı `scope`'tur (init şablonu onu yazar); emekli `role` yalnız geri
// uyum için kabul edilir. İkisi de yoksa kurulum eksiktir.
func TestDoctorScopeField(t *testing.T) {
	for _, c := range []struct {
		ad, alan string
		kod      int
	}{
		{"scope", "scope: [current]\n", 0},
		{"emekli role", "role: current\n", 0},
		{"hicbiri", "", 1},
	} {
		env, out, _ := testEnv()
		repo := doctorArea(t, "version: 1\nproject: p\nmemory_path: ../mem\n"+c.alan)
		if code := Doctor(env, repo); code != c.kod {
			t.Errorf("%s: exit = %d, %d olmalı:\n%s", c.ad, code, c.kod, out.String())
		}
	}
}

func TestDoctorNoManifest(t *testing.T) {
	env, out, _ := testEnv()
	if code := Doctor(env, t.TempDir()); code != 1 {
		t.Errorf("manifestsiz exit = %d, 1 olmalı", code)
	} else if !strings.Contains(out.String(), "context.yaml bulundu (None)") &&
		!strings.Contains(out.String(), "bulundu") {
		t.Errorf("çıktı %q", out.String())
	}
}

func TestDoctorManualWarnsWithoutFailing(t *testing.T) {
	env, out, _ := testEnv()
	repo := doctorArea(t, doctorManifest+"auto_index: false\n")
	if code := Doctor(env, repo); code != 0 {
		t.Fatalf("uyarı çıkışı bozmamalı, exit = %d:\n%s", code, out.String())
	}
	if got := out.String(); !strings.Contains(got, "otomatik harita kapalı") {
		t.Errorf("manuel uyarısı beklenir:\n%s", got)
	}
}

func TestDoctorMissingTargetWarns(t *testing.T) {
	env, out, _ := testEnv()
	repo := doctorArea(t, doctorManifest)
	if code := Doctor(env, repo); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// AGENTS.md yok: varsayılan hedef uyarı üretir, hata değil.
	if got := out.String(); !strings.Contains(got, "index hedefi yok") {
		t.Errorf("hedef uyarısı beklenir:\n%s", got)
	}
}
