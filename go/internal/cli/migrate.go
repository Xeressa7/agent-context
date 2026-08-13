package cli

import (
	"os"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// MigrateOptions mirrors the Python `ctx migrate` flags.
type MigrateOptions struct {
	Repo string
	Dry  bool // --kuru: show what would change, write nothing
}

// Migrate rewrites the retired single-axis `lifecycle` field as the
// `status` + `scope` pair.
//
// Line-based and in place: the rest of the front matter and the whole body are
// LEFT ALONE. A migration that reformats what it did not have to touch turns
// its own diff into noise, and the one line that mattered stops being visible.
func Migrate(env Env, opts MigrateOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		env.printf("HATA: %s.\n", err) // stdout — bkz. search.go
		return 2
	}
	notes, err := store.WalkNotes(ctx.MemoryPath)
	if err != nil {
		env.errorf("HATA: hafiza okunamadi: %s\n", err)
		return 2
	}

	write := !opts.Dry
	changed, skipped := 0, 0

	for _, note := range notes {
		text, err := note.Read()
		if err != nil {
			skipped++
			continue
		}
		old := strings.TrimSpace(core.ParseFrontMatter(text)["lifecycle"])
		if old == "" {
			skipped++
			continue
		}
		status, known := core.LegacyLifecycle(old)
		if !known {
			env.printf("  ! %s · bilinmeyen lifecycle: %s — ATLANDI\n", note.Rel, store.PyRepr(old))
			skipped++
			continue
		}
		scope := core.LegacyScope(old)

		replacement := []string{"status: " + status}
		if len(scope) > 0 {
			replacement = append(replacement, "scope: ["+strings.Join(scope, ", ")+"]")
		}

		var out []string
		replaced := false
		for _, line := range splitLines(text) {
			if !replaced && strings.HasPrefix(strings.TrimSpace(line), "lifecycle:") {
				out = append(out, replacement...)
				replaced = true
				continue
			}
			out = append(out, line)
		}
		if !replaced {
			skipped++
			continue
		}

		detail := " (evrensel)"
		if len(scope) > 0 {
			detail = " · scope: [" + strings.Join(scope, ", ") + "]"
		}
		env.printf("  %s · lifecycle: %s → status: %s%s\n", note.Rel, old, status, detail)

		if write {
			// The file's own line ending is preserved — same rule as
			// replaceBlock, same reason: rewriting an LF note as CRLF would put
			// every line of it in the diff for a one-line change.
			nl := "\n"
			if strings.Contains(text, "\r\n") {
				nl = "\r\n"
			}
			result := strings.Join(out, nl)
			if strings.HasSuffix(text, "\n") {
				result += nl
			}
			if err := os.WriteFile(note.Abs, []byte(result), 0o644); err != nil {
				env.errorf("HATA: %s yazilamadi: %s\n", note.Rel, err)
				return 2
			}
		}
		changed++
	}

	suffix := ""
	if opts.Dry {
		suffix = " (KURU KOSUM — dosya yazilmadi)"
	}
	env.printf("\n%d not cevrildi, %d atlandi%s\n", changed, skipped, suffix)
	if changed > 0 && write {
		env.printf("Not: tuketici repolarin context.yaml'inda `role:` yerine `scope:` " +
			"kullanabilirsiniz (role hala okunuyor).\n")
	}
	return 0
}

// splitLines mirrors Python's str.splitlines() for the endings that occur in
// notes: the trailing "\r" of a CRLF file is dropped, so rejoining with "\n"
// normalises the file — which is exactly what the Python implementation does.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // a trailing newline is not an empty line
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}
