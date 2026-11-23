package logs

import (
	"path/filepath"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var (
	claudeReadTools   = map[string]bool{"Read": true, "NotebookRead": true}
	claudeEditTools   = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}
	claudeSearchTools = map[string]bool{"Grep": true, "Glob": true, "LS": true, "ToolSearch": true, "WebSearch": true, "WebFetch": true}
	claudeTypes       = map[string]bool{"user": true, "assistant": true, "system": true, "summary": true, "cost-state": true,
		"api-request-shape": true, "file-history-snapshot": true, "queue-operation": true, "attachment": true, "progress": true,
		"custom-title": true, "agent-name": true, "tag": true, "pr-link": true, "last-prompt": true, "mode": true,
		"session-owner": true, "tools-used": true, "permission-mode": true, "ai-title": true, "artifact-autoreact-ledger": true,
		"result": true, "continued-in": true, "atis-latch": true, "api-request-blob": true, "relocated": true, "started": true,
		"api-request": true, "bridge-session": true, "file-history-delta": true, "launched": true, "artifact-comment-monitor": true}
)

// IsSubagentPath reports whether a Claude transcript is a sub-agent thread (…/subagents/…).
func IsSubagentPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/subagents/")
}

func strName(v py.Value) string {
	if v.K == py.Str {
		return v.S
	}
	return ""
}

