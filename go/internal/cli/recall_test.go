package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

var isolatedTemp = map[string]bool{}

func recallNote(t *testing.T, mem, rel, body string) {
	t.Helper()
	writeFile(t, filepath.Join(mem, filepath.FromSlash(rel)),
		"---\ntitle: "+rel+"\ndescription: "+rel+" aciklama\nupdated: 2026-10-13\n---\n\n"+body+"\n")
}

func runRecall(t *testing.T, proj, session, file string) string {
	t.Helper()
	// The per-session "already shown" record lives in the OS temp dir; a test
	// must not inherit it from an earlier run.
	if !isolatedTemp[t.Name()] {
		dir := t.TempDir()
		for _, k := range []string{"TMP", "TEMP", "TMPDIR"} {
			t.Setenv(k, dir)
		}
		isolatedTemp[t.Name()] = true
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id": session, "cwd": proj,
		"tool_input": map[string]any{"file_path": file},
	})
	env, out, _ := testEnv()
	env.In = strings.NewReader(string(payload))
	if code := HookRecall(env, ""); code != 0 {
		t.Fatalf("hook exit = %d", code)
	}
	return out.String()
}

// Dosyayı anan not gelir; yol eşleşmesi ad eşleşmesinden önce, aynı oturumda
// aynı not ikinci kez gelmez, ilgisiz dosya sessizdir.
func TestRecallByFile(t *testing.T) {
	ctx, proj := indexArea(t)
	mem := ctx.MemoryPath
	recallNote(t, mem, "facts/hakedis.md", "Kod: `app/services/seviye_talebi_service.py` (kapı).")
	recallNote(t, mem, "pitfalls/seviye-ad.md", "`seviye_talebi_service.py:120` yarisi.")
	recallNote(t, mem, "facts/ilgisiz.md", "`baska.py`")

	file := filepath.Join(proj, "backend", "app", "services", "seviye_talebi_service.py")
	out := runRecall(t, proj, "s1", file)
	var env struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("zarf JSON degil: %v\n%s", err, out)
	}
	got := env.HookSpecificOutput.AdditionalContext
	if env.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Errorf("olay = %q", env.HookSpecificOutput.HookEventName)
	}
	hak, ad := strings.Index(got, "hakedis.md"), strings.Index(got, "seviye-ad.md")
	if hak < 0 || ad < 0 || hak > ad {
		t.Errorf("yol eslesmesi once, ad eslesmesi sonra gelmeli:\n%s", got)
	}
	if strings.Contains(got, "ilgisiz.md") {
		t.Errorf("dosyayi anmayan not gelmemeli:\n%s", got)
	}
	if !strings.Contains(got, "../mem/facts/hakedis.md") {
		t.Errorf("yol repoya gore verilmeli:\n%s", got)
	}

	if again := runRecall(t, proj, "s1", file); again != "" {
		t.Errorf("ayni oturumda tekrar gelmemeli:\n%s", again)
	}
	if other := runRecall(t, proj, "s2", file); other == "" {
		t.Error("baska oturumda yeniden gelmeli")
	}
	if none := runRecall(t, proj, "s3", filepath.Join(proj, "yok.py")); none != "" {
		t.Errorf("not anmayan dosyada sessiz olmali:\n%s", none)
	}
	if memFile := runRecall(t, proj, "s4", filepath.Join(mem, "facts", "hakedis.md")); memFile != "" {
		t.Errorf("hafiza dosyasinin kendisi icin not gelmemeli:\n%s", memFile)
	}
}

