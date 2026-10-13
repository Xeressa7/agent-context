package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Session close as a MECHANISM, not a discipline.
//
// The model no longer has to remember to write anything: the close hook reads
// the transcript and drops a raw digest under `sessions/.pending/`. The NEXT
// session's capture-check surfaces it, and the lead agent — with its full
// attention available — turns it into a note.
//
// Why NO model call here: a close hook has to return in under a second on every
// tool. Spawning `claude -p` in the background (a) burns quota, (b) re-triggers
// this same vault's hooks and recurses, (c) has no equivalent on another
// provider (Codex/Hermes). A raw digest is free and deterministic.
//
// PII: a transcript can carry real names and addresses. `.pending/` is
// gitignored and machine-local; it is never committed to the memory repo.

const (
	flushDir          = "sessions/.pending"
	flushTurnCap      = 40
	flushCharCap      = 12000
	flushMinTurns     = 2
	flushRecursionEnv = "CTX_HOOK_ICINDE"
)

// The name of a text block differs per provider; measured 2026-08-28: Claude
// writes `text`, Codex writes `input_text`/`output_text`. A reader that looks
// only for `text` extracted ZERO turns from a Codex transcript and silently
// wrote nothing.
var textBlockTypes = map[string]bool{"text": true, "input_text": true, "output_text": true}

type turn struct{ who, text string }

