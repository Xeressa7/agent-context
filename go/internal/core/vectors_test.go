package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden vectors live outside the Go module on purpose: they are the shared
// contract, not Go's test data. Python (tests/test_vektorler.py) runs the same
// files. If these paths ever diverge, the two implementations stop being
// comparable — which is the one thing this whole setup exists to prevent.
const vectorDir = "../../../tests/vectors"

func load(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorDir, name))
	if err != nil {
		t.Fatalf("vector file unreadable: %v", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: invalid JSON: %v", name, err)
	}
}

func TestNormalizeVectors(t *testing.T) {
	var v struct {
		Cases []struct {
			Input  string `json:"input"`
			Expect string `json:"expect"`
			Why    string `json:"why"`
		} `json:"cases"`
	}
	load(t, "normalize.json", &v)

	if len(v.Cases) == 0 {
		t.Fatal("no cases loaded")
	}
	for _, c := range v.Cases {
		if got := Fold(c.Input); got != c.Expect {
			t.Errorf("Fold(%q) = %q, want %q\n  why: %s", c.Input, got, c.Expect, c.Why)
		}
	}
}

func TestChecksumVectors(t *testing.T) {
	var v struct {
		Luhn []struct {
			Input  string `json:"input"`
			Expect bool   `json:"expect"`
			Why    string `json:"why"`
		} `json:"luhn"`
		IBAN []struct {
			Input  string `json:"input"`
			Expect bool   `json:"expect"`
			Why    string `json:"why"`
		} `json:"iban"`
	}
	load(t, "checksum.json", &v)

	for _, c := range v.Luhn {
		if got := LuhnValid(c.Input); got != c.Expect {
			t.Errorf("LuhnValid(%q) = %v, want %v\n  why: %s", c.Input, got, c.Expect, c.Why)
		}
	}
	for _, c := range v.IBAN {
		if got := IBANValid(c.Input); got != c.Expect {
			t.Errorf("IBANValid(%q) = %v, want %v\n  why: %s", c.Input, got, c.Expect, c.Why)
		}
	}
}

func TestVisibilityVectors(t *testing.T) {
	var v struct {
		Cases []struct {
			Note      map[string]string `json:"note"`
			RepoScope []string          `json:"repo_scope"`
			Archive   bool              `json:"archive"`
			Visible   bool              `json:"visible"`
			Why       string            `json:"why"`
		} `json:"cases"`
		Validity []struct {
			Note   map[string]string `json:"note"`
			Status string            `json:"status"`
			Scope  []string          `json:"scope"`
			Why    string            `json:"why"`
		} `json:"validity"`
	}
	load(t, "visibility.json", &v)

	for _, c := range v.Cases {
		if got := Visible(c.Note, c.RepoScope, c.Archive); got != c.Visible {
			t.Errorf("Visible(%v, %v, %v) = %v, want %v\n  why: %s",
				c.Note, c.RepoScope, c.Archive, got, c.Visible, c.Why)
		}
	}
	for _, c := range v.Validity {
		status, scope := Validity(c.Note)
		if status != c.Status {
			t.Errorf("Validity(%v) status = %q, want %q\n  why: %s", c.Note, status, c.Status, c.Why)
		}
		if len(scope) != len(c.Scope) {
			t.Errorf("Validity(%v) scope = %v, want %v\n  why: %s", c.Note, scope, c.Scope, c.Why)
			continue
		}
		for i := range scope {
			if scope[i] != c.Scope[i] {
				t.Errorf("Validity(%v) scope = %v, want %v", c.Note, scope, c.Scope)
				break
			}
		}
	}
}

