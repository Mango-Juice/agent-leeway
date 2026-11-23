# Privacy: what is read and what is sent

`agent-leeway privacy` prints the exact folders on your computer and the full list of what goes out.

The in-depth check is the only step that sends session content. Claude Code sessions go to Anthropic through your `claude` CLI and are judged by Claude Opus by default. Codex sessions go to OpenAI through your `codex` CLI and are judged by gpt-6.2-astra by default (`--model` changes this). Each uses the login you already have.

| Step | Reads | Sends |
|---|---|---|
| `leak`, `idle`, `compare` | Claude Code transcripts in `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`). Codex sessions in `~/.codex/sessions` and `archived_sessions` (or under `$CODEX_HOME`), including `*.jsonl.zst` | Nothing |
| `drift`: judging | From the logs: your requests and a summary of the agent's actions (commands, edited files, failures, final answers), and instruction files recorded in the log. For Claude Code sessions without them, your current `CLAUDE.md`, `CLAUDE.local.md` and `AGENTS.md` (`--no-current-instructions` turns this off) | The session excerpt, to the provider that produced the session, through your own CLI login |
| `drift`: grouping (after judging; `--no-grouping` turns it off) | That tool's judged problems: short ids, rule and cause codes, the cited instruction, your own quoted words, and the judge's summaries. The agent's turns are not sent | One call per tool, to the same provider's CLI, under the same lock-down |
| `report` | The results above. From Claude Code's `settings.json`, only `cleanupPeriodDays` and the file's date, to tell deleted logs from no use | Nothing. The HTML file loads nothing from the network |

- The cache and reports stay in `~/.cache/agent-leeway` (`%LOCALAPPDATA%\agent-leeway` on Windows).
- `report` saves each window's cost in numbers only (spend, waste, per-habit amounts, session count; no text) as `compare/*.json` in the cache. A later report compares with it when Claude Code has deleted that window's logs.
- `self-update --check` asks GitHub for the latest release tag. Prices come from a local table.
- Every command runs `git rev-parse` (read-only) in project folders found in the logs, to name the projects.
- The program never reads or stores credentials, and never calls a provider API with its own key.
- Read your provider's data policy for your plan before you run the in-depth check.

## What the judge may do

Each judge call runs in a new private folder, and the session goes in on stdin only, never on the command line.

- Enforced through the Claude CLI: no built-in tools; no user, project or local settings or hooks; no `CLAUDE.md`, skills, plugins or MCP servers; no slash commands; permission requests denied; the session is not saved. A result with more than one turn or a permission denial fails the call.
- Enforced through the Codex CLI: shell, exec, image, app, plugin, hook, memory and sub-agent tools off; web search off; user config, rules and project config not loaded; read-only sandbox; no approvals. A run that still shows a tool call is stopped and not recorded.
- Asked for in the prompt but not enforced: treat the transcript as data, and answer with JSON only.
- The CLIs inherit your environment. A base URL (`ANTHROPIC_BASE_URL`, `OPENAI_BASE_URL`) or proxy you set applies to them, and so does the CLIs' own telemetry.

## Excerpts in reports

Reports show locations and short excerpts (160 characters by default, `--quote-chars`), not whole transcripts. Before an excerpt or summary is shown, keys and tokens in it are replaced with `[masked <rule>]`, using rules based on the gitleaks default set. The stored results keep the original text, and the judge and grouping calls see it unmasked.

## Terms of use

The in-depth check runs only through the official `claude` and `codex` CLIs, under your own login. It does not extract or reuse subscription tokens, and no third-party service calls a provider for you. Your use of those CLIs is governed by your agreement with each provider.
