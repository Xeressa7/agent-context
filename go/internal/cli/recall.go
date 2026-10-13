package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/core"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// RECALL BY FILE: when the agent touches a file, the notes that name that file
// are put in front of it.
//
// Why a file and not the user's message: both were measured on yapit
// (2026-10-13, 204 notes, 268 real messages). Word matching on the message
// injected notes into 76 messages and most were unrelated — the messages are
// conversational ("bunu da commitle", "o işi bi incele") and name their topic
// rarely. A file path is not fuzzy: 174 of the 204 notes name at least one code
// file, and 416 of the 593 named files appear in exactly ONE note. Touching
// `seviye_talebi_service.py` finds `hakedis.md` and nothing else.
//
// This is also the cheapest answer to stale memory: the note describing a file
// surfaces exactly when that file is being read or changed — the moment a stale
// claim can do damage, and the moment it can be corrected.
//
// Like every hook: silent on any failure, exit 0.

const (
	recallEvent = "PostToolUse"
	recallTop   = 2
	// recallMaxNotes: a bare file name named by more notes than this is too
	// generic to point anywhere (yapit: `main.ts` 20 notes, `common.py` 14).
	// A note naming the file by a PATH (`app/api/x.py`) still counts.
	recallMaxNotes = 3
	// recallCatalog: a note naming more distinct files than this is a catalog
	// (yapit: the idea pool names 62; the median note names 4). It is found by
	// path, never by a bare name — otherwise it surfaces for half the code.
	recallCatalog = 50
)

// genericNames are file names that say nothing about WHICH file: matched only
// by path.
var genericNames = map[string]bool{
	"__init__.py": true, "conftest.py": true, "settings.py": true, "config.py": true,
	"utils.py": true, "utils.js": true, "common.py": true, "models.py": true, "schemas.py": true,
	"db.py": true, "api.js": true, "api.py": true, "main.py": true, "main.ts": true, "main.js": true,
	"index.js": true, "index.ts": true, "index.tsx": true, "app.py": true, "app.js": true,
	"package.json": true, "readme.md": true, "agents.md": true, "types.ts": true, "constants.js": true,
}

// fileRefPattern finds backticked spans that name a file with an extension,
// optionally followed by `:line` or a symbol.
var fileRefPattern = regexp.MustCompile("`([^`\\s:]*[A-Za-z0-9_\\-]\\.[A-Za-z0-9]{1,5})(?:[:\\s][^`]*)?`")

