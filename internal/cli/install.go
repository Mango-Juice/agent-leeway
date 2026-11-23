package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/install"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

func init() {
	register(command{name: "install-skills", help: "add the agent-leeway skill to Claude Code and/or Codex (--claude, --codex)", run: cmdInstallSkills})
	register(command{name: "uninstall-skills", help: "remove the agent-leeway skill (only files it wrote)", run: cmdUninstallSkills})
	register(command{name: "self-update", help: "--check: say whether a newer release exists (downloads nothing)", run: cmdSelfUpdate})
}

func skillFlags(name string, errw io.Writer) (*flag.FlagSet, *bool, *bool, *bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errw)
	claude := fs.Bool("claude", false, "Claude Code: "+install.Targets()[0].Dir)
	codex := fs.Bool("codex", false, "Codex: "+install.Targets()[1].Dir)
	dry := fs.Bool("dry-run", false, "show what would change; change nothing")
	return fs, claude, codex, dry
}

func cmdInstallSkills(args []string, out, errw io.Writer) int {
	fs, claude, codex, dry := skillFlags("install-skills", errw)
	auto := fs.Bool("auto", false, "for the installers: every agent found here; exit 3 (not 1) when none is found")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	if *auto && (*claude || *codex) {
		fmt.Fprintln(errw, "--auto picks the agents itself; leave out --claude and --codex")
		return 2
	}
	targets := install.Pick(*claude, *codex)
	if len(targets) == 0 {
		all := install.Targets()
		msg := fmt.Sprintf("No Claude Code or Codex found on this computer (no %s or %s folder)", all[0].Root, all[1].Root)
		if *auto {
			fmt.Fprintf(out, "Skills: not added. %s.\n", msg)
			return 3 // the installers tell this apart from an error
		}
		fmt.Fprintf(errw, "%s; pass --claude or --codex to install anyway.\n", msg)
		return 1
	}
	log, err := install.Install(targets, *dry)
	for _, l := range log {
		fmt.Fprintln(out, l)
	}
	if err != nil {
		fmt.Fprintln(errw, err)
		return 1
	}
	if !*dry && !*auto {
		fmt.Fprintf(out, "Ask your agent for a \"leeway report\" (or a leak check). Remove with `%s uninstall-skills`.\n", config.Name)
	}
	return 0
}

func cmdUninstallSkills(args []string, out, errw io.Writer) int {
	fs, claude, codex, dry := skillFlags("uninstall-skills", errw)
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	targets := install.Targets()
	if *claude || *codex {
		targets = install.Pick(*claude, *codex)
	}
	log, err := install.Uninstall(targets, *dry)
	for _, l := range log {
		fmt.Fprintln(out, l)
	}
	if err != nil {
		fmt.Fprintln(errw, err)
		return 1
	}
	return 0
}

// newer reports whether version a is newer than b (v-prefixed dotted numbers; a pre-release suffix sorts lower).
func newer(a, b string) bool {
	parse := func(v string) ([]int, bool) {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		core, pre, _ := strings.Cut(v, "-")
		var n []int
		for _, p := range strings.Split(core, ".") {
			x, _ := strconv.Atoi(p)
			n = append(n, x)
		}
		return n, pre != ""
	}
	x, px := parse(a)
	y, py_ := parse(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		var xi, yi int
		if i < len(x) {
			xi = x[i]
		}
		if i < len(y) {
			yi = y[i]
		}
		if xi != yi {
			return xi > yi
		}
	}
	return !px && py_
}

// printable drops control characters (terminal escape sequences) from text a server sent.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, py.Head(s, 200))
}
