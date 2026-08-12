// Package core implements the behavioural contract defined by the golden
// vectors in tests/vectors/*.json. The Python implementation in bin/ctx.py and
// this package must produce identical results for every vector.
package core

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// undecomposable maps characters that NFKD does NOT decompose. Their ASCII
// equivalent is a different letter, not a stripped mark, so the normal
// "decompose and drop combining marks" pass cannot reach them.
//
// "Đ" -> "DJ" (not "D") is deliberate: that is the standard hr/sr/bs
// transliteration, and it puts "Đakovo" and "Djakovo" on the same key. It is
// the same class as "þ" -> "th", where one letter also expands to two.
var undecomposable = map[rune]string{
	'ı': "i", 'İ': "I", // tr/az
	'ł': "l", 'Ł': "L", // pl
	'ø': "o", 'Ø': "O", // da/no
	'đ': "dj", 'Đ': "DJ", // hr/sr/bs
	'ð': "d", 'Ð': "D",
	'þ': "th", 'Þ': "TH", // is
	'ß': "ss",
	'æ': "ae", 'Æ': "AE",
	'œ': "oe", 'Œ': "OE",
}

// Fold normalizes text for comparison: drop accents, fold to ASCII where a
// letter has no mark to strip, then lowercase.
//
// Lowercasing comes LAST and uses the Unicode default. Doing it first would
// reintroduce the bug this whole function exists to avoid: in Go as in Python,
// lowercasing "İ" yields "i" plus U+0307 (combining dot above), and text in
// that shape never matches a plain "istanbul" query. After the decomposition
// pass "İ" is already "I", so the trap cannot form.
//
// This is a SEARCH normalization, never a text transformation — it is lossy on
// purpose and must not be used to produce output shown to a user.
func Fold(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if repl, ok := undecomposable[r]; ok {
			b.WriteString(repl)
			continue
		}
		b.WriteRune(r)
	}

	decomposed := norm.NFKD.String(b.String())

	var out strings.Builder
	out.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) { // Mn = non-spacing combining mark
			continue
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String())
}