// fileRefs returns the file references in a note body, lower-cased and
// slash-normalized.
func fileRefs(body string) []string {
	var out []string
	for _, m := range fileRefPattern.FindAllStringSubmatch(body, -1) {
		ref := strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(m[1], `\`, "/")), "./")
		if strings.Contains(ref, "://") {
			continue
		}
		out = append(out, ref)
	}
	return out
}

type recallHit struct {
	rel, description string
	byPath           bool // matched a path, not just a bare name
	refs             int  // distinct files the note names: fewer = more specific
}

// RecallForFile lists the notes that name a file, path matches first. Exported
// for adapters (a Python harness, an MCP server) that want the hook's answer.
func RecallForFile(ctx store.Context, file string) ([]recallHit, error) {
	target := strings.ToLower(strings.ReplaceAll(file, `\`, "/"))
	base := target[strings.LastIndex(target, "/")+1:]
	if !strings.Contains(base, ".") {
		return nil, nil
	}
	// A note is never recalled for itself: editing memory is not code work.
	if mem, err := filepath.Abs(ctx.MemoryPath); err == nil {
		if abs, err := filepath.Abs(file); err == nil &&
			strings.HasPrefix(strings.ToLower(abs), strings.ToLower(mem)+string(filepath.Separator)) {
			return nil, nil
		}
	}

	notes, err := store.LoadNotes(ctx.MemoryPath)
	if err != nil {
		return nil, err
	}
	scope := store.RepoScope(ctx.Data)
	var byPath, byName []recallHit
	for _, n := range notes {
		if !core.Visible(n.FrontMatter, scope, false) {
			continue
		}
		pathHit, nameHit := false, false
		refs := fileRefs(n.Body)
		distinct := map[string]bool{}
		for _, ref := range refs {
			distinct[ref[strings.LastIndex(ref, "/")+1:]] = true
		}
		for _, ref := range refs {
			refBase := ref[strings.LastIndex(ref, "/")+1:]
			switch {
			case strings.Contains(ref, "/") && (target == ref || strings.HasSuffix(target, "/"+ref)):
				pathHit = true
			case refBase == base && !genericNames[base] && len(distinct) <= recallCatalog:
				nameHit = true
			}
		}
		hit := recallHit{rel: n.Rel, description: n.FrontMatter["description"],
			byPath: pathHit, refs: len(distinct)}
		switch {
		case pathHit:
			byPath = append(byPath, hit)
		case nameHit:
			byName = append(byName, hit)
		}
	}
	if len(byName)+len(byPath) > recallMaxNotes {
		byName = nil // generic name: only path matches are trusted
	}
	if len(byPath) > recallMaxNotes {
		return nil, nil // pointing everywhere is pointing nowhere
	}
	hits := append(byPath, byName...)
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].byPath != hits[j].byPath {
			return hits[i].byPath
		}
		if hits[i].refs != hits[j].refs {
			return hits[i].refs < hits[j].refs
		}
		return hits[i].rel < hits[j].rel
	})
	return hits, nil
}

// HookRecall is the PostToolUse hook for the file tools (Read/Edit/Write).
func HookRecall(env Env, repo string) int {
	raw, err := io.ReadAll(env.stdin())
	if err != nil || len(raw) == 0 {
		return 0
	}
	var payload struct {
		SessionID string         `json:"session_id"`
		Cwd       string         `json:"cwd"`
		ToolInput map[string]any `json:"tool_input"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return 0
	}
	file := ""
	for _, key := range []string{"file_path", "notebook_path"} {
		if s, _ := payload.ToolInput[key].(string); s != "" {
			file = s
			break
		}
	}
	if file == "" {
		return 0
	}
	if repo == "" {
		repo = payload.Cwd
	}
	ctx, err := store.LoadContext(repo)
	if err != nil || !autoRecallOn(ctx.Data) {
		return 0
	}
	hits, err := RecallForFile(ctx, file)
	if err != nil || len(hits) == 0 {
		return 0
	}

	// A note already shown in this session is not shown again: it is still in
	// the context, and repeating it is pure cost.
	seenPath := recallSeenPath(payload.SessionID)
	seen := map[string]bool{}
	if seenPath != "" {
		if b, err := os.ReadFile(seenPath); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				seen[l] = true
			}
		}
	}
	base := filepath.Dir(ctx.ManifestPath)
	var lines, added []string
	for _, h := range hits {
		if len(lines) == recallTop {
			break
		}
		if seen[h.rel] {
			continue
		}
		shown := filepath.Join(ctx.MemoryPath, filepath.FromSlash(h.rel))
		if rel, err := filepath.Rel(base, shown); err == nil {
			shown = rel
		}
		lines = append(lines, "- `"+filepath.ToSlash(shown)+"` — "+shorten(h.description, 110))
		added = append(added, h.rel)
	}
	if len(lines) == 0 {
		return 0
	}
	if seenPath != "" {
		if f, err := os.OpenFile(seenPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.WriteString(strings.Join(added, "\n") + "\n")
			f.Close()
		}
	}

	out, err := hookOutputFor(recallEvent,
		"HAFIZA — `"+filepath.Base(file)+"` dosyasini anan not(lar):\n"+
			strings.Join(lines, "\n")+"\n"+
			"Bu dosyada is yapiyorsan notu AC. Not kodla celisiyorsa kod otoriterdir: "+
			"notu duzelt ya da kullaniciya bildir.")
	if err != nil {
		return 0
	}
	env.printf("%s\n", out)
	return 0
}

// recallSeenPath is the per-session record of shown notes. It lives in the OS
// temp dir: it is session state, not memory, and must never be committed.
func recallSeenPath(session string) string {
	if session == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(session))
	return filepath.Join(os.TempDir(), "ctx-recall-"+hex.EncodeToString(sum[:8])+".txt")
}

// autoRecallOn reports whether recall runs (`auto_recall`, default on).
func autoRecallOn(m store.Manifest) bool {
	return !autoOffValues[strings.ToLower(strings.TrimSpace(m.Str("auto_recall")))]
}
