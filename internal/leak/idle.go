package leak

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// TTLMinutes is the assumed prompt-cache lifetime.
var TTLMinutes = map[string]float64{"claude": 60, "codex": 10}

// tailLines returns the decoded lines of the last maxBytes of a file (the first one may be partial).
func tailLines(path string, maxBytes int64) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	off := fi.Size() - maxBytes
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := strings.Split(py.DecodeBytes(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// Last is the last model response of a transcript.
type Last struct {
	Tool          string
	TS            string
	Model         logs.NS
	ContextTokens int64
	FiveMinute    bool // Claude: the last response wrote only 5-minute cache
	Cwd           logs.NS
}

// LastResponse finds the last model response, its time and context size (idle.last_response).
func LastResponse(path string) *Last {
	model := logs.NS{}
	modelDone := false
	lines := tailLines(path, 4_000_000)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, `"assistant"`) && strings.Contains(line, `"usage"`) { // Claude Code
			if o, err := py.Loads(line); err == nil && o.K == py.Obj {
				m := o.Get("message")
				u := m.Get("usage")
				if u.Truthy() && o.Get("timestamp").Truthy() && !(m.Get("model").K == py.Str && m.Get("model").S == "<synthetic>") {
					n := func(k string) int64 { f, _ := u.Get(k).Or(py.Value{K: py.Int}).Num(); return int64(f) }
					cc := u.Get("cache_creation")
					w5, _ := cc.Get("ephemeral_5m_input_tokens").Num()
					w1, _ := cc.Get("ephemeral_1h_input_tokens").Num()
					return &Last{Tool: "claude", TS: o.Get("timestamp").S, Model: logs.FromValue(m.Get("model")),
						ContextTokens: n("input_tokens") + n("cache_read_input_tokens") + n("cache_creation_input_tokens"), FiveMinute: w5 > 0 && w1 == 0,
						Cwd: logs.FromValue(o.Get("cwd"))}
				}
			}
		}
		if strings.Contains(line, `"token_count"`) { // Codex rollout
			o, err := py.Loads(line)
			if err != nil || o.K != py.Obj {
				continue
			}
			lu := o.Get("payload").Get("info").Get("last_token_usage")
			if lu.Truthy() && o.Get("timestamp").Truthy() {
				if !modelDone {
					l2 := tailLines(path, 1_000_000)
					for j := len(l2) - 1; j >= 0; j-- {
						if strings.Contains(l2[j], `"turn_context"`) {
							if x, err := py.Loads(l2[j]); err == nil && x.K == py.Obj {
								model = logs.FromValue(x.Get("payload").Get("model"))
								break
							}
						}
					}
					modelDone = true
				}
				f, _ := lu.Get("input_tokens").Or(py.Value{K: py.Int}).Num()
				return &Last{Tool: "codex", TS: o.Get("timestamp").S, Model: model, ContextTokens: int64(f), Cwd: codexCwd(path)}
			}
		}
	}
	return nil
}

// Idle is the estimate for one transcript.
type Idle struct {
	Last
	File       string
	GapMin     float64
	TTLMin     float64
	Expired    bool
	ResumeCost float64
}

// Estimate is what resuming a transcript now would cost if its prompt cache expired (idle.estimate).
func Estimate(path string, now time.Time) *Idle {
	r := LastResponse(path)
	if r == nil {
		return nil
	}
	ts := logs.ParseTime(r.TS)
	if ts == logs.NaT {
		return nil
	}
	gap := float64(now.UnixNano()-ts) / 1e9 / 60
	m := ""
	if r.Model.OK {
		m = r.Model.S
	}
	var perTok float64
	ttl := TTLMinutes[r.Tool]
	if config.Legacy {
		p := config.LegacyPriceOf(r.Tool, m)
		perTok = p.In - p.Read
		if r.Tool == "claude" {
			perTok = p.Write - p.Read
		}
	} else {
		// resuming writes the whole context to the cache again: Claude at the write rate of the cache lifetime
		// the session used (1 hour unless its last write was a 5-minute one), Codex at the input or write rate
		p := config.Prices().Response(r.Tool, m, r.ContextTokens, 0, 0, 0, 0, !r.FiveMinute)
		perTok = p.Write - p.Read
		if r.Tool == "codex" {
			perTok = p.In - p.Read
			if p.TTL > 0 {
				ttl = p.TTL.Minutes()
			}
		} else if r.FiveMinute {
			ttl = 5
		}
	}
	for _, k := range []string{"LEAK_CACHE_TTL_MIN_" + strings.ToUpper(r.Tool), "LEAK_CACHE_TTL_MIN"} {
		if v := os.Getenv(k); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				ttl = f
				break
			}
		}
	}
	return &Idle{Last: *r, File: path, GapMin: gap, TTLMin: ttl, Expired: gap > ttl, ResumeCost: float64(r.ContextTokens) * perTok / 1e6}
}

