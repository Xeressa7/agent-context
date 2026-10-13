package cli

import (
	"os"
	"path/filepath"
	"regexp"
)

// skillNames are the model-invoked verbs, listed in one place.
var skillNames = []string{"ctx-search", "ctx-write", "ctx-close"}

// skillRoots: `.agents/skills` is the Codex/agents convention, `.claude/skills`
// is where Claude Code looks. Same content in both, or one harness goes blind.
var skillRoots = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".claude", "skills"),
}

// legacySkillNames maps the pre-rename Turkish skill names to the current ones.
var legacySkillNames = [][2]string{
	{"ctx-ara", "ctx-search"},
	{"ctx-yaz", "ctx-write"},
	{"ctx-kapat", "ctx-close"},
}

// syncSkills installs every skill template under every root.
//
// Same promise as init: a file that exists is never overwritten UNLESS update
// is set. A differing file is reported (it is either an older template or the
// user's edit; we cannot tell which). write=false only computes the report.
func syncSkills(cwd string, write, update bool) (created, skipped []string, err error) {
	renamed := map[string]bool{} // dry run: dirs a real run would have renamed
	for _, root := range skillRoots {
		rel := filepath.ToSlash(root)
		for _, pair := range legacySkillNames {
			oldDir := filepath.Join(cwd, root, pair[0])
			newDir := filepath.Join(cwd, root, pair[1])
			if _, e := os.Stat(oldDir); e != nil {
				continue
			}
			if _, e := os.Stat(newDir); e == nil {
				skipped = append(skipped, rel+"/"+pair[0]+" ve "+pair[1]+" ikisi de var — ikisine de DOKUNULMADI, eskisini elle silin")
				continue
			}
			if write {
				if err := os.Rename(oldDir, newDir); err != nil {
					return created, skipped, err
				}
				renameSkillInside(filepath.Join(newDir, "SKILL.md"), pair[0], pair[1])
			} else {
				renamed[newDir] = true
			}
			created = append(created, rel+"/"+pair[0]+" -> "+pair[1]+" (eski ad tasindi)")
		}
	}

	for _, name := range skillNames {
		fresh, e := template("skills/" + name + "/SKILL.md")
		if e != nil {
			skipped = append(skipped, "skills/"+name+"/SKILL.md (sablon yok — ATLANDI)")
			continue
		}
		for _, root := range skillRoots {
			shown := filepath.ToSlash(filepath.Join(root, name, "SKILL.md"))
			target := filepath.Join(cwd, root, name, "SKILL.md")
			if renamed[filepath.Dir(target)] {
				continue // the migration line above already says it
			}
			raw, e := os.ReadFile(target)
			switch {
			case e == nil && normalizeNewlines(string(raw)) == normalizeNewlines(fresh):
				skipped = append(skipped, shown+" (mevcut)")
			case e == nil && update:
				if write {
					if err := writeText(target, fresh); err != nil {
						return created, skipped, err
					}
				}
				created = append(created, shown+" (sablonla guncellendi)")
			case e == nil || !os.IsNotExist(e):
				skipped = append(skipped, shown+" (farklı — şablondan eski ya da elle değiştirilmiş; DOKUNULMADI, ezmek için --update-skills)")
			default:
				if write {
					if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
						return created, skipped, err
					}
					if err := writeText(target, fresh); err != nil {
						return created, skipped, err
					}
				}
				created = append(created, shown+" (skill)")
			}
		}
	}
	return created, skipped, nil
}

// renameSkillInside fixes the `name:` line and the `# title` of a moved skill,
// keeping the rest (and its line endings) as the user had it.
func renameSkillInside(path, oldName, newName string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := string(raw)
	text = regexp.MustCompile(`(?m)^name:[ \t]*`+regexp.QuoteMeta(oldName)+`[ \t]*(\r?)$`).
		ReplaceAllString(text, "name: "+newName+"$1")
	text = regexp.MustCompile(`(?m)^# `+regexp.QuoteMeta(oldName)+`\b`).
		ReplaceAllString(text, "# "+newName)
	os.WriteFile(path, []byte(text), 0o644)
}
