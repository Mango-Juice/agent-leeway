package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"

	"golang.org/x/text/unicode/norm"
)

// Session is one user-started conversation with every sub-agent thread it spawned.
type Session struct {
	Key        string
	Tool       string
	SessionID  string
	Cwd        logs.NS
	Model      logs.NS
	RootSource logs.NS
	NThreads   int
	Start, End int64
	NSteps     int
	WCost      float64 // NaN when the session has no responses
	Project    string
	Exclude    bool
}

// BuildSessions prices every response and rolls threads up into sessions (sessions.build).
func (t *Tables) BuildSessions() {
	// threads with a session id, and the first model seen per thread id
	model := map[string]logs.NS{}
	userThreads := map[string]bool{}
	var th []int
	for i, x := range t.Threads {
		if !x.SessionID.OK {
			continue
		}
		th = append(th, i)
		if _, ok := model[x.ThreadID]; !ok {
			model[x.ThreadID] = x.Model
		}
		if x.Source.OK && x.Source.S == "user" {
			userThreads[x.ThreadID] = true
		}
	}
	if config.Legacy {
		for i := range t.Steps {
			s := &t.Steps[i]
			s.ThreadModel = model[s.ThreadID]
			m := ""
			if s.ThreadModel.OK {
				m = s.ThreadModel.S
			} else if _, ok := model[s.ThreadID]; !ok {
				m = "nan" // str(NaN): matches no price prefix
			}
			p := config.LegacyPriceOf(s.Tool, m)
			s.PIn, s.PRead, s.PWrite, s.POut = p.In, p.Read, p.Write, p.Out
			s.WCost = (float64(s.Uncached)*s.PIn + float64(s.Cached)*s.PRead + float64(s.CacheWrite)*s.PWrite + float64(s.Output)*s.POut) / 1e6
		}
	} else {
		t.priceResponses(model)
	}
	// human turns only count in the user-facing thread
	t.TurnsHuman = nil
	for _, u := range t.Turns {
		if userThreads[u.ThreadID] {
			t.TurnsHuman = append(t.TurnsHuman, u)
		}
	}
	// threads sorted by start (stable, NaT last); root = first user thread per session, else its first thread
	sort.SliceStable(th, func(a, b int) bool { return lessTS(t.Threads[th[a]].Start, t.Threads[th[b]].Start) })
	key := func(i int) string { return t.Threads[i].Tool + ":" + t.Threads[i].SessionID.S }
	var roots []int
	have := map[string]bool{}
	for _, i := range th {
		if x := t.Threads[i]; x.Source.OK && x.Source.S == "user" && !have[key(i)] {
			have[key(i)] = true
			roots = append(roots, i)
		}
	}
	userRoot := map[string]bool{}
	for k := range have {
		userRoot[k] = true
	}
	for _, i := range th {
		if k := key(i); !userRoot[k] && !have[k] {
			have[k] = true
			roots = append(roots, i)
		}
	}
	type agg struct {
		n          int
		start, end int64
		steps      int
		cost       kahan
		hasCost    bool
	}
	A := map[string]*agg{}
	for _, i := range th {
		k := key(i)
		a := A[k]
		if a == nil {
			a = &agg{start: NaT, end: NaT}
			A[k] = a
		}
		a.n++
		x := t.Threads[i]
		if x.Start != NaT && (a.start == NaT || x.Start < a.start) {
			a.start = x.Start
		}
		if x.End != NaT && (a.end == NaT || x.End > a.end) {
			a.end = x.End
		}
	}
	for _, s := range t.Steps {
		if !s.Key.OK {
			continue
		}
		if a := A[s.Key.S]; a != nil {
			a.steps++
			a.cost.add(s.WCost)
			a.hasCost = true
		}
	}
	t.Sessions = nil
	for _, i := range roots {
		x := t.Threads[i]
		a := A[key(i)]
		w := NaN
		if a.hasCost {
			w = a.cost.sum
		}
		t.Sessions = append(t.Sessions, Session{Key: key(i), Tool: x.Tool, SessionID: x.SessionID.S, Cwd: x.Cwd, Model: x.Model,
			RootSource: x.Source, NThreads: a.n, Start: a.start, End: a.end, NSteps: a.steps, WCost: w,
			Project: ProjectOf(x.Cwd), Exclude: a.steps == 0})
	}
}

