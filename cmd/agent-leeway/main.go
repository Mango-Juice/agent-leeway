// Command agent-leeway is an occasional check-up of AI coding agent logs (Claude Code, Codex):
// mechanical cost leaks and intent drift. Everything runs locally except the drift check, which sends each
// selected session to its own provider's official CLI after showing an estimate and asking.
package main

import (
	"os"
	_ "time/tzdata" // local-time windows work on Windows without a time zone database

	"github.com/Mango-Juice/agent-leeway/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
