---
name: agent-leeway
description: Occasional check-up of this computer's Claude Code and Codex logs with the agent-leeway command. Finds mechanical cost leaks (expired prompt cache, repeated or failing commands, re-reads, oversized tool output) and intent drift (where the agent did something other than what was asked). Use when the user asks for a leak check, a drift check, a token or cost waste report, or a Leeway report.
---

# Leeway (agent-leeway)

Everything here runs the `agent-leeway` program, which is installed separately and found on PATH.
Run the commands exactly as written; they work the same in bash, zsh and PowerShell.

## Commands

- `agent-leeway leak` - cost leaks in the last 30 days, by habit and by cause, direct conversations vs
  automations. Reads local logs only; no model calls. Options: `--since 2w`, `--tool claude`, `--json`.
- `agent-leeway report --tool claude --lang <en|ko>` - leak check plus the drift results already judged for
  Claude Code sessions, written as Markdown and HTML files on this computer, with a short summary printed.
  Pass the language the user writes to you in (`ko` for Korean, otherwise `en`). `leak` and `drift` take
  `--lang` too. The cost side always covers both tools.
- `agent-leeway drift --dry-run --tool claude` - how many Claude Code sessions are in the window and which
  of them would be judged (the 50 most recent by default; one judge call each). Nothing is sent. If you are Codex, use `--tool codex` instead of `--tool claude` in these commands.
- `agent-leeway drift --yes --tool claude` - judges those sessions with the official Claude Code CLI
  (`claude -p`), using the account it is logged in with, then makes one grouping call that groups the problems
  found by meaning (same CLI, same account; `--no-grouping` skips it). On a subscription this counts against the
  plan's usage; with an API key it costs roughly $0.15 to $0.35 per session at list price. Run it only after
  showing the user the `--dry-run` session count and list and getting their explicit approval in this
  conversation. `--max-sessions N` judges a different number of the most recent sessions (0 = all); `--budget
  USD` adds a dollar limit, mainly for API-key users. In a measured run, 30 sessions took about 3.5 minutes (3
  calls at a time, the longest call about 70 seconds). If your command tool has a time limit, use
  `--max-sessions` small enough to finish within it, or run the command in the background. Stopping it early is
  safe; finished sessions are kept and a rerun continues.
- Only Claude Code sessions are judged by default, because judging the other tool's sessions would use the
  other provider's plan. If the user asks for both tools, use `--tool all` (it runs the other provider's CLI too,
  which needs that CLI installed and logged in). The cost check (`leak`) and the report's cost side always read
  both tools' logs.
- `agent-leeway idle` - recent sessions whose prompt cache has probably expired, and what resuming costs.
- `agent-leeway privacy` - where data lives and what leaves the computer.

## When the user asks for a report

1. Run `agent-leeway report --tool claude --lang <en|ko>`. If you are Codex, use `--tool codex` instead of `--tool claude` in these commands.
2. If the summary ends with a line starting "Not judged yet" (Korean: "아직 판정하지 않음"), some Claude Code
   sessions in the period were never judged, so the report does not yet show where the agent went off for them.
   Tell the user how many, then run the `agent-leeway drift --dry-run ...` command from that line and show its
   session count and list (shortened if long; say so if it judges fewer sessions than are unjudged). Ask whether
   to judge them now, in one short message that also says: each session is
   one call to the Claude Code CLI on their account (plan usage on a subscription, roughly the estimated
   dollars with an API key); they can judge fewer or other sessions with `--max-sessions N`, `--since 7d` or
   `--project <name>`, for example only the 10 most recent, only the last week, or one project by name.
3. If they answer with a different scope, run the dry run again with it and ask again. Run
   `agent-leeway drift --yes --tool claude` with the same `--since`, `--max-sessions` and `--project` as the
   dry run they approved, only after a clear yes in this conversation, then run the report command again and
   present it. If they decline, present the report as it is.
4. If there is no such line, everything in the period is already judged: say so and present the report without
   asking.

## Presenting the report

After `agent-leeway report`, present the "What to fix first" findings in your own words or as printed,
whichever reads better in the conversation, then give the report's file path (the HTML; the Markdown copy is in
the same folder). Answer follow-up questions from the
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

- macOS or Linux: `curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh`
- Windows (PowerShell): `irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex`

Run it only after the user says yes in this conversation. It downloads the release from GitHub, checks it
against the release's checksums, installs the program and adds this skill for Claude Code and Codex. Then run
your command again, with the full path the installer printed if `agent-leeway` is still not found (a new
terminal picks up the PATH change).

<!-- agent-leeway skill repository copy sha256:7016e85847510f294bd5f1ce2b0a890ce53585d38487f09b0e1e5c42ce950a27 (installed by `agent-leeway install-skills`; `agent-leeway uninstall-skills` removes it) -->
