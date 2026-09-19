package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/store"
)

// Tüketici kaydı: "beni kimler okuyor" hafızanın `ctx.yaml`ındaki `consumers`
// listesindedir. Liste elle tutulursa yeni repo kaydı unutulur ve haritası
// sessizce bayatlar (2026-08-13 dersi); `ctx bootstrap` kaydı kendisi yapar,
// `ctx doctor` kayıtsızı ve ölü girdiyi uyarır. Yollar hafızaya GÖRELİ tutulur;
// okuma da aynı kuraldadır, iki taraf ayrışamaz.

// deadConsumerEntries lists `consumers` entries that resolve to no manifest.
// A roof-relative entry is born dead and ConsumerManifests silently skips it,
// so the refresh never reaches that repo.
func deadConsumerEntries(memory string) []string {
	var dead []string
	for _, rel := range store.MemoryConfig(memory).List("consumers") {
		candidate := filepath.Join(memory, filepath.FromSlash(rel), store.ManifestName)
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			dead = append(dead, rel)
		}
	}
	return dead
}

// undiscoveredConsumers finds repos reading this memory that are NOT in the
// consumers list. Depth 1 (neighbours, not a filesystem walk): the memory's
// parent, the current repo's parent, and the memory's grandparent. Deeper
// layouts are neither warned about nor written to.
//
// Seam note: membership is compared on absolute (lexically cleaned) paths,
// while the Python side compares resolved ones — the two agree unless a
// symlink sits in the middle, the same class of seam as the note-tree ordering.
func undiscoveredConsumers(ctx store.Context) []string {
	known := map[string]bool{}
	for _, m := range ConsumerManifests(ctx) {
		known[m] = true
	}
	repoDir := filepath.Dir(ctx.ManifestPath)
	roots := []string{filepath.Dir(ctx.MemoryPath), repoDir, filepath.Dir(filepath.Dir(ctx.MemoryPath))}
	seenRoot := map[string]bool{}
	var found []string
	for _, root := range roots {
		if seenRoot[root] {
			continue
		}
		seenRoot[root] = true
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifest := filepath.Join(root, e.Name(), store.ManifestName)
			data, err := store.ReadManifest(manifest)
			if err != nil {
				continue
			}
			rel := data.Str("memory_path")
			if rel == "" {
				continue
			}
			cand := rel
			if !filepath.IsAbs(cand) {
				cand = filepath.Join(root, e.Name(), filepath.FromSlash(rel))
			}
			abs, err := filepath.Abs(cand)
			if err != nil || abs != ctx.MemoryPath {
				continue
			}
			absManifest, err := filepath.Abs(manifest)
			if err != nil {
				continue
			}
			if !known[absManifest] && !contains(found, absManifest) {
				found = append(found, absManifest)
			}
		}
	}
	sort.Strings(found)
	return found
}

// addConsumerLine adds an entry to a bracket-form consumers line.
// Returns the text and the status: "eklendi" | "kayitli" | "bilinmiyor".
// Any non-bracket form (block list etc.) is NOT touched — silently
// normalizing it would clobber the user's hand layout.
func addConsumerLine(text, rel string) (string, string) {
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	trailing := strings.HasSuffix(text, nl)
	body := strings.TrimSuffix(text, nl)
	var lines []string
	if body != "" {
		lines = strings.Split(body, nl)
	}
	var out []string
	found := false
	for _, line := range lines {
		code := strings.SplitN(line, "#", 2)[0]
		name, value, hasColon := strings.Cut(strings.TrimSpace(code), ":")
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if !found && hasColon && strings.TrimSpace(name) == "consumers" && !indented {
			found = true
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				var items []string
				for _, p := range strings.Split(value[1:len(value)-1], ",") {
					if s := strings.TrimSpace(p); s != "" {
						items = append(items, s)
					}
				}
				if contains(items, rel) {
					return text, "kayitli"
				}
				out = append(out, "consumers: ["+strings.Join(append(items, rel), ", ")+"]")
			} else {
				return text, "bilinmiyor"
			}
			continue
		}
		out = append(out, line)
	}
	if !found {
		out = append(out, "consumers: ["+rel+"]")
	}
	result := strings.Join(out, nl)
	if trailing {
		result += nl
	}
	return result, "eklendi"
}

// registerConsumer records the current repo in the memory's consumers list
// (line-based, never destructive). Report lines mirror the Python side verbatim.
func registerConsumer(memory, repo string, write bool) []string {
	if info, err := os.Stat(memory); err != nil || !info.IsDir() {
		return []string{"! hafıza dizini yok — kayıt atlandı"}
	}
	rel, err := filepath.Rel(memory, repo)
	if err != nil {
		return []string{"! consumers: yol bağlanamadı — elle ekleyin"}
	}
	rel = filepath.ToSlash(rel)
	cfgPath := filepath.Join(memory, store.MemoryConfigName)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		if write {
			text := "# Hafıza ayarları (ctx aracı okur; `ctx bootstrap` satır bazında günceller).\n" +
				"consumers: [" + rel + "]\n"
			// writeText: Python metin modu gibi platform satır sonuyla yazar —
			// dosya iki tarafta da aynı bayt olur.
			if werr := writeText(cfgPath, text); werr != nil {
				return []string{"! ctx.yaml yazılamadı — elle ekleyin"}
			}
		}
		return []string{"+ consumers: [" + rel + "] (ctx.yaml oluşturuldu)"}
	}
	updated, status := addConsumerLine(normalizeNewlines(string(raw)), rel)
	if status == "eklendi" && write {
		if werr := writeText(cfgPath, updated); werr != nil {
			return []string{"! ctx.yaml yazılamadı — elle ekleyin"}
		}
	}
	switch status {
	case "eklendi":
		return []string{"+ consumers: [" + rel + "] eklendi"}
	case "kayitli":
		return []string{"= consumers (kayıtlı)"}
	default:
		return []string{"! consumers biçimi tanınmadı — elle ekleyin: " + rel}
	}
}
