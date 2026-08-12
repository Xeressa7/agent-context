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
	in := make(map[string]struct{}, len(repoScope))
	for _, s := range repoScope {
		in[s] = struct{}{}
	}
	for _, s := range noteScope {
		if _, ok := in[s]; ok {
			return true
		}
	}
	return false
}