// ParseClaude reads one Claude Code transcript.
func ParseClaude(path string) (*File, error) {
	b := &builder{tool: "claude", path: path}
	sub := IsSubagentPath(path)
	var st Stats
	seenMsg := map[string]int{}
	callByID := map[string]int{}
	compaction := false
	var events []Event
	var lastModel, lastShape py.Value
	su := newSetup()
	err := ReadLines(path, &st, func(o py.Value) {
		if o.K != py.Obj {
			st.NotObject++
			return
		}
		typ := o.Get("type")
		ts := o.Get("timestamp")
		if typ.K == py.Str && !claudeTypes[typ.S] {
			st.unknown("claude:" + typ.S)
		}
		// events that can precede a cache rewrite (cache_causes.claude_events: every line with a timestamp)
		if ts.Truthy() {
			et := tsOf(ts)
			switch {
			case isStr(typ, "api-request-shape"):
				h := o.Get("shapeHash")
				if !lastShape.IsNull() && !Equal(h, lastShape) {
					events = append(events, Event{et, "shape_change"}) // system prompt / tool list changed
				}
				lastShape = h
			case isStr(typ, "assistant"):
				mdl := o.Get("message").Get("model")
				if mdl.Truthy() && !isStr(mdl, "<synthetic>") {
					if !lastModel.IsNull() && !Equal(mdl, lastModel) {
						events = append(events, Event{et, "model_change"})
					}
					lastModel = mdl
				}
			case isStr(typ, "user") && isStr(o.Get("origin").Get("kind"), "human"):
				events = append(events, Event{et, "new_user_turn"})
			}
		}
		if sid := o.Get("sessionId"); sid.Truthy() && b.meta.sessionID.IsNull() {
			b.meta.sessionID = sid
		}
		if isStr(typ, "cost-state") {
			b.meta.costUSD, b.meta.linesAdded = o.Get("totalCostUSD"), o.Get("totalLinesAdded")
			return
		}
		if isStr(typ, "system") && isStr(o.Get("subtype"), "compact_boundary") {
			b.meta.nCompactions++
			compaction = true
			return
		}
		if isStr(typ, "attachment") {
			su.claudeAttachment(o.Get("attachment"), tsOf(ts))
			return
		}
		if !inStrs(typ, "user", "assistant") {
			return
		}
		if o.Get("cwd").Truthy() && !b.meta.cwd.Truthy() {
			b.meta.cwd = o.Get("cwd")
		}
		b.meta.version = b.meta.version.Or(o.Get("version"))
		b.meta.entrypoint = b.meta.entrypoint.Or(o.Get("entrypoint"))
		b.ts(ts)
		m := o.Get("message")
		if isStr(typ, "assistant") {
			mid := m.Get("id").Or(o.Get("uuid"))
			b.meta.model = b.meta.model.Or(m.Get("model"))
			mk := key(mid)
			if _, seen := seenMsg[mk]; !seen && !isStr(m.Get("model"), "<synthetic>") {
				u := m.Get("usage")
				b.addStep(ts, orZero(u.Get("input_tokens")), orZero(u.Get("cache_read_input_tokens")), orZero(u.Get("cache_creation_input_tokens")),
					orZero(u.Get("output_tokens")), orZero(u.Get("output_tokens_details").Get("thinking_tokens")), compaction)
				compaction = false
				st := &b.steps[len(b.steps)-1]
				st.Model = FromValue(m.Get("model"))
				cc := u.Get("cache_creation")
				st.CW5m, st.CW1h = orZero(cc.Get("ephemeral_5m_input_tokens")), orZero(cc.Get("ephemeral_1h_input_tokens"))
				seenMsg[mk] = len(b.steps) - 1
			}
			step, ok := seenMsg[mk]
			if !ok {
				step = -1
			}
			content := m.Get("content")
			if content.K != py.Arr {
				return
			}
			for _, c := range content.A {
				if c.K != py.Obj {
					continue
				}
				if isStr(c.Get("type"), "text") && c.Get("text").Truthy() {
					b.meta.hasFinalMsg = true
				}
				if !isStr(c.Get("type"), "tool_use") {
					continue
				}
				nameV := c.Get("name")
				name := strName(nameV)
				inp := c.Get("input").Or(py.Value{K: py.Obj, O: &py.Object{}})
				kind, target, path_ := "tool", FromValue(nameV), NS{}
				switch {
				case nameV.K == py.Str && claudeReadTools[name]:
					pv := inp.Get("file_path").Or(inp.Get("notebook_path"))
					path_ = FromValue(pv)
					kind, target = "read", Some("read:"+py.PyStr(pv)+":"+py.PyStr(inp.Get("offset"))+":"+py.PyStr(inp.Get("limit")))
				case nameV.K == py.Str && claudeEditTools[name]:
					pv := inp.Get("file_path").Or(inp.Get("notebook_path"))
					path_ = FromValue(pv)
					kind, target = "edit", Some("edit:"+py.PyStr(pv))
				case name == "Bash" && nameV.K == py.Str:
					cmdV := inp.Get("command")
					if !inp.Has("command") {
						cmdV = py.String("")
					}
					cmd := py.PyStr(cmdV)
					kind = "exec"
					if strings.Contains(cmd, "git commit") {
						kind = "commit"
					}
					target = Some("cmd:" + H(cmd))
					if mm := claudeRead.FindStringSubmatch(cmd); mm != nil {
						kind, path_, target = "read", Some(mm[1]), Some("read:"+H(cmd))
					}
				case nameV.K == py.Str && claudeSearchTools[name]:
					kind, target = "search", Some("search:"+H(py.DumpsOpt(inp, py.DumpOpts{EnsureASCII: true, SortKeys: true})))
				case name == "Skill" && nameV.K == py.Str:
					su.skillUse(inp)
				case inStrs(nameV, "Agent", "Task", "SendMessage"):
					kind = "agent"
				case nameV.K == py.Str && strings.HasPrefix(name, "mcp__"):
					kind, target = "mcp", Some("mcp:"+H(py.DumpsOpt(inp, py.DumpOpts{EnsureASCII: true, SortKeys: true})))
				}
				ci := b.addCall(Call{TS: tsOf(ts), CallID: FromValue(c.Get("id")), Name: FromValue(nameV), Kind: kind, Target: target,
					Path: path_, InChars: py.Len(py.Dumps(inp))}, step)
				callByID[key(c.Get("id"))] = ci
			}
			return
		}
		// user line
		content := m.Get("content")
		if content.K == py.Arr {
			hasResult := false
			for _, c := range content.A {
				if c.K == py.Obj && isStr(c.Get("type"), "tool_result") {
					hasResult = true
					break
				}
			}
			if hasResult {
				for _, c := range content.A {
					if !(c.K == py.Obj && isStr(c.Get("type"), "tool_result")) {
						continue
					}
					ci, ok := callByID[key(c.Get("tool_use_id"))]
					if !ok {
						continue
					}
					text := outText(c.Get("content"))
					call := &b.calls[ci]
					call.OutChars = py.Len(text)
					call.Failed = TriOf(c.Get("is_error").Truthy())
					if call.Kind == "commit" {
						b.addCommits(text, ts, FromValue(o.Get("cwd")))
					}
				}
				return
			}
		}
		if o.Get("isMeta").Truthy() || sub || o.Get("isCompactSummary").Truthy() {
			return
		}
		var text string
		switch content.K {
		case py.Str:
			text = content.S
		case py.Arr:
			var parts []string
			for _, c := range content.A {
				if c.K == py.Obj && isStr(c.Get("type"), "text") {
					parts = append(parts, textOrEmpty(c.Get("text"), c.Has("text")))
				}
			}
			text = strings.Join(parts, "\n")
		default:
			return
		}
		s := py.LStrip(text)
		if inStrs(o.Get("promptSource"), "typed", "suggestion_accepted", "queued") {
			b.meta.typedPrompts++
		}
		if origin := o.Get("origin").Get("kind"); origin.Truthy() && !isStr(origin, "human") {
			return
		}
		if strings.HasPrefix(s, "[Request interrupted by user") {
			b.meta.nAborted++
			b.turns = append(b.turns, Turn{TS: tsOf(ts), Kind: "interrupt"})
			return
		}
		if s == "" || strings.HasPrefix(s, "<local-command") || strings.HasPrefix(s, "<system-reminder>") || strings.HasPrefix(s, "<task-notification") ||
			strings.HasPrefix(s, "Caveat:") || strings.HasPrefix(s, "<command-stdout") {
			return
		}
		kind := "human"
		if strings.HasPrefix(s, "<command-name>") {
			kind = "slash"
		}
		b.turns = append(b.turns, Turn{TS: tsOf(ts), Kind: kind, Text: text})
	})
	if err != nil {
		return nil, err
	}
	if sub {
		b.meta.source = py.String("subagent")
		b.meta.parentID = b.meta.sessionID
	} else {
		b.meta.source = py.String("user")
	}
	f := b.finish()
	f.Events = events
	f.Stats = st
	if sub { // a sub-agent's listing and files are its parent's; only its skill use counts
		su = &Setup{SkillsAt: NaT, InstrAt: NaT, AgentsAt: NaT, SkillUses: su.SkillUses}
	}
	if !su.Empty() {
		f.Setup = su
	}
	return f, nil
}

func textOrEmpty(v py.Value, has bool) string {
	if !has {
		return ""
	}
	return py.PyStr(v)
}
