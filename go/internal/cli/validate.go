package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// THE validation engine. Consumers: `ctx doctor`, `ctx validate`, the pre-commit
// hook. The same logic is never written twice.
//
// Two levels:
//
//	fast (default) — structure: front matter, required fields, links, review-due
//	--full          — plus a PII/secret sweep (expensive; for pre-commit and
//	                 deliberate runs)
//
// The level split matters: a 90-day-old note is NOT stale, it is REVIEW-DUE.
// Real staleness needs machine-checkable anchors, which do not exist yet.

var requiredFields = []string{"title", "description", "updated"}

const defaultReviewDays = 90

var (
	mdLinkPattern   = regexp.MustCompile(`\[[^\]]*\]\(([^)#]+?)(?:#[^)]*)?\)`)
	wikiLinkPattern = regexp.MustCompile(`\[\[([^\]|]+)`)

	// An ASSIGNED secret. The name alone is NOT a finding — SPEC.md's own
	// recommended shape ("wants ADMIN_SECRET, the value lives outside") is
	// legitimate and must not trip the gate.
	secretPattern = regexp.MustCompile(
		`(?i)\b(api[_-]?key|secret|token|password|passwd|pwd)\b\s*[:=]\s*['"]?([A-Za-z0-9+/=_.-]{16,})`)
)

func taskArchived(fm map[string]string) bool {
	status := strings.TrimSpace(fm["status"])
	lifecycle := strings.TrimSpace(fm["lifecycle"])
	return status == core.StatusArchived || status == "closed" || status == "done" || lifecycle == "historical"
}

// A closed item left inside an ACTIVE task file is how the next session takes
// the same work twice. The universal signal is a ticked checkbox (`- [x]`);
// project-specific words ("KAPANDI", "DONE") come from the memory's own
// ctx.yaml `closed_markers` list, because the tool does not assume the
// project's language. A line carrying `ctx:acik` is exempt, for prose that
// DESCRIBES the closing format instead of being a closed item.
var checkedBoxPattern = regexp.MustCompile(`^\s*[-*+]\s*\[[xX]\]`)

const (
	openItemExemption = "ctx:acik"
	closedFindingCap  = 5
)

// OPEN.md is an index, and an index row decays in a specific way: every
// session appends "X closed (hash)" to the status cell instead of rewriting it
// to what is still open. Measured on the yapit memory (2026-09-27): 46 closing
// phrases across one index while the gate reported nothing, because OPEN.md
// was exempt from the closed-item scan. So OPEN.md gets the same scan, and a
// memory may cap the length of an index cell (`open_row_max_chars`, 0 = off):
// a status that only names what is open stays short on its own.
func longIndexCells(text string, limit int) [][2]string {
	if limit <= 0 {
		return nil
	}
	var out [][2]string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(strings.TrimSpace(line), "|") || strings.Contains(line, openItemExemption) {
			continue
		}
		for _, cell := range strings.Split(line, "|") {
			n := len([]rune(strings.TrimSpace(cell)))
			if n > limit {
				out = append(out, [2]string{itoa(i + 1), itoa(n)})
				break
			}
		}
	}
	return out
}

func closedItemLines(text string, extraMarkers []string) [][2]string {
	var out [][2]string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.Contains(line, openItemExemption) {
			continue
		}
		hit := checkedBoxPattern.MatchString(line)
		for _, m := range extraMarkers {
			if m != "" && strings.Contains(line, m) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		// KARAKTER sayar, bayt degil: Python surumu de oyle yapar ve Turkce
		// metinde bayt kirpmasi iki implementasyonu sessizce ayristirir.
		summary := strings.TrimSpace(line)
		if r := []rune(summary); len(r) > 60 {
			summary = string(r[:60])
		}
		out = append(out, [2]string{itoa(i + 1), summary})
	}
	return out
}

