package cli

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/jsonx"
	"github.com/Xeressa7/agent-context/go/internal/store"
)

// SessionStart hooks, natively.
//
// These replace templates/hooks/*.py, which were thin wrappers: run a ctx
// command, wrap its stdout in the harness's JSON envelope, stay silent on any
// failure. Doing it in the binary removes the last reason the tool needed a
// Python interpreter installed.
//
// THE FIRST RULE IS SILENCE. A hook runs on every single session start; if it
// ever prints garbage, errors, or a traceback, it damages every session until
// somebody notices. So every failure path here returns quietly with exit 0:
// no memory configured, no manifest, unreadable corpus — all silent.

const hookEventName = "SessionStart"

// hookOutput builds the harness envelope. Key order is fixed and the encoding
// follows Python's json.dumps defaults, because this output replaces a Python
// hook's byte for byte.
func hookOutput(context string) (string, error) {
	return hookOutputFor(hookEventName, context)
}

// hookOutputFor is the same envelope for another hook event.
func hookOutputFor(event, context string) (string, error) {
	inner := jsonx.NewObject()
	inner.Set("hookEventName", event)
	inner.Set("additionalContext", context)
	outer := jsonx.NewObject()
	outer.Set("hookSpecificOutput", inner)
	return jsonx.MarshalCompact(outer)
}

// HookIndex prints the memory map into the session's context.
//
// Measured on 2026-08-12: a 21-note corpus went a whole session unopened,
// because nothing ever triggers a search. The old harness-native memory's one
// real advantage was that its index loaded for free every session. This puts it
// back — from `ctx index`, the SAME producer that fills the AGENTS.md block, so
// the harness-less tools and this one never disagree.
func HookIndex(env Env, repo string) int {
	ctx, err := store.LoadContext(repo)
	if err != nil {
		return 0 // no memory in this project: silence
	}
	if !autoIndexOn(ctx.Data) {
		return 0 // manuel profil: oturum haritasız açılır
	}
	body, err := IndexBodyBudgeted(ctx, indexBudgetFor(ctx.Data))
	if err != nil {
		return 0
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return 0
	}

	out, err := hookOutput(
		"KANONIK HAFIZA HARITASI (her oturumda otomatik yuklenir — " +
			"harness auto-memory'si DEGIL, `context.yaml -> memory_path`):\n\n" +
			body +
			"\n\nKullanim: gorevin konusu yukaridaki bir satirla ortusuyorsa O NOTU AC " +
			"(tam metin orada; bu yalniz harita). Ortusme belirsizse " +
			"`ctx search \"<sorgu>\"`. Butun korpusu topluca yukleme. " +
			"Hafiza canli kodun yerine gecmez: `git log`/`git diff` otoriterdir.")
	if err != nil {
		return 0
	}
	env.printf("%s\n", out)
	return 0
}

// HookCaptureCheck warns when the PREVIOUS session closed without a capture
// decision.
//
// It never says "write this down" — that is judgement. It says the decision was
// never made, which is the one thing a tool can check. "No durable finding" is
// a valid decision; making none is not.
func HookCaptureCheck(env Env, repo string) int {
	var buf bytes.Buffer
	// The check runs through the same Close code path the user would invoke,
	// so the hook can never drift from the command it is reporting on.
	code := Close(Env{Out: &buf, Err: &buf}, CloseOptions{Repo: repo, Check: true})
	if code != 1 {
		return 0 // 0 = decision current · 2 = no memory here — silent either way
	}
	warning := strings.TrimSpace(buf.String())
	if warning == "" {
		return 0
	}

	extra := ""
	if ctx, ctxErr := store.LoadContext(repo); ctxErr == nil {
		if pending := pendingDigests(ctx.MemoryPath); len(pending) > 0 {
			if len(pending) > 3 {
				pending = pending[len(pending)-3:]
			}
			var names []string
			for _, p := range pending {
				names = append(names, "  - "+flushDir+"/"+filepath.Base(p))
			}
			extra = "\n\nO oturumun HAM DOKUMU kancayla kaydedildi (okumak icin ac, " +
				"hepsini birden yukleme):\n" + strings.Join(names, "\n") + "\n" +
				"Bulguyu nota tasidiktan sonra dokum dosyasini SIL."
		}
	}

	out, err := hookOutput(
		"CAPTURE KARARI BEKLIYOR — onceki oturum kalici bulgu karari " +
			"verilmeden kapanmis:\n\n" + warning + extra + "\n\n" +
			"Bu oturumun SONUNDA karar ver: kalici bir bulgu varsa once " +
			"hafizaya yaz, sonra `ctx close --not <yol>`; " +
			"yoksa `ctx close --yok \"gerekce\"`. Karar VERMEMEK gecersizdir. " +
			"Kullaniciyi simdi bununla mesgul etme — kapanista hallet.")
	if err != nil {
		return 0
	}
	env.printf("%s\n", out)
	return 0
}
