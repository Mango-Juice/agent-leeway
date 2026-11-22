// Package testfake is a stand-in for the judge CLIs in tests (no model calls). Test binaries run it when
// LEEWAY_FAKE_MODE is set: they read {"tier","system","prompt"} (or a CLI's stdin) and answer with
// testdata/judge/<tag>.txt for the "[fx:<tag>]" marker in the prompt (FIXTURE_JUDGE_DIR).
package testfake

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var tagRE = regexp.MustCompile(`\[fx:(\w+)\]`)

// Main is the fake judge: modes ok (canned answers), echo (returns the prompt), text (a fixed answer file), limit, fail, sleep,
// grandchild (starts a child that hangs, writes its pid to LEEWAY_FAKE_PID), claude-cli and codex-cli
// (the official CLIs' protocols). LEEWAY_FAKE_ARGV records argv.
func Main(mode string) int {
	if p := os.Getenv("LEEWAY_FAKE_ARGV"); p != "" {
		f, _ := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		fmt.Fprintln(f, strings.Join(os.Args, "\x00"))
		f.Close()
	}
	if mode == "claude-cli" || mode == "codex-cli" { // the CLIs' own help and version output (preflight)
		if out, ok := cliInfo(mode, os.Args[1:]); ok {
			fmt.Print(out)
			return 0
		}
	}
	in, _ := readAll(os.Stdin)
	switch mode {
	case "claude-cli": // `claude -p ... --system-prompt-file F` with the prompt on stdin
		sys := ""
		for i, a := range os.Args {
			if a == "--system-prompt-file" && i+1 < len(os.Args) {
				b, _ := os.ReadFile(os.Args[i+1])
				sys = string(b)
			}
		}
		o := &py.Object{}
		o.Set("result", py.String("SYSTEM="+strconv.Itoa(py.Len(sys))+" PROMPT="+in))
		u := &py.Object{}
		u.Set("input_tokens", py.Value{K: py.Int, I: 10})
		u.Set("output_tokens", py.Value{K: py.Int, I: 5})
		o.Set("usage", py.Value{K: py.Obj, O: u})
		mu := &py.Object{}
		mu.Set("claude-opus-5-5", py.Value{K: py.Obj, O: &py.Object{}})
		o.Set("modelUsage", py.Value{K: py.Obj, O: mu})
		fmt.Println(py.Dumps(py.Value{K: py.Obj, O: o}))
		return 0
	case "codex-cli": // `codex exec ... -` with the prompt on stdin, JSONL events on stdout
		// LEEWAY_FAKE_CODEX: shapes recorded from codex-cli 0.169.0 (`codex exec --json`, 2026-11-23)
		notice := `{"type":"item.completed","item":{"id":"item_0","type":"error","message":"Model metadata for ` + "`gpt-6.2-astra`" + ` not found. Defaulting to fallback metadata; this can degrade performance and cause issues."}}`
		switch os.Getenv("LEEWAY_FAKE_CODEX") {
		case "notice-ok":
			fmt.Println(`{"type":"thread.started","thread_id":"t"}`)
			fmt.Println(notice)
			fmt.Println(`{"type":"turn.started"}`)
		case "notice-fail":
			home, _ := os.UserHomeDir()
			for i := 0; i < 20; i++ { // verbose CLI logging: only the last lines reach the diagnostics file
				fmt.Fprintf(os.Stderr, "2026-11-23T00:00:%02dZ DEBUG codex_core: verbose line %d\n", i, i)
			}
			fmt.Fprintln(os.Stderr, "ERROR codex_api: unauthorized for someone@example.com, config at "+home+"/.codex/config.toml")
			fmt.Println(`{"type":"thread.started","thread_id":"t"}`)
			fmt.Println(notice)
			fmt.Println(`{"type":"turn.started"}`)
			fmt.Println(`{"type":"error","message":"Reconnecting... 2/5 (unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses)"}`)
			fmt.Println(`{"type":"item.completed","item":{"id":"item_1","type":"error","message":"Falling back from WebSockets to HTTPS transport. unexpected status 401 Unauthorized"}}`)
			fmt.Println(`{"type":"error","message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses"}`)
			fmt.Println(`{"type":"turn.failed","error":{"message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses"}}`)
			return 1
		case "refuse":
			fmt.Println(`{"type":"turn.started"}`)
			fmt.Println(`{"type":"error","message":"The 'gpt-6.2-astra' model is not supported when using Codex with a ChatGPT account."}`)
			fmt.Println(`{"type":"turn.failed","error":{"message":"The 'gpt-6.2-astra' model is not supported when using Codex with a ChatGPT account."}}`)
			return 1
		case "reroute": // the request was rerouted to another model (an error item, ModelRerouted in codex-rs)
			fmt.Println(`{"type":"item.completed","item":{"id":"item_0","type":"error","message":"model rerouted: gpt-6.2-astra -> gpt-6.2-sol (HighRiskCyberActivity)"}}`)
		case "unauthorized-dotless": // an unrelated failure for a dot-less model id: not a model refusal
			fmt.Println(`{"type":"item.completed","item":{"id":"item_0","type":"error","message":"Model metadata for ` + "`gpt-7-astra`" + ` not found. Defaulting to fallback metadata; this can degrade performance and cause issues."}}`)
			fmt.Println(`{"type":"turn.failed","error":{"message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header"}}`)
			fmt.Fprintln(os.Stderr, "Model metadata for `gpt-7-astra` not found")
			return 1
		case "tool":
			fmt.Println(`{"type":"item.started","item":{"id":"item_0","type":"command_execution","command":"ls","status":"in_progress"}}`)
		}
		answer := "PROMPT=" + in
		if a := os.Getenv("LEEWAY_FAKE_TEXT"); a != "" {
			answer = a
		}
		fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":` + py.Dumps(py.String(answer)) + `}}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":7,"reasoning_output_tokens":3}}`)
		return 0
	case "refuse": // the judge CLI refuses the model (codex-cli 0.171.0 wording, 2026-11-23)
		fmt.Fprintln(os.Stderr, "The 'gpt-6.2-astra' model is not supported when using Codex with a ChatGPT account.")
		return 1
	case "limit":
		fmt.Fprintln(os.Stderr, "Error: rate limit reached (429)")
		return 1
	case "fail":
		fmt.Println("not json")
		return 1
	case "sleep":
		time.Sleep(3 * time.Second)
	case "grandchild":
		c := exec.Command(os.Args[0], "-fake-judge")
		c.Env = append(os.Environ(), "LEEWAY_FAKE_MODE=hang", "LEEWAY_CLI_ARGS=")
		_ = c.Start()
		if p := os.Getenv("LEEWAY_FAKE_PID"); p != "" {
			_ = os.WriteFile(p, []byte(strconv.Itoa(c.Process.Pid)), 0o600)
		}
		time.Sleep(60 * time.Second)
	case "hang":
		time.Sleep(120 * time.Second)
		return 0
	}
	req, err := py.Loads(in)
	if err != nil {
		return 3
	}
	prompt := req.Get("prompt").S
	tag := "none"
	if m := tagRE.FindStringSubmatch(prompt); m != nil {
		tag = m[1]
	}
	text := `{"events": [], "verdict": "as_intended"}`
	if b, err := os.ReadFile(filepath.Join(os.Getenv("FIXTURE_JUDGE_DIR"), tag+".txt")); err == nil {
		text = string(b)
	}
	if mode == "echo" { // returns the prompt so tests can check it survived unchanged
		text = prompt
	}
	if mode == "text" { // returns the file LEEWAY_FAKE_TEXT and records the request in LEEWAY_FAKE_REQ
		b, _ := os.ReadFile(os.Getenv("LEEWAY_FAKE_TEXT"))
		text = string(b)
		if p := os.Getenv("LEEWAY_FAKE_REQ"); p != "" {
			f, _ := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			fmt.Fprintln(f, in)
			f.Close()
		}
	}
	o := &py.Object{}
	o.Set("text", py.String(text))
	u := &py.Object{}
	for _, kv := range []struct {
		k string
		v int64
	}{{"input", 3000}, {"cache_read", 100}, {"cache_write", 0}, {"output", 400}} {
		u.Set(kv.k, py.Value{K: py.Int, I: kv.v})
	}
	o.Set("usage", py.Value{K: py.Obj, O: u})
	fmt.Println(py.Dumps(py.Value{K: py.Obj, O: o}))
	return 0
}

func readAll(f *os.File) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1<<16)
	for {
		n, err := f.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String(), nil
		}
	}
}

// cliInfo answers --version, --help, exec --help and features list like the tested CLIs, minus the flag
// named in LEEWAY_FAKE_DROP (to test the preflight refusing an older CLI).
func cliInfo(mode string, args []string) (string, bool) {
	drop := os.Getenv("LEEWAY_FAKE_DROP")
	keep := func(lines ...string) string {
		var out []string
		for _, l := range lines {
			if drop == "" || !strings.Contains(l, drop) {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n") + "\n"
	}
	a := strings.Join(args, " ")
	switch {
	case strings.Contains(a, "--version"):
		if mode == "claude-cli" {
			return "2.1.329 (Claude Code)\n", true
		}
		if v := os.Getenv("LEEWAY_FAKE_CODEX_VERSION"); v != "" {
			return v + "\n", true
		}
		return "codex-cli 0.171.0\n", true
	case mode == "claude-cli" && strings.Contains(a, "--help"):
		return keep("  -p, --print", "  --model <model>", "  --output-format <format>", "  --tools <tools...>", "  --restricted", "  --safe-mode",
			"  --strict-mcp-config", "  --disable-slash-commands", "  --permission-prompts <target>", "  --no-session-persistence",
			"  --system-prompt-file <file>"), true
	case mode == "codex-cli" && strings.Contains(a, "exec --help"):
		return keep("  -m, --model <MODEL>", "  -c, --config <key=value>", "      --disable <FEATURE>", "      --ephemeral", "      --skip-git-repo-check",
			"      --ignore-user-config", "      --ignore-rules", "  -s, --sandbox <SANDBOX_MODE>", "      --json", "  -C, --cd <DIR>"), true
	case mode == "codex-cli" && strings.Contains(a, "debug models") && os.Getenv("LEEWAY_FAKE_CATALOG") == "offline":
		if strings.Contains(a, "--bundled") { // no account list, but the bundled list names the default
			return `{"models":[{"slug":"gpt-6.1-terra","display_name":"GPT-6.1 Terra","visibility":"list","priority":1}]}` + "\n", true
		}
		return "Error: could not refresh the model list\n", true
	case mode == "codex-cli" && strings.Contains(a, "debug models"):
		// the shape of `codex debug models` (codex-cli 0.171.0, 2026-11-23), cut to the fields read;
		// LEEWAY_FAKE_CATALOG=no-astra leaves GPT-6.2-Astra out (an account without access)
		models := []string{`{"slug":"gpt-6.2-sol","display_name":"GPT-6.2-Sol","visibility":"list","priority":1}`,
			`{"slug":"gpt-6.2-astra","display_name":"GPT-6.2-Astra","visibility":"list","priority":2}`,
			`{"slug":"gpt-reserve","display_name":"GPT-Reserve","visibility":"hide","priority":4}`}
		if os.Getenv("LEEWAY_FAKE_CATALOG") == "no-astra" {
			models = append(models[:1], models[2:]...)
		}
		return `{"models":[` + strings.Join(models, ",") + `]}` + "\n", true
	case mode == "codex-cli" && strings.Contains(a, "features list"):
		var lines []string
		for _, f := range []string{"shell_tool", "unified_exec", "view_image", "apps", "plugins", "remote_plugin", "hooks", "memories", "multi_agent",
			"image_generation", "browser_use", "computer_use", "skill_search", "tool_suggest", "goals"} {
			lines = append(lines, f+"   stable   true")
		}
		return keep(lines...), true
	}
	return "", false
}
