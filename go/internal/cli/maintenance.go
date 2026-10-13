package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// `ctx bakim` — which notes most need the Geçerli/Tarihçe layout.
//
// READ-ONLY, on purpose. Restructuring a note means deciding which of two
// conflicting statements holds today; for a decision only its owner can say,
// for a fact only the code can. The tool finds the candidates and ranks them;
// an agent or the owner does the rewrite. It is a report, not a second task
// list: nothing is written to tasks/.
//
// Signals, all counted OUTSIDE `## Tarihçe` (what is already history is fine):
//   - a heading carrying a date or a status word ("UYGULANDI 2026-09-08"):
//     the note grew as a log, not as a description of today;
//   - an override phrase in the prose ("... kaldırıldı", "artık"): an old and
//     a new statement standing side by side;
//   - size: a 50 KB note is opened whole for one paragraph;
//   - a decision without `## Geçerli`: decisions are what gets overridden.

type MaintenanceOptions struct {
	Repo string
	Top  int
}

var datedHeading = regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b`)

// statusWords and overrideMarkers are folded. A project adds its own through
// the memory's ctx.yaml (`status_markers`, `override_markers`).
var statusWords = []string{"uygulandi", "kodlandi", "yapildi", "cozuldu", "tamamlandi",
	"kapandi", "done", "implemented", "fixed", "shipped"}

var overrideMarkers = []string{"kaldirildi", "vazgecildi", "iptal edildi", "yerine gecti",
	"artik ", "eskiden", "gecersiz", "tersine cevrildi", "no longer", "superseded", "reverted"}

type maintenanceRow struct {
	rel                    string
	size, dated, overrides int
	hasCurrent, isDecision bool
	score                  float64
}

func maintenanceRowFor(n store.Note, status, override []string) maintenanceRow {
	r := maintenanceRow{
		rel:        n.Rel,
		size:       len(n.Body),
		hasCurrent: core.HasCurrentSection(n.Body),
		isDecision: strings.HasPrefix(n.Rel, "decisions/"),
	}
	for _, h := range core.Headings(n.Body) {
		folded := core.Fold(h)
		if datedHeading.MatchString(h) || containsAny(folded, status) {
			r.dated++
		}
	}
	for _, line := range strings.Split(core.CurrentBody(n.Body), "\n") {
		if containsAny(core.Fold(line), override) {
			r.overrides++
		}
	}
	r.score = float64(r.dated)*2 + float64(r.overrides)*3 + float64(r.size)/5000
	if r.isDecision && !r.hasCurrent {
		r.score += 2
	}
	return r
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func foldAll(words []string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		out = append(out, core.Fold(w))
	}
	return out
}

// Maintenance prints the ranked report. Exit 0: it is information, not a gate.
func Maintenance(env Env, opts MaintenanceOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		env.printf("HATA: %s.\n", err)
		return 2
	}
	notes, err := store.LoadNotes(ctx.MemoryPath)
	if err != nil {
		env.errorf("HATA: hafiza okunamadi: %s\n", err)
		return 2
	}
	config := store.MemoryConfig(ctx.MemoryPath)
	status := append(foldAll(statusWords), foldAll(config.List("status_markers"))...)
	override := append(foldAll(overrideMarkers), foldAll(config.List("override_markers"))...)

	var rows []maintenanceRow
	layered, active := 0, 0
	for _, n := range notes {
		if st, _ := core.Validity(n.FrontMatter); st == core.StatusArchived {
			continue
		}
		active++
		r := maintenanceRowFor(n, status, override)
		if r.hasCurrent {
			layered++
		}
		if r.score >= 2 {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].rel < rows[j].rel
	})

	env.printf("Bakim — %d aktif not · %d tanesi `## Geçerli` duzeninde · %d aday\n\n",
		active, layered, len(rows))
	shown := rows
	if opts.Top > 0 && len(shown) > opts.Top {
		shown = shown[:opts.Top]
	}
	for _, r := range shown {
		var why []string
		if r.dated > 0 {
			why = append(why, fmt.Sprintf("tarihli/durum basligi %d", r.dated))
		}
		if r.overrides > 0 {
			why = append(why, fmt.Sprintf("geri-alma ifadesi %d", r.overrides))
		}
		if r.size >= 10000 {
			why = append(why, fmt.Sprintf("%d KB", r.size/1000))
		}
		if !r.hasCurrent {
			why = append(why, "Geçerli yok")
		}
		env.printf("%5.1f  %s · %s\n", r.score, r.rel, strings.Join(why, " · "))
	}
	if len(rows) > len(shown) {
		env.printf("(+%d aday daha — \"--top N\")\n", len(rows)-len(shown))
	}
	if len(rows) > 0 {
		env.printf("\nDuzen: en ustte `## Geçerli` (yalniz bugun gecerli olan, degisince BASTAN yazilir),\n" +
			"en altta `## Tarihçe` (degisiklik basina tek satir: tarih · eski -> yeni · kimin karari).\n" +
			"Karar notunda hangisinin gecerli oldugunu urun sahibi soyler; fact notunda kod.\n")
	}
	return 0
}
