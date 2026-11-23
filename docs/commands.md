# Commands, options and settings

Every command prints its options with `-h`.

## Commands

| Command | What it does | Sends anything? |
|---|---|---|
| `agent-leeway leak` | Cost leaks by cause and habit, direct conversations vs automation, compared with the previous window | No |
| `agent-leeway drift` | Judges recent sessions for drift, then groups the problems by meaning. Asks before it starts | Session excerpts, to that session's own provider |
| `agent-leeway report` | The cost check plus the drift results judged so far, compared with the previous period of the same length, as an HTML file, a Markdown file and a terminal summary | Only with `--run-drift` |
| `agent-leeway compare <before.json> <after.json>` | Compares two `leak --json` outputs by habit and automation | No |
| `agent-leeway idle [--days 3]` | Recent sessions whose prompt cache has probably expired, and what resuming would cost | No |
| `agent-leeway rubric` | Shows the rubric and its settings S1–S10, or copies it for editing | No |
| `agent-leeway prices` | Shows the price table (USD per million tokens, with sources and dates) | No |
| `agent-leeway privacy` | The folders it uses and what leaves this computer | No |
| `agent-leeway install-skills`, `uninstall-skills` | Add or remove the skill (`--claude`, `--codex`, `--dry-run`) | No |
| `agent-leeway self-update --check` | Says whether a newer release exists and prints the install line | Asks GitHub for the latest release tag |

## Options of `leak`, `drift` and `report`

| Option | Meaning |
|---|---|
| `--since 30d` | Window: `30d`, `2w`, `12h` and so on |
| `--tool claude\|codex\|all` | Which tool's sessions. Default `all` from a terminal; the skill passes the calling tool. In `report` it limits only the drift part |
| `--lang en\|ko` | Language of the summary and report. Default: settings `"language"`, then `LC_ALL` / `LANG`, then English |
| `--claude-dir`, `--codex-dir` | Read logs from another folder (`idle` and `privacy` take these too) |
| `--no-archived` | Skip Codex `archived_sessions` |
| `--refresh` | Ignore the cache and read the logs again |
| `--workers N` | Parallel log readers |
| `--cache DIR` | Another cache folder |

`leak` also takes `--json`, `--no-prev` (no comparison with the previous window) and `--top 5` (sessions to list).

## Options of `drift` (and `report`)

| Option | Meaning |
|---|---|
| `--dry-run` | Show which sessions would be judged and the estimate. Sends nothing |
| `--write-inputs DIR` | Write the exact judge input of each session, and the system prompt, to read before agreeing |
| `--recheck` | Check sessions again with the current model even if another model already checked them under the same rubric (by default a session is checked once, whatever model) |
| `--debug-log` | Write a diagnostics file of the judge calls under the cache folder (`debug/judge-<time>.log`): the CLI and its version, the exact arguments, exit status, the last lines of stderr, and the type of each output event with any error message. It holds no prompt, no session text and no answer; the home folder is shown as `~`, and e-mail addresses and secrets are replaced, so it is safe to attach to an issue. Verbose-logging variables such as `RUST_LOG` are cleared for judge calls. The path is printed. `LEEWAY_DEBUG=1` does the same. When a run fails without it, the last line suggests it |
| `--yes` | Skip the confirmation (needed when there is no terminal) |
| `--max-sessions 50` | Judge at most this many sessions not judged yet, most recent first. `0` = all in the window |
| `--budget USD` | Optional limit in API list-price dollars, checked before each call. No default |
| `--project TEXT` | Only projects whose name contains this text |
| `--include-auto` | Also judge automated sessions |
| `--model claude=opus,codex=gpt-6.2-astra` | Judging models, overriding the automatic choice (see [how-it-works.md](how-it-works.md#judging-models)). A session is judged only by its own provider |
| `--concurrency 3` | Judge calls at the same time |
| `--timeout 30m` | Time limit per call. The call and everything it started are ended |
| `--rubric FILE`, `--settings FILE` | Another rubric file, or a JSON file that overrides S1–S10 |
| `--no-current-instructions` | Do not add your current `CLAUDE.md` and `AGENTS.md` to Claude Code sessions |
| `--no-grouping`, `--grouping claude=off` | Skip the grouping step, for both tools or one |
| `--group-only` | Judge nothing; group the sessions already judged |

`report` also takes `--run-drift` (judge first), `--out DIR` (default: `reports` in the cache folder) and `--quote-chars 160` (longest excerpt).

`report` compares the window with the one just before it, of the same length (`--since 30d`: the 30 days before). Each card on the first screen says Better, Worse or Similar with the two rates, and the whole picture lists what changed most. A part is not compared when:

- the window is under a day;
- cost: the previous period spent under $5 or had fewer than 3 sessions;
- in-depth: either period has fewer than 8 checked sessions, or under 60% of its sessions checked, or the two periods were checked with different judging models or rubric versions;
- Claude Code has deleted the older logs and no saved numbers cover them (the comparison then covers Codex only, or nothing).

The whole picture then says why. Details: [how-it-works.md](how-it-works.md#compared-with-the-previous-period).

## Settings file

Optional: `~/.config/agent-leeway/settings.json`, or `%APPDATA%\agent-leeway\settings.json` on Windows.

```json
{"judge": {"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"},
 "language": "ko",
 "grouping": {"claude": true, "codex": true},
 "rubric": "/path/to/my-rubric.md",
 "rubric_settings": {"S1": "4"},
 "origin": {"auto_entrypoints": ["sdk-cli"], "interactive_originators": ["codex_cli_rs", "codex-tui"]}}
```

The `"origin"` lists decide what counts as automation ([how-it-works.md](how-it-works.md#direct-conversations-and-automation)). A list you give replaces that list's defaults. A list you leave out keeps them.

## Prices

`agent-leeway prices` shows the built-in table. `agent-leeway prices --edit` copies it to the settings folder as `prices.json`, which then replaces the built-in table. When the table is more than 90 days old, or a promotional price in it has ended, `leak` and `drift` print one line saying so.

## The rubric

The judge follows [rubric.md](rubric.md). To change it:

1. `agent-leeway rubric --copy-to my-rubric.md` (`--force` replaces an existing file and backs it up first).
2. Edit the copy.
3. Pass `--rubric my-rubric.md`, or set `"rubric"` in settings.json.

- Part A has settings S1–S10, for example how many repeated minor drifts count as one major drift (default 3). Override them with `rubric_settings` in settings.json, or `--settings FILE` for one run.
- Part B is exactly what the judge sees.
- Part C gives each rule's evidence grade and sources. Rules marked "design choice" or "maintainer default" have no literature behind them.

Results judged with your own rubric are marked "custom rubric".

**Exit codes of `drift`.** 0: done (or nothing to judge). 1: a run that stopped on failures, a usage limit or a setup problem. 2: a usage error. 4: the judge CLI refused the expected judging model (unknown to it, or not allowed for your account). Nothing more was judged with another model; the output names the next model and the exact command to judge with it after you agree.
