package drift

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/proc"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// TestedCLIs are the judge CLI versions the lock-down flags were checked against (one real probe call each).
var TestedCLIs = map[string]string{"claude": "2.1.329 (Claude Code)", "codex": "codex-cli 0.171.0"}

// claudeNeeds are the flags ClaudeArgs relies on, as `claude --help` spells them.
var claudeNeeds = [][]string{{"--print"}, {"--model"}, {"--output-format"}, {"--tools"}, {"--restricted"}, {"--safe-mode"},
	{"--strict-mcp-config"}, {"--disable-slash-commands"}, {"--permission-prompts"}, {"--no-session-persistence"},
	{"--system-prompt-file", "--system-prompt[-file]"}}

// codexNeeds are the flags CodexArgs relies on, as `codex exec --help` spells them.
var codexNeeds = [][]string{{"--model"}, {"--config"}, {"--disable"}, {"--ephemeral"}, {"--skip-git-repo-check"}, {"--ignore-user-config"},
	{"--ignore-rules"}, {"--sandbox"}, {"--json"}, {"--cd"}}

var (
	preflightMu   sync.Mutex
	preflightDone = map[string]preflightResult{}
)

type preflightResult struct {
	version string
	err     error
}

// Preflight checks, without any model call, that a judge CLI is installed and supports every flag the
// lock-down needs (from its --help output, and for Codex `codex features list`), and returns its version.
// A missing flag is an error naming it: the judge is never run with a weaker command line.
func Preflight(provider string) (string, error) {
	if c := os.Getenv("LEEWAY_JUDGE_CMD"); c != "" {
		return "LEEWAY_JUDGE_CMD", nil
	}
	preflightMu.Lock()
	defer preflightMu.Unlock()
	if r, ok := preflightDone[provider]; ok {
		return r.version, r.err
	}
	v, err := preflight(provider)
	preflightDone[provider] = preflightResult{v, err}
	return v, err
}

func runText(exe string, args ...string) (string, error) {
	dir, err := privateDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	r, err := proc.Run(context.Background(), exe, args, dir, nil, nil, 30*time.Second)
	if err != nil {
		return "", err
	}
	return py.DecodeBytes(r.Stdout) + "\n" + py.DecodeBytes(r.Stderr), nil
}

func preflight(provider string) (string, error) {
	exe, err := Resolve(provider)
	if err != nil {
		return "", &ErrConfig{Msg: fmt.Sprintf("the %s CLI was not found", provider)}
	}
	vtext, err := runText(exe, "--version")
	if err != nil {
		return "", &ErrConfig{Msg: fmt.Sprintf("%s --version failed: %v", provider, err)}
	}
	version := strings.TrimSpace(strings.SplitN(strings.TrimSpace(vtext), "\n", 2)[0])
	helpArgs, needs := []string{"--help"}, claudeNeeds
	if provider == "codex" {
		helpArgs, needs = []string{"exec", "--help"}, codexNeeds
	}
	help, err := runText(exe, helpArgs...)
	if err != nil {
		return version, &ErrConfig{Msg: fmt.Sprintf("%s %s failed: %v", provider, strings.Join(helpArgs, " "), err)}
	}
	for _, alts := range needs {
		found := false
		for _, a := range alts {
			if strings.Contains(help, a) {
				found = true
			}
		}
		if !found {
			return version, &ErrConfig{Msg: fmt.Sprintf("%s %s does not support %s, which the judge lock-down needs; update it (tested with %s)", provider, version, alts[0], TestedCLIs[provider])}
		}
	}
	if provider == "codex" {
		feats, err := runText(exe, "features", "list")
		if err != nil {
			return version, &ErrConfig{Msg: "codex features list failed: " + err.Error()}
		}
		known := map[string]bool{}
		for _, line := range strings.Split(feats, "\n") {
			if f := strings.Fields(line); len(f) > 0 {
				known[f[0]] = true
			}
		}
		for _, f := range codexDisabled {
			if !known[f] {
				return version, &ErrConfig{Msg: fmt.Sprintf("codex %s does not list the feature %q, which the judge lock-down switches off; update it (tested with %s)", version, f, TestedCLIs["codex"])}
			}
		}
	}
	return version, nil
}

// preflightVersion is the version a passed preflight found ("" when unknown).
func preflightVersion(provider string) string {
	preflightMu.Lock()
	defer preflightMu.Unlock()
	return preflightDone[provider].version
}

// OlderThanTested says whether a CLI version ("codex-cli 0.169.0", "2.1.329 (Claude Code)") is older than the
// tested one; false when either cannot be read.
func OlderThanTested(provider, version string) bool {
	a, b := versionNums(version), versionNums(TestedCLIs[provider])
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// BareVersion is the version number alone ("codex-cli 0.169.0" -> "0.169.0").
func BareVersion(s string) string {
	if m := versionRe.FindString(s); m != "" {
		return m
	}
	return s
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func versionNums(s string) []int {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	var out []int
	for _, x := range m[1:] {
		n, _ := strconv.Atoi(x)
		out = append(out, n)
	}
	return out
}

// ResetPreflight forgets earlier checks (tests of several CLI versions in one process).
func ResetPreflight() { resetPreflight() }

// resetPreflight forgets earlier checks (tests).
func resetPreflight() {
	preflightMu.Lock()
	preflightDone = map[string]preflightResult{}
	preflightMu.Unlock()
}