// taskStateFindings is the structural gate for the state files that the note
// walker intentionally skips. Every active/current task file must be reachable
// from tasks/OPEN.md; archived task files must not remain in that index. Local
// markdown links in OPEN.md must also resolve, otherwise a closed task can
// silently remain as a dead row.
func taskStateFindings(memory string) []Finding {
	tasksDir := filepath.Join(memory, "tasks")
	openPath := filepath.Join(tasksDir, "OPEN.md")
	var findings []Finding
	if _, err := os.Stat(openPath); err != nil {
		return []Finding{{Level: "hata", Path: "tasks/OPEN.md", Msg: "acik is indeksi yok"}}
	}
	raw, err := os.ReadFile(openPath)
	if err != nil {
		return []Finding{{Level: "hata", Path: "tasks/OPEN.md", Msg: "acik is indeksi okunamadi"}}
	}
	tasksRoot, _ := filepath.Abs(tasksDir)
	config := store.MemoryConfig(memory)
	extraMarkers := config.List("closed_markers")
	for i, c := range closedItemLines(string(raw), extraMarkers) {
		if i >= closedFindingCap {
			findings = append(findings, Finding{Level: "hata", Path: "tasks/OPEN.md",
				Msg: "indekste kapanis beyani olan baska satirlar da var"})
			break
		}
		findings = append(findings, Finding{Level: "hata", Path: "tasks/OPEN.md:" + c[0],
			Msg: "indeks satiri kapanan isi anlatiyor; yalniz ACIK kalani yaz: " + c[1]})
	}
	for _, c := range longIndexCells(string(raw), config.Int("open_row_max_chars", 0)) {
		findings = append(findings, Finding{Level: "hata", Path: "tasks/OPEN.md:" + c[0],
			Msg: "indeks hucresi " + c[1] + " karakter; gecmis degil yalniz acik kalan yazilir"})
	}
	listed := map[string]bool{}
	for _, match := range mdLinkPattern.FindAllStringSubmatch(string(raw), -1) {
		target := strings.TrimSpace(match[1])
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
			strings.HasPrefix(target, "mailto:") || !strings.HasSuffix(target, ".md") {
			continue
		}
		resolved, _ := filepath.Abs(filepath.Join(tasksDir, filepath.FromSlash(target)))
		if _, err := os.Stat(resolved); err != nil {
			findings = append(findings, Finding{Level: "hata", Path: "tasks/OPEN.md", Msg: "kirik task linki: " + target})
			continue
		}
		rel, err := filepath.Rel(tasksRoot, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // decisions/domain gibi tasks disi yerel linkler indeks degildir
		}
		rel = filepath.ToSlash(rel)
		if rel != "OPEN.md" {
			listed[rel] = true
		}
	}

	err = filepath.WalkDir(tasksDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if filepath.Clean(path) == filepath.Clean(openPath) {
			return nil
		}
		rel, relErr := filepath.Rel(tasksRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		text, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fm := core.ParseFrontMatter(string(text))
		if taskArchived(fm) {
			if listed[rel] {
				findings = append(findings, Finding{Level: "hata", Path: "tasks/" + rel,
					Msg: "arsiv task OPEN.md'de listeleniyor; satiri sil"})
			}
		} else if !listed[rel] {
			findings = append(findings, Finding{Level: "hata", Path: "tasks/" + rel,
				Msg: "aktif task OPEN.md'de listelenmiyor"})
		}
		if !taskArchived(fm) {
			closed := closedItemLines(string(text), extraMarkers)
			for i, c := range closed {
				if i >= closedFindingCap {
					findings = append(findings, Finding{Level: "hata", Path: "tasks/" + rel,
						Msg: itoa(len(closed)-closedFindingCap) + " kapanmis madde daha var"})
					break
				}
				findings = append(findings, Finding{Level: "hata", Path: "tasks/" + rel + ":" + c[0],
					Msg: "kapanmis madde aktif task dosyasinda duruyor; SIL: " + c[1]})
			}
		}
		return nil
	})
	if err != nil {
		findings = append(findings, Finding{Level: "hata", Path: "tasks", Msg: "task dizini okunamadi"})
	}
	return findings
}

// Finding is one validation result.
type Finding struct {
	Level string // "hata" | "uyari"
	Path  string
	Msg   string
}

// ValidateOptions mirrors the Python `ctx validate` flags.
type ValidateOptions struct {
	Repo   string
	Memory string // direct path, skipping the manifest search (for pre-commit)
	Full   bool   // also run the PII/secret sweep
}

