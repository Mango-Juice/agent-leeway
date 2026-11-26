package logs

// The Codex reader as it was before it read rollouts in two streaming passes (v0.1.13), kept unchanged for the
// parity tests: every fixture, and the real logs through a local tool, must parse to the same File.

import (
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// parseCodexLegacy is ParseCodex of v0.1.13: every line decoded and held, then four passes.
func parseCodexLegacy(path string) (*File, error) {
	fid := fileID(path)
	b := &builder{tool: "codex", path: path}
	var st Stats
	var lines []py.Value
	err := ReadLines(path, &st, func(v py.Value) {
		if v.K != py.Obj {
			st.NotObject++ // a valid JSON line that is not an object is ignored
			return
		}
		lines = append(lines, v)
	})
	if err != nil {
		return nil, err
	}
	haveItems := false
	maxOrd := int64(-1)
	for _, o := range lines {
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
	}
	nUsage, nTokenCount := 0, 0
	for _, o := range lines {
		if isStr(o.Get("type"), "token_usage_record") {
			nUsage++
		} else if isStr(o.Get("payload").Get("type"), "token_count") {
			nTokenCount++
		}
	}
	if nUsage > 0 && nTokenCount == 0 {
		st.UsageOnlyNew++ // usage only in the newer record kind: this file's responses are not counted
	}
	histStart, hasHist := int64(0), false
	var lastTotal, curModel py.Value
	compaction := false
	callByID := map[string]int{}
	su := newSetup()
	for _, o := range lines {
		typ := o.Get("type")
		p := o.Get("payload")
		if p.K != py.Obj {
			p = py.Value{}
		}
		pt := p.Get("type")
		ts := o.Get("timestamp")
		if isStr(typ, "session_meta") {
			if id := p.Get("id"); (id.K == py.Str && id.S == fid) || b.meta.threadID.IsNull() {
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
			continue
		}
		// skip history inherited from the parent thread
		if ord := o.Get("ordinal"); hasHist && ord.K == py.Int && ord.Big == "" && ord.I < histStart {
			continue
		}
		if o.Get("metadata").Get("inherited_user_message").Truthy() {
			continue
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
				continue
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
					path_ = Some(py.Strip(m[1]))
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
					kind, path_ = "read", Some(m[1])
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
	f := b.finish()
	for i, ci := range b.imageCall {
		if f.Calls[ci].Step >= 0 {
			f.Images[i].Step = f.Calls[ci].Step
		}
	}
	f.Events = codexEventsLegacy(lines, fid, maxOrd)
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

// codexEvents lists what can precede a cache rewrite in one Codex file (own records only), as
// cache_causes.codex_events does.
func codexEventsLegacy(lines []py.Value, fid string, maxOrd int64) []Event {
	var out []Event
	hist, hasHist := int64(0), false
	for _, o := range lines {
		p := o.Get("payload")
		if isStr(o.Get("type"), "session_meta") && p.K == py.Obj && isStr(p.Get("id"), fid) {
			h := p.Get("subagent_history_start_ordinal")
			hasHist = false
			if f, ok := h.Num(); ok && h.K != py.Bool && f <= float64(maxOrd) {
				hist, hasHist = int64(f), true
			}
		}
	}
	var lastModel, lastEffort py.Value
	for _, o := range lines {
		if ord := o.Get("ordinal"); hasHist && ord.K == py.Int && ord.Big == "" && ord.I < hist {
			continue
		}
		if o.Get("metadata").Get("inherited_user_message").Truthy() {
			continue
		}
		typ := o.Get("type")
		p := o.Get("payload")
		if p.K != py.Obj {
			p = py.Value{}
		}
		pt := p.Get("type")
		ts := tsOf(o.Get("timestamp"))
		add := func(k string) { out = append(out, Event{TS: ts, Kind: k}) }
		switch {
		case isStr(typ, "turn_context"):
			model := p.Get("model")
			effort := p.Get("effort").Or(p.Get("collaboration_mode").Get("settings").Get("reasoning_effort"))
			if !lastModel.IsNull() && !Equal(model, lastModel) {
				add("model_change")
			}
			if !lastEffort.IsNull() && !Equal(effort, lastEffort) {
				add("effort_change")
			}
			lastModel, lastEffort = model, effort
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
	return out
}
