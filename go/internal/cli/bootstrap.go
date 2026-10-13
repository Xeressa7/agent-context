package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/jsonx"
)

// BootstrapOptions mirrors the Python `ctx bootstrap` flags.
type BootstrapOptions struct {
	InitOptions
	Dry bool // --kuru
}

// hookEntry is one SessionStart hook.
//
// legacyScript is kept for MIGRATION only: before 2026-08-13 the hooks were two
// Python files registered as `python ".../ctx_index.py"`. The registration now
// calls the ctx wrapper directly, but an old entry has to be recognised and
// updated IN PLACE — otherwise the same hook would run twice.
type hookEntry struct {
	event        string
	name         string
	legacyScript string
	message      string
	timeout      int
	matcher      string // tool-name filter; "" = the event's first group
}

var hookEntries = []hookEntry{
	{"SessionStart", "capture-check", "ctx_capture_check.py", "Capture karari kontrol ediliyor...", 20, ""},
	{"SessionStart", "index", "ctx_index.py", "Hafiza haritasi yukleniyor...", 20, ""},
	// Close and compaction: the session digest is dropped to disk. The model
	// has to remember nothing — MECHANISM, not discipline. The transcript path
	// arrives on stdin and no model is called, so it returns fast.
	{"SessionEnd", "flush --arac claude --sebep sessionend", "", "Oturum dokumu kaydediliyor...", 15, ""},
	{"PreCompact", "flush --arac claude --sebep precompact", "", "Oturum dokumu kaydediliyor...", 15, ""},
	// A file the agent reads or edits brings the notes that name it. Only the
	// file tools: a matcher-less PostToolUse would spawn ctx on every Bash call.
	{"PostToolUse", "recall", "", "Hafiza taraniyor...", 10, recallMatcher},
}

const recallMatcher = "Read|Edit|MultiEdit|Write|NotebookEdit"

// ctxWrapper is the single entry point hooks call. The WRAPPER is registered,
// not an implementation: bin/ctx picks the compiled binary when there is one
// and Python otherwise, so settings.json survives the migration either way.
func ctxWrapper() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".agent-context", "bin", "ctx")
	}
	return filepath.Join(home, ".agent-context", "bin", "ctx")
}

func hookCommand(name string) string {
	return `"` + ctxWrapper() + `" hook ` + name
}

// Bootstrap sets a new project up in one command.
//
// The design promise is IDEMPOTENT and NON-DESTRUCTIVE: no existing file is
// overwritten (settings.json only gains the entry it is missing), and a second
// run prints "all current". --kuru shows what it would do and writes nothing.
func Bootstrap(env Env, opts BootstrapOptions) int {
	write := !opts.Dry
	if opts.Dry {
		env.printf("KURU KOSUM — hicbir dosya yazilmayacak\n\n")
	}

	cwd, err := os.Getwd()
	if err != nil {
		env.errorf("HATA: calisma dizini okunamadi: %s\n", err)
		return 2
	}
	env.printf("proje kokku: %s\n", cwd)

	memoryRel := memoryRelPath(opts.InitOptions)
	memory := memoryRel
	if !filepath.IsAbs(memory) {
		memory = filepath.Join(cwd, filepath.FromSlash(memoryRel))
	}
	if abs, err := filepath.Abs(memory); err == nil {
		memory = abs
	}

	env.printf("\n[1/4] hafiza + adapter\n")
	if write {
		if code := Init(env, opts.InitOptions); code != 0 {
			return code
		}
	} else {
		env.printf("  ctx init -> %s + context.yaml + AGENTS.md + CLAUDE.md + .agents/skills (3 skill)\n", memory)
		if entry, status, _ := ensureGitignored(cwd, memoryRel, false); status == "olusturuldu" || status == "eklendi" {
			env.printf("  .gitignore -> %s\n", entry)
		}
	}

	// 1b) bu repo hafızanın consumers listesine kaydolur — liste elle tutulursa
	// yeni repo unutulur ve haritası sessizce bayatlar.
	for _, line := range registerConsumer(memory, cwd, write) {
		env.printf("   %s\n", line)
	}

	env.printf("\n[2/4] SessionStart kancalari\n")
	for _, line := range installHooks(write) {
		env.printf("   %s\n", line)
	}

	env.printf("\n[3/4] kalite kapisi (pre-commit)\n")
	for _, line := range installQualityGate(memory, write) {
		env.printf("   %s\n", line)
	}

	env.printf("\n[4/4] harita + saglik\n")
	if opts.Dry {
		env.printf("   (kuru kosum — index/doctor atlandi)\n")
		return 0
	}
	// The block is written even for an EMPTY corpus: the markers should be in
	// the consumer files from the start, so there is somewhere to refresh when
	// the first note appears.
	Index(env, IndexOptions{Write: true})
	code := Doctor(env, "")

	env.printf("\nKALAN ELLE ADIM:\n")
	env.printf("  · hafiza reposu icin uzak depo (gh repo create <ad>-memory --private) — PII taramasi sonrasi\n")
	env.printf("  · `ctx` PATH'te degilse: bin/ dizinini PATH'e ekleyin\n")
	return code
}

