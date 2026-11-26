package logs

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// codexTypes are record types this version knows (token_usage_record repeats token_count's usage in
// newer CLIs and is not needed while token_count is present; see UsageOnlyNew).
var codexTypes = map[string]bool{"session_meta": true, "turn_context": true, "response_item": true, "event_msg": true,
	"compacted": true, "world_state": true, "token_usage_record": true, "inter_agent_communication_metadata": true,
	"retained_context": true}

// classifyCmd maps a Codex parsed_cmd list to (kind, path).
func classifyCmd(parsed py.Value) (string, NS) {
	var kinds []py.Value
	var first py.Value
	if parsed.K == py.Arr {
		for i, p := range parsed.A {
			if p.K == py.Obj {
				kinds = append(kinds, p.Get("type"))
			}
			if i == 0 {
				first = p
			}
		}
	}
	if len(kinds) > 0 {
		all := func(ok func(py.Value) bool) bool {
			for _, k := range kinds {
				if !ok(k) {
					return false
				}
			}
			return true
		}
		if all(func(k py.Value) bool { return isStr(k, "read") }) {
			return "read", FromValue(first.Get("path").Or(first.Get("name")))
		}
		if all(func(k py.Value) bool { return inStrs(k, "search", "list_files") }) {
			return "search", NS{}
		}
	}
	return "exec", NS{}
}

func joinCmd(cmd py.Value) string {
	if cmd.K == py.Arr {
		parts := make([]string, len(cmd.A))
		for i, x := range cmd.A {
			parts[i] = py.PyStr(x)
		}
		return strings.Join(parts, " ")
	}
	return py.PyStr(cmd)
}

// ParseCodex reads one Codex rollout file in two streaming passes, so a rollout of hundreds of MB is never held
// decoded in memory: the first pass collects what the second needs to know in advance (whether item_completed
// records exist, the highest ordinal, the history start of the file's own session_meta, the usage record kinds,
// the record types), the second reads the records in order. The second pass reads exactly the bytes the first
// one did: a session being written can grow in between. A file replaced in between (another checksum) is read
// again once.
func ParseCodex(path string) (*File, error) {
	f, err := parseCodex(path)
	if errors.Is(err, errChanged) {
		f, err = parseCodex(path)
	}
	return f, err
}

// betweenPasses runs between the two passes of parseCodex (tests: a file that grows or is replaced meanwhile).
var betweenPasses func(path string)

// errChanged is a file whose content changed between the two passes (not only grown at its end).
var errChanged = errors.New("the log file changed while it was read; run again")

