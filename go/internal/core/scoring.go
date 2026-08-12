package core

import (
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

// Field-weighted scoring.
//
// Candidate selection is OR: a note matching at least ONE query word is a
// candidate. The earlier AND rule produced a measured failure — a two-word
// query returned nothing for the note that described exactly that topic,
// because one of the words did not literally appear in it.
//
// Each word scores from its highest-weighted field only (one field per word,
// not a sum). The raw score is then scaled by COVERAGE, so a note answering the
// whole query still outranks one answering half of it: AND's ranking quality
// without AND's elimination.
//
// A multi-word query whose full phrase appears verbatim in a field earns that
// field's weight again as a PHRASE BONUS.

// CoverageBase is the floor of the coverage multiplier: a note matching half of
// a two-word query keeps 70% of its raw score.
const CoverageBase = 0.4

type matchMode int

const (
	modeEqual matchMode = iota
	modeContains
)

type fieldSpec struct {
	Label  string // shown to the user as the reason
	Key    string
	Weight int
	Mode   matchMode
}

// Order matters twice: it selects the highest-weighted matching field, and it
// orders the reasons shown for a hit.
var fieldSpecs = []fieldSpec{
	{"yol", "yol-tam", 100, modeEqual},
	{"yol", "yol-kismi", 70, modeContains},
	{"kod", "kod", 60, modeContains},
	{"başlık", "baslik", 40, modeContains},
	{"etiket", "etiket", 30, modeContains},
	{"bileşen", "bilesen", 20, modeContains},
	{"açıklama", "aciklama", 12, modeContains},
	{"gövde", "govde", 3, modeContains},
}

// FieldWeights exposes the table so tests can compare it against the vectors.
func FieldWeights() map[string]int {
	out := make(map[string]int, len(fieldSpecs))
	for _, f := range fieldSpecs {
		out[f.Key] = f.Weight
	}
	return out
}

var codeSpanPattern = regexp.MustCompile("`([^`\n]+)`")

// codeLike decides whether a backticked span is an identifier/path (a proxy for
// code) or just emphasised prose. Prose scored at the code weight would send a
// heavy hit to the wrong note.
func codeLike(span string) bool {
	if span == "" || strings.ContainsAny(span, " \t\n") {
		return false
	}
	if strings.ContainsAny(span, ".:_/\\(-") {
		return true
	}
	runes := []rune(span)
	for i := 1; i < len(runes); i++ {
		if runes[i] >= 'A' && runes[i] <= 'Z' && runes[i-1] >= 'a' && runes[i-1] <= 'z' {
			return true // camelCase
		}
	}
	return false
}

func codeSpans(body string) string {
	var parts []string
	for _, m := range codeSpanPattern.FindAllStringSubmatch(body, -1) {
		if codeLike(m[1]) {
			parts = append(parts, m[1])
		}
	}
	return strings.Join(parts, " ")
}

// Note is the scoring input: everything already extracted from the file.
type Note struct {
	RelPath     string
	Title       string
	Tags        string
	Components  string
	Description string
	Body        string
}

// Score is the result for one note against one query.
type Score struct {
	Value   float64
	Reasons []string
	Matched int
}

// ScoreNote scores a note against pre-folded query words and phrase.
// Returns ok=false when no word matched at all: not a candidate.
func ScoreNote(n Note, words []string, phrase string) (Score, bool) {
	stem := strings.TrimSuffix(filepath.Base(n.RelPath), filepath.Ext(n.RelPath))
	fields := map[string]string{
		"yol-tam":   Fold(stem),
		"yol-kismi": Fold(filepath.ToSlash(n.RelPath)),
		"kod":       Fold(codeSpans(n.Body)),
		"baslik":    Fold(n.Title),
		"etiket":    Fold(n.Tags),
		"bilesen":   Fold(n.Components),
		"aciklama":  Fold(n.Description),
		"govde":     Fold(n.Body),
	}

	hit := func(spec fieldSpec, needle string) bool {
		v := fields[spec.Key]
		if spec.Mode == modeEqual {
			return needle == v
		}
		return strings.Contains(v, needle)
	}

	raw := 0
	matched := 0
	var reasonOrder []string
	seen := map[string]bool{}

	for _, w := range words {
		for _, spec := range fieldSpecs {
			if !hit(spec, w) {
				continue
			}
			raw += spec.Weight
			matched++
			if !seen[spec.Label] {
				seen[spec.Label] = true
				reasonOrder = append(reasonOrder, spec.Label)
			}
			break // highest-weighted field only
		}
	}

	if matched == 0 {
		return Score{}, false
	}

	coverage := float64(matched) / float64(len(words))
	value := float64(raw) * (CoverageBase + (1-CoverageBase)*coverage)

	if phrase != "" && len(words) > 1 {
		for _, spec := range fieldSpecs {
			if !hit(spec, phrase) {
				continue
			}
			value += float64(spec.Weight)
			if !seen["ifade"] {
				seen["ifade"] = true
				reasonOrder = append([]string{"ifade"}, reasonOrder...)
			}
			break
		}
	}

	// Reasons are ordered by weight, with the phrase bonus first.
	rank := map[string]int{"ifade": -1}
	for i, spec := range fieldSpecs {
		if _, ok := rank[spec.Label]; !ok {
			rank[spec.Label] = i
		}
	}
	sortByRank(reasonOrder, rank)
	if len(reasonOrder) > 3 {
		reasonOrder = reasonOrder[:3]
	}

	return Score{
		Value:   math.Round(value*10) / 10,
		Reasons: reasonOrder,
		Matched: matched,
	}, true
}

// sortByRank is an insertion sort: the slice holds at most a handful of labels,
// and a stable hand-rolled sort keeps the ordering rule visible.
func sortByRank(labels []string, rank map[string]int) {
	for i := 1; i < len(labels); i++ {
		for j := i; j > 0 && rank[labels[j]] < rank[labels[j-1]]; j-- {
			labels[j], labels[j-1] = labels[j-1], labels[j]
		}
	}
}
