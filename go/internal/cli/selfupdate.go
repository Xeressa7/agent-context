package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Kurulum güncellemesi: ev dizinindeki kanonik kopya (`~/.agent-context`)
// fetch + fast-forward ile çekilir.
//
// Kurallar: kirli ağaçta DURULUR (kullanıcının düzenlemesi ezilmez), derleme
// OTOMATİK yapılmaz — çalışan binary Windows'ta kilitlidir, sarmalayıcı
// derlenmemişse Python'a düşer; `ctx doctor` bayatlığı gösterir.
// `--check` yalnız bakar, hiçbir şey yazmaz.

// SelfUpdateOptions mirrors the Python `ctx self-update` flags.
type SelfUpdateOptions struct {
	Check bool
}

// installRoot is the updated installation: always the canonical copy in the
// home directory (like doctor's staleness check, not the running executable).
func installRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".agent-context")
	}
	return filepath.Join(home, ".agent-context")
}

// runGit runs git in dir and returns (stdout, exit code). Like the Python
// helper, git's absence and git's failure look the same.
func runGit(dir string, args ...string) (string, int) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", 1
	}
	return strings.TrimSpace(out.String()), 0
}

func readVersion(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return "?"
	}
	if v := strings.TrimSpace(string(raw)); v != "" {
		return v
	}
	return "?"
}

// SelfUpdate checks for or applies an update of the installation.
func SelfUpdate(env Env, opts SelfUpdateOptions) int {
	root := installRoot()
	if info, err := os.Stat(filepath.Join(root, ".git")); err != nil || !info.IsDir() {
		env.printf("HATA: kurulum bir git deposu değil: %s\n", root)
		return 2
	}
	if rev, _ := runGit(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); rev == "" {
		env.printf("HATA: uzak depo takibi yok — elle güncelleyin.\n")
		return 2
	}
	if _, code := runGit(root, "fetch", "origin", "--quiet"); code != 0 {
		env.printf("HATA: uzak depo erişilemedi — çevrimdışı olabilirsiniz.\n")
		return 2
	}
	local := readVersion(root)
	remote, _ := runGit(root, "show", "@{u}:VERSION")
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = "?"
	}
	behind := -1
	if n, _ := runGit(root, "rev-list", "--count", "HEAD..@{u}"); n != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			behind = v
		}
	}
	if behind <= 0 {
		env.printf("✓ güncel (sürüm %s)\n", local)
		return 0
	}
	if opts.Check {
		env.printf("güncelleme var: sürüm %s → %s (%d commit)\n", local, remote, behind)
		env.printf("  uygulamak için: ctx self-update\n")
		return 0
	}
	dirty := 0
	if st, _ := runGit(root, "status", "--porcelain"); st != "" {
		for _, line := range strings.Split(st, "\n") {
			if strings.TrimSpace(line) != "" {
				dirty++
			}
		}
	}
	if dirty > 0 {
		env.printf("! çalışma ağacı kirli (%d dosya) — önce commit/stash yapın.\n", dirty)
		return 1
	}
	if _, code := runGit(root, "pull", "--ff-only"); code != 0 {
		env.printf("HATA: çekilemedi (fast-forward dışı?) — elle bakın.\n")
		return 2
	}
	env.printf("✓ güncellendi: sürüm %s → %s\n", local, readVersion(root))
	env.printf("  derleme: go varsa `go build -o bin/ctx.exe ./go/cmd/ctx` " +
		"(yoksa Python sürümü koşar; `ctx doctor` bayatlığı gösterir)\n")
	return 0
}
