package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

func init() {
	register(command{name: "leak", help: "leak check: cost leaks by cause and habit, direct vs automation (no model calls)", run: cmdLeak})
	register(command{name: "compare", help: "compare two `leak --json` outputs", run: cmdCompare})
	register(command{name: "idle", help: "recent sessions whose prompt cache has probably expired, and what resuming would cost", run: cmdIdle})
	register(command{name: "privacy", help: "where data lives and what leaves this computer", run: cmdPrivacy})
}

// logOpts are the options shared by commands that read logs.
type logOpts struct {
	since      string
	tool       string
	cache      string
	refresh    bool
	claudeDir  string
	codexDir   string
	noArchived bool
	workers    int
	lang       string
}

func addLogDirs(fs *flag.FlagSet, o *logOpts) {
	fs.StringVar(&o.claudeDir, "claude-dir", "", "Claude Code transcripts folder (default: "+config.ClaudeDir()+")")
	fs.StringVar(&o.codexDir, "codex-dir", "", "Codex sessions folder (default: "+config.CodexDir()+")")
}

func addCommon(fs *flag.FlagSet, o *logOpts) {
	fs.StringVar(&o.since, "since", "30d", "window, e.g. 30d, 2w, 12h")
	fs.StringVar(&o.tool, "tool", "", "only one tool: claude or codex (all = both, the default)")
	fs.StringVar(&o.cache, "cache", "", "cache folder (default: "+config.CacheDir()+")")
	fs.BoolVar(&o.refresh, "refresh", false, "ignore the cache and read the logs again")
	fs.BoolVar(&o.noArchived, "no-archived", false, "do not read Codex archived_sessions")
	fs.IntVar(&o.workers, "workers", 0, "parallel log readers (default: number of CPUs)")
	fs.StringVar(&o.lang, "lang", "", "language of the summary and report: en or ko (default: settings.json \"language\", then LC_ALL / LANG, then en)")
	addLogDirs(fs, o)
}

// dirs applies --claude-dir / --codex-dir, then settings.json, then the detected defaults.
func (o *logOpts) dirs() (claude, codex string, codexDirs []string) {
	st := config.UserSettings()
	claude, codex = o.claudeDir, o.codexDir
	if claude == "" {
		claude = st.ClaudeDir
	}
	if claude == "" {
		claude = config.ClaudeDir()
	}
	if codex == "" {
		codex = st.CodexDir
	}
	if codex == "" {
		codex = config.CodexDir()
	}
	codexDirs = []string{codex}
	if !o.noArchived {
		codexDirs = append(codexDirs, filepath.Join(filepath.Dir(filepath.Clean(codex)), "archived_sessions"))
	}
	return
}

func (o *logOpts) cacheDir() string {
	if o.cache != "" {
		return o.cache
	}
	return config.CacheDir()
}

func (o *logOpts) checkTool(errw io.Writer) bool {
	if o.tool == "all" {
		o.tool = ""
	}
	if o.tool != "" && o.tool != "claude" && o.tool != "codex" {
		fmt.Fprintln(errw, "--tool must be claude or codex")
		return false
	}
	if !config.ValidLanguage(o.lang) {
		fmt.Fprintln(errw, "--lang must be en or ko")
		return false
	}
	return true
}

var sinceRE = regexp.MustCompile(`^(\d+)\s*([dhw])$`)

// parseSince turns 30d / 2w / 12h into the window start.
func parseSince(s string, now time.Time) (time.Time, error) {
	m := sinceRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, fmt.Errorf("%s", messages.Cli["since_format"])
	}
	n, err := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
	if err != nil || n <= 0 || time.Duration(n) > (100*365*24*time.Hour)/unit { // at most about 100 years, no overflow
		return time.Time{}, fmt.Errorf("%s", messages.Cli["since_format"])
	}
	return now.Add(-time.Duration(n) * unit), nil
}

// explicitDirs reports whether the log folders were chosen by the user (flag, settings or environment); a
// missing chosen folder is an error, a missing default folder only means the tool is not used here.
func (o *logOpts) explicitDirs() (claude, codex bool) {
	st := config.UserSettings()
	claude = o.claudeDir != "" || st.ClaudeDir != "" || os.Getenv("LEEWAY_CLAUDE_DIR") != "" || os.Getenv("CLAUDE_CONFIG_DIR") != ""
	codex = o.codexDir != "" || st.CodexDir != "" || os.Getenv("LEEWAY_CODEX_DIR") != "" || os.Getenv("CODEX_HOME") != ""
	return
}

