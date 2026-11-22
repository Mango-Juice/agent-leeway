// Package install puts the agent-leeway skill where Claude Code and Codex look for skills, and removes it.
//
//	Claude Code: <CLAUDE_CONFIG_DIR or ~/.claude>/skills/agent-leeway/SKILL.md
//	Codex:       ~/.agents/skills/agent-leeway/SKILL.md   (Codex custom prompts were removed in 0.118.0)
//
// One SKILL.md serves both. It only tells the agent to run `agent-leeway <command>` from PATH; it contains no
// shell scripts. A SKILL.md that was not written by agent-leeway is backed up before it is replaced and is never
// removed by uninstall.
package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

// marker identifies a SKILL.md written by agent-leeway.
const marker = "<!-- agent-leeway skill "

// Skill renders the SKILL.md text for this build (install lines and version filled in).
func Skill() []byte { return SkillFor(meta.Version, "claude") }

// SkillForAgent is the copy installed for one agent (claude or codex): it judges that agent's sessions by default.
func SkillForAgent(agent string) []byte { return SkillFor(meta.Version, agent) }

// RepoCopy is the version-independent copy published in the repository (skill/agent-leeway/SKILL.md),
// for people who install skills with `npx skills add`.
const RepoCopy = "repository copy"

// SkillFor renders SKILL.md with the given version text for one agent ("claude" or "codex"; "" = the repository
// copy, used by either agent). The copy passes its own agent to --tool, so the drift check judges only the
// calling tool's sessions by default. Its last line records the SHA-256 of everything above it, so a copy the user
// edited is recognized as theirs (backed up, never deleted).
func SkillFor(version, agent string) []byte {
	tool, name, cli, note := agent, map[string]string{"claude": "Claude Code", "codex": "Codex"}[agent], map[string]string{"claude": "`claude -p`", "codex": "`codex exec`"}[agent], ""
	if agent == "" {
		tool, name, cli = "claude", "Claude Code", "`claude -p`"
		note = " If you are Codex, use `--tool codex` instead of `--tool claude` in these commands."
	}
	r := strings.NewReplacer("{{INSTALL_SH}}", meta.InstallSh(), "{{INSTALL_PS1}}", meta.InstallPs1(), "{{VERSION}}", version,
		"{{AGENT}}", tool, "{{AGENT_NAME}}", name, "{{AGENT_CLI}}", cli, "{{AGENT_NOTE}}", note)
	text := r.Replace(string(assets.SkillTemplate))
	i := strings.LastIndex(text, marker)
	sum := sha256.Sum256([]byte(text[:i]))
	return []byte(strings.Replace(text, "{{HASH}}", hex.EncodeToString(sum[:]), 1))
}

// Target is one agent's skill location.
type Target struct {
	Agent string // "claude" or "codex"
	Dir   string // .../skills/agent-leeway
	Root  string // the agent's own folder; its presence means the agent is installed
}

// Targets lists the skill locations.
func Targets() []Target {
	home := config.Home()
	return []Target{
		{Agent: "claude", Dir: filepath.Join(config.ClaudeConfigDir(), "skills", config.Name), Root: config.ClaudeConfigDir()},
		{Agent: "codex", Dir: filepath.Join(home, ".agents", "skills", config.Name), Root: config.CodexHome()},
	}
}

// Pick returns the targets for the chosen agents; none chosen = every agent found on this computer.
func Pick(claude, codex bool) []Target {
	var out []Target
	for _, t := range Targets() {
		if (claude && t.Agent == "claude") || (codex && t.Agent == "codex") {
			out = append(out, t)
		} else if !claude && !codex {
			if fi, err := os.Stat(t.Root); err == nil && fi.IsDir() {
				out = append(out, t)
			}
		}
	}
	return out
}

// ours: the file carries our marker line and everything above it is exactly what was written.
func ours(b []byte) bool {
	i := bytes.LastIndex(b, []byte(marker))
	if i < 0 {
		return false
	}
	line := string(b[i:])
	j := strings.Index(line, "sha256:")
	if j < 0 || len(line) < j+7+64 {
		return false
	}
	sum := sha256.Sum256(b[:i])
	return line[j+7:j+7+64] == hex.EncodeToString(sum[:])
}

// Install writes SKILL.md for each target. dryRun only reports. Returns one line per action.
func Install(targets []Target, dryRun bool) ([]string, error) {
	var log []string
	for _, t := range targets {
		body := SkillForAgent(t.Agent)
		p := filepath.Join(t.Dir, "SKILL.md")
		old, err := os.ReadFile(p)
		switch {
		case err == nil && bytes.Equal(old, body):
			log = append(log, fmt.Sprintf("%s: already up to date (%s)", t.Agent, p))
			continue
		case err == nil && !ours(old):
			bak := p + ".bak-" + time.Now().Format("20060102-150405")
			log = append(log, fmt.Sprintf("%s: existing %s was not written by %s; backed up to %s", t.Agent, p, config.Name, bak))
			if !dryRun {
				if err := os.Rename(p, bak); err != nil {
					return log, err
				}
			}
		}
		log = append(log, fmt.Sprintf("%s: write %s", t.Agent, p))
		if dryRun {
			continue
		}
		if err := os.MkdirAll(t.Dir, 0o755); err != nil {
			return log, err
		}
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, body, 0o644); err != nil {
			return log, err
		}
		if err := os.Rename(tmp, p); err != nil {
			return log, err
		}
	}
	return log, nil
}

// Uninstall removes SKILL.md files written by agent-leeway (and the folder when it is left empty).
func Uninstall(targets []Target, dryRun bool) ([]string, error) {
	var log []string
	for _, t := range targets {
		p := filepath.Join(t.Dir, "SKILL.md")
		b, err := os.ReadFile(p)
		if err != nil {
			log = append(log, fmt.Sprintf("%s: nothing to remove (%s)", t.Agent, p))
			continue
		}
		if !ours(b) {
			log = append(log, fmt.Sprintf("%s: %s was not written by %s, or was changed after install; left in place", t.Agent, p, config.Name))
			continue
		}
		log = append(log, fmt.Sprintf("%s: remove %s", t.Agent, p))
		if dryRun {
			if bak := newestBackup(p); bak != "" {
				log = append(log, fmt.Sprintf("%s: restore your earlier SKILL.md from %s", t.Agent, bak))
			}
			continue
		}
		if err := os.Remove(p); err != nil {
			return log, err
		}
		if bak := newestBackup(p); bak != "" {
			if err := os.Rename(bak, p); err != nil {
				return log, err
			}
			log = append(log, fmt.Sprintf("%s: restored your earlier SKILL.md from %s", t.Agent, bak))
			continue
		}
		if ents, err := os.ReadDir(t.Dir); err == nil && len(ents) == 0 {
			_ = os.Remove(t.Dir)
		}
	}
	return log, nil
}

// newestBackup is the latest SKILL.md.bak-<time> made by install-skills ("" when none).
func newestBackup(p string) string {
	m, _ := filepath.Glob(p + ".bak-*")
	if len(m) == 0 {
		return ""
	}
	sort.Strings(m) // the time stamp sorts by date
	return m[len(m)-1]
}