// lessTS orders timestamps ascending with NaT last.
func lessTS(a, b int64) bool {
	if a == NaT {
		return false
	}
	if b == NaT {
		return true
	}
	return a < b
}

var (
	gitMu    sync.Mutex
	gitCache = map[string][2]string{}
)

// gitTop returns (toplevel, main repository top) for a folder, or empty strings. Read-only git call.
func gitTop(cwd string) (string, string) {
	gitMu.Lock()
	if v, ok := gitCache[cwd]; ok {
		gitMu.Unlock()
		return v[0], v[1]
	}
	gitMu.Unlock()
	top, main := gitTopUncached(cwd)
	gitMu.Lock()
	gitCache[cwd] = [2]string{top, main}
	gitMu.Unlock()
	return top, main
}

func gitTopUncached(cwd string) (string, string) {
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return "", ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--show-toplevel", "--git-common-dir").Output()
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	top := parts[0]
	common := ""
	if len(parts) > 1 {
		common = strings.TrimSpace(parts[1])
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(cwd, common)
	}
	if r, err := filepath.EvalSymlinks(common); err == nil {
		common = r
	}
	mainTop := top
	if filepath.Base(common) == ".git" {
		mainTop = filepath.Dir(common)
	}
	home := config.Home()
	if mainTop == home || mainTop == filepath.Dir(home) { // a repo spanning the home folder says nothing
		return "", ""
	}
	return top, mainTop
}

// ProjectOf names a session's project: the main git repository folder name, else the folder name (NFC).
func ProjectOf(cwd logs.NS) string {
	if !cwd.Truthy() {
		return "unknown"
	}
	_, main := gitTop(cwd.S)
	p := main
	if p == "" {
		p = cwd.S
	}
	name := baseName(strings.TrimRight(p, "/"))
	if name == "" {
		name = cwd.S
	}
	return norm.NFC.String(name)
}

// baseName is os.path.basename for paths found in logs (they may come from another OS: both separators).
func baseName(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	if i < 0 {
		return p
	}
	return p[i+1:]
}

// priceResponses prices each response by the model recorded on it (the thread's first model when the
// response has none) at the price table's rates, and sets the cache lifetime that applies before it:
// Claude, the lifetime of the thread's latest cache write (5 minutes or 1 hour as the log records it);
// Codex, the model's documented minimum lifetime.
func (t *Tables) priceResponses(threadModel map[string]logs.NS) {
	prices := config.Prices()
	user := map[string]bool{}
	for _, x := range t.Threads {
		if x.Source.OK && x.Source.S == "user" {
			user[x.ThreadID] = true
		}
	}
	lastTTL := map[string]time.Duration{}
	for i := range t.Steps {
		s := &t.Steps[i]
		s.ThreadModel = threadModel[s.ThreadID]
		m := s.Step.Model
		if !m.Truthy() || m.S == "<synthetic>" {
			m = s.ThreadModel
		}
		main := user[s.ThreadID] // Claude Code: 1-hour cache for the main conversation, 5 minutes for sub-agents
		p := prices.Response(s.Tool, m.S, s.Uncached, s.Cached, s.CacheWrite, s.CW5m, s.CW1h, main)
		s.PIn, s.PRead, s.PWrite, s.POut = p.In, p.Read, p.Write, p.Out
		s.WCost = (float64(s.Uncached)*s.PIn + float64(s.Cached)*s.PRead + float64(s.CacheWrite)*s.PWrite + float64(s.Output)*s.POut) / 1e6
		if !p.Known {
			name := m.S
			if name == "" {
				name = "(no model recorded)"
			}
			s.Assumed = name
		}
		if s.Tool == "codex" {
			s.TTL = p.TTL
			if s.TTL == 0 {
				s.TTL = 10 * time.Minute
			}
			continue
		}
		ttl, ok := lastTTL[s.ThreadID]
		if !ok {
			ttl = 5 * time.Minute
			if main {
				ttl = time.Hour
			}
		}
		s.TTL = ttl
		switch {
		case s.CW1h > 0:
			lastTTL[s.ThreadID] = time.Hour
		case s.CW5m > 0:
			lastTTL[s.ThreadID] = 5 * time.Minute
		case s.CacheWrite > 0 && !ok:
			lastTTL[s.ThreadID] = ttl
		}
	}
}