func parseCodex(path string) (*File, error) {
	// both passes read the same open file: a rollout moved, archived or replaced by name in between does not
	// change what the second pass reads
	fh, err := os.Open(path) // read-only; Go opens with read/write share flags on Windows
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	// the id of the file's own session_meta: the thread id in its name, else its first session_meta's id (read in
	// the first pass); the reference's name slice when there is neither
	fid := codexThreadID(path)
	metaSeen := false
	keyID := fileID(path)
	b := &builder{tool: "codex", path: path}
	var st Stats
	haveItems := false
	maxOrd := int64(-1)
	nUsage, nTokenCount := 0, 0
	// subagent_history_start_ordinal of the last session_meta of this file's own thread, as a number (nothing
	// from the first pass is kept but numbers and flags)
	ownHist, ownHistOK := 0.0, false
	n, sum, err := scanUpTo(fh, path, -1, true, func(line []byte) {
		handle(line, &st, func(o py.Value) {
			if o.K != py.Obj {
				st.NotObject++ // a valid JSON line that is not an object is ignored
				return
			}
			p := o.Get("payload")
			if isStr(o.Get("type"), "event_msg") && p.K == py.Obj && isStr(p.Get("type"), "item_completed") && isStr(p.Get("item").Get("type"), "CommandExecution") {
				haveItems = true
			}
			if ord := o.Get("ordinal"); ord.K == py.Int && ord.Big == "" && ord.I > maxOrd {
				maxOrd = ord.I
			}
			if t := o.Get("type"); t.K == py.Str && !codexTypes[t.S] {
				st.unknown("codex:" + t.S)
			}
			if isStr(o.Get("type"), "token_usage_record") {
				nUsage++
			} else if isStr(p.Get("type"), "token_count") {
				nTokenCount++
			}
			if isStr(o.Get("type"), "session_meta") && p.K == py.Obj && !metaSeen {
				metaSeen = true
				if fid == "" {
					fid = fileID(path) // no id to go by: the reference's name slice, as before
					if id := p.Get("id"); id.K == py.Str && id.S != "" {
						fid = id.S
					}
				}
			}
			if isStr(o.Get("type"), "session_meta") && p.K == py.Obj && isStr(p.Get("id"), fid) {
				h := p.Get("subagent_history_start_ordinal")
				ownHist, ownHistOK = h.Num()
				ownHistOK = ownHistOK && h.K != py.Bool
			}
		})
	})
	if err != nil {
		return nil, err
	}
	if fid == "" {
		fid = fileID(path)
	}
	if nUsage > 0 && nTokenCount == 0 {
		st.UsageOnlyNew++ // usage only in the newer record kind: this file's responses are not counted
	}
	ev := &codexEv{}
	if ownHistOK && ownHist <= float64(maxOrd) {
		ev.hist, ev.hasHist = int64(ownHist), true
		if float64(ev.hist) != ownHist { // fractional start ordinal: compare as float, as the main pass does
			ev.hist++
		}
	}
	histStart, hasHist := int64(0), false
	var lastTotal, curModel py.Value
	compaction := false
	callByID := map[string]int{}
	su := newSetup()
	process := func(o py.Value) {
		typ := o.Get("type")
		p := o.Get("payload")
		if p.K != py.Obj {
			p = py.Value{}
		}
		pt := p.Get("type")
		ts := o.Get("timestamp")
		if isStr(typ, "session_meta") {
			// the main pass keeps its rule (the name slice, so in practice the first session_meta, which is the
			// file's own): only the cache events use the file's own id
			if id := p.Get("id"); (id.K == py.Str && id.S == keyID) || b.meta.threadID.IsNull() {
				b.meta.threadID = id
				b.meta.sessionID = p.Get("session_id").Or(id)
				b.meta.parentID = p.Get("parent_thread_id").Or(p.Get("forked_from_id"))
				src := p.Get("thread_source")
				if !src.Truthy() {
					if p.Get("source").K == py.Obj {
						src = py.String("subagent")
					} else {
						src = py.String("user")
					}
				}
				b.meta.source = src
				b.meta.cwd = p.Get("cwd")
				b.meta.version = p.Get("cli_version")
				b.meta.originator = p.Get("originator")
				if ls := p.Get("source"); ls.K == py.Str {
					b.meta.launchSource = ls
				} else {
					b.meta.launchSource = py.Value{}
				}
				h := p.Get("subagent_history_start_ordinal")
				hasHist = false
				if f, ok := h.Num(); ok && h.K != py.Bool {
					if f <= float64(maxOrd) {
						histStart, hasHist = int64(f), true
						if float64(int64(f)) != f { // fractional start ordinal: compare as float
							histStart = int64(f) + 1
						}
					}
				}
			}
			return
		}
		// skip history inherited from the parent thread
		if ord := o.Get("ordinal"); hasHist && ord.K == py.Int && ord.Big == "" && ord.I < histStart {
			return
		}
		if o.Get("metadata").Get("inherited_user_message").Truthy() {
			return
		}
		b.ts(ts)
		switch {
		case isStr(typ, "world_state"):
			su.codexWorldState(p, tsOf(ts))
			su.codexHostSkills(p, tsOf(ts))
		case isStr(typ, "response_item") && isStr(pt, "message") && isStr(p.Get("role"), "developer"):
			codexDevList(su, typ, pt, p, tsOf(ts))
		case isStr(typ, "response_item") && isStr(pt, "local_shell_call"):
			su.mentions(argText(p.Get("action")), runDirs(p.Get("action"))...)
		case isStr(typ, "turn_context"):
			b.meta.model = b.meta.model.Or(p.Get("model"))
			if p.Get("model").Truthy() {
				curModel = p.Get("model")
			}
			if p.Get("cwd").Truthy() {
				b.meta.cwd = b.meta.cwd.Or(p.Get("cwd"))
			}
		case isStr(typ, "compacted") || (isStr(pt, "item_completed") && isStr(p.Get("item").Get("type"), "ContextCompaction")):
			b.meta.nCompactions++
			compaction = true
		case isStr(pt, "token_count"):
			info := p.Get("info")
			tot, lu := info.Get("total_token_usage"), info.Get("last_token_usage")
			if !lu.Truthy() || Equal(tot, lastTotal) {
				return
			}
			lastTotal = tot
			inp, cached := orZero(lu.Get("input_tokens")), orZero(lu.Get("cached_input_tokens"))
			cw := orZero(lu.Get("cache_write_input_tokens"))
			unc := inp - cached - cw
			if unc < 0 {
				unc = 0
			}
			b.addStep(ts, unc, cached, cw, orZero(lu.Get("output_tokens")), orZero(lu.Get("reasoning_output_tokens")), compaction)
			b.steps[len(b.steps)-1].Model = FromValue(curModel.Or(b.meta.model))
			compaction = false
		case isStr(pt, "task_started"):
			b.trigger = p.Get("turn_attribution").Get("turn_trigger").Or(py.String("unattributed"))
		case isStr(pt, "turn_aborted"):
			b.meta.nAborted++
			b.turns = append(b.turns, Turn{TS: tsOf(ts), Kind: "interrupt"})
		case isStr(pt, "task_complete"):
			if p.Get("last_agent_message").Truthy() {
				b.meta.hasFinalMsg = true
			}
		case isStr(typ, "response_item") && isStr(pt, "message") && isStr(p.Get("role"), "user"):
			for _, im := range codexImages(p.Get("content"), tsOf(ts), b.lastStep()) {
				if n := len(b.pending); n > 0 { // after a call whose response's usage is not logged yet: that response comes first
					if b.imageCall == nil {
						b.imageCall = map[int]int{}
					}
					b.imageCall[len(b.images)] = b.pending[n-1]
				}
				b.images = append(b.images, im)
			}
			kinds := p.Get("internal_chat_message_metadata_passthrough").Get("content_item_kinds")
			var texts []string
			if c := p.Get("content"); c.K == py.Arr {
				for _, x := range c.A {
					if x.K == py.Obj && inStrs(x.Get("type"), "input_text", "text") {
						texts = append(texts, textOf(x.Get("text")))
					}
				}
			}
			text := joinNonEmpty(texts)
			if strings.Contains(text, "<skill>") {
				su.codexPick(text)
			}
			var human bool
			if !kinds.IsNull() {
				human = pyIn("user.text", kinds)
				if pyIn("user.heartbeat", kinds) {
					b.trigger = py.String("automation_heartbeat_scheduled")
				}
				if human {
					var keep []string
					if kinds.K == py.Arr {
						for i, t := range texts {
							if i < len(kinds.A) && isStr(kinds.A[i], "user.text") {
								keep = append(keep, t)
							}
						}
					}
					if s := strings.Join(keep, "\n"); s != "" {
						text = s
					}
				}
			} else {
				s := py.LStrip(text)
				human = s != "" && !strings.HasPrefix(s, "<") && !strings.HasPrefix(s, "# AGENTS.md")
			}
			if strings.HasPrefix(py.LStrip(text), "<heartbeat") {
				b.trigger = py.String("automation_heartbeat_scheduled")
			}
			if human {
				b.turns = append(b.turns, Turn{TS: tsOf(ts), Kind: "human", Text: text})
			}
		case isStr(typ, "response_item") && inStrs(pt, "function_call", "custom_tool_call", "tool_search_call"):
			name := p.Get("name").Or(pt)
			var argsV py.Value
			if isStr(pt, "function_call") {
				argsV = p.Get("arguments")
			} else {
				argsV = p.Get("input")
			}
			var args string
			if argsV.K == py.Str {
				args = argsV.S
			} else {
				args = py.Dumps(argsV.Or(py.String("")))
			}
			// every call's input, whatever the tool and whether item_completed records exist, decoded as written
			if argsV.K == py.Str {
				su.mentions(argTextRaw(argsV.S), runDirs(argsV)...)
			} else {
				su.mentions(argText(argsV), runDirs(argsV)...)
			}
			kind, path_, target := "tool", NS{}, FromValue(name)
			switch {
			case isStr(name, "apply_patch") || (isStr(name, "exec") && strings.Contains(py.Head(args, 200), "*** Begin Patch")):
				kind = "edit"
				if m := patchFileRE.FindStringSubmatch(args); m != nil {
					path_ = Some(strings.Clone(py.Strip(m[1]))) // not the whole patch kept alive
				}
				target = Some("edit:" + path_.S)
			case inStrs(name, "exec_command", "shell", "local_shell") && !haveItems:
				var cmdV py.Value
				if j, err := py.Loads(args); err == nil && j.K == py.Obj {
					cmdV = j.Get("cmd").Or(j.Get("command"))
				} else {
					cmdV = py.String(args)
				}
				cmd := joinCmd(cmdV)
				kind, target = "exec", Some("cmd:"+H(cmd))
				if m := codexReadRE.FindStringSubmatch(cmd); m != nil {
					kind, path_ = "read", Some(strings.Clone(m[1]))
				}
				if strings.Contains(cmd, "git commit") {
					kind = "commit"
				}
			case inStrs(name, "exec", "exec_command", "shell"):
				kind = "wrapper" // code-mode wrapper; the real commands come from item_completed
			case inStrs(name, "spawn_agent", "send_message", "wait_agent", "followup_task", "wait"):
				kind = "agent"
			}
			ci := b.addCall(Call{TS: tsOf(ts), CallID: FromValue(p.Get("call_id")), Name: FromValue(name), Kind: kind, Target: target,
				Path: path_, InChars: py.Len(args)}, -1)
			callByID[key(p.Get("call_id"))] = ci
		case isStr(typ, "response_item") && inStrs(pt, "function_call_output", "custom_tool_call_output", "tool_search_output"):
			ci, ok := callByID[key(p.Get("call_id"))]
			for _, im := range codexImages(p.Get("output"), tsOf(ts), b.lastStep()) {
				if ok { // the response that issued the call reads the output next time: its step is set when its usage arrives
					if b.imageCall == nil {
						b.imageCall = map[int]int{}
					}
					b.imageCall[len(b.images)] = ci
				}
				b.images = append(b.images, im)
			}
			text := outText(p.Get("output"))
			if ok {
				c := &b.calls[ci]
				c.OutChars = py.Len(text)
				if m := exitRE.FindStringSubmatch(py.Head(text, 600)); m != nil {
					_, nonzero, _ := py.ParseInt(m[1])
					c.Failed = TriOf(nonzero)
				} else if strings.HasPrefix(text, "Script failed") || strings.HasPrefix(text, "aborted") {
					c.Failed = TriTrue
				} else if c.Failed == TriNone && c.Kind != "wrapper" {
					c.Failed = TriFalse
				}
				if c.Kind == "commit" {
					b.addCommits(text, ts, FromValue(b.meta.cwd))
				}
			}
		case isStr(pt, "item_completed"):
			it := p.Get("item")
			t := it.Get("type")
			step := b.lastStep()
			switch {
			case isStr(t, "CommandExecution"):
				cmd := joinCmd(it.Get("command"))
				su.mentions(argText(it.Get("command")), py.PyStr(it.Get("cwd")))
				parsed := it.Get("parsed_cmd")
				if !parsed.Truthy() {
					parsed = py.Value{K: py.Arr}
				}
				kind, path_ := classifyCmd(parsed)
				if strings.Contains(cmd, "git commit") {
					kind = "commit"
				}
				outp := it.Get("aggregated_output").Or(py.String(""))
				outs := py.PyStr(outp)
				prefix := "cmd:"
				if kind == "read" {
					prefix = "read:"
				}
				ec := it.Get("exit_code")
				b.addCall(Call{TS: tsOf(ts), CallID: FromValue(it.Get("id")), Name: Some("command"), Kind: kind, Target: Some(prefix + H(cmd)),
					Path: path_, Failed: TriOf(!(ec.IsNull() || ec.NumEq0())), OutChars: pyLen(outp), InChars: py.Len(cmd),
					Cwd: FromValue(it.Get("cwd")), HasCwd: true}, step)
				if kind == "commit" {
					cwd := FromValue(it.Get("cwd").Or(b.meta.cwd))
					b.addCommits(outs, ts, cwd)
				}
			case isStr(t, "FileChange"):
				for _, fp := range iterKeys(it.Get("changes")) {
					b.addCall(Call{TS: tsOf(ts), CallID: FromValue(it.Get("id")), Name: Some("file_change"), Kind: "edit_record",
						Target: Some("edit:" + fp), Path: Some(fp), Failed: TriOf(!isStr(it.Get("status"), "completed")), InChars: -1}, step)
				}
			case isStr(t, "McpToolCall"):
				su.mentions(argText(it.Get("arguments")))
				res := it.Get("result").Or(py.Value{K: py.Obj, O: &py.Object{}})
				c := Call{TS: tsOf(ts), CallID: FromValue(it.Get("id")), Name: Some("mcp:" + py.PyStr(it.Get("server")) + "." + py.PyStr(it.Get("tool"))),
					Kind: "mcp", Target: Some("mcp:" + H(py.DumpsOpt(it.Get("arguments"), py.DumpOpts{EnsureASCII: true, SortKeys: true}))), InChars: -1}
				if res.K == py.Obj {
					c.Failed = TriOf(res.Get("isError").Truthy())
					c.OutChars = py.Len(outText(res.Get("content")))
				}
				b.addCall(c, step)
			case isStr(t, "AgentMessage"):
				b.meta.hasFinalMsg = true
			}
		}
	}
	if betweenPasses != nil {
		betweenPasses(path)
	}
	if _, err := fh.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var again Stats // the first pass counted the unusable lines
	n2, sum2, err := scanUpTo(fh, path, n, true, func(line []byte) {
		handleOwned(line, &again, func(o py.Value) {
			if o.K == py.Obj {
				process(o)
				ev.add(o)
			}
		})
	})
	if err != nil || n2 != n || sum2 != sum { // rewritten in place between the passes (it read fine the first time)
		return nil, errChanged
	}
	f := b.finish()
	for i, ci := range b.imageCall {
		if f.Calls[ci].Step >= 0 {
			f.Images[i].Step = f.Calls[ci].Step
		}
	}
	f.Events = ev.out
	f.Stats = st
	su.codexResolve()
	su.cx = nil
	if isStr(b.meta.source, "subagent") { // a sub-agent thread's own list is not counted (no session twice): only its skill use
		su = &Setup{SkillsAt: NaT, InstrAt: NaT, AgentsAt: NaT, SkillUses: su.SkillUses,
			SkillMentions: su.SkillMentions}
	}
	if !su.Empty() {
		f.Setup = su
	}
	return f, nil
}

