package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// THE STRUCTURAL CAPTURE DECISION. The problem it solves: writing to memory was
// purely voluntary until now — if the agent forgot, nothing happened. All three
// gaps found in the 2026-08-12 audit came from exactly that.
//
// Principle: THE JUDGEMENT IS THE AGENT'S, THE DISCIPLINE IS THE TOOL'S. This
// command never says "write this down" — that is a judgement call. It says
// "this session does not close before the decision is made".
//
//	ctx close --yok ["gerekce"]        no durable finding — a VALID decision
//	ctx close --not <yol> [--not ...]  these notes were written
//	ctx close                          status + decision pending (exit 1)
//	ctx close --kontrol                did the previous session decide? (for hooks)
//
// The decision lands in <memory>/sessions/capture-log.jsonl, and the Capture
// Decision Coverage metric falls out of that log for free.
const captureLog = "sessions/capture-log.jsonl"

// CloseOptions mirrors the Python `ctx close` flags.
type CloseOptions struct {
	Repo       string
	None       bool     // --yok given
	NoneReason string   // its optional argument
	Notes      []string // --not, repeatable
	Check      bool     // --kontrol
}

// captureRecord is written as one JSON line. Field ORDER matters: the Python
// implementation dumps a dict in insertion order, and the log is compared
// line-for-line, so a struct (ordered) is used instead of a map (sorted).
type captureRecord struct {
	Timestamp string   `json:"ts"`
	Repo      string   `json:"repo"`
	Head      string   `json:"head"`
	Dirty     int      `json:"kirli"`
	Decision  string   `json:"karar,omitempty"`
	Notes     []string `json:"notlar,omitempty"`
	Reason    string   `json:"gerekce,omitempty"`
}

func git(repo string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// repoState is one git repository's (HEAD, dirty count).
func repoState(dir string) (head string, dirty int) {
	head = git(dir, "rev-parse", "--short", "HEAD")
	for _, line := range strings.Split(git(dir, "status", "--porcelain"), "\n") {
		if strings.TrimSpace(line) != "" {
			dirty++
		}
	}
	return head, dirty
}

// workState collects the "real work happened here" signals. A dirty file means
// work was done.
//
// The BOUNDARY is the manifest's directory and the git search never crosses it.
// Measured 2026-08-13: the `yapit/` umbrella is not a git repo, so
// `rev-parse --show-toplevel` climbed up and took the user's HOME DIRECTORY for
// the project — "repo: onur5 · HEAD: ? · degisik dosya: 63". Records landed
// under that same name, so `--kontrol` answered "current" every time: the gate
// that carries the whole discipline was CLOSED exactly where sessions start.
//
// When the umbrella is not a repo, the repositories beneath it are AGGREGATED,
// so the gate also works in a multi-repo layout. The HEAD fingerprint is built
// from their heads, so a commit in any one of them re-arms the gate.
func workState(repo, boundary string) (name, head string, dirty int) {
	top := git(repo, "rev-parse", "--show-toplevel")
	if top != "" && withinBoundary(top, boundary) {
		head, dirty = repoState(top)
		return path.Base(filepath.ToSlash(top)), head, dirty
	}
	if boundary == "" {
		return path.Base(filepath.ToSlash(repo)), "", 0
	}

	entries, err := os.ReadDir(boundary)
	if err != nil {
		return filepath.Base(boundary), "", 0
	}
	var parts []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(boundary, e.Name())
		if _, err := os.Stat(filepath.Join(child, ".git")); err != nil {
			continue
		}
		childHead, childDirty := repoState(child)
		if childHead == "" {
			childHead = "?"
		}
		parts = append(parts, e.Name()+"@"+childHead)
		dirty += childDirty
	}
	return filepath.Base(boundary), strings.Join(parts, ","), dirty
}

// withinBoundary reports whether a git top level lies at or below the manifest
// directory. An empty boundary disables the check.
func withinBoundary(top, boundary string) bool {
	if boundary == "" {
		return true
	}
	a, err1 := filepath.Abs(filepath.FromSlash(top))
	b, err2 := filepath.Abs(boundary)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(b, a)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func readCaptureLog(memory string) []captureRecord {
	raw, err := os.ReadFile(filepath.Join(memory, filepath.FromSlash(captureLog)))
	if err != nil {
		return nil
	}
	var records []captureRecord
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r captureRecord
		if json.Unmarshal([]byte(line), &r) != nil {
			continue // a corrupt line must not take the whole log down
		}
		records = append(records, r)
	}
	return records
}

