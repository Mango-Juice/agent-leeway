package drift

import (
	"context"
	"fmt"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/proc"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/secrets"
)

// NoteText is a CLI's own non-fatal notice (Codex "error" items: config warnings, deprecation notices, model
// reroutes, transport fallbacks) as shown to people: secrets masked, one line, at most 300 characters.
func NoteText(s string) string {
	s = strings.Join(strings.Fields(secrets.Mask(s)), " ")
	return py.Head(s, 300)
}

type notesKey struct{}

// Notes collects the CLI notices of one judge call.
type Notes struct {
	mu   sync.Mutex
	List []string
}

func (n *Notes) add(s string) {
	if n == nil || s == "" {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, x := range n.List {
		if x == s {
			return
		}
	}
	n.List = append(n.List, s)
}

// WithNotes returns a context whose judge calls record their CLI notices in n.
func WithNotes(ctx context.Context, n *Notes) context.Context {
	return context.WithValue(ctx, notesKey{}, n)
}

func notesOf(ctx context.Context) *Notes { n, _ := ctx.Value(notesKey{}).(*Notes); return n }

// ---------------------------------------------------------------- diagnostics (--debug-log, LEEWAY_DEBUG)

var (
	debugMu   sync.Mutex
	debugPath string
)

// EnableDebug starts the diagnostics file of this run under dir and returns its path. It records, per judge
// call, the CLI and its version, the arguments (they hold no prompt or session text: those go on stdin), the
// exit status, stderr (secrets masked, capped) and the event types and error messages of the output; never the
// answer text, the prompt or the session.
func EnableDebug(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "judge-"+time.Now().Format("20060102-150405")+".log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	f.Close()
	debugMu.Lock()
	debugPath = p
	debugMu.Unlock()
	return p, nil
}

// DebugPath is the diagnostics file of this run ("" when off).
func DebugPath() string { debugMu.Lock(); defer debugMu.Unlock(); return debugPath }

func debugf(format string, a ...any) {
	debugMu.Lock()
	defer debugMu.Unlock()
	if debugPath == "" {
		return
	}
	f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format, a...)
}

// debugCall writes one judge call: no prompt, no session, no answer text.
func debugCall(tier, exe, version string, args []string, r proc.Result, events []string, err error) {
	if DebugPath() == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s %s\n", time.Now().Format(time.RFC3339), tier)
	fmt.Fprintf(&b, "cli: %s (%s)\n", redact(exe), version)
	var ra []string
	for _, a := range args {
		ra = append(ra, redact(a))
	}
	fmt.Fprintf(&b, "args: %q\n", ra)
	fmt.Fprintf(&b, "exit: %d\n", r.ExitCode)
	if s := strings.TrimSpace(redact(py.DecodeBytes(r.Stderr))); s != "" {
		lines := strings.Split(s, "\n")
		if len(lines) > 12 { // the last lines carry the error; verbose logs are cut
			lines = append([]string{fmt.Sprintf("(%d earlier lines left out)", len(lines)-12)}, lines[len(lines)-12:]...)
		}
		for i, l := range lines {
			lines[i] = py.Head(l, 300)
		}
		fmt.Fprintf(&b, "stderr:\n%s\n", strings.Join(lines, "\n"))
	}
	for _, e := range events {
		fmt.Fprintf(&b, "event: %s\n", redact(e))
	}
	if err != nil {
		fmt.Fprintf(&b, "result: error: %s\n", NoteText(err.Error()))
	} else {
		b.WriteString("result: ok\n")
	}
	debugf("%s\n", b.String())
}

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)

// redact makes diagnostics safe to attach to a public issue: secrets masked, the home folder shown as ~ and
// anything that looks like an e-mail address replaced.
func redact(s string) string {
	s = secrets.Mask(s)
	if h := config.Home(); h != "" {
		s = strings.ReplaceAll(s, h, "~")
	}
	return emailRe.ReplaceAllString(s, "<email>")
}
