package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Harita: üretici TEK (IndexBody), tüketici çok (hook + dosya bloğu).
// Blok notun kendisi değildir — satır başına "ne var, nerede".

func indexArea(t *testing.T) (store.Context, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	mem := filepath.Join(root, "mem")
	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nrole: current\n")
	writeFile(t, filepath.Join(proj, "AGENTS.md"),
		"# A\n\n<!-- ctx:index:basla -->\neski\n<!-- ctx:index:bitir -->\n")
	writeFile(t, filepath.Join(mem, "README.md"), "# h\n")
	writeFile(t, filepath.Join(mem, "tasks", "OPEN.md"), "x\n")
	writeFile(t, filepath.Join(mem, "sessions", "CURRENT.md"), "x\n")
	writeFile(t, filepath.Join(mem, "facts", "a.md"),
		"---\ntitle: A notu\ndescription: A aciklama\nstatus: active\nupdated: 2026-08-13\n---\n\ngovde\n")
	ctx, err := store.LoadContext(proj)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, proj
}

func TestIndexBodyMap(t *testing.T) {
	ctx, _ := indexArea(t)
	body, err := IndexBody(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hafıza haritası", "1 not", "**facts/**", "`a.md`", "A aciklama"} {
		if !strings.Contains(body, want) {
			t.Errorf("harita %q içermiyor:\n%s", want, body)
		}
	}
}

func TestIndexBodyBudgetShrinks(t *testing.T) {
	ctx, _ := indexArea(t)
	full, err := IndexBody(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	// Bütçe en kısa biçimden bile küçükse not düşmez, yüksek sesle söyler.
	short, err := IndexBodyBudgeted(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(short)) <= 10 {
		t.Errorf("tavan notu da bütçenin parçası, 10 karaktere sığamaz: %q", short)
	}
	if !strings.Contains(short, "korpus bolunmeli") {
		t.Errorf("tavan aşımı açıkça yazılmalı:\n%s", short)
	}
	if len([]rune(short)) >= len([]rune(full))+500 {
		t.Errorf("kısaltma anlamsız büyümüş")
	}
}

func TestIndexWriteAndCheck(t *testing.T) {
	_, proj := indexArea(t)
	env, out, _ := testEnv()
	if code := Index(env, IndexOptions{Repo: proj, Write: true}); code != 0 {
		t.Fatalf("yazma exit = %d: %s", code, out.String())
	}
	after := readFile(t, filepath.Join(proj, "AGENTS.md"))
	if !strings.Contains(after, "`a.md`") || strings.Contains(after, "\neski\n") {
		t.Errorf("blok tazelenmedi:\n%s", after)
	}

	// İkinci koşum güncel der…
	env2, _, _ := testEnv()
	if code := Index(env2, IndexOptions{Repo: proj, Check: true}); code != 0 {
		t.Errorf("güncel blokta kontrol 0 dönmeli")
	}
	// …açıklama değişince bayat.
	mem := filepath.Join(proj, "..", "mem")
	writeFile(t, filepath.Join(mem, "facts", "a.md"),
		"---\ntitle: A notu\ndescription: YENI aciklama\nstatus: active\nupdated: 2026-08-13\n---\n\ngovde\n")
	env3, out3, _ := testEnv()
	if code := Index(env3, IndexOptions{Repo: proj, Check: true}); code != 1 {
		t.Errorf("bayat blokta kontrol 1 dönmeli, %d: %s", code, out3.String())
	}
}

func TestReplaceBlockKeepsEndings(t *testing.T) {
	crlf := "a\r\n<!-- ctx:index:basla -->\r\neski\r\n<!-- ctx:index:bitir -->\r\nb\r\n"
	updated, ok := replaceBlock(crlf, "yeni")
	if !ok {
		t.Fatal("işaretler bulunamadı")
	}
	if !strings.Contains(updated, "\r\n") || strings.Contains(updated, "\n\n") {
		t.Errorf("satır sonu korunmalı: %q", updated)
	}
	if _, ok := replaceBlock("işaretsiz metin", "yeni"); ok {
		t.Error("işaretsiz dosyada false dönmeli")
	}
}

// Bileşen süzgeci: yalnız harita daralır, yalnız istenince, ve daraldığı söylenir.
func TestIndexComponentsFilter(t *testing.T) {
	ctx, proj := indexArea(t)
	mem := ctx.MemoryPath
	note := func(name, components string) {
		writeFile(t, filepath.Join(mem, "pitfalls", name),
			"---\ntitle: "+name+"\ndescription: d\ncomponents: "+components+"\nupdated: 2026-08-13\n---\n\ngovde\n")
	}
	note("sunucu.md", "[server]")
	note("istemci.md", "[client]")
	note("ikisi.md", "[server, client]")

	manifest := "version: 1\nproject: p\nmemory_path: ../mem\nrole: current\ncomponents: [client]\n"
	writeFile(t, filepath.Join(proj, "context.yaml"), manifest)
	ctx, _ = store.LoadContext(proj)
	off, err := IndexBody(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(off, "`sunucu.md`") || strings.Contains(off, "bileşen:") {
		t.Errorf("index_components yokken harita daralmamalı:\n%s", off)
	}

	writeFile(t, filepath.Join(proj, "context.yaml"), manifest+"index_components: true\n")
	ctx, _ = store.LoadContext(proj)
	on, err := IndexBody(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"`istemci.md`", "`ikisi.md`", "`a.md`", "bileşen: client", "3 not",
		"Başka bileşenlerin 1 notu"} {
		if !strings.Contains(on, want) {
			t.Errorf("süzülmüş harita %q içermiyor:\n%s", want, on)
		}
	}
	if strings.Contains(on, "`sunucu.md`") {
		t.Errorf("başka bileşenin notu haritada olmamalı:\n%s", on)
	}

	all, err := IndexBody(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all, "`sunucu.md`") {
		t.Errorf("--all süzgeci kaldırmalı:\n%s", all)
	}
}

// Açıkça adı verilen hedef üretilmiş dosyadır: yoksa --write onu kurar, --check
// yalnız söyler. Varsayılan AGENTS.md asla sıfırdan üretilmez.
func TestIndexGeneratesMissingTarget(t *testing.T) {
	_, proj := indexArea(t)
	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nrole: current\nindex_targets: [MEMORY-INDEX.md]\n")
	target := filepath.Join(proj, "MEMORY-INDEX.md")

	env, out, _ := testEnv()
	if code := Index(env, IndexOptions{Repo: proj, Check: true}); code != 1 {
		t.Errorf("eksik hedefte kontrol 1 dönmeli, %d: %s", code, out.String())
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("--check dosya yazmamalı")
	}

	env2, out2, _ := testEnv()
	if code := Index(env2, IndexOptions{Repo: proj, Write: true}); code != 0 {
		t.Fatalf("yazma exit = %d: %s", code, out2.String())
	}
	text := readFile(t, target)
	for _, want := range []string{indexStart, indexEnd, "`a.md`", "ELLE DÜZENLEME"} {
		if !strings.Contains(text, want) {
			t.Errorf("üretilen dosya %q içermiyor:\n%s", want, text)
		}
	}
	env3, out3, _ := testEnv()
	if code := Index(env3, IndexOptions{Repo: proj, Check: true}); code != 0 {
		t.Errorf("üretimden sonra kontrol 0 dönmeli: %s", out3.String())
	}

	// ctx close da aynı dosyayı kurar: taze klonda ilk yeni not haritayı getirir.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.LoadContext(proj)
	if err != nil {
		t.Fatal(err)
	}
	env4, _, _ := testEnv()
	refreshIndex(env4, ctx)
	if _, err := os.Stat(target); err != nil {
		t.Errorf("close tazelemesi eksik üretilmiş hedefi kurmalı: %v", err)
	}
}

func TestIndexDoesNotGenerateDefaultAgents(t *testing.T) {
	_, proj := indexArea(t)
	agents := filepath.Join(proj, "AGENTS.md")
	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	env, out, _ := testEnv()
	if code := Index(env, IndexOptions{Repo: proj, Write: true}); code != 1 {
		t.Errorf("varsayılan hedef yoksa 1 dönmeli, %d: %s", code, out.String())
	}
	if _, err := os.Stat(agents); err == nil {
		t.Error("router'sız AGENTS.md üretilmemeli")
	}
}