// Close records the session's capture decision.
func Close(env Env, opts CloseOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		env.printf("HATA: %s.\n", err) // stdout — bkz. search.go
		return 2
	}
	repo := opts.Repo
	if repo == "" {
		if wd, err := os.Getwd(); err == nil {
			repo = wd
		}
	}
	// Sinir: manifest dizini — git aramasi projeden disari cikamaz.
	name, head, dirty := workState(repo, filepath.Dir(ctx.ManifestPath))
	records := readCaptureLog(ctx.MemoryPath)

	if opts.Check {
		var last *captureRecord
		if len(records) > 0 {
			last = &records[len(records)-1]
		}
		if dirty > 0 && (last == nil || last.Head != head || last.Repo != name) {
			env.printf("⚠ %s: %d degisik dosya var, bu HEAD icin capture karari YOK.\n"+
				"  `ctx close --yok \"...\"` ya da `ctx close --not <yol>` ile kapat.\n",
				name, dirty)
			return 1
		}
		env.printf("✓ capture karari guncel\n")
		return 0
	}

	if !opts.None && len(opts.Notes) == 0 {
		shownHead := head
		if shownHead == "" {
			shownHead = "?"
		}
		env.printf("repo: %s · HEAD: %s · degisik dosya: %d\n", name, shownHead, dirty)
		if len(records) > 0 {
			last := records[len(records)-1]
			env.printf("son karar: %s · %s\n", orQuestion(last.Timestamp), orQuestion(last.Decision))
		}
		env.printf("\nKARAR BEKLENIYOR — bu oturumda kalici bir bulgu var mi?\n")
		env.printf("  yok  : ctx close --yok \"gerekce\"\n")
		env.printf("  var  : ctx close --not pitfalls/ornek.md [--not ...]\n")
		env.printf("\n'Durable bulgu yok' GECERLI bir karardir; karar VERMEMEK gecersizdir.\n")
		return 1
	}

	record := captureRecord{
		Timestamp: time.Now().Format("2006-01-02T15:04:05"),
		Repo:      name,
		Head:      head,
		Dirty:     dirty,
	}
	if len(opts.Notes) > 0 {
		var missing []string
		for _, n := range opts.Notes {
			if info, err := os.Stat(filepath.Join(ctx.MemoryPath, filepath.FromSlash(n))); err != nil || info.IsDir() {
				missing = append(missing, n)
			}
		}
		if len(missing) > 0 {
			env.printf("HATA: hafizada bulunmayan not(lar): %s\n", strings.Join(missing, ", "))
			return 2
		}
		record.Decision = "captured"
		record.Notes = opts.Notes
	} else {
		record.Decision = "none"
		if opts.NoneReason != "" {
			record.Reason = opts.NoneReason
		}
	}

	if err := appendRecord(ctx.MemoryPath, record); err != nil {
		env.errorf("HATA: capture log yazilamadi: %s\n", err)
		return 2
	}

	if record.Decision == "captured" {
		env.printf("✓ capture karari kaydedildi: %d not (%s)\n",
			len(opts.Notes), strings.Join(opts.Notes, ", "))
		// A new note means the always-loaded map is STALE. Leaving that to a
		// separate discipline is exactly what this system once got wrong: the
		// tool with a hook (Claude Code) renders the map live, but Codex and
		// Cursor read it from the file and would never see the new note.
		// Refreshing it is part of closing.
		refreshIndex(env, ctx)
	} else {
		env.printf("✓ capture karari kaydedildi: durable bulgu yok\n")
	}

	records = readCaptureLog(ctx.MemoryPath)
	closed := 0
	for _, r := range records {
		if r.Decision == "none" || r.Decision == "captured" {
			closed++
		}
	}
	if len(records) > 0 {
		env.printf("  Capture Decision Coverage: %d/%d oturum kararla kapandi\n",
			closed, len(records))
	}
	return 0
}

func appendRecord(memory string, record captureRecord) error {
	logPath := filepath.Join(memory, filepath.FromSlash(captureLog))
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	line, err := encodeRecord(record)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// encodeRecord serialises one log line.
//
// Written by hand instead of with json.Marshal for ONE reason: both
// implementations append to the same file, and Python's json.dumps puts a space
// after every ':' and ',' while Go's encoder is compact. Two spellings of the
// same record in one log is a difference a reader would have to think about —
// and the whole point of this port is that there is nothing to think about.
// Key order is the Python dict's insertion order.
func encodeRecord(r captureRecord) (string, error) {
	var parts []string
	add := func(key string, value any) error {
		encoded, err := jsonValue(value)
		if err != nil {
			return err
		}
		parts = append(parts, `"`+key+`": `+encoded)
		return nil
	}
	for _, field := range []struct {
		key   string
		value any
		skip  bool
	}{
		{key: "ts", value: r.Timestamp},
		{key: "repo", value: r.Repo},
		{key: "head", value: r.Head},
		{key: "kirli", value: r.Dirty},
		{key: "karar", value: r.Decision, skip: r.Decision == ""},
		{key: "notlar", value: r.Notes, skip: len(r.Notes) == 0},
		{key: "gerekce", value: r.Reason, skip: r.Reason == ""},
	} {
		if field.skip {
			continue
		}
		if err := add(field.key, field.value); err != nil {
			return "", err
		}
	}
	return "{" + strings.Join(parts, ", ") + "}\n", nil
}

// jsonValue encodes a single value, spacing lists the way Python does.
func jsonValue(value any) (string, error) {
	if list, ok := value.([]string); ok {
		items := make([]string, 0, len(list))
		for _, item := range list {
			encoded, err := jsonValue(item)
			if err != nil {
				return "", err
			}
			items = append(items, encoded)
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // Python's ensure_ascii=False leaves text as written
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// refreshIndex refreshes EVERY consumer's map, not just this repo's.
//
// A new note makes all of their blocks stale at the same moment, and the repo
// you happened to run `ctx close` from is not special. Refreshing only the
// current one is what let two repos sit nine days behind.
func refreshIndex(env Env, ctx store.Context) {
	for _, manifestPath := range ConsumerManifests(ctx) {
		other, err := consumerContext(manifestPath)
		if err != nil {
			continue
		}
		body, err := IndexBody(other, false)
		if err != nil {
			continue
		}
		for _, target := range IndexTargets(other) {
			raw, err := os.ReadFile(target)
			if err != nil {
				continue
			}
			text := string(raw)
			updated, ok := replaceBlock(text, body)
			if !ok || updated == text {
				continue
			}
			if os.WriteFile(target, []byte(updated), 0o644) == nil {
				env.printf("  ↻ %s · hafiza haritasi tazelendi\n", shortTarget(target))
			}
		}
	}
}

func orQuestion(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