func TestScoringVectors(t *testing.T) {
	var v struct {
		Weights      map[string]int `json:"weights"`
		CoverageBase float64        `json:"coverage_base"`
		Cases        []struct {
			Path        string   `json:"path"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Body        string   `json:"body"`
			Query       string   `json:"query"`
			Score       *float64 `json:"score"`
			Reasons     []string `json:"reasons"`
			Matched     int      `json:"matched"`
			Why         string   `json:"why"`
		} `json:"cases"`
	}
	load(t, "scoring.json", &v)

	// The weight table itself is part of the contract: a silent drift should
	// break here first, before any individual case.
	got := FieldWeights()
	for k, want := range v.Weights {
		if got[k] != want {
			t.Errorf("weight %q = %d, want %d", k, got[k], want)
		}
	}
	if len(got) != len(v.Weights) {
		t.Errorf("weight table size = %d, want %d", len(got), len(v.Weights))
	}
	if CoverageBase != v.CoverageBase {
		t.Errorf("CoverageBase = %v, want %v", CoverageBase, v.CoverageBase)
	}

	for _, c := range v.Cases {
		var words []string
		for _, w := range strings.Fields(c.Query) {
			words = append(words, Fold(w))
		}
		phrase := Fold(strings.Join(strings.Fields(c.Query), " "))

		note := Note{RelPath: c.Path, Title: c.Title, Description: c.Description, Body: c.Body}
		score, ok := ScoreNote(note, words, phrase)

		if c.Score == nil {
			if ok {
				t.Errorf("%s|%s: expected no candidate, got %v\n  why: %s",
					c.Path, c.Query, score, c.Why)
			}
			continue
		}
		if !ok {
			t.Errorf("%s|%s: expected candidate, got none\n  why: %s", c.Path, c.Query, c.Why)
			continue
		}
		if score.Value != *c.Score {
			t.Errorf("%s|%s: score = %v, want %v\n  why: %s",
				c.Path, c.Query, score.Value, *c.Score, c.Why)
		}
		if score.Matched != c.Matched {
			t.Errorf("%s|%s: matched = %d, want %d", c.Path, c.Query, score.Matched, c.Matched)
		}
		if len(score.Reasons) != len(c.Reasons) {
			t.Errorf("%s|%s: reasons = %v, want %v\n  why: %s",
				c.Path, c.Query, score.Reasons, c.Reasons, c.Why)
			continue
		}
		for i := range score.Reasons {
			if score.Reasons[i] != c.Reasons[i] {
				t.Errorf("%s|%s: reasons = %v, want %v", c.Path, c.Query, score.Reasons, c.Reasons)
				break
			}
		}
	}
}

func TestFrontMatterParseVectors(t *testing.T) {
	var v struct {
		Parse []struct {
			Text   string            `json:"text"`
			Expect map[string]string `json:"expect"`
			Why    string            `json:"why"`
		} `json:"parse"`
	}
	load(t, "frontmatter.json", &v)

	if len(v.Parse) == 0 {
		t.Fatal("no parse cases loaded")
	}
	for _, c := range v.Parse {
		got := ParseFrontMatter(c.Text)
		if len(got) != len(c.Expect) {
			t.Errorf("ParseFrontMatter(%q) = %v, want %v\n  why: %s",
				c.Text, got, c.Expect, c.Why)
			continue
		}
		for k, want := range c.Expect {
			if got[k] != want {
				t.Errorf("ParseFrontMatter(%q)[%q] = %q, want %q\n  why: %s",
					c.Text, k, got[k], want, c.Why)
			}
		}
	}
}

func TestListVectors(t *testing.T) {
	var v struct {
		List []struct {
			Input  any      `json:"input"`
			Expect []string `json:"expect"`
			Why    string   `json:"why"`
		} `json:"list"`
	}
	load(t, "frontmatter.json", &v)

	for _, c := range v.List {
		s, isString := c.Input.(string)
		if !isString {
			continue // real-list and null shapes are the manifest reader's job
		}
		got := SplitList(s)
		if len(got) != len(c.Expect) {
			t.Errorf("SplitList(%q) = %v, want %v\n  why: %s", s, got, c.Expect, c.Why)
			continue
		}
		for i := range got {
			if got[i] != c.Expect[i] {
				t.Errorf("SplitList(%q) = %v, want %v", s, got, c.Expect)
				break
			}
		}
	}
}
