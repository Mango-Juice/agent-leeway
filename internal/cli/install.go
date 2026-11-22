package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/install"
	"github.com/Mango-Juice/agent-leeway/internal/meta"
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
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	targets := install.Pick(*claude, *codex)
	if len(targets) == 0 {
		fmt.Fprintln(errw, "Neither Claude Code nor Codex was found on this computer; pass --claude or --codex to install anyway.")
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
	if !*dry {
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

func cmdSelfUpdate(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(errw)
	check := fs.Bool("check", false, "look up the latest release and print how to update (downloads nothing)")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	if !*check {
		fmt.Fprintf(errw, "%s does not replace itself. Use `%s self-update --check`, then the install line it prints.\n", config.Name, config.Name)
		return 2
	}
	api := os.Getenv("LEEWAY_RELEASE_API")
	if api == "" {
		api = meta.LatestReleaseAPI()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	req.Header.Set("User-Agent", config.Name+"/"+meta.Version) // no personal data
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(errw, "could not check for updates:", err)
		return 1
	}
	defer resp.Body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel) != nil || rel.TagName == "" {
		fmt.Fprintf(errw, "could not check for updates (HTTP %d)\n", resp.StatusCode)
		return 1
	}
	rel.TagName, rel.HTMLURL = printable(rel.TagName), printable(rel.HTMLURL)
	if !newer(rel.TagName, meta.Version) {
		fmt.Fprintf(out, "%s %s is up to date (latest release %s).\n", config.Name, meta.Version, rel.TagName)
		return 0
	}
	fmt.Fprintf(out, "A newer release is available: %s (you have %s). %s\nTo update, run the install line again:\n  macOS/Linux: %s\n  Windows:     %s\n",
		rel.TagName, meta.Version, rel.HTMLURL, meta.InstallSh(), meta.InstallPs1())
	return 0
}
