package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Tüketici kaydı: liste hafızanın ctx.yaml'ında (hafızaya GÖRELİ), keşif
// derinliği 1, kayıt bootstrap'ta, uyarı doctor'da.

func consumerArea(t *testing.T) (mem, catiManifest string) {
	t.Helper()
	root := t.TempDir()
	roof := filepath.Join(root, "roof")
	mem = filepath.Join(roof, "mem")
	writeFile(t, filepath.Join(mem, "ctx.yaml"),
		"# ayar\npii_profiles: [tr]\nconsumers: [.., ../sunc]\n")
	mkRepo := func(ad, mp string) {
		d := filepath.Join(roof, ad)
		writeFile(t, filepath.Join(d, "context.yaml"),
			"version: 1\nproject: "+ad+"\nmemory_path: "+mp+"\nrole: current\n")
	}
	mkRepo("cati", "../mem")
	mkRepo("sunc", "../mem")
	mkRepo("yeni", "../mem")
	if err := os.MkdirAll(filepath.Join(roof, "baska"), 0o755); err != nil {
		t.Fatal(err)
	}
	return mem, filepath.Join(roof, "cati", "context.yaml")
}

func loadCtx(t *testing.T, manifest string) store.Context {
	t.Helper()
	ctx, err := store.LoadContext(filepath.Dir(manifest))
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestUndiscoveredFindsUnlisted(t *testing.T) {
	mem, mevcut := consumerArea(t)
	ctx := loadCtx(t, mevcut)
	got := undiscoveredConsumers(ctx)
	if len(got) != 1 || !strings.HasSuffix(got[0], filepath.Join("yeni", "context.yaml")) {
		t.Errorf("kayıtsız = %v, [yeni] olmalı", got)
	}
	_ = mem
}

func TestUndiscoveredEmptyWhenListed(t *testing.T) {
	mem, mevcut := consumerArea(t)
	cfg := readFile(t, filepath.Join(mem, "ctx.yaml"))
	writeFile(t, filepath.Join(mem, "ctx.yaml"),
		strings.Replace(cfg, "[.., ../sunc]", "[.., ../sunc, ../yeni]", 1))
	if got := undiscoveredConsumers(loadCtx(t, mevcut)); len(got) != 0 {
		t.Errorf("hepsi listedeyken %v", got)
	}
}

func TestDeadEntriesFlagRoofRelative(t *testing.T) {
	mem, _ := consumerArea(t)
	roof := filepath.Dir(mem)
	writeFile(t, filepath.Join(roof, "context.yaml"),
		"version: 1\nproject: cati\nmemory_path: mem\nrole: current\n")
	writeFile(t, filepath.Join(mem, "ctx.yaml"), "consumers: [.., ../sunc, server]\n")
	// `..` çözülür (çatı manifesti var), `server` ölüdür.
	if got := deadConsumerEntries(mem); len(got) != 1 || got[0] != "server" {
		t.Errorf("ölü = %v", got)
	}
}

func TestRegisterConsumer(t *testing.T) {
	mem, _ := consumerArea(t)
	repo := filepath.Join(filepath.Dir(mem), "yeni")

	if got := registerConsumer(mem, repo, true); got[0] != "+ consumers: [../yeni] eklendi" {
		t.Errorf("rapor %q", got)
	}
	after := norm(readFile(t, filepath.Join(mem, "ctx.yaml")))
	if !strings.Contains(after, "consumers: [.., ../sunc, ../yeni]") {
		t.Errorf("satır genişlemedi:\n%s", after)
	}
	if !strings.HasPrefix(after, "# ayar\npii_profiles: [tr]\n") {
		t.Errorf("el düzeni bozuldu:\n%s", after)
	}
	if got := registerConsumer(mem, repo, true); got[0] != "= consumers (kayıtlı)" {
		t.Errorf("ikinci koşum %q", got)
	}
	// Kuru koşum yazmaz ama niyeti söyler.
	writeFile(t, filepath.Join(mem, "ctx.yaml"), "pii_profiles: [tr]\n")
	if got := registerConsumer(mem, repo, false); got[0] != "+ consumers: [../yeni] eklendi" {
		t.Errorf("kuru rapor %q", got)
	}
	if got := norm(readFile(t, filepath.Join(mem, "ctx.yaml"))); strings.Contains(got, "consumers") {
		t.Errorf("kuru koşum yazmış:\n%s", got)
	}
}

func TestRegisterConsumerUnknownForm(t *testing.T) {
	mem, _ := consumerArea(t)
	blok := "pii_profiles: [tr]\nconsumers:\n  - ..\n  - ../sunc\n"
	writeFile(t, filepath.Join(mem, "ctx.yaml"), blok)
	repo := filepath.Join(filepath.Dir(mem), "yeni")
	got := registerConsumer(mem, repo, true)
	if !strings.HasPrefix(got[0], "! consumers biçimi tanınmadı") {
		t.Errorf("rapor %q", got)
	}
	if after := norm(readFile(t, filepath.Join(mem, "ctx.yaml"))); after != blok {
		t.Errorf("bilinmeyen biçime dokunulmamalı:\n%s", after)
	}
}

func TestAddConsumerLineCRLF(t *testing.T) {
	in := "pii_profiles: [tr]\r\nconsumers: [..]\r\n"
	got, st := addConsumerLine(normalizeNewlines(in), "../yeni")
	if st != "eklendi" {
		t.Fatalf("durum %q", st)
	}
	// Ham CRLF normalize edilir; platform satır sonu yazımda konur.
	if !strings.Contains(got, "consumers: [.., ../yeni]") {
		t.Errorf("çıktı %q", got)
	}
	if _, st := addConsumerLine(got, "../yeni"); st != "kayitli" {
		t.Errorf("kayıtlı durum %q", st)
	}
}