// loadTables reads logs touched since `from` (minus a day) and builds every table. It says which folders were
// read and how many session files each had, and fails when a chosen folder is missing or unreadable, or when no
// log file exists at all.
func loadTables(o *logOpts, from time.Time, errw io.Writer) (*pipeline.Tables, error) {
	claude, _, codexDirs := o.dirs()
	srcs, folders := logs.DiscoverReport(logs.Discovery{CodexDirs: codexDirs, ClaudeDir: claude, SinceEpoch: float64(from.Add(-24*time.Hour).UnixNano()) / 1e9})
	oldestClaude, err := reportFolders(o, folders, from, errw)
	if err != nil {
		return nil, err
	}
	fc := logs.NewFileCache(o.cacheDir())
	if o.refresh { // --refresh re-reads every file and rewrites its cache entry; entries of vanished files go
		for _, s := range srcs {
			fc.Forget(s.Path)
		}
		fc.Prune(srcs)
	}
	if len(srcs) > 300 {
		fmt.Fprintln(errw, py.Format(messages.Cli["reading"], map[string]any{"n": len(srcs)}))
	}
	files, skipped := logs.LoadReport(srcs, o.workers, fc)
	tb := pipeline.FromFiles(files)
	tb.Skipped = len(skipped)
	tb.OldestClaude = oldestClaude
	tb.Build()
	warnStats(tb, errw)
	for i, p := range skipped {
		if i == 5 {
			fmt.Fprintln(errw, py.Format(messages.Cli["skip_more"], map[string]any{"n": len(skipped) - 5}))
			break
		}
		fmt.Fprintln(errw, "  "+p)
	}
	return tb, nil
}

