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
		return callFake(ctx, fake, tier, model, system, prompt, wd, timeout)
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
	if prov == "claude" {
		args := ClaudeArgs(model, sys.Name())
		if err := checkShimArgs(exe, append(args, wd, exe)); err != nil {
			return "", config.Usage{}, err
		}
		r, err := proc.Run(ctx, exe, args, wd, nil, []byte(Stdin(tier, prompt)), timeout)
		if err != nil {
			return "", config.Usage{}, err
		}
		return parseClaude(r, config.Prices().JudgeModel(tier))
	}
	effort := config.CodexEffort[model]
	if effort == "" {
		effort = "low"
	}
	args := CodexArgs(model, effort, sys.Name(), wd)
	if err := checkShimArgs(exe, append(args, wd, exe)); err != nil {
		return "", config.Usage{}, err
	}
	r, err := proc.Run(ctx, exe, args, wd, nil, []byte(Stdin(tier, prompt)), timeout)
	if err != nil {
		return "", config.Usage{}, err
	}
	return parseCodex(r, model)
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
// Requires Claude Code 2.1.259 or later (--permission-prompts); an older CLI rejects the flags and the run stops.
func ClaudeArgs(model, systemFile string) []string {
	return []string{"-p", "--model", model, "--output-format", "json", "--tools", "", "--restricted", "--safe-mode", "--strict-mcp-config",
		"--disable-slash-commands", "--permission-prompts", "none", "--no-session-persistence", "--system-prompt-file", systemFile}
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
	args := []string{"exec", "-m", model, "-c", "model_reasoning_effort=" + effort, "-c", "model_instructions_file=" + systemFile,
		"-c", "web_search=disabled", "-c", "approval_policy=never", "-c", "project_root_markers=[]"}
	for _, f := range codexDisabled {
		args = append(args, "--disable", f)
	}
	return append(args, "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "--ignore-rules", "-s", "read-only", "--json", "-C", dir, "-")
}

func trimmed(b []byte) string { return strings.TrimSpace(py.DecodeBytes(b)) }

// argError recognizes a CLI that rejected its command line (unknown flag, too old).
func argError(r proc.Result, name string) error {
	if r.ExitCode != 0 && usageBanner.MatchString(trimmed(r.Stderr)) {
		return &ErrConfig{Msg: name + " rejected its arguments (is it too old?): " + py.Head(trimmed(r.Stderr), 300)}
	}
	return nil
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
	model := strings.Join(models, ",") // no model matched the judge: keep what the CLI reported
	if len(judgeKeys) > 0 {
		model = strings.Join(judgeKeys, ",")
	} else {
		extra = nil
	}
	cc := u.Get("cache_creation")
	return py.PyStr(d.Get("result")), config.Usage{Input: num(u.Get("input_tokens")), CacheRead: num(u.Get("cache_read_input_tokens")),
		CacheWrite: num(u.Get("cache_creation_input_tokens")), CacheWrite5m: num(cc.Get("ephemeral_5m_input_tokens")),
		CacheWrite1h: num(cc.Get("ephemeral_1h_input_tokens")), Output: num(u.Get("output_tokens")), Model: model, Extra: extra}, nil
}

// codexAllowedItems are the only items a judge without tools may produce.
var codexAllowedItems = map[string]bool{"agent_message": true, "reasoning": true}

func parseCodex(r proc.Result, model string) (string, config.Usage, error) {
	text := ""
	var usage *config.Usage
	var problem string
	for _, line := range strings.Split(py.DecodeBytes(r.Stdout), "\n") {
		o, err := py.Loads(strings.TrimSuffix(line, "\r"))
		if err != nil || o.K != py.Obj {
			continue
		}
		typ := o.Get("type").StrOr("")
		if strings.HasPrefix(typ, "item.") {
			it := o.Get("item").Get("type").StrOr("")
			if !codexAllowedItems[it] {
				return "", config.Usage{}, &ErrConfig{Msg: "the Codex judge produced a " + it + " item although tools are off; stopped, nothing recorded"}
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
			return "", config.Usage{}, e
		}
		if problem != "" {
			return "", config.Usage{}, classify("codex failed: " + problem)
		}
		return "", config.Usage{}, errors.New("codex failed: " + py.Tail(trimmed(r.Stderr), 300))
	}
	return text, *usage, nil
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
