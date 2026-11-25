# Privacy: what is read and what is sent

`agent-leeway privacy` prints the exact folders on your computer and the full list of what goes out.

The in-depth check is the only step that sends session content. Claude Code sessions go to Anthropic through your `claude` CLI and are judged by Claude Opus by default. Codex sessions go to OpenAI through your `codex` CLI and are judged by gpt-6.2-astra by default (`--model` changes this). Each uses the login you already have.

| Step | Reads | Sends |
|---|---|---|
| `leak`, `idle`, `compare` | Claude Code transcripts in `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`). Codex sessions in `~/.codex/sessions` and `archived_sessions` (or under `$CODEX_HOME`), including `*.jsonl.zst` | Nothing |
| `drift`: judging | From the logs: your requests and a summary of the agent's actions (commands, edited files, failures, final answers), and instruction files recorded in the log. For Claude Code sessions without them, your current `CLAUDE.md`, `CLAUDE.local.md` and `AGENTS.md` (`--no-current-instructions` turns this off) | The session excerpt, to the provider that produced the session, through your own CLI login |
| `drift`: grouping (after judging; `--no-grouping` turns it off) | That tool's judged problems: short ids, rule and cause codes, the cited instruction, your own quoted words, and the judge's summaries. The agent's turns are not sent | One call per tool, to the same provider's CLI, under the same lock-down |
| `report` | The results above. From Claude Code's `settings.json`, only `cleanupPeriodDays` and the file's date, to tell deleted logs from no use. For "Your setup": only `skillOverrides` from Claude Code's user, project and local `settings.json`, and only the `project_doc_max_bytes` line of Codex's `config.toml` | Nothing. The HTML file loads nothing from the network |

- The cache and reports stay in `~/.cache/agent-leeway` (`%LOCALAPPDATA%\agent-leeway` on Windows).
- `report` saves each window's cost in numbers only (spend, waste, per-habit amounts, session count, the largest single session's waste and its share of spend; no text) as `compare/*.json` in the cache. A later report compares with it when Claude Code has deleted that window's logs.
- After each in-depth check, `drift` appends one numbers-only record per tool to `compare/checks.jsonl` in the cache. `report` can append one too, when it rebuilds a record from the results of a check made by 0.1.5 or 0.1.6. A record holds: when, the window, how many sessions were checked and how many had serious misses, counts per kind of problem, the judging model and rubric, how the sessions were chosen, the scope, and the dates and session ids of the checked sessions. The session ids are stored as they are; they are the same ids the results cache already holds. A scope of one project is stored as a hash of its name with a random salt made once per install (`compare/salt`), so the name cannot be recovered by guessing common names; never the name itself. No text, no paths. Records and saved costs older than 400 days are deleted when a new one is written. The results cache stays the source of every verdict; the records are used only to compare.
- `self-update --check` asks GitHub for the latest release tag. Prices come from a local table.
- "Your setup" uses, from the logs, the names of the skills in the list and the number of characters each takes there, the names of skills listed without a description, the names of the skills invoked (by the agent or by a command you typed; only names made of letters, digits and `. _ - + @ :`, up to 200 characters) and how many times (to list the least used first and to find the ones not used), the line count and path of each `CLAUDE.md` file Claude Code loaded, and the size of the `AGENTS.md` text Codex sent and whether it ends with a line break. For Codex skills: the names, folders and line lengths of the skills in the list Codex sent, the names of skills you picked, and, from the inputs of tool calls (commands and other tool arguments): the folder names that follow a `skills` or `.system` folder and the names of folders that hold a `SKILL.md` or a `scripts` folder (only names made of letters, digits, `. _ - + @` and an escaped space, up to 100 characters; anything else, such as command text, is not kept as text), and a hash of every folder path a path in the input passes through, and of the folder a call runs in (its `workdir` or `cwd`, a `cd`, a `-C`) joined with the leading parts, up to three, of each relative path in the same call (up to 20,000 path hashes and 2,000 joined ones per log file, besides those of the skills in that file's own list). The hashes are 64-bit FNV-1a values without a salt: they let the report recognize a skill's whole folder path (a custom root, a read of its `references/`) without keeping the path, but a known path can be hashed and compared, so they are not a secret. The commands and arguments themselves are not kept. Their text is not kept. These counts and names stay in the local cache of parsed logs and in your report; they are not written to the comparison records.
- For screenshots left in the conversation, only an image's width and height are read, from its header in the log: the first bytes for PNG, GIF and WebP; for a JPEG, the segment headers up to the frame header, at most 4 MB and 64 segments in. The image is never decoded, stored, hashed or sent.
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
