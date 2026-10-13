package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	root "github.com/Xeressa7/agent-context"
)

// InitOptions mirrors the Python `ctx init` arguments.
type InitOptions struct {
	Project    string
	MemoryPath string
	Role       string
	Components string
}

// Init scaffolds a memory and the adapter files next to it.
//
// The promise is NON-DESTRUCTIVE: nothing that already exists is overwritten,
// and the summary says explicitly what was created and what was left alone. A
// setup command that silently clobbers an AGENTS.md is a command nobody dares
// run twice.
func Init(env Env, opts InitOptions) int {
	cwd, err := os.Getwd()
	if err != nil {
		env.errorf("HATA: calisma dizini okunamadi: %s\n", err)
		return 2
	}
	memoryRel := memoryRelPath(opts)
	memory := memoryRel
	if !filepath.IsAbs(memory) {
		memory = filepath.Join(cwd, filepath.FromSlash(memoryRel))
	}
	if abs, err := filepath.Abs(memory); err == nil {
		memory = abs
	}

	var created, skipped []string

	if _, err := os.Stat(memory); err == nil {
		skipped = append(skipped, memory+" (mevcut — dokunulmadi)")
	} else {
		if err := scaffoldMemory(memory, opts.Project); err != nil {
			env.errorf("HATA: hafiza iskeleti kurulamadi: %s\n", err)
			return 2
		}
		// git init but NO commit: the first commit is the user's call, after
		// they have looked at what is about to be tracked (PII lives here).
		exec.Command("git", "init", "--quiet", memory).Run()
		created = append(created, memory+" (iskelet + git init, COMMIT YOK)")
	}

	switch entry, status, err := ensureGitignored(cwd, memoryRel, true); {
	case err != nil:
		env.errorf("HATA: .gitignore yazilamadi: %s\n", err)
		return 2
	case status == "olusturuldu":
		created = append(created, ".gitignore ("+entry+")")
	case status == "eklendi":
		created = append(created, ".gitignore'a "+entry+" eklendi")
	case status == "mevcut":
		skipped = append(skipped, ".gitignore ("+entry+" zaten var)")
	}

	manifest := filepath.Join(cwd, "context.yaml")
	if _, err := os.Stat(manifest); err == nil {
		skipped = append(skipped, "context.yaml (mevcut)")
	} else {
		text, err := template("context.yaml")
		if err != nil {
			env.errorf("HATA: %s\n", err)
			return 2
		}
		text = strings.NewReplacer(
			"{project}", opts.Project,
			"{memory_path}", memoryRel,
			"{role}", opts.Role,
			"{components}", opts.Components,
		).Replace(text)
		if err := writeText(manifest, text); err != nil {
			env.errorf("HATA: context.yaml yazilamadi: %s\n", err)
			return 2
		}
		created = append(created, "context.yaml")
	}

	router, err := template("AGENTS-router.md")
	if err != nil {
		env.errorf("HATA: %s\n", err)
		return 2
	}
	router = strings.NewReplacer(
		"{memory_path}", memoryRel,
		"{filter}", "product + "+opts.Role,
	).Replace(router)

	agents := filepath.Join(cwd, "AGENTS.md")
	if _, err := os.Stat(agents); err == nil {
		skipped = append(skipped, "AGENTS.md (MEVCUT — DOKUNULMADI)")
		env.printf("AGENTS.md mevcut; router blogunu elle ekleyin:\n\n")
		env.printf("%s\n", router)
	} else {
		if err := writeText(agents, router); err != nil {
			env.errorf("HATA: AGENTS.md yazilamadi: %s\n", err)
			return 2
		}
		created = append(created, "AGENTS.md (router)")
	}

	claude := filepath.Join(cwd, "CLAUDE.md")
	if _, err := os.Stat(claude); err == nil {
		skipped = append(skipped, "CLAUDE.md (mevcut)")
	} else {
		text, err := template("CLAUDE.md")
		if err != nil {
			env.errorf("HATA: %s\n", err)
			return 2
		}
		if err := writeText(claude, text); err != nil {
			env.errorf("HATA: CLAUDE.md yazilamadi: %s\n", err)
			return 2
		}
		created = append(created, "CLAUDE.md")
	}

	// Modelin cagirdigi fiiller: arama, yazma, kapatma. Harnness'lar
	// `.agents/skills/` altindaki SKILL.md'leri dosyadan kesfeder; mevcut
	// icerik ASLA ezilmez (ikinci kosum "mevcut" der, farkli icerige dokunmaz).
	for _, name := range skillNames {
		fresh, err := template("skills/" + name + "/SKILL.md")
		if err != nil {
			skipped = append(skipped, ".agents/skills/"+name+"/SKILL.md (sablon yok — ATLANDI)")
			continue
		}
		target := filepath.Join(cwd, ".agents", "skills", name, "SKILL.md")
		raw, err := os.ReadFile(target)
		switch {
		case err == nil && normalizeNewlines(string(raw)) == normalizeNewlines(fresh):
			skipped = append(skipped, ".agents/skills/"+name+"/SKILL.md (mevcut)")
		case err == nil || !os.IsNotExist(err):
			skipped = append(skipped, ".agents/skills/"+name+"/SKILL.md (MEVCUT — DOKUNULMADI)")
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				env.errorf("HATA: skill yazilamadi: %s\n", err)
				return 2
			}
			if err := writeText(target, fresh); err != nil {
				env.errorf("HATA: skill yazilamadi: %s\n", err)
				return 2
			}
			created = append(created, ".agents/skills/"+name+"/SKILL.md (skill)")
		}
	}

	env.printf("\n--- ozet ---\n")
	for _, c := range created {
		env.printf("  olusturuldu: %s\n", c)
	}
	for _, s := range skipped {
		env.printf("  atlandi    : %s\n", s)
	}
	return 0
}

