// Command ctx is the Go implementation of the agent context tool.
//
// The Python implementation in bin/ctx.py is still canonical. This binary takes
// over one command at a time, and the two are held to the same behaviour by the
// golden vectors in tests/vectors plus a byte-for-byte output comparison on a
// real memory. Until every command is here and that comparison is clean, changes
// go to the Python side first.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Xeressa7/agent-context/go/internal/cli"
)

const usage = `ctx — agent context araci (Go)

Kullanim:
  ctx search <sorgu> [--repo YOL] [--all] [--top N] [--min PUAN]
  ctx index [--repo YOL] [--all] [--yaz] [--kontrol]
  ctx validate [--repo YOL] [--memory YOL] [--tam]
  ctx doctor [--repo YOL]
  ctx close [--repo YOL] [--yok [GEREKCE]] [--not YOL]... [--kontrol]
  ctx init <proje> [--memory-path YOL] [--role current|legacy] [--components LISTE]
  ctx migrate [--repo YOL] [--kuru]
  ctx bootstrap --proje AD [--memory-path YOL] [--role ...] [--components ...] [--kuru]
  ctx hook index | capture-check | flush  (oturum kancalari — harness cagirir)
   ctx preferences [--repo YOL] [--profile normal|ekonomik|manuel] [--index-budget N]
   ctx self-update [--check]  (kurulumu güncelle: fetch + fast-forward)

Butun komutlar Go'da.
`

func main() {
	os.Exit(run(cli.Env{Out: os.Stdout, Err: os.Stderr, In: os.Stdin}, os.Args[1:]))
}

// run dispatches argv. Env is injected (not read from the process) so tests
// can run the whole command line with buffers — including the flag parsing
// that used to be covered only by the cross-implementation comparison.
func run(env cli.Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	switch args[0] {
	case "search":
		return runSearch(env, args[1:])
	case "index":
		return runIndex(env, args[1:])
	case "validate":
		return runValidate(env, args[1:])
	case "doctor":
		return runDoctor(env, args[1:])
	case "close":
		return runClose(env, args[1:])
	case "init":
		return runInit(env, args[1:])
	case "migrate":
		return runMigrate(env, args[1:])
	case "bootstrap":
		return runBootstrap(env, args[1:])
	case "hook":
		return runHook(env, args[1:])
	case "preferences":
		return runPreferences(env, args[1:])
	case "self-update":
		return runSelfUpdate(env, args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "HATA: bilinmeyen komut: %s\n\n%s", args[0], usage)
		return 2
	}
}

// reorder moves positional arguments behind the flags.
//
// Go's flag package STOPS at the first non-flag token, so `search hakedis
// --top 3` would silently drop --top. Python's argparse accepts either order,
// and the two are compared verbatim — a flag that works in one and is ignored
// in the other is worse than a crash, because nothing says anything went wrong.
//
// Whether a flag consumes the next token is asked of the FlagSet itself rather
// than kept in a second list that would drift out of date.
func reorder(fs *flag.FlagSet, args []string) (flags, positional []string) {
	takesValue := func(name string) bool {
		f := fs.Lookup(strings.TrimLeft(name, "-"))
		if f == nil {
			return false // unknown: let flag.Parse produce the error
		}
		boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool })
		return !(ok && boolFlag.IsBoolFlag())
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && takesValue(arg) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return flags, positional
}

func runSearch(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.SearchOptions{}
	fs.StringVar(&opts.Repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.BoolVar(&opts.All, "all", false, "kapsam filtresi uygulama")
	fs.IntVar(&opts.Top, "top", 8, "en fazla kac kanonik sonuc gosterilsin")
	fs.Float64Var(&opts.Min, "min", 0, "bu puanin altindaki sonuclari gizle")
	flags, rest := reorder(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, "HATA: sorgu verilmedi.\n")
		return 2
	}
	opts.Query = rest[0]
	return cli.Search(env, opts)
}

