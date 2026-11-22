// Package cli is the command line: `agent-leeway <command> [options]`.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/install"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/meta"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
)

type command struct {
	name, help string
	hidden     bool
	run        func(args []string, out, errw io.Writer) int
}

var commands []command

func register(c command) { commands = append(commands, c) }

// Main runs one command and returns the exit code.
func Main(args []string, out, errw io.Writer) int {
	messages.SetLang("en") // drift and report switch to their --lang after parsing flags
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(out)
		return 0
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Fprintf(out, "%s %s\n", config.Name, meta.Version)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:], out, errw)
		}
	}
	fmt.Fprintf(errw, "unknown command %q (see `%s --help`)\n", args[0], config.Name)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "%s: occasional check-up of your AI coding agent logs (Claude Code, Codex): mechanical cost leaks and intent drift.\n\n", meta.DisplayName)
	fmt.Fprintf(w, "usage: %s <command> [options]   (%s <command> -h for its options; %s --version)\n\ncommands:\n", config.Name, config.Name, config.Name)
	order := []string{"leak", "drift", "report", "compare", "idle", "rubric", "prices", "privacy", "install-skills", "uninstall-skills", "self-update"}
	for _, n := range order {
		for _, c := range commands {
			if c.name == n && !c.hidden {
				fmt.Fprintf(w, "  %-18s %s\n", c.name, c.help)
			}
		}
	}
}

func init() {
	register(command{name: "debug-write-skill", hidden: true, help: "write skills/agent-leeway/SKILL.md (the repository copy)", run: func(args []string, out, errw io.Writer) int {
		p := filepath.Join("skills", config.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			err = os.WriteFile(p, install.SkillFor(install.RepoCopy, ""), 0o644)
			if err == nil {
				fmt.Fprintln(out, "wrote", p)
				return 0
			}
		}
		fmt.Fprintln(errw, "could not write", p)
		return 1
	}})
	register(command{name: "debug-dump", hidden: true, help: "write the intermediate tables (parity checks against the Python reference)", run: cmdDump})
}

// cmdDump reads logs and writes the tables in the reference's dump format.
func cmdDump(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("debug-dump", flag.ContinueOnError)
	fs.SetOutput(errw)
	claude := fs.String("claude-dir", config.ClaudeDir(), "Claude Code transcripts folder")
	codex := fs.String("codex-dir", config.CodexDir(), "Codex sessions folder")
	archived := fs.Bool("archived", false, "also read Codex archived_sessions (the Python reference does not)")
	since := fs.Float64("since-epoch", 0, "only files modified since this Unix time (seconds)")
	root := fs.String("root", "", "paths in the dump are relative to this folder (default: the parent of the log folders)")
	dst := fs.String("out", "", "output folder")
	workers := fs.Int("workers", 0, "parallel readers (default: CPUs)")
	cacheDir := fs.String("cache", "", "per-file cache folder (default: none)")
	if fs.Parse(args) != nil || *dst == "" {
		fmt.Fprintln(errw, "usage: debug-dump --out <folder> [--claude-dir ...] [--codex-dir ...] [--since-epoch N] [--root ...]")
		return 2
	}
	dirs := []string{*codex}
	if *archived {
		dirs = append(dirs, filepath.Join(filepath.Dir(*codex), "archived_sessions"))
	}
	t0 := time.Now()
	srcs := logs.Discover(logs.Discovery{CodexDirs: dirs, ClaudeDir: *claude, SinceEpoch: *since})
	var fc *logs.FileCache
	if *cacheDir != "" {
		fc = logs.NewFileCache(*cacheDir)
	}
	files, skipped := logs.Load(srcs, *workers, fc)
	t1 := time.Now()
	tb := pipeline.FromFiles(files)
	tb.Build()
	t2 := time.Now()
	r := *root
	if r == "" {
		r = commonParent(*claude, *codex)
	}
	if err := tb.Dump(r, *dst); err != nil {
		fmt.Fprintln(errw, err)
		return 1
	}
	fmt.Fprintf(errw, "%d files (%d unreadable) read in %.2fs, tables built in %.2fs; %s\n", len(files), skipped,
		t1.Sub(t0).Seconds(), t2.Sub(t1).Seconds(), statsLine(tb.Stats))
	return 0
}

func commonParent(a, b string) string {
	a, b = filepath.Clean(a), filepath.Clean(b)
	for !strings.HasPrefix(b+string(filepath.Separator), a+string(filepath.Separator)) {
		p := filepath.Dir(a)
		if p == a {
			return a
		}
		a = p
	}
	return a
}

// statsLine reports lines and records that were not used, so nothing is dropped silently.
func statsLine(s logs.Stats) string {
	parts := []string{fmt.Sprintf("%d broken lines, %d non-object lines skipped", s.Broken, s.NotObject)}
	n := 0
	for _, v := range s.Unknown {
		n += v
	}
	if n > 0 {
		var kinds []string
		for k, v := range s.Unknown {
			kinds = append(kinds, fmt.Sprintf("%s x%d", k, v))
		}
		if len(kinds) > 6 {
			kinds = append(kinds[:6], "…")
		}
		parts = append(parts, fmt.Sprintf("%d records of unknown type ignored (%s)", n, strings.Join(kinds, ", ")))
	}
	return strings.Join(parts, "; ")
}

// helpRC: -h is a successful request for help (exit 0); a bad option is a usage error (exit 2).
func helpRC(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}