// collectText pulls only the PLAIN TEXT out of a message body. Tool calls, tool
// results and thinking blocks are skipped: they are the trace of execution, not
// the conversation, and they push the real exchange past the cap.
func collectText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, raw := range v {
			switch p := raw.(type) {
			case string:
				if s := strings.TrimSpace(p); s != "" {
					parts = append(parts, s)
				}
			case map[string]any:
				kind, _ := p["type"].(string)
				if !textBlockTypes[kind] {
					continue
				}
				if s, _ := p["text"].(string); strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func turnsClaude(records []map[string]any) []turn {
	var out []turn
	for _, d := range records {
		kind, _ := d["type"].(string)
		if kind != "user" && kind != "assistant" {
			continue
		}
		msg, ok := d["message"].(map[string]any)
		if !ok {
			continue
		}
		if text := collectText(msg["content"]); text != "" {
			out = append(out, turn{whoLabel(kind == "user"), text})
		}
	}
	return out
}

// Hermes keeps sessions in SQLite and exports them with `hermes sessions
// export`; the exported shape is the same OpenAI message form Codex uses, so
// the Codex reader serves both. No SQLite parser is written.
func turnsCodex(records []map[string]any) []turn {
	var out []turn
	for _, d := range records {
		if kind, _ := d["type"].(string); kind != "response_item" {
			continue
		}
		payload, ok := d["payload"].(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := payload["type"].(string); kind != "message" {
			continue
		}
		role, _ := payload["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		if text := collectText(payload["content"]); text != "" {
			out = append(out, turn{whoLabel(role == "user"), text})
		}
	}
	return out
}

func whoLabel(isUser bool) string {
	if isUser {
		return "Kullanici"
	}
	return "Ajan"
}

func readJSONL(path string) []map[string]any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

// digestBody joins the last turns under the cap, always cutting at a TURN
// BOUNDARY: slicing mid-character-count makes the conversation lie, because
// half a sentence says something else.
func digestBody(turns []turn) string {
	sel := turns
	if len(sel) > flushTurnCap {
		sel = sel[len(sel)-flushTurnCap:]
	}
	var blocks []string
	total := 0
	for i := len(sel) - 1; i >= 0; i-- {
		block := "**" + sel[i].who + ":** " + sel[i].text
		if total+len([]rune(block)) > flushCharCap && len(blocks) > 0 {
			break
		}
		blocks = append(blocks, block)
		total += len([]rune(block))
	}
	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}
	return strings.Join(blocks, "\n\n")
}

// FlushOptions carries what the close hook was told about the session.
type FlushOptions struct {
	Repo       string
	Transcript string
	Tool       string
	Reason     string
	Now        time.Time
}

// hermesExport writes a Hermes session out to a temporary JSONL and returns
// its path, or "" on any failure. `--redact` strips PII at the source.
func hermesExport(sessionID string) string {
	bin, err := exec.LookPath("hermes")
	if err != nil {
		return ""
	}
	target := filepath.Join(os.TempDir(), "ctx-hermes-"+sessionID+".jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "sessions", "export", "--format", "jsonl",
		"--session-id", sessionID, "--redact", "--yes", target)
	if cmd.Run() != nil {
		return ""
	}
	if st, err := os.Stat(target); err != nil || st.IsDir() {
		return ""
	}
	return target
}

// HookFlush writes the raw session digest. Every failure path is silent.
func HookFlush(env Env, opts FlushOptions) int {
	if os.Getenv(flushRecursionEnv) != "" {
		return 0
	}
	tool := opts.Tool
	if tool == "" {
		tool = "claude"
	}

	// Claude Code / Hermes hooks hand the session over as JSON on STDIN;
	// `--transcript` exists for manual calls and tests. Empty or malformed
	// stdin exits quietly — a hook NEVER breaks the session.
	path, temp := opts.Transcript, ""
	if path == "" {
		if raw, err := io.ReadAll(env.stdin()); err == nil && len(raw) > 0 {
			var payload map[string]any
			if json.Unmarshal(raw, &payload) == nil {
				if s, _ := payload["transcript_path"].(string); s != "" {
					path = s
				} else if s, _ := payload["transcript"].(string); s != "" {
					path = s
				} else if s, _ := payload["session_id"].(string); s != "" && tool == "hermes" {
					// Hermes hands over a session_id, not a file: the session
					// lives in SQLite. Exporting through the tool's OWN
					// supported interface beats parsing its schema, which would
					// break silently the moment that schema changes.
					if exported := hermesExport(s); exported != "" {
						path, temp = exported, exported
					}
				}
			}
		}
	}
	if path == "" {
		return 0
	}
	opts.Transcript = path
	if temp != "" {
		defer os.Remove(temp)
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return 0
	}

	var turns []turn
	records := readJSONL(path)
	switch tool {
	case "claude":
		turns = turnsClaude(records)
	case "codex", "hermes":
		turns = turnsCodex(records)
	default:
		return 0
	}
	if len(turns) < flushMinTurns {
		return 0 // too short to be worth capturing — correct, not an error
	}

	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		return 0
	}
	dir := filepath.Join(ctx.MemoryPath, filepath.FromSlash(flushDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0
	}

	// The same transcript at the same length is not written twice (SessionEnd
	// and PreCompact can fire back to back).
	sig := filepath.Base(opts.Transcript) + "·" + strconv.Itoa(len(turns))
	if existing, err := filepath.Glob(filepath.Join(dir, "*.md")); err == nil {
		for _, old := range existing {
			b, readErr := os.ReadFile(old)
			if readErr != nil {
				continue
			}
			head := string(b)
			if len(head) > 400 {
				head = head[:400]
			}
			if strings.Contains(head, sig) {
				return 0
			}
		}
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	stamp := now.Format("20060102-150405")
	reason := opts.Reason
	if reason == "" {
		reason = "sessionend"
	}
	shown := len(turns)
	if shown > flushTurnCap {
		shown = flushTurnCap
	}
	body := "<!-- ctx:flush imza=" + sig + " sebep=" + reason + " -->\n" +
		"# Kaydedilmemis oturum dokumu — " + stamp + "\n\n" +
		"Arac: `" + tool + "` · kaynak: `" + filepath.Base(opts.Transcript) + "` · " +
		strconv.Itoa(len(turns)) + " tur (son " + strconv.Itoa(shown) + " tanesi asagida).\n\n" +
		"Bu HAM dokumdur, not degildir. Kalici bulgu varsa uygun klasore yaz,\n" +
		"sonra `ctx close --note <yol>` calistir ve bu dosyayi SIL.\n\n---\n\n" +
		digestBody(turns) + "\n"
	if os.WriteFile(filepath.Join(dir, tool+"-"+stamp+".md"), []byte(body), 0o644) != nil {
		return 0
	}
	return 0
}

// pendingDigests lists the raw digests waiting to be turned into notes.
func pendingDigests(memory string) []string {
	hits, err := filepath.Glob(filepath.Join(memory, filepath.FromSlash(flushDir), "*.md"))
	if err != nil {
		return nil
	}
	sort.Strings(hits)
	return hits
}
