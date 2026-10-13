package cli

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// The ALWAYS-LOADED MAP. Measured on 2026-08-12: a 21-note corpus went a whole
// session without a single note being opened, because nothing ever triggers a
// search. The one real advantage the old harness-native memory had was that its
// index loaded into every session for free; this command puts that back,
// harness-INDEPENDENTLY.
//
// One producer (this command), many consumers:
//   - Claude Code      -> a SessionStart hook prints it straight into context
//   - Codex/Cursor/... -> a marked block inside AGENTS.md (`ctx index --yaz`)
//
// The block is NOT the notes: one line per note, "what exists, where". The ban
// on loading the whole corpus survives — the agent sees the map and OPENS the
// note it needs.
const (
	indexStart = "<!-- ctx:index:basla -->"
	indexEnd   = "<!-- ctx:index:bitir -->"

	descLimit = 95
)

// folderOrder is reading priority: product truth first, then architecture and
// decisions, then lessons. Anything unlisted follows, alphabetically.
var folderOrder = []string{"facts", "decisions", "pitfalls"}

// IndexOptions mirrors the Python `ctx index` flags.
type IndexOptions struct {
	Repo  string
	All   bool
	Write bool // refresh the marked block in the target files
	Check bool // report stale blocks without writing
}

// shorten squeezes whitespace and clips to a word boundary.
func shorten(text string, limit int) string {
	text = squeeze(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	cut := string(runes[:limit])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}

// indexBudget is the character ceiling for the map INJECTED by the hook. The
// block written into files is uncapped: a file is read on demand, an injection
// eats every session's context.
//
// Over budget, NO NOTE IS DROPPED — descriptions shrink instead. Dropping notes
// is a silent loss of coverage: the map reads as "this is all of it" and the
// invisible note is never searched for. A shortened description still leaves
// the line there, and the agent can open the note.
const indexBudget = 16000

// descLimitSteps shrink the descriptions; namesOnly is the last tier. A 35-
// character tier existed until 2026-10-13 and was dropped: measured on yapit,
// a description clipped that far kept nothing ("`fatura-hakedis.md` — F"),
// while the file names alone — descriptive slugs — cost ~2,800 characters for
// 204 notes against 16,000 for the clipped list.
var descLimitSteps = []int{descLimit, 80, 55}

const namesOnly = 0

const namesOnlyNote = "\n\n[not: harita yalniz dosya adlarindan ibaret — aciklama ve tam metin " +
	"notun kendisinde; `ctx search \"<sorgu>\"` aciklamalarda da arar]"

// IndexBodyBudgeted fits the map under the ceiling, full first, then with
// progressively shorter descriptions, then as file names only.
// `index_style: adlar` goes straight to the names.
func IndexBodyBudgeted(ctx store.Context, budget int) (string, error) {
	if indexNamesOnly(ctx.Data) {
		body, err := indexBodyLimited(ctx, false, namesOnly)
		return body + namesOnlyNote, err
	}
	body, err := IndexBody(ctx, false)
	if err != nil || budget <= 0 || len([]rune(body)) <= budget {
		return body, err
	}
	for _, limit := range descLimitSteps[1:] {
		body, err = indexBodyLimited(ctx, false, limit)
		if err != nil {
			return body, err
		}
		if len([]rune(body)) <= budget {
			return body + "\n\n[not: harita tavana sigsin diye aciklamalar " +
				itoa(limit) + " karaktere kisaltildi — tam metin notun kendisindedir]", nil
		}
	}
	body, err = indexBodyLimited(ctx, false, namesOnly)
	if err != nil {
		return body, err
	}
	if len([]rune(body)) > budget {
		return body + "\n\n[not: harita yalniz dosya adlariyla bile tavani asiyor; " +
			"korpus bolunmeli — `ctx doctor`]", nil
	}
	return body + namesOnlyNote, nil
}

// indexNamesOnly reports `index_style: adlar` (names-only map, any budget).
func indexNamesOnly(m store.Manifest) bool {
	switch strings.ToLower(strings.TrimSpace(m.Str("index_style"))) {
	case "adlar", "names":
		return true
	}
	return false
}

// IndexBody renders the note map. Deterministic by construction: the walk is
// path-sorted and the folder order is fixed.
func IndexBody(ctx store.Context, all bool) (string, error) {
	return indexBodyLimited(ctx, all, descLimit)
}

func indexBodyLimited(ctx store.Context, all bool, limit int) (string, error) {
	notes, err := store.LoadNotes(ctx.MemoryPath)
	if err != nil {
		return "", err
	}
	scope := store.RepoScope(ctx.Data)
	var components []string
	if !all && indexComponentsOn(ctx.Data) {
		components = ctx.Data.List("components")
	}

	groups := map[string][]string{}
	count, elsewhere := 0, 0
	for _, n := range notes {
		if !all && !core.Visible(n.FrontMatter, scope, false) {
			continue
		}
		if !core.ComponentsMatch(n.FrontMatter, components) {
			elsewhere++
			continue
		}
		group, name, _ := strings.Cut(n.Rel, "/")
		if limit == namesOnly {
			groups[group] = append(groups[group], strings.TrimSuffix(name, ".md"))
		} else {
			groups[group] = append(groups[group],
				"- `"+name+"` — "+shorten(n.FrontMatter["description"], limit))
		}
		count++
	}

	scopeText := "tümü"
	if len(scope) > 0 {
		scopeText = strings.Join(scope, ", ")
	}
	if len(components) > 0 {
		scopeText += " · bileşen: " + strings.Join(components, ", ")
	}
	memoryName := filepath.Base(filepath.FromSlash(ctx.Data.Str("memory_path")))
	// `.memory` names nothing; the project does.
	if strings.HasPrefix(memoryName, ".") && ctx.Data.Str("project") != "" {
		memoryName = ctx.Data.Str("project")
	}
	lines := []string{
		"### Hafıza haritası — " + memoryName + " · kapsam: " + scopeText + " · " + itoa(count) + " not",
		"",
		"Bu bir **harita**, notun kendisi değil: ilgili satırı görünce **o notu aç**. " +
			"Hedefli arama `ctx search \"<sorgu>\"`; bütün korpusu topluca yükleme.",
		"Açık işler `" + orDefault(ctx.Data.Str("task_state"), "tasks/OPEN.md") + "` · " +
			"oturum devamı `" + orDefault(ctx.Data.Str("session_state"), "sessions/CURRENT.md") + "`.",
	}
	// The narrowing is said out loud: a map that silently omits notes reads as
	// "this is all of it", and the omitted note is never searched for.
	if elsewhere > 0 {
		lines = append(lines, "Başka bileşenlerin "+itoa(elsewhere)+" notu bu haritada yok; "+
			"`ctx search` onları da bulur.")
	}

	var ordered []string
	for _, g := range folderOrder {
		if _, ok := groups[g]; ok {
			ordered = append(ordered, g)
		}
	}
	var rest []string
	for g := range groups {
		if !contains(folderOrder, g) {
			rest = append(rest, g)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)

	for _, g := range ordered {
		if limit == namesOnly {
			lines = append(lines, "", "**"+g+"/** ("+itoa(len(groups[g]))+"): "+
				strings.Join(groups[g], " · "))
			continue
		}
		lines = append(lines, "", "**"+g+"/**")
		lines = append(lines, groups[g]...)
	}
	return strings.Join(lines, "\n"), nil
}

// indexComponentsOn reports whether the map narrows by the repo's components
// (`index_components`, default off). Opt-in because `ctx init` writes a
// placeholder component ("app") that existing notes never carry: turning the
// filter on by default would empty those maps.
func indexComponentsOn(m store.Manifest) bool {
	switch strings.ToLower(strings.TrimSpace(m.Str("index_components"))) {
	case "true", "1", "yes", "evet", "acik", "açık":
		return true
	}
	return false
}

// Index prints the map, or refreshes it inside the target files.
func Index(env Env, opts IndexOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		env.printf("HATA: %s.\n", err) // stdout — bkz. search.go
		return 2
	}

	if !opts.Write && !opts.Check {
		body, err := IndexBody(ctx, opts.All)
		if err != nil {
			env.errorf("HATA: hafiza okunamadi: %s\n", err)
			return 2
		}
		env.printf("%s\n", body)
		return 0
	}

	problems := 0
	for _, manifest := range ConsumerManifests(ctx) {
		problems += refreshConsumer(env, manifest, opts)
	}
	if problems > 0 {
		return 1
	}
	return 0
}

// ConsumerManifests lists every repo that reads this memory, the current one
// first.
//
// Why it exists: ONE memory, MANY consumers. Until 2026-08-13 a refresh only
// wrote to the current repo's targets, so the others drifted silently —
// measured: the yapit-server and yapit-client blocks still showed two notes'
// old descriptions nine days after they changed, and the harness-less tools in
// those repos were reading a stale corpus. SPEC promises "staleness is closed
// structurally"; the promise held PER REPO, now it holds per memory.
//
// The list lives in the memory's own ctx.yaml (`consumers:`), because "who
// reads me" is a property of the memory.
func ConsumerManifests(ctx store.Context) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() || seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}
	add(ctx.ManifestPath)
	for _, rel := range store.MemoryConfig(ctx.MemoryPath).List("consumers") {
		add(filepath.Join(ctx.MemoryPath, filepath.FromSlash(rel), store.ManifestName))
	}
	return out
}

