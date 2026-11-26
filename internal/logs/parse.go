package logs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var (
	commitRE    = py.Re(`\[([^\]\s]+)(?: \(root-commit\))? ([0-9a-f]{7,40})\]`)
	exitRE      = py.Re(`(?:Process exited with code|Exit code:?)\s*(-?\d+)`)
	patchFileRE = py.Re(`\*\*\* (?:Update|Add|Delete) File: (.+)`)
	codexReadRE = py.Re(`^\s*(?:cat|sed -n \S+|head(?: -n ?\d+)?|tail(?: -n ?\d+)?|nl -ba)\s+(\S+)\s*$`)
	claudeRead  = py.Re(`^\s*(?:cat|sed -n \S+|head(?: -n ?\d+)?|tail(?: -n ?\d+)?)\s+(\S+)\s*$`)
)

// builder collects one thread while a file is read (the reference's Thread class).
type builder struct {
	tool, path string
	meta       metaV
	trigger    py.Value
	steps      []Step
	calls      []Call
	turns      []Turn
	commits    []Commit
	pending    []int
	images     []Image
	imageCall  map[int]int // image index -> call index (Codex: the call's response is known only later)
	startRaw   string
	endRaw     string
}

// metaV holds thread fields as raw values while reading ("a or b" assignments keep Python's semantics).
type metaV struct {
	threadID, sessionID, parentID, source, cwd, model, version, originator, entrypoint py.Value
	nAborted, nCompactions                                                             int
	costUSD, linesAdded                                                                py.Value
	hasFinalMsg                                                                        bool
	launchSource                                                                       py.Value
	typedPrompts                                                                       int
}

func (b *builder) ts(v py.Value) {
	if v.K != py.Str || v.S == "" {
		return
	}
	if b.startRaw == "" || v.S < b.startRaw {
		b.startRaw = v.S
	}
	if b.endRaw == "" || v.S > b.endRaw {
		b.endRaw = v.S
	}
}

func (b *builder) addStep(ts py.Value, uncached, cached, cw, output, reasoning int64, afterCompaction bool) {
	seq := len(b.steps)
	b.steps = append(b.steps, Step{Seq: seq, TS: tsOf(ts), Trigger: FromValue(b.trigger), Uncached: uncached, Cached: cached,
		CacheWrite: cw, Output: output, Reasoning: reasoning, InputTotal: uncached + cached + cw, AfterCompaction: afterCompaction})
	for _, ci := range b.pending {
		b.calls[ci].Step = seq
	}
	b.pending = b.pending[:0]
}

func (b *builder) addCall(c Call, step int) int {
	c.Step = step
	b.calls = append(b.calls, c)
	if step < 0 {
		b.pending = append(b.pending, len(b.calls)-1)
	}
	return len(b.calls) - 1
}

func (b *builder) lastStep() int { return len(b.steps) - 1 } // -1 when there is no step yet

func (b *builder) addCommits(text string, ts py.Value, cwd NS) {
	for _, m := range commitRE.FindAllStringSubmatch(text, -1) {
		b.commits = append(b.commits, Commit{TS: tsOf(ts), Branch: strings.Clone(m[1]), SHA: strings.Clone(m[2]), Cwd: cwd}) // not the whole output kept
	}
}

// outText is the reference's out_text(): a tool result as the text the model saw.
func outText(v py.Value) string {
	switch v.K {
	case py.Null:
		return ""
	case py.Str:
		return v.S
	case py.Arr:
		parts := make([]string, len(v.A))
		for i, x := range v.A {
			if x.K == py.Obj {
				parts[i] = py.PyStr(x.Get("text").Or(x.Get("content")).Or(py.String("")))
			} else {
				parts[i] = py.PyStr(x)
			}
		}
		return strings.Join(parts, "\n")
	case py.Obj:
		return py.PyStr(v.Get("output").Or(v.Get("content")).Or(py.String(py.Dumps(v))))
	}
	return py.PyStr(v)
}

func (b *builder) finish() *File {
	last := len(b.steps) - 1
	if last < 0 {
		last = 0
	}
	for i := range b.calls {
		if b.calls[i].Step < 0 {
			b.calls[i].Step = last
		}
	}
	m := b.meta
	th := Thread{Tool: b.tool, File: b.path, SessionID: FromValue(m.sessionID), ParentID: FromValue(m.parentID), Source: FromValue(m.source),
		Cwd: FromValue(m.cwd), Start: NaT, End: NaT, Model: FromValue(m.model), Version: FromValue(m.version), NAborted: m.nAborted,
		NCompactions: m.nCompactions, CostUSD: m.costUSD, LinesAdded: m.linesAdded, HasFinalMsg: m.hasFinalMsg, LaunchSource: FromValue(m.launchSource), TypedPrompts: m.typedPrompts,
		Originator: FromValue(m.originator), Entrypoint: FromValue(m.entrypoint)}
	if b.startRaw != "" {
		th.Start, th.End = ParseTime(b.startRaw), ParseTime(b.endRaw)
	}
	// one thread id per FILE (Codex continuation files reuse thread ids; Claude sub-agent names repeat)
	base := filepath.Base(b.path)
	if b.tool == "codex" {
		th.ThreadID = fileID(b.path)
	} else {
		th.ThreadID = pySlice(base, 0, -6) + "~" + H(b.path)[:6]
	}
	if fi, err := os.Stat(b.path); err == nil {
		th.Bytes = fi.Size()
	}
	th.NSteps, th.NCalls = len(b.steps), len(b.calls)
	for _, t := range b.turns {
		if t.Kind == "human" {
			th.NHumanTurns++
		}
	}
	return &File{Thread: th, Steps: b.steps, Calls: b.calls, Turns: b.turns, Commits: b.commits, Images: b.images}
}

// pySlice is s[a:b] in Python for code points; negative indexes count from the end.
func pySlice(s string, a, b int) string {
	r := py.Runes(s)
	n := len(r)
	norm := func(i int) int {
		if i < 0 {
			i += n
			if i < 0 {
				i = 0
			}
		}
		if i > n {
			i = n
		}
		return i
	}
	a, b = norm(a), norm(b)
	if a >= b {
		return ""
	}
	var out []byte
	for _, x := range r[a:b] {
		out = py.AppendRune(out, x)
	}
	return string(out)
}

// fileID is the Codex thread id taken from a rollout file name.
func fileID(path string) string {
	return pySlice(filepath.Base(strings.TrimSuffix(path, ".zst")), -41, -6)
}

func inStrs(v py.Value, set ...string) bool {
	if v.K != py.Str {
		return false
	}
	for _, s := range set {
		if v.S == s {
			return true
		}
	}
	return false
}

func isStr(v py.Value, s string) bool { return v.K == py.Str && v.S == s }

// pyIn is Python's "x in container" for a string x (list membership or substring).
func pyIn(x string, container py.Value) bool {
	switch container.K {
	case py.Arr:
		for _, e := range container.A {
			if isStr(e, x) {
				return true
			}
		}
	case py.Str:
		return strings.Contains(container.S, x)
	case py.Obj:
		_, ok := container.O.Get(x)
		return ok
	}
	return false
}
