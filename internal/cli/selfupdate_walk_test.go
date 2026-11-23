package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

// A fake agent follows the skill's "When the user asks to update agent-leeway" (internal/assets/skill/SKILL.md). It
// runs the commands the skill names, says only what the skill tells it to, and "runs" the printed update command by
// switching the version (nothing is installed). The transcripts are goldens (testdata/expected-go/walkthrough-*.txt).
type fakeAgent struct {
	t          *testing.T
	lang, tool string
	b          strings.Builder
}

func (a *fakeAgent) user(s string)  { a.b.WriteString("USER: " + s + "\n") }
func (a *fakeAgent) agent(s string) { a.b.WriteString("AGENT: " + s + "\n") }
func (a *fakeAgent) cmd(args ...string) (string, int) {
	a.b.WriteString("  $ agent-leeway " + strings.Join(args, " ") + "\n")
	out, errs, rc := run(a.t, args...)
	for _, l := range strings.Split(strings.TrimRight(out+errs, "\n"), "\n") {
		if l != "" {
			a.b.WriteString("    " + l + "\n")
		}
	}
	return out + errs, rc
}

// update is "When the user asks to update agent-leeway".
func (a *fakeAgent) update(after func()) {
	out, rc := a.cmd("self-update", "--check", "--lang", a.lang)
	if rc != 0 {
		a.agent(strings.TrimSpace(out))
		return
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 1 { // up to date
		a.agent(lines[0])
		return
	}
	command := strings.TrimSpace(lines[2])
	a.b.WriteString("  $ " + command + "\n    (simulated: installs the new version)\n")
	after()
	v, _ := a.cmd("--version")
	if a.lang == "ko" {
		a.agent(fmt.Sprintf("%s 버전으로 업데이트했습니다.", strings.TrimSpace(strings.TrimPrefix(v, "agent-leeway"))))
	} else {
		a.agent(fmt.Sprintf("Updated to %s.", strings.TrimSpace(strings.TrimPrefix(v, "agent-leeway"))))
	}
}

func TestSkillUpdateWalkthrough(t *testing.T) {
	for _, c := range []struct{ lang, tool, method, goos, upd string }{
		{"ko", "codex", methodInstaller, "linux", "Mango-Juice/agent-leeway 최신 버전으로 업데이트해줘"},
		{"en", "claude", methodBrew, "darwin", "Update Mango-Juice/agent-leeway to the latest version"},
	} {
		fixtureEnv(t)
		oldV, oldExe, oldOS := meta.Version, executablePath, hostOS
		meta.Version, hostOS = "0.1.3", c.goos
		executablePath = func() string {
			if c.method == methodBrew {
				return "/opt/homebrew/Cellar/agent-leeway/" + meta.Version + "/bin/agent-leeway"
			}
			return "/home/u/.local/bin/agent-leeway" // privacy-ok: made-up paths
		}
		t.Setenv("HOME", "/home/u")        // privacy-ok: made-up paths
		t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up paths
		tag := "v0.1.6"
		releaseServer(t, &tag)
		a := &fakeAgent{t: t, lang: c.lang, tool: c.tool}
		toNew := func() { meta.Version = strings.TrimPrefix(tag, "v") }
		reset := func() { meta.Version = "0.1.3"; tag = "v0.1.6" }

		a.b.WriteString("### 1. a newer version exists\n")
		a.user(c.upd)
		a.update(toNew)
		a.user(c.upd)
		a.update(toNew)

		a.b.WriteString("\n### 2. offline\n")
		reset()
		tag = ""
		a.user(c.upd)
		a.update(toNew)

		golden(t, "walkthrough-"+c.lang+"-"+c.tool+".txt", a.b.String())
		meta.Version, executablePath, hostOS = oldV, oldExe, oldOS
	}
}