// Validate runs the engine and prints the report.
func Validate(env Env, opts ValidateOptions) int {
	var memory string
	if opts.Memory != "" {
		// The memory repo itself has NO context.yaml (the manifest lives in the
		// consuming repos), so callers like the pre-commit hook pass the path.
		abs, err := filepath.Abs(opts.Memory)
		if err != nil {
			env.printf("HATA: --memory dizini yok: %s\n", opts.Memory)
			return 2
		}
		memory = abs
		if info, err := os.Stat(memory); err != nil || !info.IsDir() {
			env.printf("HATA: --memory dizini yok: %s\n", memory)
			return 2
		}
		if info, err := os.Stat(filepath.Join(memory, "README.md")); err != nil || info.IsDir() {
			env.printf("HATA: %s bir hafiza deposuna benzemiyor (README.md yok).\n", memory)
			return 2
		}
	} else {
		ctx, err := store.LoadContext(opts.Repo)
		if err != nil {
			env.printf("HATA: %s.\n", err) // stdout — bkz. search.go
			return 2
		}
		memory = ctx.MemoryPath
	}

	findings, err := Check(memory, opts.Full)
	if err != nil {
		env.errorf("HATA: hafiza okunamadi: %s\n", err)
		return 2
	}

	var errs, warns []Finding
	for _, f := range findings {
		if f.Level == "hata" {
			errs = append(errs, f)
		} else {
			warns = append(warns, f)
		}
	}
	for _, f := range append(append([]Finding{}, errs...), warns...) {
		mark := "⚠"
		if f.Level == "hata" {
			mark = "✗"
		}
		env.printf("%s %s · %s\n", mark, f.Path, f.Msg)
	}

	if len(findings) == 0 {
		suffix := ""
		if !opts.Full {
			suffix = " (yapisal; PII taramasi icin --full)"
		}
		env.printf("✓ bulgu yok%s\n", suffix)
	} else {
		suffix := ""
		if !opts.Full {
			suffix = " · PII taramasi KOSULMADI (--full)"
		}
		env.printf("\n%d hata · %d uyari%s\n", len(errs), len(warns), suffix)
	}

	// The SCOPE of the protection is always stated. Otherwise, with no profile
	// configured, the sweep narrows silently and a "✓ bulgu yok" line implies a
	// clean bill for a PII class that was never even looked at — which is
	// exactly what being silently wrong looks like.
	if opts.Full {
		profiles := store.MemoryConfig(memory).List("pii_profiles")
		if len(profiles) > 0 {
			env.printf("  PII kapsami: evrensel + profil(%s)\n", strings.Join(profiles, ", "))
		} else {
			env.printf("  PII kapsami: YALNIZ EVRENSEL (e-posta · IBAN · kart). Ulkeye ozel "+
				"kimlik/telefon desenleri KOSMADI —\n  acmak icin: "+
				"%s -> pii_profiles: [tr]  (secenekler: %s)\n",
				store.MemoryConfigName, strings.Join(profileNames(), ", "))
		}
	}

	if len(errs) > 0 {
		return 1
	}
	return 0
}