// consumerContext resolves one consumer's manifest into a full context.
//
// Each consumer is computed with ITS OWN manifest: the scope filter differs
// from repo to repo (the legacy repo sees legacy notes, a current repo does
// not), so one shared body would be wrong for at least one of them.
func consumerContext(manifestPath string) (store.Context, error) {
	data, err := store.ReadManifest(manifestPath)
	if err != nil {
		return store.Context{}, err
	}
	rel := data.Str("memory_path")
	if rel == "" {
		return store.Context{}, errNoMemoryPath
	}
	memory := rel
	if !filepath.IsAbs(memory) {
		memory = filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(rel))
	}
	if abs, err := filepath.Abs(memory); err == nil {
		memory = abs
	}
	return store.Context{ManifestPath: manifestPath, Data: data, MemoryPath: memory}, nil
}

var errNoMemoryPath = errors.New("memory_path yok")

// shortTarget names a target by its repo, so four consumers do not all print
// the same bare "AGENTS.md".
func shortTarget(path string) string {
	return filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
}

// refreshConsumer checks or rewrites one consumer's blocks and returns how many
// targets were a problem.
func refreshConsumer(env Env, manifestPath string, opts IndexOptions) int {
	ctx, err := consumerContext(manifestPath)
	if err != nil {
		return 0
	}
	body, err := IndexBody(ctx, opts.All)
	if err != nil {
		return 0
	}

	problems := 0
	for _, target := range IndexTargets(ctx) {
		raw, err := os.ReadFile(target)
		if err != nil {
			switch {
			case !generatedTarget(ctx, target, err):
				env.printf("✗ %s · dosya yok\n", target)
				problems++
			case !opts.Write:
				env.printf("⚠ %s · uretilmis harita dosyasi yok — `ctx index --yaz`\n", target)
				problems++
			default:
				if err := writeGenerated(ctx, target, body); err != nil {
					env.printf("✗ %s · yazilamadi: %s\n", target, err)
					problems++
					continue
				}
				env.printf("✓ %s · uretildi\n", target)
			}
			continue
		}
		text := string(raw)
		updated, ok := replaceBlock(text, body)
		switch {
		case !ok:
			env.printf("✗ %s · ctx:index isareti yok — hedef dosyaya su iki satiri "+
				"komsu satirlarda ekle:\n    %s\n    %s\n", target, indexStart, indexEnd)
			problems++
		case updated == text:
			env.printf("✓ %s · guncel\n", shortTarget(target))
		case !opts.Write:
			env.printf("⚠ %s · index blogu BAYAT — `ctx index --yaz`\n", target)
			problems++
		default:
			if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
				env.printf("✗ %s · yazilamadi: %s\n", target, err)
				problems++
				continue
			}
			env.printf("✓ %s · yazildi\n", target)
		}
	}
	return problems
}

