// Package store is the filesystem layer: it finds the manifest, reads the
// memory's own settings, and walks the note tree. Everything language-neutral
// about behaviour lives in internal/core; this package only decides WHICH bytes
// core gets to see.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
)

// ManifestName is the consumer repo's pointer at its memory.
const ManifestName = "context.yaml"

// Value is one manifest entry. The format is a flat `key: value` file with at
// most one level of indentation, so three shapes cover it: a scalar, a bracket
// list, and a nested block. A key with an empty value parses as an empty block,
// matching the Python reader.
type Value struct {
	Text   string
	Items  []string
	Fields map[string]Value
	// FieldOrder keeps nested keys in file order. Python dicts preserve
	// insertion order and `ctx doctor` prints a manifest value through repr(),
	// so a Go map's random iteration would produce a different report for the
	// same file.
	FieldOrder []string
	IsList     bool
}

// Manifest is a parsed manifest file.
type Manifest map[string]Value

// Str returns a scalar entry, or "" when the key is absent or not a scalar.
func (m Manifest) Str(key string) string {
	v, ok := m[key]
	if !ok || v.IsList {
		return ""
	}
	return v.Text
}

// List returns a list entry. A bracket list is used as-is; a scalar is split on
// commas, so `scope: a, b` and `scope: [a, b]` mean the same thing. Without
// this, "server, worker" would be taken as ONE tag and scope matching would
// fail silently.
func (m Manifest) List(key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	if v.IsList {
		out := make([]string, 0, len(v.Items))
		for _, item := range v.Items {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return core.SplitList(v.Text)
}

// Int returns an integer entry, falling back to def when absent or unreadable.
func (m Manifest) Int(key string, def int) int {
	raw := strings.TrimSpace(m.Str(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// ParseManifest reads the flat `key: value` format with one level of nesting.
//
// Comment stripping is naive by design (everything after the first '#' goes):
// the format has no quoted strings, so there is no case where a '#' inside a
// value is meaningful.
func ParseManifest(text string) Manifest {
	data := Manifest{}
	parent := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(strings.SplitN(raw, "#", 2)[0], " \t\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if !strings.Contains(line, ":") {
			continue
		}
		key, rest, _ := strings.Cut(strings.TrimSpace(line), ":")
		key, rest = strings.TrimSpace(key), strings.TrimSpace(rest)

		value := Value{Text: rest}
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			value.IsList = true
			for _, p := range strings.Split(rest[1:len(rest)-1], ",") {
				if s := strings.TrimSpace(p); s != "" {
					value.Items = append(value.Items, s)
				}
			}
		}

		if indented && parent != "" {
			outer := data[parent]
			if outer.Fields == nil {
				outer.Fields = map[string]Value{}
			}
			if _, seen := outer.Fields[key]; !seen {
				outer.FieldOrder = append(outer.FieldOrder, key)
			}
			outer.Fields[key] = value
			data[parent] = outer
			continue
		}
		parent = key
		if rest == "" {
			value = Value{Fields: map[string]Value{}}
		}
		data[key] = value
	}
	return data
}

// ReadManifest parses a manifest file from disk.
func ReadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(decode(raw)), nil
}

// FindManifest walks up from start looking for context.yaml, returning "" when
// there is none.
func FindManifest(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ManifestName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Context is a resolved manifest: where it was found, what it said, and the
// absolute path to the memory it points at.
type Context struct {
	ManifestPath string
	Data         Manifest
	MemoryPath   string
}

// LoadContext resolves the manifest for a starting directory ("" means the
// working directory). Both failures are user errors, not internal ones, so they
// carry the message the CLI prints.
func LoadContext(start string) (Context, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Context{}, err
		}
		start = wd
	}
	path := FindManifest(start)
	if path == "" {
		return Context{}, fmt.Errorf("%s bulunamadi (%s'den yukari arandi)", ManifestName, start)
	}
	data, err := ReadManifest(path)
	if err != nil {
		return Context{}, err
	}
	rel := data.Str("memory_path")
	if rel == "" {
		return Context{}, fmt.Errorf("%s icinde memory_path yok", path)
	}
	memory := rel
	if !filepath.IsAbs(memory) {
		memory = filepath.Join(filepath.Dir(path), rel)
	}
	memory, err = filepath.Abs(memory)
	if err != nil {
		return Context{}, err
	}
	return Context{ManifestPath: path, Data: data, MemoryPath: memory}, nil
}

// Truthy reports whether an entry would be truthy in Python: a missing key, an
// empty string, an empty list and an empty block are all false.
func (m Manifest) Truthy(key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	if v.IsList {
		return len(v.Items) > 0
	}
	if v.Fields != nil {
		return len(v.Fields) > 0
	}
	return v.Text != ""
}

// Repr renders an entry the way Python's repr() does. `ctx doctor` prints
// manifest values through it, and the two implementations are compared
// verbatim, so the quoting rules have to match rather than merely look similar.
func (m Manifest) Repr(key string) string {
	v, ok := m[key]
	if !ok {
		return "None"
	}
	switch {
	case v.IsList:
		parts := make([]string, 0, len(v.Items))
		for _, item := range v.Items {
			parts = append(parts, PyRepr(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case v.Fields != nil:
		parts := make([]string, 0, len(v.FieldOrder))
		for _, k := range v.FieldOrder {
			inner := v.Fields[k]
			parts = append(parts, PyRepr(k)+": "+Manifest{k: inner}.Repr(k))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return PyRepr(v.Text)
	}
}

// PyRepr quotes a string the way Python's repr() does: single quotes, switching
// to double quotes when the value itself contains a single quote.
func PyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// RepoScope is the consuming repo's scope: the `scope` list, falling back to the
// retired single-valued `role` field.
func RepoScope(m Manifest) []string {
	if scope := m.List("scope"); len(scope) > 0 {
		return scope
	}
	if role := strings.TrimSpace(m.Str("role")); role != "" {
		return []string{role}
	}
	return nil
}
