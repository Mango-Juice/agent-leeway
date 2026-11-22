package drift

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/proc"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Judges run through the user's own official CLIs: `claude -p` for Claude Code sessions, `codex exec` for Codex
// sessions. This is the only place where session text leaves the machine, and only to the session's own
// provider. The prompt and the transcript go on stdin, never on the command line; the system prompt goes in
// a temporary file. LEEWAY_JUDGE_CMD replaces the CLIs with a local program (tests, dry runs): it gets
// {"tier","system","prompt"} as JSON on stdin and prints {"text": ..., "usage": {...}}.

// limitPhrase matches a usage-limit or rate-limit message. It is applied only to the provider's structured
// error (Claude's result on is_error, Codex's error events), never to arbitrary output.
var limitPhrase = regexp.MustCompile(`(?i)\b(rate[ _-]?limit(s|ed)?|usage[ _-]?limit(s)?|quota|too many requests|overloaded|status 429|429 too many|credit balance is too low|out of (extra )?usage)\b`)

// ErrLimit marks a usage-limit / rate-limit answer: no further calls should start.
type ErrLimit struct{ Msg string }

func (e *ErrLimit) Error() string { return e.Msg }

// ErrConfig marks a judge CLI that rejected its arguments, is too old, or acted (used a tool) although
// tools are switched off: no further calls should start, and nothing is recorded.
type ErrConfig struct{ Msg string }

func (e *ErrConfig) Error() string { return e.Msg }

func classify(msg string) error {
	msg = strings.TrimSpace(msg)
	if limitPhrase.MatchString(msg) {
		return &ErrLimit{Msg: py.Head(msg, 300)}
	}
	return errors.New(py.Head(msg, 300))
}

// usageBanner is how argument parsers (clap, commander) start an argument error.
var usageBanner = regexp.MustCompile(`(?m)^(Usage:|error: (unexpected|unknown|invalid)|error: unrecognized)`)

// DefaultTimeout per judge call.
const DefaultTimeout = 30 * time.Minute

// cmdMeta are characters cmd.exe interprets even inside quotes; with a .cmd/.bat shim we refuse to pass them.
const cmdMeta = "&|<>^%!\"\r\n"

// Resolve finds a provider's CLI. On Windows a native .exe is preferred over a .cmd/.bat shim.
func Resolve(provider string) (string, error) {
	name := provider
	if runtime.GOOS == "windows" {
		// the native installer's location (docs/16), then any .exe on PATH, before a .cmd shim
		if provider == "claude" {
			p := filepath.Join(config.Home(), ".local", "bin", "claude.exe")
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				return p, nil
			}
		}
		if p, err := exec.LookPath(name + ".exe"); err == nil {
			return p, nil
		}
	}
	return exec.LookPath(name)
}

// Available reports whether sessions of this provider can be judged here.
func Available(provider string) bool {
	if os.Getenv("LEEWAY_JUDGE_CMD") != "" {
		return true
	}
	_, err := Resolve(provider)
	return err == nil
}

func isShim(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".cmd" || ext == ".bat"
}

// checkShimArgs fails closed when a batch shim would receive characters cmd.exe could interpret.
func checkShimArgs(path string, args []string) error {
	if !isShim(path) {
		return nil
	}
	for _, a := range args {
		if strings.ContainsAny(a, cmdMeta) {
			return fmt.Errorf("%s is a batch shim and an argument contains characters cmd.exe would interpret; install the native executable or set a temporary folder without such characters", filepath.Base(path))
		}
	}
	return nil
}

// Call runs one judge call. -> answer text, usage, seconds.
func Call(ctx context.Context, tier, system, prompt string, timeout time.Duration) (string, config.Usage, float64, error) {
	t0 := time.Now()
	text, u, err := call(ctx, tier, system, prompt, timeout)
	return text, u, time.Since(t0).Seconds(), err
}

