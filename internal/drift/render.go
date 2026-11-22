package drift

import (
	"crypto/md5"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Length limits of the judging input (code points).
const (
	MaxInstr      = 5000
	MaxInstrTotal = 9000
	MaxU          = 2500
	MaxA          = 1200
)

// Clip shortens t to n code points with a note of how much was left out.
func Clip(t string, n int) string {
	l := py.Len(t)
	if l <= n {
		return t
	}
	return py.Head(t, n) + fmt.Sprintf(" …(%d chars omitted)", l-n)
}

var (
	reCommandName = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	reCommandArgs = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	reAnswer      = py.Re(`(?s)<send_user_message_question_reply>\s*(.*?)\s*</send_user_message_question_reply>`)
	reFileLine    = py.Re(`^##\s+(.+?):\s+(/\S.*|~/\S.*)$`)
	reMyRequest   = py.Re(`##\s*My request(?: for Codex)?:\s*`)
	reBrowser     = regexp.MustCompile(`(?s)<in-app-browser-context[^>]*>.*?</in-app-browser-context>`)
	reImageRef    = py.Re(`\[Image #\d+\]`)
	rePasted      = regexp.MustCompile(`</?pasted_content[^>]*>`)
	reReminder    = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
)

// removeImageTags is re.sub(r'<image\b[^>]*>(?:</image>)?', ”, t) with Python's Unicode \b.
func removeImageTags(t string) string {
	if !strings.Contains(t, "<image") {
		return t
	}
	var b strings.Builder
	i := 0
	for i < len(t) {
		if strings.HasPrefix(t[i:], "<image") && boundaryAt(t, i+6) {
			if j := strings.IndexByte(t[i+6:], '>'); j >= 0 {
				end := i + 6 + j + 1
				if strings.HasPrefix(t[end:], "</image>") {
					end += len("</image>")
				}
				i = end
				continue
			}
		}
		_, sz := py.DecodeRune(t[i:])
		b.WriteString(t[i : i+sz])
		i += sz
	}
	return b.String()
}

// CleanTurn returns a user turn as displayed: request text, attachment names and its source
// (owner, coordinator, slash, answer).
func CleanTurn(text string) (string, []string, string) {
	t := text
	source := "owner"
	var atts []string
	if strings.Contains(t, "You are a dispatched worker") || strings.Contains(t, "=== TASK ===") {
		source = "coordinator" // written by an orchestrating agent, not typed by the owner
		if _, after, ok := strings.Cut(t, "=== TASK ==="); ok {
			t = after
		}
	}
	if m := reCommandName.FindStringSubmatch(t); m != nil {
		s := py.Strip(m[1])
		if a := reCommandArgs.FindStringSubmatch(t); a != nil && py.Strip(a[1]) != "" {
			s += " " + py.Strip(a[1])
		}
		return s, atts, "slash"
	}
	if loc := reAnswer.FindStringSubmatchIndex(t); loc != nil {
		if s, ok := answers(t[loc[2]:loc[3]]); ok {
			rest := py.Strip(t[:loc[0]] + t[loc[1]:])
			if rest != "" {
				s = append(s, rest)
			}
			return strings.Join(s, "\n"), atts, "answer"
		}
	}
	for _, line := range strings.Split(t, "\n") {
		if fm := reFileLine.FindStringSubmatch(py.Strip(line)); fm != nil {
			atts = append(atts, py.Strip(fm[1]))
		}
	}
	if strings.Contains(t, "## My request") {
		if all := reMyRequest.FindAllStringIndex(t, -1); len(all) > 0 {
			t = t[all[len(all)-1][1]:]
		}
	}
	t = reBrowser.ReplaceAllLiteralString(t, "[browser page context]")
	t = removeImageTags(t)
	t = reImageRef.ReplaceAllLiteralString(t, "[image]")
	t = rePasted.ReplaceAllLiteralString(t, "\n")
	t = reReminder.ReplaceAllLiteralString(t, "")
	return py.Strip(t), atts, source
}

// answers formats answers to the agent's questions; ok=false where Python's try block would fail.
func answers(js string) ([]string, bool) {
	v, err := py.Loads(js)
	if err != nil {
		return nil, false
	}
	items := []py.Value{v}
	if v.K == py.Arr {
		items = v.A
	}
	var out []string
	for _, it := range items {
		if it.K != py.Obj {
			return nil, false
		}
		a := it.Get("answer")
		var as string
		if a.K == py.Arr {
			parts := make([]string, len(a.A))
			for i, x := range a.A {
				parts[i] = py.PyStr(x)
			}
			as = strings.Join(parts, ", ")
		} else {
			as = py.PyStr(a)
		}
		q := py.String("")
		if it.Has("question") {
			q = it.Get("question")
		}
		out = append(out, "[answered the agent's question] "+py.PyStr(q)+" → "+as)
	}
	return out, true
}

// ---------------------------------------------------------------- agent replies and instructions from raw logs

type reply struct {
	ts   int64
	text string
}

// agentMessages lists the agent's visible replies in a root thread file. Unlike the reference, a broken line
// is skipped instead of ending the scan (docs/17).
func agentMessages(tool, path string) []reply {
	var out []reply
	_ = logs.ReadRawLines(path, func(line string) {
		var o py.Value
		var err error
		if tool == "codex" {
			if !strings.Contains(line, `"role":"assistant"`) && !strings.Contains(line, `"task_complete"`) {
				return
			}
			if o, err = py.Loads(line); err != nil || o.K != py.Obj {
				return
			}
			p := o.Get("payload")
			if o.Get("metadata").Get("inherited_user_message").Truthy() {
				return
			}
			if isStrV(p.Get("type"), "message") && isStrV(p.Get("role"), "assistant") {
				var parts []string
				for _, c := range listOf(p.Get("content")) {
					if c.K == py.Obj {
						parts = append(parts, textField(c))
					}
				}
				if txt := strings.Join(parts, "\n"); py.Strip(txt) != "" {
					out = append(out, reply{tsOfV(o.Get("timestamp")), txt})
				}
			}
			return
		}
		if !strings.Contains(line, `"type":"assistant"`) {
			return
		}
		if o, err = py.Loads(line); err != nil || o.K != py.Obj {
			return
		}
		var parts []string
		for _, c := range listOf(o.Get("message").Get("content")) {
			if c.K == py.Obj && isStrV(c.Get("type"), "text") {
				parts = append(parts, textField(c))
			}
		}
		if txt := strings.Join(parts, "\n"); py.Strip(txt) != "" {
			out = append(out, reply{tsOfV(o.Get("timestamp")), txt})
		}
	})
	var keep []reply
	for _, r := range out {
		if r.ts != logs.NaT {
			keep = append(keep, r)
		}
	}
	return keep
}

func textField(c py.Value) string {
	if !c.Has("text") {
		return ""
	}
	return py.PyStr(c.Get("text"))
}

func tsOfV(v py.Value) int64 {
	if !v.Truthy() || v.K != py.Str {
		return logs.NaT
	}
	return logs.ParseTime(v.S)
}

// strings yields every string inside a JSON value (dict values and list items, in order).
func stringsIn(v py.Value, fn func(string)) {
	switch v.K {
	case py.Str:
		fn(v.S)
	case py.Obj:
		for _, x := range v.O.Vals {
			stringsIn(x, fn)
		}
	case py.Arr:
		for _, x := range v.A {
			stringsIn(x, fn)
		}
	}
}

// Instruction is an instruction file's text with its kind.
type Instruction struct{ Kind, Text string }

var (
	reClaudeHead = py.Re(`Contents of (\S*?CLAUDE(?:\.local)?\.md)[^\n]*:\s*\n`)
	reAgentsHead = py.Re(`^# AGENTS\.md instructions for \S+`)
	reNextBlock  = py.Re(`^\n\s*Contents of \S*?\.md`)
)

// claudeBlocks is CLAUDE_BLOCK.finditer: a header, then text up to the next "Contents of …md" header,
// "</system-reminder>" or the end (the lookahead RE2 does not have, done by hand).
func claudeBlocks(s string, fn func(path, body string)) {
	pos := 0
	for pos <= len(s) {
		loc := reClaudeHead.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			return
		}
		path := s[pos+loc[2] : pos+loc[3]]
		start := pos + loc[1]
		end := len(s)
		for p := start; p < len(s); p++ {
			if s[p] == '<' && strings.HasPrefix(s[p:], "</system-reminder>") {
				end = p
				break
			}
			if s[p] == '\n' && reNextBlock.MatchString(s[p:]) {
				end = p
				break
			}
		}
		fn(path, s[start:end])
		if end == pos+loc[0] { // empty match guard (cannot happen: the header is not empty)
			end++
		}
		pos = end
	}
}

