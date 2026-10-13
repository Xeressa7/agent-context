package core

import "strings"

// Validity is described by TWO independent axes:
//
//	Status: "active" | "archived"  — does the note describe today?
//	Scope:  free-form tags         — where does it apply? EMPTY MEANS UNIVERSAL
//
// The previous model had a single axis (lifecycle: product|current|legacy|
// historical) whose current/legacy split came from the story of the project
// that produced this tool. A single-era project had a dead concept; a
// three-era project had no room. Scope tags are the PROJECT's vocabulary now,
// not the tool's.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// legacyLifecycle maps the retired single-axis field onto the two axes, so old
// notes keep working unchanged.
var legacyLifecycle = map[string]struct {
	status string
	scope  []string
}{
	"product":    {StatusActive, nil},
	"historical": {StatusArchived, nil},
	"current":    {StatusActive, []string{"current"}},
	"legacy":     {StatusActive, []string{"legacy"}},
}

// SplitList normalizes a list field.
//
// The two parsers produce two shapes: the manifest reader yields a real list,
// the frontmatter reader yields "a, b" as one string. Splitting on commas reads
// both correctly — without it "server, worker" is taken as ONE tag and scope
// matching fails silently.
func SplitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validity returns the (status, scope) pair for a note's front matter,
// translating the retired lifecycle field when the new ones are absent.
func Validity(fm map[string]string) (string, []string) {
	status := strings.TrimSpace(fm["status"])
	scope := SplitList(fm["scope"])
	if status != "" || len(scope) > 0 {
		if status == "" {
			status = StatusActive
		}
		return status, scope
	}
	if mapped, ok := legacyLifecycle[strings.TrimSpace(fm["lifecycle"])]; ok {
		return mapped.status, mapped.scope
	}
	return StatusActive, nil
}

// Visible reports whether a note should be read in the given repo scope.
//
// Two sentences: an archived note does not appear unless asked for; a note
// without scope is universal, a note with scope appears only on intersection.
// An undefined repo scope disables filtering entirely, so a single-era project
// never has to write scope at all.
func Visible(fm map[string]string, repoScope []string, includeArchived bool) bool {
	status, noteScope := Validity(fm)
	if status == StatusArchived && !includeArchived {
		return false
	}
	if len(noteScope) == 0 || len(repoScope) == 0 {
		return true
	}
	return intersects(noteScope, repoScope)
}

// ComponentsMatch reports whether a note belongs on a repo's MAP by component.
//
// Same rule as scope, applied to a different axis: a note without components
// is universal, a repo without components applies no filter, otherwise the
// two must intersect. It is deliberately NOT part of Visible: components say
// where a note is most useful, not where it is true, so search keeps every
// component and only the always-loaded map — which costs every session —
// narrows by it, and only when the repo opts in.
func ComponentsMatch(fm map[string]string, repoComponents []string) bool {
	noteComponents := SplitList(fm["components"])
	if len(noteComponents) == 0 || len(repoComponents) == 0 {
		return true
	}
	return intersects(noteComponents, repoComponents)
}

func intersects(a, b []string) bool {
	in := make(map[string]struct{}, len(b))
	for _, s := range b {
		in[s] = struct{}{}
	}
	for _, s := range a {
		if _, ok := in[s]; ok {
			return true
		}
	}
	return false
}

// LegacyLifecycle reports whether a retired lifecycle value is one this tool
// ever defined, so validation can tell "misspelled" from "outdated".
func LegacyLifecycle(value string) (string, bool) {
	mapped, ok := legacyLifecycle[strings.TrimSpace(value)]
	return mapped.status, ok
}

// LegacyScope returns the scope tags a retired lifecycle value maps to.
func LegacyScope(value string) []string {
	return legacyLifecycle[strings.TrimSpace(value)].scope
}
