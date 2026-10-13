package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// Bakım raporu: günlüğe dönmüş not üstte, düzenli not aday değil, Tarihçe'deki
// tarih ve geri-alma sayılmaz. Hiçbir dosya yazılmaz.
func TestMaintenanceRanksLogLikeNotes(t *testing.T) {
	ctx, proj := indexArea(t)
	mem := ctx.MemoryPath
	logLike := "---\ntitle: g\ndescription: d\n---\n\n## Ödeme ucu — UYGULANDI 2026-09-08\n\nx\n\n" +
		"## Kararlar (2026-10-04)\n\naynı seviye serbest (eski engel kaldırıldı)\n"
	layered := "---\ntitle: l\ndescription: d\n---\n\n## Geçerli\n\naynı seviye serbest.\n\n" +
		"## Tarihçe\n\n- 2026-10-13: engel kaldırıldı (ürün sahibi)\n\n### UYGULANDI 2026-09-08\n"
	writeFile(t, filepath.Join(mem, "decisions", "gunluk.md"), logLike)
	writeFile(t, filepath.Join(mem, "decisions", "duzenli.md"), layered)
	before := readFile(t, filepath.Join(mem, "decisions", "gunluk.md"))

	env, out, _ := testEnv()
	if code := Maintenance(env, MaintenanceOptions{Repo: proj, Top: 10}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"1 tanesi `## Geçerli`", "decisions/gunluk.md", "tarihli/durum basligi 2",
		"geri-alma ifadesi 1", "Geçerli yok"} {
		if !strings.Contains(got, want) {
			t.Errorf("rapor %q icermiyor:\n%s", want, got)
		}
	}
	if strings.Contains(got, "duzenli.md") {
		t.Errorf("Geçerli/Tarihçe düzenindeki not aday olmamali:\n%s", got)
	}
	if readFile(t, filepath.Join(mem, "decisions", "gunluk.md")) != before {
		t.Error("bakim salt okunur olmali")
	}
}

// Arama varsayılan olarak Tarihçe'yi saymaz; --tarihce sayar.
func TestSearchSkipsHistoryByDefault(t *testing.T) {
	ctx, proj := indexArea(t)
	writeFile(t, filepath.Join(ctx.MemoryPath, "decisions", "seviye.md"),
		"---\ntitle: Seviye talebi\ndescription: d\n---\n\n## Geçerli\n\nserbest\n\n"+
			"## Tarihçe\n\n- 2026-10-04: zorlukengeli vardi\n")

	env, out, _ := testEnv()
	Search(env, SearchOptions{Repo: proj, Query: "zorlukengeli", Top: 5})
	if strings.Contains(out.String(), "seviye.md") {
		t.Errorf("tarihcedeki sozcuk varsayilan aramada bulunmamali:\n%s", out.String())
	}
	env2, out2, _ := testEnv()
	Search(env2, SearchOptions{Repo: proj, Query: "zorlukengeli", Top: 5, History: true})
	if !strings.Contains(out2.String(), "seviye.md") {
		t.Errorf("--tarihce ile bulunmali:\n%s", out2.String())
	}
}