// privateDir makes a fresh, empty folder for one judge call: under the user's own cache folder (never a shared
// temporary folder), mode 0700, with an unpredictable name. The caller removes it.
func privateDir() (string, error) {
	base := filepath.Join(config.CacheDir(), "judge")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	for _, d := range []string{config.CacheDir(), base} {
		if err := checkPrivate(d); err != nil {
			return "", &ErrConfig{Msg: err.Error()}
		}
	}
	return os.MkdirTemp(base, "run-")
}

func call(ctx context.Context, tier, system, prompt string, timeout time.Duration) (string, config.Usage, error) {
	prov, model, _ := strings.Cut(tier, ":")
	wd, err := privateDir()
	if err != nil {
		return "", config.Usage{}, err
	}
	defer os.RemoveAll(wd) // runs on success, error, timeout and cancel (SIGTERM and SIGHUP cancel too)
	if fake := os.Getenv("LEEWAY_JUDGE_CMD"); fake != "" {
		text, u, err := callFake(ctx, fake, tier, model, system, prompt, wd, timeout)
		return text, u, asModelErr(tier, err, proc.Result{})
	}
	sys, err := os.CreateTemp(wd, "system-*.md")
	if err != nil {
		return "", config.Usage{}, err
	}
	defer os.Remove(sys.Name())
	if _, err := sys.WriteString(system); err != nil {
		sys.Close()
		return "", config.Usage{}, err
	}
	sys.Close()
	if _, err := Preflight(prov); err != nil { // never run a CLI that lacks a lock-down flag
		return "", config.Usage{}, err
	}
	exe, err := Resolve(prov)
	if err != nil {
		return "", config.Usage{}, fmt.Errorf("%s CLI not found", prov)
	}
	model = CLIModel(tier) // the exact checked id, an alias for "newest", or "" for the CLI's default
	if prov == "claude" {
		args := ClaudeArgs(model, sys.Name())
		if err := checkShimArgs(exe, append(args, wd, exe)); err != nil {
			return "", config.Usage{}, err
		}
		r, err := proc.Run(ctx, exe, args, wd, judgeEnv(), []byte(Stdin(tier, prompt)), timeout)
		if err != nil {
			debugCall(tier, exe, preflightVersion(prov), args, r, nil, err)
			return "", config.Usage{}, err
		}
		text, u, perr := parseClaude(r, config.Prices().JudgeModel(tier))
		perr = asModelErr(tier, perr, r)
		debugCall(tier, exe, preflightVersion(prov), args, r, claudeEvents(r), perr)
		return text, u, perr
	}
	effort := config.CodexEffort[model]
	if effort == "" {
		effort = "low"
	}
	args := CodexArgs(model, effort, sys.Name(), wd)
	if err := checkShimArgs(exe, append(args, wd, exe)); err != nil {
		return "", config.Usage{}, err
	}
	r, err := proc.Run(ctx, exe, args, wd, judgeEnv(), []byte(Stdin(tier, prompt)), timeout)
	if err != nil {
		debugCall(tier, exe, preflightVersion(prov), args, r, nil, err)
		return "", config.Usage{}, err
	}
	text, u, notes, perr := parseCodex(r, model)
	perr = asModelErr(tier, perr, r)
	for _, n := range notes { // Codex rerouted the request to another model: a model change, never recorded
		if perr == nil && strings.HasPrefix(strings.ToLower(n), "model rerouted") {
			perr = &ErrModel{Tier: tier, Msg: n}
		}
	}
	for _, n := range notes {
		notesOf(ctx).add(n)
	}
	debugCall(tier, exe, preflightVersion(prov), args, r, codexEvents(r), perr)
	return text, u, perr
}

func num(v py.Value) float64 { f, _ := v.Num(); return f }