// claudeDir is the harness's own config directory.
func claudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// installHooks copies the SessionStart hooks and REGISTERS them.
//
// Two separate things: copying the file (easy) and telling the harness about it
// (the actual work). settings.json carries the user's OWN settings — it is
// never overwritten, only the missing entry is added, so a second run changes
// nothing at all.
func installHooks(write bool) []string {
	var report []string
	settingsPath := filepath.Join(claudeDir(), "settings.json")
	if write {
		os.MkdirAll(claudeDir(), 0o755)
	}
	settings := jsonx.NewObject()
	if raw, err := os.ReadFile(settingsPath); err == nil {
		parsed, err := jsonx.Parse(raw)
		if err != nil {
			return append(report, "! settings.json okunamadi ("+err.Error()+
				") — hook KAYDEDILMEDI, elle ekleyin")
		}
		settings = parsed
	}

	command := func(item any) (*jsonx.Object, string) {
		entry, ok := item.(*jsonx.Object)
		if !ok {
			return nil, ""
		}
		if cmd, ok := entry.Get("command"); ok {
			if s, ok := cmd.(string); ok {
				return entry, s
			}
		}
		return entry, ""
	}

	changed := false
	for _, hook := range hookEntries {
		bucket := settings.Object("hooks").Array(hook.event)
		at := 0
		if hook.matcher != "" {
			// A matcher hook lives in its OWN group: the user's groups for
			// other tools are never touched.
			at = -1
			for i, item := range bucket {
				if g, ok := item.(*jsonx.Object); ok {
					if m, _ := g.Get("matcher"); m == hook.matcher {
						at = i
						break
					}
				}
			}
			if at < 0 {
				g := jsonx.NewObject()
				g.Set("matcher", hook.matcher)
				g.Set("hooks", []any{})
				bucket = append(bucket, g)
				at = len(bucket) - 1
			}
		} else if len(bucket) == 0 {
			g := jsonx.NewObject()
			g.Set("hooks", []any{})
			bucket = append(bucket, g)
		}
		group, ok := bucket[at].(*jsonx.Object)
		if !ok {
			return append(report, "! settings.json okunamadi ("+hook.event+
				" bicimi beklenmedik) — hook KAYDEDILMEDI, elle ekleyin")
		}
		list := group.Array("hooks")
		want := hookCommand(hook.name)
		// Matching looks at the hook name's FIRST WORD; the flags after it
		// (`--arac ...`) can change later, and when they do the entry must be
		// updated IN PLACE, never appended a second time.
		root := strings.Fields(hook.name)[0]

		var found *jsonx.Object
		var current string
		for _, item := range list {
			entry, cmd := command(item)
			if entry == nil {
				continue
			}
			if (hook.legacyScript != "" && strings.Contains(cmd, hook.legacyScript)) ||
				strings.Contains(cmd, "hook "+root) {
				found, current = entry, cmd
				break
			}
		}

		switch {
		case found == nil:
			entry := jsonx.NewObject()
			entry.Set("type", "command")
			entry.Set("command", want)
			entry.Set("shell", "bash")
			entry.Set("timeout", json.Number(itoa(hook.timeout)))
			entry.Set("statusMessage", hook.message)
			list = append(list, entry)
			changed = true
			report = append(report, "+ settings.json["+hook.event+"]: "+root+" eklendi")
		case current != want:
			// IN PLACE: the user's ordering and any other fields they set are
			// left exactly as they were.
			found.Set("command", want)
			changed = true
			report = append(report, "↻ settings.json["+hook.event+"]: "+root+" guncellendi")
		default:
			report = append(report, "= settings.json["+hook.event+"]: "+root+" kayitli")
		}
		group.Set("hooks", list)
		settings.Object("hooks").Set(hook.event, bucket)
	}

	if changed && write {
		if text, err := jsonx.Marshal(settings); err == nil {
			writeText(settingsPath, text)
		}
	}
	return report
}

// installQualityGate wires the memory repo's pre-commit gate.
//
// The hook carries NO validation logic: it calls the one engine, `ctx validate`.
func installQualityGate(memory string, write bool) []string {
	var report []string
	if _, err := os.Stat(filepath.Join(memory, ".git")); err != nil {
		return append(report, "! hafiza bir git reposu degil — kalite kapisi atlandi")
	}
	text, err := template("hooks/pre-commit")
	if err != nil {
		return append(report, "! pre-commit sablonu yok")
	}

	hookDir := filepath.Join(memory, ".githooks")
	target := filepath.Join(hookDir, "pre-commit")
	if existing, err := os.ReadFile(target); err == nil &&
		normalizeNewlines(string(existing)) == normalizeNewlines(text) {
		report = append(report, "= .githooks/pre-commit (guncel)")
	} else {
		if write {
			os.MkdirAll(hookDir, 0o755)
			writeText(target, text)
		}
		report = append(report, "+ .githooks/pre-commit")
	}

	current := git(memory, "config", "--get", "core.hooksPath")
	switch {
	case current == ".githooks":
		report = append(report, "= core.hooksPath (.githooks)")
	case current != "":
		// Somebody else's hooks path: LEAVE IT. Silently repointing a repo's
		// hooks is the kind of help nobody asked for.
		report = append(report, "! core.hooksPath baska bir degerde ("+current+") — DOKUNULMADI")
	default:
		if write {
			exec.Command("git", "-C", memory, "config", "core.hooksPath", ".githooks").Run()
		}
		report = append(report, "+ core.hooksPath = .githooks")
	}
	return report
}

// normalizeNewlines makes the "is it already current?" check independent of
// line endings, mirroring Python's universal-newline read. Without it the
// comparison would fail forever on Windows: the file is WRITTEN in text mode
// (CRLF) and the template is read as LF, so every run would rewrite it.
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