// Check is the engine itself: it returns findings in a fixed order (walk order,
// then check order within a note) so two runs — and two implementations — print
// identical reports.
func Check(memory string, full bool) ([]Finding, error) {
	config := store.MemoryConfig(memory)
	patterns := piiPatterns(config.List("pii_profiles"))
	reviewDays := config.Int("review_days", defaultReviewDays)

	notes, err := store.LoadNotes(memory)
	if err != nil {
		return nil, err
	}

	stems := map[string]bool{}
	for _, n := range notes {
		stems[strings.TrimSuffix(filepath.Base(n.Rel), filepath.Ext(n.Rel))] = true
	}
	titles := map[string]string{}
	today := time.Now()

	findings := taskStateFindings(memory)
	for _, n := range notes {
		path := n.Rel
		add := func(level, msg string) { findings = append(findings, Finding{level, path, msg}) }
		fm := n.FrontMatter

		if len(fm) == 0 {
			add("hata", "frontmatter yok ya da ayristirilamadi")
			continue
		}

		// The validity axis: the new status/scope pair, or the retired
		// lifecycle field. Neither present means universal and active — a
		// single-era project should not have to write scope at all.
		if oldLC := strings.TrimSpace(fm["lifecycle"]); oldLC != "" {
			if _, known := core.LegacyLifecycle(oldLC); !known {
				add("hata", "gecersiz lifecycle: "+pyRepr(oldLC))
			} else {
				add("uyari", "`lifecycle: "+oldLC+"` eskidi → `ctx migrate` "+
					"(status/scope ikilisine cevirir)")
			}
		}
		if status := strings.TrimSpace(fm["status"]); status != "" &&
			status != core.StatusActive && status != core.StatusArchived {
			add("hata", "gecersiz status: "+pyRepr(status)+" (gecerli: "+
				core.StatusActive+", "+core.StatusArchived+")")
		}
		for _, field := range requiredFields {
			if fm[field] == "" {
				add("hata", "zorunlu alan eksik: "+field)
			}
		}

		if title := fm["title"]; title != "" {
			if first, seen := titles[title]; seen {
				add("uyari", "baslik tekrari: "+first+" ile ayni")
			} else {
				titles[title] = path
			}
		}

		if sup := fm["superseded_by"]; sup != "" && sup != "null" && sup != "None" {
			if _, err := os.Stat(filepath.Join(memory, sup)); err != nil {
				add("hata", "superseded_by hedefi yok: "+sup)
			}
		}

		body := core.SplitBody(n.Text) // NOT n.Body: the 100k search cap must not silently
		// shorten what validation looks at — a long note would lose link checks.
		for _, m := range mdLinkPattern.FindAllStringSubmatch(body, -1) {
			target := strings.TrimSpace(m[1])
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "mailto:") || !strings.HasSuffix(target, ".md") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(n.Abs), filepath.FromSlash(target))); err != nil {
				add("hata", "kirik markdown link: "+target)
			}
		}

		for _, m := range wikiLinkPattern.FindAllStringSubmatch(body, -1) {
			name := strings.TrimSpace(m[1])
			if !stems[name] {
				add("uyari", "cozumlenmeyen wiki-link: [["+name+"]] "+
					"(eski arsive isaret ediyor olabilir)")
			}
		}

		// review-due applies to ACTIVE notes only. An archived note already
		// says it does not describe today; asking it for freshness is
		// meaningless.
		if status, _ := core.Validity(fm); status == core.StatusActive {
			updated, present := fm["updated"]
			when, ok := parseDate(updated)
			if !ok {
				add("hata", "updated tarihi okunamadi: "+pyReprOptional(updated, present))
			} else if days := daysBetween(when, today); days > reviewDays {
				add("uyari", "review-due: "+itoa(days)+" gundur dogrulanmadi (stale DEGIL)")
			}
		}

		if full {
			for _, m := range secretPattern.FindAllStringSubmatch(n.Text, -1) {
				add("hata", "deger atanmis gizli anahtar gorunumu: "+m[1]+"=***")
			}
			for _, p := range patterns {
				if p.match(n.Text) {
					add("hata", "olasi PII: "+p.name)
				}
			}
		}
	}
	return findings, nil
}

// ---------------------------------------------------------------- PII

// piiPattern is a named detector. Some decide on format alone; the two
// expensive ones decide on a CHECKSUM.
//
// FORMAT GUESSING IS NOT ENOUGH, A CHECKSUM IS REQUIRED. Measured during the
// 0.5.0 round: format-only patterns called ordinary prose PII — "2024 2025 2026
// 2027" looked like a credit card, "AB12CDEF1234…" like an IBAN. A gate that
// cries wolf is NOT a gate: the user learns to reach for --no-verify and the
// real leak walks through too. So format only nominates a candidate; Luhn and
// ISO 7064 mod-97 decide. A random digit string passes Luhn ~1/10 of the time
// and mod-97 ~1/97.
type piiPattern struct {
	name       string
	re         *regexp.Regexp
	digitEdges bool              // reject a hit touching another digit
	verify     func(string) bool // nil means format alone decides
}

func (p piiPattern) match(text string) bool {
	offset := 0
	for offset <= len(text) {
		loc := p.re.FindStringIndex(text[offset:])
		if loc == nil {
			return false
		}
		start, end := offset+loc[0], offset+loc[1]
		hit := text[start:end]
		// Go's regexp has no lookarounds, so the (?<!\d)…(?!\d) guards are
		// checked here. Retrying from start+1 rather than end reproduces the
		// backtracking engine: a rejected hit must not hide a valid one that
		// begins inside it.
		edgesOK := true
		if p.digitEdges {
			edgesOK = !endsInDigit(text[:start]) && !startsWithDigit(text[end:])
		}
		if edgesOK && (p.verify == nil || p.verify(hit)) {
			return true
		}
		offset = start + 1
	}
	return false
}

