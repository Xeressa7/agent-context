// Package cli holds the command implementations. Commands write to an injected
// writer rather than straight to stdout, so a test can assert on the exact
// bytes — which is the whole finishing criterion for the Go port: the two
// implementations must print the SAME thing for the same memory.
package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Env is what a command needs from the outside world.
type Env struct {
	Out io.Writer
	Err io.Writer
}

func (e Env) printf(format string, a ...any) {
	fmt.Fprintf(e.Out, format, a...)
}

func (e Env) errorf(format string, a ...any) {
	fmt.Fprintf(e.Err, format, a...)
}

// clip truncates to n RUNES, not bytes: the Python implementation slices
// characters, and Turkish descriptions are full of two-byte ones. Cutting by
// bytes would both shorten the text and risk splitting a rune.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// squeeze collapses all whitespace runs to single spaces, the same way Python's
// " ".join(s.split()) does.
func squeeze(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func itoa(n int) string { return strconv.Itoa(n) }