// InstructionsFor finds instruction blocks recorded in the log files, deduplicated in first-seen order.
func InstructionsFor(tool string, files []string) []Instruction {
	seen := map[[16]byte]bool{}
	var out []Instruction
	add := func(kind, text string) {
		t := py.Strip(text)
		if py.Len(t) < 20 {
			return
		}
		h := md5.Sum([]byte(py.ReplaceSurrogates(t, "?")))
		if seen[h] {
			return
		}
		seen[h] = true
		out = append(out, Instruction{kind, t})
	}
	for _, f := range files {
		_ = logs.ReadRawLines(f, func(line string) {
			if tool == "codex" {
				if !strings.Contains(line, "AGENTS.md instructions") {
					return
				}
				o, err := py.Loads(line)
				if err != nil || o.K != py.Obj {
					return
				}
				p := o.Get("payload")
				if !isStrV(o.Get("type"), "response_item") || !isStrV(p.Get("role"), "user") {
					return
				}
				for _, c := range listOf(p.Get("content")) {
					t := ""
					if c.K == py.Obj {
						t = c.Get("text").StrOr("")
					}
					if strings.HasPrefix(py.LStrip(t), "# AGENTS.md instructions") {
						add("AGENTS.md (project/global instructions)", reAgentsHead.ReplaceAllLiteralString(py.Strip(t), "# AGENTS.md instructions"))
					}
				}
				return
			}
			if !strings.Contains(line, "CLAUDE.md") || !strings.Contains(line, "Contents of") {
				return
			}
			o, err := py.Loads(line)
			if err != nil {
				return
			}
			stringsIn(o, func(s string) {
				claudeBlocks(s, func(path, body string) {
					kind := "CLAUDE.md (project)"
					if strings.Contains(path, "/.claude/") {
						kind = "CLAUDE.md (user global)"
					}
					add(kind, body)
				})
			})
		})
	}
	return out
}

