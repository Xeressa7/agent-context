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
	name         string
	legacyScript string
	message      string
	timeout      int
}

var hookEntries = []hookEntry{
	{"capture-check", "ctx_capture_check.py", "Capture karari kontrol ediliyor...", 20},
	{"index", "ctx_index.py", "Hafiza haritasi yukleniyor...", 20},
}

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

	memoryRel := opts.MemoryPath
	if memoryRel == "" {
		memoryRel = "../" + opts.Project + "-memory"
	}
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
		env.printf("  ctx init -> %s + context.yaml + AGENTS.md + CLAUDE.md\n", memory)
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

	sessionStart := settings.Object("hooks").Array("SessionStart")
	if len(sessionStart) == 0 {
		group := jsonx.NewObject()
		group.Set("hooks", []any{})
		sessionStart = append(sessionStart, group)
	}
	group, ok := sessionStart[0].(*jsonx.Object)
	if !ok {
		return append(report, "! settings.json okunamadi (SessionStart bicimi beklenmedik)"+
			" — hook KAYDEDILMEDI, elle ekleyin")
	}
	list := group.Array("hooks")

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
		want := hookCommand(hook.name)

		var found *jsonx.Object
		var current string
		for _, item := range list {
			entry, cmd := command(item)
			if entry == nil {
				continue
			}
			if strings.Contains(cmd, hook.legacyScript) || strings.Contains(cmd, "hook "+hook.name) {
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
			report = append(report, "+ settings.json: "+hook.name+" eklendi")
		case current != want:
			// IN PLACE: the user's ordering and any other fields they set are
			// left exactly as they were.
			found.Set("command", want)
			changed = true
			report = append(report, "↻ settings.json: "+hook.name+" → `ctx hook "+hook.name+"` olarak guncellendi")
		default:
			report = append(report, "= settings.json: "+hook.name+" kayitli")
		}
	}
	group.Set("hooks", list)
	settings.Object("hooks").Set("SessionStart", sessionStart)

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
