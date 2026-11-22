// Command v010stub stands in for the v0.1.0 release in the installer tests. Its install-skills has no --auto
// flag and prints v0.1.0's messages, so the tests cover the installers' fallback for releases from before --auto.
// It writes a placeholder SKILL.md; the real skill text is tested elsewhere.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "install-skills" {
		fmt.Println("agent-leeway 0.1.0")
		return
	}
	fs := flag.NewFlagSet("install-skills", flag.ContinueOnError)
	h := home()
	claudeRoot := envOr("CLAUDE_CONFIG_DIR", filepath.Join(h, ".claude"))
	codexRoot := envOr("CODEX_HOME", filepath.Join(h, ".codex"))
	claudeDir := filepath.Join(claudeRoot, "skills", "agent-leeway")
	codexDir := filepath.Join(h, ".agents", "skills", "agent-leeway")
	fs.Bool("claude", false, "Claude Code: "+claudeDir)
	fs.Bool("codex", false, "Codex: "+codexDir)
	fs.Bool("dry-run", false, "show what would change; change nothing")
	if err := fs.Parse(os.Args[2:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}
	wrote := false
	for _, t := range []struct{ agent, root, dir string }{{"claude", claudeRoot, claudeDir}, {"codex", codexRoot, codexDir}} {
		if fi, err := os.Stat(t.root); err != nil || !fi.IsDir() {
			continue
		}
		p := filepath.Join(t.dir, "SKILL.md")
		if err := os.MkdirAll(t.dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, []byte("v0.1.0 skill for "+t.agent+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s: write %s\n", t.agent, p)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(os.Stderr, "Neither Claude Code nor Codex was found on this computer; pass --claude or --codex to install anyway.")
		os.Exit(1)
	}
	fmt.Println("Ask your agent for a \"leeway report\" (or a leak check). Remove with `agent-leeway uninstall-skills`.")
}