// Genel ad (conftest.py) ve çok nota dağılmış ad hiçbir yeri göstermez; yolla
// anılırsa yine gelir. Katalog not yalnız yolla bulunur.
func TestRecallGenericAndCatalog(t *testing.T) {
	ctx, proj := indexArea(t)
	mem := ctx.MemoryPath
	recallNote(t, mem, "facts/c1.md", "`conftest.py`")
	recallNote(t, mem, "facts/c2.md", "`backend/tests/conftest.py`")
	for _, n := range []string{"d1", "d2", "d3", "d4"} {
		recallNote(t, mem, "facts/"+n+".md", "`ortak.py`")
	}
	var catalog []string
	for i := 0; i < recallCatalog+1; i++ {
		catalog = append(catalog, "`f"+itoa(i)+".py`")
	}
	recallNote(t, mem, "decisions/havuz.md", strings.Join(catalog, " "))

	got := runRecall(t, proj, "g", filepath.Join(proj, "backend", "tests", "conftest.py"))
	if !strings.Contains(got, "c2.md") || strings.Contains(got, "c1.md") {
		t.Errorf("genel ad yalniz yolla eslesmeli:\n%s", got)
	}
	if got := runRecall(t, proj, "g", filepath.Join(proj, "ortak.py")); got != "" {
		t.Errorf("dort nota dagilan ad sessiz kalmali:\n%s", got)
	}
	if got := runRecall(t, proj, "g", filepath.Join(proj, "f7.py")); got != "" {
		t.Errorf("katalog not ad ile gelmemeli:\n%s", got)
	}
}

func TestRecallOffAndBadInput(t *testing.T) {
	ctx, proj := indexArea(t)
	recallNote(t, ctx.MemoryPath, "facts/x.md", "`ozel_dosya.py`")
	env, out, _ := testEnv()
	env.In = strings.NewReader("bozuk")
	if HookRecall(env, proj) != 0 || out.Len() != 0 {
		t.Error("bozuk girdi sessiz 0 olmali")
	}
	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nauto_recall: false\n")
	if got := runRecall(t, proj, "o", filepath.Join(proj, "ozel_dosya.py")); got != "" {
		t.Errorf("auto_recall: false iken sessiz olmali:\n%s", got)
	}
}

// Son kademe dosya adları: hiçbir not düşmez, açıklama kırpıntısı kalmaz.
func TestIndexNamesOnlyTier(t *testing.T) {
	ctx, proj := indexArea(t)
	for i := 0; i < 40; i++ {
		recallNote(t, ctx.MemoryPath, "facts/uzun-not-"+itoa(i)+".md", "govde")
	}
	ctx, _ = store.LoadContext(proj)
	body, err := IndexBodyBudgeted(ctx, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "yalniz dosya adlarindan") || !strings.Contains(body, "uzun-not-39") ||
		!strings.Contains(body, "**facts/** (41):") {
		t.Errorf("ad kademesi bekleniyordu:\n%s", body)
	}
	if strings.Contains(body, "aciklama") && !strings.Contains(body, "aciklama ve tam metin") {
		t.Errorf("ad kademesinde aciklama olmamali:\n%s", body)
	}

	writeFile(t, filepath.Join(proj, "context.yaml"),
		"version: 1\nproject: p\nmemory_path: ../mem\nindex_style: adlar\n")
	ctx, _ = store.LoadContext(proj)
	forced, _ := IndexBodyBudgeted(ctx, 0)
	if !strings.Contains(forced, "**facts/** (41):") {
		t.Errorf("index_style: adlar bütçeden bağımsız ad kademesi vermeli:\n%s", forced)
	}
}

// Aynı yolu dörtten çok not ansa da yol isabetlidir: en odaklı (en az dosya
// anan) iki not gelir, hiçbiri susturulmaz.
func TestRecallManyPathMatchesKeepsMostSpecific(t *testing.T) {
	ctx, proj := indexArea(t)
	mem := ctx.MemoryPath
	recallNote(t, mem, "decisions/odakli.md", "`app/repositories/beton.py`")
	for _, n := range []string{"g1", "g2", "g3"} {
		recallNote(t, mem, "facts/"+n+".md", "`app/repositories/beton.py` `a.py` `b.py` `c.py`")
	}
	got := runRecall(t, proj, "m", filepath.Join(proj, "backend", "app", "repositories", "beton.py"))
	if !strings.Contains(got, "odakli.md") {
		t.Errorf("en odaklı not gelmeli:\n%s", got)
	}
	if n := strings.Count(got, "- `"); n != recallTop {
		t.Errorf("%d not gelmeli, %d geldi:\n%s", recallTop, n, got)
	}
}