// CurrentFiles are the instruction files read from disk when a log keeps none (Claude Code logs).
var CurrentFiles = []string{"CLAUDE.md", "CLAUDE.local.md", filepath.Join(".claude", "CLAUDE.md"), "AGENTS.md"}

const currentWarning = "read from the current file; may differ from what applied when the session ran"

// CurrentInstructionFiles reads instruction files as they exist now for a working folder: the user's global
// CLAUDE.md, then each folder from the outermost below home down to cwd.
func CurrentInstructionFiles(tool, cwd string) []Instruction {
	var out []Instruction
	home := config.Home()
	// read follows a symbolic link only when it points inside within (the link's project folder, or home for
	// the global file): a CLAUDE.md that links to ~/.aws/credentials is not sent to the judge.
	read := func(p, within string) (string, bool) {
		li, err := os.Lstat(p)
		if err != nil {
			return "", false
		}
		if li.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(p)
			base, err2 := filepath.EvalSymlinks(within)
			if err != nil || err2 != nil {
				return "", false
			}
			if r, err := filepath.Rel(base, target); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
				return "", false
			}
		}
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return "", false
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", false
		}
		return py.DecodeBytes(b), true
	}
	if tool == "claude" {
		if t, ok := read(filepath.Join(config.ClaudeConfigDir(), "CLAUDE.md"), home); ok {
			out = append(out, Instruction{"CLAUDE.md (user global, " + currentWarning + ")", t})
		}
	}
	if cwd == "" {
		return out
	}
	d, err := filepath.Abs(cwd)
	if err != nil {
		return out
	}
	var chain []string
	for {
		r, err := filepath.Rel(home, d)
		if err != nil || r == "." || strings.HasPrefix(r, "..") {
			break
		}
		chain = append(chain, d)
		p := filepath.Dir(d)
		if p == d {
			break
		}
		d = p
	}
	for i := len(chain) - 1; i >= 0; i-- { // outermost first, closest last
		for _, name := range CurrentFiles {
			if t, ok := read(filepath.Join(chain[i], name), chain[i]); ok {
				out = append(out, Instruction{filepath.ToSlash(name) + " (project, " + currentWarning + ")", t})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- renderer

// Renderer builds the judging input of sessions from the pipeline tables and the raw logs.
type Renderer struct {
	t       *pipeline.Tables
	turns   []pipeline.TurnRow // human turns sorted by time (stable)
	threads map[string][]logs.Thread
	calls   map[string][]pipeline.CallRow
	steps   map[string][]pipeline.StepRow
}

// NewRenderer indexes the tables by session.
func NewRenderer(t *pipeline.Tables) *Renderer {
	R := &Renderer{t: t, threads: map[string][]logs.Thread{}, calls: map[string][]pipeline.CallRow{}, steps: map[string][]pipeline.StepRow{}}
	R.turns = append([]pipeline.TurnRow(nil), t.TurnsHuman...)
	sort.SliceStable(R.turns, func(a, b int) bool { return lessTS(R.turns[a].TS, R.turns[b].TS) })
	for _, x := range t.Threads {
		if x.SessionID.OK {
			k := x.Tool + ":" + x.SessionID.S
			R.threads[k] = append(R.threads[k], x)
		}
	}
	for _, c := range t.Calls {
		if c.Key.OK && c.Kind != "wrapper" {
			R.calls[c.Key.S] = append(R.calls[c.Key.S], c)
		}
	}
	for _, s := range t.Steps {
		if s.Key.OK {
			R.steps[s.Key.S] = append(R.steps[s.Key.S], s)
		}
	}
	return R
}

func lessTS(a, b int64) bool {
	if a == logs.NaT {
		return false
	}
	if b == logs.NaT {
		return true
	}
	return a < b
}

// Session is one rendered session.
type Session struct {
	Text         string
	Locs         map[string]string
	LocOrder     []string
	NUser        int
	NInstr       int
	InstrCurrent bool
}

// summarize describes what the agent did between two user requests.
func summarize(calls []pipeline.CallRow, nSteps, spawned int) string {
	if nSteps == 0 && len(calls) == 0 {
		return "no agent activity recorded"
	}
	parts := []string{fmt.Sprintf("%d responses", nSteps)}
	var cmd []pipeline.CallRow
	for _, c := range calls {
		switch c.Kind {
		case "exec", "read", "search", "commit", "mcp":
			cmd = append(cmd, c)
		}
	}
	if len(cmd) > 0 {
		nf := 0
		counts := map[string]int{}
		var fails []bool
		for _, c := range cmd {
			if c.Failed == logs.TriTrue {
				nf++
			}
			if c.Target.OK {
				counts[c.Target.S]++
			}
			fails = append(fails, c.Failed == logs.TriTrue)
		}
		s := fmt.Sprintf("%d commands/lookups", len(cmd))
		if nf > 0 {
			s += fmt.Sprintf(" (%d failed)", nf)
		}
		parts = append(parts, s)
		rep := 0
		for _, n := range counts {
			if n >= 3 {
				rep += n - 2
			}
		}
		if rep > 0 {
			parts = append(parts, fmt.Sprintf("same command repeated %dx", rep))
		}
		run := 0
		for i := 1; i < len(fails); i++ {
			if fails[i-1] && fails[i] {
				run++
			}
		}
		if run > 0 {
			parts = append(parts, fmt.Sprintf("%d consecutive failures", run))
		}
	}
	var names []string
	have := map[string]bool{}
	nEdits := 0
	for _, c := range calls {
		if (c.Kind == "edit" || c.Kind == "edit_record") && c.Path.OK {
			nEdits++
			b := baseName(c.Path.S)
			if !have[b] {
				have[b] = true
				names = append(names, b)
			}
		}
	}
	if nEdits > 0 {
		s := "edited " + strings.Join(names[:min(4, len(names))], ", ")
		if len(names) > 4 {
			s += fmt.Sprintf(" and %d more", len(names)-4)
		}
		parts = append(parts, s)
	} else {
		parts = append(parts, "no files edited")
	}
	if spawned > 0 {
		parts = append(parts, fmt.Sprintf("%d sub-agents", spawned))
	}
	nc := 0
	for _, c := range calls {
		if c.Kind == "commit" {
			nc++
		}
	}
	if nc > 0 {
		parts = append(parts, fmt.Sprintf("%d commit commands", nc))
	}
	return strings.Join(parts, " · ")
}

// baseName is os.path.basename for a path recorded in a log (POSIX on POSIX systems; both separators on Windows).
func baseName(p string) string {
	seps := "/"
	if os.PathSeparator == '\\' {
		seps = `/\`
	}
	if i := strings.LastIndexAny(p, seps); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Session renders one session (key "tool:session_id"). currentInstr: when the log has no instruction
// blocks, read the working folder's instruction files now.
func (R *Renderer) Session(key string, currentInstr bool) Session {
	tool, _, _ := strings.Cut(key, ":")
	TH := R.threads[key]
	var roots []logs.Thread
	for _, x := range TH {
		if x.Source.OK && x.Source.S == "user" {
			roots = append(roots, x)
		}
	}
	if len(roots) == 0 && len(TH) > 0 {
		s := append([]logs.Thread(nil), TH...)
		sort.SliceStable(s, func(a, b int) bool { return lessTS(s[a].Start, s[b].Start) })
		roots = s[:1]
	}
	var replies []reply
	var files []string
	for _, x := range roots {
		replies = append(replies, agentMessages(tool, x.File)...)
		files = append(files, x.File)
	}
	sort.SliceStable(replies, func(a, b int) bool {
		if replies[a].ts != replies[b].ts {
			return replies[a].ts < replies[b].ts
		}
		return replies[a].text < replies[b].text
	})
	instr := InstructionsFor(tool, files)
	instrCurrent := false
	if currentInstr && len(instr) == 0 {
		cwd := ""
		for _, x := range roots {
			if x.Cwd.OK {
				cwd = x.Cwd.S
				break
			}
		}
		instr = CurrentInstructionFiles(tool, cwd)
		instrCurrent = len(instr) > 0
	}
	S := Session{Locs: map[string]string{}}
	setLoc := func(k, v string) { S.Locs[k] = v; S.LocOrder = append(S.LocOrder, k) }
	var parts []string
	if len(instr) > 0 {
		parts = append(parts, "## Instructions that applied to this session (as found in the log)")
		total := 0
		for i, in := range instr {
			t := Clip(in.Text, min(MaxInstr, max(MaxInstrTotal-total, 500)))
			total += py.Len(t)
			setLoc(fmt.Sprintf("I%d", i+1), t)
			parts = append(parts, fmt.Sprintf("[I%d] %s\n%s", i+1, in.Kind, t))
		}
	} else {
		parts = append(parts, "## Instructions that applied to this session\n(none found in the log)")
	}
	parts = append(parts, "\n## Session transcript (U = user request, A = what the agent did after that request)")
	var T []pipeline.TurnRow
	for _, u := range R.turns {
		if u.Key.OK && u.Key.S == key && (u.Kind == "human" || u.Kind == "slash" || u.Kind == "interrupt") {
			T = append(T, u)
		}
	}
	C, St := R.calls[key], R.steps[key]
	n := 0
	for i, t := range T {
		lo := t.TS
		hi := int64(math.MaxInt64)
		if i+1 < len(T) {
			hi = T[i+1].TS
		}
		if t.Kind == "interrupt" {
			parts = append(parts, "(the user interrupted the agent)")
			continue
		}
		n++
		text, atts, src := CleanTurn(t.Text)
		u := Clip(text, MaxU)
		if len(atts) > 0 {
			u += "\n(attachments: " + strings.Join(atts[:min(6, len(atts))], ", ") + ")"
		}
		setLoc(fmt.Sprintf("U%d", n), u)
		tag := map[string]string{"slash": " [slash command]", "answer": " [answer to the agent's question]"}[src]
		parts = append(parts, fmt.Sprintf("[U%d]%s %s", n, tag, u))
		inSeg := func(ts int64) bool { return lo != logs.NaT && ts != logs.NaT && ts >= lo && ts < hi }
		var segC []pipeline.CallRow
		for _, c := range C {
			if inSeg(c.TS) {
				segC = append(segC, c)
			}
		}
		nS := 0
		for _, s := range St {
			if inSeg(s.TS) {
				nS++
			}
		}
		spawned := 0
		for _, x := range TH {
			if inSeg(x.Start) && !(x.Source.OK && x.Source.S == "user") {
				spawned++
			}
		}
		var last string
		have := false
		for _, r := range replies {
			if inSeg(r.ts) {
				last, have = r.text, true
			}
		}
		a := "activity: " + summarize(segC, nS, spawned)
		if have {
			a += "\nlast reply: " + Clip(last, MaxA)
		} else {
			a += "\nlast reply: (none recorded)"
		}
		setLoc(fmt.Sprintf("A%d", n), a)
		parts = append(parts, fmt.Sprintf("[A%d] %s", n, a))
	}
	S.Text = strings.Join(parts, "\n\n")
	S.NUser, S.NInstr, S.InstrCurrent = n, len(instr), instrCurrent
	return S
}
