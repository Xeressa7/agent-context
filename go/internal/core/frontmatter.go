package core

import "strings"

// Front matter is parsed with a deliberately small hand-written reader, not a
// YAML library: the whole tool depends on stdlib only, and a note's header is a
// flat key/value block by contract (SPEC.md). Anything a real YAML parser would
// add here — anchors, block scalars, nested maps — is something the format does
// not allow in the first place.
//
// Locked by tests/vectors/frontmatter.json, the same file the Python
// implementation reads.

// ParseFrontMatter returns the note's header fields, or an empty map when the
// text has no front matter or the block is never closed.
//
// Values are flattened to strings on purpose: a list `[a, b]` becomes "a, b",
// which SplitList then splits back. Both parsers (this one and the manifest
// reader) therefore feed SplitList a shape it understands.
func ParseFrontMatter(text string) map[string]string {
	fields := map[string]string{}
	if !strings.HasPrefix(text, "---") {
		return fields
	}
	end := strings.Index(text[3:], "\n---")
	if end == -1 {
		return fields
	}
	for _, line := range strings.Split(text[3:3+end], "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		// A line starting with space, tab or dash is a nested value or a list
		// item — the flat contract says it belongs to no key here, so it is
		// skipped rather than guessed at.
		if line[0] == ' ' || line[0] == '\t' || line[0] == '-' {
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		value = strings.Trim(value, "'")
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			parts := strings.Split(value[1:len(value)-1], ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			value = strings.Join(parts, ", ")
		}
		fields[strings.TrimSpace(key)] = value
	}
	return fields
}

// SplitBody drops the front matter and returns the prose below it. Scoring runs
// on the body alone so that a header field never counts twice — once as its own
// weighted field and again as body text.
func SplitBody(text string) string {
	if !strings.HasPrefix(text, "---") {
		return text
	}
	end := strings.Index(text[3:], "\n---")
	if end == -1 {
		return text
	}
	nl := strings.Index(text[3+end+1:], "\n")
	if nl == -1 {
		return ""
	}
	return text[3+end+1+nl+1:]
}
