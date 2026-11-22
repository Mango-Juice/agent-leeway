---
name: agent-leeway
description: Occasional check-up of this computer's Claude Code and Codex logs with the agent-leeway command. Finds mechanical cost leaks (expired prompt cache, repeated or failing commands, re-reads, oversized tool output) and intent drift (where the agent did something other than what was asked). Use when the user asks for a leak check, a drift check, a token or cost waste report, or a Leeway report.
---

# Leeway (agent-leeway)

Everything here runs the `agent-leeway` program, which is installed separately and found on PATH.
Run the commands exactly as written; they work the same in bash, zsh and PowerShell.

## Commands

- `agent-leeway leak` - cost leaks in the last 30 days, by habit and by cause, direct conversations vs
  automations. Reads local logs only; no model calls. Options: `--since 2w`, `--tool {{AGENT}}`, `--json`.
- `agent-leeway report --lang <en|ko>` - leak check plus the drift results already judged, written as Markdown
  and HTML files on this computer, with a short summary printed. Pass the language the user writes to you in
  (`ko` for Korean, otherwise `en`). `leak` and `drift` take `--lang` too.
- `agent-leeway drift --dry-run --tool {{AGENT}}` - how many {{AGENT_NAME}} sessions are in the window and which
  of them would be judged (the 50 most recent by default; one judge call each). Nothing is sent.{{AGENT_NOTE}}
- `agent-leeway drift --yes --tool {{AGENT}}` - judges those sessions with the official {{AGENT_NAME}} CLI
  ({{AGENT_CLI}}), using the account it is logged in with, then makes one grouping call that groups the problems
  found by meaning (same CLI, same account; `--no-grouping` skips it). On a subscription this counts against the
  plan's usage; with an API key it costs roughly $0.15 to $0.35 per session at list price. Run it only after
  showing the user the `--dry-run` session count and list and getting their explicit approval in this
  conversation. `--max-sessions N` judges a different number of the most recent sessions (0 = all); `--budget
  USD` adds a dollar limit, mainly for API-key users. In a measured run, 30 sessions took about 3.5 minutes (3
  calls at a time, the longest call about 70 seconds). If your command tool has a time limit, use
  `--max-sessions` small enough to finish within it, or run the command in the background. Stopping it early is
  safe; finished sessions are kept and a rerun continues.
- Only {{AGENT_NAME}} sessions are judged by default, because judging the other tool's sessions would use the
  other provider's plan. If the user asks for both tools, use `--tool all` (it runs the other provider's CLI too,
  which needs that CLI installed and logged in). The cost check (`leak`) and the report's cost side always read
  both tools' logs.
- To judge and report in one step: `agent-leeway report --run-drift --yes --tool {{AGENT}} --lang <en|ko>`, with
  the same approval as `drift --yes`.
- `agent-leeway idle` - recent sessions whose prompt cache has probably expired, and what resuming costs.
- `agent-leeway privacy` - where data lives and what leaves the computer.

## Presenting the report

After `agent-leeway report`, present the "What to fix first" findings in your own words or as printed,
whichever reads better in the conversation, then give the two file paths. Answer follow-up questions from the
report files. Keep these rules:

- Name the tool (Claude Code or Codex) whenever a piece of advice is specific to one tool.
- Never compare the two judges' drift rates; they are calibrated differently.
- Never edit instruction files (CLAUDE.md, AGENTS.md) or settings unless the user asks you to.
- Keep the source labels ("Official docs", "Our rule of thumb", ...) when you repeat advice.
- The judges' summaries are the judging model's words (in the user's language for results judged since rubric version +t2, in English before); present them as the judge's view, not as facts.

For other commands, show the output as it is. Amounts are USD at API list prices (list-price equivalents, not a
bill).

## If agent-leeway is not installed

If running `agent-leeway` fails because the command is not found, do not download or install anything on your
own. Show the user the install line for their system and ask whether you should run it:

- macOS or Linux: `{{INSTALL_SH}}`
- Windows (PowerShell): `{{INSTALL_PS1}}`

Run it only after the user says yes in this conversation. It downloads the release from GitHub, checks it
against the release's checksums, installs the program and adds this skill for Claude Code and Codex. Then run
your command again, with the full path the installer printed if `agent-leeway` is still not found (a new
terminal picks up the PATH change).

<!-- agent-leeway skill {{VERSION}} sha256:{{HASH}} (installed by `agent-leeway install-skills`; `agent-leeway uninstall-skills` removes it) -->
