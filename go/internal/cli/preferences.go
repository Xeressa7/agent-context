package cli

import (
	"os"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Oturum bağlam tercihi: harita bütçesi + otomatik harita.
//
// Profiller: normal (16000, açık) · ekonomik (6000, açık) · manuel (harita
// yok). Bayraksız koşum gösterir; --profile/--index-budget context.yaml'i
// SATIR BAZLI düzenler — anahtar sırası, yorumlar ve satır sonu korunur, çünkü
// tam YAML yazıcı iki implementasyonda aynı baytları üretmezdi.

// economicalIndexBudget mirrors Python's EKONOMIK_BUTCE.
const economicalIndexBudget = 6000

// PreferencesOptions mirrors the Python `ctx preferences` flags. BudgetGiven
// tells "not passed" apart from a passed (possibly invalid) value, the way
// argparse's None default does.
type PreferencesOptions struct {
	Repo        string
	Profile     string
	Budget      int
	BudgetGiven bool
}

// indexBudgetFor reads the injection ceiling: `context.yaml → index_budget`,
// falling back to the 16000 default. An unreadable/non-positive value falls
// back too — silently emptying the map would be worse than exceeding it.
func indexBudgetFor(m store.Manifest) int {
	if n := m.Int("index_budget", indexBudget); n > 0 {
		return n
	}
	return indexBudget
}

// autoOffValues mirrors Python's _KAPALI_DEGERLER.
var autoOffValues = map[string]bool{
	"false": true, "0": true,
	"hayir": true, "hayır": true,
	"no": true, "kapali": true, "kapalı": true,
}

// autoIndexOn reports whether the session map is injected (`auto_index`,
// default on).
func autoIndexOn(m store.Manifest) bool {
	return !autoOffValues[strings.ToLower(strings.TrimSpace(m.Str("auto_index")))]
}

// profileName derives the profile label from the effective values.
func profileName(budget int, auto bool) string {
	if !auto {
		return "manuel"
	}
	switch budget {
	case indexBudget:
		return "normal"
	case economicalIndexBudget:
		return "ekonomik"
	}
	return "özel"
}

// setManifestLine edits one top-level `key: value` line in place, appends it
// when absent, or drops the line when value is nil. Comment lines and indented
// (in-block) lines never match. Line endings are normalized on entry (callers
// pass raw bytes); the platform endings are restored on write, the same way
// Python's text-mode IO translates through \n.
func setManifestLine(text, key string, value *string) string {
	text = normalizeNewlines(text)
	trailing := strings.HasSuffix(text, "\n")
	body := strings.TrimSuffix(text, "\n")
	var lines []string
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	var out []string
	found := false
	for _, line := range lines {
		code := strings.SplitN(line, "#", 2)[0]
		name, _, hasColon := strings.Cut(strings.TrimSpace(code), ":")
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if !found && hasColon && strings.TrimSpace(name) == key && !indented {
			found = true
			if value != nil {
				out = append(out, key+": "+*value)
			}
			continue
		}
		out = append(out, line)
	}
	if !found && value != nil {
		out = append(out, key+": "+*value)
	}
	result := strings.Join(out, "\n")
	if trailing {
		result += "\n"
	}
	return result
}

func strp(s string) *string { return &s }

// Preferences shows or edits the session context preference.
func Preferences(env Env, opts PreferencesOptions) int {
	ctx, err := store.LoadContext(opts.Repo)
	if err != nil {
		env.printf("HATA: %s.\n", err) // stdout — bkz. search.go
		return 2
	}
	if opts.Profile == "" && !opts.BudgetGiven {
		budget := indexBudgetFor(ctx.Data)
		auto := autoIndexOn(ctx.Data)
		state := "açık"
		if !auto {
			state = "kapalı"
		}
		env.printf("profil: %s · harita bütçesi: %d karakter · otomatik harita: %s\n",
			profileName(budget, auto), budget, state)
		return 0
	}
	raw, err := os.ReadFile(ctx.ManifestPath)
	if err != nil {
		env.errorf("HATA: context.yaml okunamadi: %s\n", err)
		return 2
	}
	text := normalizeNewlines(string(raw))
	switch opts.Profile {
	case "ekonomik":
		text = setManifestLine(text, "index_budget", strp(itoa(economicalIndexBudget)))
		text = setManifestLine(text, "auto_index", nil)
		env.printf("✓ profil: ekonomik (index_budget: %d yazıldı)\n", economicalIndexBudget)
	case "manuel":
		text = setManifestLine(text, "auto_index", strp("false"))
		env.printf("✓ profil: manuel (auto_index: false yazıldı)\n")
	case "normal":
		text = setManifestLine(text, "index_budget", nil)
		text = setManifestLine(text, "auto_index", nil)
		env.printf("✓ profil: normal (varsayılanlara dönüldü)\n")
	case "":
		// yalnız --index-budget: aşağıdaki dal yazar
	default:
		env.printf("HATA: bilinmeyen profil: %s (geçerli: normal, ekonomik, manuel)\n", opts.Profile)
		return 2
	}
	if opts.BudgetGiven {
		if opts.Budget <= 0 {
			env.printf("HATA: bütçe pozitif olmalı: %d\n", opts.Budget)
			return 2
		}
		text = setManifestLine(text, "index_budget", strp(itoa(opts.Budget)))
		env.printf("✓ harita bütçesi: %d karakter (index_budget yazıldı)\n", opts.Budget)
	}
	if err := writeText(ctx.ManifestPath, text); err != nil {
		env.errorf("HATA: context.yaml yazilamadi: %s\n", err)
		return 2
	}
	return 0
}
