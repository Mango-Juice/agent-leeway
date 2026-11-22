// Package assets embeds the files the binary ships with: the drift rubric and the agent skill template.
package assets

import _ "embed"

// Rubric is the bundled drift rubric (v2.1, English). Users can copy and edit it (`rubric --copy-to`).
//
//go:embed rubric.md
var Rubric []byte

// SkillTemplate is the agent skill (one SKILL.md for Claude Code and Codex) with {{INSTALL_SH}},
// {{INSTALL_PS1}} and {{VERSION}} placeholders; install.Skill fills them.
//
//go:embed skill/SKILL.md
var SkillTemplate []byte

// Prices is the price table (USD per million tokens, with the source page and date of each row).
//
//go:embed prices.json
var Prices []byte

// Advice is the advice catalogue shown in the report (per finding and tool: what to do, the exact command or
// setting, an optional standing rule to copy, and dated sources). A copy in the settings folder replaces it.
//
//go:embed advice.json
var Advice []byte

// Judges lists the preferred judging model families per tool and the exact versions checked with the fixed test
// set (drift/judges.go reads it).
//
//go:embed judges.json
var Judges []byte