// codexDevList reads the skill list a developer message holds, if any.
func codexDevList(su *Setup, typ, pt, p py.Value, ts int64) {
	if !isStr(typ, "response_item") || !isStr(pt, "message") || !isStr(p.Get("role"), "developer") {
		return
	}
	if c := p.Get("content"); c.K == py.Arr {
		for _, x := range c.A {
			if t := textOf(x.Get("text")); strings.Contains(t, "<skills_instructions>") {
				su.codexSkillList(t, ts)
			}
		}
	}
}

func joinNonEmpty(xs []string) string {
	var keep []string
	for _, x := range xs {
		if x != "" {
			keep = append(keep, x)
		}
	}
	return strings.Join(keep, "\n")
}

// pyLen is len() of a str, list or dict value.
func pyLen(v py.Value) int {
	switch v.K {
	case py.Str:
		return py.Len(v.S)
	case py.Arr:
		return len(v.A)
	case py.Obj:
		return len(v.O.Keys)
	}
	return 0
}

// iterKeys is "for x in v" for a dict (keys), list (string items) or str (characters).
func iterKeys(v py.Value) []string {
	switch v.K {
	case py.Obj:
		return v.O.Keys
	case py.Arr:
		var out []string
		for _, x := range v.A {
			out = append(out, py.PyStr(x))
		}
		return out
	case py.Str:
		var out []string
		for _, r := range py.Runes(v.S) {
			out = append(out, string(py.AppendRune(nil, r)))
		}
		return out
	}
	return nil
}

