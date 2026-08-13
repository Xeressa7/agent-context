package store

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Xeressa7/agent-context/go/internal/core"
)

// MemoryConfigName is the memory repo's OWN settings file.
//
// Why it does not live in the consumer's context.yaml: these are properties of
// the MEMORY (which folders are not notes, whose personal data is stored), not
// of the repo reading it. The pre-commit gate also runs from inside the memory
// repo with no manifest in sight, and it has to find the settings there.
const MemoryConfigName = "ctx.yaml"

// structuralSkips are the directories that are never notes. Only what SPEC.md
// defines as structural belongs here: tasks/ and sessions/ hold STATE rather
// than durable knowledge, .git is plumbing. Project-specific folders go to the
// memory's own ctx.yaml instead — a hard-coded name here would silently hide
// another project's notes.
var structuralSkips = []string{"tasks", "sessions", ".git"}

// MemoryConfig reads the memory's own settings; a missing or unreadable file is
// not an error, it just means defaults.
func MemoryConfig(memory string) Manifest {
	m, err := ReadManifest(filepath.Join(memory, MemoryConfigName))
	if err != nil {
		return Manifest{}
	}
	return m
}

// SkipDirs is the structural set plus the memory's own skip_dirs entries.
func SkipDirs(memory string) map[string]struct{} {
	skip := make(map[string]struct{}, len(structuralSkips)+2)
	for _, d := range structuralSkips {
		skip[d] = struct{}{}
	}
	for _, d := range MemoryConfig(memory).List("skip_dirs") {
		skip[strings.Trim(d, "/")] = struct{}{}
	}
	return skip
}

// NoteFile is one note on disk: absolute path plus the slash-separated path
// relative to the memory root, which is what scoring, the map and validation
// all display.
type NoteFile struct {
	Abs string
	Rel string
}

// WalkNotes lists the notes under a memory root, in a deterministic order.
//
// Skipped: anything under a skip directory, and any file sitting directly in
// the root (README.md and friends are the memory's own scaffolding, not notes).
//
// Ordering is by the slash-separated relative path, byte-wise. The Python
// implementation sorts pathlib.Path objects, which on Windows compares
// lowercased backslash paths — the two rules only diverge when a directory name
// and a file name differ at the separator position, which no note tree here
// does. Written down because it is a real (if narrow) seam between the two
// implementations.
func WalkNotes(memory string) ([]NoteFile, error) {
	skip := SkipDirs(memory)
	var notes []NoteFile

	err := filepath.WalkDir(memory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(memory, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if _, hit := skip[d.Name()]; hit {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		slashed := filepath.ToSlash(rel)
		if !strings.Contains(slashed, "/") {
			return nil // sits in the memory root: scaffolding, not a note
		}
		notes = append(notes, NoteFile{Abs: path, Rel: slashed})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Rel < notes[j].Rel })
	return notes, nil
}

// Read returns a note's text with invalid bytes replaced, mirroring the Python
// reader's errors="replace": a single corrupt file must not abort a whole scan.
func (n NoteFile) Read() (string, error) {
	raw, err := os.ReadFile(n.Abs)
	if err != nil {
		return "", err
	}
	return decode(raw), nil
}

// Note bundles a file with its parsed header and body, which is what every
// command past this layer actually wants.
type Note struct {
	NoteFile
	Text        string
	FrontMatter map[string]string
	Body        string
}

// bodyLimit caps how much prose is scored. Matches the Python reader: a runaway
// file must not turn one search into a full-corpus scan.
const bodyLimit = 100_000

// LoadNotes reads and parses every note under a memory root.
func LoadNotes(memory string) ([]Note, error) {
	files, err := WalkNotes(memory)
	if err != nil {
		return nil, err
	}
	notes := make([]Note, 0, len(files))
	for _, f := range files {
		text, err := f.Read()
		if err != nil {
			return nil, err
		}
		// Counted in runes, not bytes: Python slices characters, and a
		// byte-wise cut would both truncate earlier and split a rune.
		body := core.SplitBody(text)
		if utf8.RuneCountInString(body) > bodyLimit {
			body = string([]rune(body)[:bodyLimit])
		}
		notes = append(notes, Note{
			NoteFile:    f,
			Text:        text,
			FrontMatter: core.ParseFrontMatter(text),
			Body:        body,
		})
	}
	return notes, nil
}

// decode turns file bytes into a string, replacing invalid UTF-8 rather than
// failing.
func decode(raw []byte) string {
	return strings.ToValidUTF8(string(raw), "�")
}