// recentFiles: Claude transcripts one folder deep (no sub-agents) and Codex rollouts, modified in the last days.
func recentFiles(claudeDir, codexDir string, days int, now time.Time) []string {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	var out []string
	consider := func(p string) {
		if logs.IsSubagentPath(p) {
			return
		}
		if fi, err := os.Stat(p); err == nil && !fi.ModTime().Before(cutoff) {
			out = append(out, p)
		}
	}
	if ents, err := os.ReadDir(claudeDir); err == nil {
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			files, _ := filepath.Glob(filepath.Join(claudeDir, e.Name(), "*.jsonl"))
			for _, f := range files {
				if !strings.HasPrefix(filepath.Base(f), ".") {
					consider(f)
				}
			}
		}
	}
	for _, s := range logs.Discover(logs.Discovery{CodexDirs: []string{codexDir}}) {
		if strings.HasSuffix(s.Path, ".jsonl") {
			consider(s.Path)
		}
	}
	return out
}

// IdleReport lists recent sessions whose cache has probably expired and what resuming would cost.
func IdleReport(claudeDir, codexDir string, days int, now time.Time) string {
	var rows []*Idle
	for _, f := range recentFiles(claudeDir, codexDir, days, now) {
		if e := Estimate(f, now); e != nil && e.Expired && e.ResumeCost >= 0.05 && e.GapMin <= float64(days*1440) {
			rows = append(rows, e)
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].ResumeCost > rows[b].ResumeCost })
	lines := []string{F(messages.Idle["title"], "days", days)}
	if len(rows) == 0 {
		lines = append(lines, messages.Idle["none"])
		return strings.Join(lines, "\n")
	}
	total := len(rows)
	if len(rows) > 15 {
		rows = rows[:15]
	}
	for _, r := range rows {
		// the same project label as the leak check, and a short session key: no home path, no full file name
		lines = append(lines, F(messages.Idle["line"], "tool", r.Tool, "hours", r.GapMin/60, "ktok", float64(r.ContextTokens)/1000, "usd", r.ResumeCost,
			"where", pipeline.ProjectOf(r.Cwd), "file", shortKey(r.File)))
	}
	if total > len(rows) {
		lines = append(lines, F(messages.Idle["showing"], "n", len(rows), "m", total))
	}
	lines = append(lines, messages.Idle["advice1"], messages.Idle["advice2"])
	return strings.Join(lines, "\n")
}

// shortKey is the last 8 characters of a session id, from its file name.
func shortKey(path string) string {
	b := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".zst"), ".jsonl")
	return py.Tail(b, 8)
}

// codexCwd reads the working folder from a rollout's session_meta (first lines).
func codexCwd(path string) logs.NS {
	f, err := os.Open(path)
	if err != nil {
		return logs.NS{}
	}
	defer f.Close()
	buf := make([]byte, 256<<10)
	n, _ := io.ReadFull(f, buf)
	for _, line := range strings.SplitN(string(buf[:n]), "\n", 20) {
		if strings.Contains(line, `"session_meta"`) {
			if o, err := py.Loads(strings.TrimSuffix(line, "\r")); err == nil {
				return logs.FromValue(o.Get("payload").Get("cwd"))
			}
		}
	}
	return logs.NS{}
}