// ClaudeArgs is the `claude -p` command line (no prompt text). Enforced by the CLI:
//   - --tools "": no built-in tools (no shell, file, web or edit tools);
//   - --restricted: user, project and local settings files are ignored (no hooks or permissions from them),
//     command-running tools and WebFetch are removed, file tools are confined to the working folder;
//   - --safe-mode: CLAUDE.md files, skills, plugins, hooks, MCP servers and custom agents are not loaded;
//   - --strict-mcp-config with no --mcp-config: no MCP servers;
//   - --disable-slash-commands; --permission-prompts none: anything that would ask is denied;
//   - --no-session-persistence: the judge session is not saved.
//
// No --fallback-model: the CLI would switch models silently mid-run, and a judge model is only changed with the
// user's consent (judges.go).
//
// Requires Claude Code 2.1.259 or later (--permission-prompts); an older CLI rejects the flags and the run stops.
func ClaudeArgs(model, systemFile string) []string {
	return []string{"-p", "--model", model, "--output-format", "json", "--tools", "", "--restricted", "--safe-mode",
		"--strict-mcp-config", "--disable-slash-commands", "--permission-prompts", "none", "--no-session-persistence", "--system-prompt-file", systemFile}
}

// codexDisabled are Codex features that give the model a tool or load outside configuration
// (`codex features list`; docs: learn.chatgpt.com config reference, read 2026-11-22).
var codexDisabled = []string{"shell_tool", "unified_exec", "view_image", "apps", "plugins", "remote_plugin", "hooks", "memories",
	"multi_agent", "image_generation", "browser_use", "computer_use", "skill_search", "tool_suggest", "goals"}