// generatedTarget reports whether a MISSING target may be produced from
// scratch: only one the manifest names explicitly. The default AGENTS.md
// carries the router written by `ctx bootstrap`; a map-only AGENTS.md would
// pass for a set-up repo while lacking it.
//
// Why missing targets are produced at all: a map file is build output. Kept in
// git, every new note costs one commit per consumer repo — measured on yapit:
// 326 commits across three repos were nothing but index refreshes. Ignored by
// git, a fresh clone has no file, so the refresh has to create it.
func generatedTarget(ctx store.Context, target string, readErr error) bool {
	if !errors.Is(readErr, os.ErrNotExist) || len(ctx.Data.List("index_targets")) == 0 {
		return false
	}
	info, err := os.Stat(filepath.Dir(target))
	return err == nil && info.IsDir()
}

// writeGenerated creates a map-only target file around the marked block.
func writeGenerated(ctx store.Context, target, body string) error {
	memoryName := filepath.Base(filepath.FromSlash(ctx.Data.Str("memory_path")))
	text := "# Hafıza haritası — " + memoryName + "\n\n" +
		"> Bu dosyayı `ctx index --yaz` üretir; ELLE DÜZENLEME. Üretilmiş dosyadır,\n" +
		"> git'te tutulması gerekmez (yerelde yeniden üretilir).\n\n" +
		indexStart + "\n" + body + "\n" + indexEnd + "\n"
	return os.WriteFile(target, []byte(text), 0o644)
}

// IndexTargets resolves context.yaml's index_targets (default AGENTS.md). The
// target need not be AGENTS.md: any file carrying the marked block — a Cursor
// rule, copilot-instructions.md — can be a consumer.
func IndexTargets(ctx store.Context) []string {
	names := ctx.Data.List("index_targets")
	if len(names) == 0 {
		names = []string{"AGENTS.md"}
	}
	base := filepath.Dir(ctx.ManifestPath)
	out := make([]string, 0, len(names))
	for _, n := range names {
		p := n
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, n)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		out = append(out, abs)
	}
	return out
}

// replaceBlock swaps the INSIDE of the marked block. The file's line endings are
// PRESERVED — rewriting an LF file as CRLF would drag the entire file into the
// diff.
func replaceBlock(text, body string) (string, bool) {
	i := strings.Index(text, indexStart)
	j := strings.Index(text, indexEnd)
	if i == -1 || j == -1 || j < i {
		return "", false
	}
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	block := strings.Join(append([]string{indexStart}, append(strings.Split(body, "\n"), indexEnd)...), nl)
	return text[:i] + block + text[j+len(indexEnd):], true
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func orDefault(value, def string) string {
	if value == "" {
		return def
	}
	return value
}
