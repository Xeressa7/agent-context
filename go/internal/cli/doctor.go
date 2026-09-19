package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Doctor reports on the INSTALLATION rather than the notes: is the manifest
// where it should be, does it point at a reachable memory, is the always-loaded
// map still fresh.
//
// Warnings deliberately do NOT affect the exit code. A health check that goes
// red for something nobody is going to fix stops being read at all, and the one
// real failure then goes unnoticed with it.
func Doctor(env Env, repo string) int {
	failed := 0
	check := func(ok bool, text string) {
		mark := "✗ "
		if ok {
			mark = "✓ "
		}
		env.printf("%s%s\n", mark, text)
		if !ok {
			failed = 1
		}
	}

	start := repo
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			env.printf("✗ calisma dizini okunamadi: %s\n", err)
			return 1
		}
		start = wd
	}
	manifestPath := store.FindManifest(start)
	if manifestPath == "" {
		check(false, "context.yaml bulundu (None)")
		return 1
	}
	check(true, "context.yaml bulundu ("+manifestPath+")")

	data, err := store.ReadManifest(manifestPath)
	if err != nil {
		env.printf("✗ context.yaml okunamadi: %s\n", err)
		return 1
	}
	for _, field := range []string{"project", "memory_path", "role"} {
		check(data.Truthy(field), "context.yaml · "+field+": "+data.Repr(field))
	}

	memory := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(data.Str("memory_path")))
	if abs, err := filepath.Abs(memory); err == nil {
		memory = abs
	}
	info, err := os.Stat(memory)
	isDir := err == nil && info.IsDir()
	check(isDir, "memory_path erisilebilir ("+memory+")")
	readme, err := os.Stat(filepath.Join(memory, "README.md"))
	check(err == nil && !readme.IsDir(), "memory/README.md var")

	// Note validation comes from THE one engine — the same logic `ctx validate`
	// and the pre-commit hook run. It is never written twice.
	count := 0
	var errs, warns []Finding
	if isDir {
		if notes, err := store.WalkNotes(memory); err == nil {
			count = len(notes)
		}
		if findings, err := Check(memory, false); err == nil {
			for _, f := range findings {
				if f.Level == "hata" {
					errs = append(errs, f)
				} else {
					warns = append(warns, f)
				}
			}
		}
	}
	summary := itoa(count) + " not yapisal dogrulamadan gecti"
	if len(errs) > 0 {
		summary += " — " + itoa(len(errs)) + " HATA"
	}
	check(len(errs) == 0, summary)
	for i, f := range errs {
		if i == 5 {
			env.printf("    (+%d hata daha — 'ctx validate' ile tamamini gor)\n", len(errs)-5)
			break
		}
		env.printf("    ✗ %s · %s\n", f.Path, f.Msg)
	}

	openFile, err := os.Stat(filepath.Join(memory, "tasks", "OPEN.md"))
	check(err == nil && !openFile.IsDir(), "tasks/OPEN.md var")
	current, err := os.Stat(filepath.Join(memory, "sessions", "CURRENT.md"))
	check(err == nil && !current.IsDir(), "sessions/CURRENT.md var")

	// Is the compiled binary STALE? The wrapper prefers it SILENTLY, which
	// makes this the installation's quietest failure mode: edit a source,
	// forget to rebuild, and every command keeps running the old code — the
	// output looks right, it is merely the wrong version. This is the one place
	// that can say so.
	for _, line := range staleBinary() {
		warns = append(warns, Finding{"uyari", "bin/ctx", line})
	}

	// A stale block means the harness-less tools (Codex/Cursor) are reading an
	// out-of-date corpus. Warning level only.
	if isDir {
		ctx := store.Context{ManifestPath: manifestPath, Data: data, MemoryPath: memory}
		if body, err := IndexBody(ctx, false); err == nil {
			for _, target := range IndexTargets(ctx) {
				raw, err := os.ReadFile(target)
				if err != nil {
					warns = append(warns, Finding{"uyari", target, "index hedefi yok"})
					continue
				}
				text := string(raw)
				updated, ok := replaceBlock(text, body)
				switch {
				case !ok:
					warns = append(warns, Finding{"uyari", filepath.Base(target),
						"ctx:index isareti yok — blok kurulmamis"})
				case updated != text:
					warns = append(warns, Finding{"uyari", filepath.Base(target),
						"index blogu bayat — `ctx index --yaz`"})
				}
			}
		}
	}

	if !autoIndexOn(data) {
		warns = append(warns, Finding{"uyari", "context.yaml",
			"otomatik harita kapalı (auto_index: false) — oturumlar haritasız açılır"})
	}

	// Tüketici kaydı bakımı: ölü girdi tazelenmeyen repo demek, kayıtsız repo
	// listeye hiç girmemiş demek. İkisi de uyarı — çıkış kodunu etkilemez.
	if isDir {
		for _, dead := range deadConsumerEntries(memory) {
			warns = append(warns, Finding{"uyari", "ctx.yaml",
				"consumers girdisi çözülemedi: " + dead + " — liste hafızaya göreli olmalı"})
		}
		ctx := store.Context{ManifestPath: manifestPath, Data: data, MemoryPath: memory}
		for _, m := range undiscoveredConsumers(ctx) {
			warns = append(warns, Finding{"uyari", filepath.Base(filepath.Dir(m)),
				"hafızayı okuyor ama consumers listesinde yok — haritası tazelenmiyor " +
					"(`ctx bootstrap` ile kaydet)"})
		}
	}

	if len(warns) > 0 {
		env.printf("\n%d uyari (cikis kodunu etkilemez):\n", len(warns))
		for i, w := range warns {
			if i == 8 {
				env.printf("  (+%d uyari daha)\n", len(warns)-8)
				break
			}
			env.printf("  ⚠ %s · %s\n", w.Path, w.Msg)
		}
	}
	return failed
}

// staleBinary reports whether the compiled ctx is older than its sources.
//
// The install location is fixed (~/.agent-context) rather than derived from the
// running executable: doctor audits the INSTALLATION, not whichever binary
// happens to be running, and the two implementations must give the same answer.
func staleBinary() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, ".agent-context")

	var binary string
	for _, name := range []string{"ctx.exe", "ctx.bin"} {
		p := filepath.Join(root, "bin", name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			binary = p
			break
		}
	}
	if binary == "" {
		return nil // no binary: the wrapper picks Python, staleness is moot
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil
	}
	built := info.ModTime()

	var newer []string
	consider := func(path string) {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() || !st.ModTime().After(built) {
			return
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			newer = append(newer, filepath.ToSlash(rel))
		}
	}
	consider(filepath.Join(root, "bin", "ctx.py"))
	consider(filepath.Join(root, "go.mod"))
	consider(filepath.Join(root, "embed.go"))
	filepath.WalkDir(filepath.Join(root, "go"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() &&
			strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			consider(path)
		}
		return nil
	})
	// Templates are EMBEDDED, so editing one without rebuilding is exactly as
	// stale as editing code.
	filepath.WalkDir(filepath.Join(root, "templates"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			consider(path)
		}
		return nil
	})
	if len(newer) == 0 {
		return nil
	}

	sort.Strings(newer)
	sample := strings.Join(newer[:min(3, len(newer))], ", ")
	if len(newer) > 3 {
		sample += " (+" + itoa(len(newer)-3) + ")"
	}
	return []string{"derlenmis ctx BAYAT — " + itoa(len(newer)) +
		" kaynak daha yeni (" + sample + "); `go build -o bin/ctx.exe ./go/cmd/ctx`"}
}