// CodexArgs is the `codex exec` command line (no prompt text). Enforced by the CLI: the features above are off
// (no shell or exec tool, no image viewing, apps, plugins, hooks, memories or sub-agents), web search is
// disabled, user config, rules and project config are not loaded (an empty folder that is not a trusted
// project is the working root, and it is its own project root so no AGENTS.md is found above it), the sandbox
// is read-only and approvals are never asked. parseCodex also fails the call if any tool item appears anyway.
func CodexArgs(model, effort, systemFile, dir string) []string {
	args := []string{"exec"}
	if model != "" { // "" = the CLI's own default model
		args = append(args, "-m", model)
	}
	args = append(args, "-c", "model_reasoning_effort="+effort, "-c", "model_instructions_file="+systemFile,
		"-c", "web_search=disabled", "-c", "approval_policy=never", "-c", "project_root_markers=[]")
	for _, f := range codexDisabled {
		args = append(args, "--disable", f)
	}
	return append(args, "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "--ignore-rules", "-s", "read-only", "--json", "-C", dir, "-")
}

// verboseEnv are variables that make the judge CLIs log verbosely to stderr (and into the diagnostics file);
// they are cleared for judge calls.
var verboseEnv = []string{"RUST_LOG", "RUST_BACKTRACE", "RUST_LIB_BACKTRACE", "ANTHROPIC_LOG", "DEBUG", "CLAUDE_CODE_DEBUG_LOGS_DIR"}

// judgeEnv is the environment of a judge call: the user's, without verbose-logging variables.
func judgeEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, v := range verboseEnv {
			drop = drop || strings.EqualFold(k, v)
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

func trimmed(b []byte) string { return strings.TrimSpace(py.DecodeBytes(b)) }

// asModelErr turns a refusal of the model itself (unknown, or not allowed for the account) into ErrModel. The
// Codex notice "Model metadata for X not found" is not a refusal: the call goes on with that model.
func asModelErr(tier string, err error, r proc.Result) error {
	if err == nil {
		return nil
	}
	if me, ok := IsErrModel(err); ok {
		if me.Tier == "" {
			me.Tier = tier
		}
		return me
	}
	var cfg *ErrConfig
	var lim *ErrLimit
	if errors.As(err, &cfg) || errors.As(err, &lim) {
		return err
	}
	msg := err.Error()
	text := msg + "\n" + trimmed(r.Stderr)
	if modelRefused.MatchString(text) {
		return &ErrModel{Tier: tier, Msg: NoteText(msg)}
	}
	return err
}

// argError recognizes a CLI that rejected its command line (unknown flag, too old).
func argError(r proc.Result, name string) error {
	if r.ExitCode != 0 && usageBanner.MatchString(trimmed(r.Stderr)) {
		return &ErrConfig{Msg: name + " rejected its arguments (is it too old?): " + py.Head(trimmed(r.Stderr), 300)}
	}
	return nil
}

// aliasModel says whether a judge model name is an alias the CLI resolves itself ("opus@latest", "default",
// a bare family name), not an exact model id.
func aliasModel(m string) bool {
	return strings.HasSuffix(m, "@latest") || m == "default" || !strings.Contains(m, "-")
}

// parseClaude reads `claude -p --output-format json`. judgeModel is the price-table id of the chosen judge (e.g.
// claude-opus-5-5): the record names that model, and any other model in modelUsage (the CLI's own helper calls)
// is kept as Extra usage and priced separately, so the "judged by" line never names a helper model.
func parseClaude(r proc.Result, judgeModel string) (string, config.Usage, error) {
	d, err := py.Loads(py.DecodeBytes(r.Stdout))
	if err != nil || d.K != py.Obj {
		if e := argError(r, "claude"); e != nil {
			return "", config.Usage{}, e
		}
		return "", config.Usage{}, classify("claude error: " + py.Head(trimmed(r.Stderr), 200))
	}
	if d.Get("is_error").Truthy() || !d.Get("result").Truthy() {
		msg := d.Get("result").Or(d.Get("subtype"))
		s := trimmed(r.Stderr)
		if msg.Truthy() {
			s = py.PyStr(msg)
		}
		return "", config.Usage{}, classify("claude error: " + py.Head(s, 200))
	}
	// with no tools the judge answers in one turn; anything else means a tool was used or asked for
	if n, ok := d.Get("num_turns").Num(); ok && n > 1 {
		return "", config.Usage{}, &ErrConfig{Msg: fmt.Sprintf("the Claude judge took %v turns although tools are off; stopped", n)}
	}
	if pd := d.Get("permission_denials"); pd.K == py.Arr && len(pd.A) > 0 {
		return "", config.Usage{}, &ErrConfig{Msg: "the Claude judge asked to use a tool although tools are off; stopped"}
	}
	u := d.Get("usage")
	var models, judgeKeys []string
	var extra []config.ModelTokens
	if mu := d.Get("modelUsage"); mu.K == py.Obj {
		models = mu.O.Keys
		for _, k := range models {
			if k == judgeModel || (judgeModel != "" && strings.HasPrefix(k, judgeModel)) {
				judgeKeys = append(judgeKeys, k)
				continue
			}
			e := mu.Get(k)
			extra = append(extra, config.ModelTokens{Model: k, Input: num(e.Get("inputTokens")), CacheRead: num(e.Get("cacheReadInputTokens")),
				CacheWrite: num(e.Get("cacheCreationInputTokens")), Output: num(e.Get("outputTokens"))})
		}
	}
	model := strings.Join(judgeKeys, ",")
	fallback := false
	if len(judgeKeys) == 0 && len(models) > 0 {
		if !aliasModel(judgeModel) { // an exact model was asked for and another answered: a model change
			return "", config.Usage{}, &ErrModel{Msg: "the Claude CLI answered with " + strings.Join(models, ",") + " instead of " + judgeModel}
		}
		// an alias ("opus", "default"): the main model is the one with the most output; the rest are helpers
		main := models[0]
		for _, k := range models {
			if num(d.Get("modelUsage").Get(k).Get("outputTokens")) > num(d.Get("modelUsage").Get(main).Get("outputTokens")) {
				main = k
			}
		}
		model, fallback = main, true
		extra = nil
		for _, k := range models {
			if k == main {
				continue
			}
			e := d.Get("modelUsage").Get(k)
			extra = append(extra, config.ModelTokens{Model: k, Input: num(e.Get("inputTokens")), CacheRead: num(e.Get("cacheReadInputTokens")),
				CacheWrite: num(e.Get("cacheCreationInputTokens")), Output: num(e.Get("outputTokens"))})
		}
	}
	cc := u.Get("cache_creation")
	return py.PyStr(d.Get("result")), config.Usage{Input: num(u.Get("input_tokens")), CacheRead: num(u.Get("cache_read_input_tokens")),
		CacheWrite: num(u.Get("cache_creation_input_tokens")), CacheWrite5m: num(cc.Get("ephemeral_5m_input_tokens")),
		CacheWrite1h: num(cc.Get("ephemeral_1h_input_tokens")), Output: num(u.Get("output_tokens")), Model: model, Extra: extra, Fallback: fallback}, nil
}

// codexAllowedItems are the only items a judge without tools may produce. An "error" item is the CLI's own
// non-fatal notice (codex-rs exec_events.rs: "Describes a non-fatal error surfaced as an item"; emitted for
// config warnings, warnings, deprecation notices and model reroutes, e.g. 0.169.0's "Model metadata for
// `gpt-6.2-astra` not found ..." or "Falling back from WebSockets to HTTPS transport ..."): it is not a tool
// call, so it is kept as a note and the turn decides the outcome. Every other item (command_execution,
// file_change, mcp_tool_call, collab_tool_call, web_search, todo_list, or anything new) stops the run.
var codexAllowedItems = map[string]bool{"agent_message": true, "reasoning": true}

// parseCodex reads `codex exec --json`: the answer, the usage, and the CLI's notices (error items).
func parseCodex(r proc.Result, model string) (string, config.Usage, []string, error) {
	text := ""
	var usage *config.Usage
	var problem string
	var notes []string
	for _, line := range strings.Split(py.DecodeBytes(r.Stdout), "\n") {
		o, err := py.Loads(strings.TrimSuffix(line, "\r"))
		if err != nil || o.K != py.Obj {
			continue
		}
		typ := o.Get("type").StrOr("")
		if strings.HasPrefix(typ, "item.") {
			it := o.Get("item").Get("type").StrOr("")
			if it == "error" {
				if typ == "item.completed" {
					notes = append(notes, NoteText(o.Get("item").Get("message").StrOr("")))
				}
				continue
			}
			if !codexAllowedItems[it] {
				return "", config.Usage{}, notes, &ErrConfig{Msg: "the Codex judge produced a " + it + " item (a tool call) although tools are off; stopped, nothing recorded"}
			}
		}
		if typ == "item.completed" && isStrV(o.Get("item").Get("type"), "agent_message") {
			text = o.Get("item").Get("text").StrOr("")
		}
		if typ == "error" || typ == "turn.failed" {
			m := o.Get("message").Or(o.Get("error").Get("message")).Or(o.Get("error"))
			problem = py.PyStr(m)
		}
		if typ == "turn.completed" {
			u := o.Get("usage")
			usage = &config.Usage{Input: num(u.Get("input_tokens")) - num(u.Get("cached_input_tokens")), CacheRead: num(u.Get("cached_input_tokens")),
				CacheWrite: num(u.Get("cache_write_input_tokens")), Output: num(u.Get("output_tokens")) + num(u.Get("reasoning_output_tokens")), Model: model}
		}
	}
	if usage == nil {
		if e := argError(r, "codex"); e != nil {
			return "", config.Usage{}, notes, e
		}
		if problem != "" {
			return "", config.Usage{}, notes, classify("codex failed: " + NoteText(problem))
		}
		if len(notes) > 0 { // no turn result: the CLI's own last notice is the best explanation
			return "", config.Usage{}, notes, classify("codex failed: " + notes[len(notes)-1])
		}
		if s := trimmed(r.Stderr); s != "" {
			return "", config.Usage{}, notes, errors.New("codex failed: " + NoteText(py.Tail(s, 300)))
		}
		return "", config.Usage{}, notes, fmt.Errorf("codex failed: exit status %d, no message", r.ExitCode)
	}
	return text, *usage, notes, nil
}

// codexEvents lists the output's event types for the diagnostics file, with the messages of errors and
// error items; never the answer or reasoning text.
func codexEvents(r proc.Result) []string {
	var out []string
	for _, line := range strings.Split(py.DecodeBytes(r.Stdout), "\n") {
		o, err := py.Loads(strings.TrimSuffix(line, "\r"))
		if err != nil || o.K != py.Obj {
			continue
		}
		typ := o.Get("type").StrOr("")
		e := typ
		if strings.HasPrefix(typ, "item.") {
			it := o.Get("item").Get("type").StrOr("")
			e += " " + it
			if it == "error" {
				e += ": " + NoteText(o.Get("item").Get("message").StrOr(""))
			}
		}
		if typ == "error" || typ == "turn.failed" {
			e += ": " + NoteText(py.PyStr(o.Get("message").Or(o.Get("error").Get("message")).Or(o.Get("error"))))
		}
		out = append(out, e)
	}
	return out
}

// claudeEvents is the non-content part of `claude -p --output-format json` for the diagnostics file.
func claudeEvents(r proc.Result) []string {
	d, err := py.Loads(py.DecodeBytes(r.Stdout))
	if err != nil || d.K != py.Obj {
		return []string{"no JSON on stdout"}
	}
	e := fmt.Sprintf("type=%s subtype=%s is_error=%v num_turns=%s", d.Get("type").StrOr(""), d.Get("subtype").StrOr(""), d.Get("is_error").Truthy(), py.PyStr(d.Get("num_turns")))
	if pd := d.Get("permission_denials"); pd.K == py.Arr {
		e += fmt.Sprintf(" permission_denials=%d", len(pd.A))
	}
	if mu := d.Get("modelUsage"); mu.K == py.Obj {
		e += " models=" + strings.Join(mu.O.Keys, ",")
	}
	out := []string{e}
	if d.Get("is_error").Truthy() {
		out = append(out, "error: "+NoteText(py.PyStr(d.Get("result"))))
	}
	return out
}

// callFake runs LEEWAY_JUDGE_CMD (split on spaces; no shell).
func callFake(ctx context.Context, fake, tier, model, system, prompt, wd string, timeout time.Duration) (string, config.Usage, error) {
	parts := strings.Fields(fake)
	req := &py.Object{}
	req.Set("tier", py.String(tier))
	req.Set("system", py.String(system))
	req.Set("prompt", py.String(prompt))
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	r, err := proc.Run(ctx, parts[0], parts[1:], wd, nil, []byte(py.Dumps(py.Value{K: py.Obj, O: req})), timeout)
	if err != nil {
		return "", config.Usage{}, err
	}
	if r.ExitCode != 0 {
		return "", config.Usage{}, classify("judge command failed: " + py.Tail(trimmed(r.Stderr), 300))
	}
	d, err := py.Loads(py.DecodeBytes(r.Stdout))
	if err != nil || d.K != py.Obj {
		return "", config.Usage{}, fmt.Errorf("judge command printed no JSON")
	}
	u := d.Get("usage")
	m := u.Get("model")
	ms := model
	if m.K == py.Str {
		ms = m.S
	}
	return py.PyStr(d.Get("text").Or(py.String(""))), config.Usage{Input: num(u.Get("input")), CacheRead: num(u.Get("cache_read")), CacheWrite: num(u.Get("cache_write")),
		Output: num(u.Get("output")), Model: ms}, nil
}

// codexSuffix is appended to the Codex judge's input (part of the judge input, unchanged since the reference).
const codexSuffix = "\n\nDo not use tools; answer with the JSON directly."

// Stdin is exactly what a judge CLI reads on stdin for a session.
func Stdin(tier, prompt string) string {
	if strings.HasPrefix(tier, "codex:") {
		return prompt + codexSuffix
	}
	return prompt
}