func runIndex(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.IndexOptions{}
	fs.StringVar(&opts.Repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.BoolVar(&opts.All, "all", false, "kapsam filtresi uygulama")
	fs.BoolVar(&opts.Write, "yaz", false, "hedef dosyalardaki ctx:index blogunu tazele")
	fs.BoolVar(&opts.Check, "kontrol", false, "bloklar bayat mi? yazmaz; bayatsa cikis 1")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.Index(env, opts)
}

func runValidate(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.ValidateOptions{}
	fs.StringVar(&opts.Repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.StringVar(&opts.Memory, "memory", "", "hafiza deposunun yolu (manifest aramayi atlar)")
	fs.BoolVar(&opts.Full, "tam", false, "PII/secret taramasini da kos (pahali)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.Validate(env, opts)
}

func runDoctor(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var repo string
	fs.StringVar(&repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.Doctor(env, repo)
}

// runClose parses by hand rather than with flag: `--yok` takes an OPTIONAL
// argument (argparse's nargs="?"), which the flag package cannot express, and
// `--not` is repeatable.
func runClose(env cli.Env, args []string) int {
	opts := cli.CloseOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inline, hasInline := strings.Cut(arg, "=")
		next := func() (string, bool) {
			if hasInline {
				return inline, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch name {
		case "--repo", "-repo":
			v, ok := next()
			if !ok {
				fmt.Fprintf(os.Stderr, "HATA: --repo bir deger bekliyor.\n")
				return 2
			}
			opts.Repo = v
		case "--not", "-not":
			v, ok := next()
			if !ok {
				fmt.Fprintf(os.Stderr, "HATA: --not bir yol bekliyor.\n")
				return 2
			}
			opts.Notes = append(opts.Notes, v)
		case "--kontrol", "-kontrol":
			opts.Check = true
		case "--yok", "-yok":
			opts.None = true
			if hasInline {
				opts.NoneReason = inline
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.NoneReason = args[i]
			}
		default:
			fmt.Fprintf(os.Stderr, "HATA: bilinmeyen secenek: %s\n", arg)
			return 2
		}
	}
	return cli.Close(env, opts)
}

func runInit(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.InitOptions{}
	fs.StringVar(&opts.MemoryPath, "memory-path", "", "hafiza deposunun yolu")
	fs.StringVar(&opts.Role, "role", "current", "kapsam etiketi (current|legacy)")
	fs.StringVar(&opts.Components, "components", "app", "bilesen listesi")
	flags, rest := reorder(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, "HATA: proje adi verilmedi.\n")
		return 2
	}
	opts.Project = rest[0]
	return cli.Init(env, opts)
}

func runMigrate(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.MigrateOptions{}
	fs.StringVar(&opts.Repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.BoolVar(&opts.Dry, "kuru", false, "hicbir sey yazma, ne yapacagini goster")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.Migrate(env, opts)
}

func runBootstrap(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.BootstrapOptions{}
	fs.StringVar(&opts.Project, "proje", "", "proje adi (zorunlu)")
	fs.StringVar(&opts.MemoryPath, "memory-path", "", "hafiza deposunun yolu")
	fs.StringVar(&opts.Role, "role", "current", "kapsam etiketi (current|legacy)")
	fs.StringVar(&opts.Components, "components", "", "bilesen listesi")
	fs.BoolVar(&opts.Dry, "kuru", false, "hicbir sey yazma, ne yapacagini goster")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opts.Project == "" {
		fmt.Fprint(os.Stderr, "HATA: --proje zorunlu.\n")
		return 2
	}
	return cli.Bootstrap(env, opts)
}

// runPreferences scans the raw args for --index-budget the way argparse sees
// "given": a passed-but-invalid value (0, negative) must reach the command
// and fail there, not look like "not passed".
func runPreferences(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("preferences", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.PreferencesOptions{}
	fs.StringVar(&opts.Repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.StringVar(&opts.Profile, "profile", "", "hazir profil (normal|ekonomik|manuel)")
	fs.IntVar(&opts.Budget, "index-budget", 0, "harita butcesi, karakter")
	for _, a := range args {
		name := a
		if i := strings.Index(a, "="); i >= 0 {
			name = a[:i]
		}
		if name == "-index-budget" || name == "--index-budget" {
			opts.BudgetGiven = true
			break
		}
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.Preferences(env, opts)
}

func runSelfUpdate(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := cli.SelfUpdateOptions{}
	fs.BoolVar(&opts.Check, "check", false, "yalniz bak, hicbir sey yazma")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.SelfUpdate(env, opts)
}

// runHook dispatches the SessionStart hooks. They live in the binary rather
// than in copied script files so that a working install needs nothing but ctx.
func runHook(env cli.Env, args []string) int {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var repo, transcript, tool, reason string
	fs.StringVar(&repo, "repo", "", "manifest aramasinin baslayacagi dizin")
	fs.StringVar(&transcript, "transcript", "", "[flush] okunacak oturum dokumu (JSONL)")
	fs.StringVar(&tool, "arac", "", "[flush] claude | codex | hermes")
	fs.StringVar(&reason, "sebep", "", "[flush] sessionend | precompact")
	flags, rest := reorder(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, "HATA: hook adi verilmedi (index | capture-check | flush).\n")
		return 2
	}
	switch rest[0] {
	case "index":
		return cli.HookIndex(env, repo)
	case "capture-check":
		return cli.HookCaptureCheck(env, repo)
	case "flush":
		return cli.HookFlush(env, cli.FlushOptions{
			Repo: repo, Transcript: transcript, Tool: tool, Reason: reason})
	default:
		fmt.Fprintf(os.Stderr, "HATA: bilinmeyen hook: %s\n", rest[0])
		return 2
	}
}