var universalPII = []piiPattern{
	{name: "e-posta", re: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)},
	{
		name:   "IBAN",
		re:     regexp.MustCompile(`\b[A-Z]{2}[\p{Nd}]{2}(?:[ -]?[A-Za-z0-9]{4}){2,7}[ -]?[A-Za-z0-9]{1,4}\b`),
		verify: core.IBANValid,
	},
	{
		name:       "kredi karti",
		re:         regexp.MustCompile(`(?:[\p{Nd}]{4}[ -]?){3}[\p{Nd}]{4}`),
		digitEdges: true,
		verify: func(s string) bool {
			return core.LuhnValid(strings.NewReplacer(" ", "", "-", "").Replace(s))
		},
	},
}

// Country-specific patterns run ONLY when named in the memory's pii_profiles.
// "An 11-digit number" is a national ID in Turkey but an order number
// elsewhere: run everywhere and it cries wolf, run nowhere and real PII walks.
var piiProfiles = map[string][]piiPattern{
	"tr": {
		{name: "TC kimlik gorunumlu 11 hane", re: regexp.MustCompile(`[1-9][\p{Nd}]{10}`), digitEdges: true},
		{name: "telefon (TR cep)", re: regexp.MustCompile(`0?5[\p{Nd}]{2}[\s.-]?[\p{Nd}]{3}[\s.-]?[\p{Nd}]{2}[\s.-]?[\p{Nd}]{2}`), digitEdges: true},
	},
	"us": {
		{name: "SSN gorunumlu", re: regexp.MustCompile(`[\p{Nd}]{3}-[\p{Nd}]{2}-[\p{Nd}]{4}`), digitEdges: true},
		{name: "telefon (US)", re: regexp.MustCompile(`\(?[\p{Nd}]{3}\)?[\s.-][\p{Nd}]{3}[\s.-][\p{Nd}]{4}`), digitEdges: true},
	},
	"de": {
		{name: "Steuer-ID gorunumlu 11 hane", re: regexp.MustCompile(`[\p{Nd}]{11}`), digitEdges: true},
	},
}

func piiPatterns(profiles []string) []piiPattern {
	out := append([]piiPattern{}, universalPII...)
	for _, name := range profiles {
		out = append(out, piiProfiles[strings.ToLower(strings.TrimSpace(name))]...)
	}
	return out
}

func profileNames() []string {
	names := make([]string, 0, len(piiProfiles))
	for k := range piiProfiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------- helpers

func endsInDigit(s string) bool {
	for _, r := range reverseRunes(s, 1) {
		return unicode.IsDigit(r)
	}
	return false
}

func startsWithDigit(s string) bool {
	for _, r := range s {
		return unicode.IsDigit(r)
	}
	return false
}

func reverseRunes(s string, n int) []rune {
	runes := []rune(s)
	if len(runes) < n {
		return nil
	}
	return runes[len(runes)-n:]
}

// parseDate reads the leading YYYY-MM-DD of a value, matching the Python
// reader's tolerance for trailing time components.
func parseDate(value string) (time.Time, bool) {
	s := strings.TrimSpace(value)
	if len([]rune(s)) > 10 {
		s = string([]rune(s)[:10])
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return time.Time{}, false
		}
		nums[i] = n
	}
	if nums[1] < 1 || nums[1] > 12 || nums[2] < 1 || nums[2] > 31 {
		return time.Time{}, false
	}
	t := time.Date(nums[0], time.Month(nums[1]), nums[2], 0, 0, 0, 0, time.UTC)
	if t.Year() != nums[0] || int(t.Month()) != nums[1] || t.Day() != nums[2] {
		return time.Time{}, false // normalised away, e.g. 2026-02-30
	}
	return t, true
}

// daysBetween counts whole days the way date arithmetic does: on calendar
// dates, ignoring the clock.
func daysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// pyRepr renders a value the way Python's repr() does, because the messages are
// compared against the Python implementation verbatim.
func pyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// pyReprOptional distinguishes a missing key (Python's None) from an empty one.
func pyReprOptional(value string, present bool) string {
	if !present {
		return "None"
	}
	return pyRepr(value)
}