// codexEv lists what can precede a cache rewrite in one Codex file (own records only), as
// cache_causes.codex_events does, one record at a time: hist is the history start of the file's own session_meta
// (from the first pass).
type codexEv struct {
	hist                  int64
	hasHist               bool
	lastModel, lastEffort py.Value
	out                   []Event
}

func (e *codexEv) add(o py.Value) {
	if ord := o.Get("ordinal"); e.hasHist && ord.K == py.Int && ord.Big == "" && ord.I < e.hist {
		return
	}
	if o.Get("metadata").Get("inherited_user_message").Truthy() {
		return
	}
	typ := o.Get("type")
	p := o.Get("payload")
	if p.K != py.Obj {
		p = py.Value{}
	}
	pt := p.Get("type")
	ts := tsOf(o.Get("timestamp"))
	add := func(k string) { e.out = append(e.out, Event{TS: ts, Kind: k}) }
	switch {
	case isStr(typ, "turn_context"):
		model := p.Get("model")
		effort := p.Get("effort").Or(p.Get("collaboration_mode").Get("settings").Get("reasoning_effort"))
		if !e.lastModel.IsNull() && !Equal(model, e.lastModel) {
			add("model_change")
		}
		if !e.lastEffort.IsNull() && !Equal(effort, e.lastEffort) {
			add("effort_change")
		}
		e.lastModel, e.lastEffort = model, effort
	case isStr(pt, "configuration_update"):
		add("config_update")
	case inStrs(pt, "additional_tools", "tool_search_output"):
		add("tools_change")
	case isStr(typ, "world_state") && p.Get("full").Truthy():
		add("world_state_full")
	case isStr(pt, "thread_settings_applied"):
		add("settings_applied")
	case isStr(pt, "message") && isStr(p.Get("role"), "developer"):
		add("developer_msg")
	case isStr(pt, "task_started"):
		add("new_user_turn")
	case isStr(pt, "item_completed") && isStr(p.Get("item").Get("type"), "SubAgentActivity"):
		add("subagent_activity")
	}
}

// textOf is c.get('text', ”) used as text: a string, "" when missing or falsy, else its str().
func textOf(v py.Value) string {
	if v.K == py.Str {
		return v.S
	}
	if !v.Truthy() {
		return ""
	}
	return py.PyStr(v)
}
