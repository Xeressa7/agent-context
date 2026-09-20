package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Ortak test iskelesi: komutlar enjekte edilmis yaziciya yazar (Env), dosya
// etkileri t.TempDir()'da olur. Hicbir test ag kullanmaz, hicbir test
// $HOME'a dokunmaz (gereken yerde t.Setenv ile sahte ev).

func testEnv() (Env, *bytes.Buffer, *bytes.Buffer) {
	var out, err bytes.Buffer
	return Env{Out: &out, Err: &err}, &out, &err
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// chdir switch the working directory for commands that default to it
// (init, bootstrap); callers must not t.Parallel.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	})
}

// gitOK is a test-only git runner; it fails the test on error because a
// fixture that did not build proves nothing. (Production code has runGit,
// which reports the exit code instead of failing.)
func gitOK(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, errOut.String())
	}
	return strings.TrimSpace(out.String())
}

// fakeHome points $HOME/$USERPROFILE at a temp dir so bootstrap and
// self-update never touch the real installation.
func fakeHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// norm unifies line endings for comparison: the byte-level CRLF contract lives
// here as "normalize, then compare exact" — both implementations translate
// through \n, so normalized equality IS the contract.
func norm(s string) string {
	return normalizeNewlines(s)
}

func isWindows() bool { return runtime.GOOS == "windows" }