// reportFolders prints what each log folder held and turns missing or unreadable folders into errors.
func reportFolders(o *logOpts, folders []logs.Folder, from time.Time, errw io.Writer) (int64, error) {
	exClaude, exCodex := o.explicitDirs()
	total, inWindow := 0, 0
	var newest, oldestClaude int64
	var problems []string
	for i, f := range folders {
		if o.tool != "" && f.Tool != o.tool {
			continue
		}
		archived := f.Tool == "codex" && i > 0
		name := messages.Tool[f.Tool]
		if archived {
			name += " (archived)"
		}
		if f.Err != nil {
			explicit := (f.Tool == "claude" && exClaude) || (f.Tool == "codex" && exCodex && !archived)
			switch {
			case os.IsNotExist(f.Err) && (archived || !explicit):
				if !archived {
					fmt.Fprintln(errw, py.Format(messages.Cli["folder_absent"], map[string]any{"tool": name, "dir": f.Dir}))
				}
			case os.IsNotExist(f.Err):
				problems = append(problems, py.Format(messages.Cli["folder_missing"], map[string]any{"tool": name, "dir": f.Dir}))
			default:
				problems = append(problems, py.Format(messages.Cli["folder_unreadable"], map[string]any{"tool": name, "dir": f.Dir, "err": f.Err.Error()}))
			}
			continue
		}
		if archived && f.Files == 0 {
			continue
		}
		fmt.Fprintln(errw, py.Format(messages.Cli["folder_read"], map[string]any{"tool": name, "dir": f.Dir, "n": f.Files, "w": f.InWindow}))
		if f.Unreadable > 0 {
			fmt.Fprintln(errw, py.Format(messages.Cli["folder_sub_unreadable"], map[string]any{"n": f.Unreadable}))
		}
		total += f.Files
		inWindow += f.InWindow
		if f.Newest > newest {
			newest = f.Newest
		}
		if f.Tool == "claude" && f.Files > 0 {
			oldestClaude = f.Oldest
		}
	}
	if len(problems) > 0 {
		return 0, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	if total == 0 {
		return 0, fmt.Errorf("%s", messages.Cli["no_logs"])
	}
	if inWindow == 0 {
		fmt.Fprintln(errw, py.Format(messages.Cli["none_in_window"], map[string]any{"date": time.Unix(0, newest).In(time.Local).Format("2006-01-02")}))
	}
	// Claude Code deletes transcripts after 30 days by default: say so when the window reaches before the oldest one
	if oldestClaude > 0 && from.UnixNano() < oldestClaude {
		fmt.Fprintln(errw, py.Format(messages.Cli["claude_retention"], map[string]any{"date": time.Unix(0, oldestClaude).In(time.Local).Format("2006-01-02")}))
	}
	return oldestClaude, nil
}

// warnStats reports what could not be used instead of dropping it silently.
func warnStats(tb *pipeline.Tables, errw io.Writer) {
	s := tb.Stats
	if tb.Skipped > 0 {
		fmt.Fprintln(errw, py.Format(messages.Cli["skip_unreadable_n"], map[string]any{"n": tb.Skipped}))
	}
	if s.Broken > 0 || s.NotObject > 0 {
		fmt.Fprintln(errw, py.Format(messages.Cli["broken_lines"], map[string]any{"n": s.Broken + s.NotObject}))
	}
	n := 0
	var kinds []string
	for k, v := range s.Unknown {
		n += v
		kinds = append(kinds, k)
	}
	if n > 0 {
		if len(kinds) > 5 {
			kinds = append(kinds[:5], "…")
		}
		fmt.Fprintln(errw, py.Format(messages.Cli["unknown_records"], map[string]any{"n": n, "kinds": strings.Join(kinds, ", ")}))
	}
	if s.UsageOnlyNew > 0 {
		fmt.Fprintln(errw, py.Format(messages.Cli["usage_format_changed"], map[string]any{"n": s.UsageOnlyNew}))
	}
}

type leakSet struct {
	R, H, A, P, PH, PA *leak.Report
	E                  *leak.Events // the events behind them, for per-tool sums without reading the tables again
}

func leakReports(o *logOpts, since, now time.Time, withPrev bool, errw io.Writer) (leakSet, *pipeline.Tables, error) {
	span := now.Sub(since)
	prevSince := since.Add(-span)
	from := since
	if withPrev {
		from = prevSince
	}
	tb, err := loadTables(o, from, errw)
	if err != nil {
		return leakSet{}, nil, err
	}
	E := leak.LoadEvents(tb, o.tool)
	until := now.Add(time.Minute)
	loc := time.Local
	run := func(s, u time.Time, origin string) *leak.Report { return leak.Compute(E, s, u, o.tool, origin, loc) }
	x := leakSet{R: run(since, until, ""), H: run(since, until, "human"), A: run(since, until, "auto"), E: E}
	if withPrev {
		x.P, x.PH, x.PA = run(prevSince, since, ""), run(prevSince, since, "human"), run(prevSince, since, "auto")
	}
	return x, tb, nil
}

func printOut(out io.Writer, s string) { fmt.Fprintln(out, py.ReplaceSurrogates(s, "�")) }

func cmdLeak(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("leak", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o logOpts
	addCommon(fs, &o)
	top := fs.Int("top", 5, "sessions to list")
	asJSON := fs.Bool("json", false, "print JSON")
	noPrev := fs.Bool("no-prev", false, "skip the comparison with the previous window")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	if !o.checkTool(errw) {
		return 2
	}
	now := config.Now()
	since, err := parseSince(o.since, now)
	if err != nil {
		fmt.Fprintln(errw, err)
		return 2
	}
	x, _, err := leakReports(&o, since, now, !*noPrev, errw)
	if err != nil {
		fmt.Fprintln(errw, err)
		return 1
	}
	if *asJSON {
		printOut(out, py.DumpsOpt(leak.JSON(x.R, x.H, x.A, x.P, x.PH, x.PA), py.DumpOpts{Indent: 1}))
	} else {
		printOut(out, leak.Render(x.R, *top, x.P, x.H, x.A, x.PH, x.PA, now, time.Local))
		if !config.Legacy {
			// the leak side of "What to fix first", with what to do per tool
			m := newModel(x.R, nil, config.Language(o.lang), now, since, 0, nil, nil, errw)
			fmt.Fprint(out, py.ReplaceSurrogates("\n"+report.TerminalLeak(m), "�"))
		}
	}
	return 0
}

func cmdCompare(args []string, out, errw io.Writer) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(out, "usage: %s compare <before.json> <after.json>\n\nCompares two `%s leak --json` files: leak per $100 of spend by cause and by habit, before -> after,\nwith both window lengths and a warning when they differ or overlap.\n", config.Name, config.Name)
		return 0
	}
	if len(args) != 2 {
		fmt.Fprintf(errw, "usage: %s compare <before.json> <after.json>   (-h for help)\n", config.Name)
		return 2
	}
	var docs [2]py.Value
	for i, p := range args {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		if docs[i], err = py.Loads(py.DecodeBytes(b)); err != nil || docs[i].K != py.Obj {
			fmt.Fprintf(errw, "%s: not a `leak --json` file\n", p)
			return 1
		}
	}
	printOut(out, leak.Compare(docs[0], docs[1]))
	return 0
}

func cmdIdle(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("idle", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o logOpts
	addLogDirs(fs, &o)
	days := fs.Int("days", 3, "look at sessions used in the last N days")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	if *days <= 0 || *days > 3650 {
		fmt.Fprintln(errw, "--days must be between 1 and 3650")
		return 2
	}
	claude, codex, _ := o.dirs()
	printOut(out, leak.IdleReport(claude, codex, *days, config.Now()))
	return 0
}

func cmdPrivacy(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("privacy", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o logOpts
	addLogDirs(fs, &o)
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	claude, codex, codexDirs := o.dirs()
	printOut(out, messages.Privacy)
	src := "built in, read " + config.Prices().Read
	if config.Prices().Path != "" {
		src = config.Prices().Path
	}
	printOut(out, py.Format(messages.Cli["privacy_details"], map[string]any{"prices": src}))
	printOut(out, py.Format(messages.Cli["privacy_paths"], map[string]any{"cache": config.CacheDir(), "reports": config.ReportDir(),
		"settings": filepath.Join(config.ConfigDir(), "settings.json"), "claude": claude, "codex": strings.Join(codexDirs, ", ")}))
	_ = codex
	return 0
}
