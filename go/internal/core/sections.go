package core

import "strings"

// Note layout: what is true TODAY is kept apart from how it got there.
//
// Measured on yapit (2026-10-13, 204 notes): decisions changed by being
// appended to — 315 headings in 111 notes carried a date or a status word
// ("UYGULANDI 2026-09-08"), and a reversed decision lived as a parenthesis in
// the middle of the line that still stated the old one. Search matched the
// whole body, so a historical paragraph surfaced as if it were current.
//
// The layout is two optional level-2 sections: `## Geçerli` (only what holds
// now, rewritten when it changes) and `## Tarihçe` (one line per change, kept
// for "why did this change?"). Headings are matched folded, so `Gecerli`,
// `GEÇERLİ` and the English `Current`/`History` all count.

var currentHeadings = []string{"gecerli", "current"}
var historyHeadings = []string{"tarihce", "history", "changelog"}

// headingLevel returns the markdown heading level of a line (0 = not one) and
// its text.
func headingLevel(line string) (int, string) {
	trimmed := strings.TrimRight(line, " \t\r")
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(trimmed[level:])
}

func headingIs(text string, names []string) bool {
	folded := Fold(text)
	for _, n := range names {
		if strings.HasPrefix(folded, n) {
			return true
		}
	}
	return false
}

// CurrentBody drops every `## Tarihçe` section (up to the next heading of
// level 1 or 2). Search and recall score this by default: history is for the
// question "why did it change", not for "how does it work".
func CurrentBody(body string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		if level, text := headingLevel(line); level > 0 && level <= 2 {
			skipping = level == 2 && headingIs(text, historyHeadings)
		}
		if !skipping {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// HasCurrentSection reports whether the note opens what holds today under a
// `## Geçerli` heading.
func HasCurrentSection(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if level, text := headingLevel(line); level == 2 && headingIs(text, currentHeadings) {
			return true
		}
	}
	return false
}

// Headings returns the heading texts outside the history sections, with
// their levels. Maintenance reads them for dated/status headings.
func Headings(body string) []string {
	var out []string
	for _, line := range strings.Split(CurrentBody(body), "\n") {
		if level, text := headingLevel(line); level >= 2 {
			out = append(out, text)
		}
	}
	return out
}
