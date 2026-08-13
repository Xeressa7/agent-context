package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// SearchOptions mirrors the Python `ctx search` flags.
type SearchOptions struct {
	Query string
	Repo  string
	All   bool // ignore the scope filter
	Top   int
	Min   float64
}

type searchHit struct {
	score       float64
	path        string
	validity    string
	description string
	reasons     []string
	matched     int
}

// Search prints the ranked notes for a query.
//
// Candidate generation is OR: a note matching ONE query word is a candidate.
// The old AND rule produced a measured failure — the query "tenant izolasyonu"
// dropped the very note about tenant isolation, because the inflected second
// word did not appear literally. Ranking still rewards full coverage, so AND's
// ordering quality survives without its silent eliminations.
func Search(env Env, opts SearchOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		// Python surumu bu hatayi STDOUT'a basar ve hook'lar ciktiyi oradan
		// okur; akim da davranisin parcasi, stderr'e kaydirilamaz.
		env.printf("HATA: %s.\n", err)
		return 2
	}
	scope := store.RepoScope(ctx.Data)

	var words []string
	for _, w := range strings.Fields(opts.Query) {
		words = append(words, core.Fold(w))
	}
	if len(words) == 0 {
		env.printf("HATA: bos sorgu.\n")
		return 2
	}
	phrase := core.Fold(strings.Join(strings.Fields(opts.Query), " "))

	notes, err := store.LoadNotes(ctx.MemoryPath)
	if err != nil {
		env.errorf("HATA: hafiza okunamadi: %s\n", err)
		return 2
	}

	var hits []searchHit
	for _, n := range notes {
		if !opts.All && !core.Visible(n.FrontMatter, scope, false) {
			continue
		}
		score, ok := core.ScoreNote(core.Note{
			RelPath:     n.Rel,
			Title:       n.FrontMatter["title"],
			Tags:        n.FrontMatter["tags"],
			Components:  n.FrontMatter["components"],
			Description: n.FrontMatter["description"],
			Body:        n.Body,
		}, words, phrase)
		if !ok || score.Value < opts.Min {
			continue
		}
		status, noteScope := core.Validity(n.FrontMatter)
		validity := strings.Join(noteScope, ",")
		if validity == "" {
			validity = "evrensel"
			if status == core.StatusArchived {
				validity = "arsiv"
			}
		}
		hits = append(hits, searchHit{
			score:       score.Value,
			path:        n.Rel,
			validity:    validity,
			description: n.FrontMatter["description"],
			reasons:     score.Reasons,
			matched:     score.Matched,
		})
	}

	// Score descending, path ascending on ties — determinism is the point: the
	// same memory and query must always print the same lines.
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].path < hits[j].path
	})

	shown := hits
	if len(shown) > opts.Top {
		shown = shown[:opts.Top]
	}
	multi := len(words) > 1
	for _, h := range shown {
		coverage := ""
		if multi {
			coverage = fmt.Sprintf(" · %d/%d", h.matched, len(words))
		}
		env.printf("%s · %s · [%s]%s · %s\n",
			h.path, h.validity, strings.Join(h.reasons, "+"), coverage,
			clip(h.description, 120))
	}
	if len(hits) > opts.Top {
		env.printf("(+%d sonuç daha — \"--top N\" ile artır)\n", len(hits)-opts.Top)
	}
	if len(hits) == 0 {
		env.printf("(eslesme yok)\n")
	}

	printArchive(env, ctx, words, phrase)
	return 0
}

// printArchive lists hits from an INHERITED, non-canonical index (a flat list
// handed over from some earlier system). It runs only when context.yaml names
// one, always after the canonical results, under its own heading and capped at
// five lines: an inherited record must never look like a canonical note.
func printArchive(env Env, ctx store.Context, words []string, phrase string) {
	rel := ctx.Data.Str("archive_index")
	if rel == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(ctx.MemoryPath, rel))
	if err != nil {
		return
	}

	type archiveHit struct {
		matched int
		phrase  bool
		line    string
	}
	var found []archiveHit
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		folded := core.Fold(line)
		n := 0
		for _, w := range words {
			if strings.Contains(folded, w) {
				n++
			}
		}
		if n > 0 {
			found = append(found, archiveHit{n, strings.Contains(folded, phrase), line})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].matched != found[j].matched {
			return found[i].matched > found[j].matched
		}
		return found[i].phrase && !found[j].phrase
	})
	if len(found) == 0 {
		return
	}

	env.printf("\n--- eski arşiv (%s) ---\n", rel)
	for i, h := range found {
		if i == 5 {
			break
		}
		env.printf("[eski arşiv] %s\n", clip(h.line[2:], 160))
	}
	if len(found) > 5 {
		env.printf("(+%d eski arşiv sonucu daha)\n", len(found)-5)
	}
}