// defaultMemoryDir is where the memory lives when --memory-path is not given:
// INSIDE the project, as a gitignored folder — the layout people expect from
// tools like this. A memory shared by several repos of one product still goes
// next to them, explicitly: `--memory-path ../<product>-memory`.
const defaultMemoryDir = ".memory"

// memoryRelPath is the memory_path written to context.yaml. ONE place, so init
// and bootstrap can never disagree about the default.
func memoryRelPath(opts InitOptions) string {
	if opts.MemoryPath != "" {
		return opts.MemoryPath
	}
	return defaultMemoryDir
}

// ensureGitignored keeps a memory that lives INSIDE the project out of the
// project's own repo: the memory is a separate Git repo and may carry PII.
//
// Same promise as the rest of init: the user's .gitignore is never rewritten,
// only the missing line is appended (in the file's own line endings), and a
// second run reports "mevcut". A memory outside the project (../x-memory, an
// absolute path) is none of this repo's business — status "". With write=false
// (bootstrap --kuru) the status is computed and nothing is written.
func ensureGitignored(cwd, memoryRel string, write bool) (entry, status string, err error) {
	if filepath.IsAbs(memoryRel) {
		return "", "", nil
	}
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(memoryRel)))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", "", nil
	}
	entry = "/" + rel + "/"

	path := filepath.Join(cwd, ".gitignore")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if !write {
			return entry, "olusturuldu", nil
		}
		text := "# ctx hafizasi — ayri bir Git reposu, bu repoya girmez\n" + entry + "\n"
		if err := writeText(path, text); err != nil {
			return entry, "", err
		}
		return entry, "olusturuldu", nil
	}
	if err != nil {
		return entry, "", err
	}

	text := string(raw)
	for _, line := range strings.Split(normalizeNewlines(text), "\n") {
		if strings.Trim(strings.TrimSpace(line), "/") == rel {
			return entry, "mevcut", nil
		}
	}
	if !write {
		return entry, "eklendi", nil
	}
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += nl
	}
	text += entry + nl
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return entry, "", err
	}
	return entry, "eklendi", nil
}

// memoryFolders is the note taxonomy as folders. They are created empty (with
// .gitkeep) so the shape of the memory is visible before the first note exists.
var memoryFolders = []string{"facts", "decisions", "pitfalls"}

// skillNames are the model-invoked verbs, installed as `.agents/skills/`.
// The list lives in ONE place so Python and Go install the same set.
var skillNames = []string{"ctx-search", "ctx-write", "ctx-close"}

func scaffoldMemory(memory, project string) error {
	for _, d := range []string{"tasks", "sessions"} {
		if err := os.MkdirAll(filepath.Join(memory, d), 0o755); err != nil {
			return err
		}
	}
	for _, d := range memoryFolders {
		if err := os.MkdirAll(filepath.Join(memory, d), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(memory, d, ".gitkeep"), nil, 0o644); err != nil {
			return err
		}
	}
	for _, pair := range [][2]string{
		{"memory/README.md", "README.md"},
		{"memory/tasks/OPEN.md", filepath.Join("tasks", "OPEN.md")},
		{"memory/sessions/CURRENT.md", filepath.Join("sessions", "CURRENT.md")},
	} {
		text, err := template(pair[0])
		if err != nil {
			return err
		}
		if err := writeText(filepath.Join(memory, pair[1]),
			strings.ReplaceAll(text, "{project}", project)); err != nil {
			return err
		}
	}
	return nil
}

// template reads one embedded scaffolding file.
func template(name string) (string, error) {
	raw, err := root.Templates.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// writeText writes a generated file the way Python's Path.write_text does —
// which on Windows means TEXT MODE, translating "\n" to "\r\n".
//
// This is not a preference, it is parity: the Python implementation has always
// written scaffolding with platform line endings, and a Go binary that wrote LF
// instead would make every generated file differ between the two. (Note the
// contrast with `ctx index --yaz`, which deliberately PRESERVES whatever the
// target file already uses — there the file belongs to someone else.)
func writeText(path, text string) error {
	if runtime.GOOS == "windows" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
